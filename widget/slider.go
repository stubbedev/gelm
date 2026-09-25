package widget

import (
	"math"

	"github.com/stubbedev/gelm/render"
)

// Slider is a horizontal slider over [min, max] with an optional step.
// Dragging is input's job (M4): it calls SetValue with values derived from
// ValueFromX and flips Pressed around a drag.
type Slider struct {
	node
	min, max, step, value float64

	// Pressed reports whether a drag is in progress.
	Pressed bool

	// OnChanged fires after every value change, including programmatic
	// ones.
	OnChanged func(v float64)
}

// NewSlider returns a horizontal slider over [min, max] at value, with
// changes snapped to step (0 disables snapping).
func NewSlider(min, max, step, value float64) *Slider {
	s := &Slider{min: min, max: max, step: step}
	s.value = s.clamp(value)
	return s
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

// ValueFromX maps a canvas x inside the trough to a value, with 4px of
// handle padding on each side so the extremes are reachable.
func (s *Slider) ValueFromX(x int) float64 {
	span := float64(s.bounds.W - 8)
	if span <= 0 {
		return s.min
	}
	t := (float64(x-s.bounds.X) - 4) / span
	t = math.Min(1, math.Max(0, t))
	return s.min + t*(s.max-s.min)
}

// Measure wants a fixed 200x18 trough, clamped to con.
func (s *Slider) Measure(con Constraints) Size {
	return clampSize(Size{W: 200, H: 18}, con)
}

// Paint draws the trough and the handle at the value's position.
func (s *Slider) Paint(cv *render.Canvas) {
	cy := s.bounds.Y + s.bounds.H/2
	trough := render.Rect{X: s.bounds.X + 4, Y: cy - 2, W: s.bounds.W - 8, H: 4}
	cv.RoundedRect(trough, 2, render.RGB(0x45, 0x47, 0x5a))

	filled := trough
	filled.W = int(float64(trough.W) * s.fraction())
	cv.RoundedRect(filled, 2, render.RGB(0x89, 0xb4, 0xfa))

	knob := 14
	kx := s.bounds.X + 4 + int(float64(s.bounds.W-8)*s.fraction()) - knob/2
	cv.RoundedRect(render.Rect{X: kx, Y: cy - knob/2, W: knob, H: knob}, knob/2, render.RGB(0xcd, 0xd6, 0xf4))
}

func (s *Slider) fraction() float64 {
	if s.max == s.min {
		return 0
	}
	return (s.value - s.min) / (s.max - s.min)
}

// HitTest returns the slider when p is inside its bounds.
func (s *Slider) HitTest(p Point) Widget {
	return s.HitLeaf(s, p)
}

// SetPressed implements PressSetter; the trough brightens while dragging.
func (s *Slider) SetPressed(on bool) { s.Pressed = on }

// DragMove sets the value from the pointer position while pressed.
func (s *Slider) DragMove(p Point) {
	s.SetValue(s.ValueFromX(p.X))
}
