package component

import (
	"slices"
	"sync"
	"sync/atomic"
)

// StateWatcher observes a launched component's model, relm4's
// Controller::state: a controller's owner subscribes to the model as
// it changes after each batch of updates, and reads it between
// changes. Reach it through Controller.State.
type StateWatcher[In, Out any] struct {
	cx *Context[In, Out]
}

// Subscribe registers fn to run on the component's loop after every
// batch of its updates, receiving the component itself as the live
// model: C is the component's type, inferred from fn. There is no
// initial call and no copy of the model, so fn must only read it: it
// must not change the model, and must not send the component input -
// a Send from inside the notification re-enters the component it
// observes. lys-ui cannot run it reentrantly (input sent from inside a
// batch lands on the next loop pass, like a self-send from Update),
// but an observer has no reason to rely on that. Subscribe is safe
// from any goroutine, and after shutdown fn never runs. The returned
// unsubscribe is idempotent, safe from any goroutine and safe after
// shutdown.
func (w *StateWatcher[In, Out]) Subscribe[C Component[In, Out]](fn func(C)) (unsubscribe func()) {
	model, ok := w.cx.model.(C)
	if !ok {
		panic("component: Subscribe with a type the component is not")
	}
	if w.cx.dead {
		return func() {}
	}
	return w.cx.state.add(func() { fn(model) })
}

// Model returns the component's model as it is now, typed as C: the
// same value Launch or Context.Launch was given. Read it on the loop -
// from a Subscribe callback, or a parent's own Update - so it does not
// race a batch in flight.
func (w *StateWatcher[In, Out]) Model[C Component[In, Out]]() C {
	model, ok := w.cx.model.(C)
	if !ok {
		panic("component: Model with a type the component is not")
	}
	return model
}

// stateSubs is a component's state subscriptions: added and released
// from any goroutine, notified on the loop after each batch of
// updates.
type stateSubs struct {
	mu   sync.Mutex
	subs []*stateSub
}

type stateSub struct {
	fn      func()
	stopped atomic.Bool
}

func (s *stateSubs) add(fn func()) (unsubscribe func()) {
	sub := &stateSub{fn: fn}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = append(s.subs, sub)
	return sync.OnceFunc(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		sub.stopped.Store(true)
		s.subs = slices.DeleteFunc(s.subs, func(x *stateSub) bool { return x == sub })
	})
}

func (s *stateSubs) notify() {
	s.mu.Lock()
	subs := slices.Clone(s.subs)
	s.mu.Unlock()
	for _, sub := range subs {
		if sub.stopped.Load() {
			continue
		}
		sub.fn()
	}
}

func (s *stateSubs) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sub := range s.subs {
		sub.stopped.Store(true)
	}
	s.subs = nil
}
