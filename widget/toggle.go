package widget

import (
	"github.com/stubbedev/gelm/render"
)

// Switch is a boolean toggle painted as a pill with a sliding knob.
type Switch struct {
	node
	on bool

	// OnChanged fires after every state change, including programmatic
	// ones.
	OnChanged func(on bool)
}

// NewSwitch returns a switch in the given state.
func NewSwitch(on bool) *Switch {
	return &Switch{on: on}
}

// On reports the switch state.
func (s *Switch) On() bool { return s.on }

// SetOn changes the state and fires OnChanged when it flipped.
func (s *Switch) SetOn(on bool) {
	if on == s.on {
		return
	}
	s.on = on
	if s.OnChanged != nil {
		s.OnChanged(on)
	}
}

// Toggle flips the state and fires OnChanged.
func (s *Switch) Toggle() {
	s.SetOn(!s.on)
}

// Measure wants a fixed 40x22 pill, clamped to con.
func (s *Switch) Measure(con Constraints) Size {
	return clampSize(Size{W: 40, H: 22}, con)
}

// Paint draws the track and knob. The knob sits at the right when on.
func (s *Switch) Paint(cv *render.Canvas) {
	track := render.RGB(0x45, 0x47, 0x5a)
	if s.on {
		track = render.RGB(0x89, 0xb4, 0xfa)
	}
	cv.RoundedRect(s.bounds, s.bounds.H/2, track)

	knobD := s.bounds.H - 6
	kx := s.bounds.X + 3
	if s.on {
		kx = s.bounds.X + s.bounds.W - 3 - knobD
	}
	cv.RoundedRect(render.Rect{X: kx, Y: s.bounds.Y + 3, W: knobD, H: knobD}, knobD/2, render.RGB(0xcd, 0xd6, 0xf4))
}

// HitTest returns the switch when p is inside its bounds.
func (s *Switch) HitTest(p Point) Widget {
	return s.HitLeaf(s, p)
}

// Click toggles the switch; the Router invokes it on press+release.
func (s *Switch) Click() { s.Toggle() }

// SetPressed is a no-op: the switch has no pressed visual.
func (s *Switch) SetPressed(bool) {}

// ProgressBar paints a read-only fill over a trough.
type ProgressBar struct {
	node
	value float64
}

// NewProgressBar returns a progress bar at value clamped to [0, 1].
func NewProgressBar(value float64) *ProgressBar {
	return &ProgressBar{value: math01(value)}
}

func math01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Value returns the fill fraction in [0, 1].
func (p *ProgressBar) Value() float64 { return p.value }

// SetValue clamps v to [0, 1] and repaints on the next frame.
func (p *ProgressBar) SetValue(v float64) {
	p.value = math01(v)
}

// Measure wants a fixed 160x10 trough, clamped to con.
func (p *ProgressBar) Measure(con Constraints) Size {
	return clampSize(Size{W: 160, H: 10}, con)
}

// Paint draws the trough and the proportional fill.
func (p *ProgressBar) Paint(cv *render.Canvas) {
	cv.RoundedRect(p.bounds, p.bounds.H/2, render.RGB(0x45, 0x47, 0x5a))
	fill := p.bounds
	fill.W = int(float64(p.bounds.W) * p.value)
	if fill.W > 0 {
		cv.RoundedRect(fill, p.bounds.H/2, render.RGB(0x89, 0xb4, 0xfa))
	}
}

// HitTest returns the bar when p is inside its bounds.
func (p *ProgressBar) HitTest(pt Point) Widget {
	return p.HitLeaf(p, pt)
}

// CheckButton is a boolean checkbox with a painted check mark.
type CheckButton struct {
	node
	checked bool

	// OnChanged fires after every state change, including programmatic
	// ones.
	OnChanged func(checked bool)
}

// NewCheckButton returns a checkbox in the given state.
func NewCheckButton(checked bool) *CheckButton {
	return &CheckButton{checked: checked}
}

// Checked reports the state.
func (c *CheckButton) Checked() bool { return c.checked }

// SetChecked changes the state and fires OnChanged when it flipped.
func (c *CheckButton) SetChecked(checked bool) {
	if checked == c.checked {
		return
	}
	c.checked = checked
	if c.OnChanged != nil {
		c.OnChanged(checked)
	}
}

// Toggle flips the state and fires OnChanged.
func (c *CheckButton) Toggle() {
	c.SetChecked(!c.checked)
}

// Measure wants a fixed 20x20 box, clamped to con.
func (c *CheckButton) Measure(con Constraints) Size {
	return clampSize(Size{W: 20, H: 20}, con)
}

// Paint draws the box; when checked, an accent fill and a check mark.
func (c *CheckButton) Paint(cv *render.Canvas) {
	box := c.bounds
	if box.W > box.H {
		box.W = box.H
	}
	if box.H > box.W {
		box.H = box.W
	}
	border := render.RGB(0x58, 0x5b, 0x70)
	cv.RoundedRect(box, 4, border)
	inner := box
	inner.X += 2
	inner.Y += 2
	inner.W -= 4
	inner.H -= 4
	if c.checked {
		cv.RoundedRect(inner, 3, render.RGB(0x89, 0xb4, 0xfa))
		cx, cy := inner.X+inner.W/2, inner.Y+inner.H/2
		cv.Line(cx-4, cy, cx-1, cy+3, 2, render.RGB(0x11, 0x11, 0x1b))
		cv.Line(cx-1, cy+3, cx+4, cy-3, 2, render.RGB(0x11, 0x11, 0x1b))
		return
	}
	cv.RoundedRect(inner, 3, render.RGB(0x1e, 0x1e, 0x2e))
}

// HitTest returns the checkbox when p is inside its bounds.
func (c *CheckButton) HitTest(p Point) Widget {
	return c.HitLeaf(c, p)
}

// Click toggles the checkbox; the Router invokes it on press+release.
func (c *CheckButton) Click() { c.Toggle() }

// Spacer is empty layout space of a fixed size.
type Spacer struct {
	node
	nat Size
}

// NewSpacer returns a spacer wanting w x h.
func NewSpacer(w, h int) *Spacer {
	return &Spacer{nat: Size{W: w, H: h}}
}

// Measure returns the spacer's size, clamped to con.
func (s *Spacer) Measure(con Constraints) Size {
	return clampSize(s.nat, con)
}

// Paint paints nothing.
func (s *Spacer) Paint(*render.Canvas) {}

// HitTest returns nil: empty space is not interactive.
func (s *Spacer) HitTest(Point) Widget { return nil }
