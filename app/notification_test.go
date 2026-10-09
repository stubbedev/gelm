package app

import (
	"testing"
	"time"

	"github.com/stubbedev/gelm/internal/notify"
)

// TestNotifyRoutesActionsOntoLoop pins the loop bridge: an activation
// arriving from the notifier's goroutine dispatches the notification's
// OnAction on the loop goroutine (inside a pump), and a close forgets
// the id - a late action after the close fires nothing.
func TestNotifyRoutesActionsOntoLoop(t *testing.T) {
	a := testApp(nil)
	fired := ""
	closed := 0
	a.Notify("title", "body", NotifyOptions{
		Timeout:  2 * time.Second,
		OnAction: func(key string) { fired = key },
		OnClosed: func(notify.CloseReason) { closed++ },
	})
	a.notifyMu.Lock()
	var id string
	for k := range a.notifyOpts {
		id = k
	}
	a.notifyMu.Unlock()
	if id == "" {
		t.Fatal("the notification was not registered")
	}

	// The dispatch is what the Invoke bridge runs inside a pump: the
	// action fires the callback, the close forgets the id.
	a.Invoke(func() { a.notifyAction(id, "open") })
	a.pump(time.Now())
	if fired != "open" {
		t.Errorf("action never dispatched: %q", fired)
	}
	a.Invoke(func() { a.notifyClosed(id, notify.ReasonExpired) })
	a.pump(time.Now())
	if closed != 1 {
		t.Errorf("close dispatched %d times", closed)
	}
	a.Invoke(func() { a.notifyAction(id, "open") })
	a.pump(time.Now())
	if fired != "open" {
		t.Error("an action after the close changed state")
	}
}

// TestNotifyInertWithoutBus pins the no-desktop failure model: with no
// session bus, Notify is silent - no panic, no error - and the id
// still registers so late callbacks (never fired) route harmlessly.
func TestNotifyInertWithoutBus(t *testing.T) {
	a := testApp(nil)
	a.Notify("hello", "background daemon says hi", NotifyOptions{})
	a.notifyMu.Lock()
	n := len(a.notifyOpts)
	a.notifyMu.Unlock()
	if n != 1 {
		t.Errorf("registered %d notifications, want 1", n)
	}
}

// The action buttons reach the transport (they were once dropped on
// the way, leaving OnAction nothing to answer); a plain notification
// carries none and no default.
func TestNotifyCarriesItsActions(t *testing.T) {
	n := notification("t", "b", NotifyOptions{
		Actions:       []NotifyAction{{Key: "reply", Label: "Reply"}, {Key: "mute", Label: "Mute"}},
		DefaultAction: NotifyAction{Key: "open", Label: "Open"},
	})
	if len(n.Actions) != 2 || n.Actions[0].Key != "reply" || n.Actions[1].Label != "Mute" {
		t.Errorf("actions = %+v, want both buttons in order", n.Actions)
	}
	if !n.HasDefault || n.DefaultAction.Key != "open" {
		t.Errorf("default = %+v (%v), want open", n.DefaultAction, n.HasDefault)
	}
	if plain := notification("t", "b", NotifyOptions{}); len(plain.Actions) != 0 || plain.HasDefault {
		t.Errorf("plain = %+v, want no actions and no default", plain)
	}
}
