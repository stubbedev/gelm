package widget

import (
	"strings"
	"unicode"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/internal/logutil"
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
// right-aligned accelerator label, and — while the menu is open — a
// matching application accelerator fires the row's action through the
// popup key path (see RawKeyHandler). Items nests a submenu;
// activating such a row fires OnSubmenu so the host opens a child
// popup beside it.
//
// Mnemonic is the row's Alt-letter shortcut while the menu is open:
// an explicit letter (case-insensitive), or zero to auto-resolve the
// first letter of the label no earlier row took. A letter claimed by
// an earlier row wins and the later one is dropped with a Debug log;
// the underline under the resolved letter is painted for free.
type MenuItem struct {
	Label    string
	OnClick  func()
	Kind     ItemKind
	Checked  bool
	Group    string
	Accel    string
	Mnemonic rune
	Items    []MenuItem
}

// MenuSeparator returns a separator item.
func MenuSeparator() MenuItem { return MenuItem{Kind: ItemSeparator} }

// Menu is a vertical action list for popups and context menus: hovered
// row highlight, pointer or keyboard activation (arrows move, Enter
// activates, Esc dismisses, Right opens a submenu), check and radio
// rows, accelerator labels, per-row Alt-letter mnemonics, and nested
// submenus through OnSubmenu.
type Menu struct {
	node
	face      render.Font
	sizePx    float64
	items     []MenuItem
	hovered   int
	itemH     int
	OnDismiss func()
	// OnSubmenu fires when a row with nested Items is activated; the
	// host opens a child popup for it beside the parent row.
	OnSubmenu func(index int, items []MenuItem)

	// mnemonics maps each row's lowercased Alt-letter to its row index;
	// mnemRunes holds the letter's rune index in the row's label (-1
	// when the row has no mnemonic). Resolved once at construction.
	mnemonics map[rune]int
	mnemRunes []int
}

// NewMenu returns a menu showing items, painted with face at sizePx.
// Face may be a render.Chain for mixed-script fallback. A nil face
// panics here (see requireFace) instead of failing later, in shaping.
func NewMenu(face render.Font, sizePx float64, items ...MenuItem) *Menu {
	face = requireFace("widget.NewMenu", face)
	rows, runes := resolveMnemonics(items)
	return &Menu{
		face:      face,
		sizePx:    sizePx,
		items:     items,
		hovered:   -1,
		itemH:     face.Shape("lg", sizePx).LineHeight() + 12,
		mnemonics: rows,
		mnemRunes: runes,
	}
}

// menuDebug is the mnemonic-conflict sink: conflicts route to the
// injected library logger at Debug level (the rows still render; only
// the duplicate shortcut is dropped). Tests swap it to capture.
var menuDebug = func(msg string) { logutil.L().Debug(msg) }

// resolveMnemonics assigns each row its Alt-letter: explicit Mnemonic
// letters first (first declaration wins; duplicates drop with a Debug
// log and the losing row keeps no mnemonic at all), then the first
// label letter no earlier row claimed. Separators never take one.
func resolveMnemonics(items []MenuItem) (rows map[rune]int, runes []int) {
	rows = make(map[rune]int)
	runes = make([]int, len(items))
	explicit := make([]bool, len(items))
	for i := range items {
		runes[i] = -1
	}
	claim := func(letter rune, i int) {
		rows[letter] = i
		runes[i] = strings.IndexRune(strings.ToLower(items[i].Label), letter)
	}
	for i, it := range items {
		if it.Mnemonic == 0 || it.Kind == ItemSeparator {
			continue
		}
		explicit[i] = true
		letter := unicode.ToLower(it.Mnemonic)
		if _, taken := rows[letter]; taken {
			menuDebug("menu mnemonic drop: '" + string(letter) + "' on \"" + it.Label + "\" duplicates an earlier row")
			continue
		}
		claim(letter, i)
	}
	for i, it := range items {
		if runes[i] >= 0 || explicit[i] || it.Kind == ItemSeparator || it.Label == "" {
			continue
		}
		for _, r := range strings.ToLower(it.Label) {
			if _, taken := rows[r]; taken || !unicode.IsLetter(r) {
				continue
			}
			claim(r, i)
			break
		}
	}
	return rows, runes
}

// Measure wants the widest row (label plus indicator, accelerator, and
// submenu arrow) plus padding, and one row per item.
func (m *Menu) Measure(con Constraints) Size {
	if sz, ok := m.measureHit(con); ok {
		return sz
	}
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
	return m.measureStore(con, clampSize(Size{W: w + 16, H: len(m.items)*m.itemH + 8}, con))
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
			cv.RoundedRect(row, t.Radius, t.HoverAccent())
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
		if (it.OnClick == nil && len(it.Items) == 0) || !IsEnabled(m) {
			col = t.DisabledText()
		}
		baseline := row.Y + (row.H-lineH)/2 + int(m.face.Shape("lg", m.sizePx).Ascent()+0.5)
		sh := m.face.Shape(it.Label, m.sizePx)
		m.face.Draw(cv, sh, x, baseline, col)
		if ri := m.mnemRunes[i]; ri >= 0 {
			// The mnemonic underline: a strip under the claimed letter,
			// the same 2px underline idiom the composing and link ranges
			// use.
			x0 := x + int(sh.CaretX(ri)+0.5)
			x1 := x + int(sh.CaretX(ri+1)+0.5)
			cv.FillRect(render.Rect{X: x0, Y: baseline + 2, W: max(x1-x0, 1), H: 2}, col)
		}
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
	if !on && m.hovered != -1 {
		m.hovered = -1
		m.Invalidate()
	}
}

// HoverMove tracks the hovered row as the pointer moves inside.
// Disabled menus highlight nothing.
func (m *Menu) HoverMove(p Point) {
	if !IsEnabled(m) {
		return
	}
	if i := m.itemAt(p); i != m.hovered {
		m.hovered = i
		m.Invalidate()
	}
}

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
// dismisses the menu. A disabled menu neither fires nor dismisses.
func (m *Menu) ClickAt(p Point) {
	if !IsEnabled(m) {
		return
	}
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
		m.Invalidate()
	case item.Kind == ItemRadio:
		for j := range m.items {
			if m.items[j].Kind == ItemRadio && m.items[j].Group == item.Group {
				m.items[j].Checked = false
			}
		}
		m.items[i].Checked = true
		m.Invalidate()
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
			m.Invalidate()
			return
		}
	}
}

// KeyAction implements KeyActionHandler: arrows move the hovered row
// (clamped at the ends - pinned), Home/End jump, Enter activates, Esc
// and Left dismiss, Right opens a submenu. A disabled menu ignores
// keys outright.
func (m *Menu) KeyAction(a KeyAction, mods Mods) {
	if !IsEnabled(m) {
		return
	}
	switch a {
	case KeyDown:
		if m.hovered < 0 {
			m.hovered = m.nextSelectable(-1, 1)
			m.Invalidate()
			return
		}
		m.step(1)
	case KeyUp:
		if m.hovered < 0 {
			m.hovered = m.nextSelectable(len(m.items), -1)
			m.Invalidate()
			return
		}
		m.step(-1)
	case KeyHome:
		m.hovered = m.nextSelectable(-1, 1)
		m.Invalidate()
	case KeyEnd:
		m.hovered = m.nextSelectable(len(m.items), -1)
		m.Invalidate()
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

// ActivateMnemonic activates the row whose mnemonic letter matches sym
// (case-insensitive) — the Alt+letter route while the menu is open —
// and reports whether one fired. A submenu row opens its submenu;
// arrow navigation and Esc are untouched (they arrive as KeyActions,
// never through here).
func (m *Menu) ActivateMnemonic(sym xkb.Keysym) bool {
	if !IsEnabled(m) {
		return false
	}
	i, ok := m.mnemonics[unicode.ToLower(rune(sym))]
	if !ok {
		return false
	}
	m.hovered = i
	m.Invalidate()
	m.activate(i)
	return true
}

// RawKey implements RawKeyHandler: Alt+letter (no ctrl) activates the
// matching row mnemonic; every other raw key is not the menu's.
func (m *Menu) RawKey(code uint32, mods Mods, sym xkb.Keysym) bool {
	if mods&ModAlt != 0 && mods&ModCtrl == 0 {
		return m.ActivateMnemonic(sym)
	}
	return false
}

// DragMove keeps hover tracking during presses. Disabled menus
// highlight nothing.
func (m *Menu) DragMove(p Point) {
	if !IsEnabled(m) {
		return
	}
	m.hovered = m.itemAt(p)
}
