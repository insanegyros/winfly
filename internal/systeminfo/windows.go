package systeminfo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	hkeyLocalMachine = 0x80000002

	rrfRTRegSZ          = 0x00000002
	rrfRTRegDWORD       = 0x00000010
	rrfSubKeyWOW6464Key = 0x00010000

	computerNamePhysicalDNSHostname = 5

	wtsUserName   = 5
	wtsDomainName = 7

	relationProcessorCore = 0
	allProcessorGroups    = 0xffff

	driveFixed = 3

	genericRead     = 0x80000000
	fileShareRead   = 0x00000001
	fileShareWrite  = 0x00000002
	fileShareDelete = 0x00000004
	openExisting    = 3
	invalidHandle   = ^uintptr(0)

	ioctlStorageGetDeviceNumber      = 0x002d1080
	ioctlDiskGetLengthInfo           = 0x0007405c
	ioctlStorageQueryProperty        = 0x002d1400
	storageDeviceSeekPenaltyProperty = 7
	propertyStandardQuery            = 0

	dxgiErrorNotFound       = 0x887a0002
	dxgiAdapterFlagRemote   = 0x1
	dxgiAdapterFlagSoftware = 0x2
)

var (
	kernel32SystemInfo = syscall.NewLazyDLL("kernel32.dll")
	advapi32SystemInfo = syscall.NewLazyDLL("advapi32.dll")
	ntdllSystemInfo    = syscall.NewLazyDLL("ntdll.dll")
	wtsapi32SystemInfo = syscall.NewLazyDLL("wtsapi32.dll")
	secur32SystemInfo  = syscall.NewLazyDLL("secur32.dll")
	dxgiSystemInfo     = syscall.NewLazyDLL("dxgi.dll")

	procGetComputerNameExW               = kernel32SystemInfo.NewProc("GetComputerNameExW")
	procGetCurrentProcessId              = kernel32SystemInfo.NewProc("GetCurrentProcessId")
	procProcessIdToSessionId             = kernel32SystemInfo.NewProc("ProcessIdToSessionId")
	procGetLogicalProcessorInformationEx = kernel32SystemInfo.NewProc("GetLogicalProcessorInformationEx")
	procGetActiveProcessorCount          = kernel32SystemInfo.NewProc("GetActiveProcessorCount")
	procGlobalMemoryStatusEx             = kernel32SystemInfo.NewProc("GlobalMemoryStatusEx")
	procGetLogicalDrives                 = kernel32SystemInfo.NewProc("GetLogicalDrives")
	procGetDriveTypeW                    = kernel32SystemInfo.NewProc("GetDriveTypeW")
	procGetDiskFreeSpaceExW              = kernel32SystemInfo.NewProc("GetDiskFreeSpaceExW")
	procCreateFileW                      = kernel32SystemInfo.NewProc("CreateFileW")
	procDeviceIoControl                  = kernel32SystemInfo.NewProc("DeviceIoControl")
	procCloseHandle                      = kernel32SystemInfo.NewProc("CloseHandle")
	procGetSystemFirmwareTable           = kernel32SystemInfo.NewProc("GetSystemFirmwareTable")

	procRegGetValueW  = advapi32SystemInfo.NewProc("RegGetValueW")
	procRtlGetVersion = ntdllSystemInfo.NewProc("RtlGetVersion")

	procWTSQuerySessionInformationW = wtsapi32SystemInfo.NewProc("WTSQuerySessionInformationW")
	procWTSFreeMemory               = wtsapi32SystemInfo.NewProc("WTSFreeMemory")
	procGetUserNameExW              = secur32SystemInfo.NewProc("GetUserNameExW")

	procCreateDXGIFactory1 = dxgiSystemInfo.NewProc("CreateDXGIFactory1")
)

type rtlOSVersionInfoEx struct {
	OSVersionInfoSize uint32
	MajorVersion      uint32
	MinorVersion      uint32
	BuildNumber       uint32
	PlatformID        uint32
	CSDVersion        [128]uint16
	ServicePackMajor  uint16
	ServicePackMinor  uint16
	SuiteMask         uint16
	ProductType       byte
	Reserved          byte
}

type memoryStatusEx struct {
	Length                   uint32
	MemoryLoad               uint32
	TotalPhysical            uint64
	AvailablePhysical        uint64
	TotalPageFile            uint64
	AvailablePageFile        uint64
	TotalVirtual             uint64
	AvailableVirtual         uint64
	AvailableExtendedVirtual uint64
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type luid struct {
	LowPart  uint32
	HighPart int32
}

type dxgiAdapterDesc1 struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	AdapterLUID           luid
	Flags                 uint32
}

type storageDeviceNumber struct {
	DeviceType      uint32
	DeviceNumber    uint32
	PartitionNumber uint32
}

type storagePropertyQuery struct {
	PropertyID          uint32
	QueryType           uint32
	AdditionalParameter [1]byte
}

var iidIDXGIFactory1 = guid{
	Data1: 0x770aae78,
	Data2: 0xf26f,
	Data3: 0x4dba,
	Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87},
}

func computerName() (string, error) {
	size := uint32(256)
	buffer := make([]uint16, size)
	r1, _, callErr := procGetComputerNameExW.Call(
		computerNamePhysicalDNSHostname,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 {
		if size > uint32(len(buffer)) {
			buffer = make([]uint16, size)
			r1, _, callErr = procGetComputerNameExW.Call(
				computerNamePhysicalDNSHostname,
				uintptr(unsafe.Pointer(&buffer[0])),
				uintptr(unsafe.Pointer(&size)),
			)
		}
	}
	if r1 == 0 {
		return "", normalizeWindowsCallError(callErr)
	}
	return strings.TrimSpace(syscall.UTF16ToString(buffer[:size])), nil
}

func loggedInUser() (string, error) {
	pid, _, _ := procGetCurrentProcessId.Call()
	var sessionID uint32
	r1, _, callErr := procProcessIdToSessionId.Call(pid, uintptr(unsafe.Pointer(&sessionID)))
	if r1 != 0 {
		userName, userErr := queryWTSSessionString(sessionID, wtsUserName)
		domainName, domainErr := queryWTSSessionString(sessionID, wtsDomainName)
		if userErr == nil && strings.TrimSpace(userName) != "" {
			if domainErr == nil && strings.TrimSpace(domainName) != "" {
				return strings.TrimSpace(domainName) + "\\" + strings.TrimSpace(userName), nil
			}
			return strings.TrimSpace(userName), nil
		}
	}
	fallback, fallbackErr := currentTokenUserName()
	if fallbackErr == nil {
		return fallback, nil
	}
	if r1 == 0 {
		return "", errors.Join(normalizeWindowsCallError(callErr), fallbackErr)
	}
	return "", fallbackErr
}

func queryWTSSessionString(sessionID uint32, infoClass uintptr) (string, error) {
	var buffer *uint16
	var bytesReturned uint32
	r1, _, callErr := procWTSQuerySessionInformationW.Call(
		0,
		uintptr(sessionID),
		infoClass,
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&bytesReturned)),
	)
	if r1 == 0 {
		return "", normalizeWindowsCallError(callErr)
	}
	if buffer == nil {
		return "", errors.New("WTSQuerySessionInformationW vrátil prázdný ukazatel")
	}
	defer procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buffer)))
	length := int(bytesReturned / 2)
	if length <= 0 {
		return "", nil
	}
	value := syscall.UTF16ToString(unsafe.Slice(buffer, length))
	return strings.TrimSpace(value), nil
}

func currentTokenUserName() (string, error) {
	const nameSAMCompatible = 2
	size := uint32(256)
	buffer := make([]uint16, size)
	r1, _, callErr := procGetUserNameExW.Call(
		nameSAMCompatible,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 && size > uint32(len(buffer)) {
		buffer = make([]uint16, size)
		r1, _, callErr = procGetUserNameExW.Call(
			nameSAMCompatible,
			uintptr(unsafe.Pointer(&buffer[0])),
			uintptr(unsafe.Pointer(&size)),
		)
	}
	if r1 == 0 {
		return "", normalizeWindowsCallError(callErr)
	}
	return strings.TrimSpace(syscall.UTF16ToString(buffer[:size])), nil
}

func operatingSystem() (OperatingSystemInfo, error) {
	version := rtlOSVersionInfoEx{OSVersionInfoSize: uint32(unsafe.Sizeof(rtlOSVersionInfoEx{}))}
	status, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&version)))
	if int32(status) < 0 {
		return OperatingSystemInfo{}, fmt.Errorf("RtlGetVersion selhalo se stavem 0x%08X", uint32(status))
	}

	const currentVersionKey = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
	productName, _ := registryString(currentVersionKey, "ProductName", true)
	displayVersion, _ := registryString(currentVersionKey, "DisplayVersion", true)
	if displayVersion == "" {
		displayVersion, _ = registryString(currentVersionKey, "ReleaseId", true)
	}
	ubr, _ := registryDWORD(currentVersionKey, "UBR", true)

	productName = strings.TrimSpace(productName)
	if productName == "" {
		productName = "Microsoft Windows"
	}
	if version.BuildNumber >= 22000 && strings.Contains(productName, "Windows 10") {
		productName = strings.Replace(productName, "Windows 10", "Windows 11", 1)
	}

	build := fmt.Sprintf("%d.%d.%d", version.MajorVersion, version.MinorVersion, version.BuildNumber)
	if ubr > 0 {
		build += "." + strconv.FormatUint(uint64(ubr), 10)
	}

	return OperatingSystemInfo{
		Edition: productName,
		Version: strings.TrimSpace(displayVersion),
		Build:   build,
	}, nil
}

func cpuInfo() (CPUInfo, error) {
	const processorKey = `HARDWARE\DESCRIPTION\System\CentralProcessor\0`
	vendor, vendorErr := registryString(processorKey, "VendorIdentifier", false)
	model, modelErr := registryString(processorKey, "ProcessorNameString", false)
	mhz, mhzErr := registryDWORD(processorKey, "~MHz", false)
	cores, threads, countErr := processorCounts()

	info := CPUInfo{
		Manufacturer: normalizeCPUManufacturer(vendor),
		Model:        strings.TrimSpace(model),
		FrequencyMHz: mhz,
		Cores:        cores,
		Threads:      threads,
	}
	if info.Manufacturer == "" && info.Model == "" && info.FrequencyMHz == 0 && info.Cores == 0 && info.Threads == 0 {
		joinedErr := errors.Join(vendorErr, modelErr, mhzErr, countErr)
		if joinedErr == nil {
			joinedErr = errors.New("systém nevrátil žádné informace o procesoru")
		}
		return CPUInfo{}, joinedErr
	}
	return info, nil
}

func normalizeCPUManufacturer(value string) string {
	switch strings.TrimSpace(value) {
	case "GenuineIntel":
		return "Intel"
	case "AuthenticAMD":
		return "AMD"
	default:
		return strings.TrimSpace(value)
	}
}

func processorCounts() (uint32, uint32, error) {
	var length uint32
	_, _, firstErr := procGetLogicalProcessorInformationEx.Call(
		relationProcessorCore,
		0,
		uintptr(unsafe.Pointer(&length)),
	)
	if length == 0 {
		threads := activeProcessorCount()
		return 0, threads, normalizeWindowsCallError(firstErr)
	}
	buffer := make([]byte, length)
	r1, _, callErr := procGetLogicalProcessorInformationEx.Call(
		relationProcessorCore,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&length)),
	)
	if r1 == 0 {
		return 0, activeProcessorCount(), normalizeWindowsCallError(callErr)
	}

	var cores uint32
	for offset := 0; offset+8 <= int(length); {
		relationship := binary.LittleEndian.Uint32(buffer[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(buffer[offset+4 : offset+8]))
		if size < 8 || offset+size > int(length) {
			return cores, activeProcessorCount(), errors.New("GetLogicalProcessorInformationEx vrátil neplatnou strukturu")
		}
		if relationship == relationProcessorCore {
			cores++
		}
		offset += size
	}
	return cores, activeProcessorCount(), nil
}

func activeProcessorCount() uint32 {
	r1, _, _ := procGetActiveProcessorCount.Call(allProcessorGroups)
	if uint32(r1) != 0 {
		return uint32(r1)
	}
	return uint32(runtime.NumCPU())
}

func totalPhysicalMemory() (uint64, error) {
	status := memoryStatusEx{Length: uint32(unsafe.Sizeof(memoryStatusEx{}))}
	r1, _, callErr := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if r1 == 0 {
		return 0, normalizeWindowsCallError(callErr)
	}
	return status.TotalPhysical, nil
}

func fixedDrives() ([]DriveInfo, error) {
	mountPointsByDisk := mountedFixedDriveLetters()
	drives := make([]DriveInfo, 0, len(mountPointsByDisk))
	for deviceNumber := uint32(0); deviceNumber < 64; deviceNumber++ {
		path := fmt.Sprintf(`\\.\PhysicalDrive%d`, deviceNumber)
		handle, err := openDevice(path, genericRead)
		if err != nil {
			continue
		}
		capacity, capacityErr := physicalDiskLength(handle)
		mediaType := physicalDiskMediaType(handle)
		closeDevice(handle)
		if capacityErr != nil || capacity == 0 {
			continue
		}
		mountPoints := append([]string(nil), mountPointsByDisk[deviceNumber]...)
		sort.Strings(mountPoints)
		drives = append(drives, DriveInfo{
			Name:          fmt.Sprintf("Fyzický disk %d", deviceNumber),
			DeviceNumber:  deviceNumber,
			MediaType:     mediaType,
			MountPoints:   mountPoints,
			CapacityBytes: capacity,
		})
	}
	if len(drives) > 0 {
		sort.Slice(drives, func(i, j int) bool { return drives[i].DeviceNumber < drives[j].DeviceNumber })
		return drives, nil
	}
	return fixedVolumeFallback()
}

func mountedFixedDriveLetters() map[uint32][]string {
	result := make(map[uint32][]string)
	mask, _, callErr := procGetLogicalDrives.Call()
	if mask == 0 {
		_ = callErr
		return result
	}
	for index := 0; index < 26; index++ {
		if mask&(uintptr(1)<<index) == 0 {
			continue
		}
		root := fmt.Sprintf("%c:\\", 'A'+index)
		rootPtr, err := syscall.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		typeValue, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(rootPtr)))
		if typeValue != driveFixed {
			continue
		}
		volumePath := fmt.Sprintf(`\\.\%c:`, 'A'+index)
		handle, err := openDevice(volumePath, 0)
		if err != nil {
			continue
		}
		deviceNumber, err := volumeDeviceNumber(handle)
		closeDevice(handle)
		if err != nil {
			continue
		}
		result[deviceNumber] = append(result[deviceNumber], root)
	}
	return result
}

func fixedVolumeFallback() ([]DriveInfo, error) {
	mask, _, callErr := procGetLogicalDrives.Call()
	if mask == 0 {
		return nil, normalizeWindowsCallError(callErr)
	}
	drives := make([]DriveInfo, 0, 4)
	for index := 0; index < 26; index++ {
		if mask&(uintptr(1)<<index) == 0 {
			continue
		}
		root := fmt.Sprintf("%c:\\", 'A'+index)
		rootPtr, err := syscall.UTF16PtrFromString(root)
		if err != nil {
			continue
		}
		typeValue, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(rootPtr)))
		if typeValue != driveFixed {
			continue
		}
		var totalBytes uint64
		r1, _, _ := procGetDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(rootPtr)), 0, uintptr(unsafe.Pointer(&totalBytes)), 0)
		if r1 == 0 {
			continue
		}
		drives = append(drives, DriveInfo{
			Name:          "Pevný svazek",
			DeviceNumber:  uint32(index),
			MountPoints:   []string{root},
			CapacityBytes: totalBytes,
		})
	}
	if len(drives) == 0 {
		return nil, errors.New("nebyl nalezen žádný připojený pevný disk ani svazek")
	}
	return drives, nil
}

func openDevice(path string, desiredAccess uintptr) (uintptr, error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return invalidHandle, err
	}
	handle, _, callErr := procCreateFileW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		desiredAccess,
		fileShareRead|fileShareWrite|fileShareDelete,
		0,
		openExisting,
		0,
		0,
	)
	if handle == invalidHandle {
		return invalidHandle, normalizeWindowsCallError(callErr)
	}
	return handle, nil
}

func closeDevice(handle uintptr) {
	if handle != 0 && handle != invalidHandle {
		procCloseHandle.Call(handle)
	}
}

func volumeDeviceNumber(handle uintptr) (uint32, error) {
	var value storageDeviceNumber
	var bytesReturned uint32
	r1, _, callErr := procDeviceIoControl.Call(
		handle,
		ioctlStorageGetDeviceNumber,
		0,
		0,
		uintptr(unsafe.Pointer(&value)),
		unsafe.Sizeof(value),
		uintptr(unsafe.Pointer(&bytesReturned)),
		0,
	)
	if r1 == 0 {
		return 0, normalizeWindowsCallError(callErr)
	}
	return value.DeviceNumber, nil
}

func physicalDiskLength(handle uintptr) (uint64, error) {
	var length uint64
	var bytesReturned uint32
	r1, _, callErr := procDeviceIoControl.Call(
		handle,
		ioctlDiskGetLengthInfo,
		0,
		0,
		uintptr(unsafe.Pointer(&length)),
		unsafe.Sizeof(length),
		uintptr(unsafe.Pointer(&bytesReturned)),
		0,
	)
	if r1 == 0 {
		return 0, normalizeWindowsCallError(callErr)
	}
	return length, nil
}

func physicalDiskMediaType(handle uintptr) string {
	query := storagePropertyQuery{
		PropertyID: storageDeviceSeekPenaltyProperty,
		QueryType:  propertyStandardQuery,
	}
	var descriptor [12]byte
	var bytesReturned uint32
	r1, _, _ := procDeviceIoControl.Call(
		handle,
		ioctlStorageQueryProperty,
		uintptr(unsafe.Pointer(&query)),
		unsafe.Sizeof(query),
		uintptr(unsafe.Pointer(&descriptor[0])),
		unsafe.Sizeof(descriptor),
		uintptr(unsafe.Pointer(&bytesReturned)),
		0,
	)
	if r1 == 0 || bytesReturned < 9 {
		return "Disk"
	}
	if descriptor[8] != 0 {
		return "HDD"
	}
	return "SSD"
}

func gpuInfo() ([]GPUInfo, error) {
	var factory unsafe.Pointer
	hr, _, callErr := procCreateDXGIFactory1.Call(
		uintptr(unsafe.Pointer(&iidIDXGIFactory1)),
		uintptr(unsafe.Pointer(&factory)),
	)
	if int32(hr) < 0 || factory == nil {
		return nil, fmt.Errorf("CreateDXGIFactory1 selhalo (HRESULT 0x%08X): %w", uint32(hr), normalizeWindowsCallError(callErr))
	}
	defer comRelease(factory)

	adapters := make([]GPUInfo, 0, 2)
	for index := uint32(0); ; index++ {
		var adapter unsafe.Pointer
		hr = comCall(factory, 12, uintptr(index), uintptr(unsafe.Pointer(&adapter)))
		if uint32(hr) == dxgiErrorNotFound {
			break
		}
		if int32(hr) < 0 {
			return adapters, fmt.Errorf("IDXGIFactory1::EnumAdapters1 selhalo (HRESULT 0x%08X)", uint32(hr))
		}
		if adapter == nil {
			continue
		}

		var desc dxgiAdapterDesc1
		descHR := comCall(adapter, 10, uintptr(unsafe.Pointer(&desc)))
		comRelease(adapter)
		if int32(descHR) < 0 {
			continue
		}
		if desc.Flags&(dxgiAdapterFlagRemote|dxgiAdapterFlagSoftware) != 0 {
			continue
		}
		adapters = append(adapters, GPUInfo{
			Manufacturer:         gpuManufacturer(desc.VendorID),
			Model:                strings.TrimSpace(syscall.UTF16ToString(desc.Description[:])),
			DedicatedVideoMemory: uint64(desc.DedicatedVideoMemory),
			SharedSystemMemory:   uint64(desc.SharedSystemMemory),
		})
	}
	if len(adapters) == 0 {
		return nil, errors.New("DXGI nevrátil žádný hardwarový grafický adaptér")
	}
	return adapters, nil
}

func gpuManufacturer(vendorID uint32) string {
	switch vendorID {
	case 0x1002, 0x1022:
		return "AMD"
	case 0x10de:
		return "NVIDIA"
	case 0x8086:
		return "Intel"
	case 0x1414:
		return "Microsoft"
	case 0x13b5:
		return "Arm"
	case 0x5143:
		return "Qualcomm"
	default:
		if vendorID == 0 {
			return ""
		}
		return fmt.Sprintf("PCI výrobce 0x%04X", vendorID)
	}
}

func comCall(object unsafe.Pointer, methodIndex uintptr, args ...uintptr) uintptr {
	vtable := *(*unsafe.Pointer)(object)
	method := *(*uintptr)(unsafe.Add(vtable, methodIndex*unsafe.Sizeof(uintptr(0))))
	callArgs := make([]uintptr, 0, len(args)+1)
	callArgs = append(callArgs, uintptr(object))
	callArgs = append(callArgs, args...)
	r1, _, _ := syscall.SyscallN(method, callArgs...)
	return r1
}

func comRelease(object unsafe.Pointer) {
	if object != nil {
		comCall(object, 2)
	}
}

func registryString(subKey, valueName string, force64Bit bool) (string, error) {
	flags := uint32(rrfRTRegSZ)
	if force64Bit {
		flags |= rrfSubKeyWOW6464Key
	}
	subKeyPtr, err := syscall.UTF16PtrFromString(subKey)
	if err != nil {
		return "", err
	}
	valuePtr, err := syscall.UTF16PtrFromString(valueName)
	if err != nil {
		return "", err
	}

	var size uint32
	status, _, _ := procRegGetValueW.Call(
		hkeyLocalMachine,
		uintptr(unsafe.Pointer(subKeyPtr)),
		uintptr(unsafe.Pointer(valuePtr)),
		uintptr(flags),
		0,
		0,
		uintptr(unsafe.Pointer(&size)),
	)
	if status != 0 {
		return "", syscall.Errno(status)
	}
	if size == 0 {
		return "", nil
	}
	buffer := make([]uint16, (size+1)/2)
	status, _, _ = procRegGetValueW.Call(
		hkeyLocalMachine,
		uintptr(unsafe.Pointer(subKeyPtr)),
		uintptr(unsafe.Pointer(valuePtr)),
		uintptr(flags),
		0,
		uintptr(unsafe.Pointer(&buffer[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if status != 0 {
		return "", syscall.Errno(status)
	}
	return strings.TrimSpace(syscall.UTF16ToString(buffer)), nil
}

func registryDWORD(subKey, valueName string, force64Bit bool) (uint32, error) {
	flags := uint32(rrfRTRegDWORD)
	if force64Bit {
		flags |= rrfSubKeyWOW6464Key
	}
	subKeyPtr, err := syscall.UTF16PtrFromString(subKey)
	if err != nil {
		return 0, err
	}
	valuePtr, err := syscall.UTF16PtrFromString(valueName)
	if err != nil {
		return 0, err
	}
	var value uint32
	size := uint32(unsafe.Sizeof(value))
	status, _, _ := procRegGetValueW.Call(
		hkeyLocalMachine,
		uintptr(unsafe.Pointer(subKeyPtr)),
		uintptr(unsafe.Pointer(valuePtr)),
		uintptr(flags),
		0,
		uintptr(unsafe.Pointer(&value)),
		uintptr(unsafe.Pointer(&size)),
	)
	if status != 0 {
		return 0, syscall.Errno(status)
	}
	return value, nil
}

func normalizeWindowsCallError(err error) error {
	if err == nil {
		return syscall.EINVAL
	}
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		return syscall.EINVAL
	}
	return err
}
