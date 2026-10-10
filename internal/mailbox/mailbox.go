// Package mailbox is the coalesced-wake queue every loop messenger
// shares: items appended from any goroutine, drained in order on the
// loop by one closure the first append schedules.
package mailbox

import "sync"

// Box queues items for one drain closure. The zero value is ready.
type Box[T any] struct {
	mu    sync.Mutex
	items []T
	armed bool
	dead  bool
}

// Put appends item and reports whether the caller must schedule the
// drain: true only for the first append since the last Take. A stopped
// box drops the item and reports false.
func (b *Box[T]) Put(item T) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dead {
		return false
	}
	b.items = append(b.items, item)
	if b.armed {
		return false
	}
	b.armed = true
	return true
}

// Take returns the queued items in order and disarms the box, so the
// next Put schedules a fresh drain.
func (b *Box[T]) Take() []T {
	b.mu.Lock()
	defer b.mu.Unlock()
	items := b.items
	b.items = nil
	b.armed = false
	return items
}

// Pending counts the queued items.
func (b *Box[T]) Pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}

// Stop drops the queued items and makes every later Put a no-op.
func (b *Box[T]) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.dead = true
	b.items = nil
	b.armed = false
}

// Stopped reports whether Stop ran.
func (b *Box[T]) Stopped() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dead
}
