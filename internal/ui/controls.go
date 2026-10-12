package ui

import "jandy.cz/ess-provisioner/internal/ui/win32"

// control is any child window on a form: a label, button, text box...
// The zero value is "no control" and every method on it does nothing, so a
// form that failed half-way can still be cleaned up safely.
type control struct {
	hwnd win32.HWND
}

func (c control) setText(text string) {
	if c.hwnd != 0 {
		win32.SetText(c.hwnd, text)
	}
}

func (c control) setEnabled(enabled bool) {
	if c.hwnd != 0 {
		win32.SetEnabled(c.hwnd, enabled)
	}
}

func (c control) setVisible(visible bool) {
	if c.hwnd != 0 {
		win32.SetVisible(c.hwnd, visible)
	}
}

// listBox is a list of text items, with either single or multiple selection.
type listBox struct {
	control
}

func (l listBox) clear()                 { win32.ListClear(l.hwnd) }
func (l listBox) add(text string)        { win32.ListAdd(l.hwnd, text) }
func (l listBox) selectOnly(index int)   { win32.ListSelect(l.hwnd, index) }
func (l listBox) selectedIndex() int     { return win32.ListSelectedIndex(l.hwnd) }
func (l listBox) selectAll()             { win32.ListSelectAll(l.hwnd) }
func (l listBox) selectedIndexes() []int { return win32.ListSelectedIndexes(l.hwnd) }
func (l listBox) focusedIndex() int      { return win32.ListFocusedIndex(l.hwnd) }
func (l listBox) setSelected(index int, selected bool) {
	win32.ListSetSelected(l.hwnd, index, selected)
}

// setItems replaces the content of the list.
func (l listBox) setItems(items []string) {
	l.clear()
	for _, item := range items {
		l.add(item)
	}
}

// showPlaceholder replaces the content with one greyed-out line of text,
// such as "Loading..." or "Not found".
func (l listBox) showPlaceholder(text string) {
	l.setItems([]string{text})
	l.setEnabled(false)
}

// page is a group of controls that are shown and hidden together, such as
// the content of one tab.
type page struct {
	controls []control
}

func (p *page) setVisible(visible bool) {
	for _, c := range p.controls {
		c.setVisible(visible)
	}
}
