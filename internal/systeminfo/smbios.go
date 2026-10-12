package systeminfo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unsafe"
)

const firmwareTableProviderRSMB = 0x52534d42

func memoryModules() ([]MemoryModule, error) {
	raw, err := rawSMBIOSData()
	if err != nil {
		return nil, err
	}
	return parseSMBIOSMemoryModules(raw)
}

func rawSMBIOSData() ([]byte, error) {
	size, _, callErr := procGetSystemFirmwareTable.Call(firmwareTableProviderRSMB, 0, 0, 0)
	if size == 0 {
		return nil, normalizeWindowsCallError(callErr)
	}
	if size > 16*1024*1024 {
		return nil, fmt.Errorf("neočekávaná velikost SMBIOS tabulky: %d bajtů", size)
	}
	buffer := make([]byte, size)
	written, _, callErr := procGetSystemFirmwareTable.Call(
		firmwareTableProviderRSMB,
		0,
		uintptr(unsafe.Pointer(&buffer[0])),
		size,
	)
	if written == 0 {
		return nil, normalizeWindowsCallError(callErr)
	}
	if written > uintptr(len(buffer)) {
		return nil, errors.New("GetSystemFirmwareTable vrátil více dat, než umožňuje vyrovnávací paměť")
	}
	return buffer[:written], nil
}

func parseSMBIOSMemoryModules(raw []byte) ([]MemoryModule, error) {
	if len(raw) < 8 {
		return nil, errors.New("SMBIOS data jsou kratší než hlavička RawSMBIOSData")
	}
	tableLength := int(binary.LittleEndian.Uint32(raw[4:8]))
	if tableLength < 0 || tableLength > len(raw)-8 {
		return nil, errors.New("SMBIOS hlavička obsahuje neplatnou délku tabulky")
	}
	table := raw[8 : 8+tableLength]
	modules := make([]MemoryModule, 0, 4)

	for offset := 0; offset+4 <= len(table); {
		structureType := table[offset]
		formattedLength := int(table[offset+1])
		if formattedLength < 4 || offset+formattedLength > len(table) {
			return modules, errors.New("SMBIOS obsahuje neplatnou délku struktury")
		}
		formatted := table[offset : offset+formattedLength]
		stringsStart := offset + formattedLength
		stringsEnd, nextOffset, err := findSMBIOSStringAreaEnd(table, stringsStart)
		if err != nil {
			return modules, err
		}
		stringValues := splitSMBIOSStrings(table[stringsStart:stringsEnd])

		if structureType == 17 {
			if module, ok := parseMemoryDevice(formatted, stringValues); ok {
				modules = append(modules, module)
			}
		}
		offset = nextOffset
		if structureType == 127 {
			break
		}
	}
	if len(modules) == 0 {
		return nil, errors.New("SMBIOS neobsahuje žádný osazený paměťový modul typu 17")
	}
	return modules, nil
}

func findSMBIOSStringAreaEnd(table []byte, start int) (int, int, error) {
	if start >= len(table) {
		return 0, 0, errors.New("SMBIOS struktura nemá ukončenou oblast řetězců")
	}
	for index := start; index+1 < len(table); index++ {
		if table[index] == 0 && table[index+1] == 0 {
			return index, index + 2, nil
		}
	}
	return 0, 0, errors.New("SMBIOS oblast řetězců není ukončena dvojitou nulou")
}

func splitSMBIOSStrings(value []byte) []string {
	if len(value) == 0 {
		return nil
	}
	parts := make([]string, 0, 4)
	start := 0
	for index := 0; index <= len(value); index++ {
		if index == len(value) || value[index] == 0 {
			parts = append(parts, strings.TrimSpace(strings.ToValidUTF8(string(value[start:index]), "�")))
			start = index + 1
		}
	}
	return parts
}

func parseMemoryDevice(formatted []byte, stringsTable []string) (MemoryModule, bool) {
	if len(formatted) < 14 {
		return MemoryModule{}, false
	}
	sizeField := binary.LittleEndian.Uint16(formatted[12:14])
	capacity := memoryDeviceCapacity(sizeField, formatted)
	if capacity == 0 {
		return MemoryModule{}, false
	}

	module := MemoryModule{CapacityBytes: capacity}
	if len(formatted) >= 17 {
		module.Locator = smbiosString(stringsTable, formatted[16])
	}
	if len(formatted) >= 24 {
		module.Manufacturer = smbiosString(stringsTable, formatted[23])
	}
	if len(formatted) >= 34 {
		module.SpeedMT = uint32(binary.LittleEndian.Uint16(formatted[32:34]))
	}
	if module.SpeedMT == 0 && len(formatted) >= 23 {
		module.SpeedMT = uint32(binary.LittleEndian.Uint16(formatted[21:23]))
	}
	if module.SpeedMT == 0xffff {
		module.SpeedMT = 0
	}
	return module, true
}

func memoryDeviceCapacity(sizeField uint16, formatted []byte) uint64 {
	switch sizeField {
	case 0, 0xffff:
		return 0
	case 0x7fff:
		if len(formatted) < 32 {
			return 0
		}
		extendedMB := binary.LittleEndian.Uint32(formatted[28:32]) & 0x7fffffff
		return uint64(extendedMB) * 1024 * 1024
	default:
		value := uint64(sizeField & 0x7fff)
		if sizeField&0x8000 != 0 {
			return value * 1024
		}
		return value * 1024 * 1024
	}
}

func smbiosString(values []string, index byte) string {
	if index == 0 || int(index) > len(values) {
		return ""
	}
	return values[int(index)-1]
}
