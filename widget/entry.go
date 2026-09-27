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

	// OnChanged fires after the contents change, whatever the
	// source: typing, editing keys, clipboard, or SetText.
	OnChanged func(string)

	runes  []rune
	cursor int
	anchor int // selection anchor; equals cursor when nothing is selected

	// Echo masking: echo picks the display mode (dots, nothing) while
	// the contents stay logical, and reveal is the app's temporary
	// show-the-real-text override.
	echo   Echo
	reveal bool

	// hist is the undo/redo history; every mutation records into it.
	hist undoStack[entryState]

	// Composing (input-method preedit) display: peText shows at the
	// caret position peAt with the composing caret peCur runes into
	// it (-1 hidden). It lives outside the contents until a commit.
	peText []rune
	peAt   int
	peCur  int
}

// NewEntry returns an empty entry painted with face at sizePx.
func NewEntry(face *render.Typeface, sizePx float64, color render.Color) *Entry {
	return &Entry{face: face, sizePx: sizePx, color: color}
}

// SetPlaceholder sets the text shown when the entry is empty.
func (e *Entry) SetPlaceholder(s string) {
	if e.placeholder == s {
		return
	}
	e.placeholder = s
	e.InvalidateLayout()
}

// Text returns the entry contents.
// CursorName reports the text caret shape while hovered.
func (e *Entry) CursorName() string { return "xterm" }

func (e *Entry) Text() string {
	return string(e.runes)
}

// Undo restores the state before the most recent edit — contents,
// caret, and selection included — reporting whether there was anything
// to undo. Composing text drops first, like before any other edit.
func (e *Entry) Undo() bool {
	e.clearPreedit()
	return e.hist.undo(e.applyEntry)
}

// Redo reapplies the most recently undone edit, reporting whether
// there was one.
func (e *Entry) Redo() bool {
	e.clearPreedit()
	return e.hist.redo(e.applyEntry)
}

// entryState is one snapshot of the entry for the undo history.
type entryState struct {
	runes  []rune
	cursor int
	anchor int
}

// same reports whether two snapshots describe identical editing state.
func (s entryState) same(o entryState) bool {
	return s.cursor == o.cursor && s.anchor == o.anchor && string(s.runes) == string(o.runes)
}

// snapshot captures the current state for the history.
func (e *Entry) snapshot() entryState {
	return entryState{runes: append([]rune{}, e.runes...), cursor: e.cursor, anchor: e.anchor}
}

// applyEntry restores a snapshot exactly like the edit that produced
// it: contents, caret, and selection, plus the same damage and change
// signal the edit fired.
func (e *Entry) applyEntry(s entryState) {
	e.runes = append([]rune{}, s.runes...)
	e.cursor, e.anchor = s.cursor, s.anchor
	e.InvalidateLayout()
	e.changed()
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

// SetText replaces the contents, moves the cursor to the end, and
// invalidates the field.
func (e *Entry) SetText(s string) {
	e.clearPreedit()
	if s == e.Text() {
		return
	}
	e.runes = []rune(s)
	e.cursor = len(e.runes)
	e.anchor = e.cursor
	e.hist.reset() // app-driven replacement is not an edit to back out of
	e.InvalidateLayout()
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
	e.clearPreedit()
	start, end, active := e.Selection()
	if active {
		e.runes = append(e.runes[:start], e.runes[end:]...)
	}
	e.anchor = start
	e.cursor = start
}

// splice is the insert primitive: it drops the composing display,
// replaces any selection, and puts s at the caret. History recording
// is the callers' business — Insert lands as one entry, InsertRune as
// a coalescable typing run.
func (e *Entry) splice(s string) {
	e.clearPreedit()
	e.collapse()
	r := []rune(s)
	e.runes = append(e.runes[:e.cursor], append(append([]rune{}, r...), e.runes[e.cursor:]...)...)
	e.cursor += len(r)
	e.anchor = e.cursor
	e.InvalidateLayout()
	e.changed()
}

// Insert inserts s at the cursor. An active selection is replaced, and
// the whole insertion — a paste, a drop, a committed composition — is
// one undo entry.
func (e *Entry) Insert(s string) {
	before := e.snapshot()
	e.splice(s)
	e.hist.record(before, e.snapshot())
}

// Backspace deletes the selection, or the rune before the cursor when
// nothing is selected; one undo entry per call.
func (e *Entry) Backspace() {
	before := e.snapshot()
	e.clearPreedit()
	if _, _, active := e.Selection(); active {
		e.collapse()
		e.InvalidateLayout()
		e.changed()
	} else if e.cursor > 0 {
		e.runes = append(e.runes[:e.cursor-1], e.runes[e.cursor:]...)
		e.cursor--
		e.anchor = e.cursor
		e.InvalidateLayout()
		e.changed()
	}
	e.hist.record(before, e.snapshot())
}

// Delete deletes the selection, or the rune at the cursor when nothing
// is selected; one undo entry per call.
func (e *Entry) Delete() {
	before := e.snapshot()
	e.clearPreedit()
	if _, _, active := e.Selection(); active {
		e.collapse()
		e.InvalidateLayout()
		e.changed()
	} else if e.cursor < len(e.runes) {
		e.runes = append(e.runes[:e.cursor], e.runes[e.cursor+1:]...)
		e.InvalidateLayout()
		e.changed()
	}
	e.hist.record(before, e.snapshot())
}

// MoveCursor moves the cursor by delta runes, clamped to [0, len]. An
// active selection collapses to the edge the motion points at first,
// without moving further - the standard first-press behavior.
func (e *Entry) MoveCursor(delta int) {
	e.clearPreedit()
	if _, _, active := e.Selection(); active {
		start, end, _ := e.Selection()
		edge := start
		if delta > 0 {
			edge = end
		}
		e.cursor, e.anchor = edge, edge
		e.Invalidate()
		return
	}
	e.cursor += delta
	e.clampCursor()
	e.anchor = e.cursor
	e.Invalidate()
}

// MoveCursorExtending moves the cursor by delta runes, growing or
// shrinking the selection from its anchor (shift+arrow behavior).
func (e *Entry) MoveCursorExtending(delta int) {
	e.clearPreedit()
	e.cursor += delta
	e.clampCursor()
	e.Invalidate()
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
func (e *Entry) MoveHome() { e.clearPreedit(); e.cursor, e.anchor = 0, 0; e.Invalidate() }

// MoveEnd puts the cursor after the last rune, dropping any selection.
func (e *Entry) MoveEnd() {
	e.clearPreedit()
	e.cursor, e.anchor = len(e.runes), len(e.runes)
	e.Invalidate()
}

// Measure wants the text advance (or the placeholder's) plus padding; an
// empty field keeps its padding so the box stays visible. Clamped to con.
// Composing text counts toward the wanted width.
func (e *Entry) Measure(con Constraints) Size {
	if sz, ok := e.measureHit(con); ok {
		return sz
	}
	text := e.displayText()
	if text == "" {
		text = e.placeholder
	}
	w := 16
	if text != "" {
		w += int(e.face.Shape(text, e.sizePx).Advance() + 0.5)
	}
	h := int(e.face.Shape("lg", e.sizePx).Ascent()+e.face.Shape("lg", e.sizePx).Descent()+0.5) + 12
	return e.measureStore(con, clampSize(Size{W: w, H: h}, con))
}

// Paint draws the field: placeholder when empty, text otherwise, the
// selection highlight, the composing text with its underline, and the
// cursor bar. Zero color fields fall back to the theme.
func (e *Entry) Paint(cv *render.Canvas) {
	t := Current()
	cv.RoundedRect(e.bounds, t.Radius, t.Surface)
	disp := e.displayText()
	if len(e.runes) == 0 && !e.composing() && e.placeholder != "" {
		e.face.DrawAligned(cv, e.placeholder, e.bounds, e.sizePx, t.Border, render.AlignStart)
		return
	}
	// The highlight and the caret map through the display shape: in
	// masked modes the band covers dots, never the runes behind them.
	sh := e.face.Shape(disp, e.sizePx)
	if start, end, active := e.Selection(); active {
		x0 := e.bounds.X + 8 + int(sh.CaretX(start)+0.5)
		x1 := e.bounds.X + 8 + int(sh.CaretX(end)+0.5)
		a := t.Accent
		cv.FillRect(render.Rect{X: x0, Y: e.bounds.Y + 4, W: x1 - x0, H: e.bounds.H - 8},
			render.RGBA(a.R(), a.G(), a.B(), 90))
	}
	e.face.DrawAligned(cv, disp, e.bounds, e.sizePx, e.color, render.AlignStart)
	if e.composing() {
		// Accent underline under the composing range.
		a := t.Accent
		x0 := 8 + int(sh.CaretX(e.peAt)+0.5)
		x1 := 8 + int(sh.CaretX(e.peAt+len(e.peText))+0.5)
		cv.FillRect(render.Rect{
			X: e.bounds.X + x0, Y: e.bounds.Y + e.bounds.H - 8,
			W: max(x1-x0, 2), H: 2,
		}, render.RGBA(a.R(), a.G(), a.B(), 200))
	}
	// Cursor bar after the text before the caret; hidden while the
	// input method hides its composing caret.
	if caret := e.caretRune(); caret >= 0 {
		x := 8 + int(sh.CaretX(caret)+0.5)
		cv.FillRect(render.Rect{X: e.bounds.X + x, Y: e.bounds.Y + 6, W: 2, H: e.bounds.H - 12}, e.color)
	}
}

// Role implements Roleer.
func (e *Entry) Role() Role { return RoleEntry }

// HitTest returns the entry when p is inside its bounds.
func (e *Entry) HitTest(p Point) Widget {
	return e.HitLeaf(e, p)
}

// ClickAt places the cursor (and the selection anchor) at the clicked
// text position, dropping the composing display. The mapping runs
// through the display shape, so a click lands where the dots are while
// the cursor stays a logical rune index.
func (e *Entry) ClickAt(p Point) {
	e.clearPreedit()
	x := float64(p.X - e.bounds.X - 8)
	e.cursor = e.face.Shape(e.displayText(), e.sizePx).CaretAt(x)
	e.anchor = e.cursor
	e.Invalidate()
}

// DragMove extends the selection while the pointer drags; the anchor
// stays where the press landed.
func (e *Entry) DragMove(p Point) {
	e.clearPreedit()
	x := float64(p.X - e.bounds.X - 8)
	e.cursor = e.face.Shape(e.displayText(), e.sizePx).CaretAt(x)
	e.Invalidate()
}

// DoubleClickAt selects the run of same-class runes (word or whitespace)
// under the clicked position.
func (e *Entry) DoubleClickAt(p Point) {
	e.clearPreedit()
	if len(e.runes) == 0 {
		return
	}
	x := float64(p.X - e.bounds.X - 8)
	c := e.face.Shape(e.displayText(), e.sizePx).CaretAt(x)
	if c >= len(e.runes) {
		c = len(e.runes) - 1
	}
	wantWord := wordRune(e.runes[c])
	start := c
	for start > 0 && wordRune(e.runes[start-1]) == wantWord {
		start--
	}
	end := c + 1
	for end < len(e.runes) && wordRune(e.runes[end]) == wantWord {
		end++
	}
	e.cursor, e.anchor = end, start
	e.Invalidate()
}

// SelectAll selects the entire contents.
func (e *Entry) SelectAll() {
	e.clearPreedit()
	e.anchor = 0
	e.cursor = len(e.runes)
	e.Invalidate()
}

// InsertRune implements RuneHandler. Typed runes coalesce into one
// undo entry until a word boundary, an idle gap, or a different edit
// breaks the run.
func (e *Entry) InsertRune(r rune) {
	before := e.snapshot()
	e.splice(string(r))
	e.hist.recordTyping(before, e.snapshot(), r)
}

// KeyAction implements KeyActionHandler for editing keys. Shift-extended
// motion grows the selection from its anchor. While composing, the
// input method owns the text: backspace trims its last rune, and every
// other edit drops the composing display and applies to the contents.
func (e *Entry) KeyAction(a KeyAction, mods Mods) {
	shift := mods&ModShift != 0
	if a == KeyBackspace && e.composing() {
		e.peText = e.peText[:len(e.peText)-1]
		e.peCur = min(e.peCur, len(e.peText))
		if len(e.peText) == 0 {
			e.clearPreedit()
		}
		return
	}
	// Every action except the trim above drops the composing display.
	e.clearPreedit()
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
			e.Invalidate()
		} else {
			e.MoveHome()
		}
	case KeyEnd:
		if shift {
			e.cursor = len(e.runes)
			e.Invalidate()
		} else {
			e.MoveEnd()
		}
	case KeyEnter:
	}
}
