package widget

import (
	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// Switch is a boolean toggle painted as a pill with a sliding knob. A
// stylesheet restyles it the GTK way: `switch` is the track (background,
// border-radius, min-width/min-height, the box layers; `:checked` while
// on) and `switch slider` the knob (min size, margin, background,
// radius, shadow). Unstyled, it is the theme's 40x22 pill.
type Switch struct {
	node
	on   bool
	knob stylePart

	// OnChanged fires after every state change, including programmatic
	// ones.
	OnChanged func(on bool)
}

// NewSwitch returns a switch in the given state. The pointer turns to
// the hand over it, the way wayle's switch primitive asks.
func NewSwitch(on bool) *Switch {
	s := &Switch{on: on}
	s.cursorName = "pointer"
	s.knob.SetElement("slider")
	s.SetState(StateChecked, on)
	return s
}

// On reports the switch state.
func (s *Switch) On() bool { return s.on }

// SetOn changes the state and fires OnChanged when it flipped.
func (s *Switch) SetOn(on bool) {
	if on == s.on {
		return
	}
	s.on = on
	s.SetState(StateChecked, on)
	s.Invalidate()
	if s.OnChanged != nil {
		s.OnChanged(on)
	}
}

// Toggle flips the state and fires OnChanged.
func (s *Switch) Toggle() {
	s.SetOn(!s.on)
}

// Measure wants the 40x22 pill, floored by the stylesheet's min size,
// inside its CSS box, clamped to con.
func (s *Switch) Measure(con Constraints) Size {
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	v := s.style(s)
	return s.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		if v.HasAny(style.PropMinWidth, style.PropMinHeight) {
			return clampSize(Size{}, inner)
		}
		return clampSize(Size{W: 40, H: 22}, inner)
	}))
}

// Arrange keeps the border box inside the margins.
func (s *Switch) Arrange(r render.Rect) {
	border, _ := boxRects(boxOf(s.style(s), render.Insets{}), r)
	s.node.Arrange(border)
	setParents(s, &s.knob)
}

// Paint draws the track and knob. The knob sits at the right when on.
// Disabled, the theme's on-track and knob fade through the derived
// disabled colors (a stylesheet fades with its own :disabled rules).
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
	v := s.style(s)
	fx := pushEffects(cv, v)
	defer fx.pop(cv)
	radii := radiusOr(v, s.bounds.H/2)
	bw := borderOf(v)
	paintBoxBehind(cv, v, s.bounds, radii, bw, pickc(0, v, style.PropBackgroundColor, track))
	defer paintOutline(cv, v, s.bounds, radii)

	content := boxOf(v, render.Insets{}).padding.Shrink(bw.Shrink(s.bounds))
	kv := s.knob.style(&s.knob)
	m := render.Insets{Top: 3, Right: 3, Bottom: 3, Left: 3}
	if kv.HasAny(style.PropMarginTop, style.PropMarginRight, style.PropMarginBottom, style.PropMarginLeft) {
		m = marginOf(kv)
	}
	d := max(0, content.H-m.Top-m.Bottom)
	kw, kh := picki(kv, style.PropMinWidth, d), picki(kv, style.PropMinHeight, d)
	kx := content.X + m.Left
	if s.on {
		kx = content.X + content.W - m.Right - kw
	}
	ky := content.Y + m.Top + (d-kh)/2
	knobRect := render.Rect{X: kx, Y: ky, W: kw, H: kh}
	s.knob.Arrange(knobRect)
	kr := radiusOr(kv, min(kw, kh)/2)
	paintBoxBehind(cv, kv, knobRect, kr, borderOf(kv), pickc(0, kv, style.PropBackgroundColor, knob))
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

// ProgressBar paints a read-only fill over a trough, as GTK's node
// tree: `progressbar > trough > progress` — the bar's box, the
// trough's, and the fill's, each styled by the cascade.
type ProgressBar struct {
	node
	value float64
	// Fill and Trough color the bar; zero is the theme's accent and
	// surface, under the trough and progress rules.
	Fill, Trough render.Color
	trough       progressTrough
}

// progressTrough is the bar's trough node and its progress fill.
type progressTrough struct {
	stylePart
	progress stylePart
}

func (t *progressTrough) styleChildren() []Widget { return []Widget{&t.progress} }

// styleChildren is the trough (styleKids).
func (p *ProgressBar) styleChildren() []Widget { return []Widget{&p.trough} }

// NewProgressBar returns a progress bar at value clamped to [0, 1].
func NewProgressBar(value float64) *ProgressBar {
	p := &ProgressBar{value: math01(value)}
	p.trough.SetElement("trough")
	p.trough.progress.SetElement("progress")
	setParents(p, &p.trough)
	setParents(&p.trough, &p.trough.progress)
	return p
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

// Measure wants a fixed 160x10 trough, floored by the trough's min
// sizes, inside the bar's CSS box, clamped to con.
func (p *ProgressBar) Measure(con Constraints) Size {
	if sz, ok := p.measureHit(con); ok {
		return sz
	}
	tv := p.trough.style(&p.trough)
	w, h := picki(tv, style.PropMinWidth, 160), picki(tv, style.PropMinHeight, 10)
	v := p.style(p)
	return p.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		return clampSize(Size{W: w, H: h}, inner)
	}))
}

// Arrange records the bar's box; the trough's border box is it.
func (p *ProgressBar) Arrange(r render.Rect) {
	p.node.Arrange(r)
	p.trough.Arrange(r)
}

// Paint draws the trough and the proportional fill through their
// nodes. Zero colors fall back to the theme, under the cascade.
func (p *ProgressBar) Paint(cv *render.Canvas) {
	t := Current()
	tv := p.trough.style(&p.trough)
	fx := pushEffects(cv, tv)
	defer fx.pop(cv)
	radii := radiusOr(tv, p.bounds.H/2)
	_, inner := boxRects(boxOf(tv, render.Insets{}), p.bounds)
	paintBoxBehind(cv, tv, p.bounds, radii, borderOf(tv), pickc(0, tv, style.PropBackgroundColor, p.troughFill(t)))
	fw := int(float64(inner.W) * p.value)
	if fw > 0 {
		fr := inner
		fr.W = fw
		p.trough.progress.Arrange(fr)
		pv := p.trough.progress.style(&p.trough.progress)
		fxp := pushEffects(cv, pv)
		paintBoxBehind(cv, pv, fr, radiusOr(pv, inner.H/2), borderOf(pv), pickc(0, pv, style.PropBackgroundColor, p.progressFill(t)))
		fxp.pop(cv)
	}
}

// troughFill is the trough's color: the programmatic Trough, else the
// theme's surface.
func (p *ProgressBar) troughFill(t *Theme) render.Color {
	if p.Trough != 0 {
		return p.Trough
	}
	return t.Surface
}

// progressFill is the fill's color: the programmatic Fill, else the
// theme's accent.
func (p *ProgressBar) progressFill(t *Theme) render.Color {
	if p.Fill != 0 {
		return p.Fill
	}
	return t.Accent
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
	// check is the `check` node under the checkbox (`checkbutton >
	// check`): the drawn indicator, styled and painted through it.
	check stylePart
	// mark is the themed icon the check node's -gtk-icon-source names,
	// cached per name and tint.
	mark     *Icon
	markName string
	markTint render.Color
	// group, when set, makes the checkbox a radio button (SetGroup).
	group *radioGroup

	// OnChanged fires after every state change, including programmatic
	// ones.
	OnChanged func(checked bool)
}

// NewCheckButton returns a checkbox in the given state. The pointer
// turns to the hand over it, the way wayle's checkbox primitive asks.
func NewCheckButton(checked bool) *CheckButton {
	c := &CheckButton{checked: checked}
	c.cursorName = "pointer"
	c.check.SetElement("check")
	return c
}

// styleChildren is the check (styleKids).
func (c *CheckButton) styleChildren() []Widget { return []Widget{&c.check} }

// Checked reports the state.
func (c *CheckButton) Checked() bool { return c.checked }

// SetChecked changes the state and fires OnChanged when it flipped.
func (c *CheckButton) SetChecked(checked bool) {
	if checked == c.checked {
		return
	}
	c.checked = checked
	c.invalidateState(style.Checked)
	c.Invalidate()
	if checked && c.group != nil {
		c.group.activated(c)
	}
	if c.OnChanged != nil {
		c.OnChanged(checked)
	}
}

// Toggle flips the state and fires OnChanged; a radio button only ever
// checks - clicking the checked one keeps it checked.
func (c *CheckButton) Toggle() {
	if c.group != nil && c.checked {
		return
	}
	c.SetChecked(!c.checked)
}

// SetInconsistent marks the check indeterminate (`:indeterminate`),
// the neither-checked-nor-unchecked state a tri-state control shows.
func (c *CheckButton) SetInconsistent(on bool) { c.SetState(StateIndeterminate, on) }

// Inconsistent reports the indeterminate state.
func (c *CheckButton) Inconsistent() bool { return c.HasState(StateIndeterminate) }

// Measure sizes the indicator: the check's min-width/min-height (the
// theme's 20x20 unstyled), inside the checkbox's CSS box.
func (c *CheckButton) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	kv := c.check.style(&c.check)
	w, h := picki(kv, style.PropMinWidth, 20), picki(kv, style.PropMinHeight, 20)
	v := c.style(c)
	return c.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		return clampSize(Size{W: w, H: h}, inner)
	}))
}

// Arrange records the box and links the check below it, so the
// cascade reaches `checkbutton > check`.
func (c *CheckButton) Arrange(r render.Rect) {
	c.node.Arrange(r)
	setParents(c, &c.check)
}

// checkRing is the unstyled indicator's border width.
const checkRing = 2

// Paint draws the indicator through the check node: the box layers
// (background, border, radius) it resolves, the tick or minus bar in
// its color. Unstyled, a 2px theme-border ring around the theme
// surface, accent-filled with a mark while checked or inconsistent.
// Disabled, ring and mark fade through the derived disabled colors.
func (c *CheckButton) Paint(cv *render.Canvas) {
	t := Current()
	kv := c.check.style(&c.check)
	fx := pushEffects(cv, kv)
	defer fx.pop(cv)
	box := c.bounds
	if box.W > box.H {
		box.W = box.H
	}
	if box.H > box.W {
		box.H = box.W
	}
	var fill, tick render.Color
	if c.checked || c.Inconsistent() {
		fill, tick = t.Accent, pickc(0, kv, style.PropColor, t.OnAccent)
		if !IsEnabled(c) {
			fill, tick = t.DisabledAccent(), scaleAlpha(tick, disabledFade)
		}
	} else {
		fill = t.Bg
	}
	ring := ringOr(kv, checkRing)
	cols := borderColors(kv)
	if !kv.HasAny(style.PropBorderTopWidth, style.PropBorderRightWidth, style.PropBorderBottomWidth, style.PropBorderLeftWidth) && !IsEnabled(c) {
		cols = [4]render.Color{t.DisabledText(), t.DisabledText(), t.DisabledText(), t.DisabledText()}
	}
	radii := radiusOr(kv, 4)
	if c.group != nil {
		radii = radiusOr(kv, box.W/2)
	}
	c.check.Arrange(box)
	paintBoxBehindCol(cv, kv, box, radii, ring, pickc(0, kv, style.PropBackgroundColor, fill), cols)
	if src := kv.IconSource; src != "" && (c.checked || c.Inconsistent()) {
		// The stylesheet's themed mark (`-gtk-icon-source`) replaces the
		// painted tick, recolored by its palette over the mark color.
		tint := kv.PaletteTint
		if tint == 0 {
			tint = tick
		}
		if !IsEnabled(c) {
			tint = scaleAlpha(tint, disabledFade)
		}
		c.drawThemedMark(cv, src, box, tint)
	} else if c.group != nil && c.checked {
		// The radio's mark: a dot a third of the indicator.
		d := max(box.W/3, 2)
		cv.RoundedRect(render.Rect{X: box.X + (box.W-d)/2, Y: box.Y + (box.H-d)/2, W: d, H: d}, d/2, tick)
	} else if c.checked || c.Inconsistent() {
		drawCheckMark(cv, box, c.Inconsistent(), tick)
	}
	paintOutline(cv, kv, box, radii)
}

// drawThemedMark draws the check node's -gtk-icon-source icon centered
// in the indicator, re-tinted when the color moved on.
func (c *CheckButton) drawThemedMark(cv *render.Canvas, name string, box render.Rect, tint render.Color) {
	if c.mark == nil || c.markName != name || c.markTint != tint {
		size := max(8, min(box.W, box.H)-2*checkRing)
		c.mark = NewThemeIcon(name, size)
		c.mark.SetTint(tint)
		c.markName, c.markTint = name, tint
		setParents(c, c.mark)
	}
	c.mark.Arrange(render.Rect{
		X: box.X + (box.W-c.mark.Bounds().W)/2, Y: box.Y + (box.H-c.mark.Bounds().H)/2,
		W: c.mark.Bounds().W, H: c.mark.Bounds().H,
	})
	c.mark.Measure(Constraints{Max: Size{W: box.W, H: box.H}})
	PaintChild(cv, c.mark)
}

// drawCheckMark paints the mark inside an indicator box: the tick, or
// the minus bar when indeterminate.
func drawCheckMark(cv *render.Canvas, box render.Rect, minus bool, col render.Color) {
	bw, bh := float64(box.W), float64(box.H)
	stroke := max(2, box.W/7)
	if minus {
		y := box.Y + box.H/2
		cv.Line(box.X+int(0.26*bw), y, box.X+int(0.74*bw), y, stroke, col)
		return
	}
	x0, y0 := box.X+int(0.24*bw), box.Y+int(0.55*bh)
	x1, y1 := box.X+int(0.42*bw), box.Y+int(0.73*bh)
	x2, y2 := box.X+int(0.78*bw), box.Y+int(0.27*bh)
	cv.Line(x0, y0, x1, y1, stroke, col)
	cv.Line(x1, y1, x2, y2, stroke, col)
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

// styleChildren is the knob (styleKids).
func (s *Switch) styleChildren() []Widget { return []Widget{&s.knob} }
