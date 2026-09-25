package widget

import (
	"unicode"

	"github.com/stubbedev/gelm/render"
)

// Entry is a single-line text field. Its editing state machine (insert,
// delete, cursor motion) is input-agnostic: keyboard input in M4 calls
// these methods.
type Entry struct {
	node
	face        *render.Typeface
	sizePx      float64
	color       render.Color
	placeholder string

	// OnChanged fires after the contents change, whatever the
	// source: typing, editing keys, clipboard, or SetText.
	OnChanged func(string)

	runes  []rune
	cursor int
	anchor int // selection anchor; equals cursor when nothing is selected
}

// NewEntry returns an empty entry painted with face at sizePx.
func NewEntry(face *render.Typeface, sizePx float64, color render.Color) *Entry {
	return &Entry{face: face, sizePx: sizePx, color: color}
}

// SetPlaceholder sets the text shown when the entry is empty.
func (e *Entry) SetPlaceholder(s string) {
	e.placeholder = s
}

// Text returns the entry contents.
// CursorName reports the text caret shape while hovered.
func (e *Entry) CursorName() string { return "xterm" }

func (e *Entry) Text() string {
	return string(e.runes)
}

// changed fires OnChanged after a content change.
func (e *Entry) changed() {
	if e.OnChanged != nil {
		e.OnChanged(e.Text())
	}
}

// SelectedText implements SelectedTexter.
func (e *Entry) SelectedText() (string, bool) {
	start, end, active := e.Selection()
	if !active {
		return "", false
	}
	return string(e.runes[start:end]), true
}

// SetText replaces the contents and moves the cursor to the end.
func (e *Entry) SetText(s string) {
	if s == e.Text() {
		return
	}
	e.runes = []rune(s)
	e.cursor = len(e.runes)
	e.anchor = e.cursor
	e.changed()
}

// Cursor returns the cursor position as a rune index.
func (e *Entry) Cursor() int {
	return e.cursor
}

// Selection returns the selected rune range and whether a non-empty
// selection exists.
func (e *Entry) Selection() (start, end int, active bool) {
	start, end = e.cursor, e.anchor
	if start > end {
		start, end = end, start
	}
	return start, end, start != end
}

// collapse drops any selection: the selected runes are removed and the
// cursor rests at their start. Without a selection it only resets the
// anchor.
func (e *Entry) collapse() {
	start, end, active := e.Selection()
	if active {
		e.runes = append(e.runes[:start], e.runes[end:]...)
	}
	e.anchor = start
	e.cursor = start
}

// Insert inserts s at the cursor. An active selection is replaced.
func (e *Entry) Insert(s string) {
	e.collapse()
	r := []rune(s)
	e.runes = append(e.runes[:e.cursor], append(append([]rune{}, r...), e.runes[e.cursor:]...)...)
	e.cursor += len(r)
	e.anchor = e.cursor
	e.changed()
}

// Backspace deletes the selection, or the rune before the cursor when
// nothing is selected.
func (e *Entry) Backspace() {
	if _, _, active := e.Selection(); active {
		e.collapse()
		e.changed()
		return
	}
	if e.cursor == 0 {
		return
	}
	e.runes = append(e.runes[:e.cursor-1], e.runes[e.cursor:]...)
	e.cursor--
	e.anchor = e.cursor
	e.changed()
}

// Delete deletes the selection, or the rune at the cursor when nothing
// is selected.
func (e *Entry) Delete() {
	if _, _, active := e.Selection(); active {
		e.collapse()
		e.changed()
		return
	}
	if e.cursor >= len(e.runes) {
		return
	}
	e.runes = append(e.runes[:e.cursor], e.runes[e.cursor+1:]...)
	e.changed()
}

// MoveCursor moves the cursor by delta runes, clamped to [0, len]. An
// active selection collapses to the edge the motion points at first,
// without moving further - the standard first-press behavior.
func (e *Entry) MoveCursor(delta int) {
	if _, _, active := e.Selection(); active {
		start, end, _ := e.Selection()
		edge := start
		if delta > 0 {
			edge = end
		}
		e.cursor, e.anchor = edge, edge
		return
	}
	e.cursor += delta
	e.clampCursor()
	e.anchor = e.cursor
}

// MoveCursorExtending moves the cursor by delta runes, growing or
// shrinking the selection from its anchor (shift+arrow behavior).
func (e *Entry) MoveCursorExtending(delta int) {
	e.cursor += delta
	e.clampCursor()
}

func (e *Entry) clampCursor() {
	if e.cursor < 0 {
		e.cursor = 0
	}
	if e.cursor > len(e.runes) {
		e.cursor = len(e.runes)
	}
}

// MoveHome puts the cursor at the start, dropping any selection.
func (e *Entry) MoveHome() { e.cursor, e.anchor = 0, 0 }

// MoveEnd puts the cursor after the last rune, dropping any selection.
func (e *Entry) MoveEnd() {
	e.cursor, e.anchor = len(e.runes), len(e.runes)
}

// Measure wants the text advance (or the placeholder's) plus padding; an
// empty field keeps its padding so the box stays visible. Clamped to con.
func (e *Entry) Measure(con Constraints) Size {
	text := e.Text()
	if text == "" {
		text = e.placeholder
	}
	w := 16
	if text != "" {
		w += int(e.face.Shape(text, e.sizePx).Advance() + 0.5)
	}
	h := int(e.face.Shape("lg", e.sizePx).Ascent()+e.face.Shape("lg", e.sizePx).Descent()+0.5) + 12
	return clampSize(Size{W: w, H: h}, con)
}

// Paint draws the field: placeholder when empty, text otherwise, the
// selection highlight, and the cursor bar. Zero color fields fall back
// to the theme.
func (e *Entry) Paint(cv *render.Canvas) {
	t := Current()
	cv.RoundedRect(e.bounds, t.Radius, t.Surface)
	if len(e.runes) == 0 && e.placeholder != "" {
		e.face.DrawAligned(cv, e.placeholder, e.bounds, e.sizePx, t.Border, render.AlignStart)
		return
	}
	if start, end, active := e.Selection(); active {
		x0 := e.bounds.X + 8 + int(e.face.Shape(e.Text(), e.sizePx).CaretX(start)+0.5)
		x1 := e.bounds.X + 8 + int(e.face.Shape(e.Text(), e.sizePx).CaretX(end)+0.5)
		a := t.Accent
		cv.FillRect(render.Rect{X: x0, Y: e.bounds.Y + 4, W: x1 - x0, H: e.bounds.H - 8},
			render.RGBA(a.R(), a.G(), a.B(), 90))
	}
	e.face.DrawAligned(cv, e.Text(), e.bounds, e.sizePx, e.color, render.AlignStart)

	// Cursor bar after the text before the cursor.
	x := e.bounds.X + 8 + int(e.face.Shape(e.Text(), e.sizePx).CaretX(e.cursor)+0.5)
	cv.FillRect(render.Rect{X: x, Y: e.bounds.Y + 6, W: 2, H: e.bounds.H - 12}, e.color)
}

// HitTest returns the entry when p is inside its bounds.
func (e *Entry) HitTest(p Point) Widget {
	return e.HitLeaf(e, p)
}

// ClickAt places the cursor (and the selection anchor) at the clicked
// text position.
func (e *Entry) ClickAt(p Point) {
	x := float64(p.X - e.bounds.X - 8)
	e.cursor = e.face.Shape(e.Text(), e.sizePx).CaretAt(x)
	e.anchor = e.cursor
}

// DragMove extends the selection while the pointer drags; the anchor
// stays where the press landed.
func (e *Entry) DragMove(p Point) {
	x := float64(p.X - e.bounds.X - 8)
	e.cursor = e.face.Shape(e.Text(), e.sizePx).CaretAt(x)
}

// DoubleClickAt selects the run of same-class runes (word or whitespace)
// under the clicked position.
func (e *Entry) DoubleClickAt(p Point) {
	if len(e.runes) == 0 {
		return
	}
	x := float64(p.X - e.bounds.X - 8)
	c := e.face.Shape(e.Text(), e.sizePx).CaretAt(x)
	if c >= len(e.runes) {
		c = len(e.runes) - 1
	}
	word := func(r rune) bool {
		return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	wantWord := word(e.runes[c])
	start := c
	for start > 0 && word(e.runes[start-1]) == wantWord {
		start--
	}
	end := c + 1
	for end < len(e.runes) && word(e.runes[end]) == wantWord {
		end++
	}
	e.cursor, e.anchor = end, start
}

// SelectAll selects the entire contents.
func (e *Entry) SelectAll() {
	e.anchor = 0
	e.cursor = len(e.runes)
}

// InsertRune implements RuneHandler.
func (e *Entry) InsertRune(r rune) {
	e.Insert(string(r))
}

// KeyAction implements KeyActionHandler for editing keys. Shift-extended
// motion grows the selection from its anchor.
func (e *Entry) KeyAction(a KeyAction, mods Mods) {
	shift := mods&ModShift != 0
	switch a {
	case KeyBackspace:
		e.Backspace()
	case KeyDelete:
		e.Delete()
	case KeyLeft:
		if shift {
			e.MoveCursorExtending(-1)
		} else {
			e.MoveCursor(-1)
		}
	case KeyRight:
		if shift {
			e.MoveCursorExtending(1)
		} else {
			e.MoveCursor(1)
		}
	case KeyHome:
		if shift {
			e.cursor = 0
		} else {
			e.MoveHome()
		}
	case KeyEnd:
		if shift {
			e.cursor = len(e.runes)
		} else {
			e.MoveEnd()
		}
	case KeyEnter:
	}
}
