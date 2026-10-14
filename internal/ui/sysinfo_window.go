package ui

import (
	"context"
	"time"

	"jandy.cz/winfly/internal/systeminfo"
	"jandy.cz/winfly/internal/ui/win32"
)

// Size of the inside of the system information window, in layout units.
const (
	systemInfoWindowWidth  = 980
	systemInfoWindowHeight = 400
)

// How long collecting the system information may take before giving up on
// the parts that are still missing.
const systemInfoTimeout = 25 * time.Second

// systemInfoWindow shows the hardware and Windows details of this computer on
// four tabs. The information is collected in the background the first time
// the window is opened; closing the window only hides it.
type systemInfoWindow struct {
	app  *app
	hwnd win32.HWND

	tabs   control
	pages  []*page   // one per tab, in tab order
	values []control // every value field, to show "Načítání..." in them

	overview overviewPage
	memory   memoryPage
	gpus     gpuPage
	drives   drivePage

	loaded bool
}

func newSystemInfoWindow(a *app) (*systemInfoWindow, error) {
	w := &systemInfoWindow{app: a}
	hwnd, err := a.createTopLevelWindow(win32.TopLevelWindow{
		ClassName:    systemInfoWindowClass,
		Title:        a.cfg.ProductName + " — Systémové informace",
		Style:        win32.WS_FIXEDWINDOW,
		ClientWidth:  systemInfoWindowWidth,
		ClientHeight: systemInfoWindowHeight,
		Owner:        a.main.hwnd,
	}, w.handleMessage)
	if err != nil {
		return nil, err
	}
	w.hwnd = hwnd

	if err := w.createControls(); err != nil {
		win32.DestroyWindow(hwnd)
		return nil, err
	}
	w.showLoading()
	w.showPage(0)
	return w, nil
}

// createControls puts the tab strip at the top and builds each tab's page
// in the area below it. The page builders live in sysinfo_pages.go.
func (w *systemInfoWindow) createControls() error {
	f := w.app.newForm(w.hwnd)
	tabStrip, pageArea := clientArea(systemInfoWindowWidth, systemInfoWindowHeight).splitTop(28, spacing)

	w.tabs = f.tabStrip(tabStrip, idSystemInfoTabs, "Přehled", "Paměť RAM", "Grafické adaptéry", "Disky")

	w.pages = append(w.pages, f.startPage())
	w.overview = newOverviewPage(f, pageArea)

	w.pages = append(w.pages, f.startPage())
	w.memory = newMemoryPage(f, pageArea)

	w.pages = append(w.pages, f.startPage())
	w.gpus = newGPUPage(f, pageArea)

	w.pages = append(w.pages, f.startPage())
	w.drives = newDrivePage(f, pageArea)

	w.values = f.values
	return f.err
}

func (w *systemInfoWindow) handleMessage(msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	switch msg {
	case win32.WM_NOTIFY:
		if int(wParam) == idSystemInfoTabs {
			w.showPage(win32.TabSelectedIndex(w.tabs.hwnd))
		}
		return 0, true
	case win32.WM_COMMAND:
		w.onCommand(win32.LowWord(wParam), win32.HighWord(wParam))
		return 0, true
	case win32.WM_CLOSE:
		win32.Hide(w.hwnd)
		return 0, true
	}
	return 0, false
}

func (w *systemInfoWindow) onCommand(id, notification int) {
	if id == win32.IDCANCEL { // Escape
		win32.Hide(w.hwnd)
		return
	}
	if notification != win32.LBN_SELCHANGE {
		return
	}
	switch id {
	case idMemoryModuleList:
		w.memory.modules.selectionChanged()
	case idGPUList:
		w.gpus.adapters.selectionChanged()
	case idDriveList:
		w.drives.drives.selectionChanged()
	}
}

// showPage shows the page of the given tab and hides all the others.
func (w *systemInfoWindow) showPage(index int) {
	if index < 0 || index >= len(w.pages) {
		index = 0
	}
	for i, p := range w.pages {
		p.setVisible(i == index)
	}
}

// open shows the window and, the first time, starts collecting the information.
func (w *systemInfoWindow) open() {
	win32.Show(w.hwnd)
	win32.BringToFront(w.hwnd)

	a := w.app
	if w.loaded || !a.systemInfoLoading.CompareAndSwap(false, true) {
		return // already loaded, or loading right now
	}
	w.showLoading()
	a.logf("Načítají se systémové informace.")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), systemInfoTimeout)
		defer cancel()
		a.systemInfoResults <- systeminfo.Collect(ctx)
		a.wakeUI(wmSystemInfoLoaded)
	}()
}

// showLoading puts "Načítání..." into every value and list.
func (w *systemInfoWindow) showLoading() {
	for _, value := range w.values {
		value.setText(loadingText)
	}
	w.memory.modules.showLoading()
	w.gpus.adapters.showLoading()
	w.drives.drives.showLoading()
}

// showLoadedReport runs on the UI thread when the background collection is done.
func (w *systemInfoWindow) showLoadedReport() {
	a := w.app
	for {
		select {
		case report := <-a.systemInfoResults:
			a.systemInfoLoading.Store(false)
			w.loaded = true
			w.show(report)
			for _, warning := range report.Warnings {
				a.logf("Systémové informace — upozornění: %v", warning)
			}
			a.logf("Načítání systémových informací bylo dokončeno.")
		default:
			return
		}
	}
}

func (w *systemInfoWindow) show(report systeminfo.Report) {
	w.overview.show(report)
	w.memory.show(report.Memory)
	w.gpus.adapters.setItems(report.GPUs)
	w.drives.drives.setItems(report.Drives)
}
