package app

import (
	"testing"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// TestDialogRespondOnce pins the once-guard: the first Respond fires
// OnResponse and closes the dialog; further Responds are ignored.
func TestDialogRespondOnce(t *testing.T) {
	app := &Application{}
	responses := 0
	d := &Dialog{
		app: app,
		win: &Window{app: app},
		cfg: DialogConfig{OnResponse: func(string) { responses++ }},
	}
	d.Respond("ok")
	d.Respond("ok")
	if responses != 1 {
		t.Errorf("responses = %d after two Responds, want 1", responses)
	}
	if !d.Closed() {
		t.Error("dialog did not close after responding")
	}
}

// TestDialogResponseForKey pins the key mapping: Esc is the cancel
// response, Enter the default, and an empty mapping disables the key.
func TestDialogResponseForKey(t *testing.T) {
	cfg := DialogConfig{DefaultResponse: "ok", CancelResponse: "cancel"}

	if resp, ok := dialogResponseForKey(cfg, xkb.KeyEscape); !ok || resp != "cancel" {
		t.Errorf("escape = %q %v, want cancel", resp, ok)
	}
	if resp, ok := dialogResponseForKey(cfg, xkb.KeyReturn); !ok || resp != "ok" {
		t.Errorf("return = %q %v, want ok", resp, ok)
	}
	if resp, ok := dialogResponseForKey(cfg, xkb.KeyKPEnter); !ok || resp != "ok" {
		t.Errorf("kp-enter = %q %v, want ok", resp, ok)
	}

	empty := DialogConfig{}
	if _, ok := dialogResponseForKey(empty, xkb.KeyEscape); ok {
		t.Error("escape fired with no cancel response configured")
	}
	if _, ok := dialogResponseForKey(empty, xkb.KeyReturn); ok {
		t.Error("return fired with no default response configured")
	}
	if _, ok := dialogResponseForKey(cfg, xkb.KeyLeft); ok {
		t.Error("an unrelated key produced a dialog response")
	}
}

// TestMessageBoxDefaults pins the preset button mapping: the first
// button is the default response, the last the cancel.
func TestMessageBoxDefaults(t *testing.T) {
	buttons := []DialogButton{
		{Label: "Delete", Response: "delete"},
		{Label: "Cancel", Response: "cancel"},
	}
	if got := defaultResponse(buttons); got != "delete" {
		t.Errorf("default = %q, want delete", got)
	}
	if got := cancelResponse(buttons); got != "cancel" {
		t.Errorf("cancel = %q, want cancel", got)
	}
}

// TestModalBlocksParentInput pins the modal rule: a blocked
// surfaceInput drops pointer presses, motion, and wheel, so the parent
// window is inert while a dialog is open.
func TestModalBlocksParentInput(t *testing.T) {
	clicked := 0
	clickable := &clickCountingBox{onClick: func() { clicked++ }}
	blocked := true

	in := &surfaceInput{
		router:  &widget.Router{Root: clickable},
		request: func() {},
		blocked: func() bool { return blocked },
	}

	clickable.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})
	in.HandlePointerEnter(50, 50)
	in.HandlePointerButton(widget.BTNLeft, 1, 0)
	in.HandlePointerButton(widget.BTNLeft, 0, 0)
	in.HandlePointerAxis(0, 80)
	if clicked != 0 {
		t.Errorf("blocked parent received %d clicks", clicked)
	}

	// Once the dialog closes, blocking lifts.
	blocked = false
	in.HandlePointerEnter(50, 50)
	in.HandlePointerButton(widget.BTNLeft, 1, 0)
	in.HandlePointerButton(widget.BTNLeft, 0, 0)
	if clicked != 1 {
		t.Errorf("clicks after unblock = %d, want 1", clicked)
	}
}

// clickCountingBox is a Box that counts clicks.
type clickCountingBox struct {
	widget.Box
	onClick func()
	clicked bool
}

// HitTest returns the box itself so the outer Clicker is reachable.
func (c *clickCountingBox) HitTest(p widget.Point) widget.Widget {
	if c.Bounds().Contains(p.X, p.Y) {
		return c
	}
	return nil
}

// ClickAt implements Clicker.
func (c *clickCountingBox) ClickAt(p widget.Point) {
	c.clicked = true
	c.onClick()
}
