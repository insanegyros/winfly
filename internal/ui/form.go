package ui

import "jandy.cz/ess-provisioner/internal/ui/win32"

// form creates the controls of one window. It is the "partials" layer of the
// UI: every method creates one kind of control with the right styles, the
// right font and DPI-scaled coordinates, so the window code only says what
// goes where.
//
// Creating a control can fail. Instead of checking an error after every
// single control, the form remembers the first error and ignores all later
// calls; the window code checks form.err once at the end.
type form struct {
	parent win32.HWND
	font   win32.Font
	scale  scaler

	// page, when set, also receives every control created, so the whole tab
	// can be shown or hidden at once. See startPage.
	page *page
	// values collects every read-only value field, so they can all be reset
	// to a placeholder text at once.
	values []control

	err error
}

func newForm(parent win32.HWND, font win32.Font, scale scaler) *form {
	return &form{parent: parent, font: font, scale: scale}
}

// startPage starts collecting the controls created from now on into a new page.
func (f *form) startPage() *page {
	f.page = &page{}
	return f.page
}

// create is the single place where child controls are made.
func (f *form) create(className, text string, style, exStyle uint32, b box, id int) control {
	if f.err != nil {
		return control{}
	}
	b = f.scale.pxBox(b)
	hwnd, err := win32.CreateWindow(win32.Window{
		ClassName: className,
		Text:      text,
		Style:     win32.WS_CHILD | win32.WS_VISIBLE | style,
		ExStyle:   exStyle,
		X:         b.x,
		Y:         b.y,
		Width:     b.width,
		Height:    b.height,
		Parent:    f.parent,
		ID:        id,
	})
	if err != nil {
		f.err = err
		return control{}
	}
	win32.SetFont(hwnd, f.font)

	c := control{hwnd: hwnd}
	if f.page != nil {
		f.page.controls = append(f.page.controls, c)
	}
	return c
}

// groupBox draws a captioned frame and returns the box inside it, where the
// grouped controls should go.
func (f *form) groupBox(title string, b box) (interior box) {
	f.create(win32.ClassButton, title, win32.BS_GROUPBOX, 0, b, 0)
	return b.groupInterior()
}

// label is fixed text, vertically centred in its box so it lines up with the
// text box next to it.
func (f *form) label(text string, b box) control {
	return f.create(win32.ClassStatic, text, win32.SS_LEFT|win32.SS_CENTERIMAGE|win32.SS_NOPREFIX, 0, b, 0)
}

// button is a push button. Clicking it sends WM_COMMAND with the given id.
func (f *form) button(text string, b box, id int) control {
	return f.create(win32.ClassButton, text, win32.BS_PUSHBUTTON|win32.WS_TABSTOP, 0, b, id)
}

// readOnlyField is a one-line text box the user can select and copy from,
// but not edit.
func (f *form) readOnlyField(b box) control {
	style := uint32(win32.ES_READONLY | win32.ES_AUTOHSCROLL)
	return f.value(f.create(win32.ClassEdit, "", style, win32.WS_EX_CLIENTEDGE, b, 0))
}

// readOnlyTextArea is a multi-line, word-wrapping, read-only text box.
// Long text can always be scrolled with the mouse wheel; withScrollBar also
// shows a scroll bar, which only makes sense when the box is tall enough.
func (f *form) readOnlyTextArea(b box, withScrollBar bool) control {
	style := uint32(win32.ES_READONLY | win32.ES_MULTILINE | win32.ES_AUTOVSCROLL)
	if withScrollBar {
		style |= win32.WS_VSCROLL
	}
	return f.value(f.create(win32.ClassEdit, "", style, win32.WS_EX_CLIENTEDGE, b, 0))
}

// valueText is a value shown as plain text, without a text box around it.
func (f *form) valueText(b box) control {
	return f.value(f.label("", b))
}

func (f *form) value(c control) control {
	if c.hwnd != 0 {
		f.values = append(f.values, c)
	}
	return c
}

// logView is a scrollable, read-only text box for long, growing text.
func (f *form) logView(initialText string, b box) control {
	style := uint32(win32.ES_READONLY | win32.ES_MULTILINE | win32.ES_AUTOVSCROLL | win32.ES_AUTOHSCROLL |
		win32.WS_VSCROLL | win32.WS_HSCROLL)
	return f.create(win32.ClassEdit, initialText, style, win32.WS_EX_CLIENTEDGE, b, 0)
}

// listBox is a list where one item can be selected. Changing the selection
// sends WM_COMMAND with the given id and LBN_SELCHANGE.
func (f *form) listBox(b box, id int) listBox {
	return listBox{f.create(win32.ClassListBox, "", listBoxStyle, win32.WS_EX_CLIENTEDGE, b, id)}
}

// multiSelectListBox is a list where several items can be selected with
// Ctrl and Shift, like files in Explorer.
func (f *form) multiSelectListBox(b box, id int) listBox {
	return listBox{f.create(win32.ClassListBox, "", listBoxStyle|win32.LBS_EXTENDEDSEL, win32.WS_EX_CLIENTEDGE, b, id)}
}

const listBoxStyle = win32.WS_VSCROLL | win32.WS_TABSTOP | win32.LBS_NOTIFY | win32.LBS_NOINTEGRALHEIGHT

// tabStrip is a row of tabs. Only the tab headers are drawn; the content of
// each tab is a page placed below the strip. Switching tabs sends WM_NOTIFY
// with the given id.
func (f *form) tabStrip(b box, id int, titles ...string) control {
	tabs := f.create(win32.ClassTabControl, "", win32.WS_TABSTOP|win32.WS_CLIPSIBLINGS, 0, b, id)
	if tabs.hwnd != 0 {
		for index, title := range titles {
			win32.TabAdd(tabs.hwnd, index, title)
		}
	}
	return tabs
}
