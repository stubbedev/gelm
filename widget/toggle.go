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
	s.Invalidate()
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
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	return s.measureStore(con, clampSize(Size{W: 40, H: 22}, con))
}

// Paint draws the track and knob. The knob sits at the right when on.
// Disabled, the on-track and knob fade through the derived disabled
// colors. Zero colors fall back to the theme.
func (s *Switch) Paint(cv *render.Canvas) {
	t := Current()
	track, knob := t.Surface, t.Text
	if s.on {
		track = t.Accent
	}
	if !IsEnabled(s) {
		track, knob = t.DisabledSurface(), t.DisabledText()
		if s.on {
			track = t.DisabledAccent()
		}
	}
	cv.RoundedRect(s.bounds, s.bounds.H/2, track)

	knobD := s.bounds.H - 6
	kx := s.bounds.X + 3
	if s.on {
		kx = s.bounds.X + s.bounds.W - 3 - knobD
	}
	cv.RoundedRect(render.Rect{X: kx, Y: s.bounds.Y + 3, W: knobD, H: knobD}, knobD/2, knob)
}

// Role implements Roleer.
func (s *Switch) Role() Role { return RoleSwitch }

// HitTest returns the switch when p is inside its bounds.
func (s *Switch) HitTest(p Point) Widget {
	return s.HitLeaf(s, p)
}

// ClickAt toggles the switch; the Router invokes it on press+release.
// Disabled switches ignore clicks.
func (s *Switch) ClickAt(Point) {
	if IsEnabled(s) {
		s.Toggle()
	}
}

// SetPressed is a no-op: the switch has no pressed visual.
func (s *Switch) SetPressed(bool) {}

// KeyAction implements KeyActionHandler: Enter toggles when focused,
// which makes the switch reachable and activatable by keyboard only.
// Disabled switches ignore keys.
func (s *Switch) KeyAction(a KeyAction, _ Mods) {
	if a == KeyEnter && IsEnabled(s) {
		s.Toggle()
	}
}

// InsertRune implements RuneHandler: Space toggles, the other half of
// the GTK activation pair. Disabled switches ignore it.
func (s *Switch) InsertRune(r rune) {
	if r == ' ' && IsEnabled(s) {
		s.Toggle()
	}
}

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

// SetValue clamps v to [0, 1] and invalidates the bar so the next
// frame repaints the fill.
func (p *ProgressBar) SetValue(v float64) {
	v = math01(v)
	if v == p.value {
		return
	}
	p.value = v
	p.Invalidate()
}

// Measure wants a fixed 160x10 trough, clamped to con.
func (p *ProgressBar) Measure(con Constraints) Size {
	if sz, ok := p.measureHit(con); ok {
		return sz
	}
	return p.measureStore(con, clampSize(Size{W: 160, H: 10}, con))
}

// Paint draws the trough and the proportional fill. Zero colors fall back
// to the theme.
func (p *ProgressBar) Paint(cv *render.Canvas) {
	t := Current()
	cv.RoundedRect(p.bounds, p.bounds.H/2, t.Surface)
	fill := p.bounds
	fill.W = int(float64(p.bounds.W) * p.value)
	if fill.W > 0 {
		cv.RoundedRect(fill, p.bounds.H/2, t.Accent)
	}
}

// Role implements Roleer.
func (p *ProgressBar) Role() Role { return RoleProgressBar }

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
	c.Invalidate()
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
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	return c.measureStore(con, clampSize(Size{W: 20, H: 20}, con))
}

// Paint draws the box; when checked, an accent fill and a check mark.
// Colors fall back to the theme. Disabled, box and fill fade through
// the derived disabled colors.
func (c *CheckButton) Paint(cv *render.Canvas) {
	t := Current()
	boxCol, fillCol, tickCol := t.Border, t.Accent, t.OnAccent
	if !IsEnabled(c) {
		boxCol, fillCol = t.DisabledText(), t.DisabledAccent()
		tickCol = scaleAlpha(t.OnAccent, disabledFade)
	}
	box := c.bounds
	if box.W > box.H {
		box.W = box.H
	}
	if box.H > box.W {
		box.H = box.W
	}
	cv.RoundedRect(box, 4, boxCol)
	inner := box
	inner.X += 2
	inner.Y += 2
	inner.W -= 4
	inner.H -= 4
	if c.checked {
		cv.RoundedRect(inner, 3, fillCol)
		bw, bh := float64(box.W), float64(box.H)
		stroke := max(2, box.W/7)
		x0 := box.X + int(0.24*bw)
		y0 := box.Y + int(0.55*bh)
		x1 := box.X + int(0.42*bw)
		y1 := box.Y + int(0.73*bh)
		x2 := box.X + int(0.78*bw)
		y2 := box.Y + int(0.27*bh)
		cv.Line(x0, y0, x1, y1, stroke, tickCol)
		cv.Line(x1, y1, x2, y2, stroke, tickCol)
		return
	}
	cv.RoundedRect(inner, 3, t.Bg)
}

// Role implements Roleer.
func (c *CheckButton) Role() Role { return RoleCheckBox }

// HitTest returns the checkbox when p is inside its bounds.
func (c *CheckButton) HitTest(p Point) Widget {
	return c.HitLeaf(c, p)
}

// ClickAt toggles the checkbox; the Router invokes it on press+release.
// Disabled checkboxes ignore clicks.
func (c *CheckButton) ClickAt(Point) {
	if IsEnabled(c) {
		c.Toggle()
	}
}

// KeyAction implements KeyActionHandler: Enter toggles when focused,
// which makes the checkbox reachable and activatable by keyboard only.
// Disabled checkboxes ignore keys.
func (c *CheckButton) KeyAction(a KeyAction, _ Mods) {
	if a == KeyEnter && IsEnabled(c) {
		c.Toggle()
	}
}

// InsertRune implements RuneHandler: Space toggles, the other half of
// the GTK activation pair. Disabled checkboxes ignore it.
func (c *CheckButton) InsertRune(r rune) {
	if r == ' ' && IsEnabled(c) {
		c.Toggle()
	}
}

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
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	return s.measureStore(con, clampSize(s.nat, con))
}

// Paint paints nothing.
func (s *Spacer) Paint(*render.Canvas) {}

// HitTest returns nil: empty space is not interactive.
func (s *Spacer) HitTest(Point) Widget { return nil }
