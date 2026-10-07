package widget

import (
	"time"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/render"
)

// The adaptive vocabulary of #94: Clamp, BreakpointBin, ViewSwitcher,
// StackSwitcher, and Carousel - the pieces an app reshapes itself
// with when the window narrows. NavigationView/SplitView and
// OverlaySplitView live in navigation.go.

// Clamp centers its child at a maximum width: wider allocations pad
// the sides, narrower ones pass through untouched (the adw Clamp, a
// pure measure wrapper).
type Clamp struct {
	composite
	max int
}

// NewClamp returns a clamp over child capping its width at max.
func NewClamp(max int, child Widget) *Clamp {
	c := &Clamp{max: max}
	c.initComposite(c, child)
	return c
}

// SetMaximum changes the cap.
func (c *Clamp) SetMaximum(max int) {
	c.max = max
	c.InvalidateLayout()
}

// Maximum reports the cap.
func (c *Clamp) Maximum() int { return c.max }

// Measure measures the child capped.
func (c *Clamp) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	inner := con
	if c.max > 0 {
		inner.Max.W = min(inner.Max.W, c.max)
	}
	sz := c.root.Measure(inner)
	return c.measureStore(con, clampSize(Size{W: con.Max.W, H: sz.H}, con))
}

// Arrange centers the child's rect inside r.
func (c *Clamp) Arrange(r render.Rect) {
	c.node.Arrange(r)
	w := r.W
	if c.max > 0 {
		w = min(w, c.max)
	}
	c.root.Arrange(render.Rect{X: r.X + (r.W-w)/2, Y: r.Y, W: w, H: r.H})
	setParents(c, c.root)
}

// Breakpoint applies while the allocated width is at or below Max
// (or at or above Min, when Max is zero) - the adw 1.4 condition
// model narrowed to the width axis the adaptive shell needs.
type Breakpoint struct {
	Max int
	Min int
	// Apply runs when the breakpoint starts holding; Unapply, when it
	// stops. Batches of setters are just closures.
	Apply   func()
	Unapply func()
}

// holds reports whether width satisfies the condition.
func (b Breakpoint) holds(width int) bool {
	switch {
	case b.Max > 0 && b.Min > 0:
		return width <= b.Max && width >= b.Min
	case b.Max > 0:
		return width <= b.Max
	case b.Min > 0:
		return width >= b.Min
	}
	return false
}

// breakpoints tracks a set of Breakpoints against an allocated width:
// the one condition engine BreakpointBin and the split views share.
// evaluate runs each breakpoint's Apply or Unapply on the transitions
// only - an unchanged width, or a width on the same side of every
// condition, runs nothing.
type breakpoints struct {
	points []Breakpoint
	held   []bool
	width  int
	seen   bool
}

// add appends a breakpoint; evaluation order is add order.
func (b *breakpoints) add(bp Breakpoint) {
	b.points = append(b.points, bp)
	b.held = append(b.held, false)
	b.seen = false // re-evaluate the new point at the next width
}

// evaluate applies the conditions to width.
func (b *breakpoints) evaluate(width int) {
	if b.seen && b.width == width {
		return
	}
	b.seen, b.width = true, width
	for i, bp := range b.points {
		now := bp.holds(width)
		if now == b.held[i] {
			continue
		}
		b.held[i] = now
		if now && bp.Apply != nil {
			bp.Apply()
		} else if !now && bp.Unapply != nil {
			bp.Unapply()
		}
	}
}

// BreakpointBin wraps one widget and runs its breakpoints' apply
// batches as the allocated width crosses them - the seam an adaptive
// shell swaps layouts through. Conditions evaluate at the top of
// Arrange, before the child is laid out, so a batch's changes land in
// the same pass; a batch that changes what the child wants calls
// InvalidateLayout and the engine re-measures on demand.
type BreakpointBin struct {
	composite
	bps breakpoints
}

// NewBreakpointBin returns a bin over child with no breakpoints.
func NewBreakpointBin(child Widget) *BreakpointBin {
	b := &BreakpointBin{}
	b.initComposite(b, child)
	return b
}

// Add appends a breakpoint; apply order is add order.
func (b *BreakpointBin) Add(bp Breakpoint) {
	b.bps.add(bp)
	b.InvalidateLayout()
}

// Width reports the last allocated width (the condition input).
func (b *BreakpointBin) Width() int { return b.bps.width }

// Arrange evaluates every breakpoint against the allocated width,
// then lays the child out.
func (b *BreakpointBin) Arrange(r render.Rect) {
	b.bps.evaluate(r.W)
	b.composite.Arrange(r)
}

// ViewSwitcher is the pill strip bound to a Stack: one button per
// page (title, and an icon when given), the visible page's button
// :checked, a click switching pages - the adw pill for bottom bars
// and headers alike.
type ViewSwitcher struct {
	composite
	stack   *Stack
	face    render.Font
	sizePx  float64
	titles  map[string]string
	icons   map[string]string
	row     *Box
	buttons []*Button
	policy  StackSwitcherPolicy
}

// StackSwitcherPolicy decides which switcher buttons show.
type StackSwitcherPolicy uint8

// Switcher policies.
const (
	// SwitcherAll shows every page's button.
	SwitcherAll StackSwitcherPolicy = iota
	// SwitcherText titles only (no icons), the narrow variant.
	SwitcherText
	// SwitcherIcons icons only, the bottom-bar variant.
	SwitcherIcons
)

// NewViewSwitcher returns a strip driving stack; titles names the
// pages (missing names fall back to the page names), icons is
// optional.
func NewViewSwitcher(face render.Font, sizePx float64, stack *Stack, titles, icons map[string]string) *ViewSwitcher {
	face = requireFace("widget.NewViewSwitcher", face)
	v := &ViewSwitcher{stack: stack, face: face, sizePx: sizePx, titles: titles, icons: icons}
	v.SetElement("viewswitcher")
	v.row = NewBox(Row, 4, 4)
	v.initComposite(v, v.row)
	v.fillWidth = true
	v.surface, v.surfaceRadius = surfaceFill, 999
	v.sync()
	return v
}

// SetPolicy changes what the buttons show and rebuilds.
func (v *ViewSwitcher) SetPolicy(p StackSwitcherPolicy) {
	v.policy = p
	v.sync()
}

// sync rebuilds the strip over the stack's pages.
func (v *ViewSwitcher) sync() {
	v.row.Clear()
	v.buttons = nil
	th := Current()
	for _, name := range v.stack.Order() {
		title := name
		if t, ok := v.titles[name]; ok {
			title = t
		}
		icon := v.icons[name]
		var child Widget
		switch {
		case v.policy == SwitcherIcons && icon != "":
			child = NewThemeIcon(icon, int(v.sizePx))
		case v.policy == SwitcherText || icon == "":
			child = NewLabel(v.face, v.sizePx, title, th.Text)
		default:
			row := NewBox(Row, 6, 0)
			row.Append(NewThemeIcon(icon, int(v.sizePx)), false)
			row.Append(NewLabel(v.face, v.sizePx, title, th.Text), false)
			child = row
		}
		btn := NewButton(child, 6, 999)
		which := name
		btn.OnClick = func() { v.stack.Show(which) }
		markChecked(btn, name == v.stack.Visible())
		v.buttons = append(v.buttons, btn)
		v.row.Append(btn, false)
	}
	v.InvalidateLayout()
}

// Sync rebuilds after the stack's pages changed (public: the stack
// has no signal, the app calls this after Add).
func (v *ViewSwitcher) Sync() { v.sync() }

// ReflectVisible repaints the checked states after a programmatic
// stack.Show (same condition: the app drives, the switcher shows).
func (v *ViewSwitcher) ReflectVisible() {
	for i, name := range v.stack.Order() {
		if i < len(v.buttons) {
			markChecked(v.buttons[i], name == v.stack.Visible())
		}
	}
	v.Invalidate()
}

// markChecked marks a switcher button as the visible page's: the
// :checked state for stylesheets, and the theme's selected fill so an
// unstyled strip shows which page is current.
func markChecked(b *Button, on bool) {
	b.SetState(StateChecked, on)
	b.BgExplicit = on
	b.Bg, b.BgHover, b.BgPressed = 0, 0, 0
	if on {
		th := Current()
		b.Bg, b.BgHover, b.BgPressed = th.SurfaceHover, th.SurfaceHover, th.SurfacePressed
	}
	b.Invalidate()
}

// StackSwitcher is the tab strip bound to a Stack: underline-marked
// text tabs, GTK's classic - the ViewSwitcher's sibling for windows.
type StackSwitcher struct {
	*ViewSwitcher
}

// NewStackSwitcher returns a tab strip driving stack.
func NewStackSwitcher(face render.Font, sizePx float64, stack *Stack, titles map[string]string) *StackSwitcher {
	s := &StackSwitcher{ViewSwitcher: NewViewSwitcher(face, sizePx, stack, titles, nil)}
	s.SetPolicy(SwitcherText)
	return s
}

// Carousel is a horizontal swipeable page strip: pages side by side
// at the carousel's width, pointer drags pan (the touch ticket adds
// gestures; pointer drag works today), releases snap to the nearest
// page on the animation clock, and the painted dots index position.
type Carousel struct {
	node
	face   render.Font
	sizePx float64
	pages  []Widget
	offset float64 // in pages, fractional
	cancel anim.Cancel

	// drag state: the press position and the page offset it started
	// from.
	pressX   int
	dragFrom float64
	dragging bool

	// OnPage fires when the settled page changes.
	OnPage func(i int)
}

// NewCarousel returns an empty carousel.
func NewCarousel(face render.Font, sizePx float64) *Carousel {
	face = requireFace("widget.NewCarousel", face)
	return &Carousel{face: face, sizePx: sizePx, offset: 0}
}

// Append adds a page at the end.
func (c *Carousel) Append(w Widget) {
	c.pages = append(c.pages, w)
	c.InvalidateLayout()
}

// Pages reports the page count.
func (c *Carousel) Pages() int { return len(c.pages) }

// Page shows page i (animated snap).
func (c *Carousel) Page() int { return int(c.offset + 0.5) }

// SetPage snaps to page i without animation.
func (c *Carousel) SetPage(i int) {
	i = max(0, min(i, len(c.pages)-1))
	c.offset = float64(i)
	c.Invalidate()
	if c.OnPage != nil {
		c.OnPage(i)
	}
}

// Measure wants each page's height at the full width, the tallest
// wins.
func (c *Carousel) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	best := Size{}
	for _, p := range c.pages {
		s := p.Measure(con)
		best.H = max(best.H, s.H)
	}
	best.W = con.Max.W
	return c.measureStore(con, clampSize(best, con))
}

// Arrange lays the pages side by side at the carousel's width,
// offset by the fractional page position.
func (c *Carousel) Arrange(r render.Rect) {
	c.node.Arrange(r)
	x := r.X - int(c.offset*float64(r.W))
	for _, p := range c.pages {
		p.Arrange(render.Rect{X: x, Y: r.Y, W: r.W, H: r.H})
		x += r.W
	}
	setParents(c, c.pages...)
}

// Paint draws the visible pages clipped, then the indicator dots.
func (c *Carousel) Paint(cv *render.Canvas) {
	th := Current()
	prev := cv.PushClip(c.bounds)
	for _, p := range c.pages {
		if b, ok := p.(Boundser); ok && b.Bounds().X < c.bounds.X+c.bounds.W && b.Bounds().X+b.Bounds().W > c.bounds.X {
			PaintChild(cv, p)
		}
	}
	cv.PopClip(prev)
	if n := len(c.pages); n > 1 {
		dot := 5
		total := n * (dot*2 + 4)
		x := c.bounds.X + (c.bounds.W-total)/2
		y := c.bounds.Y + c.bounds.H - dot*3
		for i := range n {
			col := th.Border
			if i == c.Page() {
				col = th.Accent
			}
			cv.FillRect(render.Rect{X: x + dot, Y: y, W: dot, H: dot}, col)
			x += dot*2 + 4
		}
	}
}

// HitTest resolves inside the carousel.
func (c *Carousel) HitTest(p Point) Widget {
	if c.bounds.Contains(p.X, p.Y) {
		return c
	}
	return nil
}

// PressAt records the drag start.
func (c *Carousel) PressAt(p Point) {
	c.pressX = p.X
	c.dragFrom = c.offset
	c.dragging = true
}

// DragMove pans the pages with the pointer.
func (c *Carousel) DragMove(p Point) {
	if !c.dragging || c.bounds.W == 0 {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.offset = clamp01pages(c.dragFrom+float64(c.pressX-p.X)/float64(c.bounds.W), len(c.pages))
	c.Invalidate()
}

// ReleaseAt snaps to the nearest page.
func (c *Carousel) ReleaseAt(p Point) {
	if !c.dragging {
		return
	}
	c.dragging = false
	c.snap()
}

// snap animates the offset to the nearest integer page.
func (c *Carousel) snap() {
	target := float64(max(0, min(len(c.pages)-1, int(c.offset+0.5))))
	from := c.offset
	if c.cancel != nil {
		c.cancel()
	}
	page := int(target)
	c.cancel = anim.Start(220*time.Millisecond, func(t float64) {
		c.offset = from + (target-from)*easeOut(t)
		c.Invalidate()
	})
	if c.OnPage != nil {
		c.OnPage(page)
	}
}

// easeOut is the snap curve.
func easeOut(t float64) float64 { return 1 - (1-t)*(1-t) }

// clamp01pages keeps the fractional offset inside the pages.
func clamp01pages(v float64, pages int) float64 {
	if pages <= 1 {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > float64(pages-1) {
		return float64(pages - 1)
	}
	return v
}
