// The surfx coordinator: the dismissal state machine's contract.
// Dismiss flips logical state exactly once and seals input before the
// first exit frame; exit frames repaint until the tween lands and only
// then destroys; double dismissal and teardown interruptions cannot
// double-fire or double-destroy; the enter's first frame sits at
// progress 0; per-kind defaults differ and every instant path
// (reduced motion) runs the same machine with zero durations.
package surfx

import (
	"fmt"
	"testing"
	"time"

	"github.com/stubbedev/gelm/anim"
	"github.com/stubbedev/gelm/internal/animclock"
)

// recDriver is a fake Driver recording the call order, so tests can
// assert that sealing precedes the first exit frame and that the
// destroy follows the last.
type recDriver struct {
	events    []string
	reveal    float64
	frames    int
	sealed    bool
	destroyed int
}

func (d *recDriver) ApplyVisual(reveal float64) {
	d.reveal = reveal
	d.frames++
	d.events = append(d.events, fmt.Sprintf("frame %.3f", reveal))
}

func (d *recDriver) MarkFrame() {}

func (d *recDriver) SealInput() {
	d.sealed = true
	d.events = append(d.events, "seal")
}

func (d *recDriver) Destroy() {
	d.destroyed++
	d.events = append(d.events, "destroy")
}

// last, without the trailing element.
func last[T any](s []T) T { return s[len(s)-1] }

func TestDismissFiresOnceAndDestroysAfterLastFrame(t *testing.T) {
	restore := animclock.SetClock(func() time.Time { return time.Unix(1750000000, 0) })
	defer restore()
	animclock.Reset()

	d := &recDriver{}
	fx := NewCoordinator(KindMenu, d, func() bool { return true })
	closed := 0
	fx.SetOnDismissed(func() { closed++ })

	fx.Enter()
	if d.frames != 1 || d.reveal != 0 {
		t.Fatalf("Enter's first frame: reveal = %v frames = %d, want one frame at 0", d.reveal, d.frames)
	}

	if !fx.Dismiss() {
		t.Fatal("first Dismiss reported already dismissed")
	}
	if !d.sealed {
		t.Fatal("Dismiss did not seal input")
	}
	// Seal strictly before the first exit frame.
	if d.events[0] != "frame 0.000" || d.events[1] != "seal" {
		t.Fatalf("event order before exit frames: %v", d.events)
	}

	// A second dismissal mid-exit — another outside click — is a no-op.
	if fx.Dismiss() {
		t.Error("second Dismiss re-entered the machine")
	}

	// Drive the exit: every tick repaints, the destroy comes only
	// after the last frame, and nothing fires twice.
	framesBefore := d.frames
	now := time.Unix(1750000000, 0)
	for steps := 0; ; steps++ {
		if _, ok := animclock.Next(); !ok {
			break
		}
		steps++
		if steps > 1000 {
			t.Fatal("exit tween never landed")
		}
		now = now.Add(animclock.FrameInterval)
		animclock.Tick(now)
	}
	if d.frames == framesBefore {
		t.Error("exit tween painted no frames")
	}
	if last(d.events) != "destroy" {
		t.Errorf("final event = %q, want the destroy after the last frame", last(d.events))
	}
	if d.destroyed != 1 {
		t.Errorf("destroyed %d times, want exactly 1", d.destroyed)
	}
	if closed != 1 {
		t.Errorf("OnClosed fired %d times, want exactly 1", closed)
	}
	if !fx.Dismissed() || !fx.Destroyed() {
		t.Error("Dismissed/Destroyed flags not set after the exit landed")
	}
}

func TestTeardownInterruptsTweenWithoutLeaks(t *testing.T) {
	restore := animclock.SetClock(func() time.Time { return time.Unix(1750000000, 0) })
	defer restore()
	animclock.Reset()

	d := &recDriver{}
	fx := NewCoordinator(KindMenu, d, func() bool { return true })
	closed := 0
	fx.SetOnDismissed(func() { closed++ })

	fx.Enter()
	fx.Dismiss()
	// Teardown mid-exit: cancel the tween, fire once, destroy once.
	fx.Teardown()
	if closed != 1 {
		t.Errorf("OnClosed fired %d times after teardown, want exactly 1", closed)
	}
	if d.destroyed != 1 {
		t.Errorf("destroyed %d times after teardown, want exactly 1", d.destroyed)
	}
	// A tick after teardown runs nothing: the surface is gone.
	if _, ok := animclock.Next(); ok {
		t.Error("canceled tween still holds a wake deadline")
	}
	// Teardown is idempotent.
	fx.Teardown()
	if d.destroyed != 1 {
		t.Errorf("second teardown destroyed again (%d)", d.destroyed)
	}
}

func TestTeardownWithoutDismissFiresOnce(t *testing.T) {
	animclock.Reset()
	d := &recDriver{}
	fx := NewCoordinator(KindMenu, d, nil)
	closed := 0
	fx.SetOnDismissed(func() { closed++ })
	// The error path: never opened, never dismissed, surface dies.
	fx.Teardown()
	if closed != 1 || d.destroyed != 1 {
		t.Errorf("teardown: closed = %d destroyed = %d, want 1/1", closed, d.destroyed)
	}
}

func TestEnterFirstFrameAtZeroAndRestAtCompletion(t *testing.T) {
	restore := animclock.SetClock(func() time.Time { return time.Unix(1750000000, 0) })
	defer restore()
	animclock.Reset()

	d := &recDriver{}
	fx := NewCoordinator(KindMenu, d, func() bool { return true })
	fx.Enter()
	// The first frame is at progress 0 — hidden or offset — applied
	// synchronously, so the surface's first paint can never flash the
	// finished widget.
	if d.reveal != 0 {
		t.Fatalf("first frame reveal = %v, want 0", d.reveal)
	}
	// Enter is idempotent.
	fx.Enter()
	if d.frames != 1 {
		t.Fatalf("second Enter replayed a frame (frames = %d)", d.frames)
	}
	// Drive to rest.
	now := time.Unix(1750000000, 0)
	for steps := 0; ; steps++ {
		if _, ok := animclock.Next(); !ok {
			break
		}
		now = now.Add(animclock.FrameInterval)
		animclock.Tick(now)
		if steps > 1000 {
			t.Fatal("enter tween never landed")
		}
	}
	if d.reveal != 1 {
		t.Errorf("completion reveal = %v, want 1 (rest state)", d.reveal)
	}
	if d.destroyed != 0 {
		t.Error("a completed enter destroyed the surface")
	}
}

func TestInstantPathsRunTheSameMachine(t *testing.T) {
	type instantCase struct {
		name   string
		motion func() bool
		setup  func(t *testing.T)
	}
	for _, tc := range []instantCase{
		{
			name:   "GELM_NO_ANIM equivalent (anim.SetInstant)",
			motion: func() bool { return true },
			setup:  func(t *testing.T) { t.Cleanup(anim.SetInstant(true)) },
		},
		{
			name:   "package toggle (SetEnabled(false))",
			motion: func() bool { return true },
			setup:  func(t *testing.T) { t.Cleanup(SetEnabled(false)) },
		},
		{
			name:   "theme motion off (motion hook)",
			motion: func() bool { return false },
			setup:  func(t *testing.T) {},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			animclock.Reset()
			tc.setup(t)

			d := &recDriver{}
			fx := NewCoordinator(KindMenu, d, tc.motion)
			closed := 0
			fx.SetOnDismissed(func() { closed++ })

			fx.Enter()
			if d.reveal != 1 {
				t.Fatalf("instant enter reveal = %v, want 1 at once", d.reveal)
			}
			fx.Dismiss()
			if closed != 1 {
				t.Errorf("instant path fired OnClosed %d times, want exactly 1", closed)
			}
			if d.destroyed != 1 {
				t.Errorf("instant path destroyed %d times, want exactly 1", d.destroyed)
			}
			if !d.sealed {
				t.Error("instant path skipped the input seal")
			}
			// The instant surface holds no wake deadlines: the loop
			// parks immediately after.
			if _, ok := animclock.Next(); ok {
				t.Error("instant path left a tween scheduled")
			}
		})
	}
}

func TestPerKindDefaults(t *testing.T) {
	menu := Lookup(KindMenu)
	tooltip := Lookup(KindTooltip)
	if tooltip.Enter.Duration <= 0 {
		t.Fatal("tooltip enter duration missing")
	}
	if tooltip.Enter.Duration >= menu.Enter.Duration {
		t.Errorf("tooltip enter %v is not shorter than menu enter %v", tooltip.Enter.Duration, menu.Enter.Duration)
	}
	if menu.Exit.Duration <= 0 || menu.Enter.Duration <= 0 {
		t.Fatal("menu durations missing")
	}
	// Plan folds the motion decision: off collapses durations only —
	// curves and the machine's shape stay.
	fast := Plan(KindTooltip, false)
	if fast.Enter.Duration != 0 || fast.Exit.Duration != 0 {
		t.Errorf("motion-off plan = %+v, want zero durations", fast)
	}
	full := Plan(KindTooltip, true)
	if full.Enter.Duration != tooltip.Enter.Duration {
		t.Errorf("motion-on plan = %+v, want the tooltip defaults", full)
	}
}

func TestSetStyleOverridesAndClears(t *testing.T) {
	defer SetStyle(KindDialog, Style{})
	SetStyle(KindDialog, Style{Enter: Spec{Duration: 5 * time.Millisecond, Easing: anim.Linear}, Exit: Spec{Duration: 5 * time.Millisecond}})
	got := Lookup(KindDialog)
	if got.Enter.Duration != 5*time.Millisecond || got.Enter.Easing == nil {
		t.Errorf("override not applied: %+v", got)
	}
	// Zero durations clear the override, restoring the defaults.
	SetStyle(KindDialog, Style{})
	want := Defaults[KindDialog]
	got = Lookup(KindDialog)
	if got.Enter.Duration != want.Enter.Duration || got.Exit.Duration != want.Exit.Duration || got.Enter.Easing != nil {
		t.Errorf("cleared override = %+v, want the default durations back", got)
	}
}
