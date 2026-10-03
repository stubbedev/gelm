package widget

// Transitions: a rule's `transition` turns a changed computed value
// into a tween over the style cache, every other property still
// applying at once. The engine is channel-based: each animatable
// longhand - the colors, opacity, the filter's brightness, the letter
// spacing, the box geometry, and the transforms - has a channel the
// restyle fills when its computed value moved; one tween advances
// every filled channel per frame. This carries the fades a stylesheet
// writes the way a GTK theme animates them: the knob's hover fade, a
// row's wash, a zoom.

import (
	"math"
	"time"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// tweenChan is one animatable channel captured for a transition: the
// property whose `transition` declaration guards it, and the apply
// that eases the cache toward its target (e 0..1). Layout channels
// drop the measure caches per frame instead of just repainting.
type tweenChan struct {
	prop   style.Prop
	layout bool
	apply  func(cs *style.Values, e float64)
}

// colorChan captures a color channel; a side the cascade never
// declared reads as transparent and still tweens, like CSS's
// from-transparent interpolation.
func colorChan(p style.Prop, from, to render.Color, set func(*style.Values, render.Color)) *tweenChan {
	if from == to {
		return nil
	}
	return &tweenChan{prop: p, apply: func(cs *style.Values, e float64) {
		set(cs, mixColor(from, to, e))
	}}
}

// floatChan captures a float channel; a channel whose presence bit
// changed (first declaration, last rule removed) snaps.
func floatChan(p style.Prop, from, to float64, bothPresent bool, set func(*style.Values, float64)) *tweenChan {
	if !bothPresent || from == to {
		return nil
	}
	return &tweenChan{prop: p, apply: func(cs *style.Values, e float64) {
		set(cs, from+(to-from)*e)
	}}
}

// intChan captures an integer channel.
func intChan(p style.Prop, from, to int, bothPresent bool, set func(*style.Values, int), layout bool) *tweenChan {
	if !bothPresent || from == to {
		return nil
	}
	return &tweenChan{prop: p, layout: layout, apply: func(cs *style.Values, e float64) {
		set(cs, int(math.Round(float64(from)+(float64(to)-float64(from))*e)))
	}}
}

// xformChan captures a transform channel. An undeclared side reads as
// the identity - Compute leaves unset longhands zero, and the paint
// paths Has-guard them - and a declaration dropping eases back to that
// identity (a hover zoom reversing), the CSS implicit to-value. The
// parts walk their decompositions, so a rotation turns instead of
// collapsing.
func xformChan(p style.Prop, from, to style.Xform, fromPresent, toPresent bool, set func(*style.Values, style.Xform)) *tweenChan {
	if !fromPresent {
		from = style.XformIdentity
	}
	if !toPresent {
		to = style.XformIdentity
	}
	if from.M == to.M {
		return nil
	}
	return &tweenChan{prop: p, apply: func(cs *style.Values, e float64) {
		set(cs, style.LerpXform(from, to, e))
	}}
}

// transitionValues starts (or retargets) the transition tween a
// restyle asks for. The tween's start is whatever the cache holds -
// the settled values, or an earlier tween's interpolated ones - so a
// change mid-flight turns smoothly. A first style applies at once:
// nothing settled to move from, and an unarranged widget paints
// nothing yet.
func (n *node) transitionValues(old, v style.Values) {
	if n.trCancel != nil {
		n.trCancel()
		n.trCancel = nil
	}
	if v.Transition.Duration <= 0 || anim.Instant() || n.styleSeen == 0 || n.bounds.Empty() {
		return
	}
	both := func(p style.Prop) bool { return old.Has(p) && v.Has(p) }
	var chans []*tweenChan
	add := func(c *tweenChan) {
		if c != nil && v.Transition.Covers(c.prop) {
			chans = append(chans, c)
		}
	}
	add(colorChan(style.PropColor, old.Color, v.Color, func(cs *style.Values, c render.Color) { cs.Color = c }))
	add(colorChan(style.PropBackgroundColor, old.Background, v.Background, func(cs *style.Values, c render.Color) { cs.Background = c }))
	for i, p := range [4]style.Prop{style.PropBorderTopColor, style.PropBorderRightColor, style.PropBorderBottomColor, style.PropBorderLeftColor} {
		add(colorChan(p, old.BorderColor[i], v.BorderColor[i], func(cs *style.Values, c render.Color) { cs.BorderColor[i] = c }))
	}
	add(colorChan(style.PropOutlineColor, old.OutlineColor, v.OutlineColor, func(cs *style.Values, c render.Color) { cs.OutlineColor = c }))
	add(colorChan(style.PropCaretColor, old.CaretColor, v.CaretColor, func(cs *style.Values, c render.Color) { cs.CaretColor = c }))
	add(floatChan(style.PropOpacity, old.Opacity, v.Opacity, both(style.PropOpacity), func(cs *style.Values, x float64) { cs.Opacity = x }))
	add(floatChan(style.PropFilter, old.Brightness, v.Brightness, both(style.PropFilter), func(cs *style.Values, x float64) { cs.Brightness = x }))
	add(floatChan(style.PropLetterSpacing, old.LetterSpacing, v.LetterSpacing, both(style.PropLetterSpacing), func(cs *style.Values, x float64) { cs.LetterSpacing = x }))
	for i, p := range [4]style.Prop{style.PropPaddingTop, style.PropPaddingRight, style.PropPaddingBottom, style.PropPaddingLeft} {
		add(intChan(p, old.Padding.At(i), v.Padding.At(i), both(p), func(cs *style.Values, x int) { cs.Padding.SetAt(i, x) }, true))
	}
	for i, p := range [4]style.Prop{style.PropMarginTop, style.PropMarginRight, style.PropMarginBottom, style.PropMarginLeft} {
		add(intChan(p, old.Margin.At(i), v.Margin.At(i), both(p), func(cs *style.Values, x int) { cs.Margin.SetAt(i, x) }, true))
	}
	for i, p := range [4]style.Prop{style.PropBorderTopWidth, style.PropBorderRightWidth, style.PropBorderBottomWidth, style.PropBorderLeftWidth} {
		add(intChan(p, old.BorderWidth.At(i), v.BorderWidth.At(i), both(p), func(cs *style.Values, x int) { cs.BorderWidth.SetAt(i, x) }, true))
	}
	add(intChan(style.PropMinWidth, old.MinWidth, v.MinWidth, both(style.PropMinWidth), func(cs *style.Values, x int) { cs.MinWidth = x }, true))
	add(intChan(style.PropMinHeight, old.MinHeight, v.MinHeight, both(style.PropMinHeight), func(cs *style.Values, x int) { cs.MinHeight = x }, true))
	add(intChan(style.PropOutlineWidth, old.OutlineWidth, v.OutlineWidth, both(style.PropOutlineWidth), func(cs *style.Values, x int) { cs.OutlineWidth = x }, false))
	add(intChan(style.PropOutlineOffset, old.OutlineOffset, v.OutlineOffset, both(style.PropOutlineOffset), func(cs *style.Values, x int) { cs.OutlineOffset = x }, false))
	for i, p := range [4]style.Prop{style.PropBorderTopLeftRadius, style.PropBorderTopRightRadius, style.PropBorderBottomRightRadius, style.PropBorderBottomLeftRadius} {
		add(intChan(p, old.Radius.At(i), v.Radius.At(i), both(p), func(cs *style.Values, x int) { cs.Radius.SetAt(i, x) }, false))
	}
	add(xformChan(style.PropTransform, old.Transform, v.Transform, old.Has(style.PropTransform), v.Has(style.PropTransform), func(cs *style.Values, m style.Xform) { cs.Transform = m }))
	add(xformChan(style.PropIconTransform, old.IconXform, v.IconXform, old.Has(style.PropIconTransform), v.Has(style.PropIconTransform), func(cs *style.Values, m style.Xform) { cs.IconXform = m }))
	if len(chans) == 0 {
		return
	}

	dur := time.Duration(v.Transition.Duration * float64(time.Second))
	delay := time.Duration(v.Transition.Delay * float64(time.Second))
	ease := cssTiming(v.Transition.Timing)
	layout := false
	for _, c := range chans {
		if c.layout {
			layout = true
		}
	}
	total := dur + delay
	// One easing, applied once: the tween's curve maps raw progress
	// over the whole timeline (delay included) through the active
	// phase's CSS curve, so the callback's t is the eased phase - held
	// at 0 through the delay, the CSS fill-before behavior.
	n.trCancel = anim.Play(anim.Animate(total, func(t float64) {
		for _, c := range chans {
			c.apply(&n.cs, t)
		}
		if t >= 1 {
			n.trCancel = nil
		}
		if layout {
			n.InvalidateLayout()
		} else {
			n.Invalidate()
		}
	}).Easing(func(raw float64) float64 {
		if dur <= 0 {
			return 1
		}
		p := (raw*float64(total) - float64(delay)) / float64(dur)
		return ease(math.Min(math.Max(p, 0), 1))
	}))
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

// cssTiming maps the computed transition timing onto an easing: the
// cubic bezier solved per sample, steps quantized.
func cssTiming(tm style.Timing) anim.Easing {
	if tm.Steps > 0 {
		return func(t float64) float64 { return math.Floor(t*float64(tm.Steps)) / float64(tm.Steps) }
	}
	return anim.CubicBezier(tm.X1, tm.Y1, tm.X2, tm.Y2)
}
