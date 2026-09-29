package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// moduleButton is the wayle-shaped test widget: a Button wrapped by a
// type that claims the input hooks. It embeds Base and returns itself
// from HitTest, the pattern outside-the-kit widgets need for the
// router to see the hooks at all.
type moduleButton struct {
	Base
	btn     *Button
	middle  int
	right   int
	scrolls []int
}

func newModuleButton(t *testing.T) *moduleButton {
	t.Helper()
	return &moduleButton{btn: NewButton(NewLabel(testFace(t), 12, "m", 0xFF000000), 0, 0)}
}

func (m *moduleButton) Measure(con Constraints) Size { return m.btn.Measure(con) }

func (m *moduleButton) Arrange(r render.Rect) {
	m.node.Arrange(r)
	SetParents(m, m.btn)
	m.btn.Arrange(r)
}

func (m *moduleButton) Paint(cv *render.Canvas) { m.btn.Paint(cv) }

func (m *moduleButton) HitTest(p Point) Widget { return m.HitLeaf(m, p) }

func (m *moduleButton) PointerButton(button uint32) {
	switch button {
	case BTNMiddle:
		m.middle++
	case BTNRight:
		m.right++
	}
}

func (m *moduleButton) ScrollInput(dy int) bool {
	if dy == 0 {
		return false
	}
	m.scrolls = append(m.scrolls, dy)
	return true
}

func TestRouterPointerButtonHooks(t *testing.T) {
	mb := newModuleButton(t)
	root := NewBox(Row, 0, 0)
	root.Append(mb, false)
	r := Router{Root: root}
	root.Measure(Constraints{Max: Size{W: 100, H: 40}})
	root.Arrange(render.Rect{W: 100, H: 40})

	r.Press(BTNRight, Point{X: 2, Y: 2})
	r.Press(BTNMiddle, Point{X: 2, Y: 2})
	if mb.right != 1 || mb.middle != 1 {
		t.Errorf("right=%d middle=%d, want one hook each", mb.right, mb.middle)
	}
	// The primary button keeps its own protocol: no PointerButton call.
	r.Press(BTNLeft, Point{X: 2, Y: 2})
	r.Release(BTNLeft, Point{X: 2, Y: 2})
	if mb.right != 1 || mb.middle != 1 {
		t.Error("left press leaked into PointerButton")
	}
	// A disabled widget swallows the press.
	mb.SetEnabled(false)
	r.Press(BTNRight, Point{X: 2, Y: 2})
	if mb.right != 1 {
		t.Error("disabled widget received a PointerButton")
	}
}

func TestRouterScrollInputTakesPrecedence(t *testing.T) {
	mb := newModuleButton(t)
	root := NewBox(Row, 0, 0)
	root.Append(mb, false)
	r := Router{Root: root}
	root.Measure(Constraints{Max: Size{W: 100, H: 40}})
	root.Arrange(render.Rect{W: 100, H: 40})
	r.Move(Point{X: 2, Y: 2})
	if got := r.Hovered(); got != Widget(mb) {
		t.Fatalf("hovered = %T, want the module button", got)
	}

	r.Axis(0, 120)
	if len(mb.scrolls) != 1 || mb.scrolls[0] != 120 {
		t.Errorf("scrolls = %v, want one 120 step", mb.scrolls)
	}

	// Declining (dy == 0 here) falls through instead of consuming.
	mb.scrolls = nil
	r.Axis(0, 0)
	if len(mb.scrolls) != 0 {
		t.Errorf("declined step still consumed: %v", mb.scrolls)
	}
}
