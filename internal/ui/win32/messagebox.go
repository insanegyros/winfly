package win32

import "unsafe"

// ShowError shows a modal message box with a red error icon.
func ShowError(owner HWND, title, text string) {
	messageBox(owner, title, text, MB_ICONERROR)
}

// ShowWarning shows a modal message box with a yellow warning icon.
func ShowWarning(owner HWND, title, text string) {
	messageBox(owner, title, text, MB_ICONWARNING)
}

// ShowInfo shows a modal message box with a blue information icon.
func ShowInfo(owner HWND, title, text string) {
	messageBox(owner, title, text, MB_ICONINFORMATION)
}

func messageBox(owner HWND, title, text string, icon uint32) {
	procMessageBoxW.Call(
		uintptr(owner),
		uintptr(unsafe.Pointer(utf16Ptr(text))),
		uintptr(unsafe.Pointer(utf16Ptr(title))),
		uintptr(MB_OK|MB_TASKMODAL|icon),
	)
}
