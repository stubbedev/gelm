package app

import (
	"testing"
	"time"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// repNext adapts the repeater's deadline for nextWake, collapsing the
// not-held case to the zero time.
func repNext(rep *keyRepeater) time.Time {
	t, ok := rep.nextDeadline()
	if !ok {
		return time.Time{}
	}
	return t
}

// animNext adapts anim.Next the same way.
func animNext() time.Time {
	t, ok := anim.Next()
	if !ok {
		return time.Time{}
	}
	return t
}

func TestNextWake(t *testing.T) {
	never := time.Time{}
	now := time.Now()

	t.Run("a fully idle loop never wakes", func(t *testing.T) {
		if _, ok := nextWake(repNext(newKeyRepeater(0, 0)), animNext(), never, now); ok {
			t.Errorf("idle loop scheduled a wakeup")
		}
	})

	t.Run("a held key wakes at its repeat deadline", func(t *testing.T) {
		rep := newKeyRepeater(0, 0)
		rep.press(30, 0)
		wake, ok := nextWake(repNext(rep), animNext(), never, now)
		if !ok {
			t.Fatal("held key did not schedule a wakeup")
		}
		if d := time.Until(wake); d < 300*time.Millisecond || d > 500*time.Millisecond {
			t.Errorf("wake in %v, want ~400ms repeat delay", d)
		}
	})

	t.Run("a released key stops waking", func(t *testing.T) {
		rep := newKeyRepeater(0, 0)
		rep.press(30, 0)
		rep.release(30)
		if _, ok := nextWake(repNext(rep), animNext(), never, now); ok {
			t.Errorf("released key still scheduled wakeups")
		}
	})

	t.Run("a running animation wakes at its end", func(t *testing.T) {
		anim.Reset()
		anim.Start(250*time.Millisecond, func(float64) {})
		wake, ok := nextWake(repNext(newKeyRepeater(0, 0)), animNext(), never, now)
		if !ok {
			t.Fatal("animation did not schedule a wakeup")
		}
		if d := time.Until(wake); d > 260*time.Millisecond {
			t.Errorf("wake in %v, want ~250ms tween end", d)
		}
	})

	t.Run("the earliest deadline wins", func(t *testing.T) {
		anim.Reset()
		anim.Start(20*time.Millisecond, func(float64) {})
		rep := newKeyRepeater(0, 0)
		rep.press(30, 0)
		wake, ok := nextWake(repNext(rep), animNext(), never, now)
		if !ok {
			t.Fatal("no wakeup scheduled")
		}
		if d := time.Until(wake); d > 60*time.Millisecond {
			t.Errorf("wake in %v, want the ~20ms animation deadline", d)
		}
	})

	t.Run("already-due deadlines do not wake", func(t *testing.T) {
		if _, ok := nextWake(never, never, now.Add(-time.Second), now); ok {
			t.Errorf("past deadline scheduled a zero-duration wake")
		}
	})
}

// TestIdleLoopDoesNotWake drives the loop's wake decision through a
// 200ms idle window: with no key held, no animation, and no tooltip
// pending, zero wakeups may be scheduled. This is the acceptance hook
// for the parked event loop; the pre-park loop burned a core here.
func TestIdleLoopDoesNotWake(t *testing.T) {
	anim.Reset()
	rep := newKeyRepeater(0, 0)
	wakes := 0
	start := time.Now()
	for time.Since(start) < 200*time.Millisecond {
		if _, ok := nextWake(repNext(rep), animNext(), time.Time{}, time.Now()); ok {
			wakes++
		}
		// A real loop parks in Session.Step here, waking on events.
	}
	if wakes != 0 {
		t.Errorf("idle window scheduled %d wakeups, want 0", wakes)
	}
}

// TestKeyRepeatLatency pins responsiveness: a press routes immediately
// (the key event itself wakes the loop) and repeats land at the
// compositor's rate afterwards.
func TestKeyRepeatLatency(t *testing.T) {
	rep := newKeyRepeater(10, 100)
	rep.press(30, 0)

	presses := 0
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, _, ok := rep.tick(); ok {
			presses++
		}
		time.Sleep(10 * time.Millisecond)
	}
	if presses < 1 || presses > 3 {
		t.Errorf("repeats in 250ms = %d, want 1-2 at 10/s", presses)
	}
}

// TestShouldDraw pins the frame-pacing gate, especially the resize
// bypass: a configure-driven repaint must go out even with a frame
// still pending, because compositors withhold that callback until the
// surface commits at the configured size - gating it deadlocks the
// resize.
func TestShouldDraw(t *testing.T) {
	t.Run("an idle window never draws", func(t *testing.T) {
		if shouldDraw(false, false, false, false) {
			t.Error("undirtied window drew")
		}
	})
	t.Run("a dirty window waits for its frame callback", func(t *testing.T) {
		if shouldDraw(true, true, false, false) {
			t.Error("drew while the previous frame was pending")
		}
	})
	t.Run("a returned callback lets the frame go out", func(t *testing.T) {
		if !shouldDraw(true, false, false, false) {
			t.Error("pacing blocked a ready frame")
		}
	})
	t.Run("an animation keeps producing frames", func(t *testing.T) {
		if !shouldDraw(true, true, true, false) {
			t.Error("animation frames blocked by pacing")
		}
	})
	t.Run("a configure resize bypasses pacing", func(t *testing.T) {
		if !shouldDraw(true, true, false, true) {
			t.Error("resize repaint blocked by pacing; the resize would deadlock")
		}
	})
}

func TestTooltipNext(t *testing.T) {
	target := newTipTarget("hover text")
	target.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 20})
	router := &widget.Router{Root: target}

	t.Run("pending hover schedules its delay", func(t *testing.T) {
		tip := &tooltipCtl{since: time.Now()}
		router.Move(widget.Point{X: 5, Y: 5})
		tip.update(router, time.Now(), func(widget.Widget, string) tooltipWindow { return nil })
		wake, ok := tip.next()
		if !ok {
			t.Fatal("pending tooltip did not schedule a wakeup")
		}
		if d := time.Until(wake); d < 300*time.Millisecond || d > 500*time.Millisecond {
			t.Errorf("wake in %v, want ~500ms tooltip delay", d)
		}
	})

	t.Run("no hover never wakes", func(t *testing.T) {
		tip := &tooltipCtl{since: time.Now()}
		if _, ok := tip.next(); ok {
			t.Errorf("tooltip without hover scheduled a wakeup")
		}
	})
}
