package systeminfo

import (
	"strings"
	"testing"
)

func TestOperatingSystemTextKeepsEditionVersionAndBuildSeparate(t *testing.T) {
	info := OperatingSystemInfo{
		Edition: "Windows 11 Pro",
		Version: "24H2",
		Build:   "10.0.26100.4946",
	}
	text := info.Text()
	for _, expected := range []string{"Windows 11 Pro", "verze 24H2", "sestavení 10.0.26100.4946"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("text %q neobsahuje %q", text, expected)
		}
	}
}

func TestMemoryTextDistinguishesInstalledAndSystemAvailableMemory(t *testing.T) {
	report := Report{Memory: MemoryInfo{
		TotalBytes: 31 * 1024 * 1024 * 1024,
		Modules: []MemoryModule{
			{Manufacturer: "Kingston", CapacityBytes: 16 * 1024 * 1024 * 1024, SpeedMT: 6000, Locator: "DIMM_A1"},
			{Manufacturer: "Kingston", CapacityBytes: 16 * 1024 * 1024 * 1024, SpeedMT: 6000, Locator: "DIMM_B1"},
		},
	}}
	text := report.MemoryText()
	for _, expected := range []string{"Osazeno: 32,00 GB", "Dostupné systému: 31,00 GB", "Kingston [DIMM_A1]", "6000 MT/s"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("text %q neobsahuje %q", text, expected)
		}
	}
}

func TestDriveTextShowsPhysicalCapacityAndMountPoints(t *testing.T) {
	report := Report{Drives: []DriveInfo{{
		Name:          "Fyzický disk 0",
		DeviceNumber:  0,
		MediaType:     "SSD",
		MountPoints:   []string{"C:\\", "D:\\"},
		CapacityBytes: 1024 * 1024 * 1024 * 1024,
	}}}
	text := report.DriveText()
	for _, expected := range []string{"SSD — Fyzický disk 0", "1024,00 GB", "C:\\, D:\\"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("text %q neobsahuje %q", text, expected)
		}
	}
}
