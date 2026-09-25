// Package widget is gelm's retained-mode widget layer: a tree of widgets
// measured against constraints, arranged into pixel rects, and painted onto
// a render.Canvas. Input handling arrives in M4; HitTest is already here so
// event dispatch can hook into the same geometry.
package widget

import (
	"github.com/stubbedev/gelm/render"
)

// Size is a width and height in pixels.
type Size struct {
	W, H int
}

// Point is a position in pixels.
type Point struct {
	X, Y int
}

// Constraints bounds the size a widget may claim during layout. Max is the
// hard ceiling; Min what the parent needs at least.
type Constraints struct {
	Min, Max Size
}

// Widget is one node in the retained widget tree. The passes run in order:
// Measure computes how much space a widget wants, Arrange assigns its final
// rect, Paint draws it.
type Widget interface {
	// Measure returns the size this widget wants under con. The result
	// must lie within con.
	Measure(con Constraints) Size
	// Arrange assigns the widget's final pixel rect and recurses into
	// children.
	Arrange(r render.Rect)
	// Paint draws the widget, honoring any clip already set on the
	// canvas.
	Paint(cv *render.Canvas)
	// HitTest returns the deepest widget whose area contains p, or nil
	// when p misses the tree below this widget.
	HitTest(p Point) Widget
}

// node carries the arranged bounds and parent link shared by every
// implementation. Embed it; call HitLeaf from leaf HitTests and
// ArrangeRoot from implementations that position children themselves.
type node struct {
	bounds render.Rect
	parent Widget
}

// Arrange records the widget's rect.
func (n *node) Arrange(r render.Rect) {
	n.bounds = r
}

// Bounds returns the last arranged rect.
func (n *node) Bounds() render.Rect {
	return n.bounds
}

// Parent returns the container that arranged this widget, or nil for the
// tree root.
func (n *node) Parent() Widget {
	return n.parent
}

// setParent records the arranging container; containers call it on their
// children during Arrange.
func (n *node) setParent(p Widget) {
	n.parent = p
}

// parentOf returns w's parent, or nil.
func parentOf(w Widget) Widget {
	if p, ok := w.(interface{ Parent() Widget }); ok {
		return p.Parent()
	}
	return nil
}

// setParents records parent as the arranging container of every child.
func setParents(parent Widget, kids ...Widget) {
	for _, k := range kids {
		if k == nil {
			continue
		}
		if s, ok := k.(interface{ setParent(Widget) }); ok {
			s.setParent(parent)
		}
	}
}

// HitLeaf returns the widget when p falls inside its bounds, else nil. It
// is the leaf implementation of HitTest.
func (n *node) HitLeaf(self Widget, p Point) Widget {
	if n.bounds.Contains(p.X, p.Y) {
		return self
	}
	return nil
}

// clampSize pins s to con.
func clampSize(s Size, con Constraints) Size {
	if s.W < con.Min.W {
		s.W = con.Min.W
	}
	if s.H < con.Min.H {
		s.H = con.Min.H
	}
	if con.Max.W < s.W {
		s.W = con.Max.W
	}
	if con.Max.H < s.H {
		s.H = con.Max.H
	}
	return s
}
