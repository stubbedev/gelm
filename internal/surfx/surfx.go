// Package surfx is the surface-animation coordinator: one state
// machine every transient surface (menus, tooltips, toasts, dropdown
// lists, dialogs, layer overlays) drives its enter and exit tweens
// through, instead of one ad-hoc implementation per surface kind.
//
// The core idea is that dismissal is a state, not a syscall. A
// surface's teardown splits in two:
//
//   - Dismiss flips the logical state: the surface's input region goes
//     empty (clicks "through" the dying surface land beneath, the
//     GTK/fuzzel behavior), the app's OnClosed callback fires exactly
//     once, and — for a compositor-dismissed popup — the seat grab has
//     already dropped with popup_done. The surface stays mapped.
//   - The exit tween paints the way out; only when it lands does the
//     driver's Destroy run, which is the real wayland teardown.
//
// Double dismissal is a no-op: Dismiss returns false on every call
// after the first, so another outside click mid-exit cannot re-enter
// the machine, re-seal input, or re-fire callbacks.
//
// wlroots popup lifetime rules shaped the ordering: a grabbed popup
// whose grab dropped while mapped is legal (that is exactly the
// outside-click case — the compositor ends the grab before sending
// popup_done, and the surface stays mapped for the exit tween), while
// the client cannot drop the grab itself without destroying the popup
// role object, which unmaps the surface. Destroying mid-grab is
// therefore the one thing Dismiss defers: the grab lives until Destroy
// at tween end — or dies on the first outside press, which the empty
// input region turns into a popup_done that Dismiss absorbs as a
// no-op. That ordering is what makes an exit tween on a grabbed popup
// wire-legal.
package surfx

import (
	"sync"
	"time"

	"github.com/stubbedev/gelm/anim"
)

// Kind selects a surface's animation profile. The zero value is the
// menu/popover profile, the default for xdg popups.
type Kind uint8

// Surface kinds. KindMenu is the zero value — the popover default.
// Menu and Dropdown share the popover profile; the distinctions exist
// so overrides can split them later.
const (
	KindMenu Kind = iota
	KindTooltip
	KindToast
	KindDropdown
	KindDialog
	KindOverlay
)

// Spec is one tween's shape: duration and curve.
type Spec struct {
	Duration time.Duration
	Easing   anim.Easing
}

// Style is a kind's enter and exit tweens.
type Style struct {
	Enter, Exit Spec
}

// Defaults are the per-kind profiles. Menus and dialogs slide+fade in
// with the fast-start ease-out-cubic landing; tooltips are quicker and
// quieter (a pure fade wants no fanfare); toasts keep #30's timings;
// overlays (launchers, panel reveals) are the slowest and grandest.
var Defaults = map[Kind]Style{
	KindMenu: {Enter: Spec{Duration: 130 * time.Millisecond}, Exit: Spec{Duration: 90 * time.Millisecond, Easing: anim.EaseInQuad}},
	KindTooltip: {
		Enter: Spec{Duration: 80 * time.Millisecond, Easing: anim.EaseOutQuad},
		Exit:  Spec{Duration: 60 * time.Millisecond, Easing: anim.EaseInQuad},
	},
	KindToast: {
		Enter: Spec{Duration: 180 * time.Millisecond},
		Exit:  Spec{Duration: 160 * time.Millisecond, Easing: anim.Linear},
	},
	KindDropdown: {
		Enter: Spec{Duration: 110 * time.Millisecond},
		Exit:  Spec{Duration: 70 * time.Millisecond, Easing: anim.EaseInQuad},
	},
	KindDialog: {
		Enter: Spec{Duration: 150 * time.Millisecond},
		Exit:  Spec{Duration: 120 * time.Millisecond, Easing: anim.EaseInQuad},
	},
	KindOverlay: {
		Enter: Spec{Duration: 200 * time.Millisecond},
		Exit:  Spec{Duration: 150 * time.Millisecond, Easing: anim.EaseInQuad},
	},
}

var (
	mu        sync.Mutex
	overrides = map[Kind]Style{}
	enabled   = true
)

// SetStyle overrides a kind's profile; apps tune curves and durations
// here instead of forking the state machine. Passing a zero Duration
// clears the override.
func SetStyle(k Kind, s Style) {
	mu.Lock()
	defer mu.Unlock()
	if s.Enter.Duration <= 0 && s.Exit.Duration <= 0 {
		delete(overrides, k)
		return
	}
	overrides[k] = s
}

// Lookup returns the effective profile for k: the override when one
// is set, else the default.
func Lookup(k Kind) Style {
	mu.Lock()
	defer mu.Unlock()
	if s, ok := overrides[k]; ok {
		return s
	}
	return Defaults[k]
}

// SetEnabled turns surface animations on or off — the API toggle beside
// GELM_NO_ANIM. It returns the restore function. Disabled animations
// collapse the durations to zero; the state machine still runs, so
// dismissal, sealing, callbacks, and destroy all behave identically.
func SetEnabled(on bool) (restore func()) {
	mu.Lock()
	old := enabled
	enabled = on
	mu.Unlock()
	return func() {
		mu.Lock()
		enabled = old
		mu.Unlock()
	}
}

// Enabled reports whether surface animations are on.
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return enabled
}

// Plan resolves a kind's effective tween profile. Motion off — the
// package toggle, GELM_NO_ANIM, or a caller's own reduced-motion
// source such as the theme — collapses both durations to zero and is
// why callers pass their theme state here instead of consulting it
// twice. The zero-duration tweens deliver their end state once at
// launch, so the state machine's shape is identical either way.
func Plan(k Kind, motion bool) Style {
	if !motion || !Enabled() || anim.Instant() {
		s := Lookup(k)
		s.Enter.Duration, s.Exit.Duration = 0, 0
		return s
	}
	return Lookup(k)
}

// Driver is the surface-side half the coordinator drives. The surface
// wrapper (popup.Popup, a hostWindow) implements it; the coordinator
// never touches wire objects itself.
type Driver interface {
	// ApplyVisual applies the reveal fraction, 0 hidden through 1 at
	// rest: opacity through a Fader, the slide offset from the anchor,
	// whatever the surface's kind animates.
	ApplyVisual(reveal float64)
	// MarkFrame schedules a repaint of the current visual and wakes the
	// surface's loop if it is parked. Called once per tween frame.
	MarkFrame()
	// SealInput kills the surface's input: an empty input region,
	// committed, so clicks during the exit land beneath. Called once,
	// at Dismiss, before the first exit frame.
	SealInput()
	// Destroy is the real wayland teardown. Called exactly once, when
	// the exit tween lands — or immediately by Teardown.
	Destroy()
}

// phase is the coordinator's position in the enter/exit arc.
type phase uint8

const (
	phaseIdle phase = iota // before Enter
	phaseOpen              // at rest (Enter done or never wanted)
	phaseExit              // tween in flight
	phaseGone              // destroyed
)

// Coordinator is the dismissal state machine for one surface. Embed or
// hold it; the surface keeps its own wire objects behind the Driver.
// All methods are safe from any goroutine: tween callbacks land on
// whichever loop ticked the animation clock, while Dismiss usually
// comes from an input path.
type Coordinator struct {
	kind   Kind
	drv    Driver
	motion func() bool // nil: package toggle only

	mp     sync.Mutex
	ph     phase
	reveal float64

	// destroyed guards the exactly-once Destroy (and, with fire()'s
	// nil-ing of onDismissed, the exactly-once callback). Touched only
	// under mp.
	destroyed bool

	cancelEnter anim.Cancel
	cancelExit  anim.Cancel
	onDismissed func()
}

// NewCoordinator builds the machine for one surface of kind k. motion,
// when non-nil, is consulted at every launch so a live theme change
// applies to the next tween; callers typically pass their theme's
// animation flag there.
func NewCoordinator(k Kind, drv Driver, motion func() bool) *Coordinator {
	return &Coordinator{kind: k, drv: drv, motion: motion}
}

// SetOnDismissed registers the exactly-once dismissal callback. It
// fires inside the first Dismiss (the logical close), and from
// Teardown when teardown beats any dismissal — a surface interrupted
// mid-flight still tells its app it went away, once.
func (c *Coordinator) SetOnDismissed(f func()) {
	c.mp.Lock()
	c.onDismissed = f
	c.mp.Unlock()
}

// Dismissed reports whether the logical close happened.
func (c *Coordinator) Dismissed() bool {
	c.mp.Lock()
	defer c.mp.Unlock()
	return c.ph >= phaseExit
}

// Destroyed reports whether the wire teardown happened.
func (c *Coordinator) Destroyed() bool {
	c.mp.Lock()
	defer c.mp.Unlock()
	return c.destroyed
}

// Reveal returns the current visual fraction for a paint pass.
func (c *Coordinator) Reveal() float64 {
	c.mp.Lock()
	defer c.mp.Unlock()
	return c.reveal
}

// motionOn resolves the motion decision for a launch.
func (c *Coordinator) motionOn() bool {
	return c.motion == nil || c.motion()
}

// Enter starts the enter tween: the first frame sits at progress 0
// (applied synchronously, so a surface's very first paint is hidden or
// offset — never a flash of the final frame), the tween then raises
// the reveal to rest. Idempotent; a surface that skips Enter opens at
// rest.
//
// Locking note: the launch happens OUTSIDE c.mp. A zero-duration tween
// (reduced motion) fires its callback synchronously inside Play, and
// that callback takes the lock — launching under the lock would
// self-deadlock. The phase flip stays inside so concurrent Enters stay
// single-shot.
func (c *Coordinator) Enter() {
	c.mp.Lock()
	if c.destroyed || c.ph != phaseIdle {
		c.mp.Unlock()
		return
	}
	c.ph = phaseOpen
	st := Plan(c.kind, c.motionOn())
	c.mp.Unlock()
	// The first frame applies synchronously at progress 0, whatever the
	// duration, so the driver's very first paint is the hidden/offset
	// one — never a flash of the finished surface. (Reduced motion then
	// lands 1 inside play, on the same call stack.)
	c.drv.ApplyVisual(0)
	cancel := c.play(st.Enter, true, 0)
	c.mp.Lock()
	if c.ph == phaseOpen && !c.destroyed {
		c.cancelEnter = cancel
	}
	c.mp.Unlock()
}

// Dismiss flips the logical state and starts the exit tween from the
// current reveal. Returns false when the surface is already dismissed
// or gone — the double-dismiss no-op the outside-click path relies on.
// Like Enter, the launch runs outside the lock (see there).
func (c *Coordinator) Dismiss() bool {
	c.mp.Lock()
	if c.destroyed || c.ph >= phaseExit {
		c.mp.Unlock()
		return false
	}
	c.ph = phaseExit
	start := c.reveal
	if c.cancelEnter != nil {
		c.cancelEnter()
		c.cancelEnter = nil
	}
	st := Plan(c.kind, c.motionOn())
	c.mp.Unlock()
	// Input dies before the first exit frame: the seal is the whole
	// click-through guarantee, and for a still-held client-side grab it
	// also turns the next outside press into a popup_done that Dismiss
	// absorbs as its no-op second call.
	c.drv.SealInput()
	cancel := c.play(st.Exit, false, start)
	c.mp.Lock()
	if !c.destroyed {
		c.cancelExit = cancel
	}
	c.mp.Unlock()
	c.fire()
	return true
}

// play launches one tween. Rising tweens run reveal from 0 to 1; the
// exit runs from start down to 0. Zero durations (reduced motion) land
// the end state — and for the exit that means Destroy — synchronously
// inside this call.
func (c *Coordinator) play(spec Spec, rising bool, start float64) anim.Cancel {
	fn := func(p float64) {
		c.mp.Lock()
		if c.destroyed {
			// Teardown won the race against this callback: the driver is
			// gone and must not be touched again.
			c.mp.Unlock()
			return
		}
		var reveal float64
		if rising {
			reveal = p
		} else {
			reveal = start * (1 - p)
		}
		c.reveal = reveal
		c.mp.Unlock()
		c.drv.ApplyVisual(reveal)
		c.drv.MarkFrame()
		if !rising && p >= 1 {
			c.finishExit()
		}
	}
	tween := anim.Animate(spec.Duration, fn)
	if spec.Easing != nil {
		tween.Easing(spec.Easing)
	}
	return anim.Play(tween)
}

// finishExit runs when the exit tween landed: the last frame has been
// marked for paint, so the wire teardown follows. Exactly once.
func (c *Coordinator) finishExit() {
	c.mp.Lock()
	if c.destroyed {
		c.mp.Unlock()
		return
	}
	c.destroyed = true
	c.ph = phaseGone
	c.cancelExit = nil
	drv := c.drv
	c.mp.Unlock()
	drv.Destroy()
}

// Teardown cancels any tween, fires the dismissal callback if it never
// fired, and destroys the surface if it is not destroyed. This is the
// error-path and surface-teardown entry (the old Close): no leak, no
// zombie, exactly-once everywhere — and idempotent.
func (c *Coordinator) Teardown() {
	c.mp.Lock()
	first := !c.destroyed
	c.destroyed = true
	c.ph = phaseGone
	if c.cancelEnter != nil {
		c.cancelEnter()
		c.cancelEnter = nil
	}
	if c.cancelExit != nil {
		c.cancelExit()
		c.cancelExit = nil
	}
	drv := c.drv
	c.mp.Unlock()
	c.fire()
	if first {
		drv.Destroy()
	}
}

// fire runs OnDismissed at most once per coordinator.
func (c *Coordinator) fire() {
	c.mp.Lock()
	f := c.onDismissed
	c.onDismissed = nil
	c.mp.Unlock()
	if f != nil {
		f()
	}
}
