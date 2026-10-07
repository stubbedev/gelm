package widget

// dragGesture is the press-drag-release bookkeeping the swipeable
// widgets share (Carousel, BottomSheet), on the router's own gesture
// interfaces: PressAt begins it, DragMove reads the travel, PressEnd -
// the release, a cancelled press, or pointer loss alike - settles it
// exactly once.
type dragGesture struct {
	from   Point
	active bool
}

// begin records the press.
func (g *dragGesture) begin(p Point) { g.from, g.active = p, true }

// travel is the pointer's offset from the press; ok is false outside
// a gesture.
func (g *dragGesture) travel(p Point) (dx, dy int, ok bool) {
	if !g.active {
		return 0, 0, false
	}
	return p.X - g.from.X, p.Y - g.from.Y, true
}

// end closes the gesture and reports whether one was running, so the
// widget settles at most once per press.
func (g *dragGesture) end() bool {
	was := g.active
	g.active = false
	return was
}
