package anim

import (
	"testing"
	"time"
)

func TestTweenProgress(t *testing.T) {
	Reset()
	calls := 0
	last := -1.0
	Start(100*time.Millisecond, func(p float64) {
		calls++
		last = p
	})

	t.Run("before any tick nothing runs", func(t *testing.T) {
		if calls != 0 {
			t.Errorf("calls = %d, want 0 before Tick", calls)
		}
	})

	t.Run("a tick mid-flight eases between 0 and 1", func(t *testing.T) {
		Tick(time.Now().Add(50 * time.Millisecond))
		if calls != 1 {
			t.Fatalf("calls = %d, want 1", calls)
		}
		if last <= 0 || last >= 1 {
			t.Errorf("progress = %v, want strictly inside (0,1)", last)
		}
	})

	t.Run("a tick past the end delivers 1 and prunes", func(t *testing.T) {
		Tick(time.Now().Add(2 * time.Second))
		if last != 1 {
			t.Errorf("final progress = %v, want 1", last)
		}
		if Active() {
			t.Error("finished tween still counts as active")
		}
		before := calls
		Tick(time.Now())
		if calls != before {
			t.Error("finished tween kept firing after pruning")
		}
	})
}

func TestZeroDurationRunsOnce(t *testing.T) {
	Reset()
	ran := 0
	Start(0, func(p float64) {
		ran++
		if p != 1 {
			t.Errorf("progress = %v, want 1 immediately", p)
		}
	})
	if ran != 1 {
		t.Errorf("calls = %d, want exactly 1", ran)
	}
	if Active() {
		t.Error("zero-duration tween must not be active")
	}
}

func TestMultipleTweensIndependent(t *testing.T) {
	Reset()
	aDone, bDone := false, false
	Start(50*time.Millisecond, func(float64) { aDone = true })
	Start(10*time.Second, func(float64) {})
	Tick(time.Now().Add(time.Second))
	if !aDone {
		t.Error("short tween did not fire")
	}
	if !Active() {
		t.Error("long tween pruned alongside the short one")
	}

	Start(time.Second, func(float64) { bDone = true })
	Tick(time.Now().Add(3 * time.Second))
	if !bDone {
		t.Error("the one-second tween never finished")
	}
	if !Active() {
		t.Error("the ten-second tween vanished early")
	}

	Tick(time.Now().Add(20 * time.Second))
	if Active() {
		t.Error("tweens still active after every duration elapsed")
	}
}

func TestNilFunctionIgnored(t *testing.T) {
	Reset()
	Start(time.Second, nil)
	if Active() {
		t.Error("nil tween registered")
	}
}
