package app

import (
	"slices"
	"sync"
)

// This file is the typed messaging layer above Invoke (docs/threading.md):
// the optional relm4-style helpers - Component, Stream (component outputs,
// message brokers), and SharedState - built on the same coalesced-wake
// mailbox shape the Invoke queue itself uses, so a burst of messages costs
// one loop wake no matter how many Send calls produced it. Callbacks stay
// the runtime: every delivery happens on the loop goroutine with
// event-callback guarantees, and nothing here is an actor framework.

// mailbox is the shared queue shape behind the typed messengers: values
// appended from any goroutine, drained in order on the loop goroutine by a
// single closure the first append routes through Invoke. That is the same
// armed-wake rule loopQueues applies to Invoke closures: while a drain is
// pending, further appends are free; a storm of Sends arms exactly one
// wake.
type mailbox[T any] struct {
	mu    sync.Mutex
	items []T
	armed bool
	dead  bool
}

// put appends item and reports whether the caller must route the drain
// closure onto the loop: true only for the first append after a drain, the
// coalesced-wake rule. A dead mailbox drops the item and reports false.
func (m *mailbox[T]) put(item T) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dead {
		return false
	}
	m.items = append(m.items, item)
	if m.armed {
		return false
	}
	m.armed = true
	return true
}

// take returns the queued items in order and disarms, so the next put
// routes a fresh drain. The caller delivers the items on the loop
// goroutine.
func (m *mailbox[T]) take() []T {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := m.items
	m.items = nil
	m.armed = false
	return items
}

// pending counts queued items (tests).
func (m *mailbox[T]) pending() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

// stop ends the mailbox: queued items are dropped and later puts are
// no-ops, so a messenger closed by its owner or by the loop's end cannot
// accumulate items from a producer that keeps running.
func (m *mailbox[T]) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dead = true
	m.items = nil
	m.armed = false
}

// stopSet is the application's loop-owned stoppables: the fd/file
// watchers' halt hooks and the typed messengers' stop methods. Run's exit
// stops every one, so nothing registered here outlives the loop with state
// that could still be fed.
type stopSet struct {
	mu   sync.Mutex
	next int
	live map[int]func()
	dead bool
}

func (s *stopSet) add(stop func()) (id int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead {
		return 0, false
	}
	if s.live == nil {
		s.live = map[int]func(){}
	}
	s.next++
	s.live[s.next] = stop
	return s.next, true
}

func (s *stopSet) remove(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.live, id)
}

// shutdown stops every registered stoppable; later ones are refused.
func (s *stopSet) shutdown() {
	s.mu.Lock()
	stops := s.live
	s.live, s.dead = nil, true
	s.mu.Unlock()
	for _, stop := range stops {
		stop()
	}
}

// subscriberSet is the fan-out half of a messenger: callbacks added from
// any goroutine, snapshotted per message for delivery on the loop in
// subscription order.
type subscriberSet[Msg any] struct {
	mu   sync.Mutex
	next int
	subs []subscriber[Msg]
}

type subscriber[Msg any] struct {
	id int
	fn func(Msg)
}

func (s *subscriberSet[Msg]) add(fn func(Msg)) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	s.subs = append(s.subs, subscriber[Msg]{id: s.next, fn: fn})
	return s.next
}

func (s *subscriberSet[Msg]) remove(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs = slices.DeleteFunc(s.subs, func(sub subscriber[Msg]) bool { return sub.id == id })
}

func (s *subscriberSet[Msg]) snapshot() []subscriber[Msg] {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.subs)
}

// Stream is a typed publish/subscribe messenger onto the loop goroutine:
// Send publishes from any goroutine, Subscribe registers a callback that
// receives every message in Send order. It is the one shape behind the
// relm4 concepts that are all fan-out under different names: a component
// output stream (ComponentStream), a message broker (a Stream held in a
// package variable, so any nesting depth reaches it without threading
// senders), and SharedState change notifications.
//
// Delivery is deferred and non-reentrant: a Send from inside a subscriber
// (or from the loop itself) lands at the top of the loop's next pass.
type Stream[Msg any] struct {
	app  *Application
	subs subscriberSet[Msg]
	box  mailbox[Msg]
	id   int
}

// NewStream creates a Stream on the application's loop. After the loop
// ends it comes back stopped: Sends are dropped, subscriptions never
// fire.
func NewStream[Msg any](a *Application) *Stream[Msg] {
	s := &Stream[Msg]{app: a}
	id, ok := a.watchers.add(s.stop)
	if ok {
		s.id = id
	} else {
		s.box.stop()
	}
	return s
}

// Send publishes msg to every subscriber on the loop goroutine. Safe from
// any goroutine; a burst of Sends drains as one batch per loop pass.
func (s *Stream[Msg]) Send(msg Msg) {
	if s.box.put(msg) {
		s.app.Invoke(s.drain)
	}
}

// Subscribe registers fn to receive every message on the loop goroutine.
// The returned cancel removes the subscription (idempotent, safe from any
// goroutine); it takes effect at the next message, so a cancel racing a
// pending batch never rewinds deliveries already made.
func (s *Stream[Msg]) Subscribe(fn func(Msg)) (cancel func()) {
	if fn == nil {
		return func() {}
	}
	id := s.subs.add(fn)
	return sync.OnceFunc(func() { s.subs.remove(id) })
}

// Close stops the stream: queued messages are dropped, later Sends are
// no-ops, and subscriptions stop firing. The loop's end closes it too.
func (s *Stream[Msg]) Close() {
	s.stop()
	s.app.watchers.remove(s.id)
}

func (s *Stream[Msg]) stop() {
	s.box.stop()
}

// drain is the closure Send routes through Invoke: it takes the batch and
// hands each message to the subscribers live at that message.
func (s *Stream[Msg]) drain() {
	for _, msg := range s.box.take() {
		for _, sub := range s.subs.snapshot() {
			sub.fn(msg)
		}
	}
}

// Component is the optional typed update helper of docs/threading.md:
// update runs on the loop goroutine once per Send, in Send order - a
// typed Invoke. It is relm4's Component with the framework removed: the
// model is whatever update closes over, and a component with outputs
// Sends to a Stream it was given, so every crossing onto the loop stays
// visible as the one Invoke-shaped hop this package already owns.
type Component[Msg any] struct {
	app    *Application
	update func(Msg)
	box    mailbox[Msg]
	id     int
	mu     sync.Mutex
	detach bool
}

// NewComponent creates a Component whose update runs on the application's
// loop goroutine. After the loop ends it comes back stopped: Sends are
// dropped and update never runs.
func NewComponent[Msg any](a *Application, update func(Msg)) *Component[Msg] {
	c := &Component[Msg]{app: a, update: update}
	id, ok := a.watchers.add(c.stop)
	if ok {
		c.id = id
	} else {
		c.box.stop()
	}
	return c
}

// Send delivers msg to update on the loop goroutine, in Send order. Safe
// from any goroutine, from callbacks, and from inside update itself (a
// self-Send lands on the next pass, never reentrantly).
func (c *Component[Msg]) Send(msg Msg) {
	if c.box.put(msg) {
		c.app.Invoke(c.drain)
	}
}

// Detach shields the component from Shutdown: a handle handed to a scoped
// owner - a window that shuts down everything it created on close - keeps
// a shared component running. It is relm4's detach, with shutdown made
// explicit instead of drop-driven.
func (c *Component[Msg]) Detach() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.detach = true
}

// Shutdown stops the component: queued messages are dropped and later
// Sends are no-ops. A no-op itself after Detach. The loop's end shuts
// every component down too.
func (c *Component[Msg]) Shutdown() {
	c.mu.Lock()
	detached := c.detach
	c.mu.Unlock()
	if detached {
		return
	}
	c.stop()
	c.app.watchers.remove(c.id)
}

func (c *Component[Msg]) stop() {
	c.box.stop()
}

// drain is the closure Send routes through Invoke: it takes the batch and
// hands each message to update in order.
func (c *Component[Msg]) drain() {
	for _, msg := range c.box.take() {
		c.update(msg)
	}
}

// SharedState is a loop-owned observable value: Update mutates it from any
// goroutine and delivers the new value to every subscriber on the loop
// goroutine - relm4's SharedState, with the change stream being the same
// Stream shape every other messenger uses. One state shared across
// windows updates every window's widgets from the one subscription point,
// which is what makes cross-window sharing correct by construction: there
// is no per-window copy to fall out of sync.
type SharedState[T any] struct {
	mu      sync.Mutex
	val     T
	changes *Stream[T]
}

// NewSharedState creates a SharedState holding initial, notifying on the
// application's loop.
func NewSharedState[T any](a *Application, initial T) *SharedState[T] {
	return &SharedState[T]{val: initial, changes: NewStream[T](a)}
}

// Get returns a snapshot of the value, safe from any goroutine: the read
// guard. Copying is the guard - a snapshot cannot be raced by a later
// Update.
func (s *SharedState[T]) Get() T {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.val
}

// Update applies f to the value and notifies subscribers with the new
// value on the loop goroutine. Notification is unconditional, like
// relm4's set: f already knows whether it changed anything, and callers
// that need change detection compare against Get inside f.
func (s *SharedState[T]) Update(f func(*T)) {
	s.mu.Lock()
	f(&s.val)
	next := s.val
	s.mu.Unlock()
	s.changes.Send(next)
}

// Subscribe registers fn to receive the value after every Update, on the
// loop goroutine. The returned cancel removes the subscription.
func (s *SharedState[T]) Subscribe(fn func(T)) (cancel func()) {
	return s.changes.Subscribe(fn)
}

// Close stops change notifications; later Updates still mutate the value.
func (s *SharedState[T]) Close() {
	s.changes.Close()
}
