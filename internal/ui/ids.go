package ui

import "jandy.cz/winfly/internal/ui/win32"

// Window class names. Each top-level window needs its own registered class.
const (
	mainWindowClass       = "WinFlyMainWindow"
	logWindowClass        = "WinFlyLogWindow"
	systemInfoWindowClass = "WinFlySystemInfoWindow"
)

// Control IDs. Windows reports clicks and selection changes as WM_COMMAND or
// WM_NOTIFY messages carrying the ID of the control, so only controls the user
// interacts with need one.
const (
	// Main window.
	idAppList = 1001 + iota
	idSelectAllButton
	idInstallButton
	idPowerOptionsButton
	idDomainButton
	idSystemInfoButton
	idLogButton

	// System information window.
	idSystemInfoTabs
	idMemoryModuleList
	idGPUList
	idDriveList
)

// Messages that background goroutines post to the main window to hand their
// results over to the UI thread. Only the UI thread may touch controls.
const (
	wmLogLinesQueued   = win32.WM_APP + 1
	wmInstallFinished  = win32.WM_APP + 2
	wmSystemInfoLoaded = win32.WM_APP + 3
)
