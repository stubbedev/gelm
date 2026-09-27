package widget

import (
	"slices"

	"github.com/stubbedev/gelm/render"
)

// Align says how a grid child sits inside its cell along one axis when
// the cell is larger than the child's natural size.
type Align uint8

const (
	// AlignFill stretches the child across the whole cell. It is the
	// default, so plain Attach-ed children behave like Box children.
	AlignFill Align = iota
	// AlignStart pins the child to the cell's leading edge at its
	// natural size.
	AlignStart
	// AlignCenter centers the child in the cell at its natural size.
	AlignCenter
	// AlignEnd pins the child to the cell's trailing edge at its
	// natural size.
	AlignEnd
)

// gridChild is one Grid child: the widget, the cell it is attached to,
// how many cells it spans, and how it sits inside the spanned run.
type gridChild struct {
	w                Widget
	col, row         int
	colSpan, rowSpan int
	hAlign, vAlign   Align
	// nat is the child's last measured natural size; the align pass
	// reads it the way Box reads childEntry.nat.
	nat Size
}

// Grid is a rows-and-columns container. Children are attached to cells -
// optionally spanning several - and the grid derives its tracks from the
// children: a column or row is as wide or tall as the tallest child
// claiming it alone, plus what spanning children need.
//
// Sizing per axis: natural mode keeps every track at its own maximum;
// homogeneous mode gives every track the largest track's extent.
//
// When the arranged rect is larger than the natural size, the surplus is
// split equally among the tracks (earlier tracks take the remainder), so
// a spanning child grows by the share of the tracks it spans. Tracks
// never shrink below their maxima: an under-sized rect lets content
// overflow the grid, and the painter's clip decides visibility, matching
// Box.
//
// Children paint in attach order, so a later attach overlapping an
// earlier span paints on top.
type Grid struct {
	node
	colSpacing int
	rowSpacing int
	colHomog   bool
	rowHomog   bool
	child      []*gridChild

	// colW/rowH are the per-track extents Measure computed; Arrange
	// expands them into the final rect. Box keeps the same
	// measure-then-arrange contract through childEntry.nat.
	colW, rowH []int
}

// NewGrid returns an empty grid with the given spacing between columns
// and between rows.
func NewGrid(colSpacing, rowSpacing int) *Grid {
	return &Grid{colSpacing: colSpacing, rowSpacing: rowSpacing}
}

// SetColumnSpacing sets the gap between columns and returns the grid for
// chaining. The measure cache drops so the next frame reflows.
func (g *Grid) SetColumnSpacing(n int) *Grid {
	g.colSpacing = n
	g.InvalidateLayout()
	return g
}

// SetRowSpacing sets the gap between rows and returns the grid for
// chaining.
func (g *Grid) SetRowSpacing(n int) *Grid {
	g.rowSpacing = n
	g.InvalidateLayout()
	return g
}

// SetColumnHomogeneous toggles whether every column takes the widest
// column's extent (true) or its own maximum (false, the default).
func (g *Grid) SetColumnHomogeneous(h bool) *Grid {
	g.colHomog = h
	g.InvalidateLayout()
	return g
}

// SetRowHomogeneous toggles whether every row takes the tallest row's
// extent (true) or its own maximum (false, the default).
func (g *Grid) SetRowHomogeneous(h bool) *Grid {
	g.rowHomog = h
	g.InvalidateLayout()
	return g
}

// Attach puts w at (col, row) with the given spans and returns the grid
// for chaining. Spans below one and negative coordinates clamp to one
// and zero respectively.
//
// Two conflict rules, both last-wins:
//   - attaching a widget that is already attached moves it to the new
//     cell (a widget sits in the grid once);
//   - after the dust settles, whoever occupies the target cell that is
//     not the attached widget is detached - including a widget the move
//     lands on top of.
//
// Overlapping a span without sharing its origin cell is not a conflict:
// both children stay, the later attach paints and hits on top. Every
// attach drops the measure cache.
func (g *Grid) Attach(w Widget, col, row, colSpan, rowSpan int) *Grid {
	col, row = max(0, col), max(0, row)
	colSpan, rowSpan = max(1, colSpan), max(1, rowSpan)
	moved := false
	for _, c := range g.child {
		if c.w == w {
			c.col, c.row, c.colSpan, c.rowSpan = col, row, colSpan, rowSpan
			moved = true
			break
		}
	}
	if !moved {
		g.child = append(g.child, &gridChild{
			w: w, col: col, row: row, colSpan: colSpan, rowSpan: rowSpan,
		})
	}
	g.child = slices.DeleteFunc(g.child, func(c *gridChild) bool {
		return c.w != w && c.col == col && c.row == row
	})
	g.InvalidateLayout()
	return g
}

// SetAlign selects how w sits inside its spanned run along each axis.
// Unknown widgets are ignored.
func (g *Grid) SetAlign(w Widget, h, v Align) *Grid {
	for _, c := range g.child {
		if c.w == w {
			c.hAlign, c.vAlign = h, v
			g.Invalidate()
			return g
		}
	}
	return g
}

// Remove detaches w and reports whether it was attached. The measure
// cache drops so the next frame reflows without it.
func (g *Grid) Remove(w Widget) bool {
	for i, c := range g.child {
		if c.w == w {
			g.child = slices.Delete(g.child, i, i+1)
			g.InvalidateLayout()
			return true
		}
	}
	return false
}

// Children exposes the children in row-major order - the visual order
// focus traversal follows, so Tab reads the grid left-to-right down the
// rows, a spanning child visited at its origin cell - regardless of the
// order attachments arrived in.
func (g *Grid) Children() []Widget {
	sorted := slices.Clone(g.child)
	slices.SortStableFunc(sorted, func(a, b *gridChild) int {
		if a.row != b.row {
			return a.row - b.row
		}
		return a.col - b.col
	})
	out := make([]Widget, len(sorted))
	for i, c := range sorted {
		out[i] = c.w
	}
	return out
}

// extents reports the track counts: one past the farthest occupied
// cell. A grid with no children is 0x0 and measures and arranges as
// nothing.
func (g *Grid) extents() (cols, rows int) {
	for _, c := range g.child {
		cols = max(cols, c.col+c.colSpan)
		rows = max(rows, c.row+c.rowSpan)
	}
	return cols, rows
}

// Measure measures every child and derives the tracks: single-cell
// children set their track's maximum, then spanning children grow a run
// of tracks by whatever their natural size exceeds the run's current
// maxima plus internal spacing - the deficit spreads equally over the
// run, earlier tracks taking the remainder. The result is cached until
// an InvalidateLayout anywhere in the subtree or a different constraint
// arrives.
func (g *Grid) Measure(con Constraints) Size {
	if sz, ok := g.measureHit(con); ok {
		return sz
	}
	cols, rows := g.extents()
	g.colW = make([]int, cols)
	g.rowH = make([]int, rows)
	for _, c := range g.child {
		c.nat = c.w.Measure(Constraints{Max: con.Max})
	}
	for _, c := range g.child {
		if c.colSpan == 1 {
			g.colW[c.col] = max(g.colW[c.col], c.nat.W)
		}
		if c.rowSpan == 1 {
			g.rowH[c.row] = max(g.rowH[c.row], c.nat.H)
		}
	}
	for _, c := range g.child {
		growTracks(g.colW, g.colSpacing, c.col, c.colSpan, c.nat.W)
		growTracks(g.rowH, g.rowSpacing, c.row, c.rowSpan, c.nat.H)
	}
	if g.colHomog {
		evenTracks(g.colW)
	}
	if g.rowHomog {
		evenTracks(g.rowH)
	}
	nat := Size{
		W: trackSum(g.colW) + g.colSpacing*(cols-1),
		H: trackSum(g.rowH) + g.rowSpacing*(rows-1),
	}
	return g.measureStore(con, clampSize(nat, con))
}

// growTracks widens the run of n tracks starting at i so want fits: the
// deficit beyond the run's current maxima plus internal spacing spreads
// equally over the run, earlier tracks taking the remainder.
func growTracks(tracks []int, spacing, i, n, want int) {
	if n <= 0 {
		return
	}
	run := 0
	for k := i; k < i+n; k++ {
		run += tracks[k]
	}
	run += spacing * (n - 1)
	deficit := want - run
	if deficit <= 0 {
		return
	}
	per, rem := deficit/n, deficit%n
	for k := range n {
		tracks[i+k] += per
		if k < rem {
			tracks[i+k]++
		}
	}
}

// evenTracks gives every track the largest track's extent.
func evenTracks(tracks []int) {
	best := 0
	for _, t := range tracks {
		best = max(best, t)
	}
	for i := range tracks {
		tracks[i] = best
	}
}

// trackSum adds the track extents up.
func trackSum(tracks []int) int {
	total := 0
	for _, t := range tracks {
		total += t
	}
	return total
}

// Arrange assigns every child its spanned run, sized by the expanded
// tracks, aligned inside it per its Align. The surplus beyond the
// natural size splits equally among the tracks (earlier tracks take the
// remainder); a smaller rect leaves the tracks at their maxima.
func (g *Grid) Arrange(r render.Rect) {
	g.ArrangeRoot(r)
	cols, rows := g.extents()
	if cols == 0 {
		return
	}
	if len(g.colW) != cols || len(g.rowH) != rows {
		// Arrange follows Measure the way Box reads childEntry.nat;
		// without measured tracks there is nothing sane to place, so
		// the children park at empty rects instead of panicking on
		// the missing slices.
		for _, c := range g.child {
			c.w.Arrange(render.Rect{})
		}
		return
	}
	expandTracks(g.colW, g.colSpacing, r.W)
	expandTracks(g.rowH, g.rowSpacing, r.H)
	for _, c := range g.child {
		x, cw := trackExtent(g.colW, g.colSpacing, c.col, c.colSpan)
		y, ch := trackExtent(g.rowH, g.rowSpacing, c.row, c.rowSpan)
		cell := render.Rect{X: r.X + x, Y: r.Y + y, W: cw, H: ch}
		c.w.Arrange(alignRect(cell, c.nat, c.hAlign, c.vAlign))
	}
	setParents(g, g.Children()...)
}

// ArrangeRoot records the grid's own rect.
func (g *Grid) ArrangeRoot(r render.Rect) {
	g.node.Arrange(r)
}

// expandTracks splits the surplus beyond the tracks' natural maxima
// equally over every track, earlier tracks taking the remainder. An
// avail below natural leaves the tracks alone, so content overflows.
func expandTracks(tracks []int, spacing, avail int) {
	if len(tracks) == 0 {
		return
	}
	natural := trackSum(tracks) + spacing*(len(tracks)-1)
	extra := max(0, avail-natural)
	per, rem := extra/len(tracks), extra%len(tracks)
	for i := range tracks {
		tracks[i] += per
		if i < rem {
			tracks[i]++
		}
	}
}

// trackExtent returns the offset and size of the run of n tracks
// starting at i, spacing included inside the run but not before it.
func trackExtent(tracks []int, spacing, i, n int) (off, size int) {
	for k := range i {
		off += tracks[k] + spacing
	}
	for k := range n {
		size += tracks[i+k]
	}
	size += spacing * (n - 1)
	return off, size
}

// alignRect fits a child of natural size nat into its cell per the
// per-axis alignment: fill takes the whole cell, the rest keep the
// natural size and pin to an edge or center - a natural size larger
// than the cell overflows it rather than shrinking.
func alignRect(cell render.Rect, nat Size, h, v Align) render.Rect {
	out := cell
	if h != AlignFill {
		out.W = nat.W
		switch h {
		case AlignCenter:
			out.X = cell.X + (cell.W-nat.W)/2
		case AlignEnd:
			out.X = cell.X + cell.W - nat.W
		}
	}
	if v != AlignFill {
		out.H = nat.H
		switch v {
		case AlignCenter:
			out.Y = cell.Y + (cell.H-nat.H)/2
		case AlignEnd:
			out.Y = cell.Y + cell.H - nat.H
		}
	}
	return out
}

// Paint paints the children in attach order, so overlapping spans
// composite bottom to top.
func (g *Grid) Paint(cv *render.Canvas) {
	for _, c := range g.child {
		c.w.Paint(cv)
	}
}

// HitTest returns the topmost child under p (later attaches sit on
// top), or the grid itself when p is inside its bounds but over no
// child.
func (g *Grid) HitTest(p Point) Widget {
	for i := range slices.Backward(g.child) {
		if hit := g.child[i].w.HitTest(p); hit != nil {
			return hit
		}
	}
	return g.HitLeaf(g, p)
}
