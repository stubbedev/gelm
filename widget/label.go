package widget

import (
	"math"

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
	face    render.Font
	text    string
	sizePx  float64
	color   render.Color
	align   render.Alignment
	dir     Direction
	wrap    bool
	ell     EllipsizeMode
	shaped  *render.ShapedText
	natural Size
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

// styleRestyled implements styleRestyler: a changed effective face or
// size reshapes the cached run and drops the measure cache.
func (l *Label) styleRestyled(old, new style.Values) {
	of, opx := l.effStyleIn(&old)
	nf, npx := l.effStyleIn(&new)
	if of != nf || opx != npx {
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

// padding is the content inset, the stylesheet's when set.
func (l *Label) padding() int {
	return picki(l.style(l), style.PropPadding, 0)
}

// retext re-resolves and reshapes the run under the label's base
// direction and refreshes the cached natural size.
func (l *Label) retext() {
	face, px := l.effStyle()
	l.shaped = face.ShapeDir(l.text, px, l.dir)
	l.natural = Size{
		W: int(l.shaped.Advance() + 0.5),
		H: l.shaped.LineHeight(),
	}
}

// Measure returns the text's advance and line height, clamped to con.
// Both layout modes key off the offered width: wrapping reports the
// widest wrapped row by one line height per row, ellipsizing the
// truncated advance. Results memoize per constraint set, so a panel
// resize that changes the offered width re-measures on the next frame.
func (l *Label) Measure(con Constraints) Size {
	if s, ok := l.measureHit(con); ok {
		return s
	}
	return l.measureStore(con, l.measureNatural(con))
}

// measureNatural computes the wanted size for con from the label's
// current text and modes. Fit checks compare the raw advance against
// the offered width - the rounded natural size can lie by a pixel.
func (l *Label) measureNatural(con Constraints) Size {
	face, px := l.effStyle()
	lineH := l.shaped.LineHeight()
	if !l.wrap {
		if l.shaped.Advance() <= float64(con.Max.W) {
			// The whole text fits the offered width; no mode applies.
			return clampSize(l.natural, con)
		}
		if l.ell == EllipsizeNone {
			// Overflow: claim the offered box and clip, as before.
			return clampSize(l.natural, con)
		}
		truncated := render.EllipsizeText(face, l.text, l.ell, float64(con.Max.W), px)
		return clampSize(Size{
			W: int(face.Shape(truncated, px).Advance() + 0.5),
			H: lineH,
		}, con)
	}
	rows := l.wrapped(float64(con.Max.W))
	w := 0.0
	for _, ln := range rows {
		w = math.Max(w, face.Shape(ln, px).Advance())
	}
	return clampSize(Size{
		W: int(w + 0.5),
		H: len(rows)*lineH + 2*l.padding(),
	}, con)
}

// wrapped breaks the text at width, ellipsizing the final row when a
// mode is set - the shared body of Measure and Paint, so the label
// reports and draws the same rows.
func (l *Label) wrapped(width float64) []string {
	face, px := l.effStyle()
	lines := render.WrapText(face, l.text, width, px)
	if last := len(lines) - 1; l.ell != EllipsizeNone && lines[last] != "" {
		lines[last] = render.EllipsizeText(face, lines[last], l.ell, width, px)
	}
	return lines
}

// MinSize implements MinSizer: a wrapping label's floor is its widest
// unbreakable token — wrapping narrower than that clips tokens, the
// one thing wrapping promised not to do — and one line height, the
// height below which nothing paints. Without wrap there is no width
// floor: clipping and ellipsizing own the narrow rects by design.
func (l *Label) MinSize() Size {
	face, px := l.effStyle()
	floor := Size{H: l.shaped.LineHeight()}
	if !l.wrap || l.text == "" {
		return floor
	}
	w := 0.0
	for _, tok := range render.WrapText(face, l.text, 1, px) {
		w = math.Max(w, face.Shape(tok, px).Advance())
	}
	floor.W = int(w + 0.5)
	return floor
}

// Paint draws the text inside the arranged rect: the wrapped rows when
// wrapping is on, the single line ellipsized as configured otherwise.
// Nothing is painted when the rect cannot hold one line.
func (l *Label) Paint(cv *render.Canvas) {
	face, px := l.effStyle()
	col := l.textColor(l.style(l))
	pad := l.padding()
	if l.wrap {
		l.paintWrapped(cv, face, px, col, pad)
		return
	}
	text := l.text
	if l.ell != EllipsizeNone && l.shaped.Advance() > float64(l.bounds.W-2*pad) {
		text = render.EllipsizeText(face, l.text, l.ell, float64(l.bounds.W-2*pad), px)
	}
	face.DrawAlignedDir(cv, text, shrinkRect(l.bounds, pad), px, col, l.align, l.dir)
}

// paintWrapped draws the wrapped rows, the stack vertically centered in
// a taller rect, each row aligned like a single line.
func (l *Label) paintWrapped(cv *render.Canvas, face render.Font, px float64, col render.Color, pad int) {
	lines := l.wrapped(float64(l.bounds.W - 2*pad))
	lineH := l.shaped.LineHeight()
	y := l.bounds.Y + pad
	if extra := l.bounds.H - 2*pad - len(lines)*lineH; extra > 0 {
		y += extra / 2
	}
	for _, ln := range lines {
		face.DrawAlignedDir(cv, ln, render.Rect{
			X: l.bounds.X + pad, Y: y, W: l.bounds.W - 2*pad, H: lineH,
		}, px, col, l.align, l.dir)
		y += lineH
	}
}

// Role implements Roleer.
func (l *Label) Role() Role { return RoleLabel }

// HitTest returns the label when p is inside its bounds.
func (l *Label) HitTest(p Point) Widget {
	return l.HitLeaf(l, p)
}
