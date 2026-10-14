package ui

// All positions and sizes in the layout code are written in "layout units":
// pixels at 100 % display scaling (96 DPI). The form converts them to real
// pixels for the current display, so the window looks the same at 125 % or
// 150 % scaling instead of being tiny.

// Spacing shared by every window, so that all of them line up the same way.
const (
	windowPadding = 16 // between the window edge and its content
	spacing       = 12 // between neighbouring blocks (groups, button rows)
	smallSpacing  = 8  // between buttons in one row

	buttonHeight = 30
	rowHeight    = 24 // one line of a text box or label
	lineHeight   = 16 // each extra line of a multi-line text box
	rowSpacing   = 10 // between rows of a field stack
	columnGap    = 28 // between columns of a field stack
	labelGap     = 8  // between a label and its value

	// Space inside a group box: its caption takes room at the top.
	groupPaddingTop    = 26
	groupPaddingSides  = 16
	groupPaddingBottom = 16
)

// box is a rectangle in layout units.
type box struct {
	x, y, width, height int32
}

// at creates a box from its top-left corner and size.
func at(x, y, width, height int32) box {
	return box{x: x, y: y, width: width, height: height}
}

// clientArea is the whole inside of a window, minus the window padding.
func clientArea(width, height int32) box {
	return at(0, 0, width, height).inset(windowPadding, windowPadding, windowPadding, windowPadding)
}

// inset shrinks the box by the given amount on each side.
func (b box) inset(left, top, right, bottom int32) box {
	return at(b.x+left, b.y+top, b.width-left-right, b.height-top-bottom)
}

// groupInterior is the part of a group box where its content goes.
func (b box) groupInterior() box {
	return b.inset(groupPaddingSides, groupPaddingTop, groupPaddingSides, groupPaddingBottom)
}

// splitLeft cuts a column of the given width off the left side.
// It returns that column and whatever is left to the right of the gap.
func (b box) splitLeft(width, gap int32) (left, rest box) {
	left = at(b.x, b.y, width, b.height)
	rest = at(b.x+width+gap, b.y, b.width-width-gap, b.height)
	return left, rest
}

// splitRight cuts a column of the given width off the right side.
// It returns whatever is left to the left of the gap, and that column.
func (b box) splitRight(width, gap int32) (rest, right box) {
	rest = at(b.x, b.y, b.width-width-gap, b.height)
	right = at(b.x+b.width-width, b.y, width, b.height)
	return rest, right
}

// splitTop cuts a row of the given height off the top.
// It returns that row and whatever is left below the gap.
func (b box) splitTop(height, gap int32) (top, rest box) {
	top = at(b.x, b.y, b.width, height)
	rest = at(b.x, b.y+height+gap, b.width, b.height-height-gap)
	return top, rest
}

// splitBottom cuts a row of the given height off the bottom.
// It returns whatever is left above the gap, and that row.
func (b box) splitBottom(height, gap int32) (rest, bottom box) {
	rest = at(b.x, b.y, b.width, b.height-height-gap)
	bottom = at(b.x, b.y+b.height-height, b.width, height)
	return rest, bottom
}

// textAreaHeight is the height of a text box that shows the given number of lines.
func textAreaHeight(lines int32) int32 {
	return rowHeight + (lines-1)*lineHeight
}

// scaler converts layout units to physical pixels for one display DPI.
type scaler struct {
	dpi int32
}

// px converts a length from layout units to pixels, rounding to nearest.
func (s scaler) px(value int32) int32 {
	return (value*s.dpi + 48) / 96
}

// pxBox converts a whole box from layout units to pixels.
func (s scaler) pxBox(b box) box {
	return at(s.px(b.x), s.px(b.y), s.px(b.width), s.px(b.height))
}
