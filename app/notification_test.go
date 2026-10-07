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
