package widget

import (
	"math"
	"slices"
	"time"

	"github.com/stubbedev/gelm/internal/anim"
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
	s.InvalidateLayout()
	return s
}

// Children exposes the visible child for focus traversal.
func (s *Stack) Children() []Widget {
	if k, ok := s.kids[s.visible]; ok {
		return []Widget{k}
	}
	return nil
}

// Children exposes the stacked children for focus traversal.
func (o *Overlay) Children() []Widget { return o.kids }

// Show makes the child under name the visible one and invalidates the
// stack's bounds; unknown names are ignored.
func (s *Stack) Show(name string) {
	if _, ok := s.kids[name]; ok && name != s.visible {
		s.visible = name
		s.Invalidate()
	}
}

// Visible returns the visible child's name.
func (s *Stack) Visible() string {
	return s.visible
}

// Measure measures every child once and reports the largest, clamped to
// con.
func (s *Stack) Measure(con Constraints) Size {
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	best := Size{}
	for _, name := range s.order {
		nat := s.kids[name].Measure(con)
		s.measured[name] = nat
		best.W = max(best.W, nat.W)
		best.H = max(best.H, nat.H)
	}
	return s.measureStore(con, clampSize(best, con))
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
	o.InvalidateLayout()
	return o
}

// Measure reports the largest child, clamped to con.
func (o *Overlay) Measure(con Constraints) Size {
	if sz, ok := o.measureHit(con); ok {
		return sz
	}
	best := Size{}
	for _, k := range o.kids {
		nat := k.Measure(con)
		best.W = max(best.W, nat.W)
		best.H = max(best.H, nat.H)
	}
	return o.measureStore(con, clampSize(best, con))
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
// offset. Overflowing axes reserve a scrollbar gutter; the bars fade
// in on hover or scrolling and out after a dwell, drag 1:1, and gutter
// clicks page. A child smaller than the viewport is centered, or
// stretched along an axis when the matching Fill flag is set.
type Scroll struct {
	node
	child      Widget
	nat        Size
	offX, offY int

	// ShowBars enables the auto-hiding scrollbar indicators.
	ShowBars bool
	// OnScrolled fires when a wheel, bar drag, or keyboard scroll lands
	// at a NEW offset (same-offset scrolls stay silent). Nil means
	// nobody listens.
	OnScrolled func(x, y int)
	// FillX and FillY stretch a smaller child across the viewport
	// instead of centering it.
	FillX, FillY bool

	// viewW/viewH is the child area inside the reserved gutters.
	viewW, viewH   int
	childX, childY int

	// drag state: which handle is grabbed and where in it the press
	// landed, so the content tracks the pointer 1:1.
	dragV, dragH  bool
	dragGrab      int
	dragStartOffX int
	dragStartOffY int

	// auto-hide: bar alpha 0..1, the time of the last scroll or hover
	// change, and the running fade's Cancel so a fresh fade replaces
	// the previous one mid-flight.
	alpha      float64
	hovered    bool
	lastInput  time.Time
	cancelFade anim.Cancel
}

// gutter is the scrollbar strip width reserved inside the viewport.
const gutter = 8

// barDwell is how long the bars stay after the last scroll or hover;
// barFade is the fade duration through the anim package.
const (
	barDwell = 1200 * time.Millisecond
	barFade  = 220 * time.Millisecond
)

// Children exposes the wrapped child for focus traversal.
func (s *Scroll) Children() []Widget { return []Widget{s.child} }

// NewScroll wraps child in a scrollable viewport.
func NewScroll(child Widget) *Scroll {
	return &Scroll{child: child}
}

// Offset returns the current scroll offset.
func (s *Scroll) Offset() (int, int) {
	return s.offX, s.offY
}

// scrollMax reports the scrollable range per axis.
func (s *Scroll) scrollMax() (int, int) {
	return max(0, s.nat.W-s.viewW), max(0, s.nat.H-s.viewH)
}

// SetOffset scrolls to x, y, clamped so the child never leaves the
// viewport (overshoot clamps). A child smaller than the viewport stays
// pinned at 0. Scrolling invalidates the viewport bounds.
func (s *Scroll) SetOffset(x, y int) {
	maxX, maxY := s.scrollMax()
	x = min(max(0, x), maxX)
	y = min(max(0, y), maxY)
	if x == s.offX && y == s.offY {
		return
	}
	s.offX, s.offY = x, y
	s.Invalidate()
	if s.OnScrolled != nil {
		s.OnScrolled(x, y)
	}
}

// ShowBarsOnce marks the bars visible now and schedules their fade-out:
// called on scroll and hover activity.
func (s *Scroll) showBars() {
	if !s.ShowBars {
		return
	}
	s.lastInput = time.Now()
	s.fadeTo(1)
}

// fadeTo animates the bar alpha toward to, canceling any fade still
// running: the new tween's first callback takes over from the old
// one's last value. Each step invalidates the gutter strips so the
// fading bars repaint.
func (s *Scroll) fadeTo(to float64) {
	if (to == 1 && s.alpha == 1) || (to == 0 && s.alpha == 0) {
		return
	}
	if s.cancelFade != nil {
		s.cancelFade()
	}
	from := s.alpha
	s.cancelFade = anim.Start(barFade, func(t float64) {
		s.alpha = from + (to-from)*t
		s.invalidateBars()
	})
}

// SetHovered implements HoverSetter: entering shows the bars, leaving
// starts the dwell before they fade. Hovering itself paints nothing;
// the fade tweens invalidate the gutter strips as alpha moves.
func (s *Scroll) SetHovered(on bool) {
	s.hovered = on
	if on {
		s.showBars()
	}
}

// invalidateBars schedules a repaint of just the scrollbar strips -
// the only pixels an alpha change touches - instead of the whole
// viewport.
func (s *Scroll) invalidateBars() {
	if !s.ShowBars {
		return
	}
	if track, _ := s.vBarGeometry(); track.W > 0 {
		s.InvalidateRect(track)
	}
	if track, _ := s.hBarGeometry(); track.H > 0 {
		s.InvalidateRect(track)
	}
}

// Measure reports the child's natural size clamped to the offered
// constraints. The viewport itself is sized by Arrange (expanding
// children and cross stretch), so the reported natural size must stay
// bounded: an unbounded appetite here would blow up natural measurement
// in parent boxes and lay children out past the window edge.
func (s *Scroll) Measure(con Constraints) Size {
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	s.nat = s.child.Measure(Constraints{Max: Size{W: math.MaxInt, H: math.MaxInt}})
	return s.measureStore(con, clampSize(s.nat, con))
}

// Arrange pins the viewport to r, reserving a gutter for each
// overflowing axis, and places the child at the negative offset:
// stretched across the viewport when Fill is set and the child is
// smaller, centered when it is smaller without fill, otherwise at its
// natural size shifted by the offset.
func (s *Scroll) Arrange(r render.Rect) {
	s.ArrangeRoot(r)
	gutterV, gutterH := 0, 0
	if s.nat.H > r.H && r.H > 40 {
		gutterV = gutter
	}
	if s.nat.W > r.W && r.W > 40 {
		gutterH = gutter
	}
	s.viewW = r.W - gutterV
	s.viewH = r.H - gutterH

	childW, childH := s.nat.W, s.nat.H
	if s.FillX && childW < s.viewW {
		childW = s.viewW
	}
	if s.FillY && childH < s.viewH {
		childH = s.viewH
	}
	childX := 0
	if !s.FillX && childW < s.viewW {
		childX = (s.viewW - childW) / 2
	}
	childY := 0
	if !s.FillY && childH < s.viewH {
		childY = (s.viewH - childH) / 2
	}
	s.childX, s.childY = childX, childY

	s.child.Arrange(render.Rect{
		X: r.X + childX - s.offX,
		Y: r.Y + childY - s.offY,
		W: childW,
		H: childH,
	})
	setParents(s, s.child)
}

// ArrangeRoot records the viewport rect.
func (s *Scroll) ArrangeRoot(r render.Rect) {
	s.node.Arrange(r)
}

// Paint clips to the viewport, paints the shifted child, and draws
// the auto-hiding bars in their gutters at the current fade alpha.
func (s *Scroll) Paint(cv *render.Canvas) {
	prev := cv.PushClip(s.bounds)
	s.child.Paint(cv)
	if s.ShowBars && s.alpha > 0 {
		s.tickFade()
		bar := render.RGB(0x58, 0x5b, 0x70)
		a := uint8(s.alpha * 255)
		tint := render.RGBA(bar.R(), bar.G(), bar.B(), a)
		track, handle := s.vBarGeometry()
		if track.W > 0 {
			cv.RoundedRect(track, 1, render.RGBA(0, 0, 0, a/3))
			cv.RoundedRect(handle, 2, tint)
		}
		track, handle = s.hBarGeometry()
		if track.W > 0 {
			cv.RoundedRect(track, 1, render.RGBA(0, 0, 0, a/3))
			cv.RoundedRect(handle, 2, tint)
		}
	}
	cv.PopClip(prev)
}

// tickFade runs the dwell: with the pointer away and the last scroll
// older than the dwell, it fades the bars out.
func (s *Scroll) tickFade() {
	if s.hovered || time.Since(s.lastInput) < barDwell {
		return
	}
	s.fadeTo(0)
}

// Role implements Roleer.
func (s *Scroll) Role() Role { return RoleScrollArea }

// HitTest returns the widget under p: the scrollbar gutters belong to
// the scroll itself (a press there drags or pages, never hits content),
// the child's bounds already carry the scroll offset so hit-testing
// through a scrolled viewport lands on the right content, and anything
// else inside the bounds is the scroll.
func (s *Scroll) HitTest(p Point) Widget {
	if !s.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if _, handle := s.vBarGeometry(); handle.W > 0 && p.X >= handle.X {
		return s
	}
	if _, handle := s.hBarGeometry(); handle.H > 0 && p.Y >= handle.Y {
		return s
	}
	if hit := s.child.HitTest(p); hit != nil {
		return hit
	}
	return s
}

// vBarGeometry returns the vertical track and handle rects; the track
// width is zero when there is no vertical overflow.
func (s *Scroll) vBarGeometry() (track, handle render.Rect) {
	_, maxY := s.scrollMax()
	if maxY <= 0 || s.viewH <= 0 || s.nat.H <= 0 {
		return track, handle
	}
	track = render.Rect{X: s.bounds.X + s.bounds.W - gutter, Y: s.bounds.Y, W: gutter, H: s.viewH}
	h := max(24, s.viewH*s.viewH/s.nat.H)
	y := track.Y + (track.H-h)*s.offY/max(1, maxY)
	handle = render.Rect{X: track.X, Y: y, W: gutter, H: h}
	return track, handle
}

// hBarGeometry returns the horizontal track and handle rects; the
// track height is zero when there is no horizontal overflow.
func (s *Scroll) hBarGeometry() (track, handle render.Rect) {
	maxX, _ := s.scrollMax()
	if maxX <= 0 || s.viewW <= 0 || s.nat.W <= 0 {
		return track, handle
	}
	track = render.Rect{X: s.bounds.X, Y: s.bounds.Y + s.bounds.H - gutter, W: s.viewW, H: gutter}
	w := max(24, s.viewW*s.viewW/s.nat.W)
	x := track.X + (track.W-w)*s.offX/max(1, maxX)
	handle = render.Rect{X: x, Y: track.Y, W: w, H: gutter}
	return track, handle
}

// ScrollBy shifts the offset by dx, dy scroll steps of 40px and shows
// the bars.
func (s *Scroll) ScrollBy(dx, dy int) {
	s.SetOffset(s.offX+dx*40, s.offY+dy*40)
	s.showBars()
}

// SetPressed implements PressSetter: ending the press ends any drag.
func (s *Scroll) SetPressed(on bool) {
	if on {
		return // the grab geometry arrives with the first DragMove
	}
	s.dragV, s.dragH = false, false
}

// DragMove implements DragMover. The first call inside a scrollbar
// track grabs that handle (recording the press position and offset);
// later calls map pointer motion 1:1 onto content motion through the
// viewport-to-track ratio. Outside the tracks this is content drugging
// of the wrapped child, which the child handles.
func (s *Scroll) DragMove(p Point) {
	vTrack, _ := s.vBarGeometry()
	hTrack, _ := s.hBarGeometry()
	switch {
	case !s.dragV && !s.dragH && vTrack.W > 0 && p.X >= vTrack.X:
		s.dragV = true
		s.dragGrab = p.Y
		s.dragStartOffY = s.offY
	case !s.dragV && !s.dragH && hTrack.H > 0 && p.Y >= hTrack.Y:
		s.dragH = true
		s.dragGrab = p.X
		s.dragStartOffX = s.offX
	case s.dragV:
		_, handle := s.vBarGeometry()
		_, maxY := s.scrollMax()
		denom := max(1, vTrack.H-handle.H)
		s.SetOffset(s.offX, s.dragStartOffY+(p.Y-s.dragGrab)*maxY/denom)
		s.showBars()
	case s.dragH:
		_, handle := s.hBarGeometry()
		maxX, _ := s.scrollMax()
		denom := max(1, hTrack.W-handle.W)
		s.SetOffset(s.dragStartOffX+(p.X-s.dragGrab)*maxX/denom, s.offY)
		s.showBars()
	}
}

// ClickAt implements Clicker: a gutter press above or below (or left
// or right of) a handle pages by one viewport; the first DragMove of
// such a press would have grabbed the handle instead. A click on a
// handle without motion does nothing.
func (s *Scroll) ClickAt(p Point) {
	vTrack, vHandle := s.vBarGeometry()
	if vTrack.W > 0 && vTrack.Contains(p.X, p.Y) {
		if p.Y < vHandle.Y {
			s.SetOffset(s.offX, s.offY-s.viewH)
		} else if p.Y >= vHandle.Y+vHandle.H {
			s.SetOffset(s.offX, s.offY+s.viewH)
		}
		return
	}
	hTrack, hHandle := s.hBarGeometry()
	if hTrack.H > 0 && hTrack.Contains(p.X, p.Y) {
		if p.X < hHandle.X {
			s.SetOffset(s.offX-s.viewW, s.offY)
		} else if p.X >= hHandle.X+hHandle.W {
			s.SetOffset(s.offX+s.viewW, s.offY)
		}
	}
}
