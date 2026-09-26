package widget

import (
	"github.com/stubbedev/gelm/render"
)

// ItemKind distinguishes the menu row styles.
type ItemKind uint8

// Item kinds.
const (
	ItemAction ItemKind = iota
	ItemCheck
	ItemRadio
	ItemSeparator
)

// MenuItem is one row of a Menu.
//
// Kind selects the row style: a plain action, a checkbox row (Checked
// toggles on activate), a radio row (Checked is exclusive within
// items sharing the same Group), or a separator (a real rule; skipped
// by keyboard motion - use MenuSeparator). Accel paints a
// right-aligned accelerator label. Items nests a submenu; activating
// such a row fires OnSubmenu so the host opens a child popup beside
// it. Mnemonics are deliberately not implemented: accelerators are
// display-only labels.
type MenuItem struct {
	Label   string
	OnClick func()
	Kind    ItemKind
	Checked bool
	Group   string
	Accel   string
	Items   []MenuItem
}

// MenuSeparator returns a separator item.
func MenuSeparator() MenuItem { return MenuItem{Kind: ItemSeparator} }

// Menu is a vertical action list for popups and context menus: hovered
// row highlight, pointer or keyboard activation (arrows move, Enter
// activates, Esc dismisses, Right opens a submenu), check and radio
// rows, accelerator labels, and nested submenus through OnSubmenu.
type Menu struct {
	node
	face      *render.Typeface
	sizePx    float64
	items     []MenuItem
	hovered   int
	itemH     int
	OnDismiss func()
	// OnSubmenu fires when a row with nested Items is activated; the
	// host opens a child popup for it beside the parent row.
	OnSubmenu func(index int, items []MenuItem)
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

// Measure wants the widest row (label plus indicator, accelerator, and
// submenu arrow) plus padding, and one row per item.
func (m *Menu) Measure(con Constraints) Size {
	w := 0
	for _, it := range m.items {
		row := m.face.Shape(it.Label, m.sizePx).Advance() + 24
		if it.Kind == ItemCheck || it.Kind == ItemRadio {
			row += 18
		}
		if it.Accel != "" {
			row += m.face.Shape(it.Accel, m.sizePx).Advance() + 16
		}
		if len(it.Items) > 0 {
			row += 14
		}
		if adv := int(row + 0.5); adv > w {
			w = adv
		}
	}
	return clampSize(Size{W: w + 16, H: len(m.items)*m.itemH + 8}, con)
}

// selectable reports whether keyboard motion may land on row i.
func (m *Menu) selectable(i int) bool {
	return i >= 0 && i < len(m.items) && m.items[i].Kind != ItemSeparator
}

// nextSelectable returns the nearest selectable row at or after i in
// the given direction, wrapping once.
func (m *Menu) nextSelectable(i, dir int) int {
	n := len(m.items)
	for range n {
		i = (i + dir + n) % n
		if m.selectable(i) {
			return i
		}
	}
	return -1
}

// Paint draws the item rows with the hovered one highlighted, item
// indicators, accelerator labels, and submenu arrows.
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
		if it.Kind == ItemSeparator {
			y := row.Y + row.H/2
			cv.Line(row.X+8, y, row.X+row.W-8, y, 1, t.Border)
			continue
		}
		if i == m.hovered {
			hl := t.Accent
			cv.RoundedRect(row, t.Radius, render.RGBA(hl.R(), hl.G(), hl.B(), 70))
		}
		x := row.X + 8
		switch it.Kind {
		case ItemCheck:
			cv.FillRect(render.Rect{X: x, Y: row.Y + (row.H-12)/2, W: 12, H: 12}, t.Border)
			if it.Checked {
				cv.FillRect(render.Rect{X: x + 2, Y: row.Y + (row.H-12)/2 + 2, W: 8, H: 8}, t.Accent)
			}
			x += 18
		case ItemRadio:
			cv.FillRect(render.Rect{X: x, Y: row.Y + (row.H-12)/2, W: 12, H: 12}, t.Border)
			if it.Checked {
				cv.FillRect(render.Rect{X: x + 3, Y: row.Y + (row.H-12)/2 + 3, W: 6, H: 6}, t.Accent)
			}
			x += 18
		}
		col := t.Text
		if it.OnClick == nil && len(it.Items) == 0 {
			col = t.TextMuted
		}
		baseline := row.Y + (row.H-lineH)/2 + int(m.face.Shape("lg", m.sizePx).Ascent()+0.5)
		m.face.Draw(cv, m.face.Shape(it.Label, m.sizePx), x, baseline, col)
		if it.Accel != "" {
			aw := int(m.face.Shape(it.Accel, m.sizePx).Advance() + 0.5)
			m.face.Draw(cv, m.face.Shape(it.Accel, m.sizePx), row.X+row.W-12-aw, baseline, t.TextMuted)
		}
		if len(it.Items) > 0 {
			m.face.Draw(cv, m.face.Shape(">", m.sizePx), row.X+row.W-16, baseline, t.TextMuted)
		}
	}
}

// Role implements Roleer.
func (m *Menu) Role() Role { return RoleMenu }

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

// ClickAt activates the clicked row (or opens its submenu) and
// dismisses the menu.
func (m *Menu) ClickAt(p Point) {
	i := m.itemAt(p)
	if i < 0 {
		m.dismiss()
		return
	}
	m.hovered = i
	m.activate(i)
}

// activate runs row i's action: submenus open through OnSubmenu,
// checkboxes and radios flip with radio-group exclusivity, plain
// actions fire and dismiss, and rows with no action do nothing.
func (m *Menu) activate(i int) {
	item := m.items[i]
	switch {
	case item.Kind == ItemSeparator:
		return
	case len(item.Items) > 0:
		if m.OnSubmenu != nil {
			m.OnSubmenu(i, item.Items)
		}
		return
	case item.OnClick == nil:
		return
	case item.Kind == ItemCheck:
		m.items[i].Checked = !m.items[i].Checked
	case item.Kind == ItemRadio:
		for j := range m.items {
			if m.items[j].Kind == ItemRadio && m.items[j].Group == item.Group {
				m.items[j].Checked = false
			}
		}
		m.items[i].Checked = true
	}
	m.dismiss()
	item.OnClick()
}

// step moves the hover to the next selectable row in dir without
// wrapping, stopping at the ends (pinned clamping).
func (m *Menu) step(dir int) {
	i := m.hovered
	for {
		i += dir
		if i < 0 || i >= len(m.items) {
			return
		}
		if m.selectable(i) {
			m.hovered = i
			return
		}
	}
}

// KeyAction implements KeyActionHandler: arrows move the hovered row
// (clamped at the ends - pinned), Home/End jump, Enter activates, Esc
// and Left dismiss, Right opens a submenu.
func (m *Menu) KeyAction(a KeyAction, mods Mods) {
	switch a {
	case KeyDown:
		if m.hovered < 0 {
			m.hovered = m.nextSelectable(-1, 1)
			return
		}
		m.step(1)
	case KeyUp:
		if m.hovered < 0 {
			m.hovered = m.nextSelectable(len(m.items), -1)
			return
		}
		m.step(-1)
	case KeyHome:
		m.hovered = m.nextSelectable(-1, 1)
	case KeyEnd:
		m.hovered = m.nextSelectable(len(m.items), -1)
	case KeyEnter:
		if m.hovered >= 0 {
			m.activate(m.hovered)
		}
	case KeyDismiss, KeyLeft:
		m.dismiss()
	case KeyRight:
		if m.hovered >= 0 && len(m.items[m.hovered].Items) > 0 && m.OnSubmenu != nil {
			m.OnSubmenu(m.hovered, m.items[m.hovered].Items)
		}
	}
}

// dismiss tears the menu down; the host closes its popup.
func (m *Menu) dismiss() {
	if m.OnDismiss != nil {
		m.OnDismiss()
	}
}

// DragMove keeps hover tracking during presses.
func (m *Menu) DragMove(p Point) { m.hovered = m.itemAt(p) }
