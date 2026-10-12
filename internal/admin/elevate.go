package admin

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	swShowNormal = 1
	mbOK         = 0x00000000
	mbIconError  = 0x00000010
)

var (
	shell32           = syscall.NewLazyDLL("shell32.dll")
	user32            = syscall.NewLazyDLL("user32.dll")
	procIsUserAnAdmin = shell32.NewProc("IsUserAnAdmin")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
	procMessageBoxW   = user32.NewProc("MessageBoxW")
)

// IsElevated ověří, zda proces běží s oprávněním správce.
func IsElevated() bool {
	r1, _, _ := procIsUserAnAdmin.Call()
	return r1 != 0
}

// RelaunchElevated spustí stejný program znovu prostřednictvím UAC.
func RelaunchElevated() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("určení cesty ke spustitelnému souboru: %w", err)
	}
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	parameters, _ := syscall.UTF16PtrFromString("--elevated")
	r1, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(parameters)),
		0,
		swShowNormal,
	)
	if r1 <= 32 {
		return fmt.Errorf("zvýšení oprávnění UAC nebylo spuštěno (výsledek ShellExecuteW: %d)", r1)
	}
	return nil
}

// ShowError zobrazí modální chybové okno systému Windows.
func ShowError(title, message string) {
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	messagePtr, _ := syscall.UTF16PtrFromString(message)
	procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(messagePtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		mbOK|mbIconError,
	)
}
