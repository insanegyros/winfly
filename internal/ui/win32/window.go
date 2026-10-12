package win32

import (
	"fmt"
	"syscall"
	"unsafe"
)

// MessageHandler receives the messages of one window. It returns handled=false
// for every message it does not care about, and the default Windows behaviour
// is applied instead.
type MessageHandler func(msg uint32, wParam, lParam uintptr) (result uintptr, handled bool)

// handlers maps each of our top-level windows to its Go message handler.
//
// Win32 calls one window procedure per window class and has no notion of Go
// closures, so every class registered by this package shares sharedWindowProc,
// which looks the window up here. All access happens on the UI thread.
var handlers = map[HWND]MessageHandler{}

var sharedWindowProcPtr = syscall.NewCallback(sharedWindowProc)

func sharedWindowProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	handler := handlers[hwnd]
	if msg == WM_NCDESTROY {
		delete(handlers, hwnd)
	}
	if handler != nil {
		if result, handled := handler(msg, wParam, lParam); handled {
			return result
		}
	}
	return DefWindowProc(hwnd, msg, wParam, lParam)
}

// DefWindowProc applies the default Windows behaviour to a message.
func DefWindowProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	result, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return result
}

// RegisterWindowClass registers a top-level window class whose messages are
// routed to the handler set by SetMessageHandler.
func RegisterWindowClass(className string) error {
	class := wndClassEx{
		Size:       uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:    sharedWindowProcPtr,
		Instance:   ModuleHandle(),
		Cursor:     arrowCursor(),
		Background: colorBtnFace + 1,
		ClassName:  utf16Ptr(className),
	}
	ok, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
	if ok == 0 {
		return fmt.Errorf("registrace třídy okna RegisterClassExW(%s) selhala: %w", className, callError(err))
	}
	return nil
}

// SetMessageHandler connects a window created from a class registered with
// RegisterWindowClass to the Go function that handles its messages.
func SetMessageHandler(hwnd HWND, handler MessageHandler) {
	handlers[hwnd] = handler
}

// TopLevelWindow describes a window with a title bar.
type TopLevelWindow struct {
	ClassName string
	Title     string
	Style     uint32
	// ClientWidth and ClientHeight are the size of the area inside the frame,
	// in physical pixels. The frame is added automatically.
	ClientWidth  int32
	ClientHeight int32
	// Owner is the window this one belongs to. Owned windows stay above their
	// owner and are destroyed with it. Zero for the main window.
	Owner HWND
}

// CreateTopLevelWindow creates a hidden top-level window at the default position.
func CreateTopLevelWindow(spec TopLevelWindow) (HWND, error) {
	frame := Rect{Right: spec.ClientWidth, Bottom: spec.ClientHeight}
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&frame)), uintptr(spec.Style), 0, 0)

	return CreateWindow(Window{
		ClassName: spec.ClassName,
		Text:      spec.Title,
		Style:     spec.Style,
		X:         cwUseDefault,
		Y:         cwUseDefault,
		Width:     frame.Width(),
		Height:    frame.Height(),
		Parent:    spec.Owner,
	})
}

// Window holds every argument of CreateWindowExW.
type Window struct {
	ClassName string
	Text      string
	Style     uint32
	ExStyle   uint32
	X, Y      int32
	Width     int32
	Height    int32
	Parent    HWND
	// ID identifies a child control in WM_COMMAND and WM_NOTIFY messages.
	ID int
}

// CreateWindow creates a window or control.
func CreateWindow(w Window) (HWND, error) {
	hwnd, _, err := procCreateWindowExW.Call(
		uintptr(w.ExStyle),
		uintptr(unsafe.Pointer(utf16Ptr(w.ClassName))),
		uintptr(unsafe.Pointer(utf16Ptr(w.Text))),
		uintptr(w.Style),
		uintptr(w.X),
		uintptr(w.Y),
		uintptr(w.Width),
		uintptr(w.Height),
		uintptr(w.Parent),
		uintptr(w.ID),
		ModuleHandle(),
		0,
	)
	if hwnd == 0 {
		return 0, fmt.Errorf("vytvoření okna CreateWindowExW(%s) selhalo: %w", w.ClassName, callError(err))
	}
	return HWND(hwnd), nil
}

// DestroyWindow closes a window immediately, without asking it first.
func DestroyWindow(hwnd HWND) {
	if hwnd != 0 {
		procDestroyWindow.Call(uintptr(hwnd))
	}
}

// Show makes a window visible and repaints it.
func Show(hwnd HWND) {
	procShowWindow.Call(uintptr(hwnd), swShow)
	procUpdateWindow.Call(uintptr(hwnd))
}

// Hide makes a window invisible without destroying it.
func Hide(hwnd HWND) {
	procShowWindow.Call(uintptr(hwnd), swHide)
}

// SetVisible shows or hides a window.
func SetVisible(hwnd HWND, visible bool) {
	if visible {
		procShowWindow.Call(uintptr(hwnd), swShow)
	} else {
		Hide(hwnd)
	}
}

// IsVisible reports whether a window is currently shown.
func IsVisible(hwnd HWND) bool {
	visible, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
	return visible != 0
}

// BringToFront activates a window and moves it above all others.
func BringToFront(hwnd HWND) {
	procSetForegroundWindow.Call(uintptr(hwnd))
}

// SetEnabled enables or disables (greys out) a window or control.
func SetEnabled(hwnd HWND, enabled bool) {
	procEnableWindow.Call(uintptr(hwnd), boolToUintptr(enabled))
}

// SetText changes a window's title or a control's text.
func SetText(hwnd HWND, text string) {
	procSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(utf16Ptr(text))))
}

// ClientRect returns the area inside a window's frame. Left and Top are always 0.
func ClientRect(hwnd HWND) Rect {
	var client Rect
	procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&client)))
	return client
}

// Move changes a window's position and size and repaints it.
func Move(hwnd HWND, x, y, width, height int32) {
	procMoveWindow.Call(uintptr(hwnd), uintptr(x), uintptr(y), uintptr(width), uintptr(height), 1)
}

// SetFocus moves the keyboard focus to a window or control.
func SetFocus(hwnd HWND) {
	procSetFocus.Call(uintptr(hwnd))
}

// SetFont changes the font a control draws its text with.
func SetFont(hwnd HWND, font Font) {
	SendMessage(hwnd, WM_SETFONT, uintptr(font), 1)
}

func rootWindow(hwnd HWND) HWND {
	root, _, _ := procGetAncestor.Call(uintptr(hwnd), gaRoot)
	return HWND(root)
}

func arrowCursor() uintptr {
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	return cursor
}
