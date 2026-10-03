package widget

import (
	"testing"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/render"
)

// A press outside the open dropdown closes it through the animated
// exit: the tween runs, and reduced motion detaches at once. A press
// on the list, the face, or a disabled control's ruling differs.
func TestDropdownClosesOnOutsideClick(t *testing.T) {
	c := pinAnimClock(t)
	dd := newDropdown(t, "Red", "Green", "Blue")
	btn := NewButton(NewLabel(entryFace(t), 12, "other", 0), 2, 2)
	host := NewBox(Column, 4, 0)
	host.Append(dd, false)
	host.Append(btn, false)
	host.Measure(Constraints{Max: Size{W: 400, H: 200}})
	host.Arrange(render.Rect{W: 400, H: 200})
	r := &Router{Root: host}

	dd.Open()
	clickAt(r, btn)
	if !dd.closing || !dd.open {
		t.Fatal("a click on another widget: want the exit tween running")
	}
	for c.step() {
	}
	if dd.open || dd.closing {
		t.Fatalf("after the tween: open %v closing %v, want detached", dd.open, dd.closing)
	}

	dd.Open()
	r.Press(BTNLeft, Point{X: host.Bounds().W - 1, Y: host.Bounds().H - 1})
	if !dd.closing {
		t.Fatal("a click on empty space: want the exit tween running")
	}
	for c.step() {
	}
	if dd.open {
		t.Fatal("the empty-space click did not detach the list")
	}

	dd.Open()
	r.Press(BTNLeft, rowPoint(dd, 1))
	if dd.closing {
		t.Fatal("a press on the list dismissed it")
	}
	dd.Close()
	for c.step() {
	}

	dd.Open()
	b := dd.Bounds()
	r.Press(BTNLeft, Point{X: b.X + b.W/2, Y: b.Y + b.H/2})
	r.Release(BTNLeft, Point{X: b.X + b.W/2, Y: b.Y + b.H/2})
	if !dd.closing {
		t.Fatal("a face click did not toggle the list closed")
	}
	for c.step() {
	}
	if dd.open {
		t.Fatal("the face toggle did not detach the list")
	}
}

// Reduced motion collapses the click-away close: the list detaches on
// the press, synchronously.
func TestDropdownOutsideClickInstant(t *testing.T) {
	pinAnimClock(t)
	restore := anim.SetInstant(true)
	defer restore()
	dd := newDropdown(t, "Red", "Green", "Blue")
	host := NewBox(Column, 0, 0)
	host.Append(dd, false)
	host.Measure(Constraints{Max: Size{W: 400, H: 200}})
	host.Arrange(render.Rect{W: 400, H: 200})
	r := &Router{Root: host}

	dd.Open()
	r.Press(BTNLeft, Point{X: 390, Y: 190})
	if dd.open || dd.closing {
		t.Fatalf("instant mode: open %v closing %v, want detached at once", dd.open, dd.closing)
	}
}
