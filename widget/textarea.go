package widget

import (
	"strings"
	"unicode"

	"github.com/stubbedev/gelm/render"
)

// TextArea is a multi-line text editor: logical lines split on newlines,
// per-line cursor motion with a sticky preferred column, and a selection
// model shared with Entry. Text wider than the box clips; there is no
// soft wrap yet.
type TextArea struct {
	node
	face        *render.Typeface
	sizePx      float64
	color       render.Color
	placeholder string

	lines   [][]rune
	cursor  pos
	anchor  pos
	prefX   float64 // preferred visual column for vertical motion
	hasPref bool
}

// pos is a line/column cursor or anchor position.
type pos struct {
	line, col int
}

// NewTextArea returns an empty area painted with face at sizePx.
func NewTextArea(face *render.Typeface, sizePx float64, color render.Color) *TextArea {
	return &TextArea{
		face:   face,
		sizePx: sizePx,
		color:  color,
		lines:  [][]rune{{}},
	}
}

// SetPlaceholder sets the text shown when the area is empty.
func (t *TextArea) SetPlaceholder(s string) { t.placeholder = s }

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
// selection.
func (t *TextArea) SetText(s string) {
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
}

// SetCursor places the cursor and anchor at a line/column, clearing any
// selection. Columns beyond the line clamp.
func (t *TextArea) SetCursor(line, col int) {
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
}

// TextLen returns the number of logical lines, for tests and callers.
func (t *TextArea) TextLen() int { return len(t.lines) }

// Insert inserts s at the cursor; an active selection is replaced.
func (t *TextArea) Insert(s string) {
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
}

// Delete removes the selection, or one rune/line break forward.
func (t *TextArea) Delete() {
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
}

// Backspace removes the selection, or one rune/line break backward.
func (t *TextArea) Backspace() {
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

// moveVertical moves the cursor line-wise, honoring the preferred
// visual column set by a prior horizontal motion or click.
func (t *TextArea) moveVertical(dline int, extend bool) {
	if _, _, active := t.Selection(); active && !extend {
		start, _ := t.ordered()
		t.cursor, t.anchor = start, start
	}
	c := t.clamp(t.cursor)
	if !t.hasPref {
		t.prefX = t.face.Shape(string(t.lines[c.line][:c.col]), t.sizePx).Advance()
		t.hasPref = true
	}
	nl := max(min(c.line+dline, len(t.lines)-1), 0)
	t.cursor = pos{nl, t.colForX(nl, t.prefX)}
	if !extend {
		t.anchor = t.cursor
	}
}

// colForX maps a visual x offset within line l to a rune column.
func (t *TextArea) colForX(l int, x float64) int {
	if l < 0 || l >= len(t.lines) {
		return 0
	}
	return t.face.Shape(string(t.lines[l]), t.sizePx).CaretAt(x)
}

// Measure wants the widest line's advance by the total line height.
func (t *TextArea) Measure(con Constraints) Size {
	lineH := t.face.Shape("lg", t.sizePx).LineHeight()
	w := 16
	for _, l := range t.lines {
		if adv := int(t.face.Shape(string(l), t.sizePx).Advance() + 0.5); adv > w {
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

// Paint draws the lines, the selection highlight, and the cursor.
func (t *TextArea) Paint(cv *render.Canvas) {
	th := Current()
	cv.RoundedRect(t.bounds, th.Radius, th.Surface)
	lineH := t.lineHeight()
	start, end, active := t.Selection()

	if len(t.lines) == 1 && len(t.lines[0]) == 0 && t.placeholder != "" {
		t.face.DrawAligned(cv, t.placeholder, t.bounds, t.sizePx, th.Border, render.AlignStart)
		return
	}
	if active {
		sel := th.Accent
		hl := render.RGBA(sel.R(), sel.G(), sel.B(), 90)
		for l := start.line; l <= end.line; l++ {
			from, to := 0, len(t.lines[l])
			if l == start.line {
				from = start.col
			}
			if l == end.line {
				to = end.col
			}
			x0 := 8 + int(t.face.Shape(string(t.lines[l][:from]), t.sizePx).Advance()+0.5)
			x1 := 8 + int(t.face.Shape(string(t.lines[l][:to]), t.sizePx).Advance()+0.5)
			y := 6 + l*lineH
			cv.FillRect(render.Rect{X: t.bounds.X + x0, Y: t.bounds.Y + y, W: x1 - x0, H: lineH}, hl)
		}
	}
	for l, line := range t.lines {
		if len(line) == 0 {
			continue
		}
		y := 6 + l*lineH
		box := render.Rect{X: t.bounds.X + 8, Y: t.bounds.Y + y, W: t.bounds.W - 16, H: lineH}
		prev := cv.PushClip(box)
		t.face.DrawAligned(cv, string(line), box, t.sizePx, t.color, render.AlignStart)
		cv.PopClip(prev)
	}
	// Cursor bar at the cursor position.
	x := 8 + int(t.face.Shape(string(t.lines[t.cursor.line][:t.cursor.col]), t.sizePx).Advance()+0.5)
	y := 6 + t.cursor.line*lineH
	cv.FillRect(render.Rect{X: t.bounds.X + x, Y: t.bounds.Y + y + 2, W: 2, H: lineH - 4}, t.color)
}

// HitTest returns the area when p is inside its bounds.
func (t *TextArea) HitTest(p Point) Widget { return t.HitLeaf(t, p) }

// posAt maps a root-space point to a document position.
func (t *TextArea) posAt(p Point) pos {
	lineH := t.lineHeight()
	l := max(min((p.Y-t.bounds.Y-6)/lineH, len(t.lines)-1), 0)
	return pos{line: l, col: t.colForX(l, float64(p.X-t.bounds.X-8))}
}

// ClickAt places the cursor (and anchor) at the clicked position.
func (t *TextArea) ClickAt(p Point) {
	t.cursor = t.posAt(p)
	t.anchor = t.cursor
	t.hasPref = false
}

// DragMove extends the selection to the dragged position.
func (t *TextArea) DragMove(p Point) {
	t.cursor = t.posAt(p)
	t.hasPref = false
}

// DoubleClickAt selects the same-class run under the pointer.
func (t *TextArea) DoubleClickAt(p Point) {
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
	t.anchor = pos{0, 0}
	t.cursor = pos{len(t.lines) - 1, len(t.lines[len(t.lines)-1])}
	t.hasPref = false
}

// InsertRune implements RuneHandler.
func (t *TextArea) InsertRune(r rune) { t.Insert(string(r)) }

// KeyAction implements KeyActionHandler.
func (t *TextArea) KeyAction(a KeyAction, mods Mods) {
	shift := mods&ModShift != 0
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
		t.cursor.col = 0
		t.cursor = t.clamp(t.cursor)
		if !shift {
			t.anchor = t.cursor
		}
		t.hasPref = false
	case KeyEnd:
		t.cursor.col = len(t.lines[t.clamp(t.cursor).line])
		if !shift {
			t.anchor = t.cursor
		}
		t.hasPref = false
	case KeyEnter:
		t.Insert("\n")
	}
}
