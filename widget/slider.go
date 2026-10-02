package widget

import (
	"math"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// Slider is a horizontal slider over [min, max] with an optional step.
// Dragging is input's job (M4): it calls SetValue with values derived from
// ValueFromX and flips Pressed around a drag.
//
// It is GtkScale's node tree: `scale` (its box, opacity, outline, and
// :hover while the pointer is over it) holding `trough` (background,
// radius, min-height, and min-width as the natural width) holding
// `highlight` (the filled part) and `slider` (the knob: background,
// radius, min size, opacity, box-shadow). Unstyled it keeps the theme's
// 4px trough and 14px knob.
type Slider struct {
	node
	min, max, step, value float64

	// Pressed reports whether a drag is in progress.
	Pressed bool
	hovered bool
	trough  sliderTrough

	// OnChanged fires after every value change, including programmatic
	// ones.
	OnChanged func(v float64)
}

// NewSlider returns a horizontal slider over [min, max] at value, with
// changes snapped to step (0 disables snapping).
func NewSlider(min, max, step, value float64) *Slider {
	s := &Slider{min: min, max: max, step: step}
	s.value = s.clamp(value)
	s.trough.SetElement("trough")
	s.trough.highlight.SetElement("highlight")
	s.trough.knob.SetElement("slider")
	setParents(s, &s.trough)
	setParents(&s.trough, &s.trough.highlight, &s.trough.knob)
	return s
}

// sliderTrough is the scale's trough node and its highlight and knob.
type sliderTrough struct {
	entryPart
	highlight, knob entryPart
}

func (t *sliderTrough) styleChildren() []Widget { return []Widget{&t.highlight, &t.knob} }

// styleChildren is the trough (styleKids).
func (s *Slider) styleChildren() []Widget { return []Widget{&s.trough} }

// SetHovered implements HoverSetter: `scale:hover` restyles the
// trough and knob (a knob shown only on hover).
func (s *Slider) SetHovered(on bool) {
	if s.hovered != on {
		s.hovered = on
		s.invalidateState(style.Hover)
	}
}

// sliderPad is an unstyled trough's inset each side, room for the
// knob at the extremes.
const sliderPad = 4

// troughRect is the trough inside the content box: its min-height
// tall (4px unstyled), centered, the content's width (less the
// unstyled inset).
func (s *Slider) troughRect() render.Rect {
	_, c := boxRects(boxOf(s.style(s), render.Insets{}), s.bounds)
	tv := s.trough.style(&s.trough)
	h := picki(tv, style.PropMinHeight, 4)
	pad := sliderPad
	if tv.Has(style.PropMinHeight) || tv.Has(style.PropBackgroundColor) {
		pad = 0
	}
	return render.Rect{X: c.X + pad, Y: c.Y + (c.H-h)/2, W: max(c.W-2*pad, 0), H: h}
}

// knobSize is the knob's min size (14px unstyled).
func (s *Slider) knobSize() (w, h int) {
	kv := s.trough.knob.style(&s.trough.knob)
	return picki(kv, style.PropMinWidth, 14), picki(kv, style.PropMinHeight, 14)
}

// Value returns the current value.
func (s *Slider) Value() float64 {
	return s.value
}

// SetValue clamps v to [min, max], snaps it to step, and fires OnChanged
// when it moved.
func (s *Slider) SetValue(v float64) {
	v = s.clamp(v)
	if v == s.value {
		return
	}
	s.value = v
	s.Invalidate()
	if s.OnChanged != nil {
		s.OnChanged(v)
	}
}

func (s *Slider) clamp(v float64) float64 {
	if s.step > 0 {
		v = math.Round(v/s.step) * s.step
	}
	return math.Min(s.max, math.Max(s.min, v))
}

// ValueFromX maps a canvas x across the trough to a value (an unstyled
// trough is inset 4px each side so the extremes are reachable).
func (s *Slider) ValueFromX(x int) float64 {
	tr := s.troughRect()
	span := float64(tr.W)
	if span <= 0 {
		return s.min
	}
	t := float64(x-tr.X) / span
	t = math.Min(1, math.Max(0, t))
	return s.min + t*(s.max-s.min)
}

// Measure wants the trough's min-width (200px unstyled) by the taller
// of the trough and the knob, at least 18px, inside the scale's box.
func (s *Slider) Measure(con Constraints) Size {
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	v := s.style(s)
	tv := s.trough.style(&s.trough)
	_, kh := s.knobSize()
	w := picki(tv, style.PropMinWidth, 200)
	h := max(18, picki(tv, style.PropMinHeight, 4), kh)
	return s.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		return clampSize(Size{W: w, H: h}, inner)
	}))
}

// Paint draws the trough, its highlight, and the knob, each from its
// node's style over the theme's colors. Disabled, the theme colors fade
// through the derived disabled colors (a stylesheet fades with its own
// :disabled rules).
func (s *Slider) Paint(cv *render.Canvas) {
	t := Current()
	troughCol, fillCol, knobCol := t.Border, t.Accent, t.Text
	if !IsEnabled(s) {
		troughCol, fillCol, knobCol = t.DisabledText(), t.DisabledAccent(), t.DisabledText()
	}
	v := s.style(s)
	fx := pushEffects(cv, v)
	defer fx.pop(cv)
	tr := s.troughRect()
	tv := s.trough.style(&s.trough)
	trRadii := radiusOr(tv, tr.H/2)
	paintBoxBehind(cv, tv, tr, trRadii, borderOf(tv), pickc(0, tv, style.PropBackgroundColor, troughCol))

	hv := s.trough.highlight.style(&s.trough.highlight)
	filled := tr
	filled.W = int(float64(tr.W) * s.fraction())
	if filled.W > 0 {
		paintBoxBehind(cv, hv, filled, radiusOr(hv, trRadii.TopLeft), borderOf(hv), pickc(0, hv, style.PropBackgroundColor, fillCol))
	}

	kv := s.trough.knob.style(&s.trough.knob)
	kw, kh := s.knobSize()
	cx := tr.X + int(float64(tr.W)*s.fraction())
	knob := render.Rect{X: cx - kw/2, Y: tr.Y + tr.H/2 - kh/2, W: kw, H: kh}
	kfx := pushEffects(cv, kv)
	paintBoxBehind(cv, kv, knob, radiusOr(kv, min(kw, kh)/2), borderOf(kv), pickc(0, kv, style.PropBackgroundColor, knobCol))
	kfx.pop(cv)
	paintOutline(cv, v, s.bounds, radiusOr(v, 0))
}

func (s *Slider) fraction() float64 {
	if s.max == s.min {
		return 0
	}
	return (s.value - s.min) / (s.max - s.min)
}

// Role implements Roleer.
func (s *Slider) Role() Role { return RoleSlider }

// HitTest returns the slider when p is inside its bounds.
func (s *Slider) HitTest(p Point) Widget {
	return s.HitLeaf(s, p)
}

// SetPressed implements PressSetter; the trough brightens while dragging.
func (s *Slider) SetPressed(on bool) {
	if s.Pressed == on {
		return
	}
	s.Pressed = on
	s.invalidateState(style.Active)
}

// DragMove sets the value from the pointer position while pressed.
// Disabled sliders ignore the drag.
func (s *Slider) DragMove(p Point) {
	if !IsEnabled(s) {
		return
	}
	s.SetValue(s.ValueFromX(p.X))
}

// KeyAction implements KeyActionHandler: arrows nudge the value by one
// step (or a tenth of the range without one), Home and End jump.
// Disabled sliders ignore keys.
func (s *Slider) KeyAction(a KeyAction, mods Mods) {
	if !IsEnabled(s) {
		return
	}
	step := s.step
	if step <= 0 {
		step = (s.max - s.min) / 10
	}
	switch a {
	case KeyLeft, KeyDown:
		s.SetValue(s.value - step)
	case KeyRight, KeyUp:
		s.SetValue(s.value + step)
	case KeyHome:
		s.SetValue(s.min)
	case KeyEnd:
		s.SetValue(s.max)
	}
}
