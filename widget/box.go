package widget

import (
	"github.com/stubbedev/gelm/render"
)

// Axis is the main axis a Box lays its children along.
type Axis uint8

const (
	// Row lays children out left to right.
	Row Axis = iota
	// Column lays children out top to bottom.
	Column
)

// childEntry is one Box child: the widget, whether it grows into leftover
// main-axis space, and its last natural measurement.
type childEntry struct {
	w      Widget
	expand bool
	nat    Size
}

// Box is a container that lays its children along one axis with fixed
// spacing and padding. The cross axis stretches children to the inner
// height (Row) or width (Column) of the box.
//
// Leftover main-axis space is distributed equally among expanding children
// (the remainder is dropped). Children never shrink below their measured
// size; when the natural sizes overflow the available space they overflow
// the box, and the painter's clip decides what is visible.
type Box struct {
	node
	axis    Axis
	spacing int
	padding int
	child   []*childEntry
}

// NewBox returns an empty box along axis with the given spacing between
// children and padding on every side.
func NewBox(axis Axis, spacing, padding int) *Box {
	return &Box{axis: axis, spacing: spacing, padding: padding}
}

// Append adds a widget to the box and reports whether it should expand
// into leftover main-axis space. It returns the box for chaining.
func (b *Box) Append(w Widget, expand bool) *Box {
	b.child = append(b.child, &childEntry{w: w, expand: expand})
	return b
}

// main returns the extent of s along the box's main axis.
func (b *Box) main(s Size) int {
	if b.axis == Row {
		return s.W
	}
	return s.H
}

// withMain returns s with the main axis extent set to v.
func (b *Box) withMain(s Size, v int) Size {
	if b.axis == Row {
		s.W = v
	} else {
		s.H = v
	}
	return s
}

// crossOf returns the extent of s along the box's cross axis.
func (b *Box) crossOf(s Size) int {
	if b.axis == Row {
		return s.H
	}
	return s.W
}

// crossMax returns the cross-axis ceiling from con.
func (b *Box) crossMax(con Constraints) int {
	if b.axis == Row {
		return con.Max.H
	}
	return con.Max.W
}

// Measure measures every child and reports the box's natural size: the sum
// of child sizes plus spacing and padding along the main axis, the largest
// child plus padding across.
func (b *Box) Measure(con Constraints) Size {
	innerCross := max(0, b.crossMax(con)-2*b.padding)
	availMain := max(0, b.main(con.Max)-2*b.padding-b.spacing*(len(b.child)-1))

	total := 2 * b.padding
	cross := 0
	for i, c := range b.child {
		nat := c.w.Measure(Constraints{Max: b.withMain(Size{W: innerCross, H: innerCross}, availMain)})
		b.child[i].nat = nat
		total += b.main(nat) + b.spacing
		cross = max(cross, b.crossOf(nat))
	}
	if len(b.child) > 0 {
		total -= b.spacing
	}
	cross += 2 * b.padding
	return clampSize(b.withMain(Size{W: cross, H: cross}, total), con)
}

// Arrange positions the children inside r: the inner rect after padding,
// expanding children sharing the leftover main-axis space, every child
// stretched across the cross axis.
func (b *Box) Arrange(r render.Rect) {
	b.ArrangeRoot(r)
	inner := render.Rect{
		X: r.X + b.padding,
		Y: r.Y + b.padding,
		W: r.W - 2*b.padding,
		H: r.H - 2*b.padding,
	}
	if inner.Empty() {
		for _, c := range b.child {
			c.w.Arrange(render.Rect{})
		}
		return
	}

	sum := 0
	for _, c := range b.child {
		sum += b.main(c.nat)
	}
	avail := inner.W
	if b.axis == Column {
		avail = inner.H
	}
	free := avail - b.spacing*(len(b.child)-1) - sum
	expanders := 0
	for _, c := range b.child {
		if c.expand {
			expanders++
		}
	}
	extra := max(0, free) / max(1, expanders)

	pos := 0
	for _, c := range b.child {
		size := b.main(c.nat)
		if c.expand {
			size += extra
		}
		var rect render.Rect
		if b.axis == Row {
			rect = render.Rect{X: inner.X + pos, Y: inner.Y, W: size, H: inner.H}
		} else {
			rect = render.Rect{X: inner.X, Y: inner.Y + pos, W: inner.W, H: size}
		}
		c.w.Arrange(rect)
		pos += size + b.spacing
	}
}

// ArrangeRoot records the box's own rect.
func (b *Box) ArrangeRoot(r render.Rect) {
	b.node.Arrange(r)
}

// Paint paints the children in order.
func (b *Box) Paint(cv *render.Canvas) {
	for _, c := range b.child {
		c.w.Paint(cv)
	}
}

// HitTest returns the deepest child under p, or the box itself when p is
// inside its bounds but over no child (padding, spacing, leftover space).
func (b *Box) HitTest(p Point) Widget {
	for _, c := range b.child {
		if hit := c.w.HitTest(p); hit != nil {
			return hit
		}
	}
	return b.HitLeaf(b, p)
}
