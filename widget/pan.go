package widget

// Horizontal panning for the text widgets, the GTK entry scroll model:
// when the text is wider than the field, painting, the caret, and
// hit-testing shift by a scroll offset instead of letting the caret
// walk off the edge. Entry keeps one offset for its display line;
// TextArea keeps one per logical line for the unwrapped case. The
// offset is pure view state — never an undo entry, never layout.
//
// caretPad is the right margin the pan keeps past the caret: scrolling
// stops with this much of the following text still on show.
const caretPad = 4

// dragPanStep is how far one drag motion past the field's edge pans,
// so a selection drag can walk through text outside the viewport.
const dragPanStep = 8

// edgePan returns the pan for a drag motion at x: one step outward
// when the pointer is past the inner left or right edge, unchanged
// inside them. Callers clamp the result to the scrollable range.
func edgePan(x, left, right, pan int) int {
	switch {
	case x >= right:
		return pan + dragPanStep
	case x < left:
		return pan - dragPanStep
	}
	return pan
}
