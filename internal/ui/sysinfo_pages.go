package ui

import (
	"fmt"
	"strings"

	"jandy.cz/winfly/internal/systeminfo"
)

// The four tabs of the system information window. Each page has a
// constructor that lays out its controls in the given area, and a show
// method that fills them from a systeminfo report.

// Layout shared by the pages.
const (
	sysInfoLabelWidth = 130
	sysInfoListWidth  = 320
)

// ── Přehled ────────────────────────────────────────────────────────────────

type overviewPage struct {
	computerName, loggedInUser, directX control
	osEdition, osVersion, osBuild       control
	cpuModel, cpuManufacturer           control
	cpuFrequency, cpuCores, cpuThreads  control
}

func newOverviewPage(f *form, area box) overviewPage {
	var p overviewPage
	basicsBox, rest := area.splitTop(140, spacing)
	cpuBox, _ := rest.splitTop(168, spacing)

	basics := f.section("Základní informace", basicsBox, sysInfoLabelWidth).columns(2)
	p.computerName = basics[0].field("Název počítače:")
	p.loggedInUser = basics[0].field("Přihlášený uživatel:")
	p.directX = basics[0].field("Verze DirectX:")
	p.osEdition = basics[1].field("Edice Windows:")
	p.osVersion = basics[1].field("Verze Windows:")
	p.osBuild = basics[1].field("Sestavení Windows:")

	cpu := f.section("Procesor", cpuBox, sysInfoLabelWidth)
	p.cpuModel = cpu.textArea("Model:", 2)
	cpuColumns := cpu.columns(2)
	p.cpuManufacturer = cpuColumns[0].field("Výrobce:")
	p.cpuFrequency = cpuColumns[0].field("Frekvence:")
	p.cpuCores = cpuColumns[1].field("Fyzická jádra:")
	p.cpuThreads = cpuColumns[1].field("Logická vlákna:")
	return p
}

func (p overviewPage) show(report systeminfo.Report) {
	p.computerName.setText(textOrUnknown(report.ComputerName))
	p.loggedInUser.setText(textOrUnknown(report.LoggedInUser))
	p.directX.setText(textOrUnknown(report.DirectXVersion))
	p.osEdition.setText(textOrUnknown(report.OperatingSystem.Edition))
	p.osVersion.setText(textOrUnknown(report.OperatingSystem.Version))
	p.osBuild.setText(textOrUnknown(report.OperatingSystem.Build))

	p.cpuModel.setText(textOrUnknown(report.CPU.Model))
	p.cpuManufacturer.setText(textOrUnknown(report.CPU.Manufacturer))
	p.cpuFrequency.setText(formatFrequency(report.CPU.FrequencyMHz))
	p.cpuCores.setText(countOrUnknown(report.CPU.Cores))
	p.cpuThreads.setText(countOrUnknown(report.CPU.Threads))
}

// ── Paměť RAM ──────────────────────────────────────────────────────────────

type memoryPage struct {
	installed, usable control
	modules           *detailList[systeminfo.MemoryModule]
}

func newMemoryPage(f *form, area box) memoryPage {
	var p memoryPage
	summaryBox, modulesBox := area.splitTop(72, spacing)

	summary := f.section("Souhrn paměti", summaryBox, sysInfoLabelWidth).columns(2)
	p.installed = summary[0].field("Celkem osazeno:")
	p.usable = summary[1].field("Dostupné systému:")

	list, details := f.listWithDetails("Seznam modulů", "Podrobnosti vybraného modulu",
		modulesBox, sysInfoListWidth, idMemoryModuleList, sysInfoLabelWidth)
	manufacturer := details.field("Výrobce:")
	capacity := details.field("Kapacita:")
	speed := details.field("Rychlost:")
	locator := details.field("Slot / umístění:")

	p.modules = &detailList[systeminfo.MemoryModule]{
		list: list,
		describe: func(index int, module systeminfo.MemoryModule) string {
			name := strings.TrimSpace(module.Locator)
			if name == "" {
				name = fmt.Sprintf("Modul %d", index+1)
			}
			return fmt.Sprintf("%d. %s — %s", index+1, name, formatCapacity(module.CapacityBytes, false))
		},
		showDetails: func(module *systeminfo.MemoryModule) {
			if module == nil {
				setAll(notDetected, manufacturer, capacity, speed, locator)
				return
			}
			manufacturer.setText(textOrUnknown(module.Manufacturer))
			capacity.setText(formatCapacity(module.CapacityBytes, false))
			speed.setText(formatMemorySpeed(module.SpeedMT))
			locator.setText(textOrUnknown(module.Locator))
		},
	}
	return p
}

func (p memoryPage) show(memory systeminfo.MemoryInfo) {
	var installedBytes uint64
	for _, module := range memory.Modules {
		installedBytes += module.CapacityBytes
	}
	p.installed.setText(formatCapacity(installedBytes, false))
	p.usable.setText(formatCapacity(memory.TotalBytes, false))
	p.modules.setItems(memory.Modules)
}

func formatMemorySpeed(megatransfers uint32) string {
	if megatransfers == 0 {
		return notDetected
	}
	return fmt.Sprintf("%d MT/s", megatransfers)
}

// ── Grafické adaptéry ──────────────────────────────────────────────────────

type gpuPage struct {
	adapters *detailList[systeminfo.GPUInfo]
}

func newGPUPage(f *form, area box) gpuPage {
	list, details := f.listWithDetails("Seznam grafických adaptérů", "Podrobnosti vybraného adaptéru",
		area, sysInfoListWidth, idGPUList, sysInfoLabelWidth)
	manufacturer := details.field("Výrobce:")
	model := details.textArea("Model:", 2)
	dedicatedMemory := details.field("Vyhrazená VRAM:")
	sharedMemory := details.field("Sdílená paměť:")

	return gpuPage{adapters: &detailList[systeminfo.GPUInfo]{
		list: list,
		describe: func(index int, gpu systeminfo.GPUInfo) string {
			return fmt.Sprintf("%d. %s", index+1, textOrUnknown(gpu.Model))
		},
		showDetails: func(gpu *systeminfo.GPUInfo) {
			if gpu == nil {
				setAll(notDetected, manufacturer, model, dedicatedMemory, sharedMemory)
				return
			}
			manufacturer.setText(textOrUnknown(gpu.Manufacturer))
			model.setText(textOrUnknown(gpu.Model))
			dedicatedMemory.setText(formatCapacity(gpu.DedicatedVideoMemory, true))
			sharedMemory.setText(formatCapacity(gpu.SharedSystemMemory, true))
		},
	}}
}

// ── Disky ──────────────────────────────────────────────────────────────────

type drivePage struct {
	drives *detailList[systeminfo.DriveInfo]
}

func newDrivePage(f *form, area box) drivePage {
	list, details := f.listWithDetails("Seznam fyzických disků", "Podrobnosti vybraného disku",
		area, sysInfoListWidth, idDriveList, sysInfoLabelWidth)
	mediaType := details.field("Typ:")
	name := details.textArea("Název:", 2)
	deviceNumber := details.field("Číslo zařízení:")
	capacity := details.field("Celková kapacita:")
	mountPoints := details.textArea("Připojené jednotky:", 3)

	return drivePage{drives: &detailList[systeminfo.DriveInfo]{
		list: list,
		describe: func(_ int, drive systeminfo.DriveInfo) string {
			return textOrUnknown(drive.MediaType) + " — " + driveName(drive)
		},
		showDetails: func(drive *systeminfo.DriveInfo) {
			if drive == nil {
				setAll(notDetected, mediaType, name, deviceNumber, capacity, mountPoints)
				return
			}
			mediaType.setText(textOrUnknown(drive.MediaType))
			name.setText(driveName(*drive))
			deviceNumber.setText(fmt.Sprintf("%d", drive.DeviceNumber))
			capacity.setText(formatCapacity(drive.CapacityBytes, false))
			mountPoints.setText(driveMountPoints(*drive))
		},
	}}
}

// driveName is the drive's model name, or "Fyzický disk N" when it has none.
func driveName(drive systeminfo.DriveInfo) string {
	if name := strings.TrimSpace(drive.Name); name != "" {
		return name
	}
	return fmt.Sprintf("Fyzický disk %d", drive.DeviceNumber)
}

// driveMountPoints lists the drive letters of the drive, like "C:\, D:\".
func driveMountPoints(drive systeminfo.DriveInfo) string {
	if len(drive.MountPoints) == 0 {
		return "Bez přiřazeného písmene"
	}
	return strings.Join(drive.MountPoints, ", ")
}

// setAll puts the same text into every given control.
func setAll(text string, controls ...control) {
	for _, c := range controls {
		c.setText(text)
	}
}
