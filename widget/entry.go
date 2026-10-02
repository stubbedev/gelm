package widget

import (
	"math"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/internal/text"
	"github.com/stubbedev/gelm/render"
)

// Entry is a single-line text field. Its editing state machine (insert,
// delete, cursor motion) is input-agnostic: keyboard input in M4 calls
// these methods.
type Entry struct {
	node
	face        render.Font
	sizePx      float64
	color       render.Color
	dir         Direction
	placeholder string

	// OnChanged fires after the contents change, whatever the
	// source: typing, editing keys, clipboard, or SetText.
	OnChanged func(string)
	// OnActivate fires with the contents when the user presses Enter
	// in the entry (GTK's activate): submit a password, run a search,
	// move to the next field. A disabled entry never activates.
	OnActivate func(string)

	runes []rune
	// disp is the string form of runes, refreshed by setRunes: the
	// paint and measure paths read the display text several times per
	// frame, and re-converting the runes per read was the entry's
	// largest steady allocation. Every mutation of runes goes through
	// setRunes, so disp cannot drift.
	disp   string
	cursor int
	anchor int // selection anchor; equals cursor when nothing is selected

	// scrollX is the horizontal pan: the display-space x of the first
	// on-show pixel. When the text is wider than the field the caret
	// is kept in view by panning, never by editing. View state — never
	// an undo entry.
	scrollX int

	// MaxWidth caps the natural width Measure reports: a long paste
	// stops growing the layout at the cap and pans inside it instead —
	// the text widgets' counterpart of Scroll's bounded natural size.
	// Zero (the default) reports the content-hugging width. Set it
	// before the first Measure, or follow with InvalidateLayout.
	MaxWidth int

	// textWidth fixes the text area's natural width (SetTextWidth);
	// zero hugs the content.
	textWidth int

	// text is GtkText's node under the field (`entry > text`).
	text entryText

	// Echo masking: echo picks the display mode (dots, nothing) while
	// the contents stay logical, and reveal is the app's temporary
	// show-the-real-text override.
	echo   Echo
	reveal bool

	// hist is the undo/redo history; every mutation records into it.
	hist undoStack[entryState]

	// readOnly freezes user edits: typing, paste, cut, delete, and
	// undo/redo all no-op, while selection, copy, caret motion, and pan
	// keep working. Programmatic SetText is not a user edit and still
	// applies. See SetReadOnly for the full contract.
	readOnly bool

	// Composing (input-method preedit) display: peText shows at the
	// caret position peAt with the composing caret peCur runes into
	// it (-1 hidden). It lives outside the contents until a commit.
	peText []rune
	peAt   int
	peCur  int

	// bands is the selection/underline scratch the paint reuses across
	// frames, so the render path stays allocation-free in the steady
	// state.
	bands [][2]float64
}

// NewEntry returns an empty entry painted with face at sizePx. Face
// may be a render.Chain for mixed-script fallback. A nil face panics
// here (see requireFace) instead of failing later, in shaping.
func NewEntry(face render.Font, sizePx float64, color render.Color) *Entry {
	e := &Entry{face: requireFace("widget.NewEntry", face), sizePx: sizePx, color: color}
	e.text.SetElement("text")
	e.text.placeholder.SetElement("placeholder")
	e.text.selection.SetElement("selection")
	setParents(e, &e.text)
	setParents(&e.text, &e.text.placeholder, &e.text.selection)
	return e
}

// entryText is GtkText's node under an entry (`entry > text`): its
// margin, border and padding inset the text, its color paints the
// text, and its placeholder and selection children (`text >
// placeholder`, `text > selection`) color those. The entry matches
// and paints them; they are never laid out on their own.
type entryText struct {
	entryPart
	placeholder, selection entryPart
}

func (t *entryText) styleChildren() []Widget { return []Widget{&t.placeholder, &t.selection} }

// entryPart is a style-only node of an entry.
type entryPart struct{ node }

func (*entryPart) Measure(con Constraints) Size { return clampSize(Size{}, con) }
func (*entryPart) Paint(*render.Canvas)         {}
func (*entryPart) HitTest(Point) Widget         { return nil }

// styleChildren is the text node (styleKids).
func (e *Entry) styleChildren() []Widget { return []Widget{&e.text} }

// entryPad is an unstyled field's padding: lineheight plus 12 tall.
var entryPad = render.Insets{Top: 6, Right: 8, Bottom: 6, Left: 8}

// textInsets are the text area's insets inside the field's bounds:
// the field's border and padding (entryPad where unstyled) and the
// text node's margin, border and padding.
func (e *Entry) textInsets() render.Insets {
	v := e.style(e)
	b, p := borderOf(v), paddingOr(v, entryPad)
	t := boxOf(e.text.style(&e.text), render.Insets{}).outer()
	return render.Insets{
		Top:    b.Top + p.Top + t.Top,
		Right:  b.Right + p.Right + t.Right,
		Bottom: b.Bottom + p.Bottom + t.Bottom,
		Left:   b.Left + p.Left + t.Left,
	}
}

// contentRect is the text area: the bounds less textInsets.
func (e *Entry) contentRect() render.Rect {
	in := e.textInsets()
	return render.Rect{
		X: e.bounds.X + in.Left, Y: e.bounds.Y + in.Top,
		W: max(e.bounds.W-in.Left-in.Right, 0), H: max(e.bounds.H-in.Top-in.Bottom, 0),
	}
}

// Placeholder returns the text shown when the entry is empty.
func (e *Entry) Placeholder() string { return e.placeholder }

// SetDirection selects the base paragraph direction the contents
// resolve and lay out with. DirectionAuto (the default) reads it off
// the text's first strong character; DirectionRTL additionally hugs
// short text against the right edge, where its reading starts, and
// mirrors the pan. Arrow and word motion always move visually - the
// mapping runs through the same resolution (internal/text) the paint
// uses - while cursor, anchor, and selection stay logical rune
// indexes. Changing the direction re-pans and invalidates.
func (e *Entry) SetDirection(d Direction) {
	if e.dir == d {
		return
	}
	e.dir = d
	e.panToCaret()
	e.InvalidateLayout()
}

// Direction returns the base paragraph direction the entry resolves
// with.
func (e *Entry) Direction() Direction { return e.dir }

// shape shapes display text under the entry's base direction - the
// one text path the field's caret, selection, and paint share. The
// size is the stylesheet's font-size when matched, else the
// constructor's.
func (e *Entry) shape(s string) *render.ShapedText {
	return e.face.ShapeDir(s, e.fontPx(), e.dir)
}

// fontPx is the effective shaping size: the stylesheet's font-size
// when matched, else the constructor's.
func (e *Entry) fontPx() float64 {
	return fontPx(e.style(e), e.sizePx)
}

// pad is the effective horizontal text inset: the stylesheet's
// padding when set, else the built-in 8.

// SetPlaceholder sets the text shown when the entry is empty.
func (e *Entry) SetPlaceholder(s string) {
	if e.placeholder == s {
		return
	}
	e.placeholder = s
	e.InvalidateLayout()
}

// SetReadOnly toggles the read-only mode for copy-only fields (a
// generated API key, a resolved config value). The contract:
//
//   - blocked: typing, paste, drop, composition, Backspace/Delete
//     (cut), Undo/Redo, and — because they are user edits — new undo
//     entries. A history built before SetReadOnly(true) is untouched,
//     not cleared: flipping read-only back off resumes backing out the
//     very same edits.
//   - still working: selection (click, drag, double-click,
//     SelectAll), copy through SelectedText, caret motion, and pan.
//   - visual: the text keeps its normal color; only the caret fades
//     to the disabled fade. A disabled entry is the mirror image —
//     muted text — so the two states never look alike.
//
// SetText remains programmatic and applies regardless.
func (e *Entry) SetReadOnly(ro bool) {
	if e.readOnly == ro {
		return
	}
	e.readOnly = ro
	if ro {
		e.clearPreedit() // no composing display on a frozen field
	}
	e.Invalidate()
}

// ReadOnly reports whether the entry is in the read-only mode.
func (e *Entry) ReadOnly() bool { return e.readOnly }

// editable reports whether user edits may land: the field accepts
// input at all (it and its ancestors are enabled) and is not
// read-only. Every mutation entry point opens with it; SetText is the
// one deliberate exception.
func (e *Entry) editable() bool { return e.Enabled() && !e.readOnly }

// CursorName reports the text caret shape while hovered.
func (e *Entry) CursorName() string { return "xterm" }

// Text returns the entry contents.
func (e *Entry) Text() string {
	return string(e.runes)
}

// Undo restores the state before the most recent edit — contents,
// caret, and selection included — reporting whether there was anything
// to undo. Composing text drops first, like before any other edit.
// Undo is a user edit: disabled or read-only, it no-ops and returns
// false while the history waits untouched.
func (e *Entry) Undo() bool {
	if !e.editable() {
		return false
	}
	e.clearPreedit()
	return e.hist.undo(e.applyEntry)
}

// Redo reapplies the most recently undone edit, reporting whether
// there was one. Like Undo it is a user edit and no-ops while
// disabled or read-only.
func (e *Entry) Redo() bool {
	if !e.editable() {
		return false
	}
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
// signal the edit fired. The pan follows the restored caret — the
// history stores editing state, not view state.
func (e *Entry) applyEntry(s entryState) {
	e.setRunes(append([]rune{}, s.runes...))
	e.cursor, e.anchor = s.cursor, s.anchor
	e.panToCaret()
	e.InvalidateLayout()
	e.changed()
}

// changed fires OnChanged after a content change.
func (e *Entry) changed() {
	if e.OnChanged != nil {
		e.OnChanged(e.Text())
	}
}

// setRunes replaces the contents and refreshes the string form the
// paint and measure paths read. Every mutation of e.runes goes through
// here, so displayText never re-converts the runes per read.
func (e *Entry) setRunes(rs []rune) {
	e.runes = rs
	e.disp = string(rs)
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
	e.setRunes([]rune(s))
	e.cursor = len(e.runes)
	e.anchor = e.cursor
	e.hist.reset() // app-driven replacement is not an edit to back out of
	e.panToCaret()
	e.InvalidateLayout()
	e.changed()
}

// Cursor returns the cursor position as a rune index — the internal
// position representation stays rune-based (#57); cluster segmentation
// (internal/text) maps the editing steps, which move one grapheme
// cluster at a time, onto those indexes. Every position the widget
// itself produces is a cluster boundary.
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
		e.setRunes(append(e.runes[:start], e.runes[end:]...))
	}
	e.anchor = start
	e.cursor = start
}

// splice is the insert primitive: it drops the composing display,
// replaces any selection, and puts s at the caret. The caret advances
// past what was inserted, snapped forward to a grapheme cluster
// boundary — appending runes, counting clusters (#57) — so a caret
// never lands inside the character it just typed. History recording is
// the callers' business — Insert lands as one entry, InsertRune as a
// coalescable typing run.
func (e *Entry) splice(s string) {
	e.clearPreedit()
	e.collapse()
	r := []rune(s)
	e.setRunes(append(e.runes[:e.cursor], append(append([]rune{}, r...), e.runes[e.cursor:]...)...))
	e.cursor = text.SnapClusterForward(e.runes, e.cursor+len(r))
	e.anchor = e.cursor
	e.panToCaret()
	e.InvalidateLayout()
	e.changed()
}

// Insert inserts s at the cursor. An active selection is replaced, and
// the whole insertion — a paste, a drop, a committed composition — is
// one undo entry. Blocked while disabled or read-only.
func (e *Entry) Insert(s string) {
	if !e.editable() {
		return
	}
	before := e.snapshot()
	e.splice(s)
	e.hist.record(before, e.snapshot())
}

// Backspace deletes the selection, or the grapheme cluster before the
// cursor when nothing is selected — an emoji family, a flag, a base
// rune with its combining marks go as one whole (#57); one undo entry
// per call. Blocked while disabled or read-only.
func (e *Entry) Backspace() {
	if !e.editable() {
		return
	}
	before := e.snapshot()
	e.clearPreedit()
	if _, _, active := e.Selection(); active {
		e.collapse()
		e.InvalidateLayout()
		e.changed()
	} else if start := text.PrevCluster(e.runes, e.cursor); start < e.cursor {
		e.setRunes(append(e.runes[:start], e.runes[e.cursor:]...))
		e.cursor, e.anchor = start, start
		e.panToCaret()
		e.InvalidateLayout()
		e.changed()
	}
	e.hist.record(before, e.snapshot())
}

// Delete deletes the selection, or the grapheme cluster at the cursor
// when nothing is selected (#57); one undo entry per call. This is the
// cut half of ctrl+x; blocked while disabled or read-only.
func (e *Entry) Delete() {
	if !e.editable() {
		return
	}
	before := e.snapshot()
	e.clearPreedit()
	if _, _, active := e.Selection(); active {
		e.collapse()
		e.InvalidateLayout()
		e.changed()
	} else if end := text.NextCluster(e.runes, e.cursor); end > e.cursor {
		e.setRunes(append(e.runes[:e.cursor], e.runes[end:]...))
		e.panToCaret()
		e.InvalidateLayout()
		e.changed()
	}
	e.hist.record(before, e.snapshot())
}

// DeleteWordBackward removes the word before the cursor — ctrl+
// backspace, with alt+backspace as the macOS alias — or the selection
// when one is active; the whole word is one undo entry. Blocked while
// disabled or read-only.
func (e *Entry) DeleteWordBackward() {
	if !e.editable() {
		return
	}
	before := e.snapshot()
	e.clearPreedit()
	if _, _, active := e.Selection(); active {
		e.collapse()
		e.InvalidateLayout()
		e.changed()
	} else if start := text.WordStart(e.runes, e.cursor); start < e.cursor {
		e.setRunes(append(e.runes[:start], e.runes[e.cursor:]...))
		e.cursor, e.anchor = start, start
		e.panToCaret()
		e.InvalidateLayout()
		e.changed()
	}
	e.hist.record(before, e.snapshot())
}

// DeleteWordForward removes the word after the cursor — ctrl+delete —
// or the selection when one is active; the whole word is one undo
// entry. Blocked while disabled or read-only.
func (e *Entry) DeleteWordForward() {
	if !e.editable() {
		return
	}
	before := e.snapshot()
	e.clearPreedit()
	if _, _, active := e.Selection(); active {
		e.collapse()
		e.InvalidateLayout()
		e.changed()
	} else if end := text.WordEnd(e.runes, e.cursor); end > e.cursor {
		e.anchor = e.cursor
		e.cursor = end
		e.collapse()
		e.panToCaret()
		e.InvalidateLayout()
		e.changed()
	}
	e.hist.record(before, e.snapshot())
}

// stepClusters moves the cursor delta grapheme clusters — the editing
// unit (#57): an emoji family, a flag, or a base rune with its marks
// steps as one. Positions stay rune indexes; segmentation maps the
// steps. This is the logical mapping MoveCursor serves; the arrow keys
// step visually instead (stepVisually).
func (e *Entry) stepClusters(delta int) {
	step := max(min(delta, 1), -1)
	for range max(delta, -delta) {
		if step > 0 {
			e.cursor = text.NextCluster(e.runes, e.cursor)
		} else {
			e.cursor = text.PrevCluster(e.runes, e.cursor)
		}
	}
}

// stepVisually moves the cursor delta visual steps over the display
// shape — left for negative delta, right for positive — the mapping
// the arrow keys move by over mixed-direction text. The display
// resolves under the entry's base direction (internal/text, cached);
// the positions it lands on stay logical rune indexes, snapped to
// grapheme clusters.
func (e *Entry) stepVisually(delta int) {
	disp := e.displayText()
	rs := []rune(disp)
	order := text.VisualOrder(rs, text.BidiRuns(disp, e.dir))
	step := max(min(delta, 1), -1)
	for range max(delta, -delta) {
		e.cursor = text.VisualStep(rs, order, e.cursor, step)
	}
}

// collapseToEdge drops any selection, parking the cursor at its
// visually outermost boundary in the direction delta points — the
// caret x decides for a span crossing runs — so the first press of an
// arrow backs out of the selection from the side it moved from.
func (e *Entry) collapseToEdge(delta int) {
	start, end, _ := e.Selection()
	sh := e.shape(e.displayText())
	lo, hi := start, end
	if sh.CaretX(lo) > sh.CaretX(hi) {
		lo, hi = hi, lo
	}
	edge := hi
	if delta < 0 {
		edge = lo
	}
	e.cursor, e.anchor = edge, edge
	e.panToCaret()
	e.Invalidate()
}

// moveVisually is MoveCursor with the arrow keys' visual mapping: the
// cursor moves one visual step while staying a logical rune index. An
// active selection collapses to the visually-directed edge first,
// without moving further — the standard first-press behavior.
func (e *Entry) moveVisually(delta int, extend bool) {
	e.clearPreedit()
	if !extend {
		if _, _, active := e.Selection(); active {
			e.collapseToEdge(delta)
			return
		}
	}
	e.stepVisually(delta)
	if !extend {
		e.anchor = e.cursor
	}
	e.panToCaret()
	e.Invalidate()
}

// MoveCursor moves the cursor by delta grapheme clusters, clamped to
// [0, len] (#57: one emoji, one step). An active selection collapses to
// the edge the motion points at first, without moving further - the
// standard first-press behavior.
func (e *Entry) MoveCursor(delta int) {
	e.clearPreedit()
	if _, _, active := e.Selection(); active {
		start, end, _ := e.Selection()
		edge := start
		if delta > 0 {
			edge = end
		}
		e.cursor, e.anchor = edge, edge
		e.panToCaret()
		e.Invalidate()
		return
	}
	e.stepClusters(delta)
	e.anchor = e.cursor
	e.panToCaret()
	e.Invalidate()
}

// MoveCursorExtending moves the cursor by delta grapheme clusters,
// growing or shrinking the selection from its anchor (shift+arrow
// behavior); selection edges land on cluster boundaries.
func (e *Entry) MoveCursorExtending(delta int) {
	e.clearPreedit()
	e.stepClusters(delta)
	e.panToCaret()
	e.Invalidate()
}

// wordTo moves the cursor to the word edge the visual direction delta
// points at — over the shared word segmentation (internal/text) walked
// in the display's visual order, so motion, deletion, and double-click
// agree on what a word is while arrows land where the eye looks.
// With extend the selection grows or shrinks from its anchor; without
// it an active selection first collapses to the visually-directed
// edge, the standard first-press behavior.
func (e *Entry) wordTo(delta int, extend bool) {
	e.clearPreedit()
	if !extend {
		if _, _, active := e.Selection(); active {
			e.collapseToEdge(delta)
			return
		}
	}
	disp := e.displayText()
	rs := []rune(disp)
	order := text.VisualOrder(rs, text.BidiRuns(disp, e.dir))
	e.cursor = text.VisualWordStep(e.runes, order, e.cursor, delta)
	if !extend {
		e.anchor = e.cursor
	}
	e.panToCaret()
	e.Invalidate()
}

// MoveWord moves the cursor one visual word: the word edge the
// direction points at, gaps (whitespace, punctuation runs) skipped
// whole. An active selection collapses to the visually-directed edge
// first, without moving.
func (e *Entry) MoveWord(delta int) { e.wordTo(delta, false) }

// MoveWordExtending is MoveWord with shift held: the selection
// extends from its anchor to the word edge.
func (e *Entry) MoveWordExtending(delta int) { e.wordTo(delta, true) }

// MoveHome puts the cursor at the start, dropping any selection; the
// pan follows, back to zero.
func (e *Entry) MoveHome() {
	e.clearPreedit()
	e.cursor, e.anchor = 0, 0
	e.panToCaret()
	e.Invalidate()
}

// MoveEnd puts the cursor after the last rune, dropping any selection,
// and pans the field so the tail is on show.
func (e *Entry) MoveEnd() {
	e.clearPreedit()
	e.cursor, e.anchor = len(e.runes), len(e.runes)
	e.panToCaret()
	e.Invalidate()
}

// innerRect is the padded text viewport: the rect painting clips to
// and the pan moves text across.
func (e *Entry) innerRect() render.Rect {
	c := e.contentRect()
	return render.Rect{X: c.X, Y: e.bounds.Y, W: c.W, H: e.bounds.H}
}

// panCaret is the display-space caret the pan follows: the composing
// caret while the input method shows it, the end of the composing
// text when it hides it, the plain cursor otherwise.
func (e *Entry) panCaret() int {
	if caret := e.caretRune(); caret >= 0 {
		return caret
	}
	return e.peAt + len(e.peText)
}

// lineX is the screen x of the display line's left edge inside the
// inner rect. For a left-to-right line the pan pulls the line left of
// the inner rect's left edge; for a right-to-left one the line starts
// flush with the right edge — where its reading starts — and the pan
// slides the window toward the reading end, mirroring LTR exactly with
// the caret's distance from the right edge in the caret's role.
func (e *Entry) lineX(sh *render.ShapedText) int {
	left := e.contentRect().X
	if !text.RTL(e.displayText(), e.dir) {
		return left - e.scrollX
	}
	return left + e.innerRect().W - int(sh.Advance()+0.5) + e.scrollX
}

// caretX is the screen x of the caret before display rune caret.
func (e *Entry) caretX(sh *render.ShapedText, caret int) int {
	return e.lineX(sh) + int(sh.CaretX(caret)+0.5)
}

// panToCaret adjusts scrollX so the caret stays visible: flush with the
// reading's start edge, or caretPad of following text inside the far
// edge — measured from the left for an LTR line, from the right for an
// RTL one. A view mutation — callers own the repaint.
func (e *Entry) panToCaret() {
	if e.face == nil {
		return
	}
	avail := e.innerRect().W
	if avail <= 0 {
		e.scrollX = 0
		return
	}
	sh := e.shape(e.displayText())
	cx := int(sh.CaretX(e.panCaret()) + 0.5)
	if text.RTL(e.displayText(), e.dir) {
		cx = int(sh.Advance()+0.5) - cx // the caret's distance from the reading start edge
	}
	switch {
	case cx < e.scrollX:
		e.scrollX = cx
	case cx > e.scrollX+avail-caretPad:
		e.scrollX = cx - avail + caretPad
	}
	e.clampPan()
}

// clampPan pulls scrollX into the scrollable range: zero to the
// display width plus the caret margin past the right edge. Text or
// width changes shrink a stale pan without waiting for the caret.
func (e *Entry) clampPan() {
	avail := e.innerRect().W
	textW := 0
	if e.face != nil && avail > 0 {
		textW = int(e.shape(e.displayText()).Advance() + 0.5)
	}
	if avail <= 0 {
		e.scrollX = 0
		return
	}
	e.scrollX = min(max(e.scrollX, 0), max(0, textW+caretPad-avail))
}

// ScrollX returns the horizontal pan: the display-space x of the first
// on-show pixel.
func (e *Entry) ScrollX() int { return e.scrollX }

// GTKTextWidth is a GtkEntry's text width without width-chars
// (GtkText's MIN_TEXT_WIDTH): pass it to SetTextWidth for a field
// sized the way GTK sizes one.
const GTKTextWidth = 150

// SetTextWidth fixes the text area's natural width at px, padding
// aside, the way GtkEntry sizes a field: what is typed pans inside it
// instead of resizing the layout, and an empty field keeps its width.
// Zero (the default) hugs the content.
func (e *Entry) SetTextWidth(px int) {
	px = max(px, 0)
	if e.textWidth != px {
		e.textWidth = px
		e.InvalidateLayout()
	}
}

// TextWidth reports SetTextWidth.
func (e *Entry) TextWidth() int { return e.textWidth }

// Measure wants the text advance (or the placeholder's) plus padding; an
// empty field keeps its padding so the box stays visible. Clamped to con.
// Composing text counts toward the wanted width. SetTextWidth fixes the
// text's share instead. MaxWidth caps the reported width — a
// 500-character paste then stops widening the layout at the cap and
// pans inside it instead.
func (e *Entry) Measure(con Constraints) Size {
	if sz, ok := e.measureHit(con); ok {
		return sz
	}
	text := e.displayText()
	if text == "" {
		text = e.placeholder
	}
	px := e.fontPx()
	in := e.textInsets()
	w := in.Left + in.Right
	switch {
	case e.textWidth > 0:
		w += e.textWidth
	case text != "":
		w += int(e.face.Shape(text, px).Advance() + 0.5)
	}
	lg := e.face.Shape("lg", px)
	line := max(int(lg.Ascent()+lg.Descent()+0.5), picki(e.text.style(&e.text), style.PropMinHeight, 0))
	h := line + in.Top + in.Bottom
	if e.MaxWidth > 0 {
		w = min(w, max(e.MaxWidth, 16))
	}
	// The stylesheet's min-* floors hold before the constraints clamp:
	// the field claims at least the styled floor, overflowing if needed.
	v := e.style(e)
	w = max(w, picki(v, style.PropMinWidth, 0))
	h = max(h, picki(v, style.PropMinHeight, 0))
	// The stylesheet's margin sits outside the field, as in every CSS box.
	m := marginOf(v)
	return e.measureStore(con, clampSize(Size{W: w + m.Left + m.Right, H: h + m.Top + m.Bottom}, con))
}

// MinSize implements MinSizer: the stylesheet's min-* floors when set,
// no floor otherwise (the zero Size).
func (e *Entry) MinSize() Size {
	v := e.style(e)
	return Size{
		W: picki(v, style.PropMinWidth, 0),
		H: picki(v, style.PropMinHeight, 0),
	}
}

// Paint draws the field: placeholder when empty, text otherwise, the
// selection highlight, the composing text with its underline, and the
// cursor bar. Overflowing text pans: the content draws at -scrollX,
// clipped to the inner rect, so the caret never leaves the field. The
// placeholder only shows on an empty field and never pans. Zero color
// fields fall back to the theme.
//
// State colors: a disabled field fills with the derived disabled
// surface and paints text at the shared disabled fade; a read-only
// field keeps the normal text and fades only the caret, so the two
// states read differently at a glance (muted text vs muted caret).
// The stylesheet layers between the programmatic color and the theme,
// and border-width rounds the fill down inside a border-color stroke.
func (e *Entry) Paint(cv *render.Canvas) {
	t := Current()
	v := e.style(e)
	enabled := IsEnabled(e)
	bg := t.Surface
	if !enabled {
		bg = t.DisabledSurface()
	}
	bg = pickc(0, v, style.PropBackgroundColor, bg)
	radii := radiusOr(v, t.Radius)
	// The text node's color, its own or the field's it inherits.
	textCol := pickc(e.color, e.text.style(&e.text), style.PropColor, pickc(0, v, style.PropColor, e.color))
	caretCol := textCol
	if !enabled {
		textCol = scaleAlpha(textCol, disabledFade)
		caretCol = textCol
	} else if e.readOnly {
		caretCol = scaleAlpha(caretCol, disabledFade)
	}
	// The fill, then the rounded border ring over its edge, the same
	// CSS box layers every styled widget paints.
	paintBoxBehind(cv, v, e.bounds, radii, borderOf(v), bg)
	c := e.contentRect()
	disp := e.displayText()
	if len(e.runes) == 0 && !e.composing() && e.placeholder != "" {
		col := t.Border
		if pv := e.text.placeholder.style(&e.text.placeholder); pv.Declares(style.PropColor) {
			col = pv.Color
		}
		if !enabled {
			col = scaleAlpha(col, disabledFade)
		}
		// On the text's baseline at the reading's start edge, unpanned.
		ph := e.face.ShapeDir(e.placeholder, e.fontPx(), e.dir)
		x := c.X
		if text.RTL(e.placeholder, e.dir) {
			x = c.X + c.W - int(ph.Advance()+0.5)
		}
		prev := cv.PushClip(e.innerRect())
		ph.Draw(cv, x, e.baseline(ph, c), col)
		cv.PopClip(prev)
		return
	}
	// The highlight and the caret map through the display shape: in
	// masked modes the band covers dots, never the runes behind them.
	// The line origin comes from lineX, so a right-to-left line hugs
	// the right edge and pans mirrored, and a selection across mixed
	// directions highlights one band per visual run.
	sh := e.shape(disp)
	lx := e.lineX(sh)
	prev := cv.PushClip(e.innerRect())
	baseline := e.baseline(sh, c)
	start, end, selecting := e.Selection()
	var selFg render.Color
	if selecting {
		a := t.Accent
		fill := render.RGBA(a.R(), a.G(), a.B(), 90)
		sv := e.text.selection.style(&e.text.selection)
		if sv.Declares(style.PropBackgroundColor) {
			fill = sv.Background
		}
		if sv.Declares(style.PropColor) {
			selFg = sv.Color
		}
		for _, band := range sh.AppendCaretBands(e.bands[:0], start, end) {
			cv.FillRect(e.bandRect(lx, band, c), fill)
		}
	}
	sh.Draw(cv, lx, baseline, textCol)
	if selFg != 0 {
		// The selected runes redraw in the selection's color.
		for _, band := range sh.AppendCaretBands(e.bands[:0], start, end) {
			clip := cv.PushClip(e.bandRect(lx, band, c))
			sh.Draw(cv, lx, baseline, selFg)
			cv.PopClip(clip)
		}
	}
	if e.composing() {
		// Accent underline under the composing range.
		a := t.Accent
		for _, band := range sh.AppendCaretBands(e.bands[:0], e.peAt, e.peAt+len(e.peText)) {
			cv.FillRect(render.Rect{
				X: lx + int(band[0]+0.5), Y: c.Y + c.H - 2,
				W: max(int(band[1]+0.5)-int(band[0]+0.5), 2), H: 2,
			}, render.RGBA(a.R(), a.G(), a.B(), 200))
		}
	}
	// Cursor bar after the text before the caret, while the entry has
	// keyboard focus (GTK's rule); hidden while the input method hides
	// its composing caret. Every frame reads the same offset, so a
	// repaint (blink or otherwise) never jumps it.
	if caret := e.caretRune(); caret >= 0 && e.focused {
		cv.FillRect(render.Rect{X: e.caretX(sh, caret), Y: c.Y, W: 2, H: c.H}, caretCol)
	}
	cv.PopClip(prev)
}

// baseline centers sh's line in the text area c.
func (e *Entry) baseline(sh *render.ShapedText, c render.Rect) int {
	return c.Y + int(math.Round((float64(c.H)-float64(sh.LineHeight()))/2+sh.Ascent()))
}

// bandRect is a selection band's rect: the band's run across the
// text area, 2px taller each way, within the field.
func (e *Entry) bandRect(lx int, band [2]float64, c render.Rect) render.Rect {
	return render.Rect{
		X: lx + int(band[0]+0.5), Y: c.Y - 2,
		W: int(band[1]+0.5) - int(band[0]+0.5), H: c.H + 4,
	}.Intersect(e.bounds)
}

// Arrange pins the field's rect and re-pans: a resize changes the
// visible width, so the caret may need pulling back into view. The
// resize itself already scheduled the layout damage. r is the margin box;
// Bounds records the field inside the stylesheet's margin.
func (e *Entry) Arrange(r render.Rect) {
	e.node.Arrange(marginOf(e.style(e)).Shrink(r))
	e.panToCaret()
}

// Role implements Roleer.
func (e *Entry) Role() Role { return RoleEntry }

// HitTest returns the entry when p is inside its bounds.
func (e *Entry) HitTest(p Point) Widget {
	return e.HitLeaf(e, p)
}

// ClickAt places the cursor (and the selection anchor) at the clicked
// text position, dropping the composing display. The mapping runs
// through the display shape's caret table — which follows the line
// visually — and the pan, so a click on a half-visible rune lands on
// that rune, snapped to the start of its grapheme cluster (#57).
// Masked modes keep the cursor a logical index.
func (e *Entry) ClickAt(p Point) {
	e.clearPreedit()
	sh := e.shape(e.displayText())
	e.cursor = text.SnapCluster(e.runes, sh.CaretAt(float64(p.X-e.lineX(sh))))
	e.anchor = e.cursor
	e.Invalidate()
}

// DragMove extends the selection while the pointer drags; the anchor
// stays where the press landed. A motion past either edge auto-pans
// one step so the drag can reach text outside the viewport. Drag edges
// snap to cluster starts like ClickAt.
func (e *Entry) DragMove(p Point) {
	e.clearPreedit()
	inner := e.innerRect()
	e.scrollX = edgePan(p.X, inner.X, inner.X+inner.W, e.scrollX)
	e.clampPan()
	sh := e.shape(e.displayText())
	e.cursor = text.SnapCluster(e.runes, sh.CaretAt(float64(p.X-e.lineX(sh))))
	e.Invalidate()
}

// DoubleClickAt selects the run of same-class runes (word or
// whitespace) under the clicked position, over the shared word
// segmentation — the same words the ctrl+arrows steps land on.
func (e *Entry) DoubleClickAt(p Point) {
	e.clearPreedit()
	if len(e.runes) == 0 {
		return
	}
	sh := e.shape(e.displayText())
	c := sh.CaretAt(float64(p.X - e.lineX(sh)))
	start, end := text.WordRun(e.runes, c)
	e.cursor, e.anchor = end, start
	e.panToCaret()
	e.Invalidate()
}

// SelectAll selects the entire contents; the pan follows the caret to
// the end.
func (e *Entry) SelectAll() {
	e.clearPreedit()
	e.anchor = 0
	e.cursor = len(e.runes)
	e.panToCaret()
	e.Invalidate()
}

// InsertRune implements RuneHandler. Typed runes coalesce into one
// undo entry until a word boundary, an idle gap, or a different edit
// breaks the run. Blocked while disabled or read-only.
func (e *Entry) InsertRune(r rune) {
	if !e.editable() {
		return
	}
	before := e.snapshot()
	e.splice(string(r))
	e.hist.recordTyping(before, e.snapshot(), r)
}

// KeyAction implements KeyActionHandler for editing keys. Shift-extended
// motion grows the selection from its anchor; ctrl turns arrows and
// backspace/delete word-wise, alt+backspace aliasing ctrl+backspace.
// The bare keys edit one grapheme cluster per press (#57), so emoji
// and accented pairs edit whole. Interaction order: while composing,
// the input method owns the text and the bare backspace unwinds the
// composing display one rune first — that is compose state (#54),
// orthogonal to committed text; after it, ctrl/alt make backspace and
// delete word-wise over the cluster-refined words (#56); the bare keys
// apply to the contents one cluster at a time. A disabled entry
// ignores keys outright; a read-only one keeps the motion and
// selection keys and drops only the mutating ones.
func (e *Entry) KeyAction(a KeyAction, mods Mods) {
	if !e.Enabled() {
		return
	}
	shift := mods&ModShift != 0
	ctrl := mods&ModCtrl != 0
	if a == KeyBackspace && mods&(ModCtrl|ModAlt) == 0 && e.composing() && e.editable() {
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
		if e.editable() {
			if ctrl || mods&ModAlt != 0 {
				e.DeleteWordBackward()
			} else {
				e.Backspace()
			}
		}
	case KeyDelete:
		if e.editable() {
			if ctrl {
				e.DeleteWordForward()
			} else {
				e.Delete()
			}
		}
	case KeyLeft:
		if ctrl {
			if shift {
				e.MoveWordExtending(-1)
			} else {
				e.MoveWord(-1)
			}
		} else if shift {
			e.moveVisually(-1, true)
		} else {
			e.moveVisually(-1, false)
		}
	case KeyRight:
		if ctrl {
			if shift {
				e.MoveWordExtending(1)
			} else {
				e.MoveWord(1)
			}
		} else if shift {
			e.moveVisually(1, true)
		} else {
			e.moveVisually(1, false)
		}
	case KeyHome:
		if shift {
			e.cursor = 0
			e.panToCaret()
			e.Invalidate()
		} else {
			e.MoveHome()
		}
	case KeyEnd:
		if shift {
			e.cursor = len(e.runes)
			e.panToCaret()
			e.Invalidate()
		} else {
			e.MoveEnd()
		}
	case KeyEnter:
		if e.OnActivate != nil {
			e.OnActivate(e.Text())
		}
	}
}
