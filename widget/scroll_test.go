package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// newScrollFixture builds a 60x60 scroll wrapping a 60x200 child: a
// vertical range of 140px with a gutter-reserved vertical bar.
func newScrollFixture(t *testing.T) *Scroll {
	t.Helper()
	s := NewScroll(newStub(60, 200))
	s.ShowBars = true
	s.Measure(Constraints{Max: Size{W: 60, H: 60}})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
	return s
}

// TestScrollHandleDrag pins the 1:1 handle mapping: grabbing the
// handle and moving the pointer down scales pointer motion by the
// viewport-to-track ratio into content offset.
func TestScrollHandleDrag(t *testing.T) {
	s := newScrollFixture(t)
	track, handle := s.vBarGeometry()
	if track.W == 0 {
		t.Fatal("no vertical bar geometry for overflowing content")
	}

	s.SetPressed(true)
	// First drag event grabs: press inside the handle.
	s.DragMove(Point{X: track.X + 2, Y: handle.Y + 2})
	startOffY := s.offY

	s.DragMove(Point{X: track.X + 2, Y: handle.Y + 2 + 18})
	_, maxY := s.scrollMax()
	denom := track.H - handle.H
	want := min(max(0, startOffY+(18*maxY+denom/2)/denom), maxY)
	if s.offY != want {
		t.Errorf("offset after 18px drag = %d, want %d (1:1 mapping)", s.offY, want)
	}

	s.SetPressed(false)
	if _, maxY := s.scrollMax(); s.offY > maxY {
		t.Errorf("offset %d exceeds range %d (overshoot)", s.offY, maxY)
	}
}

// TestScrollPageClick pins gutter paging: clicking the track below the
// handle pages down by one viewport; clicking above pages up.
func TestScrollPageClick(t *testing.T) {
	s := newScrollFixture(t)
	track, _ := s.vBarGeometry()

	s.ClickAt(Point{X: track.X + 2, Y: track.Y + track.H - 2})
	if _, offY := s.Offset(); offY != 60 {
		t.Errorf("page down offset = %d, want 60 (one viewport)", offY)
	}
	// After the page down the handle sits lower; clicking the track
	// above its new position pages up, clamped back to the top.
	s.ClickAt(Point{X: track.X + 2, Y: track.Y + 2})
	if _, offY := s.Offset(); offY != 0 {
		t.Errorf("page up offset = %d, want clamped 0", offY)
	}
}

// TestHorizontalWheelScrolls pins horizontal axis handling on a child
// wider than the viewport.
func TestHorizontalWheelScrolls(t *testing.T) {
	s := NewScroll(newStub(200, 30))
	s.ShowBars = true
	s.Measure(Constraints{Max: Size{W: 60, H: 60}})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 60})
	s.ScrollBy(2, 0)
	if offX, _ := s.Offset(); offX != 80 {
		t.Errorf("horizontal offset = %d, want 80 (two steps)", offX)
	}
	if _, handle := s.hBarGeometry(); handle.H == 0 {
		t.Error("no horizontal bar for horizontally overflowing content")
	}
}
