package app

import (
	"slices"
	"testing"
	"time"
)

// TestMessengerWakeCoalesces extends the invoke wake budget to the
// typed layer: a hundred Sends arm exactly one invoke - the drain
// closure - so a busy messenger rides one wake per batch, never one
// per message.
func TestMessengerWakeCoalesces(t *testing.T) {
	a := testApp(nil)
	broker := NewStream[int](a)
	for range 100 {
		broker.Send(0)
	}
	if got := a.queues.pending(); got != 1 {
		t.Errorf("100 Sends left %d pending invokes, want the single drain", got)
	}
	a.pump(time.Now())
	if got := a.queues.pending(); got != 0 {
		t.Errorf("pump left %d pending invokes", got)
	}
	broker.Send(0)
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

// TestMessengersDieWithLoop is the leak half of the lifecycle: what
// Run's exit does - stopping the loop-owned set - drops every
// messenger's queue and refuses later Sends, so a producer goroutine
// that outlives the loop cannot accumulate messages in a dead app.
func TestMessengersDieWithLoop(t *testing.T) {
	a := testApp(nil)
	broker := NewStream[int](a)
	broker.Subscribe(func(int) { t.Error("subscriber ran after the loop ended") })
	state := NewSharedState(a, 0)
	state.Subscribe(func(int) { t.Error("state notified after the loop ended") })

	broker.Send(0)
	state.Update(func(i *int) { *i++ })
	a.watchers.shutdown()
	a.pump(time.Now())

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

func TestStreamSubscribersFireInSubscriptionOrder(t *testing.T) {
	a := testApp(nil)
	broker := NewStream[int](a)
	const subscribers = 32
	var order []int
	cancels := make([]func(), subscribers)
	for i := range subscribers {
		cancels[i] = broker.Subscribe(func(int) { order = append(order, i) })
	}
	cancels[5]()
	for range 50 {
		broker.Send(0)
	}
	a.pump(time.Now())
	want := make([]int, 0, subscribers-1)
	for i := range subscribers {
		if i != 5 {
			want = append(want, i)
		}
	}
	for msg := range 50 {
		got := order[msg*len(want) : (msg+1)*len(want)]
		if !slices.Equal(got, want) {
			t.Fatalf("message %d reached subscribers in order %v, want %v", msg, got, want)
		}
	}
}

func TestOnStopRunsHooksInReverseOnce(t *testing.T) {
	a := testApp(nil)
	var order []int
	for i := range 3 {
		a.OnStop(func() { order = append(order, i) })
	}
	cancel := a.OnStop(func() { t.Error("a cancelled hook ran") })
	cancel()
	a.watchers.shutdown()
	if !slices.Equal(order, []int{2, 1, 0}) {
		t.Errorf("stop hooks ran %v, want [2 1 0]", order)
	}
	ran := false
	a.OnStop(func() { ran = true })
	if !ran {
		t.Error("a hook registered after the loop ended did not run at once")
	}
}
