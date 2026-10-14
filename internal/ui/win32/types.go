package win32

// HWND is a handle to a window. In Win32 every control (button, list, text
// box...) is also a window, so this is the handle type for those too.
type HWND uintptr

// Rect is a Win32 RECT: the edges of a rectangle in pixels.
type Rect struct {
	Left, Top, Right, Bottom int32
}

// Width returns the horizontal size of the rectangle.
func (r Rect) Width() int32 { return r.Right - r.Left }

// Height returns the vertical size of the rectangle.
func (r Rect) Height() int32 { return r.Bottom - r.Top }

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

type tcItem struct {
	Mask      uint32
	State     uint32
	StateMask uint32
	Text      *uint16
	TextMax   int32
	Image     int32
	Param     uintptr
}

type point struct {
	X, Y int32
}

type msg struct {
	Hwnd    HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   point
	Private uint32
}
