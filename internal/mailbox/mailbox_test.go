package mailbox

import (
	"slices"
	"sync"
	"testing"
)

func TestPutArmsOnceUntilTake(t *testing.T) {
	var b Box[int]
	arms := 0
	for i := range 100 {
		if b.Put(i) {
			arms++
		}
	}
	if arms != 1 {
		t.Fatalf("100 puts armed %d drains, want 1", arms)
	}
	if got := b.Take(); len(got) != 100 || got[0] != 0 || got[99] != 99 {
		t.Fatalf("take = %d items, want 0..99 in order", len(got))
	}
	if !b.Put(7) {
		t.Error("put after take did not re-arm")
	}
}

func TestStopDropsAndRefuses(t *testing.T) {
	var b Box[string]
	b.Put("queued")
	b.Stop()
	if b.Put("late") {
		t.Error("a stopped box armed a drain")
	}
	if got := b.Take(); len(got) != 0 {
		t.Errorf("a stopped box kept %v", got)
	}
	if !b.Stopped() {
		t.Error("Stopped is false after Stop")
	}
}

func TestConcurrentPutsKeepPerProducerOrder(t *testing.T) {
	var b Box[[2]int]
	var wg sync.WaitGroup
	for p := range 4 {
		wg.Go(func() {
			for i := range 250 {
				b.Put([2]int{p, i})
			}
		})
	}
	wg.Wait()
	seen := make([][]int, 4)
	for _, it := range b.Take() {
		seen[it[0]] = append(seen[it[0]], it[1])
	}
	for p, got := range seen {
		if len(got) != 250 || !slices.IsSorted(got) {
			t.Errorf("producer %d: %d items, sorted=%v", p, len(got), slices.IsSorted(got))
		}
	}
}
