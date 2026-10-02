package widget

// Background-color transitions: a rule's `transition` covering
// background-color turns a changed computed color into a tween the
// paint reads out of the style cache; every other property still
// applies at once. This carries the fades a stylesheet writes the way
// a GTK theme animates them - the scale knob's hover fade, a row's
// hover and selection washes.

import (
	"math"
	"time"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// transitionBackground starts (or retargets) the background-color
// tween a restyle asks for. The tween's start is whatever the cache
// holds - the settled color, or an earlier tween's interpolated one -
// so a change mid-flight turns smoothly.
func (n *node) transitionBackground(old, v style.Values) {
	if n.bgCancel != nil {
		n.bgCancel()
		n.bgCancel = nil
	}
	if !v.Transition.Covers(style.PropBackgroundColor) || v.Background == old.Background {
		return
	}
	// A first style applies at once: there is no settled color to move
	// from, and an unarranged widget paints nothing yet.
	if n.styleSeen == 0 || anim.Instant() || n.bounds.Empty() {
		return
	}
	dur := time.Duration(v.Transition.Duration * float64(time.Second))
	delay := time.Duration(v.Transition.Delay * float64(time.Second))
	ease := cssTiming(v.Transition.Timing)
	from, to := old.Background, v.Background
	n.bgCancel = anim.Start(dur+delay, func(t float64) {
		p := 1.0
		if total := float64(dur + delay); total > 0 {
			p = (t*total - float64(delay)) / float64(dur)
		}
		switch {
		case p <= 0:
			n.cs.Background = from
		case p >= 1:
			n.cs.Background = to
			n.bgCancel = nil
		default:
			n.cs.Background = mixColor(from, to, ease(p))
		}
		n.Invalidate()
	})
}

// cssTiming maps the computed transition timing onto an easing: the
// cubic bezier solved per sample, steps quantized.
func cssTiming(tm style.Timing) anim.Easing {
	if tm.Steps > 0 {
		return func(t float64) float64 { return math.Floor(t*float64(tm.Steps)) / float64(tm.Steps) }
	}
	return anim.CubicBezier(tm.X1, tm.Y1, tm.X2, tm.Y2)
}

// mixColor interpolates premultiplied colors.
func mixColor(a, b render.Color, t float64) render.Color {
	mix := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5)
	}
	return render.Color(uint32(mix(a.A(), b.A()))<<24 |
		uint32(mix(a.R(), b.R()))<<16 |
		uint32(mix(a.G(), b.G()))<<8 |
		uint32(mix(a.B(), b.B())))
}
