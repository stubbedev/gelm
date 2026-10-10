package widget

// The CSS animation runner: node-side tweens that play the computed
// animations against the style cache, the same channels the
// transitions write. A restyle syncs the runners with the computed
// animation list - starts, pauses, resumes, retargets, stops - and
// each tick interpolates the keyframes at the current phase into the
// cached values and damages the widget. When an animation stops
// without a fill, the cascade's own values are back in the cache, the
// revert CSS asks for; fill-mode forwards/both holds the final frame
// until the rule changes.

import (
	"math"
	"time"

	"github.com/stubbedev/gelm/anim"
	"github.com/stubbedev/gelm/internal/style"
)

// animRunner is one computed animation's live state on a node.
type animRunner struct {
	// a is the computed animation this runner plays.
	a style.Animation
	// phase is the eased position within the current cycle.
	phase float64
	// cycle is the completed cycle count.
	cycle int
	// cancel stops the tween; nil when not playing.
	cancel anim.Cancel
	// cascade is the style cache at animation start, the revert
	// target; hadMask records which of the rule's channels the cascade
	// declared.
	cascade style.Values
	hadMask uint32
	// holding is a finished animation holding its fill: no tween, the
	// final frame stays in the cache until the rule changes.
	holding bool
}

// syncAnimation aligns the runners with the node's computed animation
// list, matched by name: an animation the list still carries keeps
// its runner and its mid-flight state, a new one starts, a dropped
// one reverts.
func (n *node) syncAnimation(v style.Values) {
	if anim.Instant() {
		n.stopAnimations()
		return
	}
	var keep []animRunner
	for i := range v.Animation {
		a := v.Animation[i]
		if !a.Active() {
			continue
		}
		r := n.findRunner(a.Name)
		if r == nil {
			r = &animRunner{a: a, cascade: n.cs, hadMask: a.Keyframes.Mask & n.cascadeMask()}
			// A negative delay starts mid-flight whatever the fill says;
			// a backwards fill shows the first frame through a positive
			// one.
			if a.Delay < 0 || a.Fill == style.FillBackwards || a.Fill == style.FillBoth {
				r.phase = r.fillPhase()
				n.applyFrame(&a, r.phase)
			}
		} else {
			r.a = a
		}
		keep = append(keep, *r)
	}
	// Runners whose animation vanished stop; the recompute that dropped
	// the animation already restored the cascade's channels, so no
	// revert here - it would re-apply a stale snapshot.
	for i := range n.animRuns {
		r := &n.animRuns[i]
		if r.cancel != nil {
			r.cancel()
			r.cancel = nil
		}
	}
	n.animRuns = keep
	for i := range n.animRuns {
		n.syncRunner(&n.animRuns[i])
	}
}

// findRunner returns the runner playing name, nil when none.
func (n *node) findRunner(name string) *animRunner {
	for i := range n.animRuns {
		if n.animRuns[i].a.Name == name {
			return &n.animRuns[i]
		}
	}
	return nil
}

// cascadeMask collects the channels this node's cascade declared, the
// revert's reach.
func (n *node) cascadeMask() uint32 {
	var m uint32
	for _, ch := range []style.Chan{
		style.ChOpacity, style.ChColor, style.ChBackground,
		style.ChBrightness, style.ChTransform, style.ChIconXform,
	} {
		if n.cs.Has(channelProp(ch)) {
			m |= uint32(ch)
		}
	}
	return m
}

// channelProp maps a channel to the longhand whose presence guards it.
func channelProp(ch style.Chan) style.Prop {
	switch ch {
	case style.ChOpacity:
		return style.PropOpacity
	case style.ChColor:
		return style.PropColor
	case style.ChBackground:
		return style.PropBackgroundColor
	case style.ChBrightness:
		return style.PropFilter
	case style.ChTransform:
		return style.PropTransform
	case style.ChIconXform:
		return style.PropIconTransform
	}
	return style.PropOpacity
}

// syncRunner starts, resumes, pauses, or retargets one runner - and
// re-asserts the current frame after a recompute, which restored the
// bare cascade into the cache over the animation's channels.
func (n *node) syncRunner(r *animRunner) {
	if !r.a.Running {
		if r.cancel != nil {
			r.cancel()
			r.cancel = nil
		}
		n.applyFrame(&r.a, r.phase)
		return
	}
	if r.holding {
		n.applyFrame(&r.a, endPhase(&r.a, r.cycle))
		return
	}
	if r.cancel != nil {
		n.applyFrame(&r.a, r.phase)
		return
	}
	n.playAnimation(r)
}

// stopAnimations cancels every runner and clears the state; the
// caller has just recomputed the style cache from the cascade, so the
// revert is the recompute.
func (n *node) stopAnimations() {
	for i := range n.animRuns {
		if n.animRuns[i].cancel != nil {
			n.animRuns[i].cancel()
		}
	}
	if len(n.animRuns) > 0 {
		n.animRuns = nil
		n.Invalidate()
	}
}

// fillPhase is the phase a fresh animation shows before its tween
// runs: mid-flight for a negative delay, the from frame for a
// positive one under a backwards fill.
func (r *animRunner) fillPhase() float64 {
	raw := 0.0
	if r.a.Delay < 0 {
		raw = -r.a.Delay / r.a.Duration
	}
	switch r.a.Direction {
	case style.DirReverse, style.DirAlternateReverse:
		return 1 - raw
	}
	return raw
}

// playAnimation launches the tween for one cycle; the landing tick
// relaunches for the next while iterations remain (anim callbacks may
// launch - Tick runs them with its lock released). The tween's curve
// maps raw progress over the whole timeline through the active
// phase's CSS curve, so the callback's t is the eased phase - held at
// 0 through a positive delay, already mid-flight for a negative one.
func (n *node) playAnimation(r *animRunner) {
	a := r.a
	dur := time.Duration(a.Duration * float64(time.Second))
	total := time.Duration((a.Duration + math.Abs(a.Delay)) * float64(time.Second))
	delay := time.Duration(a.Delay * float64(time.Second))
	cycle := r.cycle
	ease := cssTiming(a.Timing)
	r.cancel = anim.Play(anim.Animate(total, func(t float64) {
		if t <= 0 {
			// Inside a positive delay: the backwards fill shows the
			// first frame, otherwise the cascade holds.
			if a.Fill == style.FillBackwards || a.Fill == style.FillBoth {
				n.applyFrame(&a, r.fillPhase())
			}
			return
		}
		if t >= 1 {
			cycle++
			iter := a.Iteration
			if iter <= 0 {
				iter = 1
			}
			if !a.Infinite && float64(cycle) >= iter {
				r.cancel = nil
				r.phase = 0
				r.cycle = 0
				if a.Fill == style.FillForwards || a.Fill == style.FillBoth {
					r.holding = true
					n.applyFrame(&a, endPhase(&a, cycle-1))
					return
				}
				n.revert(r)
				return
			}
			r.phase = 0
			r.cycle = cycle
			n.playAnimation(r)
			return
		}
		r.cycle = cycle
		r.phase = cyclePhase(&a, cycle, t)
		n.applyFrame(&a, r.phase)
	}).Easing(func(raw float64) float64 {
		if dur <= 0 {
			return 1
		}
		p := (raw*float64(total) - float64(delay)) / float64(dur)
		return ease(math.Min(math.Max(p, 0), 1))
	}))
}

// cyclePhase maps raw progress through the cycle's direction.
func cyclePhase(a *style.Animation, cycle int, p float64) float64 {
	switch a.Direction {
	case style.DirReverse:
		return 1 - p
	case style.DirAlternate:
		if cycle%2 == 1 {
			return 1 - p
		}
		return p
	case style.DirAlternateReverse:
		if cycle%2 == 1 {
			return p
		}
		return 1 - p
	}
	return p
}

// endPhase is the phase an animation's last cycle ends at: 1 unless
// the direction ends it backwards.
func endPhase(a *style.Animation, lastCycle int) float64 {
	switch a.Direction {
	case style.DirReverse:
		return 0
	case style.DirAlternate:
		if (lastCycle-1)%2 == 1 {
			return 0
		}
		return 1
	case style.DirAlternateReverse:
		if (lastCycle-1)%2 == 1 {
			return 1
		}
		return 0
	}
	return 1
}

// applyFrame writes the keyframes' interpolated channels at a phase
// into the style cache. Channels the keyframes do not animate hold
// their cascade values untouched.
func (n *node) applyFrame(a *style.Animation, phase float64) {
	av := a.Keyframes.At(phase)
	mask := a.Keyframes.Mask
	if mask&uint32(style.ChOpacity) != 0 {
		n.cs.Opacity = av.Opacity
		n.cs.Set |= 1 << style.PropOpacity
	}
	if mask&uint32(style.ChColor) != 0 {
		n.cs.Color = av.Color
		n.cs.Set |= 1 << style.PropColor
	}
	if mask&uint32(style.ChBackground) != 0 {
		n.cs.Background = av.Background
		n.cs.Set |= 1 << style.PropBackgroundColor
	}
	if mask&uint32(style.ChBrightness) != 0 {
		n.cs.Brightness = av.Brightness
		n.cs.Set |= 1 << style.PropFilter
	}
	if mask&uint32(style.ChTransform) != 0 {
		n.cs.Transform = av.Transform
		n.cs.Set |= 1 << style.PropTransform
	}
	if mask&uint32(style.ChIconXform) != 0 {
		n.cs.IconXform = av.IconXform
		n.cs.Set |= 1 << style.PropIconTransform
	}
	n.Invalidate()
}

// revert puts one animation's channels back to the cascade's,
// presence bits included: an undeclared channel stops overriding.
func (n *node) revert(r *animRunner) {
	c, mask := r.cascade, r.hadMask
	slots := []struct {
		ch    style.Chan
		write func()
	}{
		{style.ChOpacity, func() {
			n.cs.Opacity = c.Opacity
			n.cs.Set |= 1 << style.PropOpacity
		}},
		{style.ChColor, func() {
			n.cs.Color = c.Color
			n.cs.Set |= 1 << style.PropColor
		}},
		{style.ChBackground, func() {
			n.cs.Background = c.Background
			n.cs.Set |= 1 << style.PropBackgroundColor
		}},
		{style.ChBrightness, func() {
			n.cs.Brightness = c.Brightness
			n.cs.Set |= 1 << style.PropFilter
		}},
		{style.ChTransform, func() {
			n.cs.Transform = c.Transform
			n.cs.Set |= 1 << style.PropTransform
		}},
		{style.ChIconXform, func() {
			n.cs.IconXform = c.IconXform
			n.cs.Set |= 1 << style.PropIconTransform
		}},
	}
	for _, s := range slots {
		if mask&uint32(s.ch) != 0 {
			s.write()
		} else {
			n.cs.Set &^= 1 << channelProp(s.ch)
		}
	}
	n.Invalidate()
}
