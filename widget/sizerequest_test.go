package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// A size request floors the measure a container takes (the border box,
// margin outside), an aligned child keeps at least the request, and a
// zero axis or a larger natural size is untouched.
func TestSizeRequest(t *testing.T) {
	child := NewDrawingArea(40, 20)
	child.SetSizeRequest(100, 0)
	box := NewBox(Row, 0, 0)
	box.Append(child, false)
	if got := box.Measure(Constraints{Max: Size{W: 500, H: 500}}); got != (Size{W: 100, H: 20}) {
		t.Errorf("box measured %v, want the 100 request and the natural 20", got)
	}
	child.SetMargin(render.Insets{Left: 5, Right: 5})
	if got := box.Measure(Constraints{Max: Size{W: 500, H: 500}}); got.W != 110 {
		t.Errorf("with margins %v, want 110 (the request plus the margin)", got)
	}
	child.SetMargin(render.Insets{})
	child.SetHAlign(AlignCenter)
	box.Measure(Constraints{Max: Size{W: 300, H: 20}})
	box.Arrange(render.Rect{W: 300, H: 20})
	if b := child.Bounds(); b.W != 100 || b.X != 0 {
		t.Errorf("aligned child at %+v, want the request's 100 wide", b)
	}
	child.SetSizeRequest(10, 5)
	if got := box.Measure(Constraints{Max: Size{W: 500, H: 500}}); got != (Size{W: 40, H: 20}) {
		t.Errorf("a request under the natural size measured %v, want the natural 40x20", got)
	}
	child.SetSizeRequest(1000, 0)
	if got := box.Measure(Constraints{Max: Size{W: 300, H: 500}}); got.W != 300 {
		t.Errorf("a request over the space measured %v, want the 300 the box has", got)
	}
	if child.SizeRequest() != (Size{W: 1000}) {
		t.Errorf("SizeRequest = %v", child.SizeRequest())
	}
}
