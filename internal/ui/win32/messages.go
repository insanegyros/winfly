package win32

import (
	"fmt"
	"unsafe"
)

// RunMessageLoop delivers messages to the windows of this thread until
// PostQuitMessage is called.
//
// Messages for our windows first pass through IsDialogMessage, which gives
// every window dialog-style keyboard handling: Tab and Shift+Tab move between
// controls, and Escape sends a WM_COMMAND with IDCANCEL.
func RunMessageLoop() error {
	var m msg
	for {
		result, _, err := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		switch int32(result) {
		case -1:
			return fmt.Errorf("načtení zprávy GetMessageW selhalo: %w", callError(err))
		case 0:
			return nil // WM_QUIT
		}

		if root := rootWindow(m.Hwnd); root != 0 {
			handledAsDialog, _, _ := procIsDialogMessageW.Call(uintptr(root), uintptr(unsafe.Pointer(&m)))
			if handledAsDialog != 0 {
				continue
			}
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// PostQuitMessage makes RunMessageLoop return.
func PostQuitMessage() {
	procPostQuitMessage.Call(0)
}

// SendMessage delivers a message to a window and waits for the result.
func SendMessage(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	result, _, _ := procSendMessageW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return result
}

// PostMessage queues a message for a window and returns immediately. It is
// safe to call from any goroutine, which makes it the way background work
// wakes up the UI thread.
func PostMessage(hwnd HWND, msg uint32, wParam, lParam uintptr) {
	procPostMessageW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
}

// LowWord returns bits 0-15 of a message parameter.
func LowWord(value uintptr) int {
	return int(value & 0xFFFF)
}

// HighWord returns bits 16-31 of a message parameter.
func HighWord(value uintptr) int {
	return int((value >> 16) & 0xFFFF)
}
