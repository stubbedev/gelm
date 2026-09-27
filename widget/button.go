package widget

import (
	"github.com/stubbedev/gelm/render"
)

// Button is a clickable region with a rounded background and one child,
// usually a Box of an icon and a label. Input arrives in M4; the hover and
// pressed states are plain fields so tests and future input code drive
// them directly.
type Button struct {
	node
	child   Widget
	padding int
	radius  int

	// Background colors, one per visual state.
	Bg        render.Color
	BgHover   render.Color
	BgPressed render.Color

	// Hovered and Pressed select the painted state; Pressed wins.
	Hovered, Pressed bool

	// OnClick fires from Click, the hook M4 input will call.
	OnClick func()
}

// NewButton returns a button wrapping child with the given inner padding
// and corner radius.
func NewButton(child Widget, padding, radius int) *Button {
	return &Button{child: child, padding: padding, radius: radius}
}

// Measure pads the child's natural size on every side.
func (b *Button) Measure(con Constraints) Size {
	if sz, ok := b.measureHit(con); ok {
		return sz
	}
	inner := Constraints{
		Min: Size{},
		Max: Size{W: max(0, con.Max.W-2*b.padding), H: max(0, con.Max.H-2*b.padding)},
	}
	nat := b.child.Measure(inner)
	return b.measureStore(con, clampSize(Size{W: nat.W + 2*b.padding, H: nat.H + 2*b.padding}, con))
}

// Arrange insets the child by the padding inside r.
func (b *Button) Arrange(r render.Rect) {
	b.ArrangeRoot(r)
	b.child.Arrange(render.Rect{
		X: r.X + b.padding,
		Y: r.Y + b.padding,
		W: max(0, r.W-2*b.padding),
		H: max(0, r.H-2*b.padding),
	})
	setParents(b, b.child)
}

// ArrangeRoot records the button's own rect.
func (b *Button) ArrangeRoot(r render.Rect) {
	b.node.Arrange(r)
}

// Paint draws the state's background, then the child. A zero Bg family
// falls back to the theme. Disabled, the fill is the derived disabled
// surface and the child paints through the shared disabled fade, so
// whatever a button wraps (label, icon) mutes without the child
// knowing about the state.
func (b *Button) Paint(cv *render.Canvas) {
	t := Current()
	bg := t.resolve(b.Bg, t.Surface)
	switch {
	case !IsEnabled(b):
		bg = t.resolve(b.Bg, t.DisabledSurface())
	case b.Pressed:
		bg = t.resolve(b.BgPressed, t.SurfacePressed)
	case b.Hovered:
		bg = t.resolve(b.BgHover, t.SurfaceHover)
	}
	cv.RoundedRect(b.bounds, b.radius, bg)
	if IsEnabled(b) {
		b.child.Paint(cv)
		return
	}
	a := cv.PushAlpha(disabledFade)
	b.child.Paint(cv)
	cv.PopAlpha(a)
}

// Role implements Roleer.
func (b *Button) Role() Role { return RoleButton }

// HitTest returns the button when p is inside its bounds.
func (b *Button) HitTest(p Point) Widget {
	return b.HitLeaf(b, p)
}

// ClickAt fires the OnClick hook, ignoring the release point. It is a
// no-op without one, and a no-op while disabled: a dead button never
// fires.
func (b *Button) ClickAt(p Point) {
	b.click()
}

// click fires the hook from ClickAt and KeyAction. Disabled buttons
// swallow clicks, keys, and activation runes alike.
func (b *Button) click() {
	if !IsEnabled(b) {
		return
	}
	if b.OnClick != nil {
		b.OnClick()
	}
}

// SetHovered implements HoverSetter; the hover shade repaints.
func (b *Button) SetHovered(on bool) {
	if b.Hovered == on {
		return
	}
	b.Hovered = on
	b.Invalidate()
}

// SetPressed implements PressSetter; the pressed shade repaints.
func (b *Button) SetPressed(on bool) {
	if b.Pressed == on {
		return
	}
	b.Pressed = on
	b.Invalidate()
}

// KeyAction implements KeyActionHandler: Enter activates the button
// when focused, which is what makes it reachable by Tab at all - the
// traversal only collects KeyActionHandler implementations.
func (b *Button) KeyAction(a KeyAction, _ Mods) {
	if a == KeyEnter {
		b.click()
	}
}

// InsertRune implements RuneHandler: Space activates the button, the
// other half of the GTK activation pair. Text never reaches a focused
// button, so a blank rune can only mean "activate".
func (b *Button) InsertRune(r rune) {
	if r == ' ' {
		b.click()
	}
}
