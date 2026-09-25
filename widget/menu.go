package widget

import (
	"github.com/stubbedev/gelm/render"
)

// MenuItem is one actionable row of a Menu. A nil OnClick paints the row
// muted and skips clicks.
type MenuItem struct {
	Label   string
	OnClick func()
}

// Menu is a vertical action list for popups and context menus: hovered
// row highlight, click to activate, click-away dismisses through
// OnDismiss.
type Menu struct {
	node
	face      *render.Typeface
	sizePx    float64
	items     []MenuItem
	hovered   int
	itemH     int
	OnDismiss func()
}

// NewMenu returns a menu showing items, painted with face at sizePx.
func NewMenu(face *render.Typeface, sizePx float64, items ...MenuItem) *Menu {
	return &Menu{
		face:    face,
		sizePx:  sizePx,
		items:   items,
		hovered: -1,
		itemH:   face.Shape("lg", sizePx).LineHeight() + 12,
	}
}

// Measure wants the widest label plus padding, and one row per item.
func (m *Menu) Measure(con Constraints) Size {
	w := 0
	for _, it := range m.items {
		if adv := int(m.face.Shape(it.Label, m.sizePx).Advance() + 0.5); adv > w {
			w = adv
		}
	}
	return clampSize(Size{W: w + 32, H: len(m.items)*m.itemH + 8}, con)
}

// Paint draws the item rows with the hovered one highlighted.
func (m *Menu) Paint(cv *render.Canvas) {
	t := Current()
	cv.RoundedRect(m.bounds, t.Radius, t.Surface)
	lineH := m.face.Shape("lg", m.sizePx).LineHeight()
	for i, it := range m.items {
		row := render.Rect{
			X: m.bounds.X + 4,
			Y: m.bounds.Y + 4 + i*m.itemH,
			W: m.bounds.W - 8,
			H: m.itemH - 2,
		}
		if i == m.hovered && it.OnClick != nil {
			hl := t.Accent
			cv.RoundedRect(row, t.Radius, render.RGBA(hl.R(), hl.G(), hl.B(), 70))
		}
		col := t.Text
		if it.OnClick == nil {
			col = t.TextMuted
		}
		baseline := row.Y + (row.H-lineH)/2 + int(m.face.Shape("lg", m.sizePx).Ascent()+0.5)
		m.face.Draw(cv, m.face.Shape(it.Label, m.sizePx), row.X+12, baseline, col)
	}
}

// HitTest returns the menu when p is inside its bounds.
func (m *Menu) HitTest(p Point) Widget { return m.HitLeaf(m, p) }

// SetHovered clears row hover when the pointer leaves the menu.
func (m *Menu) SetHovered(on bool) {
	if !on {
		m.hovered = -1
	}
}

// HoverMove tracks the hovered row as the pointer moves inside.
func (m *Menu) HoverMove(p Point) { m.hovered = m.itemAt(p) }

// itemAt maps a root-space point to an item index, -1 outside.
func (m *Menu) itemAt(p Point) int {
	if p.X < m.bounds.X || p.X >= m.bounds.X+m.bounds.W {
		return -1
	}
	i := (p.Y - m.bounds.Y - 4) / m.itemH
	if i < 0 || i >= len(m.items) {
		return -1
	}
	return i
}

// ClickAt activates the clicked row and dismisses the menu.
func (m *Menu) ClickAt(p Point) {
	i := m.itemAt(p)
	if i < 0 {
		m.dismiss()
		return
	}
	m.hovered = i
	item := m.items[i]
	if item.OnClick == nil {
		return
	}
	m.dismiss()
	item.OnClick()
}

// dismiss tears the menu down; the host closes its popup.
func (m *Menu) dismiss() {
	if m.OnDismiss != nil {
		m.OnDismiss()
	}
}

// DragMove keeps hover tracking during presses.
func (m *Menu) DragMove(p Point) { m.hovered = m.itemAt(p) }
