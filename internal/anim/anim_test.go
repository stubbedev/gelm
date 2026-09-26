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

// pinClock stops the launch/deadline clock at t0 for the test's run.
// Tick still takes its instant explicitly, so a pinned clock pins the
// whole schedule.
func pinClock(t *testing.T, t0 time.Time) {
	t.Helper()
	old := clock
	clock = func() time.Time { return t0 }
	t.Cleanup(func() { clock = old })
}

// TestTickDeterminism drives a tween over an injected clock: every
// wake lands exactly one frame period past the last tick, callbacks
// fire with the exact eased value for that instant, and the final
// callback is exactly 1 at the tween's end.
func TestTickDeterminism(t *testing.T) {
	t0 := time.Unix(0, 0)
	pinClock(t, t0)
	Reset()

	var got []float64
	end := t0.Add(100 * time.Millisecond)
	Start(100*time.Millisecond, func(p float64) { got = append(got, p) })

	for i := 0; Active(); i++ {
		wake, ok := Next()
		if !ok {
			t.Fatal("active tween scheduled no wake")
		}
		want := t0.Add(time.Duration(i+1) * FrameInterval)
		if want.After(end) {
			want = end // the landing tick is the tween's end, not a frame
		}
		if wake != want {
			t.Fatalf("wake %d at %v, want exactly %v", i, wake, want)
		}
		Tick(wake)
	}

	// 100ms at 60Hz: six in-flight ticks, then the end-of-tween tick.
	if len(got) != 7 {
		t.Fatalf("callbacks = %d, want 7 (six frames plus the landing)", len(got))
	}
	if last := got[len(got)-1]; last != 1 {
		t.Errorf("final progress = %v, want exactly 1", last)
	}
	mid := EaseOutCubic(float64(FrameInterval) / float64(100*time.Millisecond))
	if got[0] != mid {
		t.Errorf("first progress = %v, want eased %v for one frame in", got[0], mid)
	}
	if _, ok := Next(); ok {
		t.Error("finished tween still scheduled a wake")
	}
}

// TestCancelMidFlight pins the slider-vs-tween contract: Cancel stops
// the callbacks at once, leaves the last value in place, retires the
// tween from Active and Next, and is safe to repeat.
func TestCancelMidFlight(t *testing.T) {
	t0 := time.Unix(0, 0)
	pinClock(t, t0)
	Reset()

	calls, last := 0, -1.0
	cancel := Start(time.Second, func(p float64) {
		calls++
		last = p
	})
	Tick(t0.Add(50 * time.Millisecond))
	if calls != 1 {
		t.Fatalf("calls = %d, want 1 before the cancel", calls)
	}

	cancel()
	Tick(t0.Add(100 * time.Millisecond))
	Tick(t0.Add(2 * time.Second))
	if calls != 1 {
		t.Errorf("calls = %d after cancel, want no further callbacks", calls)
	}
	if last < 0 || last >= 1 {
		t.Errorf("last value = %v, want the mid-flight value kept", last)
	}
	if Active() {
		t.Error("canceled tween still counts as active")
	}
	if _, ok := Next(); ok {
		t.Error("canceled tween still scheduled a wake")
	}
	cancel() // repeated cancels do nothing
}

// TestCancelStopsTimeline kills a sequence while it holds in a delay:
// the later tween must never start.
func TestCancelStopsTimeline(t *testing.T) {
	t0 := time.Unix(0, 0)
	pinClock(t, t0)
	Reset()

	ran := false
	cancel := Play(Sequence(
		Delay(500*time.Millisecond),
		Animate(100*time.Millisecond, func(float64) { ran = true }),
	))
	Tick(t0.Add(100 * time.Millisecond))
	if !Active() {
		t.Fatal("timeline in a delay no longer counts as active")
	}
	cancel()
	Tick(t0.Add(2 * time.Second))
	if ran {
		t.Error("tween after a canceled delay ran anyway")
	}
	if Active() {
		t.Error("canceled timeline still counts as active")
	}
}

// TestSequenceOrdering runs a tween, a hold, and another tween through
// an injected clock: the first lands exactly 1, the hold costs no
// callbacks, and the second only starts after the hold.
func TestSequenceOrdering(t *testing.T) {
	t0 := time.Unix(0, 0)
	pinClock(t, t0)
	Reset()

	var order []string
	land := func(name string) func(float64) {
		return func(p float64) {
			if p == 1 {
				order = append(order, name)
			}
		}
	}
	Play(Sequence(
		Animate(100*time.Millisecond, land("a")),
		Delay(50*time.Millisecond),
		Animate(100*time.Millisecond, land("b")),
	))

	Tick(t0.Add(100 * time.Millisecond)) // a lands; b starts only now
	if len(order) != 1 || order[0] != "a" {
		t.Fatalf("order after the first tween = %v, want [a]", order)
	}
	Tick(t0.Add(140 * time.Millisecond)) // still inside the hold
	if len(order) != 1 {
		t.Errorf("callbacks during the delay: %v", order)
	}
	Tick(t0.Add(150 * time.Millisecond)) // b starts (progress 0)
	Tick(t0.Add(250 * time.Millisecond)) // b lands
	if len(order) != 2 || order[1] != "b" {
		t.Fatalf("order at the end = %v, want [a b]", order)
	}
	if Active() {
		t.Error("sequence still active after its last tween ended")
	}
}

// TestParallel runs two tweens together: the short one lands while the
// long one keeps the timeline (and the wake schedule) alive.
func TestParallel(t *testing.T) {
	t0 := time.Unix(0, 0)
	pinClock(t, t0)
	Reset()

	aDone, bDone := false, false
	Play(Parallel(
		Animate(50*time.Millisecond, func(float64) { aDone = true }),
		Animate(150*time.Millisecond, func(float64) { bDone = true }),
	))

	Tick(t0.Add(50 * time.Millisecond))
	if !aDone {
		t.Error("short parallel tween did not land at its end")
	}
	if !Active() {
		t.Fatal("long parallel tween dropped with the short one")
	}
	if wake, ok := Next(); !ok || wake != t0.Add(50*time.Millisecond).Add(FrameInterval) {
		t.Errorf("wake = %v, %v; want the frame period past the last tick", wake, ok)
	}
	Tick(t0.Add(150 * time.Millisecond))
	if !bDone {
		t.Error("long parallel tween did not land at its end")
	}
	if Active() {
		t.Error("parallel still active after both tweens ended")
	}
}

// TestNextWakesForStepStarts: while a timeline holds in a delay, the
// wake is the moment the next step begins - not a frame that would
// tick nothing.
func TestNextWakesForStepStarts(t *testing.T) {
	t0 := time.Unix(0, 0)
	pinClock(t, t0)
	Reset()

	Play(Sequence(Delay(100*time.Millisecond), Animate(50*time.Millisecond, func(float64) {})))
	wake, ok := Next()
	if !ok || wake != t0.Add(100*time.Millisecond) {
		t.Fatalf("wake = %v, %v; want the next step's start", wake, ok)
	}
	Tick(wake) // the tween starts (progress 0)
	wake, ok = Next()
	want := t0.Add(100 * time.Millisecond).Add(FrameInterval)
	if !ok || wake != want {
		t.Errorf("wake after the step started = %v, %v; want the frame deadline %v", wake, ok, want)
	}
}

// TestZeroDurationInsideTimeline: a zero-duration step delivers its
// end state once at launch, never through ticks.
func TestZeroDurationInsideTimeline(t *testing.T) {
	t0 := time.Unix(0, 0)
	pinClock(t, t0)
	Reset()

	ran := 0
	cancel := Play(Animate(0, func(p float64) {
		ran++
		if p != 1 {
			t.Errorf("progress = %v, want 1", p)
		}
	}))
	if ran != 1 {
		t.Fatalf("calls = %d at launch, want 1", ran)
	}
	Tick(t0.Add(time.Second))
	if ran != 1 {
		t.Errorf("calls = %d after a tick, want 1", ran)
	}
	cancel()
}

// TestPlayWithoutSteps: an empty timeline launches, cancels, and
// animates nothing.
func TestPlayWithoutSteps(t *testing.T) {
	Reset()
	cancel := Play()
	if Active() {
		t.Error("empty timeline is active")
	}
	cancel()
	cancel = Play(nil)
	cancel()
}
