package ui

import (
	"jandy.cz/winfly/internal/systemdialogs"
	"jandy.cz/winfly/internal/ui/win32"
)

// Size of the inside of the main window, in layout units.
const (
	mainWindowWidth  = 1000
	mainWindowHeight = 462
)

// mainWindow is the window shown at start-up: the list of apps to install,
// the details of the selected app, and buttons for the other tools.
type mainWindow struct {
	app  *app
	hwnd win32.HWND

	appList   listBox
	selectAll control
	install   control
	details   appDetails
	logButton control
}

func newMainWindow(a *app) (*mainWindow, error) {
	w := &mainWindow{app: a}
	hwnd, err := a.createTopLevelWindow(win32.TopLevelWindow{
		ClassName:    mainWindowClass,
		Title:        a.cfg.ProductName,
		Style:        win32.WS_FIXEDWINDOW,
		ClientWidth:  mainWindowWidth,
		ClientHeight: mainWindowHeight,
	}, w.handleMessage)
	if err != nil {
		return nil, err
	}
	w.hwnd = hwnd

	if err := w.createControls(); err != nil {
		win32.DestroyWindow(hwnd)
		return nil, err
	}
	w.fillAppList()
	return w, nil
}

// createControls lays out the window:
//
//	┌ Výběr aplikací ─┐ ┌ Přehled aplikace ─────────────────┐
//	│ app list        │ │ Název:      [                    ] │
//	│                 │ │ Popis:      [                    ] │
//	│ [Vybrat vše]    │ │ ...                                │
//	│ [Nainstalovat]  │ │                                    │
//	└─────────────────┘ └────────────────────────────────────┘
//	[Napájení] [Doména / skupina]     [Systémové informace] [Log]
func (w *mainWindow) createControls() error {
	f := w.app.newForm(w.hwnd)

	content, toolButtons := clientArea(mainWindowWidth, mainWindowHeight).splitBottom(buttonHeight, spacing)
	appsGroup, detailsGroup := content.splitLeft(320, spacing)

	// Left: the app list with its two buttons underneath.
	listArea, listButtons := f.groupBox("Výběr aplikací", appsGroup).splitBottom(buttonHeight, spacing)
	w.appList = f.multiSelectListBox(listArea, idAppList)
	selectAllBox, installBox := listButtons.splitLeft(104, smallSpacing)
	w.selectAll = f.button("Vybrat vše", selectAllBox, idSelectAllButton)
	w.install = f.button("Nainstalovat vybrané", installBox, idInstallButton)

	// Right: details of the selected app.
	w.details = newAppDetails(f, detailsGroup)

	// Bottom: tools that open other windows. Settings on the left, our own
	// windows on the right.
	powerBox, rest := toolButtons.splitLeft(200, smallSpacing)
	domainBox, rest := rest.splitLeft(200, smallSpacing)
	rest, logBox := rest.splitRight(150, smallSpacing)
	_, systemInfoBox := rest.splitRight(180, smallSpacing)
	f.button("Otevřít nastavení napájení", powerBox, idPowerOptionsButton)
	f.button("Otevřít doménu / skupinu", domainBox, idDomainButton)
	f.button("Systémové informace", systemInfoBox, idSystemInfoButton)
	w.logButton = f.button("Zobrazit log", logBox, idLogButton)

	return f.err
}

// show makes the window visible with the keyboard focus in the app list.
func (w *mainWindow) show() {
	win32.Show(w.hwnd)
	win32.SetFocus(w.appList.hwnd)
}

func (w *mainWindow) handleMessage(msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	a := w.app
	switch msg {
	case win32.WM_COMMAND:
		w.onCommand(win32.LowWord(wParam), win32.HighWord(wParam))
		return 0, true

	// Results handed over from background goroutines.
	case wmLogLinesQueued:
		a.log.showQueuedLines()
		return 0, true
	case wmInstallFinished:
		a.finishInstallation()
		return 0, true
	case wmSystemInfoLoaded:
		a.systemInfo.showLoadedReport()
		return 0, true

	// Do not let Windows shut down or the user close the window while an
	// installer is running.
	case win32.WM_QUERYENDSESSION:
		if a.installing.Load() {
			return 0, true
		}
		return 1, true
	case win32.WM_CLOSE:
		if a.installing.Load() {
			win32.ShowWarning(w.hwnd, a.cfg.ProductName, "Instalátor stále běží. Před zavřením tohoto okna jej dokončete.")
			return 0, true
		}
		return 0, false // the default closes the window

	case win32.WM_DESTROY:
		// Owned windows (log, system information) are destroyed with this one.
		win32.PostQuitMessage()
		return 0, true
	}
	return 0, false
}

func (w *mainWindow) onCommand(id, notification int) {
	if notification == win32.LBN_SELCHANGE && id == idAppList {
		w.showFocusedAppDetails()
		return
	}
	if notification != win32.BN_CLICKED {
		return
	}

	a := w.app
	switch id {
	case idSelectAllButton:
		w.appList.selectAll()
		w.showFocusedAppDetails()
	case idInstallButton:
		a.installSelectedApps()
	case idPowerOptionsButton:
		a.openSystemDialog("Nastavení napájení", systemdialogs.OpenPowerOptions)
	case idDomainButton:
		a.openSystemDialog("Doména nebo pracovní skupina", systemdialogs.OpenDomainOrWorkgroup)
	case idSystemInfoButton:
		a.systemInfo.open()
	case idLogButton:
		a.log.toggle()
	}
}

// fillAppList puts the enabled apps into the list, or disables the list when
// there are none.
func (w *mainWindow) fillAppList() {
	a := w.app
	if len(a.apps) == 0 {
		w.setInstallControlsEnabled(false)
		w.details.showNoApps(a.appListErr)
		return
	}
	for _, app := range a.apps {
		w.appList.add(app.Name)
	}
	w.appList.setSelected(0, true)
	w.showFocusedAppDetails()
}

// showFocusedAppDetails shows the app the user clicked last, or the first
// selected one.
func (w *mainWindow) showFocusedAppDetails() {
	apps := w.app.apps
	if len(apps) == 0 {
		return
	}
	index := w.appList.focusedIndex()
	if index < 0 || index >= len(apps) {
		selected := w.appList.selectedIndexes()
		if len(selected) == 0 {
			w.details.clear("Vyberte aplikaci")
			return
		}
		index = selected[0]
	}
	w.details.show(apps[index], installerTypeText(apps[index], w.app.imageStatuses))
}

// setInstallControlsEnabled enables or disables everything used to start an
// installation. Without any apps they stay disabled.
func (w *mainWindow) setInstallControlsEnabled(enabled bool) {
	enabled = enabled && len(w.app.apps) > 0
	w.appList.setEnabled(enabled)
	w.selectAll.setEnabled(enabled)
	w.install.setEnabled(enabled)
}

// updateLogButtonText makes the log button say what clicking it will do.
func (w *mainWindow) updateLogButtonText() {
	if w.app.log != nil && w.app.log.isVisible() {
		w.logButton.setText("Skrýt log")
	} else {
		w.logButton.setText("Zobrazit log")
	}
}

// openSystemDialog opens one of the Windows settings dialogs and logs the result.
func (a *app) openSystemDialog(name string, open func() error) {
	if err := open(); err != nil {
		a.logf("Nepodařilo se otevřít %s: %v", name, err)
		win32.ShowError(a.main.hwnd, name, err.Error())
		return
	}
	a.logf("Otevřeno: %s.", name)
}
