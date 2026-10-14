package diskimage

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	ioctlStorageGetDeviceNumber = 0x002d1080
	errorNoMoreFiles            = syscall.Errno(18)
	volumeNameBufferInUint16    = 64 // `\\?\Volume{GUID}\` má 49 znaků
	volumePathsBufferInUint16   = 1024
)

var (
	kernel32                             = syscall.NewLazyDLL("kernel32.dll")
	procFindFirstVolumeW                 = kernel32.NewProc("FindFirstVolumeW")
	procFindNextVolumeW                  = kernel32.NewProc("FindNextVolumeW")
	procFindVolumeClose                  = kernel32.NewProc("FindVolumeClose")
	procGetVolumePathNamesForVolumeNameW = kernel32.NewProc("GetVolumePathNamesForVolumeNameW")
)

// storageDeviceNumber odpovídá STORAGE_DEVICE_NUMBER.
type storageDeviceNumber struct {
	DeviceType      uint32
	DeviceNumber    uint32
	PartitionNumber uint32
}

// findVolume vrátí kořen svazku na zařízení device (např. `\\.\CDROM1`):
// první písmeno jednotky nebo přípojný bod, jinak cestu `\\?\Volume{…}\`.
// Dokud svazek neexistuje, vrátí errVolumeNotReady.
func findVolume(device string) (string, error) {
	want, err := deviceNumber(device)
	deviceMissing := errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) || errors.Is(err, syscall.ERROR_PATH_NOT_FOUND)
	switch {
	case deviceMissing:
		return "", errVolumeNotReady
	case err != nil:
		return "", fmt.Errorf("zjištění čísla zařízení %q: %w", device, err)
	}

	volume, err := findVolumeOnDevice(want)
	if err != nil {
		return "", err
	}
	if paths := volumePathNames(volume); len(paths) > 0 {
		return paths[0], nil
	}
	return volume, nil
}

// findVolumeOnDevice projde všechny svazky systému a vrátí název toho, který
// leží na zadaném zařízení.
func findVolumeOnDevice(want storageDeviceNumber) (string, error) {
	buffer := make([]uint16, volumeNameBufferInUint16)
	find, _, callErr := procFindFirstVolumeW.Call(
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
	)
	if syscall.Handle(find) == syscall.InvalidHandle {
		return "", fmt.Errorf("výčet svazků: %w", callErr)
	}
	defer procFindVolumeClose.Call(find)

	for {
		volume := syscall.UTF16ToString(buffer)
		// Svazek bez média nebo bez oprávnění se přeskočí.
		if number, err := deviceNumber(strings.TrimSuffix(volume, `\`)); err == nil {
			sameDevice := number.DeviceType == want.DeviceType && number.DeviceNumber == want.DeviceNumber
			if sameDevice {
				return volume, nil
			}
		}

		ok, _, callErr := procFindNextVolumeW.Call(
			find,
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(len(buffer)),
		)
		if ok != 0 {
			continue
		}
		if errors.Is(callErr, errorNoMoreFiles) {
			return "", errVolumeNotReady
		}
		return "", fmt.Errorf("výčet svazků: %w", callErr)
	}
}

// deviceNumber zjistí typ a číslo zařízení, na kterém leží path. Popisovač se
// otevírá bez přístupových práv, takže stačí i běžné oprávnění.
func deviceNumber(path string) (storageDeviceNumber, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return storageDeviceNumber{}, err
	}
	handle, err := syscall.CreateFile(
		pathPtr,
		0,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		nil,
		syscall.OPEN_EXISTING,
		0,
		0,
	)
	if err != nil {
		return storageDeviceNumber{}, err
	}
	defer syscall.CloseHandle(handle)

	var number storageDeviceNumber
	var returned uint32
	err = syscall.DeviceIoControl(
		handle,
		ioctlStorageGetDeviceNumber,
		nil,
		0,
		(*byte)(unsafe.Pointer(&number)),
		uint32(unsafe.Sizeof(number)),
		&returned,
		nil,
	)
	return number, err
}

// volumePathNames vrátí písmena jednotek a přípojné body svazku, například
// `E:\`. Svazek bez písmena jednotky vrátí prázdný seznam.
func volumePathNames(volume string) []string {
	volumePtr, err := syscall.UTF16PtrFromString(volume)
	if err != nil {
		return []string{}
	}
	buffer := make([]uint16, volumePathsBufferInUint16)
	var returned uint32
	ok, _, _ := procGetVolumePathNamesForVolumeNameW.Call(
		uintptr(unsafe.Pointer(volumePtr)),
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(len(buffer)),
		uintptr(unsafe.Pointer(&returned)),
	)
	if ok == 0 {
		return []string{}
	}
	return splitMultiString(buffer)
}

// splitMultiString rozdělí seznam řetězců UTF-16 oddělených nulou a ukončený
// dvěma nulami.
func splitMultiString(buffer []uint16) []string {
	paths := []string{}
	start := 0
	for i, c := range buffer {
		if c != 0 {
			continue
		}
		if i == start {
			break
		}
		paths = append(paths, syscall.UTF16ToString(buffer[start:i]))
		start = i + 1
	}
	return paths
}
