package widget

import (
	"strings"
	"unicode"

	"github.com/stubbedev/gelm/render"
)

// TextArea is a multi-line editor: logical lines split on newlines,
// per-line cursor motion with a sticky preferred column, and a
// selection model shared with Entry. With wrap enabled (the default)
// long logical lines flow across visual rows built from shaped advance
// widths; editing and Text stay logical regardless.
type TextArea struct {
	node
	face        *render.Typeface
	sizePx      float64
	color       render.Color
	placeholder string
	wrap        bool
	indent      int // spaces per TrapTab insertion; 0 inserts a tab

	lines   [][]rune
	cursor  pos
	anchor  pos
	prefX   float64 // preferred visual column for vertical motion
	hasPref bool

	// Composing (input-method preedit) display: peText shows at peAt
	// with the composing caret peCur runes into it (-1 hidden). It
	// lives outside the lines until a commit; displayLine splices it
	// in so the row cache and painting see what is on show.
	peText []rune
	peAt   pos
	peCur  int

	// Visual row cache, built lazily for the wrap width it was built
	// with. rows always covers the document: with wrap off it is the
	// identity mapping (one row per logical line).
	rows      []visualRow
	rowsWidth int
	rowsValid bool
}

// visualRow maps one painted row to a rune range of a logical line.
type visualRow struct {
	line     int // logical line index
	startCol int // first rune column (inclusive)
	endCol   int // end rune column (exclusive)
}

// SetWrap toggles soft wrapping; the default is on.
func (t *TextArea) SetWrap(on bool) {
	if t.wrap == on {
		return
	}
	t.wrap = on
	t.rowsValid = false
	t.hasPref = false
}

// SetIndent sets how many spaces Tab inserts while focused; zero (the
// default) inserts a tab character.
func (t *TextArea) SetIndent(n int) { t.indent = n }

// TrapTab implements the tab-trap rule: a plain Tab inside the area
// inserts indentation instead of moving focus. Ctrl and shift variants
// are routed to focus movement before this is asked.
func (t *TextArea) TrapTab(bool) bool {
	if t.indent > 0 {
		t.Insert(strings.Repeat(" ", t.indent))
	} else {
		t.Insert("\t")
	}
	return true
}

// NewTextArea returns an empty area painted with face at sizePx.
func NewTextArea(face *render.Typeface, sizePx float64, color render.Color) *TextArea {
	return &TextArea{
		face:   face,
		sizePx: sizePx,
		color:  color,
		lines:  [][]rune{{}},
		wrap:   true,
	}
}

// SetPlaceholder sets the text shown when the area is empty.
func (t *TextArea) SetPlaceholder(s string) { t.placeholder = s }

// pos is a line/column cursor or anchor position in the logical
// document.
type pos struct {
	line, col int
}

// Text returns the contents, lines joined with newlines.
// CursorName reports the text caret shape while hovered.
func (t *TextArea) CursorName() string { return "xterm" }

func (t *TextArea) Text() string {
	parts := make([]string, len(t.lines))
	for i, l := range t.lines {
		parts[i] = string(l)
	}
	return strings.Join(parts, "\n")
}

// SetText replaces the contents, splitting on newlines, and clears the
// selection and composing display.
func (t *TextArea) SetText(s string) {
	t.clearPreedit()
	t.lines = nil
	for part := range strings.SplitSeq(s, "\n") {
		t.lines = append(t.lines, []rune(part))
	}
	if len(t.lines) == 0 {
		t.lines = [][]rune{{}}
	}
	t.cursor = pos{0, 0}
	t.anchor = t.cursor
	t.hasPref = false
	t.rowsValid = false
}

// SetCursor places the cursor and anchor at a line/column, clearing any
// selection. Columns beyond the line clamp.
func (t *TextArea) SetCursor(line, col int) {
	t.clearPreedit()
	t.cursor = t.clamp(pos{line, col})
	t.anchor = t.cursor
	t.hasPref = false
}

// clamp keeps p inside the document.
func (t *TextArea) clamp(p pos) pos {
	if p.line < 0 {
		p.line = 0
	}
	if p.line >= len(t.lines) {
		p.line = len(t.lines) - 1
	}
	if p.col < 0 {
		p.col = 0
	}
	if p.col > len(t.lines[p.line]) {
		p.col = len(t.lines[p.line])
	}
	return p
}

// ordered returns the selection as start-before-end.
func (t *TextArea) ordered() (pos, pos) {
	if t.cursor.line < t.anchor.line || (t.cursor.line == t.anchor.line && t.cursor.col < t.anchor.col) {
		return t.cursor, t.anchor
	}
	return t.anchor, t.cursor
}

// Selection reports the selected rune range in line/col pairs.
func (t *TextArea) Selection() (start, end pos, active bool) {
	start, end = t.ordered()
	return start, end, start != end
}

// SelectedText implements SelectedTexter.
func (t *TextArea) SelectedText() (string, bool) {
	start, end, active := t.Selection()
	if !active {
		return "", false
	}
	var b strings.Builder
	for l := start.line; l <= end.line; l++ {
		from, to := 0, len(t.lines[l])
		if l == start.line {
			from = start.col
		}
		if l == end.line {
			to = end.col
		}
		if l > start.line {
			b.WriteByte('\n')
		}
		b.WriteString(string(t.lines[l][from:to]))
	}
	return b.String(), true
}

// collapse removes the selected text and leaves both ends at its start.
func (t *TextArea) collapse() {
	t.clearPreedit()
	start, end, active := t.Selection()
	if !active {
		t.cursor, t.anchor = t.clamp(t.cursor), t.clamp(t.cursor)
		return
	}
	head := append([]rune{}, t.lines[start.line][:start.col]...)
	tail := append([]rune{}, t.lines[end.line][end.col:]...)
	merged := append(head, tail...)
	rest := append([][]rune{}, t.lines[:start.line]...)
	rest = append(rest, merged)
	rest = append(rest, t.lines[end.line+1:]...)
	t.lines = rest
	t.cursor, t.anchor = start, start
	t.rowsValid = false
}

// TextLen returns the number of logical lines, for tests and callers.
func (t *TextArea) TextLen() int { return len(t.lines) }

// runeWidth returns one rune's shaped advance.
func (t *TextArea) runeWidth(r rune) float64 {
	return t.face.Shape(string(r), t.sizePx).Advance()
}

// spanWidth returns the advance of a rune substring of line l.
func (t *TextArea) spanWidth(l, from, to int) float64 {
	return t.face.Shape(string(t.lines[l][from:to]), t.sizePx).Advance()
}

// ensureRows rebuilds the visual row cache for the given available
// width when the text or width changed since the last build. With wrap
// off, rows are the identity mapping. A row always keeps at least one
// rune, so an unbreakable token wider than the viewport wraps one rune
// at a time instead of disappearing.
func (t *TextArea) ensureRows(availW int) {
	if t.rowsValid && availW == t.rowsWidth {
		return
	}
	t.rows = t.rows[:0]
	for l := range t.lines {
		line := t.displayLine(l)
		if !t.wrap {
			t.rows = append(t.rows, visualRow{l, 0, len(line)})
			continue
		}
		start, x := 0, 0.0
		limit := float64(availW)
		for i, r := range line {
			adv := t.runeWidth(r)
			if i > start && x+adv > limit {
				t.rows = append(t.rows, visualRow{l, start, i})
				start = i
				x = 0
			}
			x += adv
		}
		t.rows = append(t.rows, visualRow{l, start, len(line)})
	}
	t.rowsWidth = availW
	t.rowsValid = true
}

// rowOf returns the visual row the caret renders on: at a wrap
// boundary (a column that ends one row and starts the next) it is the
// start of the following row, which keeps downward motion progressing
// and matches the caret rendering.
func (t *TextArea) rowOf(col pos) int {
	first, withStart, last := -1, -1, -1
	for i, r := range t.rows {
		if r.line != col.line {
			if last >= 0 {
				break
			}
			continue
		}
		if first < 0 {
			first = i
		}
		last = i
		if col.col >= r.startCol {
			withStart = i
		}
	}
	if withStart >= 0 {
		return withStart
	}
	if first >= 0 {
		return first
	}
	return len(t.rows) - 1
}

// Insert inserts s at the cursor; an active selection is replaced.
func (t *TextArea) Insert(s string) {
	t.clearPreedit()
	t.collapse()
	for _, r := range s {
		switch r {
		case '\n':
			line := t.lines[t.cursor.line]
			head := append([]rune{}, line[:t.cursor.col]...)
			tail := append([]rune{}, line[t.cursor.col:]...)
			rest := append([][]rune{}, t.lines[:t.cursor.line]...)
			rest = append(rest, head, tail)
			rest = append(rest, t.lines[t.cursor.line+1:]...)
			t.lines = rest
			t.cursor.line++
			t.cursor.col = 0
		default:
			line := t.lines[t.cursor.line]
			line = append(line, 0)
			copy(line[t.cursor.col+1:], line[t.cursor.col:])
			line[t.cursor.col] = r
			t.lines[t.cursor.line] = line
			t.cursor.col++
		}
	}
	t.anchor = t.cursor
	t.hasPref = false
	t.rowsValid = false
}

// Delete removes the selection, or one rune/line break forward.
func (t *TextArea) Delete() {
	t.clearPreedit()
	if _, _, active := t.Selection(); active {
		t.collapse()
		return
	}
	c := t.clamp(t.cursor)
	line := t.lines[c.line]
	switch {
	case c.col < len(line):
		t.lines[c.line] = append(line[:c.col], line[c.col+1:]...)
	case c.line < len(t.lines)-1:
		next := t.lines[c.line+1]
		t.lines[c.line] = append(append([]rune{}, line...), next...)
		t.lines = append(t.lines[:c.line+1], t.lines[c.line+2:]...)
	}
	t.anchor = t.clamp(t.cursor)
	t.rowsValid = false
}

// Backspace removes the selection, or one rune/line break backward.
func (t *TextArea) Backspace() {
	t.clearPreedit()
	if _, _, active := t.Selection(); active {
		t.collapse()
		return
	}
	c := t.clamp(t.cursor)
	switch {
	case c.col > 0:
		t.cursor.col--
	case c.line > 0:
		t.cursor.line--
		t.cursor.col = len(t.lines[t.cursor.line])
	default:
		return
	}
	t.anchor = t.clamp(t.cursor)
	t.Delete()
}

// CursorPos returns the cursor as line/column.
func (t *TextArea) CursorPos() (line, col int) { return t.cursor.line, t.cursor.col }

// move collapses the selection, then moves the cursor without touching
// the anchor when extend is set.
func (t *TextArea) move(delta pos, extend bool) {
	t.clearPreedit()
	if !extend {
		if _, _, active := t.Selection(); active {
			start, _ := t.ordered()
			t.cursor, t.anchor = start, start
			if delta == (pos{}) {
				return
			}
		}
	}
	c := t.clamp(t.cursor)
	c.line += delta.line
	c.col += delta.col
	t.cursor = t.clamp(c)
	if !extend {
		t.anchor = t.cursor
	}
	t.hasPref = false
}

// moveVertical moves the cursor one visual row up or down, honoring
// the preferred visual column set by a prior horizontal motion or
// click. With wrap on, a visual row is a wrapped segment of a logical
// line.
func (t *TextArea) moveVertical(dline int, extend bool) {
	t.clearPreedit()
	if _, _, active := t.Selection(); active && !extend {
		start, _ := t.ordered()
		t.cursor, t.anchor = start, start
	}
	c := t.clamp(t.cursor)
	if !t.hasPref {
		t.prefX = t.spanWidth(c.line, 0, c.col)
		t.hasPref = true
	}
	if t.wrap {
		t.ensureRows(t.wrapWidth())
		row := max(min(t.rowOf(c)+dline, len(t.rows)-1), 0)
		r := t.rows[row]
		t.cursor = pos{r.line, r.startCol + t.caretIn(r, t.prefX)}
	} else {
		nl := max(min(c.line+dline, len(t.lines)-1), 0)
		t.cursor = pos{nl, t.colForX(nl, t.prefX)}
	}
	if !extend {
		t.anchor = t.cursor
	}
}

// wrapWidth returns the pixel width available for wrapping inside the
// current bounds.
func (t *TextArea) wrapWidth() int {
	if t.bounds.W < 16 {
		return 0
	}
	return t.bounds.W - 16
}

// caretIn maps a visual x offset to a column within the rune range of
// one visual row, returning a row-relative column.
func (t *TextArea) caretIn(r visualRow, x float64) int {
	line := t.lines[r.line]
	k := t.face.Shape(string(line[r.startCol:r.endCol]), t.sizePx).CaretAt(x)
	return min(k, r.endCol-r.startCol)
}

// colForX maps a visual x offset within line l to a rune column.
func (t *TextArea) colForX(l int, x float64) int {
	if l < 0 || l >= len(t.lines) {
		return 0
	}
	return t.face.Shape(string(t.lines[l]), t.sizePx).CaretAt(x)
}

// Measure reports the natural widest-line size, or, when wrapping and
// a width is offered, the offered width by the wrapped row count.
// Composing text counts toward the widest line and the row count.
func (t *TextArea) Measure(con Constraints) Size {
	lineH := t.face.Shape("lg", t.sizePx).LineHeight()
	if t.wrap && con.Max.W > 16 {
		t.ensureRows(con.Max.W - 16)
		return clampSize(Size{W: con.Max.W, H: lineH*len(t.rows) + 12}, con)
	}
	w := 16
	for l := range t.lines {
		if adv := int(t.face.Shape(string(t.displayLine(l)), t.sizePx).Advance() + 0.5); adv > w {
			w = adv
		}
	}
	w += 16
	h := lineH*len(t.lines) + 12
	return clampSize(Size{W: w, H: h}, con)
}

// lineHeight returns the integer line height of the font.
func (t *TextArea) lineHeight() int {
	return t.face.Shape("lg", t.sizePx).LineHeight()
}

// Paint draws the wrapped rows, the selection highlight, the composing
// text with its underline, and the cursor.
func (t *TextArea) Paint(cv *render.Canvas) {
	th := Current()
	cv.RoundedRect(t.bounds, th.Radius, th.Surface)
	lineH := t.lineHeight()
	start, end, active := t.Selection()
	t.ensureRows(t.wrapWidth())

	if len(t.lines) == 1 && len(t.lines[0]) == 0 && !t.composing() && t.placeholder != "" {
		t.face.DrawAligned(cv, t.placeholder, t.bounds, t.sizePx, th.Border, render.AlignStart)
		return
	}
	for i, r := range t.rows {
		line := t.displayLine(r.line)
		y := 6 + i*lineH
		if len(line) == 0 || r.startCol >= r.endCol {
			continue
		}
		// Selection band for the portion of this row inside the
		// selection.
		if active {
			from, to := r.startCol, r.endCol
			if r.line == start.line {
				from = max(from, start.col)
			}
			if r.line == end.line {
				to = min(to, end.col)
			}
			if from < to {
				sel := th.Accent
				hl := render.RGBA(sel.R(), sel.G(), sel.B(), 90)
				x0 := 8 + int(t.spanWidth(r.line, r.startCol, from)+0.5)
				x1 := 8 + int(t.spanWidth(r.line, r.startCol, to)+0.5)
				cv.FillRect(render.Rect{X: t.bounds.X + x0, Y: t.bounds.Y + y, W: x1 - x0, H: lineH}, hl)
			}
		}
		box := render.Rect{X: t.bounds.X + 8, Y: t.bounds.Y + y, W: t.bounds.W - 16, H: lineH}
		prev := cv.PushClip(box)
		t.face.DrawAligned(cv, string(line[r.startCol:r.endCol]), box, t.sizePx, t.color, render.AlignStart)
		cv.PopClip(prev)
	}
	if t.composing() {
		// Accent underline under the composing range, across every
		// visual row it touches.
		pb, pe := t.peAt.col, t.peAt.col+len(t.peText)
		a := th.Accent
		ul := render.RGBA(a.R(), a.G(), a.B(), 200)
		for i, r := range t.rows {
			if r.line != t.peAt.line || r.endCol <= pb || r.startCol >= pe {
				continue
			}
			x0 := 8 + int(t.spanWidthDisp(r.line, r.startCol, max(r.startCol, pb))+0.5)
			x1 := 8 + int(t.spanWidthDisp(r.line, r.startCol, min(r.endCol, pe))+0.5)
			y := 6 + i*lineH
			cv.FillRect(render.Rect{X: t.bounds.X + x0, Y: t.bounds.Y + y + lineH - 4, W: max(x1-x0, 2), H: 2}, ul)
		}
	}
	// Cursor bar on the caret's visual row; hidden while the input
	// method hides its composing caret.
	caret := t.caretPos()
	if !t.composing() || t.peCur >= 0 {
		row := t.rowOf(caret)
		crow := t.rows[row]
		x := 8 + int(t.spanWidthDisp(crow.line, crow.startCol, caret.col)+0.5)
		y := 6 + row*lineH
		cv.FillRect(render.Rect{X: t.bounds.X + x, Y: t.bounds.Y + y + 2, W: 2, H: lineH - 4}, t.color)
	}
}

// HitTest returns the area when p is inside its bounds.
func (t *TextArea) HitTest(p Point) Widget { return t.HitLeaf(t, p) }

// posAt maps a root-space point to a document position, resolving
// through the visual rows when wrapping.
func (t *TextArea) posAt(p Point) pos {
	lineH := t.lineHeight()
	t.ensureRows(t.wrapWidth())
	row := max(min((p.Y-t.bounds.Y-6)/lineH, len(t.rows)-1), 0)
	r := t.rows[row]
	if !t.wrap {
		return pos{line: r.line, col: t.colForX(r.line, float64(p.X-t.bounds.X-8))}
	}
	col := r.startCol + t.caretIn(r, float64(p.X-t.bounds.X-8))
	return pos{line: r.line, col: col}
}

// ClickAt places the cursor (and anchor) at the clicked position,
// dropping the composing display.
func (t *TextArea) ClickAt(p Point) {
	t.clearPreedit()
	t.cursor = t.posAt(p)
	t.anchor = t.cursor
	t.hasPref = false
}

// DragMove extends the selection to the dragged position, dropping the
// composing display.
func (t *TextArea) DragMove(p Point) {
	t.clearPreedit()
	t.cursor = t.posAt(p)
	t.hasPref = false
}

// DoubleClickAt selects the same-class run under the pointer.
func (t *TextArea) DoubleClickAt(p Point) {
	t.clearPreedit()
	at := t.posAt(p)
	line := t.lines[at.line]
	if len(line) == 0 {
		t.cursor, t.anchor = at, at
		return
	}
	col := at.col
	if col >= len(line) {
		col = len(line) - 1
	}
	word := func(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
	want := word(line[col])
	start := col
	for start > 0 && word(line[start-1]) == want {
		start--
	}
	end := col + 1
	for end < len(line) && word(line[end]) == want {
		end++
	}
	t.cursor, t.anchor = pos{at.line, end}, pos{at.line, start}
	t.hasPref = false
}

// SelectAll selects the entire document.
func (t *TextArea) SelectAll() {
	t.clearPreedit()
	t.anchor = pos{0, 0}
	t.cursor = pos{len(t.lines) - 1, len(t.lines[len(t.lines)-1])}
	t.hasPref = false
}

// InsertRune implements RuneHandler.
func (t *TextArea) InsertRune(r rune) { t.Insert(string(r)) }

// KeyAction implements KeyActionHandler. While composing, the input
// method owns the text: backspace trims its last rune, and every other
// edit drops the composing display and applies to the contents.
func (t *TextArea) KeyAction(a KeyAction, mods Mods) {
	shift := mods&ModShift != 0
	if a == KeyBackspace && t.composing() {
		t.peText = t.peText[:len(t.peText)-1]
		t.peCur = min(t.peCur, len(t.peText))
		if len(t.peText) == 0 {
			t.clearPreedit()
		}
		return
	}
	// Every action except the trim above drops the composing display.
	t.clearPreedit()
	switch a {
	case KeyBackspace:
		t.Backspace()
	case KeyDelete:
		t.Delete()
	case KeyLeft:
		c := t.clamp(t.cursor)
		if !shift {
			if _, _, active := t.Selection(); active {
				start, _ := t.ordered()
				t.cursor, t.anchor = start, start
				return
			}
		}
		switch {
		case c.col > 0:
			t.move(pos{col: -1}, shift)
		case c.line > 0:
			t.cursor = t.clamp(pos{c.line - 1, len(t.lines[c.line-1])})
			if !shift {
				t.anchor = t.cursor
			}
			t.hasPref = false
		}
	case KeyRight:
		c := t.clamp(t.cursor)
		if !shift {
			if _, _, active := t.Selection(); active {
				end := t.cursor
				if t.cursor.line < t.anchor.line || (t.cursor.line == t.anchor.line && t.cursor.col < t.anchor.col) {
					end = t.anchor
				}
				t.cursor, t.anchor = end, end
				return
			}
		}
		switch {
		case c.col < len(t.lines[c.line]):
			t.move(pos{col: 1}, shift)
		case c.line < len(t.lines)-1:
			t.cursor = t.clamp(pos{c.line + 1, 0})
			if !shift {
				t.anchor = t.cursor
			}
			t.hasPref = false
		}
	case KeyUp:
		t.moveVertical(-1, shift)
	case KeyDown:
		t.moveVertical(1, shift)
	case KeyHome:
		if t.wrap {
			t.ensureRows(t.wrapWidth())
			t.cursor.col = t.rows[t.rowOf(t.clamp(t.cursor))].startCol
		} else {
			t.cursor.col = 0
		}
		t.cursor = t.clamp(t.cursor)
		if !shift {
			t.anchor = t.cursor
		}
		t.hasPref = false
	case KeyEnd:
		if t.wrap {
			t.ensureRows(t.wrapWidth())
			t.cursor.col = t.rows[t.rowOf(t.clamp(t.cursor))].endCol
		} else {
			t.cursor.col = len(t.lines[t.clamp(t.cursor).line])
		}
		if !shift {
			t.anchor = t.cursor
		}
		t.hasPref = false
	case KeyEnter:
		t.Insert("\n")
	}
}
