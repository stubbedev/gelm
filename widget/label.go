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
	face   render.Font
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
// installed face resolver, else the constructor face) and the size
// (font-size, else the constructor size).
func (l *Label) effStyleIn(v *style.Values) (render.Font, float64) {
	px := fontPx(v, l.sizePx)
	if !v.Has(style.PropFontFamily) && !v.Has(style.PropFontWeight) {
		return l.face, px
	}
	family, weight := "", 0
	if v.Has(style.PropFontFamily) {
		family = v.FontFamily
	}
	if v.Has(style.PropFontWeight) {
		weight = v.FontWeight
	}
	if f, ok := resolveFace(family, weight); ok {
		return f, px
	}
	return l.face, px
}

// effStyle is effStyleIn over the widget's current cascade.
func (l *Label) effStyle() (render.Font, float64) { return l.effStyleIn(l.style(l)) }

// styleRestyled implements styleRestyler: a changed effective face,
// size, or text transform reshapes the cached run and drops the
// measure cache.
func (l *Label) styleRestyled(old, new style.Values) {
	of, opx := l.effStyleIn(&old)
	nf, npx := l.effStyleIn(&new)
	if of != nf || opx != npx || old.TextTransform != new.TextTransform {
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

// retext re-resolves and reshapes the run under the label's base
// direction and refreshes the cached natural size.
func (l *Label) retext() {
	face, px := l.effStyle()
	l.shaped = face.ShapeDir(l.shown(), px, l.dir)
	l.natural = Size{
		W: int(math.Ceil(l.shaped.Advance())),
		H: l.shaped.LineHeight(),
	}
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
	if l.wrap || l.ell == EllipsizeNone {
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
	face, px := l.effStyle()
	lineH := l.shaped.LineHeight()
	if !l.wrap {
		if l.shaped.Advance() <= float64(con.Max.W) || l.ell == EllipsizeNone {
			// The whole text fits the offered width, or it overflows and
			// clips: no mode applies.
			return clampSize(l.natural, con)
		}
		truncated := render.EllipsizeText(face, l.shown(), l.ell, float64(con.Max.W), px)
		return clampSize(Size{
			W: int(math.Ceil(face.Shape(truncated, px).Advance())),
			H: lineH,
		}, con)
	}
	rows := l.wrapped(float64(con.Max.W))
	w := 0.0
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
	lines := render.WrapText(face, l.shown(), width, px)
	if n := l.maxLines; n > 0 && l.ell != EllipsizeNone && len(lines) > n {
		lines = append(lines[:n-1], strings.Join(lines[n-1:], " "))
	}
	if last := len(lines) - 1; l.ell != EllipsizeNone && lines[last] != "" {
		lines[last] = render.EllipsizeText(face, lines[last], l.ell, width, px)
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
	floor := Size{H: l.shaped.LineHeight()}
	if l.wrap && l.text != "" {
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
	fx := pushEffects(cv, v)
	b := boxOf(v, render.Insets{})
	radii := radiusOr(v, 0)
	if bg := pickc(0, v, style.PropBackgroundColor, 0); bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, l.bounds, radii, b.border, bg)
	}
	content := b.padding.Shrink(b.border.Shrink(l.bounds))
	face, px := l.effStyleIn(v)
	col := l.textColor(v)
	if l.wrap {
		l.paintWrapped(cv, face, px, col, content)
	} else {
		text := l.shown()
		if l.ell != EllipsizeNone && l.shaped.Advance() > float64(content.W) {
			text = render.EllipsizeText(face, text, l.ell, float64(content.W), px)
		}
		face.DrawAlignedDir(cv, text, content, px, col, l.align, l.dir)
		if v.Has(style.PropTextDecoration) && v.Underline {
			l.paintUnderline(cv, face, px, text, content, col)
		}
	}
	paintOutline(cv, v, l.bounds, radii)
	fx.pop(cv)
}

// paintUnderline strokes the text run's baseline+2 underline, the
// text-decoration ink, spanning the shaped advance.
func (l *Label) paintUnderline(cv *render.Canvas, face render.Font, px float64, text string, content render.Rect, col render.Color) {
	sh := face.ShapeDir(text, px, l.dir)
	baseline := content.Y + (content.H-sh.LineHeight())/2 + int(sh.Ascent()+0.5)
	y := baseline + 2
	adv := int(sh.Advance() + 0.5)
	x := content.X
	switch l.align {
	case render.AlignEnd:
		x = content.X + content.W - adv
	case render.AlignCenter:
		x = content.X + (content.W-adv)/2
	}
	cv.FillRect(render.Rect{X: x, Y: y, W: adv, H: 1}, col)
}

// paintWrapped draws the wrapped rows, the stack vertically centered in
// a taller rect, each row aligned like a single line.
func (l *Label) paintWrapped(cv *render.Canvas, face render.Font, px float64, col render.Color, box render.Rect) {
	lines := l.wrapped(float64(box.W))
	lineH := l.shaped.LineHeight()
	y := box.Y
	if extra := box.H - len(lines)*lineH; extra > 0 {
		y += extra / 2
	}
	for _, ln := range lines {
		face.DrawAlignedDir(cv, ln, render.Rect{X: box.X, Y: y, W: box.W, H: lineH}, px, col, l.align, l.dir)
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

// HitTest returns the label when p is inside its bounds.
func (l *Label) HitTest(p Point) Widget {
	return l.HitLeaf(l, p)
}
