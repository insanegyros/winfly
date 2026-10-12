package win32

import (
	"strings"
	"syscall"
)

// utf16Ptr converts a Go string to the NUL-terminated UTF-16 that Win32
// expects. NUL characters inside the text would cut it short, so they are
// replaced with U+FFFD.
func utf16Ptr(text string) *uint16 {
	text = strings.ReplaceAll(text, "\x00", "�")
	ptr, _ := syscall.UTF16PtrFromString(text)
	return ptr
}

// callError turns the error returned by LazyProc.Call into something useful.
// Call always returns a non-nil error, which is "success" (errno 0) when the
// API did not set a last error.
func callError(err error) error {
	if errno, ok := err.(syscall.Errno); err == nil || ok && errno == 0 {
		return syscall.EINVAL
	}
	return err
}

func boolToUintptr(value bool) uintptr {
	if value {
		return 1
	}
	return 0
}
