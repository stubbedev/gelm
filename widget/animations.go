package widget

// The CSS animation runner: the node-side tween that plays the
// computed animation against the style cache, the same channel the
// background-color transitions write. A restyle syncs the runner with
// the computed animation - starts it, retargets it, pauses or resumes
// it, stops it - and each tick interpolates the keyframes at the
// current phase into the cached values and damages the widget. When
// the animation stops (rule removed, `animation: none`, iterations
// done) the cascade's own values are back in the cache, the revert
// CSS asks for.

import (
	"time"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/internal/style"
)

// syncAnimation aligns the runner with the node's computed animation.
func (n *node) syncAnimation(v style.Values) {
	want := v.Animation
	if !want.Active() || anim.Instant() {
		n.stopAnimation()
		return
	}
	if n.animName != want.Name {
		n.animName = want.Name
		n.animPhase = 0
		n.animCycle = 0
		n.animCascade, n.animHadOpacity, n.animHadRotation = cascadeChannels(v)
	}
	if !want.Running {
		if n.animCancel != nil {
			n.animCancel()
			n.animCancel = nil
		}
		n.applyAnimation(want)
		return
	}
	if n.animCancel == nil {
		n.playAnimation(want)
	}
}

// stopAnimation cancels a running animation. The caller has just
// recomputed the style cache from the cascade, so the revert is the
// recompute; only the tween and the name go.
func (n *node) stopAnimation() {
	if n.animCancel != nil {
		n.animCancel()
		n.animCancel = nil
	}
	if n.animName != "" {
		n.animName = ""
		n.Invalidate()
	}
}

// playAnimation launches the tween for one cycle; the landing tick
// relaunches for the next while iterations remain (anim callbacks may
// launch - Tick runs them with its lock released).
func (n *node) playAnimation(a style.Animation) {
	dur := time.Duration(a.Duration * float64(time.Second))
	n.animCancel = anim.Play(anim.Animate(dur, func(p float64) {
		if p >= 1 {
			n.animCycle++
			if !a.Infinite && float64(n.animCycle) >= animationIterations(a) {
				n.animCancel = nil
				n.animName = ""
				n.animPhase = 0
				n.animCycle = 0
				n.revertChannels()
				return
			}
			n.animPhase = 0
			n.playAnimation(a)
			return
		}
		ph := p
		if a.Alternate && n.animCycle%2 == 1 {
			ph = 1 - p
		}
		n.animPhase = ph
		n.applyAnimation(a)
	}).Easing(cssTiming(a.Timing)))
}

// animationIterations is the iteration count: the parsed number, or
// the CSS default of one.
func animationIterations(a style.Animation) float64 {
	if a.Iteration > 0 {
		return a.Iteration
	}
	return 1
}

// applyAnimation writes the keyframes' interpolated channels at the
// current phase into the style cache. Channels the keyframes do not
// animate hold their cascade values untouched.
func (n *node) applyAnimation(a style.Animation) {
	av := a.Keyframes.At(n.animPhase)
	if a.Keyframes.AnimatesOpacity() {
		n.cs.Opacity = av.Opacity
		n.cs.Set |= 1 << style.PropOpacity
	}
	if a.Keyframes.AnimatesRotation() {
		n.cs.Rotation = av.Rotation
		n.cs.Set |= 1 << style.PropIconTransform
	}
	n.Invalidate()
}

// revertChannels puts the animated channels back to the cascade's,
// presence bits included: an undeclared channel stops overriding.
func (n *node) revertChannels() {
	if n.animHadOpacity {
		n.cs.Opacity = n.animCascade.Opacity
		n.cs.Set |= 1 << style.PropOpacity
	} else {
		n.cs.Set &^= 1 << style.PropOpacity
	}
	if n.animHadRotation {
		n.cs.Rotation = n.animCascade.Rotation
		n.cs.Set |= 1 << style.PropIconTransform
	} else {
		n.cs.Set &^= 1 << style.PropIconTransform
	}
	n.Invalidate()
}

// cascadeChannels reads the cascade's animated channels and whether it
// declared them at all.
func cascadeChannels(v style.Values) (out style.AnimValues, opacity, rotation bool) {
	if v.Has(style.PropOpacity) {
		out.Opacity = v.Opacity
		opacity = true
	}
	if v.Has(style.PropIconTransform) {
		out.Rotation = v.Rotation
		rotation = true
	}
	return out, opacity, rotation
}
