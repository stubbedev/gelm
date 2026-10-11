package widget

import (
	"math"
	"strings"
	"unicode"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// EllipsizeMode names where an overflowing label truncates. It is
// render.EllipsizeMode; the alias lets label call sites read against
// the widget package.
type EllipsizeMode = render.EllipsizeMode

const (
	// EllipsizeNone never truncates: long text overflows and clips.
	EllipsizeNone = render.EllipsizeNone
	// EllipsizeStart cuts from the front: "…cated text".
	EllipsizeStart = render.EllipsizeStart
	// EllipsizeMiddle keeps the head and the tail: "trunc…text" - the
	// mode for paths and filenames.
	EllipsizeMiddle = render.EllipsizeMiddle
	// EllipsizeEnd cuts from the back: "truncated te…".
	EllipsizeEnd = render.EllipsizeEnd
)

// Label paints text. It is passive: it hits as itself but has no
// interaction behavior.
//
// The default is one line whose text overflows a narrow arrangement and
// clips. SetWrap turns on word wrapping at the offered width - Measure
// then reports one line height per wrapped row - and SetEllipsize
// truncates with an ellipsis instead, at the start, middle, or end.
// With both on, wrapping fills rows and the ellipsis applies to the
// final row only (GTK's rule): a lone unbreakable token left
// overflowing there truncates rather than spilling. A label too small
// for its text is the classic tooltip case: keep the full text in
// SetTooltip and let the mode shorten what paints.
type Label struct {
	node
	face render.Font
	// faces memoizes the cascade-styled face (styledFace).
	faces  faceCache
	text   string
	sizePx float64
	color  render.Color
	align  render.Alignment
	dir    Direction
	wrap   bool
	ell    EllipsizeMode
	// maxChars caps the natural width at that many approximate
	// character widths (GTK max-width-chars); 0 is uncapped.
	maxChars int
	// widthChars floors the natural width at that many approximate
	// character widths (GTK width-chars); 0 is unset.
	widthChars int
	// maxLines caps a wrapping, ellipsizing label at that many rows
	// (GTK lines); 0 is uncapped.
	maxLines int
	shaped   *render.ShapedText
	natural  Size
	// measured is the last Measure result, what ShrinkableWidth gives
	// from.
	measured Size
}

// NewLabel returns a label that paints text with face at sizePx
// pixels in color. Face may be a render.Chain for mixed-script
// fallback. Constructor argument order is the toolkit-wide one: face,
// then sizePx, then the content, then colors. A nil face panics here
// (see requireFace) instead of failing later, in shaping.
func NewLabel(face render.Font, sizePx float64, text string, color render.Color) *Label {
	l := &Label{face: requireFace("widget.NewLabel", face), sizePx: sizePx, text: text, color: color}
	l.retext()
	return l
}

// SetText replaces the label text and invalidates the label's bounds.
func (l *Label) SetText(text string) {
	if l.text == text {
		return
	}
	l.text = text
	l.retext()
	l.InvalidateLayout()
}

// Text returns the current label text.
func (l *Label) Text() string {
	return l.text
}

// SetFace swaps the shaping face (a fallback chain works here too);
// the shaped caches drop and the bounds invalidate.
func (l *Label) SetFace(face render.Font) {
	if l.face == face {
		return
	}
	l.face = requireFace("(*Label).SetFace", face)
	l.retext()
	l.InvalidateLayout()
}

// SizePx returns the shaping size in logical pixels.
func (l *Label) SizePx() float64 { return l.sizePx }

// SetSizePx changes the shaping size; the shaped caches drop and the
// bounds invalidate.
func (l *Label) SetSizePx(px float64) {
	if l.sizePx == px {
		return
	}
	l.sizePx = px
	l.retext()
	l.InvalidateLayout()
}

// SetAlignment selects horizontal placement when the arranged rect is
// wider than the text; wrapped rows align individually.
func (l *Label) SetAlignment(a render.Alignment) {
	if l.align == a {
		return
	}
	l.align = a
	l.Invalidate()
}

// Alignment returns the horizontal placement.
func (l *Label) Alignment() render.Alignment { return l.align }

// SetDirection selects the base paragraph direction the text resolves
// and lays out with. DirectionAuto (the default) reads it off the
// text's first strong character, so a Hebrew- or Arabic-first label
// mirrors on its own; DirectionLTR and DirectionRTL force the base,
// and start/end alignment mirrors with it. Changing the direction
// re-resolves the text and drops the measure cache.
func (l *Label) SetDirection(d Direction) {
	if l.dir == d {
		return
	}
	l.dir = d
	l.retext()
	l.InvalidateLayout()
}

// Direction returns the base paragraph direction the label resolves
// with.
func (l *Label) Direction() Direction { return l.dir }

// SetWrap toggles word wrapping at the offered width. Measure reports
// the widest wrapped row, one line height tall per row, breaking at
// Unicode UAX #14 opportunities (spaces, hyphens, CJK); a run with no
// opportunity before the width - an unbreakable token - keeps its own
// row and overflows, clipped, never disappearing. Changing the mode
// drops the measure cache.
func (l *Label) SetWrap(on bool) {
	if l.wrap == on {
		return
	}
	l.wrap = on
	l.InvalidateLayout()
}

// Wrap reports whether the label wraps at the offered width.
func (l *Label) Wrap() bool { return l.wrap }

// SetEllipsize selects where an overflowing line truncates with an
// ellipsis: at the end, at the start, in the middle (paths and
// filenames), or not at all. With wrap on, the ellipsis applies to the
// final row only; otherwise it applies to the single line. The label's
// text is never rewritten - pair the mode with SetTooltip to keep the
// whole string reachable. Changing the mode drops the measure cache.
func (l *Label) SetEllipsize(mode EllipsizeMode) {
	if l.ell == mode {
		return
	}
	l.ell = mode
	l.InvalidateLayout()
}

// Ellipsize returns the label's truncation mode.
func (l *Label) Ellipsize() EllipsizeMode { return l.ell }

// SetMaxWidthChars caps the label's natural width at n approximate
// character widths (GTK's max-width-chars): longer text overflows the
// cap and truncates by the ellipsize mode. n <= 0 removes the cap.
func (l *Label) SetMaxWidthChars(n int) {
	n = max(n, 0)
	if l.maxChars == n {
		return
	}
	l.maxChars = n
	l.InvalidateLayout()
}

// MaxWidthChars returns the width cap in characters, 0 when uncapped.
func (l *Label) MaxWidthChars() int { return l.maxChars }

// SetMaxLines limits a wrapping, ellipsizing label to n rows (GTK's
// set_lines): the text past row n folds into the final row, which the
// ellipsize mode truncates. Like GTK it has no effect unless the label
// both wraps and ellipsizes. n <= 0 removes the limit.
func (l *Label) SetMaxLines(n int) {
	n = max(n, 0)
	if l.maxLines == n {
		return
	}
	l.maxLines = n
	l.InvalidateLayout()
}

// MaxLines returns the row limit, 0 when unlimited.
func (l *Label) MaxLines() int { return l.maxLines }

// SetWidthChars floors the label's natural width at n approximate
// character widths (GTK's width-chars): a shorter text still measures
// that wide, so labels of one row line up. n <= 0 restores the text's
// own width.
func (l *Label) SetWidthChars(n int) {
	n = max(n, 0)
	if l.widthChars == n {
		return
	}
	l.widthChars = n
	l.InvalidateLayout()
}

// WidthChars returns the natural-width floor in characters, 0 when
// unset.
func (l *Label) WidthChars() int { return l.widthChars }

// approxCharSample stands in for pango's per-language sample text: the
// approximate character width is its mean advance.
const approxCharSample = "abcdefghijklmnopqrstuvwxyz0123456789"

// capWidth applies the max-width-chars cap to the offered width.
func (l *Label) capWidth(con Constraints) Constraints {
	if l.maxChars <= 0 {
		return con
	}
	face, px := l.effStyle()
	avg := face.Shape(approxCharSample, px).Advance() / float64(len(approxCharSample))
	limit := int(math.Ceil(avg * float64(l.maxChars)))
	if limit < con.Max.W {
		con.Max.W = max(limit, con.Min.W)
	}
	return con
}

// effStyleIn resolves the paint parameters one cascade value implies:
// the face (a font-family/font-weight declaration shaped through the
// installed face resolver, else the constructor face - a variable one
// taking the weight on its wght axis), its font-variation-settings and
// tnum twin (styledFace), and the size (font-size, else the
// constructor size).
func (l *Label) effStyleIn(v *style.Values) (render.Font, float64) {
	px := fontPx(v, l.sizePx)
	face, weight := l.face, 0
	if v.Has(style.PropFontWeight) {
		weight = v.FontWeight
	}
	if v.Has(style.PropFontFamily) || weight != 0 {
		family := ""
		if v.Has(style.PropFontFamily) {
			family = v.FontFamily
		}
		if f, ok := resolveFace(family, weight); ok {
			face, weight = f, 0 // the resolver served the weight
		}
	}
	return l.faces.get(face, v, weight, int(px)), px
}

// effStyle is effStyleIn over the widget's current cascade.
func (l *Label) effStyle() (render.Font, float64) { return l.effStyleIn(l.style(l)) }

// styleRestyled implements styleRestyler: a changed effective face,
// size, or text transform reshapes the cached run and drops the
// measure cache.
func (l *Label) styleRestyled(old, new style.Values) {
	of, opx := l.effStyleIn(&old)
	nf, npx := l.effStyleIn(&new)
	if of != nf || opx != npx || old.TextTransform != new.TextTransform ||
		l.effDirection(&old) != l.effDirection(&new) {
		l.retext()
		l.InvalidateLayout()
	}
}

// textColor is the ink: the programmatic color when set, else the
// stylesheet's (direct or inherited), else the constructor value —
// zero paints nothing, the no-stylesheet behavior.
func (l *Label) textColor(v *style.Values) render.Color {
	return pickc(l.color, v, style.PropColor, l.color)
}

// shown is the text as painted: the stylesheet's text-transform applied
// (uppercase, lowercase, capitalize).
func (l *Label) shown() string {
	return transformText(l.text, l.style(l).TextTransform)
}

// effDirection is the base paragraph direction the label resolves
// with: the CSS direction (and unicode-bidi's overrides, which force
// the base the same way) over the programmatic one.
func (l *Label) effDirection(v *style.Values) Direction {
	if v.Has(style.PropDirection) {
		if v.Direction == 2 {
			return DirectionRTL
		}
		return DirectionLTR
	}
	if v.Has(style.PropUnicodeBidi) && (v.UnicodeBidi == style.BidiOverride || v.UnicodeBidi == style.BidiIsolateOverride) {
		if l.shaped != nil && l.shaped.RTL() {
			return DirectionRTL
		}
		return DirectionLTR
	}
	return l.dir
}

// alignOf is the horizontal placement: the CSS text-align over the
// programmatic alignment, left and right absolute against the
// resolved direction (start and end mirror with it).
func (l *Label) alignOf(v *style.Values) render.Alignment {
	if !v.Has(style.PropTextAlign) {
		return l.align
	}
	rtl := l.shaped != nil && l.shaped.RTL()
	switch v.TextAlign {
	case style.AlignEnd:
		return render.AlignEnd
	case style.AlignLeft:
		if rtl {
			return render.AlignEnd
		}
		return render.AlignStart
	case style.AlignRight:
		if rtl {
			return render.AlignStart
		}
		return render.AlignEnd
	case style.AlignCenter:
		return render.AlignCenter
	default: // start, and justify on a single line
		return render.AlignStart
	}
}

// justifies reports whether wrapped rows distribute their leftover
// width across the spaces (text-align justify: everything but the
// final row).
func (l *Label) justifies(v *style.Values) bool {
	return v.Has(style.PropTextAlign) && v.TextAlign == style.AlignJustify
}

// wrapOf is whether rows wrap at the offered width: white-space
// nowrap and pre turn wrapping off, pre-wrap, pre-line and
// break-spaces on; normal keeps the programmatic mode.
func (l *Label) wrapOf(v *style.Values) bool {
	if v.Has(style.PropLineClamp) && v.LineClamp > 0 {
		return true
	}
	if !v.Has(style.PropWhiteSpace) {
		return l.wrap
	}
	switch v.WhiteSpace {
	case style.WhiteSpaceNowrap, style.WhiteSpacePre:
		return false
	}
	return true
}

// ellOf is the truncation mode: line-clamp implies an end ellipsis,
// text-overflow ellipsis takes end when nothing stronger is set, and
// clip clears a stylesheet ellipsis.
func (l *Label) ellOf(v *style.Values) EllipsizeMode {
	if v.Has(style.PropLineClamp) && v.LineClamp > 0 {
		return EllipsizeEnd
	}
	if v.Has(style.PropTextOverflow) {
		if v.Overflow == style.TextOverflowEllipsis {
			if l.ell == EllipsizeNone {
				return EllipsizeEnd
			}
			return l.ell
		}
		return EllipsizeNone
	}
	return l.ell
}

// maxLinesOf is the row cap: CSS line-clamp over GTK's lines.
func (l *Label) maxLinesOf(v *style.Values) int {
	if v.Has(style.PropLineClamp) && v.LineClamp > 0 {
		return v.LineClamp
	}
	return l.maxLines
}

// wrapOpts is the CSS wrap configuration the breaker takes: word-break,
// hyphens, line-break anywhere, and the white-space family's space
// preservation.
func wrapOpts(v *style.Values) render.WrapOptions {
	var o render.WrapOptions
	switch v.WordBreak {
	case style.WordBreakBreakAll:
		o.Break = render.WrapWordBreakAll
	case style.WordBreakKeepAll:
		o.Break = render.WrapWordKeepAll
	}
	switch v.Hyphens {
	case style.HyphensManual:
		o.Hyphens = render.HyphensManual
	case style.HyphensAuto:
		o.Hyphens = render.HyphensAuto
	}
	o.Anywhere = v.LineBreak == style.LineBreakAnywhere
	switch v.WhiteSpace {
	case style.WhiteSpacePre, style.WhiteSpacePreWrap, style.WhiteSpaceBreakSpaces:
		o.PreserveSpaces = true
	}
	return o
}

// indentOf is the first row's indent (text-indent) at the start edge.
func (l *Label) indentOf(v *style.Values) float64 {
	if !v.Has(style.PropTextIndent) {
		return 0
	}
	return v.TextIndent
}

// retext re-resolves and reshapes the run under the label's base
// direction and refreshes the cached natural size.
func (l *Label) retext() {
	face, px := l.effStyle()
	l.shaped = face.ShapeDir(l.shown(), px, l.effDirection(l.style(l)))
	l.natural = Size{
		W: int(math.Ceil(l.shaped.Advance())),
		H: l.lineBox(l.style(l)),
	}
}

// lineBox is the line height the label lays text out at: the CSS
// line-height when the style sets one, else the shaped line's natural
// ascent plus descent.
func (l *Label) lineBox(v *style.Values) int {
	if lh := pickf(v, style.PropLineHeight, 0); lh > 0 {
		return int(math.Ceil(lh))
	}
	return l.shaped.LineHeight()
}

// Measure returns the text's advance and line height inside the CSS box
// (padding, border, margin, min sizes), clamped to con. Both layout
// modes key off the offered width: wrapping reports the widest wrapped
// row by one line height per row, ellipsizing the truncated advance.
// Results memoize per constraint set, so a panel resize that changes
// the offered width re-measures on the next frame.
func (l *Label) Measure(con Constraints) Size {
	if s, ok := l.measureHit(con); ok {
		l.measured = s
		return s
	}
	v := l.style(l)
	l.measured = measureBox(v, boxOf(v, render.Insets{}), con, l.measureNatural)
	return l.measureStore(con, l.measured)
}

// ShrinkableWidth implements WidthShrinker: a single-line ellipsizing
// label gives up its measured width down to the ellipsis and its CSS
// box, the minimum GTK gives an ellipsized label. Wrapping and
// clipping labels keep their width.
func (l *Label) ShrinkableWidth() int {
	if l.wrapOf(l.style(l)) || l.ellOf(l.style(l)) == EllipsizeNone {
		return 0
	}
	face, px := l.effStyle()
	o := boxOf(l.style(l), render.Insets{}).outer()
	floor := int(math.Ceil(face.Shape(render.Ellipsis, px).Advance())) + o.Left + o.Right
	return max(0, l.measured.W-floor)
}

// measureNatural computes the wanted content size for con from the
// label's current text and modes. Widths round up: Paint ellipsizes
// (and wraps) against the raw advance, so a natural width rounded to
// nearest would truncate a text laid out at exactly its natural size.
func (l *Label) measureNatural(con Constraints) Size {
	sz := l.textNatural(con)
	if f := l.widthFloor(); f > sz.W {
		sz.W = f
	}
	return clampSize(sz, con)
}

// widthFloor is the width-chars floor in pixels: n approximate
// character widths, 0 when unset.
func (l *Label) widthFloor() int {
	if l.widthChars <= 0 {
		return 0
	}
	face, px := l.effStyle()
	avg := face.Shape(approxCharSample, px).Advance() / float64(len(approxCharSample))
	return int(math.Ceil(avg * float64(l.widthChars)))
}

// textNatural is the text's own wanted size under con, the measure
// label's current text and modes. Widths round up: Paint ellipsizes
// (and wraps) against the raw advance, so a natural width rounded to
// nearest would truncate a text laid out at exactly its natural size.
func (l *Label) textNatural(con Constraints) Size {
	con = l.capWidth(con)
	v := l.style(l)
	face, px := l.effStyle()
	lineH := l.lineBox(v)
	wrap, ell := l.wrapOf(v), l.ellOf(v)
	indent := int(math.Ceil(l.indentOf(v)))
	if !wrap {
		if l.shaped.Advance() <= float64(con.Max.W) || ell == EllipsizeNone {
			// The whole text fits the offered width, or it overflows and
			// clips: no mode applies. The height comes from the live line
			// box — natural was cached before any stylesheet resolved.
			sz := l.natural
			sz.H = lineH
			return clampSize(sz, con)
		}
		truncated := render.EllipsizeText(face, l.shown(), ell, float64(con.Max.W), px)
		return clampSize(Size{
			W: int(math.Ceil(face.Shape(truncated, px).Advance())),
			H: lineH,
		}, con)
	}
	rows := l.wrapped(float64(con.Max.W))
	w := float64(indent)
	for _, ln := range rows {
		w = math.Max(w, face.Shape(ln, px).Advance())
	}
	return clampSize(Size{W: int(math.Ceil(w)), H: len(rows) * lineH}, con)
}

// wrapped breaks the text at width, ellipsizing the final row when a
// mode is set - the shared body of Measure and Paint, so the label
// reports and draws the same rows.
func (l *Label) wrapped(width float64) []string {
	face, px := l.effStyle()
	v := l.style(l)
	ell, maxLines := l.ellOf(v), l.maxLinesOf(v)
	lines := render.WrapTextOpts(face, l.shown(), width, px, wrapOpts(v))
	if maxLines > 0 && ell != EllipsizeNone && len(lines) > maxLines {
		lines = append(lines[:maxLines-1], strings.Join(lines[maxLines-1:], " "))
	}
	if last := len(lines) - 1; ell != EllipsizeNone && lines[last] != "" {
		lines[last] = render.EllipsizeText(face, lines[last], ell, width, px)
	}
	return lines
}

// MinSize implements MinSizer: a wrapping label's floor is its widest
// unbreakable token — wrapping narrower than that clips tokens, the
// one thing wrapping promised not to do — and one line height, the
// height below which nothing paints. Without wrap there is no width
// floor: clipping and ellipsizing own the narrow rects by design. The
// CSS box adds around the floor.
func (l *Label) MinSize() Size {
	face, px := l.effStyle()
	v := l.style(l)
	o := boxOf(v, render.Insets{}).outer()
	floor := Size{H: l.lineBox(v)}
	if l.wrapOf(v) && l.text != "" {
		w := 0.0
		for _, tok := range render.WrapText(face, l.shown(), 1, px) {
			w = math.Max(w, face.Shape(tok, px).Advance())
		}
		floor.W = int(math.Ceil(w))
	}
	floor.W = max(floor.W, picki(v, style.PropMinWidth, 0)) + o.Left + o.Right
	floor.H = max(floor.H, picki(v, style.PropMinHeight, 0)) + o.Top + o.Bottom
	return floor
}

// Arrange records the border box inside r, the margin box.
func (l *Label) Arrange(r render.Rect) {
	border, _ := boxRects(boxOf(l.style(l), render.Insets{}), r)
	l.node.Arrange(border)
}

// Paint draws the label's CSS layers (when the stylesheet gives it
// any), then the text inside the content box: the wrapped rows when
// wrapping is on, the single line ellipsized as configured otherwise.
func (l *Label) Paint(cv *render.Canvas) {
	v := l.style(l)
	b := boxOf(v, render.Insets{})
	radii := radiusOr(v, 0)
	fx := pushEffects(cv, &l.node, v, l.bounds, radii)
	cv = fx.cv
	if bg := pickc(0, v, style.PropBackgroundColor, 0); bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, l.bounds, radii, b.border, bg)
	}
	paintPseudos(cv, l, nodeOf(l), v)
	content := b.padding.Shrink(b.border.Shrink(l.bounds))
	face, px := l.effStyleIn(v)
	col := l.textColor(v)
	if l.wrapOf(v) {
		l.paintWrapped(cv, face, px, col, content, v)
	} else {
		text := l.shown()
		if ell := l.ellOf(v); ell != EllipsizeNone && l.shaped.Advance() > float64(content.W) {
			text = render.EllipsizeText(face, text, ell, float64(content.W), px)
		}
		// A CSS line-height taller than the glyphs centers them in the
		// taller box (half-leading); a smaller one lets them overflow
		// the box, centered, rather than skipping the draw. Unstyled
		// labels keep the plain centering draw.
		line := content
		if v.Has(style.PropLineHeight) {
			if h := l.lineBox(v); h > line.H {
				line.H = h
			} else if h < line.H {
				line.Y += (line.H - l.shaped.LineHeight()) / 2
				line.H = l.shaped.LineHeight()
			}
		}
		l.drawLine(cv, face, px, text, line, col, cssDecoration(v), l.alignOf(v), l.effDirection(v), 0)
	}
	paintOutline(cv, v, l.bounds, radii)
	fx.pop()
}

// drawLine draws one aligned line of text and its decoration.
func (l *Label) drawLine(cv *render.Canvas, face render.Font, px float64, text string, box render.Rect, col render.Color, deco render.Decoration, align render.Alignment, dir Direction, justify float64) {
	sh := face.ShapeDir(text, px, dir)
	if float64(box.H) < float64(sh.LineHeight()) {
		return
	}
	x, baseline := render.AlignedPen(sh, box, align, dir)
	if justify > 0 && sh.SeparatorCount() > 0 {
		sh.DrawJustified(cv, x, baseline, col, justify)
	} else {
		sh.Draw(cv, x, baseline, col)
	}
	if deco.Lines != 0 {
		sh.DrawDecoration(cv, x, baseline, deco, col)
	}
}

// paintWrapped draws the wrapped rows, the stack vertically centered in
// a taller rect, each row aligned per the cascade - justify spreads
// everything but the final row across the width, and the first row
// carries the text indent at its start edge.
func (l *Label) paintWrapped(cv *render.Canvas, face render.Font, px float64, col render.Color, box render.Rect, v *style.Values) {
	lines := l.wrapped(float64(box.W))
	lineH := l.lineBox(v)
	align, dir := l.alignOf(v), l.effDirection(v)
	y := box.Y
	if extra := box.H - len(lines)*lineH; extra > 0 {
		y += extra / 2
	}
	deco := cssDecoration(v)
	rtl := l.shaped != nil && l.shaped.RTL()
	for i, ln := range lines {
		row, just := box, 0.0
		if indent := l.indentOf(v); i == 0 && indent != 0 {
			if rtl {
				row.W -= int(math.Round(indent))
			} else {
				row.X += int(math.Round(indent))
				row.W -= int(math.Round(indent))
			}
		}
		if l.justifies(v) && i < len(lines)-1 {
			if sh := face.ShapeDir(ln, px, dir); sh.SeparatorCount() > 0 {
				if gap := (float64(box.W) - sh.Advance()) / float64(sh.SeparatorCount()); gap > 0 {
					just = gap
				}
			}
		}
		l.drawLine(cv, face, px, ln, render.Rect{X: row.X, Y: y, W: box.W, H: lineH}, col, deco, align, dir, just)
		y += lineH
	}
}

// transformText applies a CSS text-transform.
func transformText(s string, t style.TextTransform) string {
	switch t {
	case style.TransformUppercase:
		return strings.ToUpper(s)
	case style.TransformLowercase:
		return strings.ToLower(s)
	case style.TransformCapitalize:
		rs := []rune(s)
		start := true
		for i, r := range rs {
			if unicode.IsSpace(r) {
				start = true
				continue
			}
			if start {
				rs[i] = unicode.ToTitle(r)
			}
			start = false
		}
		return string(rs)
	}
	return s
}

// Role implements Roleer.
func (l *Label) Role() Role { return RoleLabel }

func (l *Label) pseudoFace() (render.Font, float64) {
	face, px := l.effStyleIn(l.style(l))
	return face, px
}

// HitTest returns the label when p is inside its bounds.
func (l *Label) HitTest(p Point) Widget {
	return l.HitLeaf(l, p)
}
