package diskimage

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// Konstanty a struktury z virtdisk.h.
const (
	virtualStorageTypeDeviceISO = 1

	virtualDiskAccessRead      = 0x000d0000 // ATTACH_RO | DETACH | GET_INFO
	openVirtualDiskFlagNone    = 0
	attachVirtualDiskFlagRO    = 0x00000001
	attachVirtualDiskVersion1  = 1
	detachVirtualDiskFlagNone  = 0
	physicalPathBufferInUint16 = 1024
)

// virtualStorageTypeVendorMicrosoft je GUID {EC984AEC-A0F9-47E9-901F-71415A66345B}.
var virtualStorageTypeVendorMicrosoft = guid{
	Data1: 0xec984aec,
	Data2: 0xa0f9,
	Data3: 0x47e9,
	Data4: [8]byte{0x90, 0x1f, 0x71, 0x41, 0x5a, 0x66, 0x34, 0x5b},
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type virtualStorageType struct {
	DeviceID uint32
	VendorID guid
}

// attachVirtualDiskParameters odpovídá ATTACH_VIRTUAL_DISK_PARAMETERS. Unie
// verzí je zarovnaná na 8 bajtů a má 24 bajtů; pro verzi 1 stačí nuly.
type attachVirtualDiskParameters struct {
	Version uint32
	_       uint32
	_       [3]uint64
}

// virtdisk.dll není mezi KnownDLLs, proto se načítá výhradně ze System32,
// nikoli z adresáře, odkud byl WinFly spuštěn.
var (
	virtdisk                       = syscall.NewLazyDLL(systemDLLPath("virtdisk.dll"))
	procOpenVirtualDisk            = virtdisk.NewProc("OpenVirtualDisk")
	procAttachVirtualDisk          = virtdisk.NewProc("AttachVirtualDisk")
	procDetachVirtualDisk          = virtdisk.NewProc("DetachVirtualDisk")
	procGetVirtualDiskPhysicalPath = virtdisk.NewProc("GetVirtualDiskPhysicalPath")
)

func systemDLLPath(name string) string {
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	return filepath.Join(systemRoot, "System32", name)
}

// callVirtdisk zavolá funkci virtdisk.dll, které vracejí kód chyby Win32
// přímo jako návratovou hodnotu.
func callVirtdisk(proc *syscall.LazyProc, args ...uintptr) error {
	if err := proc.Find(); err != nil {
		return err
	}
	code, _, _ := proc.Call(args...)
	if code != 0 {
		return syscall.Errno(code)
	}
	return nil
}

func openVirtualDisk(path string) (syscall.Handle, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	storageType := virtualStorageType{
		DeviceID: virtualStorageTypeDeviceISO,
		VendorID: virtualStorageTypeVendorMicrosoft,
	}
	var handle syscall.Handle
	err = callVirtdisk(
		procOpenVirtualDisk,
		uintptr(unsafe.Pointer(&storageType)),
		uintptr(unsafe.Pointer(pathPtr)),
		virtualDiskAccessRead,
		openVirtualDiskFlagNone,
		0,
		uintptr(unsafe.Pointer(&handle)),
	)
	return handle, err
}

// attachVirtualDisk připojí obraz pouze pro čtení. Bez příznaku
// PERMANENT_LIFETIME se obraz odpojí se zavřením posledního popisovače.
func attachVirtualDisk(handle syscall.Handle) error {
	params := attachVirtualDiskParameters{Version: attachVirtualDiskVersion1}
	return callVirtdisk(
		procAttachVirtualDisk,
		uintptr(handle),
		0,
		attachVirtualDiskFlagRO,
		0,
		uintptr(unsafe.Pointer(&params)),
		0,
	)
}

func detachVirtualDisk(handle syscall.Handle) error {
	return callVirtdisk(procDetachVirtualDisk, uintptr(handle), detachVirtualDiskFlagNone, 0)
}

// virtualDiskPhysicalPath vrátí cestu k zařízení připojeného obrazu, například
// `\\.\CDROM1`. Pokud obraz připojen není, vrátí chybu.
func virtualDiskPhysicalPath(handle syscall.Handle) (string, error) {
	buffer := make([]uint16, physicalPathBufferInUint16)
	size := uint32(len(buffer) * 2)
	err := callVirtdisk(
		procGetVirtualDiskPhysicalPath,
		uintptr(handle),
		uintptr(unsafe.Pointer(&size)),
		uintptr(unsafe.Pointer(&buffer[0])),
	)
	if err != nil {
		return "", err
	}
	return syscall.UTF16ToString(buffer), nil
}
