package widget

import (
	"math"
	"slices"

	"github.com/stubbedev/gelm/render"
)

// Stack holds named children and shows exactly one at a time.
type Stack struct {
	node
	kids     map[string]Widget
	order    []string
	visible  string
	measured map[string]Size
}

// NewStack returns an empty stack.
func NewStack() *Stack {
	return &Stack{
		kids:     make(map[string]Widget),
		measured: make(map[string]Size),
	}
}

// Add puts a child under name; adding an existing name replaces it.
func (s *Stack) Add(name string, w Widget) *Stack {
	if _, ok := s.kids[name]; !ok {
		s.order = append(s.order, name)
	}
	s.kids[name] = w
	if s.visible == "" {
		s.visible = name
	}
	return s
}

// Show makes the child under name the visible one. Unknown names are
// ignored.
func (s *Stack) Show(name string) {
	if _, ok := s.kids[name]; ok {
		s.visible = name
	}
}

// Visible returns the visible child's name.
func (s *Stack) Visible() string {
	return s.visible
}

// Measure measures every child once and reports the largest, clamped to
// con.
func (s *Stack) Measure(con Constraints) Size {
	best := Size{}
	for _, name := range s.order {
		nat := s.kids[name].Measure(con)
		s.measured[name] = nat
		best.W = max(best.W, nat.W)
		best.H = max(best.H, nat.H)
	}
	return clampSize(best, con)
}

// Arrange assigns the whole rect to every child.
func (s *Stack) Arrange(r render.Rect) {
	s.ArrangeRoot(r)
	for _, name := range s.order {
		s.kids[name].Arrange(r)
		setParents(s, s.kids[name])
	}
}

// ArrangeRoot records the stack's own rect.
func (s *Stack) ArrangeRoot(r render.Rect) {
	s.node.Arrange(r)
}

// Paint draws only the visible child.
func (s *Stack) Paint(cv *render.Canvas) {
	if w, ok := s.kids[s.visible]; ok {
		w.Paint(cv)
	}
}

// HitTest returns the visible child under p, or the stack inside its
// bounds.
func (s *Stack) HitTest(p Point) Widget {
	if w, ok := s.kids[s.visible]; ok {
		if hit := w.HitTest(p); hit != nil {
			return hit
		}
	}
	return s.HitLeaf(s, p)
}

// Overlay stacks all children on the same rect and paints them in add
// order. The last child paints on top and wins hit tests.
type Overlay struct {
	node
	kids []Widget
}

// NewOverlay returns an empty overlay.
func NewOverlay() *Overlay { return &Overlay{} }

// Append adds a child on top.
func (o *Overlay) Append(w Widget) *Overlay {
	o.kids = append(o.kids, w)
	return o
}

// Measure reports the largest child, clamped to con.
func (o *Overlay) Measure(con Constraints) Size {
	best := Size{}
	for _, k := range o.kids {
		nat := k.Measure(con)
		best.W = max(best.W, nat.W)
		best.H = max(best.H, nat.H)
	}
	return clampSize(best, con)
}

// Arrange assigns the whole rect to every child.
func (o *Overlay) Arrange(r render.Rect) {
	o.ArrangeRoot(r)
	for _, k := range o.kids {
		k.Arrange(r)
		setParents(o, k)
	}
}

// ArrangeRoot records the overlay's own rect.
func (o *Overlay) ArrangeRoot(r render.Rect) {
	o.node.Arrange(r)
}

// Paint draws every child bottom to top.
func (o *Overlay) Paint(cv *render.Canvas) {
	for _, k := range o.kids {
		k.Paint(cv)
	}
}

// HitTest returns the topmost child under p, or the overlay inside its
// bounds.
func (o *Overlay) HitTest(p Point) Widget {
	for i := range slices.Backward(o.kids) {
		if hit := o.kids[i].HitTest(p); hit != nil {
			return hit
		}
	}
	return o.HitLeaf(o, p)
}

// Scroll embeds a child larger than the viewport and shifts it by an
// offset, clipping to its own bounds and painting scrollbars.
type Scroll struct {
	node
	child      Widget
	nat        Size
	offX, offY int

	// ShowBars toggles the painted scrollbar indicators.
	ShowBars bool
}

// NewScroll wraps child in a scrollable viewport.
func NewScroll(child Widget) *Scroll {
	return &Scroll{child: child}
}

// Offset returns the current scroll offset.
func (s *Scroll) Offset() (int, int) {
	return s.offX, s.offY
}

// SetOffset scrolls to x, y, clamped so the child never leaves the
// viewport. A child smaller than the viewport stays pinned at 0.
func (s *Scroll) SetOffset(x, y int) {
	maxX := max(0, s.nat.W-s.bounds.W)
	maxY := max(0, s.nat.H-s.bounds.H)
	s.offX = min(max(0, x), maxX)
	s.offY = min(max(0, y), maxY)
}

// Measure measures the child without the viewport's limits to learn its
// natural size, then reports the viewport as filling whatever the parent
// offers, clamped to con.
func (s *Scroll) Measure(con Constraints) Size {
	s.nat = s.child.Measure(Constraints{Max: Size{W: math.MaxInt, H: math.MaxInt}})
	return clampSize(con.Max, con)
}

// Arrange pins the viewport to r and places the child at the negative
// offset, at its natural size, so a smaller child leaves the rest of the
// viewport to the scroll itself.
func (s *Scroll) Arrange(r render.Rect) {
	s.ArrangeRoot(r)
	s.child.Arrange(render.Rect{
		X: r.X - s.offX,
		Y: r.Y - s.offY,
		W: s.nat.W,
		H: s.nat.H,
	})
	setParents(s, s.child)
}

// ArrangeRoot records the viewport rect.
func (s *Scroll) ArrangeRoot(r render.Rect) {
	s.node.Arrange(r)
}

// Paint clips to the viewport, paints the shifted child, and draws the
// scrollbar indicators when ShowBars is set and the child overflows.
func (s *Scroll) Paint(cv *render.Canvas) {
	prev := cv.PushClip(s.bounds)
	s.child.Paint(cv)
	if s.ShowBars {
		bar := render.RGB(0x58, 0x5b, 0x70)
		if s.nat.W > s.bounds.W && s.bounds.W > 0 {
			w := s.bounds.W * s.bounds.W / s.nat.W
			x := s.bounds.X + (s.bounds.W-w)*s.offX/max(1, s.nat.W-s.bounds.W)
			cv.RoundedRect(render.Rect{X: x, Y: s.bounds.Y + s.bounds.H - 4, W: max(8, w), H: 3}, 1, bar)
		}
		if s.nat.H > s.bounds.H && s.bounds.H > 0 {
			h := s.bounds.H * s.bounds.H / s.nat.H
			y := s.bounds.Y + (s.bounds.H-h)*s.offY/max(1, s.nat.H-s.bounds.H)
			cv.RoundedRect(render.Rect{X: s.bounds.X + s.bounds.W - 4, Y: y, W: 3, H: max(8, h)}, 1, bar)
		}
	}
	cv.PopClip(prev)
}

// HitTest returns the child under p inside the viewport, or the scroll
// itself. The child's bounds already carry the scroll offset.
func (s *Scroll) HitTest(p Point) Widget {
	if !s.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if hit := s.child.HitTest(p); hit != nil {
		return hit
	}
	return s
}

// ScrollBy shifts the offset by dy rows of 40px.
func (s *Scroll) ScrollBy(dy int) {
	s.SetOffset(s.offX, s.offY+dy*40)
}
