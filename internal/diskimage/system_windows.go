package diskimage

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

const (
	errorPrivilegeNotHeld = syscall.Errno(1314)
	errorNotAllAssigned   = syscall.Errno(1300)

	sePrivilegeEnabled = 0x00000002
)

var (
	advapi32                  = syscall.NewLazyDLL("advapi32.dll")
	procLookupPrivilegeValueW = advapi32.NewProc("LookupPrivilegeValueW")
	procAdjustTokenPrivileges = advapi32.NewProc("AdjustTokenPrivileges")
)

// windowsSystem připojuje obrazy přes virtdisk.dll.
type windowsSystem struct{}

func newSystem() system { return windowsSystem{} }

func (windowsSystem) openImage(path string) (diskHandle, error) {
	handle, err := openVirtualDisk(path)
	if err != nil {
		return 0, describeError(err)
	}
	return diskHandle(handle), nil
}

func (windowsSystem) attachedDevice(h diskHandle) (string, bool) {
	device, err := virtualDiskPhysicalPath(syscall.Handle(h))
	return device, err == nil && device != ""
}

func (windowsSystem) attach(h diskHandle) error {
	// AttachVirtualDisk vyžaduje povolené oprávnění SeManageVolumePrivilege.
	if err := enablePrivilege("SeManageVolumePrivilege"); err != nil {
		return describeError(err)
	}
	return describeError(attachVirtualDisk(syscall.Handle(h)))
}

func (windowsSystem) detach(h diskHandle) error {
	return describeError(detachVirtualDisk(syscall.Handle(h)))
}

func (windowsSystem) closeImage(h diskHandle) error {
	return syscall.CloseHandle(syscall.Handle(h))
}

func (windowsSystem) volumeOf(device string) (string, error) {
	return findVolume(device)
}

// describeError doplní k chybám oprávnění ErrAccessDenied, aby je volající
// rozpoznal pomocí errors.Is.
func describeError(err error) error {
	if errors.Is(err, syscall.ERROR_ACCESS_DENIED) || errors.Is(err, errorPrivilegeNotHeld) {
		return fmt.Errorf("%w: %w", ErrAccessDenied, err)
	}
	return err
}

// luidAndAttributes a tokenPrivileges odpovídají strukturám
// LUID_AND_ATTRIBUTES a TOKEN_PRIVILEGES s jedním oprávněním.
type luidAndAttributes struct {
	LowPart    uint32
	HighPart   int32
	Attributes uint32
}

type tokenPrivileges struct {
	PrivilegeCount uint32
	Privileges     [1]luidAndAttributes
}

// enablePrivilege povolí oprávnění v tokenu procesu. Správce ho má, ale ve
// výchozím stavu bývá vypnuté.
func enablePrivilege(name string) error {
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return err
	}
	var token syscall.Token
	if err := syscall.OpenProcessToken(process, syscall.TOKEN_ADJUST_PRIVILEGES|syscall.TOKEN_QUERY, &token); err != nil {
		return err
	}
	defer token.Close()

	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	privileges := tokenPrivileges{PrivilegeCount: 1}
	privileges.Privileges[0].Attributes = sePrivilegeEnabled
	ok, _, callErr := procLookupPrivilegeValueW.Call(
		0,
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&privileges.Privileges[0])),
	)
	if ok == 0 {
		return callErr
	}

	ok, _, callErr = procAdjustTokenPrivileges.Call(
		uintptr(token),
		0,
		uintptr(unsafe.Pointer(&privileges)),
		0,
		0,
		0,
	)
	if ok == 0 {
		return callErr
	}
	// AdjustTokenPrivileges uspěje i tehdy, když token oprávnění vůbec nemá.
	if errors.Is(callErr, errorNotAllAssigned) {
		return errorPrivilegeNotHeld
	}
	return nil
}
