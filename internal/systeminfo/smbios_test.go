package systeminfo

import (
	"encoding/binary"
	"testing"
)

func TestParseSMBIOSMemoryModules(t *testing.T) {
	device := make([]byte, 34)
	device[0] = 17
	device[1] = byte(len(device))
	binary.LittleEndian.PutUint16(device[12:14], 16*1024)
	device[16] = 1
	device[23] = 2
	binary.LittleEndian.PutUint16(device[32:34], 6000)

	table := append(device, []byte("DIMM_A1\x00Kingston\x00\x00")...)
	table = append(table, 127, 4, 0, 0, 0, 0)
	raw := make([]byte, 8+len(table))
	binary.LittleEndian.PutUint32(raw[4:8], uint32(len(table)))
	copy(raw[8:], table)

	modules, err := parseSMBIOSMemoryModules(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 1 {
		t.Fatalf("počet modulů = %d, očekáváno 1", len(modules))
	}
	module := modules[0]
	if module.Manufacturer != "Kingston" || module.Locator != "DIMM_A1" {
		t.Fatalf("neočekávaný modul: %#v", module)
	}
	if module.CapacityBytes != 16*1024*1024*1024 {
		t.Fatalf("kapacita = %d, očekáváno %d", module.CapacityBytes, uint64(16*1024*1024*1024))
	}
	if module.SpeedMT != 6000 {
		t.Fatalf("rychlost = %d, očekáváno 6000", module.SpeedMT)
	}
}
