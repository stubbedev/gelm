package app

import (
	"errors"
	"testing"
	"time"

	"github.com/stubbedev/gelm/internal/window"
	"github.com/stubbedev/gelm/internal/wlsession"
)

// The disconnect policy plans a rebuild only when reconnect is on, the
// connection was lost without a verdict, and every window can be
// rebuilt; the event tells the hook which way it goes.
func TestReconnectPlanning(t *testing.T) {
	lost := &wlsession.DisconnectError{Reason: DisconnectConnectionLost, Err: errors.New("eof")}
	verdict := &wlsession.DisconnectError{Reason: DisconnectProtocol, Err: errors.New("bad")}
	mk := func(windows ...*hostWindow) (*Application, *[]DisconnectedEvent) {
		var events []DisconnectedEvent
		a := &Application{windows: windows}
		a.OnDisconnect(func(ev DisconnectedEvent) { events = append(events, ev) })
		return a, &events
	}
	own := func() *hostWindow { return &hostWindow{host: &fakeHost{}, win: &Window{win: &window.Window{}}} }

	a, events := mk(own())
	a.handleDisconnect(lost)
	if a.rebuildPlan != nil || (*events)[0].Reconnecting {
		t.Error("planned a rebuild without SetReconnect")
	}

	a, events = mk(own())
	a.SetReconnect(&ReconnectOptions{})
	a.handleDisconnect(lost)
	if len(a.rebuildPlan) != 1 || !(*events)[0].Reconnecting || a.windows != nil {
		t.Errorf("lost connection: plan %d, reconnecting %v, windows left %d", len(a.rebuildPlan), (*events)[0].Reconnecting, len(a.windows))
	}

	a, events = mk(own())
	a.SetReconnect(&ReconnectOptions{})
	a.handleDisconnect(verdict)
	if a.rebuildPlan != nil || (*events)[0].Reconnecting {
		t.Error("planned a rebuild after a protocol verdict")
	}

	a, events = mk(own(), &hostWindow{host: &fakeHost{}}) // a foreign host
	a.SetReconnect(&ReconnectOptions{})
	a.handleDisconnect(lost)
	if a.rebuildPlan != nil || (*events)[0].Reconnecting {
		t.Error("planned a rebuild with a host the app cannot rebuild")
	}
}

// Without a plan, or for an error that is not a disconnect, the loop
// does not go on; a compositor that never comes back fails the
// reconnect within the timeout and leaves no plan behind.
func TestReconnectAfter(t *testing.T) {
	a := &Application{}
	if a.reconnectAfter(ErrDisconnected) {
		t.Error("reconnected with nothing planned")
	}
	dials := 0
	a.SetReconnect(&ReconnectOptions{
		Timeout: 200 * time.Millisecond,
		Connect: func() (*wlsession.Session, error) { dials++; return nil, errors.New("no compositor") },
	})
	a.rebuildPlan = []*hostWindow{}
	if a.reconnectAfter(errors.New("draw failed")) || a.rebuildPlan != nil {
		t.Error("an unrelated error reconnected or kept the plan")
	}
	a.rebuildPlan = []*hostWindow{}
	start := time.Now()
	if a.reconnectAfter(ErrDisconnected) {
		t.Error("reconnected to nothing")
	}
	if d := time.Since(start); d > 2*time.Second || dials < 2 {
		t.Errorf("redial took %s over %d attempts, want retries within the timeout", d, dials)
	}
}
