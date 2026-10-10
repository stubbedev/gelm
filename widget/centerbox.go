package widget

import (
	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// CenterBox lays three children like gtk::CenterBox: the center one at
// the middle of the allocation, the start and end children at the
// edges. The center child stays centered however wide the sides run.
type CenterBox struct {
	node
	start, center, end Widget
}

// NewCenterBox returns a three-slot row; any slot may be nil.
func NewCenterBox(start, center, end Widget) *CenterBox {
	b := &CenterBox{start: start, center: center, end: end}
	setParents(b, nonNil(start, center, end)...)
	return b
}

func nonNil(ws ...Widget) []Widget {
	out := make([]Widget, 0, len(ws))
	for _, w := range ws {
		if w != nil {
			out = append(out, w)
		}
	}
	return out
}

// Children exposes the slots in start, center, end order.
func (b *CenterBox) Children() []Widget { return nonNil(b.start, b.center, b.end) }

// Measure sizes to the three naturals side by side, as tall as the
// tallest.
func (b *CenterBox) Measure(con Constraints) Size {
	if sz, ok := b.measureHit(con); ok {
		return sz
	}
	var w, h int
	for _, c := range b.Children() {
		sz := measureChild(b, c, Constraints{Max: con.Max})
		w += sz.W
		h = max(h, sz.H)
	}
	v := b.style(b)
	if v.Has(style.PropMinWidth) {
		w = max(w, v.MinWidth)
	}
	if v.Has(style.PropMinHeight) {
		h = max(h, v.MinHeight)
	}
	return b.measureStore(con, Size{W: min(w, con.Max.W), H: min(h, con.Max.H)})
}

// ArrangeRoot records the box's own rect.
func (b *CenterBox) ArrangeRoot(r render.Rect) { b.node.Arrange(r) }

// Arrange centers the center slot in r, the start slot at the left
// edge, the end slot at the right; every slot spans the height.
func (b *CenterBox) Arrange(r render.Rect) {
	b.ArrangeRoot(r)
	o := boxOf(b.style(b), render.Insets{}).outer()
	inner := o.Shrink(r)
	cw := 0
	if b.center != nil {
		cw = measureChild(b, b.center, Constraints{Max: Size{W: inner.W, H: inner.H}}).W
	}
	cx := inner.X + (inner.W-cw)/2
	if b.start != nil {
		sw := measureChild(b, b.start, Constraints{Max: Size{W: inner.W, H: inner.H}}).W
		arrangeChild(b.start, render.Rect{X: inner.X, Y: inner.Y, W: sw, H: inner.H})
	}
	if b.center != nil {
		arrangeChild(b.center, render.Rect{X: cx, Y: inner.Y, W: cw, H: inner.H})
	}
	if b.end != nil {
		ew := measureChild(b, b.end, Constraints{Max: Size{W: inner.W, H: inner.H}}).W
		arrangeChild(b.end, render.Rect{X: inner.X + inner.W - ew, Y: inner.Y, W: ew, H: inner.H})
	}
}

// HitTest returns the slot under p, else the box.
func (b *CenterBox) HitTest(p Point) Widget {
	for _, c := range b.Children() {
		if hit := c.HitTest(p); hit != nil {
			return hit
		}
	}
	return b.HitLeaf(b, p)
}

// Paint draws the box's CSS layers, then the slots.
func (b *CenterBox) Paint(cv *render.Canvas) {
	v := b.style(b)
	fx := pushEffects(cv, v)
	radii := radiusOr(v, 0)
	bg := pickc(0, v, style.PropBackgroundColor, 0)
	if bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, b.bounds, radii, borderOf(v), bg)
	}
	for _, c := range b.Children() {
		if IsVisible(c) {
			PaintChild(cv, c)
		}
	}
	paintOutline(cv, v, b.bounds, radii)
	fx.pop(cv)
}
