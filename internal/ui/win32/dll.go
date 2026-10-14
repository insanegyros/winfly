// Package win32 is a small, hand-written binding to the parts of the Win32 API
// that the user interface needs: windows, standard controls, fonts, DPI and
// the message loop.
//
// It deliberately knows nothing about the provisioner itself. Every function
// here is a thin, typed wrapper around one or two Win32 calls, so that the ui
// package can read like plain Go instead of a wall of syscalls.
package win32

import "syscall"

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")

	// Window classes and window lifetime.
	procRegisterClassExW   = user32.NewProc("RegisterClassExW")
	procCreateWindowExW    = user32.NewProc("CreateWindowExW")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procAdjustWindowRectEx = user32.NewProc("AdjustWindowRectEx")
	procLoadCursorW        = user32.NewProc("LoadCursorW")

	// Window state.
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procEnableWindow        = user32.NewProc("EnableWindow")
	procSetWindowTextW      = user32.NewProc("SetWindowTextW")
	procGetClientRect       = user32.NewProc("GetClientRect")
	procMoveWindow          = user32.NewProc("MoveWindow")
	procSetFocus            = user32.NewProc("SetFocus")
	procGetAncestor         = user32.NewProc("GetAncestor")

	// Messages and the message loop.
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procIsDialogMessageW = user32.NewProc("IsDialogMessageW")
	procSendMessageW     = user32.NewProc("SendMessageW")
	procPostMessageW     = user32.NewProc("PostMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procMessageBoxW      = user32.NewProc("MessageBoxW")

	// Process, DPI, fonts and common controls.
	procGetModuleHandleW     = kernel32.NewProc("GetModuleHandleW")
	procGetDpiForSystem      = user32.NewProc("GetDpiForSystem")
	procCreateFontW          = gdi32.NewProc("CreateFontW")
	procDeleteObject         = gdi32.NewProc("DeleteObject")
	procInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")
)
