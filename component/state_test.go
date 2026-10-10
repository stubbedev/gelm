package component_test

import (
	"slices"
	"sync"
	"testing"

	"github.com/stubbedev/gelm/component"
	"github.com/stubbedev/gelm/component/componenttest"
	"github.com/stubbedev/gelm/widget"
)

func TestStateNotifiesAfterEachBatchOnTheLoop(t *testing.T) {
	var loop componenttest.Loop
	c := &counter{face: face(t)}
	ctrl := component.Launch(&loop, c)
	var seen []int
	unsubscribe := ctrl.State().Subscribe(func(m *counter) { seen = append(seen, m.n) })
	if len(seen) != 0 {
		t.Fatal("Subscribe ran its callback before any update")
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 3 {
			ctrl.Send(increment)
		}
	})
	wg.Wait()
	if len(seen) != 0 {
		t.Fatal("a state notification ran on the sender's goroutine")
	}
	loop.Settle()
	if !slices.Equal(seen, []int{3}) {
		t.Errorf("notifications saw %v, want one per batch: [3]", seen)
	}

	ctrl.Send(reset)
	ctrl.Send(increment)
	loop.Settle()
	if !slices.Equal(seen, []int{3, 1}) {
		t.Errorf("notifications saw %v, want [3 1] for the second batch", seen)
	}
	unsubscribe()
	ctrl.Send(increment)
	loop.Settle()
	if !slices.Equal(seen, []int{3, 1}) {
		t.Errorf("notifications after unsubscribe: %v", seen)
	}
	unsubscribe()
}

func TestStateUnsubscribeIsIdempotentAndSafeAfterShutdown(t *testing.T) {
	var loop componenttest.Loop
	ctrl := component.Launch(&loop, &counter{face: face(t)})
	var n int
	unsubscribe := ctrl.State().Subscribe(func(*counter) { n++ })
	ctrl.Send(increment)
	loop.Settle()
	if n != 1 {
		t.Fatalf("notifications: %d, want 1", n)
	}

	ctrl.Shutdown()
	unsubscribe()
	unsubscribe()
	ctrl.Send(increment)
	loop.Settle()
	if n != 1 {
		t.Errorf("notifications after shutdown: %d, want 1", n)
	}
	late := ctrl.State().Subscribe(func(*counter) { n++ })
	late()
	if n != 1 {
		t.Errorf("a subscription after shutdown fired: %d", n)
	}
}

func TestStateNotificationMayUnsubscribeOrShutdownWithoutDeadlock(t *testing.T) {
	var loop componenttest.Loop
	done := make(chan *counter, 1)
	ctrl := component.Launch(&loop, &counter{face: face(t)})
	ctrl.State().Subscribe(func(m *counter) {
		ctrl.Shutdown()
		done <- m
	})
	var wg sync.WaitGroup
	wg.Go(func() { ctrl.Send(increment) })
	wg.Wait()
	loop.Settle()
	if m := <-done; m.n != 1 {
		t.Errorf("notification saw n=%d, want 1", m.n)
	}
	if ctrl.Alive() {
		t.Error("the child outlived a shutdown from its state notification")
	}
}

func TestStateModelIsTypedAndCurrent(t *testing.T) {
	var loop componenttest.Loop
	c := &counter{face: face(t)}
	ctrl := component.Launch(&loop, c)
	m := ctrl.State().Model[*counter]()
	if m != c {
		t.Fatal("Model returned another value than the launched component")
	}
	ctrl.Send(increment)
	loop.Settle()
	if m.n != 1 {
		t.Errorf("Model's n=%d, want 1", m.n)
	}
}

func TestStateNotificationSendLandsOnTheNextPass(t *testing.T) {
	var loop componenttest.Loop
	c := &counter{face: face(t)}
	ctrl := component.Launch(&loop, c)
	var first sync.Once
	reentered := false
	ctrl.State().Subscribe(func(m *counter) {
		first.Do(func() {
			before := len(m.updates)
			ctrl.Send(increment)
			if len(m.updates) != before {
				reentered = true
			}
		})
	})
	ctrl.Send(increment)
	loop.Settle()
	if reentered {
		t.Error("Send re-entered Update during the state notification")
	}
	if c.n != 2 {
		t.Errorf("n=%d, want the notification's Send applied on the next pass", c.n)
	}
}

func TestStateSubscribeWithAWrongTypePanics(t *testing.T) {
	var loop componenttest.Loop
	ctrl := component.Launch(&loop, &counter{face: face(t)})
	defer func() {
		if recover() == nil {
			t.Error("Subscribe with a type the component is not did not panic")
		}
	}()
	ctrl.State().Subscribe(func(*otherCounter) {})
}

type otherCounter struct{}

func (*otherCounter) Init(*component.Context[counterMsg, counterOut]) widget.Widget {
	return widget.NewSpacer(1, 1)
}

func (*otherCounter) Update(*component.Context[counterMsg, counterOut], counterMsg) {}
