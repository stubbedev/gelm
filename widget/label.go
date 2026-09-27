package widget

import (
	"math"

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

// retext reshapes the run and refreshes the cached natural size.
func (l *Label) retext() {
	l.shaped = l.face.Shape(l.text, l.sizePx)
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
		truncated := render.EllipsizeText(l.face, l.text, l.ell, float64(con.Max.W), l.sizePx)
		return clampSize(Size{
			W: int(l.face.Shape(truncated, l.sizePx).Advance() + 0.5),
			H: lineH,
		}, con)
	}
	rows := l.wrapped(float64(con.Max.W))
	w := 0.0
	for _, ln := range rows {
		w = math.Max(w, l.face.Shape(ln, l.sizePx).Advance())
	}
	return clampSize(Size{
		W: int(w + 0.5),
		H: len(rows) * lineH,
	}, con)
}

// wrapped breaks the text at width, ellipsizing the final row when a
// mode is set - the shared body of Measure and Paint, so the label
// reports and draws the same rows.
func (l *Label) wrapped(width float64) []string {
	lines := render.WrapText(l.face, l.text, width, l.sizePx)
	if last := len(lines) - 1; l.ell != EllipsizeNone && lines[last] != "" {
		lines[last] = render.EllipsizeText(l.face, lines[last], l.ell, width, l.sizePx)
	}
	return lines
}

// Paint draws the text inside the arranged rect: the wrapped rows when
// wrapping is on, the single line ellipsized as configured otherwise.
// Nothing is painted when the rect cannot hold one line.
func (l *Label) Paint(cv *render.Canvas) {
	if l.wrap {
		l.paintWrapped(cv)
		return
	}
	text := l.text
	if l.ell != EllipsizeNone && l.shaped.Advance() > float64(l.bounds.W) {
		text = render.EllipsizeText(l.face, l.text, l.ell, float64(l.bounds.W), l.sizePx)
	}
	l.face.DrawAligned(cv, text, l.bounds, l.sizePx, l.color, l.align)
}

// paintWrapped draws the wrapped rows, the stack vertically centered in
// a taller rect, each row aligned like a single line.
func (l *Label) paintWrapped(cv *render.Canvas) {
	lines := l.wrapped(float64(l.bounds.W))
	lineH := l.shaped.LineHeight()
	y := l.bounds.Y
	if extra := l.bounds.H - len(lines)*lineH; extra > 0 {
		y += extra / 2
	}
	for _, ln := range lines {
		l.face.DrawAligned(cv, ln, render.Rect{
			X: l.bounds.X, Y: y, W: l.bounds.W, H: lineH,
		}, l.sizePx, l.color, l.align)
		y += lineH
	}
}

// Role implements Roleer.
func (l *Label) Role() Role { return RoleLabel }

// HitTest returns the label when p is inside its bounds.
func (l *Label) HitTest(p Point) Widget {
	return l.HitLeaf(l, p)
}
