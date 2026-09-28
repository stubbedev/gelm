package widget

import (
	"slices"

	"github.com/stubbedev/gelm/internal/style"
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
//
// A row's start/end semantics mirror with the box's direction: an RTL
// row flows from the right edge, so the first child sits rightmost,
// where an RTL reader starts. Columns are unaffected.
type Box struct {
	node
	axis    Axis
	dir     Direction
	spacing int
	padding int
	child   []*childEntry
}

// Children exposes the box's children in append order for focus
// traversal.
func (b *Box) Children() []Widget {
	out := make([]Widget, len(b.child))
	for i, c := range b.child {
		out[i] = c.w
	}
	return out
}

// SetDirection selects the base direction the box's main axis flows
// along: an RTL row lays its children out right to left, mirroring
// start/end placement. Columns ignore it. Changing the direction
// re-arranges.
func (b *Box) SetDirection(d Direction) {
	if b.dir == d {
		return
	}
	b.dir = d
	b.InvalidateLayout()
}

// Direction returns the base direction the main axis flows along.
func (b *Box) Direction() Direction { return b.dir }

// NewBox returns an empty box along axis with the given spacing between
// children and padding on every side.
func NewBox(axis Axis, spacing, padding int) *Box {
	return &Box{axis: axis, spacing: spacing, padding: padding}
}

// Append adds a widget to the box and reports whether it should expand
// into leftover main-axis space. It returns the box for chaining. The
// box's measure cache drops so the next frame sees the new child.
func (b *Box) Append(w Widget, expand bool) *Box {
	b.child = append(b.child, &childEntry{w: w, expand: expand})
	b.InvalidateLayout()
	return b
}

// detachChild drops the child at index i: the removal hook fires while
// the child is still linked to the box (so a router can move a removed
// focus to its traversal neighbor), then the entry goes, the child's
// parent link clears, and the box reflows.
func (b *Box) detachChild(i int) {
	w := b.child[i].w
	notifyRemoved(w)
	b.child = slices.Delete(b.child, i, i+1)
	clearParents(w)
	b.InvalidateLayout()
}

// Remove detaches w, found by identity, and reports whether it was a
// child. The removed widget's parent link clears, so it can be
// appended elsewhere without a double parent; removing a widget that
// is not (or no longer) a child reports false and changes nothing.
// The measure cache drops so the next frame reflows without it.
func (b *Box) Remove(w Widget) bool {
	for i, c := range b.child {
		if c.w == w {
			b.detachChild(i)
			return true
		}
	}
	return false
}

// RemoveAt detaches the child at index i; an out-of-range index is a
// no-op.
func (b *Box) RemoveAt(i int) {
	if i < 0 || i >= len(b.child) {
		return
	}
	b.detachChild(i)
}

// Clear detaches every child at once — the wholesale rebuild path.
// Each child's parent link clears and the removal hook fires per
// child; clearing an empty box does nothing.
func (b *Box) Clear() {
	if len(b.child) == 0 {
		return
	}
	ws := make([]Widget, len(b.child))
	for i, c := range b.child {
		ws[i] = c.w
	}
	for _, w := range ws {
		notifyRemoved(w)
	}
	b.child = nil
	clearParents(ws...)
	b.InvalidateLayout()
}

// InsertAt puts w at index i with Append's expand meaning, so dynamic
// UIs can reorder without a rebuild. Out-of-range indexes clamp to the
// ends; the measure cache drops like every other mutation.
func (b *Box) InsertAt(i int, w Widget, expand bool) {
	i = min(max(i, 0), len(b.child))
	b.child = slices.Insert(b.child, i, &childEntry{w: w, expand: expand})
	b.InvalidateLayout()
}

// SetEnabled turns the box's subtree on or off: the per-query enable
// walk (IsEnabled) folds the box's flag into every descendant, whose
// own flag stays untouched — re-enabling the box never resurrects a
// child the app disabled on purpose. The walk marks the whole subtree
// for repaint, since each descendant's effective state changed.
func (b *Box) SetEnabled(enabled bool) {
	b.node.SetEnabled(enabled)
	invalidateTree(b)
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
// child plus padding across. The result is cached until an InvalidateLayout
// anywhere in the subtree or a different constraint arrives, so a static
// tree costs no recursion on later frames.
func (b *Box) Measure(con Constraints) Size {
	if sz, ok := b.measureHit(con); ok {
		return sz
	}
	pad := b.pad()
	innerCross := max(0, b.crossMax(con)-2*pad)
	availMain := max(0, b.main(con.Max)-2*pad-b.spacing*(len(b.child)-1))

	total := 2 * pad
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
	cross += 2 * pad
	return b.measureStore(con, clampSize(b.withMain(Size{W: cross, H: cross}, total), con))
}

// pad is the effective padding: the stylesheet's when set, else the
// constructor default.
func (b *Box) pad() int {
	return picki(b.style(b), style.PropPadding, b.padding)
}

// Arrange positions the children inside r: the inner rect after padding,
// expanding children sharing the leftover main-axis space, every child
// stretched across the cross axis.
func (b *Box) Arrange(r render.Rect) {
	b.ArrangeRoot(r)
	pad := b.pad()
	inner := render.Rect{
		X: r.X + pad,
		Y: r.Y + pad,
		W: r.W - 2*pad,
		H: r.H - 2*pad,
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
	rtl := b.axis == Row && b.dir == DirectionRTL
	for _, c := range b.child {
		size := b.main(c.nat)
		if c.expand {
			size += extra
		}
		var rect render.Rect
		if b.axis == Row {
			x := inner.X + pos
			if rtl {
				x = inner.X + inner.W - pos - size // the row flows from the right edge
			}
			rect = render.Rect{X: x, Y: inner.Y, W: size, H: inner.H}
		} else {
			rect = render.Rect{X: inner.X, Y: inner.Y + pos, W: inner.W, H: size}
		}
		c.w.Arrange(rect)
		setParents(b, c.w)
		pos += size + b.spacing
	}
}

// ArrangeRoot records the box's own rect.
func (b *Box) ArrangeRoot(r render.Rect) {
	b.node.Arrange(r)
}

// Paint fills the background when the stylesheet gives the box one
// (a bare box paints nothing — theme-only boxes are transparent), then
// paints the children in order.
func (b *Box) Paint(cv *render.Canvas) {
	v := b.style(b)
	if bg := pickc(0, v, style.PropBackgroundColor, 0); bg != 0 {
		cv.RoundedRect(b.bounds, picki(v, style.PropBorderRadius, 0), bg)
	}
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
