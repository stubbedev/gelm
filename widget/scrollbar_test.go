package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// scrolledColumn is a 100x100 Scroll over a 100x400 child beside a
// vertical Scrollbar bound to it.
func scrolledColumn() (*Scroll, *Scrollbar, *Box) {
	s := NewScroll(NewSpacer(100, 400))
	bar := NewScrollbar(s, Column)
	row := NewBox(Row, 0, 0)
	row.Append(s, true)
	row.Append(bar, false)
	row.Measure(Constraints{Max: Size{W: 108, H: 100}})
	row.Arrange(render.Rect{W: 108, H: 100})
	return s, bar, row
}

// TestScrollRange pins the shared bar arithmetic.
func TestScrollRange(t *testing.T) {
	r := ScrollRange{Offset: 0, Page: 100, Total: 400}
	if r.Max() != 300 || !r.Overflows() {
		t.Fatalf("max=%d overflows=%v", r.Max(), r.Overflows())
	}
	if pos, length := r.thumb(100); pos != 0 || length != 25 {
		t.Errorf("thumb = %d+%d, want 0+25", pos, length)
	}
	if got := r.dragged(0, 75, 100); got != 300 {
		t.Errorf("a full-track drag = %d, want the max", got)
	}
	if got := r.paged(90, 100); got != 100 {
		t.Errorf("a click below the thumb = %d, want one page", got)
	}
	if got := r.paged(10, 100); got != 0 {
		t.Errorf("a click on the thumb moved it to %d", got)
	}
	if (ScrollRange{Page: 100, Total: 50}).Overflows() {
		t.Error("a short content overflows")
	}
}

// TestScrollbar pins the binding both ways: the bar drags and pages
// its Scroll, a Scroll offset change repaints the bar, and the release
// after a drag does not also page.
func TestScrollbar(t *testing.T) {
	s, bar, _ := scrolledColumn()
	b := bar.Bounds()
	if b.W != gutter || b.H != 100 {
		t.Fatalf("bar bounds = %+v", b)
	}
	bar.ClickAt(Point{X: b.X + 2, Y: 90})
	if _, y := s.Offset(); y != s.AxisRange(Column).Page {
		t.Errorf("a track click scrolled to %d, want one page", y)
	}

	s.SetOffset(0, 0)
	bar.invalid = false
	s.SetOffset(0, 50)
	if !bar.invalid {
		t.Error("a scroll offset change did not repaint the bar")
	}

	s.SetOffset(0, 0)
	bar.PressAt(Point{X: b.X + 2, Y: 5})
	bar.DragMove(Point{X: b.X + 2, Y: 5 + 75})
	bar.ClickAt(Point{X: b.X + 2, Y: 95})
	bar.PressEnd()
	if _, y := s.Offset(); y != 300 {
		t.Errorf("dragging the thumb to the end scrolled to %d, want 300", y)
	}
}

// TestGoldenScrollbar pins the standalone bar beside its scroll.
func TestGoldenScrollbar(t *testing.T) {
	th := DarkTheme()
	s, _, row := scrolledColumn()
	s.SetOffset(0, 150)
	NewGolden(t, row, "scrollbar", goldenTheme(th), goldenFrame(108, 100))
}
