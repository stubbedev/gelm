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
	// cross places the child across the box (AppendAligned);
	// AlignFill stretches it.
	cross Align
}

// Box is a container that lays its children along one axis with fixed
// spacing and padding. The cross axis stretches children to the inner
// height (Row) or width (Column) of the box, unless a child was added
// with AppendAligned.
//
// Leftover main-axis space is distributed equally among expanding children
// (the remainder is dropped). A column short of room takes the shortfall
// from its expanding Shrinker children (a Scroll, or a box holding one),
// down to their floors; a row takes it from any WidthShrinker child (an
// entry, an ellipsizing label, a flow box), measuring those again at the
// width they get. Past the floors the natural sizes overflow the box and
// the painter's clip decides what is visible.
//
// A row's start/end semantics mirror with the box's direction: an RTL
// row flows from the right edge, so the first child sits rightmost,
// where an RTL reader starts. Columns are unaffected.
type Box struct {
	node
	axis    Axis
	dir     Direction
	spacing int
	padding render.Insets
	child   []*childEntry
	// homogeneous gives every child the largest child's main-axis size
	// (gtk_box_layout's homogeneous): a segmented control's buttons all
	// measure as wide as the widest label.
	homogeneous bool
	// layer memoizes the offscreen the transform paints through, so a
	// hovering swatch reuses its buffer instead of allocating per frame.
	layer *render.Layer
}

// Children exposes the box's children in append order for focus
// traversal.
func (b *Box) Children() []Widget {
	return b.appendChildren(make([]Widget, 0, len(b.child)))
}

// appendChildren appends the box's children in append order, matching
// Children — the snapshot the per-frame damage walk buffers instead of
// a fresh slice per container per frame.
func (b *Box) appendChildren(buf []Widget) []Widget {
	for _, c := range b.child {
		buf = append(buf, c.w)
	}
	return buf
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

// SetHomogeneous equalizes the children's main-axis sizes to the
// largest child's (gtk_box_set_homogeneous). Changing it relayouts.
func (b *Box) SetHomogeneous(on bool) {
	if b.homogeneous == on {
		return
	}
	b.homogeneous = on
	b.InvalidateLayout()
}

// Homogeneous reports whether the children share the largest child's
// main-axis size.
func (b *Box) Homogeneous() bool { return b.homogeneous }

// NewBox returns an empty box along axis with the given spacing between
// children and padding on every side.
func NewBox(axis Axis, spacing, padding int) *Box {
	return &Box{axis: axis, spacing: spacing, padding: render.UniformInsets(padding)}
}

// Append adds a widget to the box and reports whether it should expand
// into leftover main-axis space. It returns the box for chaining. The
// box's measure cache drops so the next frame sees the new child.
func (b *Box) Append(w Widget, expand bool) *Box {
	b.child = append(b.child, &childEntry{w: w, expand: expand})
	b.InvalidateLayout()
	restyleChildren(b)
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
	restyleChildren(b)
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

// AppendAligned adds a child placed across the box by cross, GTK's
// valign in a row and halign in a column: AlignFill stretches it
// (Append), the others keep its natural cross size pinned to the
// start, center or end.
func (b *Box) AppendAligned(w Widget, expand bool, cross Align) *Box {
	b.child = append(b.child, &childEntry{w: w, expand: expand, cross: cross})
	b.InvalidateLayout()
	restyleChildren(b)
	return b
}

// InsertAt puts w at index i with Append's expand meaning, so dynamic
// UIs can reorder without a rebuild. Out-of-range indexes clamp to the
// ends; the measure cache drops like every other mutation.
func (b *Box) InsertAt(i int, w Widget, expand bool) {
	i = min(max(i, 0), len(b.child))
	b.child = slices.Insert(b.child, i, &childEntry{w: w, expand: expand})
	b.InvalidateLayout()
	restyleChildren(b)
}

// Move moves the child at index from to index to (both clamped), in
// place: the child stays attached, so its focus and state survive.
func (b *Box) Move(from, to int) {
	if from < 0 || from >= len(b.child) {
		return
	}
	to = min(max(to, 0), len(b.child)-1)
	if from == to {
		return
	}
	c := b.child[from]
	b.child = slices.Insert(slices.Delete(b.child, from, from+1), to, c)
	b.InvalidateLayout()
	restyleChildren(b)
}

func (b *Box) expands(c *childEntry) bool { return c.expand || WantsExpand(c.w, b.axis) }

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

// Measure measures every child and reports the box's natural size: the
// sum of child sizes plus spacing along the main axis, the largest child
// across, all inside the CSS box (padding, border, margin, min sizes).
// The result is cached until an InvalidateLayout anywhere in the
// subtree or a different constraint arrives, so a static tree costs no
// recursion on later frames.
func (b *Box) Measure(con Constraints) Size {
	if sz, ok := b.measureHit(con); ok {
		return sz
	}
	v := b.style(b)
	spacing := b.gap(v)
	return b.measureStore(con, measureBox(v, b.box(v), con, func(inner Constraints) Size {
		innerCross := b.crossMax(inner)
		availMain := b.main(inner.Max)
		count := 0
		for _, c := range b.child {
			if IsVisible(c.w) {
				count++
			}
		}
		if count > 0 {
			availMain -= spacing * (count - 1)
		}
		availMain = max(0, availMain)
		total, cross, shown := 0, 0, 0
		for i, c := range b.child {
			if !IsVisible(c.w) {
				b.child[i].nat = Size{}
				setParents(b, c.w)
				continue
			}
			nat := measureChild(b, c.w, Constraints{Max: b.withMain(Size{W: innerCross, H: innerCross}, availMain)})
			b.child[i].nat = nat
			total += b.main(nat) + spacing
			cross = max(cross, b.crossOf(nat))
			shown++
		}
		if shown > 0 {
			total -= spacing
		}
		// Homogeneous: every child carries the largest child's main size
		// in its natural (gtk_box_layout's homogeneous), which Arrange
		// lays out.
		if b.homogeneous {
			biggest := 0
			for _, c := range b.child {
				if IsVisible(c.w) {
					biggest = max(biggest, b.main(c.nat))
				}
			}
			for i, c := range b.child {
				if IsVisible(c.w) {
					b.child[i].nat = b.withMain(c.nat, biggest)
				}
			}
			total = biggest*shown + spacing*max(shown-1, 0)
		}
		// Height for width: a row too narrow for its children narrows
		// those that can give, and measures them again at the width
		// they get, a wrapping child growing taller.
		if b.axis == Row && total-spacing*max(shown-1, 0) > availMain {
			if takes := b.shrinkTakes(total - spacing*max(shown-1, 0) - availMain); takes != nil {
				total, cross = 0, 0
				for i, c := range b.child {
					if !IsVisible(c.w) {
						continue
					}
					if takes[i] > 0 {
						b.child[i].nat = measureChild(b, c.w, Constraints{Max: Size{W: c.nat.W - takes[i], H: innerCross}})
					}
					total += b.child[i].nat.W + spacing
					cross = max(cross, b.child[i].nat.H)
				}
				total -= spacing
			}
		}
		if above, below, ok := b.baselineGroup(); ok {
			cross = max(cross, above+below)
		}
		return b.withMain(Size{W: cross, H: cross}, total)
	}))
}

// baselineGroup spans a row's baseline-aligned children: the most any
// reaches above the shared baseline and below it.
func (b *Box) baselineGroup() (above, below int, ok bool) {
	if b.axis != Row {
		return 0, 0, false
	}
	for _, c := range b.child {
		if c.cross != AlignBaseline || !IsVisible(c.w) {
			continue
		}
		if base, has := baselineOf(c.w); has {
			above, below, ok = max(above, base), max(below, c.nat.H-base), true
		}
	}
	return above, below, ok
}

// Baseline implements Baseliner. A row's is its baseline group's,
// centered across the row as Arrange places it, else its first child
// with a baseline where its alignment puts it; a column's is its first
// child's.
func (b *Box) Baseline() (int, bool) {
	top := b.box(b.style(b)).outer().Top
	cross := 0
	for _, c := range b.child {
		if IsVisible(c.w) {
			cross = max(cross, b.crossOf(c.nat))
		}
	}
	if above, below, ok := b.baselineGroup(); ok {
		cross = max(cross, above+below)
		return top + (cross-above-below)/2 + above, true
	}
	for _, c := range b.child {
		if !IsVisible(c.w) {
			continue
		}
		base, ok := baselineOf(c.w)
		if b.axis == Column {
			return top + base, ok
		}
		if !ok {
			continue
		}
		switch c.cross {
		case AlignStart:
		case AlignEnd:
			base += cross - c.nat.H
		default: // stretched or centered text sits centered
			base += (cross - c.nat.H) / 2
		}
		return top + base, true
	}
	return 0, false
}

// box is the box's resolved CSS box: the stylesheet's padding where
// set, else the programmatic padding.
func (b *Box) box(v *style.Values) cssInsets { return boxOf(v, b.padding) }

// gap is the spacing between children: the constructor spacing plus
// the stylesheet's border-spacing along the main axis, as GTK 4's
// GtkBoxLayout adds the CSS spacing to its spacing property
// (gtkboxlayout.c get_spacing). A stylesheet's blanket
// `* { border-spacing: 0 }` therefore keeps a box's own spacing.
func (b *Box) gap(v *style.Values) int {
	if !v.Has(style.PropBorderSpacing) {
		return b.spacing
	}
	if b.axis == Row {
		return b.spacing + v.BorderSpacingH
	}
	return b.spacing + v.BorderSpacingV
}

// SetPadding sets the box's programmatic padding per side; the
// stylesheet's padding still wins where it sets a side. The box
// relayouts.
func (b *Box) SetPadding(p render.Insets) {
	if b.padding == p {
		return
	}
	b.padding = p
	b.InvalidateLayout()
}

// Padding returns the programmatic padding.
func (b *Box) Padding() render.Insets { return b.padding }

// SetSpacing sets the programmatic gap between children.
func (b *Box) SetSpacing(spacing int) {
	if b.spacing == spacing {
		return
	}
	b.spacing = spacing
	b.InvalidateLayout()
}

// Spacing returns the programmatic gap between children.
func (b *Box) Spacing() int { return b.spacing }

// Arrange positions the children inside the content box: expanding
// children share the leftover main-axis space, every child stretches
// across the cross axis. r is the margin box; Bounds records the border
// box.
func (b *Box) Arrange(r render.Rect) {
	v := b.style(b)
	border, inner := boxRects(b.box(v), r)
	b.ArrangeRoot(border)
	spacing := b.gap(v)
	if inner.Empty() {
		for _, c := range b.child {
			c.w.Arrange(render.Rect{})
			setParents(b, c.w)
		}
		return
	}

	sum := 0
	shown := 0
	for _, c := range b.child {
		if !IsVisible(c.w) {
			continue
		}
		sum += b.main(c.nat)
		shown++
	}
	avail := inner.W
	if b.axis == Column {
		avail = inner.H
	}
	free := avail - spacing*(shown-1) - sum
	expanders := 0
	for _, c := range b.child {
		if b.expands(c) && IsVisible(c.w) {
			expanders++
		}
	}
	extra := max(0, free) / max(1, expanders)
	takes := b.shrinkTakes(-free)

	pos := 0
	rtl := b.axis == Row && b.dir == DirectionRTL
	above, below, grouped := b.baselineGroup()
	groupTop := inner.Y + (inner.H-above-below)/2
	for i, c := range b.child {
		if !IsVisible(c.w) {
			c.w.Arrange(render.Rect{})
			setParents(b, c.w)
			continue
		}
		size := b.main(c.nat)
		if b.expands(c) {
			size += extra
		}
		if takes != nil {
			size -= takes[i]
		}
		var rect render.Rect
		if b.axis == Row {
			x := inner.X + pos
			if rtl {
				x = inner.X + inner.W - pos - size // the row flows from the right edge
			}
			rect = render.Rect{X: x, Y: inner.Y, W: size, H: inner.H}
			natural := Size{W: size, H: min(c.nat.H, inner.H)}
			switch c.cross {
			case AlignFill:
			case AlignBaseline:
				if base, ok := baselineOf(c.w); grouped && ok {
					rect.Y, rect.H = groupTop+above-base, natural.H
				} else {
					rect = alignRect(rect, natural, AlignFill, AlignCenter)
				}
			default:
				rect = alignRect(rect, natural, AlignFill, c.cross)
			}
		} else {
			rect = render.Rect{X: inner.X, Y: inner.Y + pos, W: inner.W, H: size}
			if c.cross != AlignFill {
				rect = alignRect(rect, Size{W: min(c.nat.W, inner.W), H: size}, c.cross, AlignFill)
			}
		}
		arrangeChild(c.w, rect)
		setParents(b, c.w)
		pos += size + spacing
	}
}

// ArrangeRoot records the box's own rect.
func (b *Box) ArrangeRoot(r render.Rect) {
	b.node.Arrange(r)
}

// Paint draws the box's CSS layers when the stylesheet gives it any (a
// bare box paints nothing — theme-only boxes are transparent), then the
// children in order, then the outline; opacity and filter wrap it all.
// A transform declaration paints the whole subtree into an offscreen
// layer and composites it back through the affine, so the transform
// applies to the children as one.
func (b *Box) Paint(cv *render.Canvas) {
	v := b.style(b)
	if v.Has(style.PropTransform) && v.Transform.M != render.Identity {
		b.paintTransformed(cv, v)
		return
	}
	fx := pushEffects(cv, v)
	radii := radiusOr(v, 0)
	bg := pickc(0, v, style.PropBackgroundColor, 0)
	if bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, b.bounds, radii, borderOf(v), bg)
	}
	for _, c := range b.child {
		if !IsVisible(c.w) {
			continue
		}
		PaintChild(cv, c.w)
	}
	paintOutline(cv, v, b.bounds, radii)
	fx.pop(cv)
}

// paintTransformed paints the box's subtree offscreen and composites
// it through the CSS transform about the transform-origin: the pivot
// the stylesheet's fractions and offsets name, the box's center by
// default. The layer is the transformed bounds, so a swatch swung past
// its box still lands.
func (b *Box) paintTransformed(cv *render.Canvas, v *style.Values) {
	ox := float64(b.bounds.X) + v.OriginFrac[0]*float64(b.bounds.W) + v.OriginPx[0]
	oy := float64(b.bounds.Y) + v.OriginFrac[1]*float64(b.bounds.H) + v.OriginPx[1]
	m := v.Transform.M.About(ox, oy)
	region := m.MapBounds(b.bounds)
	region = region.Union(b.bounds)
	l := cv.Layer(b.layer, region)
	b.layer = l
	lc := l.Canvas()
	fx := pushEffects(lc, v)
	radii := radiusOr(v, 0)
	bg := pickc(0, v, style.PropBackgroundColor, 0)
	if bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(lc, v, b.bounds, radii, borderOf(v), bg)
	}
	for _, c := range b.child {
		if !IsVisible(c.w) {
			continue
		}
		PaintChild(lc, c.w)
	}
	paintOutline(lc, v, b.bounds, radii)
	fx.pop(lc)
	cv.Composite(l, l.Canvas().Rect(), m, 1)
}

// HitTest returns the deepest child under p, or the box itself when p is
// inside its bounds but over no child (padding, spacing, leftover space).
func (b *Box) HitTest(p Point) Widget {
	for _, c := range b.child {
		if !IsVisible(c.w) {
			continue
		}
		if hit := c.w.HitTest(p); hit != nil {
			return hit
		}
	}
	return b.HitLeaf(b, p)
}
