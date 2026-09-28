package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// flooredPane is a stub pane with a MinSizer floor, for the clamp pins.
type flooredPane struct {
	stub
	floor Size
}

func (s *flooredPane) MinSize() Size { return s.floor }

// panedFrame measures and arranges a paned into w x h and returns it.
func panedFrame(p *Paned, w, h int) *Paned {
	p.Measure(Constraints{Max: Size{W: w, H: h}})
	p.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
	return p
}

// TestPanedDragMovesDividerAndClamps pins the pointer half of #71: a
// drag inside the handle's hit zone (slop included) moves the divider
// live, the panes relayout around it, and the MinSizer floors clamp
// both ends — never starving a pane past its declared minimum.
func TestPanedDragMovesDividerAndClamps(t *testing.T) {
	start := &flooredPane{floor: Size{W: 40, H: 0}}
	start.nat = Size{W: 120, H: 30}
	end := &flooredPane{floor: Size{W: 50, H: 0}}
	end.nat = Size{W: 120, H: 30}
	var positions []int
	p := NewPaned(Row, start, end)
	p.OnPositionChanged = func(pos int) { positions = append(positions, pos) }
	panedFrame(p, 300, 60)

	// The handle sits at the initial (clamped) position with room on
	// both sides; a press in the slop zone grabs it.
	if got := p.Position(); got != 120 {
		t.Fatalf("initial position = %d, want the natural 120", got)
	}
	handleX := 120 + panedHandleW/2
	if p.HitTest(Point{X: handleX - panedHandleSlop, Y: 30}) != Widget(p) {
		t.Fatal("a press in the slop zone missed the handle")
	}

	// Dragging to the middle moves the divider and both panes.
	p.SetPressed(true)
	p.DragMove(Point{X: 150, Y: 30})
	if got := p.Position(); got != 150-panedHandleW/2 {
		t.Fatalf("position after drag = %d, want pointer-centered %d", got, 150-panedHandleW/2)
	}
	if start.Bounds().W != p.Position() || end.Bounds().X != p.Position()+panedHandleW {
		t.Errorf("panes = %+v %+v, want them relaid around the divider", start.Bounds(), end.Bounds())
	}

	// The end pane's floor clamps the far end.
	p.DragMove(Point{X: 299, Y: 30})
	if got := p.Position(); got != 300-panedHandleW-50 {
		t.Fatalf("position past the end floor = %d, want the clamp %d", got, 300-panedHandleW-50)
	}
	// The start pane's floor clamps the near end.
	p.DragMove(Point{X: 0, Y: 30})
	if got := p.Position(); got != 40 {
		t.Fatalf("position past the start floor = %d, want the clamp 40", got)
	}
	// A disabled pane never drags.
	p.SetEnabled(false)
	p.DragMove(Point{X: 150, Y: 30})
	if got := p.Position(); got != 40 {
		t.Fatalf("disabled drag moved the divider to %d", got)
	}
	p.SetEnabled(true)

	// The hook fired once per applied change, always with the clamped
	// truth.
	if len(positions) < 3 {
		t.Errorf("position changes = %v, want one per applied drag", positions)
	}
	for _, pos := range positions {
		if pos < 40 || pos > 300-panedHandleW-50 {
			t.Errorf("a change reported %d, outside the clamps", pos)
		}
	}
}

// TestPanedKeyboardNudges pins the keyboard half: arrows step along
// the axis (a twentieth of the span), Home and End pin to the
// extremes, the cross-axis arrows do nothing, and the floors own the
// extremes when they must.
func TestPanedKeyboardNudges(t *testing.T) {
	start := &flooredPane{floor: Size{W: 40, H: 0}}
	start.nat = Size{W: 100, H: 30}
	end := &flooredPane{floor: Size{W: 50, H: 0}}
	end.nat = Size{W: 100, H: 30}
	p := NewPaned(Row, start, end)
	panedFrame(p, 306, 60)
	step := (306 - panedHandleW) / 20

	p.KeyAction(KeyRight, 0)
	if got := p.Position(); got != 100+step {
		t.Fatalf("nudge = %d, want %d (one step from the natural)", got, step+100)
	}
	p.KeyAction(KeyLeft, 0)
	if got := p.Position(); got != 100 {
		t.Fatalf("back nudge = %d, want the natural 100 back", got)
	}
	// The cross-axis arrows are not the handle's.
	p.KeyAction(KeyDown, 0)
	p.KeyAction(KeyUp, 0)
	if got := p.Position(); got != 100 {
		t.Fatalf("cross-axis arrows moved the divider: %d", got)
	}
	p.KeyAction(KeyEnd, 0)
	if got := p.Position(); got != 306-panedHandleW-50 {
		t.Fatalf("end = %d, want the clamped extreme", got)
	}
	p.KeyAction(KeyHome, 0)
	if got := p.Position(); got != 40 {
		t.Fatalf("home = %d, want the start floor", got)
	}
}

// TestPanedResizeRenormalizes pins GTK's keep-child-one semantics: a
// resize keeps the requested position while it fits and re-normalizes
// it to the nearest clamp when it does not — growing never stretches
// the start pane back.
func TestPanedResizeRenormalizes(t *testing.T) {
	start := newStub(100, 30)
	end := newStub(100, 30)
	p := NewPaned(Row, start, end)
	panedFrame(p, 400, 60)

	p.SetPosition(150)
	p.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 60})
	if got := p.Position(); got != 150 {
		t.Fatalf("position = %d, want the request 150", got)
	}

	// Shrinking below the request: the position clamps, and the
	// shrunken position is what survives when the window grows again.
	p.Arrange(render.Rect{X: 0, Y: 0, W: 120, H: 60})
	if got := p.Position(); got != 120-panedHandleW {
		t.Fatalf("position in a 120 rect = %d, want the clamp %d", got, 120-panedHandleW)
	}
	p.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 60})
	if got := p.Position(); got != 120-panedHandleW {
		t.Fatalf("position regrown = %d, want the re-normalized value kept", got)
	}
}

// TestPanedNestingAndNilPanes pins the sizing model: a paned inside a
// paned lays out through the same arrangement, and a missing pane
// collapses to the other taking everything.
func TestPanedNestingAndNilPanes(t *testing.T) {
	inner := NewPaned(Column, newStub(50, 40), newStub(50, 40))
	outer := NewPaned(Row, inner, newStub(80, 100))
	panedFrame(outer, 300, 100)

	innerRect := inner.Bounds()
	if innerRect.W != outer.Position() || innerRect.H != 100 {
		t.Fatalf("inner paned = %+v, want the start share of 300x100", innerRect)
	}
	innerStart := inner.Children()[0].(interface{ Bounds() render.Rect }).Bounds()
	if innerStart.H != inner.Position() || innerStart.W != innerRect.W {
		t.Fatalf("inner start pane = %+v, want the column split", innerStart)
	}

	only := NewPaned(Row, newStub(30, 30), nil)
	panedFrame(only, 200, 50)
	if got := only.Children()[0].(interface{ Bounds() render.Rect }).Bounds(); got != (render.Rect{W: 200, H: 50}) {
		t.Errorf("lone pane = %+v, want the whole 200x50", got)
	}
	if got := only.Position(); got != 0 {
		t.Errorf("a missing pane still positions at %d", got)
	}
}

// TestPanedChildrenKeepTheirInput pins that the handle owns only its
// zone: hits in the panes reach the panes (their own interactive
// widgets), and the Paned joins focus traversal through Children.
func TestPanedChildrenKeepTheirInput(t *testing.T) {
	start := NewButton(newStub(20, 20), 4, 4)
	end := newStub(40, 40)
	p := NewPaned(Row, start, end)
	panedFrame(p, 300, 60)

	if hit := p.HitTest(Point{X: 10, Y: 30}); hit != start.HitTest(Point{X: 10, Y: 30}) {
		t.Errorf("hit in the start pane = %T, want the pane's own widget", hit)
	}
	if hit := p.HitTest(Point{X: 250, Y: 30}); hit != Widget(end) {
		t.Errorf("hit in the end pane = %T, want the end pane", hit)
	}
	if got := len(p.Children()); got != 2 {
		t.Errorf("Children = %d entries, want both panes", got)
	}
	if p.CursorName() != "col-resize" {
		t.Errorf("row cursor = %q, want col-resize", p.CursorName())
	}
	if v := NewPaned(Column, nil, nil).CursorName(); v != "row-resize" {
		t.Errorf("column cursor = %q, want row-resize", v)
	}
}
