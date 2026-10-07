package app

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestComponentCounterRunsUpdateOnLoop is the threading.md counter
// example made executable: Send crosses from a foreign goroutine as a
// typed message and update runs on the loop goroutine, in Send order -
// the off-loop hook stays silent exactly like an Invoke-routed touch.
func TestComponentCounterRunsUpdateOnLoop(t *testing.T) {
	a := testApp(nil)
	type msg struct{ kind string }
	type counter struct {
		n int
		s string
	}
	c := &counter{}
	comp := NewComponent(a, func(m msg) {
		c.n++
		c.s = strconv.Itoa(c.n)
	})

	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() { comp.Send(msg{"tick"}) })
	}
	wg.Wait()
	if !a.pump(time.Now()) {
		t.Fatal("a pending component drain did not run work")
	}
	if c.n != 3 || c.s != "3" {
		t.Errorf("counter after three ticks = %d %q, want 3 \"3\"", c.n, c.s)
	}
}

// TestComponentDeliversInSendOrder pins the ordering half of the
// component contract: messages from one goroutine deliver in Send
// order, and a self-Send from inside update lands on the next pass
// instead of reentering.
func TestComponentDeliversInSendOrder(t *testing.T) {
	a := testApp(nil)
	var order []int
	var comp *Component[int]
	comp = NewComponent(a, func(n int) {
		order = append(order, n)
		if n == 2 {
			comp.Send(3)
		}
	})
	comp.Send(1)
	comp.Send(2)
	a.pump(time.Now())
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("first pass delivered %v, want [1 2]", order)
	}
	a.pump(time.Now())
	if len(order) != 3 || order[2] != 3 {
		t.Errorf("self-send delivered %v, want it on the pass after [1 2]", order)
	}
}

// TestMessengerWakeCoalesces extends the invoke wake budget to the
// typed layer: a hundred Sends arm exactly one invoke - the drain
// closure - so a busy messenger rides one wake per batch, never one
// per message.
func TestMessengerWakeCoalesces(t *testing.T) {
	a := testApp(nil)
	comp := NewComponent(a, func(int) {})
	for range 100 {
		comp.Send(0)
	}
	if got := a.queues.pending(); got != 1 {
		t.Errorf("100 Sends left %d pending invokes, want the single drain", got)
	}
	a.pump(time.Now())
	if got := a.queues.pending(); got != 0 {
		t.Errorf("pump left %d pending invokes", got)
	}
	comp.Send(0)
	if got := a.queues.pending(); got != 1 {
		t.Error("Send after a drain did not route a fresh one")
	}
}

// TestStreamFansOutToSubscribers is the broker half: every subscriber
// sees every message in Send order, a cancel takes effect at the next
// message, and a subscriber added mid-stream sees later messages only.
func TestStreamFansOutToSubscribers(t *testing.T) {
	a := testApp(nil)
	broker := NewStream[string](a)
	var first, second, late []string
	cancel := broker.Subscribe(func(s string) { first = append(first, s) })
	broker.Subscribe(func(s string) { second = append(second, s) })

	broker.Send("one")
	broker.Send("two")
	a.pump(time.Now())
	cancel()
	broker.Subscribe(func(s string) { late = append(late, s) })
	broker.Send("three")
	a.pump(time.Now())

	if len(first) != 2 || first[1] != "two" {
		t.Errorf("first subscriber saw %v, want [one two]", first)
	}
	if len(second) != 3 || second[2] != "three" {
		t.Errorf("second subscriber saw %v, want all three", second)
	}
	if len(late) != 1 || late[0] != "three" {
		t.Errorf("late subscriber saw %v, want only [three]", late)
	}
}

// TestStreamSendFromSubscriberDefers pins the no-reentrancy rule: a
// Send from inside a subscriber never delivers inside the running
// pass - it lands on the next one.
func TestStreamSendFromSubscriberDefers(t *testing.T) {
	a := testApp(nil)
	broker := NewStream[int](a)
	n := 0
	broker.Subscribe(func(i int) {
		n += i
		if i < 2 {
			broker.Send(i + 1)
		}
	})
	broker.Send(1)
	a.pump(time.Now())
	if n != 1 {
		t.Fatalf("first pass summed %d, want 1 (no reentrant delivery)", n)
	}
	a.pump(time.Now())
	if n != 3 {
		t.Errorf("chained sends summed %d, want 3", n)
	}
}

// TestComponentShutdownAndDetach covers the ownership semantics:
// Shutdown drops queued and future messages, and Detach shields a
// shared component from a scoped owner's Shutdown.
func TestComponentShutdownAndDetach(t *testing.T) {
	a := testApp(nil)
	ran := 0
	comp := NewComponent(a, func(int) { ran++ })

	comp.Send(0)
	comp.Shutdown()
	comp.Send(0)
	a.pump(time.Now())
	if ran != 0 {
		t.Errorf("shutdown delivered %d messages, want 0", ran)
	}

	shared := NewComponent(a, func(int) { ran++ })
	shared.Detach()
	shared.Shutdown()
	shared.Send(0)
	a.pump(time.Now())
	if ran != 1 {
		t.Errorf("detached component delivered %d messages, want 1", ran)
	}
}

// TestMessengersDieWithLoop is the leak half of the lifecycle: what
// Run's exit does - stopping the loop-owned set - drops every
// messenger's queue and refuses later Sends, so a producer goroutine
// that outlives the loop cannot accumulate messages in a dead app.
func TestMessengersDieWithLoop(t *testing.T) {
	a := testApp(nil)
	comp := NewComponent(a, func(int) { t.Error("update ran after the loop ended") })
	broker := NewStream[int](a)
	broker.Subscribe(func(int) { t.Error("subscriber ran after the loop ended") })
	state := NewSharedState(a, 0)
	state.Subscribe(func(int) { t.Error("state notified after the loop ended") })

	comp.Send(0)
	broker.Send(0)
	state.Update(func(i *int) { *i++ })
	a.watchers.shutdown()
	a.pump(time.Now())

	comp.Send(0)
	broker.Send(0)
	state.Update(func(i *int) { *i++ })
	a.pump(time.Now())
	if got := state.Get(); got != 2 {
		t.Errorf("a closed SharedState stopped mutating: Get = %d, want 2", got)
	}
}

// TestSharedStateNotifiesWithValue covers the observable-value
// contract: Get snapshots from any goroutine, Update notifies with the
// new value on the loop, and the notification stream is the same
// subscribe/cancel shape as every other messenger.
func TestSharedStateNotifiesWithValue(t *testing.T) {
	a := testApp(nil)
	state := NewSharedState(a, 0)
	var seen []int
	state.Subscribe(func(v int) { seen = append(seen, v) })

	for v := range 3 {
		state.Update(func(i *int) { *i = v })
	}
	if got := state.Get(); got != 2 {
		t.Fatalf("Get = %d, want the last update", got)
	}
	a.pump(time.Now())
	if len(seen) != 3 || seen[0] != 0 || seen[2] != 2 {
		t.Errorf("notifications = %v, want [0 1 2]", seen)
	}
}

// TestStreamCloseDropsQueuedMessages pins Close on the broker shape:
// messages queued but not yet drained are dropped, not delivered.
func TestStreamCloseDropsQueuedMessages(t *testing.T) {
	a := testApp(nil)
	broker := NewStream[int](a)
	ran := false
	broker.Subscribe(func(int) { ran = true })
	broker.Send(0)
	broker.Close()
	a.pump(time.Now())
	if ran {
		t.Error("a message queued before Close was still delivered")
	}
}
