package ui

// This file holds the bigger building blocks ("partials") that the windows
// are assembled from: labelled rows of values, captioned sections and the
// "list on the left, details on the right" layout.

// fieldStack places "Label:  [value]" rows below each other. Every row call
// creates the label and the value control, and moves down to the next row.
type fieldStack struct {
	form       *form
	area       box   // where the rows go; area.y is the top of the next row
	labelWidth int32 // the same for every row, so the values line up
}

// stack starts a field stack in the given area.
func (f *form) stack(area box, labelWidth int32) *fieldStack {
	return &fieldStack{form: f, area: area, labelWidth: labelWidth}
}

// section draws a captioned group box and returns a field stack for its inside.
func (f *form) section(title string, b box, labelWidth int32) *fieldStack {
	return f.stack(f.groupBox(title, b), labelWidth)
}

// field adds a row with a one-line read-only text box.
func (s *fieldStack) field(label string) control {
	valueBox := s.row(label, rowHeight)
	return s.form.readOnlyField(valueBox)
}

// textArea adds a row with a read-only text box tall enough for the given
// number of lines. Boxes of more than three lines get a scroll bar.
func (s *fieldStack) textArea(label string, lines int32) control {
	valueBox := s.row(label, textAreaHeight(lines))
	return s.form.readOnlyTextArea(valueBox, lines > 3)
}

// text adds a row whose value is plain text without a text box around it.
func (s *fieldStack) text(label string) control {
	valueBox := s.row(label, rowHeight)
	return s.form.valueText(valueBox)
}

// row creates the label of a new row and returns the box for its value.
func (s *fieldStack) row(label string, height int32) (valueBox box) {
	rowBox, rest := s.area.splitTop(height, rowSpacing)
	labelBox, valueBox := rowBox.splitLeft(s.labelWidth, labelGap)
	labelBox.height = rowHeight // the label lines up with the first line of the value

	s.form.label(label, labelBox)
	s.area = rest
	return valueBox
}

// columns splits the rest of the stack into side-by-side stacks of equal
// width. Use them for short values that would waste space at full width.
func (s *fieldStack) columns(count int32) []*fieldStack {
	width := (s.area.width - (count-1)*columnGap) / count
	stacks := make([]*fieldStack, count)
	rest := s.area
	for i := range stacks {
		var column box
		column, rest = rest.splitLeft(width, columnGap)
		stacks[i] = s.form.stack(column, s.labelWidth)
	}
	return stacks
}

// listWithDetails draws two groups side by side: a list on the left and a
// field stack for the details of the selected item on the right.
func (f *form) listWithDetails(listTitle, detailsTitle string, b box, listWidth int32, listID int, labelWidth int32) (listBox, *fieldStack) {
	listGroup, detailsGroup := b.splitLeft(listWidth, spacing)
	list := f.listBox(f.groupBox(listTitle, listGroup), listID)
	details := f.section(detailsTitle, detailsGroup, labelWidth)
	return list, details
}
