package app

import (
	"testing"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/internal/popup"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// TestAnchorOrigin pins the gravity math: below by default, flipping
// inside the host when the content would overflow, and clamped along
// the perpendicular axis.
func TestAnchorOrigin(t *testing.T) {
	host := render.Rect{X: 0, Y: 0, W: 400, H: 300}
	size := widget.Size{W: 100, H: 50}

	t.Run("gravity bottom opens below the anchor", func(t *testing.T) {
		anchor := render.Rect{X: 100, Y: 100, W: 60, H: 20}
		x, y := anchorOrigin(host, anchor, size, popup.GravityBottom)
		if x != 100 || y != 120 {
			t.Errorf("got (%d,%d), want (100,120)", x, y)
		}
	})

	t.Run("gravity bottom flips above when there is no room", func(t *testing.T) {
		anchor := render.Rect{X: 100, Y: 280, W: 60, H: 20}
		_, y := anchorOrigin(host, anchor, size, popup.GravityBottom)
		if y != 280-50 {
			t.Errorf("y = %d, want flipped above the anchor", y)
		}
	})

	t.Run("gravity top opens above the anchor", func(t *testing.T) {
		anchor := render.Rect{X: 100, Y: 100, W: 60, H: 20}
		_, y := anchorOrigin(host, anchor, size, popup.GravityTop)
		if y != 100-50 {
			t.Errorf("y = %d, want above", y)
		}
	})

	t.Run("gravity top flips below when pinned at the top", func(t *testing.T) {
		anchor := render.Rect{X: 100, Y: 10, W: 60, H: 20}
		_, y := anchorOrigin(host, anchor, size, popup.GravityTop)
		if y != 10+20 {
			t.Errorf("y = %d, want flipped below the anchor", y)
		}
	})

	t.Run("gravity right opens to the right and flips at the edge", func(t *testing.T) {
		anchor := render.Rect{X: 100, Y: 100, W: 60, H: 20}
		x, _ := anchorOrigin(host, anchor, size, popup.GravityRight)
		if x != 160 {
			t.Errorf("x = %d, want right of the anchor", x)
		}
		anchor = render.Rect{X: 350, Y: 100, W: 40, H: 20}
		x, _ = anchorOrigin(host, anchor, size, popup.GravityRight)
		if x != 350-100 {
			t.Errorf("x = %d, want flipped left of the anchor", x)
		}
	})

	t.Run("gravity left opens to the left and flips at the edge", func(t *testing.T) {
		anchor := render.Rect{X: 150, Y: 100, W: 40, H: 20}
		x, _ := anchorOrigin(host, anchor, size, popup.GravityLeft)
		if x != 150-100 {
			t.Errorf("x = %d, want left of the anchor", x)
		}
		anchor = render.Rect{X: 20, Y: 100, W: 40, H: 20}
		x, _ = anchorOrigin(host, anchor, size, popup.GravityLeft)
		if x != 20+40 {
			t.Errorf("x = %d, want flipped right of the anchor", x)
		}
	})
}

// fakePopoverHost is a wire-free Host for registry tests.
type fakePopoverHost struct{ id int }

func (f *fakePopoverHost) EnsureUsable() error      { return nil }
func (f *fakePopoverHost) Closed() bool             { return false }
func (f *fakePopoverHost) Size() (int, int)         { return 400, 300 }
func (f *fakePopoverHost) HostSurface() *wl.Surface { return nil }

// TestPopoverRegistry pins the re-anchor rule: opening a second
// popover on the same host closes the first; a different host is
// unaffected.
func TestPopoverRegistry(t *testing.T) {
	var reg popoverRegistry
	h1 := &fakePopoverHost{id: 1}

	h2 := &fakePopoverHost{id: 2}

	p1 := &Popover{}
	if prev := reg.openOrReplace(h1, p1); prev != nil {
		t.Error("first open replaced something")
	}

	p2 := &Popover{}
	if prev := reg.openOrReplace(h1, p2); prev != p1 {
		t.Errorf("re-anchor returned %v, want the first popover", prev)
	}

	p3 := &Popover{}
	if prev := reg.openOrReplace(h2, p3); prev != nil {
		t.Error("a different host should not replace")
	}

	if got := reg.take(h2); got != p3 {
		t.Error("take returned the wrong popover")
	}
	if got := reg.take(h2); got != nil {
		t.Error("take on an empty host returned a popover")
	}
}

// TestPopoverKeyRoot pins Esc dismissal and forwarding of other keys
// to the content.
func TestPopoverKeyRoot(t *testing.T) {
	dismissed := 0
	actions := []widget.KeyAction{}
	content := &keySpy{onAction: func(a widget.KeyAction, m widget.Mods) {
		actions = append(actions, a)
	}}
	root := &popoverKeyRoot{onDismiss: func() { dismissed++ }, content: content}

	root.KeyAction(widget.KeyDismiss, 0)
	if dismissed != 1 {
		t.Errorf("dismissals = %d, want 1", dismissed)
	}
	root.KeyAction(widget.KeyDown, 0)
	if len(actions) != 1 || actions[0] != widget.KeyDown {
		t.Errorf("forwarded actions = %v, want [KeyDown]", actions)
	}
}

// keySpy records forwarded KeyActions.
type keySpy struct {
	onAction func(widget.KeyAction, widget.Mods)
}

func (k *keySpy) Measure(widget.Constraints) widget.Size { return widget.Size{} }
func (k *keySpy) Arrange(render.Rect)                    {}
func (k *keySpy) Paint(*render.Canvas)                   {}
func (k *keySpy) HitTest(widget.Point) widget.Widget     { return nil }
func (k *keySpy) KeyAction(a widget.KeyAction, m widget.Mods) {
	k.onAction(a, m)
}
