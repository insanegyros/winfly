// Package ui is the Windows user interface of ESSENS Provisioner, built
// directly on Win32 without any GUI framework.
//
// How the files fit together:
//
//   - ui.go, app.go       start-up and the state shared by all windows
//   - ids.go              control IDs and internal message numbers
//   - layout.go           spacing constants and the box type used for layout
//   - controls.go         small typed wrappers around control handles
//   - form.go             basic controls: labels, buttons, text boxes, lists
//   - sections.go         bigger building blocks made of those controls
//   - detail_list.go      a list whose selected item fills a details panel
//   - main_window.go      the main window and its buttons
//   - app_details.go      the "Přehled aplikace" panel of the main window
//   - installation.go     installing the selected apps in the background
//   - log_window.go       the log window and the logf function
//   - sysinfo_window.go   the system information window and its tabs
//   - sysinfo_pages.go    the content of each system information tab
//   - format.go           formatting of values for display
//
// The raw Win32 calls live in the win32 subpackage.
package ui

import (
	"errors"
	"fmt"

	"jandy.cz/ess-provisioner/internal/config"
	"jandy.cz/ess-provisioner/internal/download"
	"jandy.cz/ess-provisioner/internal/ui/win32"
)

// Run shows the main window and blocks until the user closes it.
// appListErr is the error from loading the remote app list, if any; the
// window then shows an empty, disabled list.
//
// Run must be called from a goroutine locked to its OS thread.
func Run(cfg *config.Config, appListErr error) error {
	if cfg == nil {
		return errors.New("konfigurace je nil")
	}
	downloader, err := download.New(cfg.Network, cfg.CacheDirectory)
	if err != nil {
		return fmt.Errorf("inicializace správce stahování: %w", err)
	}
	if err := win32.InitCommonControls(); err != nil {
		return err
	}
	for _, className := range []string{mainWindowClass, logWindowClass, systemInfoWindowClass} {
		if err := win32.RegisterWindowClass(className); err != nil {
			return err
		}
	}

	a := newApp(cfg, appListErr, downloader)
	defer a.close()

	if err := a.createWindows(); err != nil {
		return err
	}
	a.main.show()
	a.logStartupState()

	return win32.RunMessageLoop()
}
