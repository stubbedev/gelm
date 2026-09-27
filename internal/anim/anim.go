// Package anim runs time-based animations: tweens advanced once per
// rendered frame, combinable into timelines (Sequence, Parallel,
// Delay) and cancelable mid-flight. The animation clock is
// timer-driven: while tweens run, Next hands the app loop the next
// frame deadline to wake on — frame-callback paced when the compositor
// answers, and the fallback pacer when it cannot (an occluded surface
// stops scheduling callbacks). With nothing running the loop parks
// until the next event and the clock costs nothing.
//
// anim is internal: gelm exposes no public animation API yet (widget
// APIs take no durations today), so curves and timelines stay a
// module-private implementation detail behind widget-level defaults.
package anim

import (
	"os"
	"sync"
	"time"
)

// FrameInterval is the animation clock's frame period: the fallback
// pacing for ticks when the compositor's frame callbacks do not arrive
// (occluded surface), and the deadline granularity of Next while a
// tween is mid-flight.
const FrameInterval = time.Second / 60

// instant collapses every tween to its end state: launches deliver
// fn(1) immediately and hold no schedule. It is the reduced-motion
// escape hatch — GELM_NO_ANIM=1 in the environment, or SetInstant from
// code. Delays keep their durations (a toast still waits out its
// timeout), so behavior keeps its timing skeleton and only the motion
// goes away. The state machines driving the tweens run unchanged —
// the same code path, just instant.
var instant = os.Getenv("GELM_NO_ANIM") == "1"

// Instant reports whether tweens currently collapse to their end state.
func Instant() bool { return instant }

// SetInstant turns the reduced-motion collapse on or off and returns
// the restore function. Already-running tweens keep their schedules;
// only launches after the call are affected.
func SetInstant(on bool) (restore func()) {
	mu.Lock()
	old := instant
	instant = on
	mu.Unlock()
	return func() {
		mu.Lock()
		instant = old
		mu.Unlock()
	}
}

// clock is the time source for launches and deadlines; tests pin it to
// make tick schedules exact. Tick takes its instant explicitly, so a
// pinned clock pins the whole schedule.
var clock = time.Now

// tstep is one flattened, absolutely-timed piece of a launched
// timeline: the half-open window [start, end).
type tstep struct {
	start  time.Time
	end    time.Time
	fn     func(t float64) // eased progress in [0, 1]; springs may overshoot
	easing Easing
	grp    *group
	done   bool
}

// group is one launched timeline's cancelation token: Cancel marks it
// dead and every step it owns stops firing on the next tick.
type group struct {
	dead bool
}

var (
	mu       sync.Mutex
	active   []*tstep
	lastTick time.Time
)

// Start runs fn once per frame for dur with eased progress 0 through 1
// (the EaseOutCubic default; Animate picks other curves). When dur is
// zero or negative fn runs once with 1. The returned Cancel stops the
// tween mid-flight; dropping it just lets the tween run out.
func Start(dur time.Duration, fn func(t float64)) Cancel {
	return Play(Animate(dur, fn))
}

// Cancel stops one launched timeline: every piece that has not
// finished is dropped and never fires again. The widget state the
// timeline was driving keeps whatever value the last callback left -
// canceling moves no pixels, so callers that change widget state
// mid-flight (a slider grab against a running tween) invalidate the
// widget themselves. Safe to call any number of times, and a no-op
// once the timeline finished on its own.
type Cancel func()

// Step is one timed piece of an animation timeline: a tween (Animate),
// a hold (Delay), or a composition (Sequence, Parallel). Timelines
// launch with Play.
type Step interface {
	// expand flattens the step into timed pieces beginning at offset
	// off from the timeline's start, appending to out and returning
	// the offset the step ends at. Unexported on purpose: steps are
	// built by this package's constructors.
	expand(off time.Duration, out *[]titem) time.Duration
}

// titem is a step's contribution before launch pins absolute times.
type titem struct {
	at     time.Duration
	dur    time.Duration
	fn     func(t float64)
	easing Easing
}

// Tween animates one value: fn runs once per frame with eased progress
// over dur. Build one with Animate, curve it with Easing.
type Tween struct {
	dur    time.Duration
	fn     func(t float64)
	easing Easing
}

// Animate declares a tween over dur; when dur is zero or negative fn
// runs once with 1 at launch. The curve defaults to EaseOutCubic.
func Animate(dur time.Duration, fn func(t float64)) *Tween {
	return &Tween{dur: dur, fn: fn, easing: EaseOutCubic}
}

// Easing replaces the tween's curve and returns the tween for
// chaining.
func (t *Tween) Easing(e Easing) *Tween {
	t.easing = e
	return t
}

// expand implements Step.
func (t *Tween) expand(off time.Duration, out *[]titem) time.Duration {
	*out = append(*out, titem{at: off, dur: t.dur, fn: t.fn, easing: t.easing})
	return off + t.dur
}

// delay is a hold between steps.
type delay time.Duration

// Delay holds the timeline for d before the next step runs.
func Delay(d time.Duration) Step { return delay(d) }

// expand implements Step.
func (d delay) expand(off time.Duration, _ *[]titem) time.Duration {
	return off + time.Duration(d)
}

// sequence runs steps back to back.
type sequence []Step

// Sequence runs steps one after another: each begins when the previous
// ends. It is the reveal-then-settle, show-toast-then-fade shape.
func Sequence(steps ...Step) Step { return sequence(steps) }

// expand implements Step.
func (s sequence) expand(off time.Duration, out *[]titem) time.Duration {
	for _, st := range s {
		if st == nil {
			continue
		}
		off = st.expand(off, out)
	}
	return off
}

// parallel runs steps at the same time.
type parallel []Step

// Parallel runs steps together; the step ends when the last of them
// ends.
func Parallel(steps ...Step) Step { return parallel(steps) }

// expand implements Step.
func (p parallel) expand(off time.Duration, out *[]titem) time.Duration {
	end := off
	for _, st := range p {
		if st == nil {
			continue
		}
		if e := st.expand(off, out); e > end {
			end = e
		}
	}
	return end
}

// Play launches steps as one timeline — several top-level steps run in
// sequence — and returns its Cancel. Every piece is scheduled up front
// from the same instant, so a timeline's shape is fixed at launch and
// ticks land on exact offsets regardless of when frames happen to
// come.
func Play(steps ...Step) Cancel {
	g := &group{}
	cancel := Cancel(func() {
		mu.Lock()
		g.dead = true
		mu.Unlock()
	})
	var items []titem
	var off time.Duration
	for _, s := range steps {
		if s == nil {
			continue
		}
		off = s.expand(off, &items)
	}
	now := clock()
	launch := make([]*tstep, 0, len(items))
	for _, it := range items {
		if it.fn == nil {
			continue
		}
		if instant {
			it.dur = 0 // reduced motion: the tween lands at launch
		}
		if it.dur <= 0 {
			it.fn(1) // a zero-duration tween delivers its end state once
			continue
		}
		easing := it.easing
		if easing == nil {
			easing = EaseOutCubic
		}
		start := now.Add(it.at)
		launch = append(launch, &tstep{
			start:  start,
			end:    start.Add(it.dur),
			fn:     it.fn,
			easing: easing,
			grp:    g,
		})
	}
	if len(launch) > 0 {
		mu.Lock()
		active = append(active, launch...)
		mu.Unlock()
	}
	return cancel
}

// Active reports whether any tween is still running or scheduled; the
// app keeps its animation clock ticking while it is true.
func Active() bool {
	mu.Lock()
	defer mu.Unlock()
	for _, t := range active {
		if !t.done && !t.grp.dead {
			return true
		}
	}
	return false
}

// Next returns the earliest time the loop must wake to keep the
// animation clock running: one frame deadline past the last tick while
// a tween is mid-flight (the compositor's frame callbacks usually pace
// sooner), or a later step's start while a timeline holds in a delay.
// False means nothing runs and the loop may park until the next event.
func Next() (time.Time, bool) {
	mu.Lock()
	defer mu.Unlock()
	now := clock()
	if lastTick.After(now) {
		// The animation clock has ticked past the wall clock (tests pin
		// the clock and drive Tick with explicit instants); it is the
		// reference the deadlines hang off.
		now = lastTick
	}
	if lastTick.IsZero() {
		lastTick = now
	}
	nextFrame := lastTick.Add(FrameInterval)
	if nextFrame.Before(now) {
		nextFrame = now
	}
	var next time.Time
	found := false
	consider := func(t time.Time) {
		if !found || t.Before(next) {
			next, found = t, true
		}
	}
	for _, t := range active {
		if t.done || t.grp.dead {
			continue
		}
		if now.Before(t.start) {
			consider(t.start)
			continue
		}
		end := t.end
		if nextFrame.Before(end) {
			end = nextFrame
		}
		consider(end)
	}
	return next, found
}

// Tick advances every running tween to now and drops finished or
// canceled ones. Callbacks run after the pass settles, with the lock
// released, so a callback may Start, Play, or Cancel freely — a
// landing tick that tears down or relaunches is the intended shape.
// A callback that cancels its own group still stops that group's
// later steps in the same tick. It reports whether any callback ran,
// so the caller knows a frame is owed. Call it once per loop pass
// before painting.
func Tick(now time.Time) bool {
	type firing struct {
		fn    func(float64)
		eased float64
		grp   *group
	}
	mu.Lock()
	var due []firing
	keep := active[:0]
	for _, t := range active {
		if t.done || t.grp.dead {
			continue
		}
		if now.Before(t.start) {
			keep = append(keep, t) // a sequence's later step, still waiting
			continue
		}
		p := float64(now.Sub(t.start)) / float64(t.end.Sub(t.start))
		if p >= 1 {
			p = 1
			t.done = true
		}
		due = append(due, firing{fn: t.fn, eased: t.easing(p), grp: t.grp})
		if !t.done {
			keep = append(keep, t)
		}
	}
	active = keep
	lastTick = now
	mu.Unlock()
	ran := false
	for _, f := range due {
		mu.Lock()
		dead := f.grp.dead
		mu.Unlock()
		if dead {
			continue
		}
		f.fn(f.eased)
		ran = true
	}
	return ran
}

// SetClock replaces the time source launches and deadlines read and
// returns the restore function. Passing nil restores time.Now. Tests
// pin it — usually to a mutable variable they advance by hand, so a
// launch between ticks still lands on the current test instant and
// tick schedules stay exact (see pinClock in this package's tests and
// the widget/app suites that drive the same clock from outside).
func SetClock(fn func() time.Time) (restore func()) {
	mu.Lock()
	old := clock
	if fn == nil {
		fn = time.Now
	}
	clock = fn
	mu.Unlock()
	return func() {
		mu.Lock()
		clock = old
		mu.Unlock()
	}
}

// Reset drops every timeline; tests use it between cases.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	active = nil
	lastTick = clock()
}
