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
	inner := Constraints{
		Min: Size{},
		Max: Size{W: max(0, con.Max.W-2*b.padding), H: max(0, con.Max.H-2*b.padding)},
	}
	nat := b.child.Measure(inner)
	return clampSize(Size{W: nat.W + 2*b.padding, H: nat.H + 2*b.padding}, con)
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

// Paint draws the state's background, then the child.
func (b *Button) Paint(cv *render.Canvas) {
	bg := b.Bg
	switch {
	case b.Pressed:
		bg = b.BgPressed
	case b.Hovered:
		bg = b.BgHover
	}
	cv.RoundedRect(b.bounds, b.radius, bg)
	b.child.Paint(cv)
}

// HitTest returns the button when p is inside its bounds.
func (b *Button) HitTest(p Point) Widget {
	return b.HitLeaf(b, p)
}

// Click fires the OnClick hook. It is a no-op without one.
func (b *Button) Click() {
	if b.OnClick != nil {
		b.OnClick()
	}
}

// SetHovered implements HoverSetter.
func (b *Button) SetHovered(on bool) { b.Hovered = on }

// SetPressed implements PressSetter.
func (b *Button) SetPressed(on bool) { b.Pressed = on }

// KeyAction activates the button on Enter when focused.
func (b *Button) KeyAction(a KeyAction) {
	if a == KeyEnter {
		b.Click()
	}
}
