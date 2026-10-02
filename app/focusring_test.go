package app

import (
	"testing"

	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// The ring marks keyboard focus only: a click focuses without one
// (GTK's :focus-visible), Tab traversal shows it.
func TestFocusRingFollowsFocusVisible(t *testing.T) {
	a, b := widget.NewSwitch(false), widget.NewSwitch(false)
	root := widget.NewBox(widget.Column, 0, 0)
	root.Append(a, false).Append(b, false)
	root.Measure(widget.Constraints{Max: widget.Size{W: 100, H: 100}})
	root.Arrange(render.Rect{W: 100, H: 100})
	w := &hostWindow{router: &widget.Router{Root: root}}

	w.router.Press(widget.BTNLeft, widget.Point{X: 5, Y: 5})
	w.router.Release(widget.BTNLeft, widget.Point{X: 5, Y: 5})
	if w.router.Focused() != widget.Widget(a) {
		t.Fatalf("a click focused %v, want the first switch", w.router.Focused())
	}
	if r := w.focusRingRect(); !r.Empty() {
		t.Errorf("pointer focus shows a ring at %v", r)
	}
	w.router.FocusNext()
	if r := w.focusRingRect(); r.Empty() || widget.FocusVisible(a) || !widget.FocusVisible(b) {
		t.Errorf("keyboard focus ring %v (a visible %v, b visible %v)", r, widget.FocusVisible(a), widget.FocusVisible(b))
	}
}
