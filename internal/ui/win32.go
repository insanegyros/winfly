package ui

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	csHRedraw = 0x0002
	csVRedraw = 0x0001

	wsOverlapped  = 0x00000000
	wsChild       = 0x40000000
	wsVisible     = 0x10000000
	wsDisabled    = 0x08000000
	wsCaption     = 0x00C00000
	wsBorder      = 0x00800000
	wsVScroll     = 0x00200000
	wsHScroll     = 0x00100000
	wsSysMenu     = 0x00080000
	wsThickFrame  = 0x00040000
	wsMinimizeBox = 0x00020000
	wsMaximizeBox = 0x00010000
	wsTabStop     = 0x00010000

	wsExClientEdge = 0x00000200

	bsPushButton = 0x00000000
	bsGroupBox   = 0x00000007

	lbsNotify           = 0x0001
	lbsNoIntegralHeight = 0x0100
	lbsExtendedSel      = 0x0800

	iccTabClasses = 0x00000008
	tcifText      = 0x0001
	tcmFirst      = 0x1300
	tcmGetCurSel  = tcmFirst + 11
	tcmInsertItem = tcmFirst + 62

	esMultiline   = 0x0004
	esAutoVScroll = 0x0040
	esAutoHScroll = 0x0080
	esReadOnly    = 0x0800

	wmCreate          = 0x0001
	wmDestroy         = 0x0002
	wmSize            = 0x0005
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmCommand         = 0x0111
	wmSetFont         = 0x0030
	wmNotify          = 0x004E
	wmApp             = 0x8000

	bnClicked    = 0
	lbnSelChange = 1

	lbAddString     = 0x0180
	lbResetContent  = 0x0184
	lbSetSel        = 0x0185
	lbSetCurSel     = 0x0186
	lbGetCurSel     = 0x0188
	lbGetSelCount   = 0x0190
	lbGetSelItems   = 0x0191
	lbGetCaretIndex = 0x019F
	lbErr           = ^uintptr(0)

	emSetSel     = 0x00B1
	emReplaceSel = 0x00C2

	swHide = 0
	swShow = 5

	colorBtnFace   = 15
	defaultGUIFont = 17
	idcArrow       = 32512

	mbOK              = 0x00000000
	mbIconError       = 0x00000010
	mbIconWarning     = 0x00000030
	mbIconInformation = 0x00000040
	mbTaskModal       = 0x00002000
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSmall  uintptr
}

type initCommonControlsEx struct {
	Size uint32
	ICC  uint32
}

type tcItemW struct {
	Mask      uint32
	State     uint32
	StateMask uint32
	Text      *uint16
	TextMax   int32
	Image     int32
	Param     uintptr
}

type point struct {
	X int32
	Y int32
}

type message struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
	Private uint32
}

type rect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procSendMessageW        = user32.NewProc("SendMessageW")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procSetWindowTextW      = user32.NewProc("SetWindowTextW")
	procEnableWindow        = user32.NewProc("EnableWindow")
	procMessageBoxW         = user32.NewProc("MessageBoxW")
	procLoadCursorW         = user32.NewProc("LoadCursorW")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procGetClientRect       = user32.NewProc("GetClientRect")
	procMoveWindow          = user32.NewProc("MoveWindow")

	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	procGetStockObject     = gdi32.NewProc("GetStockObject")
	procInitCommonControls = comctl32.NewProc("InitCommonControlsEx")
)

func initCommonControls() error {
	value := initCommonControlsEx{
		Size: uint32(unsafe.Sizeof(initCommonControlsEx{})),
		ICC:  iccTabClasses,
	}
	r1, _, callErr := procInitCommonControls.Call(uintptr(unsafe.Pointer(&value)))
	if r1 == 0 {
		return fmt.Errorf("inicializace společných ovládacích prvků InitCommonControlsEx selhala: %w", normalizeCallError(callErr))
	}
	return nil
}

func moduleHandle() (uintptr, error) {
	r1, _, callErr := procGetModuleHandleW.Call(0)
	if r1 == 0 {
		return 0, fmt.Errorf("volání GetModuleHandleW selhalo: %w", normalizeCallError(callErr))
	}
	return r1, nil
}

func loadArrowCursor() uintptr {
	r1, _, _ := procLoadCursorW.Call(0, idcArrow)
	return r1
}

func registerWindowClass(instance uintptr, className string, wndProc uintptr) error {
	classPtr := utf16Ptr(className)
	wc := wndClassEx{
		Size:       uint32(unsafe.Sizeof(wndClassEx{})),
		Style:      csHRedraw | csVRedraw,
		WndProc:    wndProc,
		Instance:   instance,
		Cursor:     loadArrowCursor(),
		Background: colorBtnFace + 1,
		ClassName:  classPtr,
	}
	r1, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if r1 == 0 {
		return fmt.Errorf("registrace třídy okna RegisterClassExW(%s) selhala: %w", className, normalizeCallError(callErr))
	}
	return nil
}

func createWindow(exStyle uint32, className, text string, style uint32, x, y, width, height int32, parent, menu, instance uintptr) (uintptr, error) {
	classPtr := utf16Ptr(className)
	textPtr := utf16Ptr(text)
	r1, _, callErr := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(classPtr)),
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(style),
		uintptr(x),
		uintptr(y),
		uintptr(width),
		uintptr(height),
		parent,
		menu,
		instance,
		0,
	)
	if r1 == 0 {
		return 0, fmt.Errorf("vytvoření okna CreateWindowExW(%s) selhalo: %w", className, normalizeCallError(callErr))
	}
	return r1, nil
}

func defWindowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	r1, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r1
}

func showAndUpdateWindow(hwnd uintptr) {
	showWindow(hwnd, true)
	procUpdateWindow.Call(hwnd)
}

func showWindow(hwnd uintptr, visible bool) {
	command := uintptr(swHide)
	if visible {
		command = swShow
	}
	procShowWindow.Call(hwnd, command)
}

func isWindowVisible(hwnd uintptr) bool {
	r1, _, _ := procIsWindowVisible.Call(hwnd)
	return r1 != 0
}

func bringWindowToFront(hwnd uintptr) {
	procSetForegroundWindow.Call(hwnd)
}

func destroyWindow(hwnd uintptr) {
	if hwnd != 0 {
		procDestroyWindow.Call(hwnd)
	}
}

func runMessageLoop() error {
	var msg message
	for {
		r1, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		switch int32(r1) {
		case -1:
			return fmt.Errorf("načtení zprávy GetMessageW selhalo: %w", normalizeCallError(callErr))
		case 0:
			return nil
		default:
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}
}

func postQuitMessage(code int32) {
	procPostQuitMessage.Call(uintptr(code))
}

func sendMessage(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	r1, _, _ := procSendMessageW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r1
}

func postMessage(hwnd uintptr, msg uint32, wParam, lParam uintptr) bool {
	r1, _, _ := procPostMessageW.Call(hwnd, uintptr(msg), wParam, lParam)
	return r1 != 0
}

func setWindowText(hwnd uintptr, text string) error {
	textPtr := utf16Ptr(text)
	r1, _, callErr := procSetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(textPtr)))
	if r1 == 0 {
		return fmt.Errorf("nastavení textu okna SetWindowTextW selhalo: %w", normalizeCallError(callErr))
	}
	return nil
}

func enableWindow(hwnd uintptr, enabled bool) {
	value := uintptr(0)
	if enabled {
		value = 1
	}
	procEnableWindow.Call(hwnd, value)
}

func showMessage(parent uintptr, title, text string, flags uint32) {
	titlePtr := utf16Ptr(title)
	textPtr := utf16Ptr(text)
	procMessageBoxW.Call(
		parent,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(flags|mbOK|mbTaskModal),
	)
}

func stockGUIFont() uintptr {
	r1, _, _ := procGetStockObject.Call(defaultGUIFont)
	return r1
}

func applyFont(hwnd, font uintptr) {
	sendMessage(hwnd, wmSetFont, font, 1)
}

func appendEditText(hwnd uintptr, text string) {
	if !strings.HasSuffix(text, "\r\n") {
		text += "\r\n"
	}
	sendMessage(hwnd, emSetSel, ^uintptr(0), ^uintptr(0))
	ptr := utf16Ptr(text)
	sendMessage(hwnd, emReplaceSel, 0, uintptr(unsafe.Pointer(ptr)))
}

func addListItem(hwnd uintptr, text string) {
	ptr := utf16Ptr(text)
	sendMessage(hwnd, lbAddString, 0, uintptr(unsafe.Pointer(ptr)))
}

func clearList(hwnd uintptr) {
	sendMessage(hwnd, lbResetContent, 0, 0)
}

func selectSingleListItem(hwnd uintptr, index int) {
	sendMessage(hwnd, lbSetCurSel, uintptr(index), 0)
}

func listCurrentIndex(hwnd uintptr) int {
	value := sendMessage(hwnd, lbGetCurSel, 0, 0)
	if value == lbErr {
		return -1
	}
	return int(value)
}

func listSelections(hwnd uintptr) []int {
	countRaw := sendMessage(hwnd, lbGetSelCount, 0, 0)
	if countRaw == lbErr || countRaw == 0 {
		return nil
	}
	count := int(countRaw)
	items := make([]int32, count)
	gotRaw := sendMessage(hwnd, lbGetSelItems, uintptr(count), uintptr(unsafe.Pointer(&items[0])))
	if gotRaw == lbErr || gotRaw == 0 {
		return nil
	}
	got := int(gotRaw)
	result := make([]int, 0, got)
	for _, item := range items[:got] {
		result = append(result, int(item))
	}
	return result
}

func listCaretIndex(hwnd uintptr) int {
	value := sendMessage(hwnd, lbGetCaretIndex, 0, 0)
	if value == lbErr {
		return -1
	}
	return int(value)
}

func selectListItem(hwnd uintptr, index int, selected bool) {
	value := uintptr(0)
	if selected {
		value = 1
	}
	sendMessage(hwnd, lbSetSel, value, uintptr(index))
}

func selectAllListItems(hwnd uintptr) {
	sendMessage(hwnd, lbSetSel, 1, ^uintptr(0))
}

func insertTabItem(hwnd uintptr, index int, text string) error {
	item := tcItemW{
		Mask: tcifText,
		Text: utf16Ptr(text),
	}
	result := sendMessage(hwnd, tcmInsertItem, uintptr(index), uintptr(unsafe.Pointer(&item)))
	if result == ^uintptr(0) {
		return fmt.Errorf("vložení karty %q selhalo", text)
	}
	return nil
}

func tabCurrentIndex(hwnd uintptr) int {
	value := sendMessage(hwnd, tcmGetCurSel, 0, 0)
	if value == ^uintptr(0) {
		return -1
	}
	return int(value)
}

func getClientRect(hwnd uintptr) (rect, error) {
	var value rect
	r1, _, callErr := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&value)))
	if r1 == 0 {
		return rect{}, fmt.Errorf("zjištění rozměrů okna GetClientRect selhalo: %w", normalizeCallError(callErr))
	}
	return value, nil
}

func moveWindow(hwnd uintptr, x, y, width, height int32, repaint bool) {
	repaintValue := uintptr(0)
	if repaint {
		repaintValue = 1
	}
	procMoveWindow.Call(hwnd, uintptr(x), uintptr(y), uintptr(width), uintptr(height), repaintValue)
}

func lowWord(value uintptr) uint16 {
	return uint16(value & 0xffff)
}

func highWord(value uintptr) uint16 {
	return uint16((value >> 16) & 0xffff)
}

func utf16Ptr(value string) *uint16 {
	value = strings.ReplaceAll(value, "\x00", "\uFFFD")
	ptr, _ := syscall.UTF16PtrFromString(value)
	return ptr
}

func normalizeCallError(err error) error {
	if err == nil {
		return syscall.EINVAL
	}
	if errno, ok := err.(syscall.Errno); ok && errno == 0 {
		return syscall.EINVAL
	}
	return err
}
