package widget

import (
	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// composite is the shared core of widgets built from other widgets: one
// inner root every pass delegates to, an optional painted surface
// behind it, and the embedding widget (self) as the parent the root
// reports and the hit a press on bare background resolves to. An
// embedder overrides only the pass that is genuinely its own - a
// minimum height, a hit rule, an extra decoration - and inherits the
// rest, so the delegation exists exactly once.
type composite struct {
	node
	self Widget
	root Widget
	// fillWidth claims the full offered width (bars, rows, lists);
	// off, the composite wants its root's natural size.
	fillWidth bool
	// minHeight floors the measured height (chrome bars keep their
	// height when their content is short).
	minHeight int
	// surface, when set, fills the bounds behind the root through the
	// cascade: background-color over this theme fallback, border, and
	// border-radius over surfaceRadius.
	surface       func(th *Theme) Color
	surfaceRadius int
}

// surfaceFill is the plain control surface, the fallback most
// composite surfaces paint.
func surfaceFill(th *Theme) Color { return th.Surface }

// initComposite wires the core: self is the embedding widget, root the
// tree it is built from.
func (c *composite) initComposite(self, root Widget) {
	c.self, c.root = self, root
}

// setRoot swaps the delegated tree (a mode switch); the layout reflows.
func (c *composite) setRoot(root Widget) {
	c.root = root
	c.InvalidateLayout()
}

// Measure measures the root, widened to the offer under fillWidth.
func (c *composite) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	sz := c.root.Measure(con)
	if c.fillWidth {
		sz.W = con.Max.W
	}
	sz.H = max(sz.H, c.minHeight)
	return c.measureStore(con, clampSize(sz, con))
}

// Arrange lays the root over the whole rect.
func (c *composite) Arrange(r render.Rect) {
	c.node.Arrange(r)
	c.root.Arrange(r)
	setParents(c.self, c.root)
}

// Paint draws the surface (when configured) then the root.
func (c *composite) Paint(cv *render.Canvas) {
	if c.surface != nil {
		c.paintSurface(cv, c.bounds, c.surface(Current()))
	}
	PaintChild(cv, c.root)
}

// paintSurface fills rect with the composite's cascade box: the
// stylesheet's background-color over fill, its border, and its radius
// over surfaceRadius. Embedders that paint a surface on a sub-rect or
// with a state-dependent fill call it from their own Paint.
func (c *composite) paintSurface(cv *render.Canvas, rect render.Rect, fill Color) {
	v := c.style(c.self)
	paintBoxBehind(cv, v, rect, radiusOr(v, c.surfaceRadius), borderOf(v), pickc(0, v, style.PropBackgroundColor, fill))
}

// HitTest resolves into the root; bare background is the embedder.
func (c *composite) HitTest(p Point) Widget {
	if !c.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if hit := c.root.HitTest(p); hit != nil {
		return hit
	}
	return c.self
}

// styleChildren is the root (styleKids).
func (c *composite) styleChildren() []Widget { return []Widget{c.root} }
