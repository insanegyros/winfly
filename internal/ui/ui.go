package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"jandy.cz/ess-provisioner/internal/config"
	"jandy.cz/ess-provisioner/internal/download"
	"jandy.cz/ess-provisioner/internal/install"
	"jandy.cz/ess-provisioner/internal/systemdialogs"
	"jandy.cz/ess-provisioner/internal/systeminfo"
)

const (
	mainWindowClass       = "ESSProvisionerMainWindow"
	logWindowClass        = "ESSProvisionerLogWindow"
	systemInfoWindowClass = "ESSProvisionerSystemInfoWindow"

	idAppList         = 1001
	idSelectAll       = 1002
	idInstallSelected = 1003
	idOverviewName    = 1010
	idOverviewDesc    = 1011
	idOverviewURL     = 1012
	idOverviewArgs    = 1013
	idOverviewSHA     = 1014

	idOpenPowerOptions      = 1101
	idOpenDomainOrWorkgroup = 1201
	idOpenSystemInfo        = 1251
	idToggleLog             = 1301
	idLog                   = 1302

	idSystemInfoTabs        = 1400
	idSystemComputerName    = 1401
	idSystemLoggedInUser    = 1402
	idSystemOSEdition       = 1403
	idSystemOSVersion       = 1404
	idSystemOSBuild         = 1405
	idSystemDirectX         = 1406
	idSystemCPUManufacturer = 1410
	idSystemCPUModel        = 1411
	idSystemCPUFrequency    = 1412
	idSystemCPUCores        = 1413
	idSystemCPUThreads      = 1414
	idSystemMemoryInstalled = 1420
	idSystemMemoryUsable    = 1421
	idSystemMemoryList      = 1422
	idSystemMemoryMaker     = 1423
	idSystemMemoryCapacity  = 1424
	idSystemMemorySpeed     = 1425
	idSystemMemoryLocator   = 1426
	idSystemGPUList         = 1430
	idSystemGPUManufacturer = 1431
	idSystemGPUModel        = 1432
	idSystemGPUVRAM         = 1433
	idSystemGPUSharedMemory = 1434
	idSystemDriveList       = 1440
	idSystemDriveType       = 1441
	idSystemDriveName       = 1442
	idSystemDriveNumber     = 1443
	idSystemDriveCapacity   = 1444
	idSystemDriveMounts     = 1445

	systemInfoPageOverview = 0
	systemInfoPageMemory   = 1
	systemInfoPageGPU      = 2
	systemInfoPageDrives   = 3
	systemInfoPageCount    = 4

	wmAppLog        = wmApp + 1
	wmAppResult     = wmApp + 2
	wmAppSystemInfo = wmApp + 3
)

type controls struct {
	appList         uintptr
	selectAll       uintptr
	installSelected uintptr
	overviewName    uintptr
	overviewDesc    uintptr
	overviewURL     uintptr
	overviewArgs    uintptr
	overviewSHA     uintptr
	openPower       uintptr
	openDomain      uintptr
	openSystemInfo  uintptr
	toggleLog       uintptr
}

type systemInfoControls struct {
	tabs  uintptr
	pages [systemInfoPageCount][]uintptr

	computerName uintptr
	loggedInUser uintptr
	osEdition    uintptr
	osVersion    uintptr
	osBuild      uintptr
	directX      uintptr

	cpuManufacturer uintptr
	cpuModel        uintptr
	cpuFrequency    uintptr
	cpuCores        uintptr
	cpuThreads      uintptr

	memoryInstalled    uintptr
	memoryUsable       uintptr
	memoryList         uintptr
	memoryManufacturer uintptr
	memoryCapacity     uintptr
	memorySpeed        uintptr
	memoryLocator      uintptr

	gpuList         uintptr
	gpuManufacturer uintptr
	gpuModel        uintptr
	gpuVRAM         uintptr
	gpuSharedMemory uintptr

	driveList     uintptr
	driveType     uintptr
	driveName     uintptr
	driveNumber   uintptr
	driveCapacity uintptr
	driveMounts   uintptr
}

type operationResult struct {
	name        string
	successText string
	err         error
}

type state struct {
	cfg        *config.Config
	appListErr error
	apps       []config.AppConfig
	downloader *download.Manager
	instance   uintptr
	hwnd       uintptr
	font       uintptr
	controls   controls
	createErr  error

	logWindow    uintptr
	logEdit      uintptr
	logCreateErr error
	logLines     []string

	systemInfoWindow    uintptr
	systemInfoControls  systemInfoControls
	systemInfoCreateErr error
	systemInfoReport    systeminfo.Report
	systemInfoPage      int
	systemInfoLoaded    bool
	systemInfoLoading   atomic.Bool

	busy         atomic.Bool
	logCh        chan string
	resultCh     chan operationResult
	systemInfoCh chan systeminfo.Report
}

var (
	activeState       *state
	mainWindowProcPtr = syscall.NewCallback(mainWindowProc)
	logWindowProcPtr  = syscall.NewCallback(logWindowProc)
	systemInfoProcPtr = syscall.NewCallback(systemInfoWindowProc)
)

func Run(cfg *config.Config, appListErr error) error {
	if cfg == nil {
		return errors.New("konfigurace je nil")
	}
	downloader, err := download.New(cfg.Network, cfg.CacheDirectory)
	if err != nil {
		return fmt.Errorf("inicializace správce stahování: %w", err)
	}
	instance, err := moduleHandle()
	if err != nil {
		return err
	}
	if err := initCommonControls(); err != nil {
		return err
	}
	if err := registerWindowClass(instance, mainWindowClass, mainWindowProcPtr); err != nil {
		return err
	}
	if err := registerWindowClass(instance, logWindowClass, logWindowProcPtr); err != nil {
		return err
	}
	if err := registerWindowClass(instance, systemInfoWindowClass, systemInfoProcPtr); err != nil {
		return err
	}

	s := &state{
		cfg:          cfg,
		appListErr:   appListErr,
		apps:         cfg.EnabledApps(),
		downloader:   downloader,
		instance:     instance,
		font:         stockGUIFont(),
		logCh:        make(chan string, 256),
		resultCh:     make(chan operationResult, 8),
		systemInfoCh: make(chan systeminfo.Report, 1),
	}
	activeState = s
	defer func() { activeState = nil }()

	const cwUseDefault int32 = -2147483648
	hwnd, err := createWindow(
		0,
		mainWindowClass,
		cfg.ProductName,
		wsOverlapped|wsCaption|wsSysMenu|wsMinimizeBox,
		cwUseDefault,
		cwUseDefault,
		1020,
		500,
		0,
		0,
		instance,
	)
	if err != nil {
		if s.createErr != nil {
			return s.createErr
		}
		return err
	}
	s.hwnd = hwnd
	if err := s.createLogWindow(); err != nil {
		return err
	}
	if err := s.createSystemInfoWindow(); err != nil {
		return err
	}

	showAndUpdateWindow(hwnd)
	s.logf("Připraveno. ESSENS Provisioner běží s oprávněním správce.")
	if s.appListErr != nil {
		s.logf("Vzdálený seznam aplikací se nepodařilo načíst: %v", s.appListErr)
		s.logf("Instalace aplikací je zakázána a seznam je prázdný.")
	} else if len(s.apps) == 0 {
		s.logf("Vzdálená konfigurace neobsahuje žádnou povolenou aplikaci.")
	} else {
		s.logf("Vzdálený seznam aplikací byl načten. Počet povolených aplikací: %d.", len(s.apps))
	}
	return runMessageLoop()
}

func mainWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	s := activeState
	if s == nil {
		return defWindowProc(hwnd, msg, wParam, lParam)
	}

	switch msg {
	case wmCreate:
		s.hwnd = hwnd
		if err := s.createControls(); err != nil {
			s.createErr = err
			return ^uintptr(0)
		}
		return 0
	case wmCommand:
		s.handleCommand(int(lowWord(wParam)), int(highWord(wParam)))
		return 0
	case wmAppLog:
		s.drainLogs()
		return 0
	case wmAppResult:
		s.drainResults()
		return 0
	case wmAppSystemInfo:
		s.drainSystemInfoResults()
		return 0
	case wmQueryEndSession:
		if s.busy.Load() {
			return 0
		}
		return 1
	case wmClose:
		if s.busy.Load() {
			showMessage(hwnd, s.cfg.ProductName, "Instalátor stále běží. Před zavřením tohoto okna jej dokončete.", mbIconWarning)
			return 0
		}
		return defWindowProc(hwnd, msg, wParam, lParam)
	case wmDestroy:
		if s.logWindow != 0 {
			destroyWindow(s.logWindow)
		}
		if s.systemInfoWindow != 0 {
			destroyWindow(s.systemInfoWindow)
		}
		postQuitMessage(0)
		return 0
	default:
		return defWindowProc(hwnd, msg, wParam, lParam)
	}
}

func systemInfoWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	s := activeState
	if s == nil {
		return defWindowProc(hwnd, msg, wParam, lParam)
	}

	switch msg {
	case wmCreate:
		s.systemInfoWindow = hwnd
		if err := s.createSystemInfoControls(); err != nil {
			s.systemInfoCreateErr = err
			return ^uintptr(0)
		}
		return 0
	case wmCommand:
		s.handleSystemInfoCommand(int(lowWord(wParam)), int(highWord(wParam)))
		return 0
	case wmNotify:
		if int(wParam) == idSystemInfoTabs {
			s.showSystemInfoPage(tabCurrentIndex(s.systemInfoControls.tabs))
		}
		return 0
	case wmClose:
		showWindow(hwnd, false)
		return 0
	case wmDestroy:
		s.systemInfoWindow = 0
		s.systemInfoControls = systemInfoControls{}
		return 0
	default:
		return defWindowProc(hwnd, msg, wParam, lParam)
	}
}

func logWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	s := activeState
	if s == nil {
		return defWindowProc(hwnd, msg, wParam, lParam)
	}

	switch msg {
	case wmCreate:
		s.logWindow = hwnd
		if err := s.createLogControls(); err != nil {
			s.logCreateErr = err
			return ^uintptr(0)
		}
		return 0
	case wmSize:
		s.resizeLogControl()
		return 0
	case wmClose:
		showWindow(hwnd, false)
		s.updateLogButtonText()
		return 0
	case wmDestroy:
		s.logWindow = 0
		s.logEdit = 0
		return 0
	default:
		return defWindowProc(hwnd, msg, wParam, lParam)
	}
}

func (s *state) createControls() error {
	create := func(exStyle uint32, className, text string, style uint32, x, y, width, height int32, id int) (uintptr, error) {
		hwnd, err := createWindow(exStyle, className, text, style, x, y, width, height, s.hwnd, uintptr(id), s.instance)
		if err != nil {
			return 0, err
		}
		applyFont(hwnd, s.font)
		return hwnd, nil
	}
	label := func(text string, x, y, width, height int32) error {
		_, err := create(0, "STATIC", text, wsChild|wsVisible, x, y, width, height, 0)
		return err
	}
	group := func(text string, x, y, width, height int32) error {
		_, err := create(0, "BUTTON", text, wsChild|wsVisible|bsGroupBox, x, y, width, height, 0)
		return err
	}
	readOnlyEdit := func(x, y, width, height int32, id int, multiline bool) (uintptr, error) {
		style := uint32(wsChild | wsVisible | esReadOnly | esAutoHScroll)
		if multiline {
			style = wsChild | wsVisible | wsVScroll | esMultiline | esAutoVScroll | esReadOnly
		}
		return create(wsExClientEdge, "EDIT", "", style, x, y, width, height, id)
	}

	if err := group("Instalace aplikací", 18, 16, 964, 384); err != nil {
		return err
	}
	var err error
	//if err := group("Výběr aplikací", 30, 42, 314, 338); err != nil {
	//	return err
	//}
	s.controls.appList, err = create(wsExClientEdge, "LISTBOX", "", wsChild|wsVisible|wsVScroll|wsTabStop|lbsNotify|lbsNoIntegralHeight|lbsExtendedSel, 46, 42, 282, 290, idAppList)
	if err != nil {
		return err
	}
	s.controls.selectAll, err = create(0, "BUTTON", "Vybrat vše", wsChild|wsVisible|wsTabStop|bsPushButton, 46, 348, 104, 30, idSelectAll)
	if err != nil {
		return err
	}
	s.controls.installSelected, err = create(0, "BUTTON", "Nainstalovat vybrané", wsChild|wsVisible|wsTabStop|bsPushButton, 168, 348, 160, 30, idInstallSelected)
	if err != nil {
		return err
	}

	if err := group("Přehled aplikace", 350, 42, 612, 338); err != nil {
		return err
	}
	if err := label("Název:", 368, 73, 132, 22); err != nil {
		return err
	}
	s.controls.overviewName, err = readOnlyEdit(510, 68, 430, 25, idOverviewName, false)
	if err != nil {
		return err
	}
	if err := label("Popis:", 368, 109, 132, 22); err != nil {
		return err
	}
	s.controls.overviewDesc, err = readOnlyEdit(510, 104, 430, 76, idOverviewDesc, true)
	if err != nil {
		return err
	}
	if err := label("Adresa pro stažení:", 368, 194, 132, 22); err != nil {
		return err
	}
	s.controls.overviewURL, err = readOnlyEdit(510, 189, 430, 25, idOverviewURL, false)
	if err != nil {
		return err
	}
	if err := label("Parametry instalátoru:", 368, 230, 132, 36); err != nil {
		return err
	}
	s.controls.overviewArgs, err = readOnlyEdit(510, 225, 430, 55, idOverviewArgs, true)
	if err != nil {
		return err
	}
	if err := label("Platnost SHA-256:", 368, 298, 132, 22); err != nil {
		return err
	}
	s.controls.overviewSHA, err = create(0, "STATIC", "", wsChild|wsVisible, 510, 294, 430, 28, idOverviewSHA)
	if err != nil {
		return err
	}

	if len(s.apps) == 0 {
		enableWindow(s.controls.appList, false)
		enableWindow(s.controls.selectAll, false)
		enableWindow(s.controls.installSelected, false)
		s.updateEmptyAppOverview()
	} else {
		for _, app := range s.apps {
			addListItem(s.controls.appList, app.Name)
		}
		selectListItem(s.controls.appList, 0, true)
		s.updateAppOverview()
	}

	//if err := group("Nastavení napájení", 18, 416, 464, 130); err != nil {
	//	return err
	//}
	//if err := label("Otevře klasické Ovládací panely s nastavením tlačítek napájení a rychlého spuštění. ESSENS Provisioner nastavení přímo nemění.", 34, 448, 420, 42); err != nil {
	//	return err
	//}
	s.controls.openPower, err = create(0, "BUTTON", "Otevřít nastavení napájení", wsChild|wsVisible|wsTabStop|bsPushButton, 16, 408, 186, 30, idOpenPowerOptions)
	if err != nil {
		return err
	}

	//if err := group("Doména nebo pracovní skupina počítače", 500, 416, 482, 130); err != nil {
	//	return err
	//}
	//if err := label("Otevře Vlastnosti systému pro správu názvu počítače, domény nebo pracovní skupiny. ESSENS Provisioner počítač do domény přímo nepřipojuje.", 516, 448, 446, 42); err != nil {
	//	return err
	//}
	s.controls.openDomain, err = create(0, "BUTTON", "Otevřít doménu / skupinu", wsChild|wsVisible|wsTabStop|bsPushButton, 210, 408, 232, 30, idOpenDomainOrWorkgroup)
	if err != nil {
		return err
	}

	//if err := label("Průběh instalace se zapisuje do samostatného okna protokolu.", 18, 573, 620, 24); err != nil {
	//	return err
	//}
	s.controls.openSystemInfo, err = create(0, "BUTTON", "Systémové informace", wsChild|wsVisible|wsTabStop|bsPushButton, 594, 408, 200, 30, idOpenSystemInfo)
	if err != nil {
		return err
	}

	s.controls.toggleLog, err = create(0, "BUTTON", "Zobrazit log", wsChild|wsVisible|wsTabStop|bsPushButton, 806, 408, 176, 30, idToggleLog)
	if err != nil {
		return err
	}
	return nil
}

func (s *state) createLogWindow() error {
	const cwUseDefault int32 = -2147483648
	hwnd, err := createWindow(
		0,
		logWindowClass,
		s.cfg.ProductName+" — Log",
		wsOverlapped|wsCaption|wsSysMenu|wsMinimizeBox|wsMaximizeBox|wsThickFrame,
		cwUseDefault,
		cwUseDefault,
		780,
		500,
		s.hwnd,
		0,
		s.instance,
	)
	if err != nil {
		if s.logCreateErr != nil {
			return s.logCreateErr
		}
		return err
	}
	s.logWindow = hwnd
	return nil
}

func (s *state) createSystemInfoWindow() error {
	const cwUseDefault int32 = -2147483648
	hwnd, err := createWindow(
		0,
		systemInfoWindowClass,
		s.cfg.ProductName+" — Systémové informace",
		wsOverlapped|wsCaption|wsSysMenu|wsMinimizeBox,
		cwUseDefault,
		cwUseDefault,
		1000,
		700,
		s.hwnd,
		0,
		s.instance,
	)
	if err != nil {
		if s.systemInfoCreateErr != nil {
			return s.systemInfoCreateErr
		}
		return err
	}
	s.systemInfoWindow = hwnd
	return nil
}

func (s *state) createSystemInfoControls() error {
	create := func(exStyle uint32, className, text string, style uint32, x, y, width, height int32, id int) (uintptr, error) {
		hwnd, err := createWindow(exStyle, className, text, style, x, y, width, height, s.systemInfoWindow, uintptr(id), s.instance)
		if err != nil {
			return 0, err
		}
		applyFont(hwnd, s.font)
		return hwnd, nil
	}
	pageCreate := func(page int, exStyle uint32, className, text string, style uint32, x, y, width, height int32, id int) (uintptr, error) {
		hwnd, err := create(exStyle, className, text, style, x, y, width, height, id)
		if err != nil {
			return 0, err
		}
		s.systemInfoControls.pages[page] = append(s.systemInfoControls.pages[page], hwnd)
		return hwnd, nil
	}
	label := func(page int, text string, x, y, width, height int32) error {
		_, err := pageCreate(page, 0, "STATIC", text, wsChild|wsVisible, x, y, width, height, 0)
		return err
	}
	group := func(page int, text string, x, y, width, height int32) error {
		_, err := pageCreate(page, 0, "BUTTON", text, wsChild|wsVisible|bsGroupBox, x, y, width, height, 0)
		return err
	}
	readOnlyEdit := func(page int, x, y, width, height int32, id int, multiline bool) (uintptr, error) {
		style := uint32(wsChild | wsVisible | esReadOnly | esAutoHScroll)
		if multiline {
			style = wsChild | wsVisible | wsVScroll | esMultiline | esAutoVScroll | esReadOnly
		}
		return pageCreate(page, wsExClientEdge, "EDIT", "", style, x, y, width, height, id)
	}
	list := func(page int, x, y, width, height int32, id int) (uintptr, error) {
		style := uint32(wsChild | wsVisible | wsVScroll | wsTabStop | lbsNotify | lbsNoIntegralHeight)
		return pageCreate(page, wsExClientEdge, "LISTBOX", "", style, x, y, width, height, id)
	}

	var err error
	s.systemInfoControls.tabs, err = create(0, "SysTabControl32", "", wsChild|wsVisible|wsTabStop, 16, 16, 952, 626, idSystemInfoTabs)
	if err != nil {
		return err
	}
	for index, title := range []string{"Přehled", "Paměť RAM", "Grafické adaptéry", "Disky"} {
		if err := insertTabItem(s.systemInfoControls.tabs, index, title); err != nil {
			return err
		}
	}

	// Přehled: základní informace a procesor.
	if err := group(systemInfoPageOverview, "Základní informace", 32, 52, 920, 190); err != nil {
		return err
	}
	if err := label(systemInfoPageOverview, "Název počítače:", 50, 84, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.computerName, err = readOnlyEdit(systemInfoPageOverview, 190, 79, 275, 25, idSystemComputerName, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageOverview, "Přihlášený uživatel:", 50, 122, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.loggedInUser, err = readOnlyEdit(systemInfoPageOverview, 190, 117, 275, 25, idSystemLoggedInUser, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageOverview, "Verze DirectX:", 50, 160, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.directX, err = readOnlyEdit(systemInfoPageOverview, 190, 155, 275, 25, idSystemDirectX, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageOverview, "Edice Windows:", 494, 84, 126, 22); err != nil {
		return err
	}
	s.systemInfoControls.osEdition, err = readOnlyEdit(systemInfoPageOverview, 628, 79, 300, 25, idSystemOSEdition, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageOverview, "Verze Windows:", 494, 122, 126, 22); err != nil {
		return err
	}
	s.systemInfoControls.osVersion, err = readOnlyEdit(systemInfoPageOverview, 628, 117, 300, 25, idSystemOSVersion, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageOverview, "Sestavení Windows:", 494, 160, 126, 22); err != nil {
		return err
	}
	s.systemInfoControls.osBuild, err = readOnlyEdit(systemInfoPageOverview, 628, 155, 300, 25, idSystemOSBuild, false)
	if err != nil {
		return err
	}

	if err := group(systemInfoPageOverview, "Procesor", 32, 254, 920, 346); err != nil {
		return err
	}
	if err := label(systemInfoPageOverview, "Výrobce:", 50, 288, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.cpuManufacturer, err = readOnlyEdit(systemInfoPageOverview, 190, 283, 275, 25, idSystemCPUManufacturer, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageOverview, "Model:", 50, 326, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.cpuModel, err = readOnlyEdit(systemInfoPageOverview, 190, 321, 738, 68, idSystemCPUModel, true)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageOverview, "Frekvence:", 50, 410, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.cpuFrequency, err = readOnlyEdit(systemInfoPageOverview, 190, 405, 275, 25, idSystemCPUFrequency, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageOverview, "Fyzická jádra:", 494, 410, 126, 22); err != nil {
		return err
	}
	s.systemInfoControls.cpuCores, err = readOnlyEdit(systemInfoPageOverview, 628, 405, 300, 25, idSystemCPUCores, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageOverview, "Logická vlákna:", 494, 448, 126, 22); err != nil {
		return err
	}
	s.systemInfoControls.cpuThreads, err = readOnlyEdit(systemInfoPageOverview, 628, 443, 300, 25, idSystemCPUThreads, false)
	if err != nil {
		return err
	}

	// Paměť RAM: souhrn, seznam modulů a podrobnosti vybraného modulu.
	if err := group(systemInfoPageMemory, "Souhrn paměti", 32, 52, 920, 106); err != nil {
		return err
	}
	if err := label(systemInfoPageMemory, "Celkem osazeno:", 50, 86, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.memoryInstalled, err = readOnlyEdit(systemInfoPageMemory, 190, 81, 275, 25, idSystemMemoryInstalled, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageMemory, "Dostupné systému:", 494, 86, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.memoryUsable, err = readOnlyEdit(systemInfoPageMemory, 636, 81, 292, 25, idSystemMemoryUsable, false)
	if err != nil {
		return err
	}
	if err := group(systemInfoPageMemory, "Seznam modulů", 32, 170, 330, 430); err != nil {
		return err
	}
	s.systemInfoControls.memoryList, err = list(systemInfoPageMemory, 50, 198, 294, 380, idSystemMemoryList)
	if err != nil {
		return err
	}
	if err := group(systemInfoPageMemory, "Podrobnosti vybraného modulu", 374, 170, 578, 430); err != nil {
		return err
	}
	if err := label(systemInfoPageMemory, "Výrobce:", 394, 210, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.memoryManufacturer, err = readOnlyEdit(systemInfoPageMemory, 536, 205, 392, 25, idSystemMemoryMaker, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageMemory, "Kapacita:", 394, 250, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.memoryCapacity, err = readOnlyEdit(systemInfoPageMemory, 536, 245, 392, 25, idSystemMemoryCapacity, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageMemory, "Rychlost:", 394, 290, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.memorySpeed, err = readOnlyEdit(systemInfoPageMemory, 536, 285, 392, 25, idSystemMemorySpeed, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageMemory, "Slot / umístění:", 394, 330, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.memoryLocator, err = readOnlyEdit(systemInfoPageMemory, 536, 325, 392, 25, idSystemMemoryLocator, false)
	if err != nil {
		return err
	}

	// Grafické adaptéry: seznam všech hardwarových adaptérů a detail vybraného.
	if err := group(systemInfoPageGPU, "Seznam grafických adaptérů", 32, 52, 330, 548); err != nil {
		return err
	}
	s.systemInfoControls.gpuList, err = list(systemInfoPageGPU, 50, 80, 294, 498, idSystemGPUList)
	if err != nil {
		return err
	}
	if err := group(systemInfoPageGPU, "Podrobnosti vybraného adaptéru", 374, 52, 578, 548); err != nil {
		return err
	}
	if err := label(systemInfoPageGPU, "Výrobce:", 394, 92, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.gpuManufacturer, err = readOnlyEdit(systemInfoPageGPU, 536, 87, 392, 25, idSystemGPUManufacturer, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageGPU, "Model:", 394, 132, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.gpuModel, err = readOnlyEdit(systemInfoPageGPU, 536, 127, 392, 68, idSystemGPUModel, true)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageGPU, "Vyhrazená VRAM:", 394, 214, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.gpuVRAM, err = readOnlyEdit(systemInfoPageGPU, 536, 209, 392, 25, idSystemGPUVRAM, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageGPU, "Sdílená paměť:", 394, 254, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.gpuSharedMemory, err = readOnlyEdit(systemInfoPageGPU, 536, 249, 392, 25, idSystemGPUSharedMemory, false)
	if err != nil {
		return err
	}
	//if err := label(systemInfoPageGPU, "Takty GPU a grafické paměti nejsou součástí standardního rozhraní DXGI a aplikace je bez knihoven výrobce nezjišťuje.", 394, 304, 520, 58); err != nil {
	//	return err
	//}

	// Disky: seznam fyzických zařízení a podrobnosti vybraného disku.
	if err := group(systemInfoPageDrives, "Seznam fyzických disků", 32, 52, 330, 548); err != nil {
		return err
	}
	s.systemInfoControls.driveList, err = list(systemInfoPageDrives, 50, 80, 294, 498, idSystemDriveList)
	if err != nil {
		return err
	}
	if err := group(systemInfoPageDrives, "Podrobnosti vybraného disku", 374, 52, 578, 548); err != nil {
		return err
	}
	if err := label(systemInfoPageDrives, "Typ:", 394, 92, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.driveType, err = readOnlyEdit(systemInfoPageDrives, 536, 87, 392, 25, idSystemDriveType, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageDrives, "Název:", 394, 132, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.driveName, err = readOnlyEdit(systemInfoPageDrives, 536, 127, 392, 50, idSystemDriveName, true)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageDrives, "Číslo zařízení:", 394, 194, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.driveNumber, err = readOnlyEdit(systemInfoPageDrives, 536, 189, 392, 25, idSystemDriveNumber, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageDrives, "Celková kapacita:", 394, 234, 132, 22); err != nil {
		return err
	}
	s.systemInfoControls.driveCapacity, err = readOnlyEdit(systemInfoPageDrives, 536, 229, 392, 25, idSystemDriveCapacity, false)
	if err != nil {
		return err
	}
	if err := label(systemInfoPageDrives, "Připojené jednotky:", 394, 274, 132, 36); err != nil {
		return err
	}
	s.systemInfoControls.driveMounts, err = readOnlyEdit(systemInfoPageDrives, 536, 269, 392, 72, idSystemDriveMounts, true)
	if err != nil {
		return err
	}

	s.setSystemInfoLoadingText()
	s.showSystemInfoPage(systemInfoPageOverview)
	return nil
}

func (s *state) createLogControls() error {
	hwnd, err := createWindow(
		wsExClientEdge,
		"EDIT",
		strings.Join(s.logLines, "\r\n"),
		wsChild|wsVisible|wsVScroll|wsHScroll|esMultiline|esAutoVScroll|esAutoHScroll|esReadOnly,
		12,
		12,
		720,
		430,
		s.logWindow,
		idLog,
		s.instance,
	)
	if err != nil {
		return err
	}
	s.logEdit = hwnd
	applyFont(hwnd, s.font)
	s.resizeLogControl()
	return nil
}

func (s *state) resizeLogControl() {
	if s.logWindow == 0 || s.logEdit == 0 {
		return
	}
	client, err := getClientRect(s.logWindow)
	if err != nil {
		return
	}
	const margin int32 = 12
	width := client.Right - client.Left - 2*margin
	height := client.Bottom - client.Top - 2*margin
	if width < 1 || height < 1 {
		return
	}
	moveWindow(s.logEdit, margin, margin, width, height, true)
}

func (s *state) handleCommand(id, notification int) {
	switch {
	case id == idSelectAll && notification == bnClicked:
		selectAllListItems(s.controls.appList)
		s.updateAppOverview()
	case id == idInstallSelected && notification == bnClicked:
		s.installSelectedApps()
	case id == idOpenPowerOptions && notification == bnClicked:
		s.openSystemDialog("Nastavení napájení", systemdialogs.OpenPowerOptions)
	case id == idOpenDomainOrWorkgroup && notification == bnClicked:
		s.openSystemDialog("Doména nebo pracovní skupina", systemdialogs.OpenDomainOrWorkgroup)
	case id == idOpenSystemInfo && notification == bnClicked:
		s.openSystemInfoWindow()
	case id == idToggleLog && notification == bnClicked:
		s.toggleLogWindow()
	case id == idAppList && notification == lbnSelChange:
		s.updateAppOverview()
	}
}

func (s *state) updateAppOverview() {
	if len(s.apps) == 0 {
		s.updateEmptyAppOverview()
		return
	}
	index := listCaretIndex(s.controls.appList)
	if index < 0 || index >= len(s.apps) {
		selected := listSelections(s.controls.appList)
		if len(selected) == 0 {
			s.clearAppOverview("Vyberte aplikaci")
			return
		}
		index = selected[0]
	}
	app := s.apps[index]
	description := app.Description
	if description == "" {
		description = "(neuvedeno)"
	}
	arguments := "(neuvedeno)"
	if len(app.Arguments) > 0 {
		arguments = strings.Join(app.Arguments, " ")
	}
	shaStatus := "✗ Chybí nebo je neplatný — ověřování je vypnuto"
	if app.HasValidSHA256() {
		shaStatus = "✓ Platný — ověřování je zapnuto"
	}
	_ = setWindowText(s.controls.overviewName, app.Name)
	_ = setWindowText(s.controls.overviewDesc, description)
	_ = setWindowText(s.controls.overviewURL, app.URL)
	_ = setWindowText(s.controls.overviewArgs, arguments)
	_ = setWindowText(s.controls.overviewSHA, shaStatus)
}

func (s *state) updateEmptyAppOverview() {
	name := "Žádné dostupné aplikace"
	description := "Vzdálená konfigurace neobsahuje žádnou povolenou aplikaci."
	if s.appListErr != nil {
		name = "Seznam aplikací není k dispozici"
		description = "Načtení vzdálené konfigurace selhalo. Instalace aplikací je zakázána."
	}
	_ = setWindowText(s.controls.overviewName, name)
	_ = setWindowText(s.controls.overviewDesc, description)
	_ = setWindowText(s.controls.overviewURL, "")
	_ = setWindowText(s.controls.overviewArgs, "")
	_ = setWindowText(s.controls.overviewSHA, "—")
}

func (s *state) clearAppOverview(name string) {
	_ = setWindowText(s.controls.overviewName, name)
	_ = setWindowText(s.controls.overviewDesc, "")
	_ = setWindowText(s.controls.overviewURL, "")
	_ = setWindowText(s.controls.overviewArgs, "")
	_ = setWindowText(s.controls.overviewSHA, "—")
}

func (s *state) installSelectedApps() {
	indices := listSelections(s.controls.appList)
	if len(indices) == 0 {
		showMessage(s.hwnd, s.cfg.ProductName, "Vyberte alespoň jednu aplikaci.", mbIconWarning)
		return
	}
	apps := make([]config.AppConfig, 0, len(indices))
	for _, index := range indices {
		if index >= 0 && index < len(s.apps) {
			apps = append(apps, s.apps[index])
		}
	}
	if len(apps) == 0 {
		return
	}

	s.startOperation("Instalace aplikací", "Instalace vybraných aplikací byla dokončena.", func(ctx context.Context) error {
		var failures []error
		restartRequired := false
		for _, app := range apps {
			path, err := s.downloader.Fetch(ctx, app, s.logf)
			if err != nil {
				s.logf("%s: operace selhala: %v", app.Name, err)
				failures = append(failures, fmt.Errorf("%s: %w", app.Name, err))
				continue
			}
			result, err := install.Run(ctx, app, path, s.logf)
			if err != nil {
				s.logf("%s: operace selhala: %v", app.Name, err)
				failures = append(failures, fmt.Errorf("%s: %w", app.Name, err))
				continue
			}
			restartRequired = restartRequired || result.RestartRequired
		}
		if restartRequired {
			s.logf("Alespoň jeden instalátor požaduje restart systému Windows.")
		}
		return errors.Join(failures...)
	})
}

func (s *state) openSystemDialog(name string, open func() error) {
	if err := open(); err != nil {
		s.logf("Nepodařilo se otevřít %s: %v", name, err)
		showMessage(s.hwnd, name, err.Error(), mbIconError)
		return
	}
	s.logf("Otevřeno: %s.", name)
}

func (s *state) handleSystemInfoCommand(id, notification int) {
	if notification != lbnSelChange {
		return
	}
	switch id {
	case idSystemMemoryList:
		s.updateSelectedMemoryModule()
	case idSystemGPUList:
		s.updateSelectedGPU()
	case idSystemDriveList:
		s.updateSelectedDrive()
	}
}

func (s *state) showSystemInfoPage(page int) {
	if page < 0 || page >= systemInfoPageCount {
		page = systemInfoPageOverview
	}
	for pageIndex, handles := range s.systemInfoControls.pages {
		visible := pageIndex == page
		for _, hwnd := range handles {
			showWindow(hwnd, visible)
		}
	}
	s.systemInfoPage = page
}

func (s *state) openSystemInfoWindow() {
	if s.systemInfoWindow == 0 {
		return
	}
	showAndUpdateWindow(s.systemInfoWindow)
	bringWindowToFront(s.systemInfoWindow)

	if s.systemInfoLoaded || !s.systemInfoLoading.CompareAndSwap(false, true) {
		return
	}
	s.setSystemInfoLoadingText()
	s.logf("Načítají se systémové informace.")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		report := systeminfo.Collect(ctx)
		s.systemInfoCh <- report
		postMessage(s.hwnd, wmAppSystemInfo, 0, 0)
	}()
}

func (s *state) setSystemInfoLoadingText() {
	const loading = "Načítání..."
	for _, hwnd := range []uintptr{
		s.systemInfoControls.computerName,
		s.systemInfoControls.loggedInUser,
		s.systemInfoControls.osEdition,
		s.systemInfoControls.osVersion,
		s.systemInfoControls.osBuild,
		s.systemInfoControls.directX,
		s.systemInfoControls.cpuManufacturer,
		s.systemInfoControls.cpuModel,
		s.systemInfoControls.cpuFrequency,
		s.systemInfoControls.cpuCores,
		s.systemInfoControls.cpuThreads,
		s.systemInfoControls.memoryInstalled,
		s.systemInfoControls.memoryUsable,
		s.systemInfoControls.memoryManufacturer,
		s.systemInfoControls.memoryCapacity,
		s.systemInfoControls.memorySpeed,
		s.systemInfoControls.memoryLocator,
		s.systemInfoControls.gpuManufacturer,
		s.systemInfoControls.gpuModel,
		s.systemInfoControls.gpuVRAM,
		s.systemInfoControls.gpuSharedMemory,
		s.systemInfoControls.driveType,
		s.systemInfoControls.driveName,
		s.systemInfoControls.driveNumber,
		s.systemInfoControls.driveCapacity,
		s.systemInfoControls.driveMounts,
	} {
		_ = setWindowText(hwnd, loading)
	}
	for _, hwnd := range []uintptr{
		s.systemInfoControls.memoryList,
		s.systemInfoControls.gpuList,
		s.systemInfoControls.driveList,
	} {
		clearList(hwnd)
		addListItem(hwnd, loading)
		enableWindow(hwnd, false)
	}
}

func (s *state) updateSystemInfoControls(report systeminfo.Report) {
	s.systemInfoReport = report
	_ = setWindowText(s.systemInfoControls.computerName, systemInfoValue(report.ComputerName))
	_ = setWindowText(s.systemInfoControls.loggedInUser, systemInfoValue(report.LoggedInUser))
	_ = setWindowText(s.systemInfoControls.osEdition, systemInfoValue(report.OperatingSystem.Edition))
	_ = setWindowText(s.systemInfoControls.osVersion, systemInfoValue(report.OperatingSystem.Version))
	_ = setWindowText(s.systemInfoControls.osBuild, systemInfoValue(report.OperatingSystem.Build))
	_ = setWindowText(s.systemInfoControls.directX, systemInfoValue(report.DirectXVersion))

	_ = setWindowText(s.systemInfoControls.cpuManufacturer, systemInfoValue(report.CPU.Manufacturer))
	_ = setWindowText(s.systemInfoControls.cpuModel, systemInfoValue(report.CPU.Model))
	_ = setWindowText(s.systemInfoControls.cpuFrequency, systemInfoFrequency(report.CPU.FrequencyMHz))
	_ = setWindowText(s.systemInfoControls.cpuCores, systemInfoCount(report.CPU.Cores))
	_ = setWindowText(s.systemInfoControls.cpuThreads, systemInfoCount(report.CPU.Threads))

	var installedBytes uint64
	for _, module := range report.Memory.Modules {
		installedBytes += module.CapacityBytes
	}
	_ = setWindowText(s.systemInfoControls.memoryInstalled, systemInfoCapacity(installedBytes, false))
	_ = setWindowText(s.systemInfoControls.memoryUsable, systemInfoCapacity(report.Memory.TotalBytes, false))

	s.populateMemoryModuleList()
	s.populateGPUList()
	s.populateDriveList()
}

func (s *state) populateMemoryModuleList() {
	list := s.systemInfoControls.memoryList
	clearList(list)
	if len(s.systemInfoReport.Memory.Modules) == 0 {
		addListItem(list, "Nezjištěno")
		enableWindow(list, false)
		s.setMemoryModuleDetails(nil)
		return
	}
	for index, module := range s.systemInfoReport.Memory.Modules {
		locator := strings.TrimSpace(module.Locator)
		if locator == "" {
			locator = fmt.Sprintf("Modul %d", index+1)
		}
		addListItem(list, fmt.Sprintf("%d. %s — %s", index+1, locator, systemInfoCapacity(module.CapacityBytes, false)))
	}
	enableWindow(list, true)
	selectSingleListItem(list, 0)
	s.updateSelectedMemoryModule()
}

func (s *state) updateSelectedMemoryModule() {
	index := listCurrentIndex(s.systemInfoControls.memoryList)
	if index < 0 || index >= len(s.systemInfoReport.Memory.Modules) {
		s.setMemoryModuleDetails(nil)
		return
	}
	module := s.systemInfoReport.Memory.Modules[index]
	s.setMemoryModuleDetails(&module)
}

func (s *state) setMemoryModuleDetails(module *systeminfo.MemoryModule) {
	if module == nil {
		for _, hwnd := range []uintptr{
			s.systemInfoControls.memoryManufacturer,
			s.systemInfoControls.memoryCapacity,
			s.systemInfoControls.memorySpeed,
			s.systemInfoControls.memoryLocator,
		} {
			_ = setWindowText(hwnd, "Nezjištěno")
		}
		return
	}
	_ = setWindowText(s.systemInfoControls.memoryManufacturer, systemInfoValue(module.Manufacturer))
	_ = setWindowText(s.systemInfoControls.memoryCapacity, systemInfoCapacity(module.CapacityBytes, false))
	speed := "Nezjištěno"
	if module.SpeedMT > 0 {
		speed = fmt.Sprintf("%d MT/s", module.SpeedMT)
	}
	_ = setWindowText(s.systemInfoControls.memorySpeed, speed)
	_ = setWindowText(s.systemInfoControls.memoryLocator, systemInfoValue(module.Locator))
}

func (s *state) populateGPUList() {
	list := s.systemInfoControls.gpuList
	clearList(list)
	if len(s.systemInfoReport.GPUs) == 0 {
		addListItem(list, "Nezjištěno")
		enableWindow(list, false)
		s.setGPUDetails(nil)
		return
	}
	for index, gpu := range s.systemInfoReport.GPUs {
		addListItem(list, fmt.Sprintf("%d. %s", index+1, systemInfoValue(gpu.Model)))
	}
	enableWindow(list, true)
	selectSingleListItem(list, 0)
	s.updateSelectedGPU()
}

func (s *state) updateSelectedGPU() {
	index := listCurrentIndex(s.systemInfoControls.gpuList)
	if index < 0 || index >= len(s.systemInfoReport.GPUs) {
		s.setGPUDetails(nil)
		return
	}
	gpu := s.systemInfoReport.GPUs[index]
	s.setGPUDetails(&gpu)
}

func (s *state) setGPUDetails(gpu *systeminfo.GPUInfo) {
	if gpu == nil {
		for _, hwnd := range []uintptr{
			s.systemInfoControls.gpuManufacturer,
			s.systemInfoControls.gpuModel,
			s.systemInfoControls.gpuVRAM,
			s.systemInfoControls.gpuSharedMemory,
		} {
			_ = setWindowText(hwnd, "Nezjištěno")
		}
		return
	}
	_ = setWindowText(s.systemInfoControls.gpuManufacturer, systemInfoValue(gpu.Manufacturer))
	_ = setWindowText(s.systemInfoControls.gpuModel, systemInfoValue(gpu.Model))
	_ = setWindowText(s.systemInfoControls.gpuVRAM, systemInfoCapacity(gpu.DedicatedVideoMemory, true))
	_ = setWindowText(s.systemInfoControls.gpuSharedMemory, systemInfoCapacity(gpu.SharedSystemMemory, true))
}

func (s *state) populateDriveList() {
	list := s.systemInfoControls.driveList
	clearList(list)
	if len(s.systemInfoReport.Drives) == 0 {
		addListItem(list, "Nezjištěno")
		enableWindow(list, false)
		s.setDriveDetails(nil)
		return
	}
	for _, drive := range s.systemInfoReport.Drives {
		mediaType := systemInfoValue(drive.MediaType)
		name := strings.TrimSpace(drive.Name)
		if name == "" {
			name = fmt.Sprintf("Fyzický disk %d", drive.DeviceNumber)
		}
		addListItem(list, mediaType+" — "+name)
	}
	enableWindow(list, true)
	selectSingleListItem(list, 0)
	s.updateSelectedDrive()
}

func (s *state) updateSelectedDrive() {
	index := listCurrentIndex(s.systemInfoControls.driveList)
	if index < 0 || index >= len(s.systemInfoReport.Drives) {
		s.setDriveDetails(nil)
		return
	}
	drive := s.systemInfoReport.Drives[index]
	s.setDriveDetails(&drive)
}

func (s *state) setDriveDetails(drive *systeminfo.DriveInfo) {
	if drive == nil {
		for _, hwnd := range []uintptr{
			s.systemInfoControls.driveType,
			s.systemInfoControls.driveName,
			s.systemInfoControls.driveNumber,
			s.systemInfoControls.driveCapacity,
			s.systemInfoControls.driveMounts,
		} {
			_ = setWindowText(hwnd, "Nezjištěno")
		}
		return
	}
	_ = setWindowText(s.systemInfoControls.driveType, systemInfoValue(drive.MediaType))
	name := strings.TrimSpace(drive.Name)
	if name == "" {
		name = fmt.Sprintf("Fyzický disk %d", drive.DeviceNumber)
	}
	_ = setWindowText(s.systemInfoControls.driveName, name)
	_ = setWindowText(s.systemInfoControls.driveNumber, fmt.Sprintf("%d", drive.DeviceNumber))
	_ = setWindowText(s.systemInfoControls.driveCapacity, systemInfoCapacity(drive.CapacityBytes, false))
	mounts := "Bez přiřazeného písmene"
	if len(drive.MountPoints) > 0 {
		mounts = strings.Join(drive.MountPoints, ", ")
	}
	_ = setWindowText(s.systemInfoControls.driveMounts, mounts)
}

func systemInfoValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Nezjištěno"
	}
	return value
}

func systemInfoCount(value uint32) string {
	if value == 0 {
		return "Nezjištěno"
	}
	return fmt.Sprintf("%d", value)
}

func systemInfoFrequency(valueMHz uint32) string {
	if valueMHz == 0 {
		return "Nezjištěno"
	}
	value := fmt.Sprintf("%.2f", float64(valueMHz)/1000)
	return strings.ReplaceAll(value, ".", ",") + " GHz"
}

func systemInfoCapacity(value uint64, zeroIsKnown bool) string {
	if value == 0 && !zeroIsKnown {
		return "Nezjištěno"
	}
	formatted := fmt.Sprintf("%.2f", float64(value)/(1024*1024*1024))
	return strings.ReplaceAll(formatted, ".", ",") + " GB"
}

func (s *state) drainSystemInfoResults() {
	for {
		select {
		case report := <-s.systemInfoCh:
			s.systemInfoLoading.Store(false)
			s.systemInfoLoaded = true
			s.updateSystemInfoControls(report)
			for _, warning := range report.Warnings {
				s.logf("Systémové informace — upozornění: %v", warning)
			}
			s.logf("Načítání systémových informací bylo dokončeno.")
		default:
			return
		}
	}
}

func (s *state) toggleLogWindow() {
	if s.logWindow == 0 {
		return
	}
	if isWindowVisible(s.logWindow) {
		showWindow(s.logWindow, false)
	} else {
		showAndUpdateWindow(s.logWindow)
		bringWindowToFront(s.logWindow)
	}
	s.updateLogButtonText()
}

func (s *state) updateLogButtonText() {
	if s.controls.toggleLog == 0 {
		return
	}
	text := "Zobrazit log"
	if s.logWindow != 0 && isWindowVisible(s.logWindow) {
		text = "Skrýt log"
	}
	_ = setWindowText(s.controls.toggleLog, text)
}

func (s *state) startOperation(name, successText string, work func(context.Context) error) bool {
	if !s.busy.CompareAndSwap(false, true) {
		s.showBusy()
		return false
	}
	s.setInstallationControlsEnabled(false)
	s.logf("Spouští se: %s", name)
	go func() {
		err := work(context.Background())
		s.resultCh <- operationResult{name: name, successText: successText, err: err}
		postMessage(s.hwnd, wmAppResult, 0, 0)
	}()
	return true
}

func (s *state) showBusy() {
	showMessage(s.hwnd, s.cfg.ProductName, "Instalace aplikace již probíhá.", mbIconWarning)
}

func (s *state) setInstallationControlsEnabled(enabled bool) {
	if len(s.apps) == 0 {
		return
	}
	enableWindow(s.controls.appList, enabled)
	enableWindow(s.controls.selectAll, enabled)
	enableWindow(s.controls.installSelected, enabled)
}

func (s *state) logf(format string, args ...any) {
	line := fmt.Sprintf("%s  %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	select {
	case s.logCh <- line:
		postMessage(s.hwnd, wmAppLog, 0, 0)
	default:
		// Pracovní gorutiny nesmí blokovat, pokud se uživatelské rozhraní zavírá nebo nestíhá zprávy.
	}
}

func (s *state) drainLogs() {
	for {
		select {
		case line := <-s.logCh:
			s.logLines = append(s.logLines, line)
			if s.logEdit != 0 {
				appendEditText(s.logEdit, line)
			}
		default:
			return
		}
	}
}

func (s *state) drainResults() {
	for {
		select {
		case result := <-s.resultCh:
			s.busy.Store(false)
			s.setInstallationControlsEnabled(true)
			if result.err != nil {
				s.logf("Selhání: %s: %v", result.name, result.err)
				showMessage(s.hwnd, result.name, result.err.Error(), mbIconError)
			} else {
				s.logf("Dokončeno: %s", result.name)
				showMessage(s.hwnd, result.name, result.successText, mbIconInformation)
			}
		default:
			return
		}
	}
}
