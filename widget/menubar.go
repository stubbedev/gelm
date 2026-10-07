package widget

import (
	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// MenuBar is the in-window primary navigation (#91): a horizontal bar
// of root buttons, each opening its menu below through the existing
// popover machinery - the PopoverMenuBar shape. The bar never opens
// popovers itself (that is app plumbing); it reports the activation.
//
// Keyboard: F10 focuses the bar's first button when the app routes the
// key to it - MenuBar implements KeyActionHandler for exactly that
// handoff.
type MenuBar struct {
	node
	face   render.Font
	sizePx float64
	root   *Box

	// OnRoot fires when root i activates (click or keyboard): the app
	// opens the menu popover anchored at that root's button.
	OnRoot func(i int, anchor Boundser)

	buttons []*Button
	focused bool
}

// NewMenuBar returns a bar with one button per root label.
func NewMenuBar(face render.Font, sizePx float64, roots ...string) *MenuBar {
	face = requireFace("widget.NewMenuBar", face)
	m := &MenuBar{face: face, sizePx: sizePx}
	m.SetElement("menubar")
	m.root = NewBox(Row, 2, 4)
	for i, label := range roots {
		which := i
		btn := NewButton(NewLabel(face, sizePx, label, Current().Text), 10, 5)
		btn.OnClick = func() { m.fire(which) }
		m.buttons = append(m.buttons, btn)
		m.root.Append(btn, false)
	}
	return m
}

// Root returns root i's button, the anchor for its popover.
func (m *MenuBar) Root(i int) Boundser {
	if i < 0 || i >= len(m.buttons) {
		return nil
	}
	return m.buttons[i]
}

// Roots reports the root count.
func (m *MenuBar) Roots() int { return len(m.buttons) }

// SetFocused paints the bar's focus state (F10 handoff).
func (m *MenuBar) SetFocused(on bool) {
	m.SetState(StateSelected, on)
	m.Invalidate()
}

func (m *MenuBar) fire(i int) {
	if m.OnRoot != nil {
		m.OnRoot(i, m.buttons[i])
	}
}

// KeyAction handles the F10 handoff: it activates the first root,
// the way GTK menu bars open on F10.
func (m *MenuBar) KeyAction(a KeyAction, mods Mods) {
	if a != KeyEnter && a != KeySpace || len(m.buttons) == 0 {
		return
	}
	m.fire(0)
}

// Measure wants the buttons' row, full width.
func (m *MenuBar) Measure(con Constraints) Size {
	if sz, ok := m.measureHit(con); ok {
		return sz
	}
	inner := m.root.Measure(con)
	return m.measureStore(con, clampSize(Size{W: con.Max.W, H: inner.H}, con))
}

// Arrange fills and lays the row.
func (m *MenuBar) Arrange(r render.Rect) {
	m.node.Arrange(r)
	m.root.Arrange(r)
	setParents(m, m.root)
}

// Paint draws the bar's surface then the row.
func (m *MenuBar) Paint(cv *render.Canvas) {
	th := Current()
	v := m.style(m)
	paintBoxBehind(cv, v, m.bounds, radiusOr(v, 0), borderOf(v), pickc(0, v, style.PropBackgroundColor, th.Surface))
	PaintChild(cv, m.root)
}

// HitTest resolves into the row.
func (m *MenuBar) HitTest(p Point) Widget {
	if !m.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if hit := m.root.HitTest(p); hit != nil {
		return hit
	}
	return m
}

// styleChildren is the row (styleKids).
func (m *MenuBar) styleChildren() []Widget { return []Widget{m.root} }
