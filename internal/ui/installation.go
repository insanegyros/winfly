package ui

import (
	"context"
	"errors"
	"fmt"

	"jandy.cz/winfly/internal/config"
	"jandy.cz/winfly/internal/install"
	"jandy.cz/winfly/internal/ui/win32"
)

const installTitle = "Instalace aplikací"

// installResult is what the background installation reports back to the UI.
type installResult struct {
	err error // all failures joined together; nil when every app installed
}

// installSelectedApps downloads and installs the apps selected in the list,
// one after another, in a background goroutine. The install controls are
// disabled until it finishes.
func (a *app) installSelectedApps() {
	apps := a.selectedApps()
	if len(apps) == 0 {
		win32.ShowWarning(a.main.hwnd, a.cfg.ProductName, "Vyberte alespoň jednu aplikaci.")
		return
	}
	if !a.installing.CompareAndSwap(false, true) {
		win32.ShowWarning(a.main.hwnd, a.cfg.ProductName, "Instalace aplikace již probíhá.")
		return
	}

	a.main.setInstallControlsEnabled(false)
	a.logf("Spouští se: %s", installTitle)
	go func() {
		err := a.installApps(context.Background(), apps)
		a.installResults <- installResult{err: err}
		a.wakeUI(wmInstallFinished)
	}()
}

// selectedApps returns the apps selected in the list, in list order.
func (a *app) selectedApps() []config.AppConfig {
	var apps []config.AppConfig
	for _, index := range a.main.appList.selectedIndexes() {
		if index >= 0 && index < len(a.apps) {
			apps = append(apps, a.apps[index])
		}
	}
	return apps
}

// installApps runs on a background goroutine. A failed app is logged and
// skipped, so one broken installer does not stop the rest.
func (a *app) installApps(ctx context.Context, apps []config.AppConfig) error {
	var failures []error
	restartRequired := false

	for _, app := range apps {
		restart, err := a.installApp(ctx, app)
		if err != nil {
			// The log gets the technical details, the dialog a readable message.
			a.logf("%s: operace selhala: %v", app.Name, err)
			failures = append(failures, fmt.Errorf("%s: %s", app.Name, install.UserMessage(err)))
			continue
		}
		restartRequired = restartRequired || restart
	}

	if restartRequired {
		a.logf("Alespoň jeden instalátor požaduje restart systému Windows.")
	}
	return errors.Join(failures...)
}

// installApp downloads one installer and runs it. A disk image that already
// failed validation is skipped before it is downloaded again.
func (a *app) installApp(ctx context.Context, app config.AppConfig) (restartRequired bool, err error) {
	if err := a.imageStatuses.CheckAllowed(app); err != nil {
		return false, err
	}
	installerPath, err := a.downloader.Fetch(ctx, app, a.logf)
	if err != nil {
		return false, err
	}
	result, err := install.Run(ctx, app, installerPath, a.logf)
	a.imageStatuses.Record(app, result, err)
	if err != nil {
		return false, err
	}
	return result.RestartRequired, nil
}

// finishInstallation runs on the UI thread once the background work is done.
func (a *app) finishInstallation() {
	for {
		select {
		case result := <-a.installResults:
			a.installing.Store(false)
			a.main.setInstallControlsEnabled(true)
			a.main.showFocusedAppDetails() // image states may have changed
			if result.err != nil {
				a.logf("Selhání: %s: %v", installTitle, result.err)
				win32.ShowError(a.main.hwnd, installTitle, result.err.Error())
			} else {
				a.logf("Dokončeno: %s", installTitle)
				win32.ShowInfo(a.main.hwnd, installTitle, "Instalace vybraných aplikací byla dokončena.")
			}
		default:
			return
		}
	}
}
