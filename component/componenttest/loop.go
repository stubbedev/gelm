// Package componenttest runs components without a compositor: a Loop
// that queues invoked work until the test drives it.
package componenttest

import (
	"slices"
	"sync"
)

// Loop is a component.Loop driven by hand. Invoke queues work from any
// goroutine; Pass and Settle run it on the calling goroutine.
type Loop struct {
	mu      sync.Mutex
	queue   []func()
	stops   []*func()
	stopped bool
}

// Invoke queues fn for the next pass. After Stop it is dropped.
func (l *Loop) Invoke(fn func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.stopped {
		l.queue = append(l.queue, fn)
	}
}

// OnStop registers fn to run at Stop, in reverse registration order.
// After Stop it runs at once.
func (l *Loop) OnStop(fn func()) (cancel func()) {
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		fn()
		return func() {}
	}
	entry := &fn
	l.stops = append(l.stops, entry)
	l.mu.Unlock()
	return sync.OnceFunc(func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.stops = slices.DeleteFunc(l.stops, func(e *func()) bool { return e == entry })
	})
}

// Pass runs the work queued so far, like one loop pass: work queued
// while it runs waits for the next pass. It reports how much ran.
func (l *Loop) Pass() int {
	l.mu.Lock()
	queue := l.queue
	l.queue = nil
	l.mu.Unlock()
	for _, fn := range queue {
		fn()
	}
	return len(queue)
}

// Settle runs passes until no work is queued.
func (l *Loop) Settle() {
	for l.Pass() > 0 {
	}
}

// Stop ends the loop: queued work is dropped, stop hooks run, and later
// Invokes are dropped.
func (l *Loop) Stop() {
	l.mu.Lock()
	l.stopped = true
	l.queue = nil
	stops := l.stops
	l.stops = nil
	l.mu.Unlock()
	for _, fn := range slices.Backward(stops) {
		(*fn)()
	}
}
