package ui

// detailList connects a list box with a details panel: the list shows one
// line per item, and selecting a line shows that item's details.
//
// The system information window uses it for memory modules, graphics
// adapters and drives, which all work the same way.
type detailList[T any] struct {
	list  listBox
	items []T

	// describe returns the text of the list line for an item.
	describe func(index int, item T) string
	// showDetails fills the details panel; nil means "nothing to show".
	showDetails func(item *T)
}

// setItems replaces the items and selects the first one. Without any items
// the list is disabled and says so.
func (d *detailList[T]) setItems(items []T) {
	d.items = items
	if len(items) == 0 {
		d.list.showPlaceholder(notDetected)
		d.showDetails(nil)
		return
	}

	d.list.clear()
	for index, item := range items {
		d.list.add(d.describe(index, item))
	}
	d.list.setEnabled(true)
	d.list.selectOnly(0)
	d.selectionChanged()
}

// showLoading disables the list until the items are known.
func (d *detailList[T]) showLoading() {
	d.list.showPlaceholder(loadingText)
}

// selectionChanged shows the details of the newly selected item.
func (d *detailList[T]) selectionChanged() {
	index := d.list.selectedIndex()
	if index < 0 || index >= len(d.items) {
		d.showDetails(nil)
		return
	}
	d.showDetails(&d.items[index])
}
