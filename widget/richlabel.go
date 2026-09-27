package widget

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/stubbedev/gelm/render"
)

// VariantFunc returns the face a bold or italic run shapes with. It
// must never return nil; returning the base face for a variant the
// platform lacks is the expected downgrade and renders fine.
type VariantFunc func(bold, italic bool) *render.Typeface

// BaseVariants serves base for every style. Use it when only one face
// is loaded: markup bold and italic then render with the base face.
// gelm never synthesizes faux bold or italic outlines. Apps that load
// real variants wire them here, e.g. via internal/sysfont's Variant.
func BaseVariants(base *render.Typeface) VariantFunc {
	return func(bool, bool) *render.Typeface { return base }
}

// RichLabel paints a single line of styled text driven by the minimal
// markup subset ParseMarkup accepts; see that function for the grammar
// and the escaping rules. Input ParseMarkup rejects renders literally,
// tags and all, so malformed markup can never panic or break a layout.
//
// Text is shaped once per run and drawn on one shared baseline; the
// line height is the maximum over the runs, so a taller face grows the
// box instead of misaligning its neighbors. Links (<a href=...>) are
// opt-in: they underline in the run's color, and installing
// OnLinkClick makes them clickable with a hand cursor.
type RichLabel struct {
	node
	base     render.Font
	variants VariantFunc
	markup   string
	sizePx   float64
	color    render.Color
	align    render.Alignment

	// runs is the parsed (or literal fallback) run list; shaped
	// mirrors it with per-run shaping and metrics.
	runs   []MarkupRun
	shaped []*richRun
	// lineAsc and lineDesc are the line's max ascent and descent,
	// advance the total line width, runes the rune count of text.
	lineAsc, lineDesc, advance float64
	runes                      int
	text                       string
	natural                    Size

	// selStart/selEnd bound the highlighted rune range while selOn.
	selStart, selEnd int
	selOn            bool

	// hoverP is the last pointer position while hovered; hoverValid
	// gates link cursor lookups.
	hoverP     Point
	hoverValid bool

	// OnLinkClick fires when a press-release lands on a link run.
	OnLinkClick func(href string)
}

// richRun is one shaped markup run.
type richRun struct {
	sh    *render.ShapedText
	face  render.Font
	style TextStyle
	start int // rune offset of the run's first rune within Text()
}

// NewRichLabel returns a rich label that paints markup with base at
// sizePx pixels in color. Styled runs shape with the base face until
// SetVariants provides real variants.
func NewRichLabel(base *render.Typeface, markup string, sizePx float64, color render.Color) *RichLabel {
	l := &RichLabel{base: base, markup: markup, sizePx: sizePx, color: color}
	l.retext()
	return l
}

// SetVariants installs the face variants bold and italic runs shape
// with, and re-shapes the label. Plain runs always shape with the base
// face.
func (l *RichLabel) SetVariants(v VariantFunc) {
	l.variants = v
	l.retext()
	l.InvalidateLayout()
}

// SetMarkup replaces the label markup and invalidates its bounds.
// Markup ParseMarkup rejects renders literally.
func (l *RichLabel) SetMarkup(markup string) {
	if l.markup == markup {
		return
	}
	l.markup = markup
	l.retext()
	l.InvalidateLayout()
}

// Markup returns the current markup string.
func (l *RichLabel) Markup() string { return l.markup }

// Text returns the label's decoded plain text: the concatenated run
// text, or the raw input when the markup was rejected.
func (l *RichLabel) Text() string { return l.text }

// SetAlignment selects horizontal placement when the arranged rect is
// wider than the line.
func (l *RichLabel) SetAlignment(a render.Alignment) {
	if l.align == a {
		return
	}
	l.align = a
	l.Invalidate()
}

// faceFor resolves the face a run shapes with: bold and italic runs
// use the variants when one is installed, and every other run - and
// any variant the provider cannot supply - uses the base face.
func (l *RichLabel) faceFor(st TextStyle) render.Font {
	if l.variants != nil && (st.Bold || st.Italic) {
		if f := l.variants(st.Bold, st.Italic); f != nil {
			return f
		}
	}
	return l.base
}

// retext re-parses the markup, reshapes every run, and refreshes the
// cached natural size. Runs rejected by the parser render literally.
func (l *RichLabel) retext() {
	runs, ok := ParseMarkup(l.markup)
	if !ok {
		runs = []MarkupRun{{Text: l.markup}}
	}
	l.runs = runs
	l.shaped = make([]*richRun, 0, len(runs))
	var sb strings.Builder
	asc, desc, adv := 0.0, 0.0, 0.0
	runeOff := 0
	for _, r := range runs {
		sb.WriteString(r.Text)
		face := l.faceFor(r.Style)
		sh := face.Shape(r.Text, l.sizePx)
		asc = math.Max(asc, sh.Ascent())
		desc = math.Max(desc, sh.Descent())
		adv += sh.Advance()
		l.shaped = append(l.shaped, &richRun{sh: sh, face: face, style: r.Style, start: runeOff})
		runeOff += utf8.RuneCountInString(r.Text)
	}
	if len(l.shaped) == 0 {
		// Empty text still reserves the font's line height, like
		// Label.
		sh := l.faceFor(TextStyle{}).Shape("", l.sizePx)
		asc, desc = sh.Ascent(), sh.Descent()
	}
	l.lineAsc, l.lineDesc, l.advance = asc, desc, adv
	l.runes = runeOff
	l.text = sb.String()
	l.natural = Size{W: int(adv + 0.5), H: int(math.Ceil(asc + desc))}
}

// Measure returns the line's total advance and its line height - the
// maximum over the runs - clamped to con.
func (l *RichLabel) Measure(con Constraints) Size {
	if s, ok := l.measureHit(con); ok {
		return s
	}
	return l.measureStore(con, clampSize(l.natural, con))
}

// lineGeom resolves the aligned line origin and the runs' shared
// baseline. ok is false when the arranged box cannot hold one line,
// the same guard a plain label's DrawAligned applies.
func (l *RichLabel) lineGeom() (lineX float64, baseline int, ok bool) {
	lineH := l.natural.H
	if l.bounds.H < lineH {
		return 0, 0, false
	}
	switch l.align {
	case render.AlignCenter:
		lineX = float64(l.bounds.X) + (float64(l.bounds.W)-l.advance)/2
	case render.AlignEnd:
		lineX = float64(l.bounds.X) + float64(l.bounds.W) - l.advance
	default:
		lineX = float64(l.bounds.X)
	}
	baseline = l.bounds.Y + int(math.Round((float64(l.bounds.H)-float64(lineH))/2+l.lineAsc))
	return lineX, baseline, true
}

// Paint draws the selection highlight, then each run on the shared
// baseline: its variant face, its color (a span color wins over the
// label color), and an underline for link runs.
func (l *RichLabel) Paint(cv *render.Canvas) {
	if start, end, on := l.selection(); on {
		a := Current().Accent
		hl := render.RGBA(a.R(), a.G(), a.B(), 90)
		for _, b := range l.SelectionBands(start, end) {
			cv.FillRect(b, hl)
		}
	}
	lineX, baseline, ok := l.lineGeom()
	if !ok {
		return
	}
	x := lineX
	for _, r := range l.shaped {
		dx := int(math.Round(x))
		col := l.color
		if r.style.Color != 0 {
			col = r.style.Color
		}
		r.face.Draw(cv, r.sh, dx, baseline, col)
		if r.style.Href != "" {
			under := render.Rect{X: dx, Y: baseline + 1, W: int(r.sh.Advance() + 0.5), H: max(1, int(l.sizePx/14))}
			cv.FillRect(under, col)
		}
		x += r.sh.Advance()
	}
}

// selection returns the ordered, clamped selected range and whether
// one is active.
func (l *RichLabel) selection() (start, end int, on bool) {
	if !l.selOn {
		return 0, 0, false
	}
	start, end = l.selStart, l.selEnd
	if start > end {
		start, end = end, start
	}
	start = max(0, min(start, l.runes))
	end = max(0, min(end, l.runes))
	return start, end, start < end
}

// SetSelection highlights the rune range [start, end) of Text(),
// clamped; an empty range paints nothing.
func (l *RichLabel) SetSelection(start, end int) {
	l.selStart, l.selEnd, l.selOn = start, end, true
	l.Invalidate()
}

// ClearSelection drops the highlight.
func (l *RichLabel) ClearSelection() {
	if !l.selOn {
		return
	}
	l.selOn = false
	l.Invalidate()
}

// SelectionBands returns the selection rectangles covering runes
// [start, end) of Text(), in root coordinates: one band per run the
// range touches, spanning the widget's full height. Rich labels have
// no editing caret; bands are the whole of their selection geometry.
func (l *RichLabel) SelectionBands(start, end int) []render.Rect {
	start = max(0, min(start, l.runes))
	end = max(0, min(end, l.runes))
	if start >= end {
		return nil
	}
	lineX, _, ok := l.lineGeom()
	if !ok {
		return nil
	}
	var bands []render.Rect
	x := lineX
	for _, r := range l.shaped {
		n := utf8.RuneCountInString(r.sh.Text())
		runX := x
		x += r.sh.Advance()
		// Clamp the range into the run's rune span.
		lo := min(max(start, r.start), r.start+n) - r.start
		hi := min(max(end, r.start), r.start+n) - r.start
		if lo >= hi {
			continue
		}
		cs := r.sh.CaretPositions()
		x0 := int(math.Round(runX + cs[lo]))
		x1 := int(math.Round(runX + cs[hi]))
		if x1 <= x0 {
			continue
		}
		bands = append(bands, render.Rect{X: x0, Y: l.bounds.Y, W: x1 - x0, H: l.bounds.H})
	}
	return bands
}

// linkAt returns the href of the link run under p, in root
// coordinates.
func (l *RichLabel) linkAt(p Point) (string, bool) {
	if !l.bounds.Contains(p.X, p.Y) {
		return "", false
	}
	lineX, _, ok := l.lineGeom()
	if !ok {
		return "", false
	}
	x := lineX
	for _, r := range l.shaped {
		w := r.sh.Advance()
		if r.style.Href != "" && float64(p.X) >= x && float64(p.X) < x+w {
			return r.style.Href, true
		}
		x += w
	}
	return "", false
}

// ClickAt fires OnLinkClick when the click lands on a link run.
func (l *RichLabel) ClickAt(p Point) {
	if href, ok := l.linkAt(p); ok && l.OnLinkClick != nil {
		l.OnLinkClick(href)
	}
}

// HoverMove records the pointer position for link lookups.
func (l *RichLabel) HoverMove(p Point) {
	l.hoverP, l.hoverValid = p, true
}

// SetHovered forgets the pointer position on leave.
func (l *RichLabel) SetHovered(on bool) {
	if !on {
		l.hoverValid = false
	}
}

// CursorName asks for the pointing hand while the pointer sits on a
// link run.
func (l *RichLabel) CursorName() string {
	if l.hoverValid {
		if _, ok := l.linkAt(l.hoverP); ok {
			return "hand1"
		}
	}
	return ""
}

// Role implements Roleer.
func (l *RichLabel) Role() Role { return RoleLabel }

// HitTest returns the label when p is inside its bounds.
func (l *RichLabel) HitTest(p Point) Widget {
	return l.HitLeaf(l, p)
}
