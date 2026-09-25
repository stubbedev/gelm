package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// stub is a fixed-size leaf that records its arranged rect and paints
// nothing.
type stub struct {
	node
	nat  Size
	rect render.Rect
}

func newStub(w, h int) *stub {
	return &stub{nat: Size{W: w, H: h}}
}

func (s *stub) Measure(con Constraints) Size {
	return clampSize(s.nat, con)
}

func (s *stub) Arrange(r render.Rect) {
	s.node.Arrange(r)
	s.rect = r
}

func (s *stub) Paint(*render.Canvas) {}

func (s *stub) HitTest(p Point) Widget {
	return s.HitLeaf(s, p)
}

func TestBoxMeasureRow(t *testing.T) {
	t.Run("sums children, spacing, and padding", func(t *testing.T) {
		b := NewBox(Row, 2, 1)
		b.Append(newStub(10, 5), false)
		b.Append(newStub(8, 4), false)
		got := b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		if got.W != 1+10+2+8+1 {
			t.Errorf("main extent = %d, want 22", got.W)
		}
		if got.H != 5+2 {
			t.Errorf("cross extent = %d, want 7", got.H)
		}
	})

	t.Run("transposes for a column", func(t *testing.T) {
		b := NewBox(Column, 2, 1)
		b.Append(newStub(10, 5), false)
		b.Append(newStub(8, 4), false)
		got := b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		if got.H != 1+5+2+4+1 {
			t.Errorf("main extent = %d, want 13", got.H)
		}
		if got.W != 10+2 {
			t.Errorf("cross extent = %d, want 12", got.W)
		}
	})

	t.Run("empty box is just padding", func(t *testing.T) {
		got := NewBox(Row, 4, 3).Measure(Constraints{Max: Size{W: 100, H: 100}})
		if got != (Size{W: 6, H: 6}) {
			t.Errorf("empty box = %v, want 6x6", got)
		}
	})

	t.Run("clamps to constraints", func(t *testing.T) {
		b := NewBox(Row, 2, 1)
		b.Append(newStub(50, 5), false)
		got := b.Measure(Constraints{Max: Size{W: 30, H: 3}})
		if got.W != 30 || got.H != 3 {
			t.Errorf("measured %v, want clamped to 30x3", got)
		}
	})
}

func TestBoxArrangeRow(t *testing.T) {
	t.Run("children line up with spacing and stretch the cross axis", func(t *testing.T) {
		b := NewBox(Row, 2, 1)
		a := newStub(10, 5)
		c := newStub(8, 4)
		b.Append(a, false)
		b.Append(c, false)

		b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})

		if a.rect != (render.Rect{X: 1, Y: 1, W: 10, H: 18}) {
			t.Errorf("first child rect = %v, want x=1 h stretched to 18", a.rect)
		}
		if c.rect != (render.Rect{X: 13, Y: 1, W: 8, H: 18}) {
			t.Errorf("second child rect = %v, want x=13", c.rect)
		}
	})

	t.Run("expanders split the leftover main space", func(t *testing.T) {
		b := NewBox(Row, 0, 0)
		a := newStub(10, 5)
		c := newStub(10, 5)
		d := newStub(10, 5)
		b.Append(a, false)
		b.Append(c, true)
		b.Append(d, true)

		b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 70, H: 20})

		// 70 - 30 natural = 40 free, split between two expanders.
		if c.rect.W != 30 || d.rect.W != 30 {
			t.Errorf("expander widths = %d, %d, want 30, 30", c.rect.W, d.rect.W)
		}
		if a.rect.W != 10 {
			t.Errorf("non-expander width = %d, want 10", a.rect.W)
		}
		if d.rect.X != 40 {
			t.Errorf("last child x = %d, want 40", d.rect.X)
		}
	})

	t.Run("no expander leaves leftover space unused", func(t *testing.T) {
		b := NewBox(Row, 0, 0)
		a := newStub(10, 5)
		b.Append(a, false)

		b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 20})

		if a.rect.W != 10 {
			t.Errorf("child width = %d, want natural 10", a.rect.W)
		}
	})

	t.Run("overflowing children keep their natural size", func(t *testing.T) {
		b := NewBox(Row, 0, 0)
		a := newStub(30, 5)
		c := newStub(30, 5)
		b.Append(a, false)
		b.Append(c, false)

		b.Measure(Constraints{Max: Size{W: 40, H: 20}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})

		if a.rect.W != 30 || c.rect.W != 30 {
			t.Errorf("overflowing children shrank: %d, %d", a.rect.W, c.rect.W)
		}
		if c.rect.X != 30 {
			t.Errorf("second child x = %d, want 30 (overflow, not clamped)", c.rect.X)
		}
	})

	t.Run("column layout runs down", func(t *testing.T) {
		b := NewBox(Column, 3, 0)
		a := newStub(10, 5)
		c := newStub(10, 7)
		b.Append(a, false)
		b.Append(c, false)

		b.Measure(Constraints{Max: Size{W: 100, H: 100}})
		b.Arrange(render.Rect{X: 0, Y: 0, W: 50, H: 40})

		if a.rect != (render.Rect{X: 0, Y: 0, W: 50, H: 5}) {
			t.Errorf("first child = %v, want w stretched to 50", a.rect)
		}
		if c.rect != (render.Rect{X: 0, Y: 8, W: 50, H: 7}) {
			t.Errorf("second child = %v, want y=8", c.rect)
		}
	})
}

func TestBoxHitTest(t *testing.T) {
	b := NewBox(Row, 2, 1)
	a := newStub(10, 5)
	b.Append(a, false)
	b.Measure(Constraints{Max: Size{W: 100, H: 100}})
	b.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 20})

	t.Run("over a child returns the child", func(t *testing.T) {
		if got := b.HitTest(Point{X: 5, Y: 5}); got != Widget(a) {
			t.Errorf("hit = %v, want the stub", got)
		}
	})

	t.Run("over padding returns the box", func(t *testing.T) {
		if got := b.HitTest(Point{X: 0, Y: 0}); got != Widget(b) {
			t.Errorf("hit = %v, want the box", got)
		}
	})

	t.Run("outside returns nil", func(t *testing.T) {
		if got := b.HitTest(Point{X: 200, Y: 200}); got != nil {
			t.Errorf("hit = %v, want nil", got)
		}
	})
}
