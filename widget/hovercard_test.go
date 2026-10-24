package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/anim"
)

func newTestHoverCard(t *testing.T) (*HoverCard, *Button, *fakeSurfaceHost) {
	t.Helper()
	t.Cleanup(anim.SetInstant(true))
	face := goldenFace(t)
	trigger := NewButton(NewLabel(face, 14, "@nextjs", 0), 8, 8)
	h := NewHoverCard(trigger, NewLabel(face, 14, "The React Framework", 0))
	host := &fakeSurfaceHost{}
	SetSurfaceHost(h, host)
	arrangeTree(t, h, 200, 40)
	return h, trigger, host
}

// Hovering the trigger opens the card; it stays while the pointer is
// on the card and closes once the pointer left both.
func TestHoverCardFollowsThePointer(t *testing.T) {
	h, trigger, host := newTestHoverCard(t)
	r := &Router{Root: h}
	b := trigger.Bounds()
	r.Move(Point{X: b.X + 2, Y: b.Y + 2})
	if !h.Open() || len(host.popovers) != 1 {
		t.Fatalf("hovering the trigger left the card closed (%d popovers)", len(host.popovers))
	}
	h.body.hoverWithin(true)
	r.Leave()
	if !h.Open() {
		t.Fatal("the card closed while the pointer was on it")
	}
	h.body.hoverWithin(false)
	if h.Open() || host.closed != 1 {
		t.Errorf("the card stayed open after the pointer left both (%d closes)", host.closed)
	}
}

// Visible keyboard focus on the trigger opens the card; moving it away
// closes it.
func TestHoverCardOpensOnKeyboardFocus(t *testing.T) {
	h, trigger, host := newTestHoverCard(t)
	other := NewButton(NewLabel(goldenFace(t), 14, "other", 0), 8, 8)
	root := NewBox(Column, 0, 0).Append(h, false).Append(other, false)
	arrangeTree(t, root, 200, 100)
	r := &Router{Root: root}
	r.SetFocus(other)
	r.FocusPrev()
	if r.Focused() != Widget(trigger) || !h.Open() {
		t.Fatalf("keyboard focus on the trigger left the card closed (focus %T)", r.Focused())
	}
	r.FocusNext()
	if h.Open() || host.closed != 1 {
		t.Error("moving the focus away kept the card open")
	}
}
