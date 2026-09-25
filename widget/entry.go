package widget

import (
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

	runes  []rune
	cursor int
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
func (e *Entry) Text() string {
	return string(e.runes)
}

// SetText replaces the contents and moves the cursor to the end.
func (e *Entry) SetText(s string) {
	e.runes = []rune(s)
	e.cursor = len(e.runes)
}

// Cursor returns the cursor position as a rune index.
func (e *Entry) Cursor() int {
	return e.cursor
}

// Insert inserts s at the cursor and leaves the cursor after it.
func (e *Entry) Insert(s string) {
	r := []rune(s)
	e.runes = append(e.runes[:e.cursor], append(append([]rune{}, r...), e.runes[e.cursor:]...)...)
	e.cursor += len(r)
}

// Backspace deletes the rune before the cursor; it is a no-op at the start.
func (e *Entry) Backspace() {
	if e.cursor == 0 {
		return
	}
	e.runes = append(e.runes[:e.cursor-1], e.runes[e.cursor:]...)
	e.cursor--
}

// Delete deletes the rune at the cursor; it is a no-op at the end.
func (e *Entry) Delete() {
	if e.cursor >= len(e.runes) {
		return
	}
	e.runes = append(e.runes[:e.cursor], e.runes[e.cursor+1:]...)
}

// MoveCursor moves the cursor by delta runes, clamped to [0, len].
func (e *Entry) MoveCursor(delta int) {
	e.cursor += delta
	if e.cursor < 0 {
		e.cursor = 0
	}
	if e.cursor > len(e.runes) {
		e.cursor = len(e.runes)
	}
}

// MoveHome puts the cursor at the start.
func (e *Entry) MoveHome() { e.cursor = 0 }

// MoveEnd puts the cursor after the last rune.
func (e *Entry) MoveEnd() { e.cursor = len(e.runes) }

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

// Paint draws the field: placeholder when empty, text otherwise, and the
// cursor bar. Zero color fields fall back to the theme.
func (e *Entry) Paint(cv *render.Canvas) {
	t := Current()
	cv.RoundedRect(e.bounds, t.Radius, t.Surface)
	if len(e.runes) == 0 && e.placeholder != "" {
		e.face.DrawAligned(cv, e.placeholder, e.bounds, e.sizePx, t.Border, render.AlignStart)
		return
	}
	e.face.DrawAligned(cv, e.Text(), e.bounds, e.sizePx, e.color, render.AlignStart)

	// Cursor bar after the text before the cursor.
	prefix := string(e.runes[:e.cursor])
	x := e.bounds.X + 8
	if prefix != "" {
		x += int(e.face.Shape(prefix, e.sizePx).Advance() + 0.5)
	}
	cv.FillRect(render.Rect{X: x, Y: e.bounds.Y + 6, W: 2, H: e.bounds.H - 12}, e.color)
}

// HitTest returns the entry when p is inside its bounds.
func (e *Entry) HitTest(p Point) Widget {
	return e.HitLeaf(e, p)
}

// InsertRune implements RuneHandler.
func (e *Entry) InsertRune(r rune) {
	e.Insert(string(r))
}

// KeyAction implements KeyActionHandler for editing keys.
func (e *Entry) KeyAction(a KeyAction) {
	switch a {
	case KeyBackspace:
		e.Backspace()
	case KeyDelete:
		e.Delete()
	case KeyLeft:
		e.MoveCursor(-1)
	case KeyRight:
		e.MoveCursor(1)
	case KeyHome:
		e.MoveHome()
	case KeyEnd:
		e.MoveEnd()
	case KeyEnter:
	}
}
