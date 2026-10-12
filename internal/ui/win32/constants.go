package win32

// Window styles (WS_*).
const (
	WS_OVERLAPPED   = 0x00000000
	WS_CHILD        = 0x40000000
	WS_VISIBLE      = 0x10000000
	WS_CLIPSIBLINGS = 0x04000000
	WS_CAPTION      = 0x00C00000
	WS_VSCROLL      = 0x00200000
	WS_HSCROLL      = 0x00100000
	WS_SYSMENU      = 0x00080000
	WS_THICKFRAME   = 0x00040000
	WS_MINIMIZEBOX  = 0x00020000
	WS_MAXIMIZEBOX  = 0x00010000
	WS_TABSTOP      = 0x00010000

	// WS_FIXEDWINDOW is a normal window that the user cannot resize or maximize.
	WS_FIXEDWINDOW = WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_MINIMIZEBOX
	// WS_RESIZABLEWINDOW is a normal window with a sizing border and maximize button.
	WS_RESIZABLEWINDOW = WS_FIXEDWINDOW | WS_THICKFRAME | WS_MAXIMIZEBOX
)

// Extended window styles (WS_EX_*).
const (
	WS_EX_CLIENTEDGE = 0x00000200 // sunken 3D border used by text boxes and lists
)

// Control-specific styles.
const (
	BS_PUSHBUTTON = 0x00000000
	BS_GROUPBOX   = 0x00000007

	SS_LEFT        = 0x00000000
	SS_CENTERIMAGE = 0x00000200 // vertically centres single-line text
	SS_NOPREFIX    = 0x00000080 // shows "&" literally instead of as a mnemonic

	ES_MULTILINE   = 0x0004
	ES_AUTOVSCROLL = 0x0040
	ES_AUTOHSCROLL = 0x0080
	ES_READONLY    = 0x0800

	LBS_NOTIFY           = 0x0001
	LBS_NOINTEGRALHEIGHT = 0x0100
	LBS_EXTENDEDSEL      = 0x0800
)

// Window messages (WM_*).
const (
	WM_DESTROY         = 0x0002
	WM_SIZE            = 0x0005
	WM_CLOSE           = 0x0010
	WM_QUERYENDSESSION = 0x0011
	WM_SETFONT         = 0x0030
	WM_NOTIFY          = 0x004E
	WM_NCDESTROY       = 0x0082
	WM_COMMAND         = 0x0111

	// WM_APP is the first message number free for application use.
	WM_APP = 0x8000
)

// Notification codes delivered through WM_COMMAND and WM_NOTIFY.
const (
	BN_CLICKED    = 0
	LBN_SELCHANGE = 1

	// IDCANCEL is sent as a WM_COMMAND when the user presses Escape.
	IDCANCEL = 2
)

// Common control registration flags (ICC_*).
const (
	ICC_STANDARD_CLASSES = 0x00004000
	ICC_TAB_CLASSES      = 0x00000008
)

// Message box flags (MB_*).
const (
	MB_OK              = 0x00000000
	MB_ICONERROR       = 0x00000010
	MB_ICONWARNING     = 0x00000030
	MB_ICONINFORMATION = 0x00000040
	MB_TASKMODAL       = 0x00002000
)

// Standard window class names of the built-in controls.
const (
	ClassButton     = "BUTTON"
	ClassStatic     = "STATIC"
	ClassEdit       = "EDIT"
	ClassListBox    = "LISTBOX"
	ClassTabControl = "SysTabControl32"
)

// Message constants used internally by this package.
const (
	cwUseDefault   = -0x80000000
	colorBtnFace   = 15
	idcArrow       = 32512
	gaRoot         = 2
	swHide         = 0
	swShow         = 5
	defaultDPI     = 96
	fwNormal       = 400
	defaultCharset = 1
	clearTypeQual  = 5

	emSetSel     = 0x00B1
	emReplaceSel = 0x00C2

	lbAddString     = 0x0180
	lbResetContent  = 0x0184
	lbSetSel        = 0x0185
	lbSetCurSel     = 0x0186
	lbGetCurSel     = 0x0188
	lbGetSelCount   = 0x0190
	lbGetSelItems   = 0x0191
	lbGetCaretIndex = 0x019F
	lbErr           = ^uintptr(0)

	tcmFirst      = 0x1300
	tcmGetCurSel  = tcmFirst + 11
	tcmInsertItem = tcmFirst + 62
	tcifText      = 0x0001
)
