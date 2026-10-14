package ui

import (
	"sync/atomic"

	"jandy.cz/winfly/internal/config"
	"jandy.cz/winfly/internal/download"
	"jandy.cz/winfly/internal/systeminfo"
	"jandy.cz/winfly/internal/ui/win32"
)

// app is the state shared by all windows of the program.
//
// Only the UI thread may touch windows and controls. Background goroutines
// (downloads, installers, system information) send their results through
// the channels below and post a message to the main window, which then picks
// the results up on the UI thread.
type app struct {
	cfg        *config.Config
	apps       []config.AppConfig // the enabled apps, in the order of the list
	appListErr error              // why the app list could not be loaded, if it failed
	downloader *download.Manager

	font  win32.Font
	scale scaler

	main       *mainWindow
	log        *logWindow
	systemInfo *systemInfoWindow

	installing        atomic.Bool
	pendingLogLines   chan string
	installResults    chan installResult
	systemInfoResults chan systeminfo.Report
	systemInfoLoading atomic.Bool
}

func newApp(cfg *config.Config, appListErr error, downloader *download.Manager) *app {
	dpi := win32.SystemDPI()
	return &app{
		cfg:               cfg,
		apps:              cfg.EnabledApps(),
		appListErr:        appListErr,
		downloader:        downloader,
		font:              win32.CreateFont("Segoe UI", 9, dpi),
		scale:             scaler{dpi: dpi},
		pendingLogLines:   make(chan string, 256),
		installResults:    make(chan installResult, 8),
		systemInfoResults: make(chan systeminfo.Report, 1),
	}
}

// createWindows creates all three windows. Only the main window is shown;
// the log and system information windows stay hidden until opened.
func (a *app) createWindows() error {
	var err error
	if a.main, err = newMainWindow(a); err != nil {
		return err
	}
	if a.log, err = newLogWindow(a); err != nil {
		return err
	}
	if a.systemInfo, err = newSystemInfoWindow(a); err != nil {
		return err
	}
	return nil
}

func (a *app) close() {
	win32.DeleteFont(a.font)
}

// newForm starts building the controls of a window with the app's font and scaling.
func (a *app) newForm(parent win32.HWND) *form {
	return newForm(parent, a.font, a.scale)
}

// createTopLevelWindow creates a hidden window whose inside is the given
// size in layout units, and routes its messages to handler.
func (a *app) createTopLevelWindow(spec win32.TopLevelWindow, handler win32.MessageHandler) (win32.HWND, error) {
	spec.ClientWidth = a.scale.px(spec.ClientWidth)
	spec.ClientHeight = a.scale.px(spec.ClientHeight)
	hwnd, err := win32.CreateTopLevelWindow(spec)
	if err != nil {
		return 0, err
	}
	win32.SetMessageHandler(hwnd, handler)
	return hwnd, nil
}

// wakeUI asks the UI thread to process one of the wm* messages from ids.go.
// Safe to call from any goroutine.
func (a *app) wakeUI(message uint32) {
	win32.PostMessage(a.main.hwnd, message, 0, 0)
}

func (a *app) logStartupState() {
	a.logf("Připraveno. WinFly běží s oprávněním správce.")
	switch {
	case a.appListErr != nil:
		a.logf("Vzdálený seznam aplikací se nepodařilo načíst: %v", a.appListErr)
		a.logf("Instalace aplikací je zakázána a seznam je prázdný.")
	case len(a.apps) == 0:
		a.logf("Vzdálená konfigurace neobsahuje žádnou povolenou aplikaci.")
	default:
		a.logf("Vzdálený seznam aplikací byl načten. Počet povolených aplikací: %d.", len(a.apps))
	}
}
