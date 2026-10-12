package systeminfo

import (
	"context"
	"fmt"
	"strings"
)

const unknownValue = "Nezjištěno"

// Report obsahuje systémové informace získané pouze pomocí standardních API Windows
// a vestavěných součástí systému.
type Report struct {
	ComputerName    string
	LoggedInUser    string
	OperatingSystem OperatingSystemInfo
	DirectXVersion  string
	CPU             CPUInfo
	Memory          MemoryInfo
	GPUs            []GPUInfo
	Drives          []DriveInfo
	Warnings        []error
}

type OperatingSystemInfo struct {
	Edition string
	Version string
	Build   string
}

type CPUInfo struct {
	Manufacturer string
	Model        string
	FrequencyMHz uint32
	Cores        uint32
	Threads      uint32
}

type MemoryInfo struct {
	TotalBytes uint64
	Modules    []MemoryModule
}

type MemoryModule struct {
	Manufacturer  string
	CapacityBytes uint64
	SpeedMT       uint32
	Locator       string
}

type GPUInfo struct {
	Manufacturer         string
	Model                string
	DedicatedVideoMemory uint64
	SharedSystemMemory   uint64
}

type DriveInfo struct {
	Name          string
	DeviceNumber  uint32
	MediaType     string
	MountPoints   []string
	CapacityBytes uint64
}

// Collect sestaví částečný report. Selhání jedné položky nezablokuje ostatní;
// podrobnosti jsou vráceny v Report.Warnings.
func Collect(ctx context.Context) Report {
	report := Report{}

	if value, err := computerName(); err != nil {
		report.Warnings = append(report.Warnings, fmt.Errorf("název počítače: %w", err))
	} else {
		report.ComputerName = value
	}

	if value, err := loggedInUser(); err != nil {
		report.Warnings = append(report.Warnings, fmt.Errorf("přihlášený uživatel: %w", err))
	} else {
		report.LoggedInUser = value
	}

	if value, err := operatingSystem(); err != nil {
		report.Warnings = append(report.Warnings, fmt.Errorf("operační systém: %w", err))
	} else {
		report.OperatingSystem = value
	}

	if value, err := cpuInfo(); err != nil {
		report.Warnings = append(report.Warnings, fmt.Errorf("procesor: %w", err))
	} else {
		report.CPU = value
	}

	if total, err := totalPhysicalMemory(); err != nil {
		report.Warnings = append(report.Warnings, fmt.Errorf("celková operační paměť: %w", err))
	} else {
		report.Memory.TotalBytes = total
	}
	if modules, err := memoryModules(); err != nil {
		report.Warnings = append(report.Warnings, fmt.Errorf("moduly operační paměti: %w", err))
	} else {
		report.Memory.Modules = modules
	}

	if values, err := gpuInfo(); err != nil {
		report.Warnings = append(report.Warnings, fmt.Errorf("grafické adaptéry: %w", err))
	} else {
		report.GPUs = values
	}

	if values, err := fixedDrives(); err != nil {
		report.Warnings = append(report.Warnings, fmt.Errorf("pevné disky a oddíly: %w", err))
	} else {
		report.Drives = values
	}

	if value, err := directXVersion(ctx); err != nil {
		report.Warnings = append(report.Warnings, fmt.Errorf("verze DirectX: %w", err))
	} else {
		report.DirectXVersion = value
	}

	return report
}

func (r Report) GeneralValues() (computerNameValue, userValue, osValue, directXValue string) {
	return valueOrUnknown(r.ComputerName), valueOrUnknown(r.LoggedInUser), r.OperatingSystem.Text(), valueOrUnknown(r.DirectXVersion)
}

func (info OperatingSystemInfo) Text() string {
	parts := make([]string, 0, 3)
	if value := strings.TrimSpace(info.Edition); value != "" {
		parts = append(parts, value)
	}
	if value := strings.TrimSpace(info.Version); value != "" {
		parts = append(parts, "verze "+value)
	}
	if value := strings.TrimSpace(info.Build); value != "" {
		parts = append(parts, "sestavení "+value)
	}
	if len(parts) == 0 {
		return unknownValue
	}
	return strings.Join(parts, " — ")
}

func (r Report) CPUText() string {
	lines := []string{
		"Výrobce: " + valueOrUnknown(r.CPU.Manufacturer),
		"Model: " + valueOrUnknown(r.CPU.Model),
	}
	if r.CPU.FrequencyMHz > 0 {
		lines = append(lines, fmt.Sprintf("Frekvence: %s GHz", formatDecimal(float64(r.CPU.FrequencyMHz)/1000, 2)))
	}
	if r.CPU.Cores > 0 {
		lines = append(lines, fmt.Sprintf("Jádra: %d", r.CPU.Cores))
	}
	if r.CPU.Threads > 0 {
		lines = append(lines, fmt.Sprintf("Vlákna: %d", r.CPU.Threads))
	}
	return strings.Join(lines, "\r\n")
}

func (r Report) MemoryText() string {
	lines := make([]string, 0, len(r.Memory.Modules)+3)
	var installedBytes uint64
	for _, module := range r.Memory.Modules {
		installedBytes += module.CapacityBytes
	}
	if installedBytes > 0 {
		lines = append(lines, "Osazeno: "+formatBytesGB(installedBytes))
	}
	if r.Memory.TotalBytes > 0 {
		lines = append(lines, "Dostupné systému: "+formatBytesGB(r.Memory.TotalBytes))
	}
	for index, module := range r.Memory.Modules {
		manufacturer := valueOrUnknown(module.Manufacturer)
		capacity := unknownValue
		if module.CapacityBytes > 0 {
			capacity = formatBytesGB(module.CapacityBytes)
		}
		speed := unknownValue
		if module.SpeedMT > 0 {
			speed = fmt.Sprintf("%d MT/s", module.SpeedMT)
		}
		location := ""
		if strings.TrimSpace(module.Locator) != "" {
			location = " [" + strings.TrimSpace(module.Locator) + "]"
		}
		lines = append(lines, fmt.Sprintf("%d. %s%s — %s — %s", index+1, manufacturer, location, capacity, speed))
	}
	if len(lines) == 0 {
		return unknownValue
	}
	return strings.Join(lines, "\r\n")
}

func (r Report) GPUText() string {
	if len(r.GPUs) == 0 {
		return unknownValue
	}
	blocks := make([]string, 0, len(r.GPUs))
	for index, gpu := range r.GPUs {
		lines := []string{fmt.Sprintf("%d. %s — %s", index+1, valueOrUnknown(gpu.Manufacturer), valueOrUnknown(gpu.Model))}
		if gpu.DedicatedVideoMemory > 0 {
			lines = append(lines, "   Vyhrazená VRAM: "+formatBytesGB(gpu.DedicatedVideoMemory))
		} else {
			lines = append(lines, "   Vyhrazená VRAM: 0 GB")
		}
		if gpu.SharedSystemMemory > 0 {
			lines = append(lines, "   Sdílená systémová paměť: až "+formatBytesGB(gpu.SharedSystemMemory))
		}
		blocks = append(blocks, strings.Join(lines, "\r\n"))
	}
	return strings.Join(blocks, "\r\n\r\n")
}

func (r Report) DriveText() string {
	if len(r.Drives) == 0 {
		return unknownValue
	}
	lines := make([]string, 0, len(r.Drives))
	for _, drive := range r.Drives {
		capacity := unknownValue
		if drive.CapacityBytes > 0 {
			capacity = formatBytesGB(drive.CapacityBytes)
		}
		name := strings.TrimSpace(drive.Name)
		if name == "" {
			name = fmt.Sprintf("Fyzický disk %d", drive.DeviceNumber)
		}
		if strings.TrimSpace(drive.MediaType) != "" {
			name = strings.TrimSpace(drive.MediaType) + " — " + name
		}
		mountPoints := "bez přiřazeného písmene"
		if len(drive.MountPoints) > 0 {
			mountPoints = strings.Join(drive.MountPoints, ", ")
		}
		lines = append(lines, fmt.Sprintf("%s — %s — %s", name, capacity, mountPoints))
	}
	return strings.Join(lines, "\r\n")
}

func valueOrUnknown(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return unknownValue
	}
	return value
}

func formatBytesGB(value uint64) string {
	return formatDecimal(float64(value)/(1024*1024*1024), 2) + " GB"
}

func formatDecimal(value float64, decimals int) string {
	return strings.ReplaceAll(fmt.Sprintf("%.*f", decimals, value), ".", ",")
}
