// Package anim runs time-based animations: tweens advanced once per
// rendered frame. While tweens are active the app redraws continuously;
// the last frame after the final tween finishes leaves the widget in its
// end state.
package anim

import (
	"sync"
	"time"
)

// tween is one running animation.
type tween struct {
	start time.Time
	dur   time.Duration
	fn    func(t float64) // t is the eased progress in [0, 1]
	done  bool
}

var (
	mu     sync.Mutex
	active []*tween
)

// Start runs fn once per frame for dur with progress 0 through 1. When
// dur is zero or negative fn runs once with 1.
func Start(dur time.Duration, fn func(t float64)) {
	if fn == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if dur <= 0 {
		fn(1)
		return
	}
	active = append(active, &tween{start: time.Now(), dur: dur, fn: fn})
}

// Active reports whether any tween is still running; the app keeps
// redrawing while it is true.
func Active() bool {
	mu.Lock()
	defer mu.Unlock()
	for _, t := range active {
		if !t.done {
			return true
		}
	}
	return false
}

// Tick advances every running tween to now, calling fn with eased
// progress, and drops finished ones. Call it once per frame before
// painting.
func Tick(now time.Time) {
	mu.Lock()
	defer mu.Unlock()
	keep := active[:0]
	for _, t := range active {
		if t.done {
			continue
		}
		elapsed := now.Sub(t.start)
		p := float64(elapsed) / float64(t.dur)
		if p >= 1 {
			p = 1
			t.done = true
		}
		t.fn(ease(p))
		if !t.done {
			keep = append(keep, t)
		}
	}
	active = keep
}

// ease smooths linear progress with ease-out cubic; fast start, gentle
// landing.
func ease(p float64) float64 {
	return 1 - (1-p)*(1-p)*(1-p)
}

// Reset drops every tween; tests use it between cases.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	active = nil
}
