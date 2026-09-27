package widget

import (
	"github.com/stubbedev/gelm/render"
)

// Label paints a single line of shaped text. It is passive: it hits as
// itself but has no interaction behavior.
type Label struct {
	node
	face    render.Font
	text    string
	sizePx  float64
	color   render.Color
	align   render.Alignment
	shaped  *render.ShapedText
	natural Size
}

// NewLabel returns a label that paints text with face at sizePx pixels.
// Face may be a render.Chain for mixed-script fallback.
func NewLabel(face render.Font, text string, sizePx float64, color render.Color) *Label {
	l := &Label{face: face, text: text, sizePx: sizePx, color: color}
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
// wider than the text.
func (l *Label) SetAlignment(a render.Alignment) {
	l.align = a
}

// retext reshapes the run and refreshes the cached natural size.
func (l *Label) retext() {
	l.shaped = l.face.Shape(l.text, l.sizePx)
	l.natural = Size{
		W: int(l.shaped.Advance() + 0.5),
		H: l.shaped.LineHeight(),
	}
}

// Measure returns the text's advance and line height, clamped to con.
func (l *Label) Measure(con Constraints) Size {
	if s, ok := l.measureHit(con); ok {
		return s
	}
	return l.measureStore(con, clampSize(l.natural, con))
}

// Paint draws the text inside the arranged rect; nothing is painted when
// the rect cannot hold one line.
func (l *Label) Paint(cv *render.Canvas) {
	l.face.DrawAligned(cv, l.text, l.bounds, l.sizePx, l.color, l.align)
}

// Role implements Roleer.
func (l *Label) Role() Role { return RoleLabel }

// HitTest returns the label when p is inside its bounds.
func (l *Label) HitTest(p Point) Widget {
	return l.HitLeaf(l, p)
}
