package win32

import (
	"strings"
	"unsafe"
)

// This file wraps the messages understood by the standard controls: the
// edit box, the list box and the tab control.

// AppendLine adds a line of text to the end of a multi-line edit box and
// scrolls it into view.
func AppendLine(edit HWND, line string) {
	if !strings.HasSuffix(line, "\r\n") {
		line += "\r\n"
	}
	SendMessage(edit, emSetSel, ^uintptr(0), ^uintptr(0)) // caret to the end
	SendMessage(edit, emReplaceSel, 0, uintptr(unsafe.Pointer(utf16Ptr(line))))
}

// ListAdd appends an item to a list box.
func ListAdd(list HWND, text string) {
	SendMessage(list, lbAddString, 0, uintptr(unsafe.Pointer(utf16Ptr(text))))
}

// ListClear removes every item from a list box.
func ListClear(list HWND) {
	SendMessage(list, lbResetContent, 0, 0)
}

// ListSelect selects one item of a single-selection list box.
func ListSelect(list HWND, index int) {
	SendMessage(list, lbSetCurSel, uintptr(index), 0)
}

// ListSelectedIndex returns the selected item of a single-selection list box,
// or -1 when nothing is selected.
func ListSelectedIndex(list HWND) int {
	return listIndex(SendMessage(list, lbGetCurSel, 0, 0))
}

// ListSetSelected selects or deselects one item of a multi-selection list box.
func ListSetSelected(list HWND, index int, selected bool) {
	SendMessage(list, lbSetSel, boolToUintptr(selected), uintptr(index))
}

// ListSelectAll selects every item of a multi-selection list box.
func ListSelectAll(list HWND) {
	SendMessage(list, lbSetSel, 1, ^uintptr(0)) // index -1 means "all items"
}

// ListSelectedIndexes returns every selected item of a multi-selection list box.
func ListSelectedIndexes(list HWND) []int {
	count := SendMessage(list, lbGetSelCount, 0, 0)
	if count == lbErr || count == 0 {
		return nil
	}
	items := make([]int32, count)
	got := SendMessage(list, lbGetSelItems, count, uintptr(unsafe.Pointer(&items[0])))
	if got == lbErr {
		return nil
	}
	indexes := make([]int, 0, got)
	for _, item := range items[:got] {
		indexes = append(indexes, int(item))
	}
	return indexes
}

// ListFocusedIndex returns the item with the focus rectangle in a
// multi-selection list box (the one the user clicked last), or -1.
func ListFocusedIndex(list HWND) int {
	return listIndex(SendMessage(list, lbGetCaretIndex, 0, 0))
}

func listIndex(result uintptr) int {
	if result == lbErr {
		return -1
	}
	return int(result)
}

// TabAdd appends a tab to a tab control.
func TabAdd(tabs HWND, index int, title string) {
	item := tcItem{Mask: tcifText, Text: utf16Ptr(title)}
	SendMessage(tabs, tcmInsertItem, uintptr(index), uintptr(unsafe.Pointer(&item)))
}

// TabSelectedIndex returns the active tab of a tab control, or -1.
func TabSelectedIndex(tabs HWND) int {
	return listIndex(SendMessage(tabs, tcmGetCurSel, 0, 0))
}
