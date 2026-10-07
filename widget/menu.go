package widget

import (
	"strings"
	"unicode"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/internal/logutil"
	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/internal/text"
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
	// ItemHeader is a group's caption: muted text, never hovered,
	// stepped onto, or activated.
	ItemHeader
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
//
// Icon paints at the row's leading edge, after any check or radio
// indicator, at the icon's natural size; once one row carries an icon
// every label shifts by the same slot so the column stays aligned.
// Disabled greys a row out whatever its action: it takes no hover,
// no keyboard motion, no mnemonic, and a click on it neither fires
// nor dismisses - the state an application toggles on a live row
// (a check item that cannot change right now) without dropping its
// OnClick.
type MenuItem struct {
	Label    string
	OnClick  func()
	Kind     ItemKind
	Checked  bool
	Group    string
	Accel    string
	Mnemonic rune
	Items    []MenuItem
	Icon     *Icon
	Disabled bool
}

// inert reports whether the row paints in the disabled ink: explicitly
// disabled, or with neither an action nor a submenu.
func (it MenuItem) inert() bool {
	return it.Disabled || (it.OnClick == nil && len(it.Items) == 0)
}

// MenuSeparator returns a separator item.
func MenuSeparator() MenuItem { return MenuItem{Kind: ItemSeparator} }

// Menu is a vertical action list for popups and context menus: hovered
// row highlight, pointer or keyboard activation (arrows move, Enter
// activates, Esc dismisses, Right opens a submenu), check and radio
// rows, accelerator labels, per-row Alt-letter mnemonics, and nested
// submenus through OnSubmenu.
//
// It styles as GTK's menu popup: the `popover.menu` card, a
// `contents` node padding the rows, and one `modelbutton` per row (a
// `separator` for ItemSeparator) — the hovered row :hover, inert rows
// :disabled, check and radio rows :checked while on.
type Menu struct {
	node
	face    render.Font
	sizePx  float64
	dir     Direction
	items   []MenuItem
	hovered int
	// keyed marks a highlight the keyboard made: a pointer that is
	// over no row (the popup's shadow gutter, a separator, outside)
	// does not take it away - only pointing at a row does. A
	// compositor reports such pointer events on its own when a popup
	// maps under a resting cursor, which must not undo arrow-key
	// navigation.
	keyed     bool
	itemH     int
	OnDismiss func()
	// OnSubmenu fires when a row with nested Items is activated; the
	// host opens a child popup for it beside the parent row.
	OnSubmenu func(index int, items []MenuItem)
	// OnHover fires when the pointer moves onto another row, with its
	// index: a nested menu opens the row's submenu, or closes the one
	// a sibling had open, on hover as GTK's does.
	OnHover func(index int)

	// AccelLabel, when set, formats each row's Accel for display (a
	// symbol form, a localized one); the binding itself is untouched.
	AccelLabel func(accel string) string

	// contents pads the rows (`popover.menu > contents`); rows are the
	// per-item nodes under it.
	contents stylePart
	rows     []menuRow

	// mnemonics maps each row's lowercased Alt-letter to its row index;
	// mnemRunes holds the letter's rune index in the row's label (-1
	// when the row has no mnemonic). Resolved once at construction.
	mnemonics map[rune]int
	mnemRunes []int
}

// menuRow is one row node of a menu: a `modelbutton` (or a
// `separator`), hovered by the menu's model, never laid out on its
// own.
type menuRow struct {
	stylePart
	hovered bool
}

// NewMenu returns a menu showing items, painted with face at sizePx.
// Face may be a render.Chain for mixed-script fallback. A nil face
// panics here (see requireFace) instead of failing later, in shaping.
func NewMenu(face render.Font, sizePx float64, items ...MenuItem) *Menu {
	face = requireFace("widget.NewMenu", face)
	rows, runes := resolveMnemonics(items)
	m := &Menu{
		face:      face,
		sizePx:    sizePx,
		items:     items,
		hovered:   -1,
		itemH:     face.Shape("lg", sizePx).LineHeight() + 12,
		mnemonics: rows,
		mnemRunes: runes,
	}
	m.SetElement("popover")
	m.AddClass("menu")
	m.contents.SetElement("contents")
	m.buildRows()
	return m
}

// SetItems replaces the rows in place - a live model update that
// keeps the menu (and its popover) open; the hovered row stays when it
// still exists.
func (m *Menu) SetItems(items ...MenuItem) {
	m.items = items
	m.mnemonics, m.mnemRunes = resolveMnemonics(items)
	if m.hovered >= len(items) || m.hovered >= 0 && !m.selectable(m.hovered) {
		m.hovered = -1
	}
	m.buildRows()
	m.InvalidateLayout()
}

// accelText is a row's accelerator as displayed.
func (m *Menu) accelText(it MenuItem) string {
	if it.Accel == "" || m.AccelLabel == nil {
		return it.Accel
	}
	return m.AccelLabel(it.Accel)
}

// buildRows creates the row nodes over the current items and links
// them below the contents.
func (m *Menu) buildRows() {
	m.rows = make([]menuRow, len(m.items))
	kids := make([]Widget, len(m.items))
	for i := range m.items {
		if m.items[i].Kind == ItemSeparator {
			m.rows[i].SetElement("separator")
		} else {
			m.rows[i].SetElement("modelbutton")
		}
		kids[i] = &m.rows[i]
	}
	setParents(m, &m.contents)
	setParents(&m.contents, kids...)
	m.syncRowStates()
	m.syncHovered()
}

// syncRowStates mirrors the items into the row nodes: inert rows are
// :disabled, check and radio rows :checked while on.
func (m *Menu) syncRowStates() {
	for i := range m.items {
		it := &m.items[i]
		m.rows[i].SetEnabled(!it.inert())
		m.rows[i].SetState(StateChecked, (it.Kind == ItemCheck || it.Kind == ItemRadio) && it.Checked)
	}
}

// syncHovered mirrors the hovered row index into the row nodes'
// :hover.
func (m *Menu) syncHovered() {
	for i := range m.rows {
		on := i == m.hovered && m.selectable(i)
		if m.rows[i].hovered != on {
			m.rows[i].hovered = on
			m.rows[i].invalidateState(style.Hover)
		}
	}
}

// menuDebug is the mnemonic-conflict sink: conflicts route to the
// injected library logger at Debug level (the rows still render; only
// the duplicate shortcut is dropped). Tests swap it to capture.
var menuDebug = func(msg string) { logutil.L().Debug(msg) }

// SetDirection selects the base paragraph direction the row labels
// resolve and lay out with: DirectionAuto (the default) reads it off
// each label's first strong character, and a right-to-left label hugs
// the row's right edge. Changing the direction invalidates.
func (m *Menu) SetDirection(d Direction) {
	if m.dir == d {
		return
	}
	m.dir = d
	m.Invalidate()
}

// Direction returns the base paragraph direction the labels resolve
// with.
func (m *Menu) Direction() Direction { return m.dir }

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
		if it.Mnemonic == 0 || it.Kind == ItemSeparator || it.Kind == ItemHeader || it.Disabled {
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
		if runes[i] >= 0 || explicit[i] || it.Kind == ItemSeparator || it.Kind == ItemHeader || it.Disabled || it.Label == "" {
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

// Measure wants the widest row (label plus indicator, accelerator,
// and submenu arrow) plus the row and contents padding, and one band
// per item — the bands sized by the styled modelbuttons, as the list
// sizes rows.
func (m *Menu) Measure(con Constraints) Size {
	if sz, ok := m.measureHit(con); ok {
		return sz
	}
	w := 0
	slot := m.iconSlot()
	for i, it := range m.items {
		rv := m.rows[i].style(&m.rows[i])
		pad := boxOf(rv, menuRowPad).outer()
		row := m.face.Shape(it.Label, m.sizePx).Advance() + float64(slot)
		if it.Kind == ItemCheck || it.Kind == ItemRadio {
			row += 18
		}
		if acc := m.accelText(it); acc != "" {
			row += m.face.Shape(acc, m.sizePx).Advance() + 16
		}
		if len(it.Items) > 0 {
			row += 16
		}
		if adv := int(row+0.5) + pad.Left + pad.Right; adv > w {
			w = adv
		}
	}
	cv := m.contents.style(&m.contents)
	co := boxOf(cv, menuContentsPad).outer()
	v := m.style(m)
	return m.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		h := len(m.items)*m.stride() + co.Top + co.Bottom
		return clampSize(Size{W: w + co.Left + co.Right, H: h}, inner)
	}))
}

// menuContentsPad is the unstyled row inset each side; menuRowPad the
// unstyled label inset inside a row.
var (
	menuContentsPad = render.UniformInsets(4)
	menuRowPad      = render.Insets{Right: 8, Left: 8}
)

// bandH is the laid-out row band: the styled modelbuttons' floor
// (min-height plus vertical padding) over the theme's line band.
func (m *Menu) bandH() int {
	h := m.itemH - 2
	for i := range m.rows {
		rv := m.rows[i].style(&m.rows[i])
		pad := boxOf(rv, menuRowPad).outer()
		h = max(h, picki(rv, style.PropMinHeight, 0)+pad.Top+pad.Bottom)
	}
	return h
}

// iconSlot is the width every row reserves for icons: the widest
// row icon's natural width plus a gap, or zero when no row has one.
func (m *Menu) iconSlot() int {
	w := 0
	for _, it := range m.items {
		if it.Icon != nil {
			w = max(w, it.Icon.Measure(Constraints{Max: Size{W: m.itemH * 4, H: m.itemH}}).W)
		}
	}
	if w == 0 {
		return 0
	}
	return w + 8
}

// selectable reports whether keyboard motion may land on row i.
func (m *Menu) selectable(i int) bool {
	return i >= 0 && i < len(m.items) && m.items[i].Kind != ItemSeparator && m.items[i].Kind != ItemHeader && !m.items[i].Disabled
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

// Paint draws the menu through its nodes — the `popover.menu` card,
// the `contents` inset, and each row (the hovered one :hover, its
// background and label color from the cascade), with item indicators,
// accelerator labels, and submenu arrows. Labels resolve under the
// menu's base direction: a right-to-left label shapes reordered and
// hugs the row's right edge; the check/radio indicators, accelerators,
// and submenu arrows keep their positions.
func (m *Menu) Paint(cv *render.Canvas) {
	t := Current()
	v := m.style(m)
	fx := pushEffects(cv, v)
	radii := radiusOr(v, t.Radius)
	paintBoxBehind(cv, v, m.bounds, radii, borderOf(v), pickc(0, v, style.PropBackgroundColor, t.Surface))
	paintOutline(cv, v, m.bounds, radii)
	fx.pop(cv)

	cw := m.contents.style(&m.contents)
	cfx := pushEffects(cv, cw)
	_, rowsRect := boxRects(boxOf(cw, menuContentsPad), m.bounds)
	m.contents.Arrange(rowsRect)
	cradii := radiusOr(cw, 0)
	paintBoxBehind(cv, cw, rowsRect, cradii, borderOf(cw), pickc(0, cw, style.PropBackgroundColor, 0))
	cfx.pop(cv)

	lineH := m.face.Shape("lg", m.sizePx).LineHeight()
	slot := m.iconSlot()
	band, stride := m.bandH(), m.bandH()+2
	for i, it := range m.items {
		rp := &m.rows[i]
		rv := rp.style(rp)
		pad := boxOf(rv, menuRowPad)
		_, rowContent := boxRects(pad, render.Rect{
			X: rowsRect.X,
			Y: rowsRect.Y + i*stride,
			W: rowsRect.W,
			H: band,
		})
		row := pad.margin.Shrink(render.Rect{X: rowsRect.X, Y: rowsRect.Y + i*stride, W: rowsRect.W, H: band})
		rp.Arrange(row)
		rfx := pushEffects(cv, rv)
		rradii := radiusOr(rv, t.Radius)
		var hoverBg render.Color
		if i == m.hovered && m.selectable(i) {
			hoverBg = t.HoverAccent()
		}
		paintBoxBehind(cv, rv, row, rradii, borderOf(rv), pickc(0, rv, style.PropBackgroundColor, hoverBg))
		rfx.pop(cv)
		if it.Kind == ItemSeparator {
			y := row.Y + row.H/2
			cv.Line(row.X+8, y, row.X+row.W-8, y, 1, t.Border)
			continue
		}
		x := rowContent.X
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
		if it.Icon != nil {
			// The icon hangs off the menu for invalidation (a theme
			// switch re-resolves it) and paints centered in its slot.
			setParents(m, it.Icon)
			sz := it.Icon.Measure(Constraints{Max: Size{W: slot, H: row.H}})
			it.Icon.Arrange(render.Rect{X: x, Y: row.Y + (row.H-sz.H)/2, W: sz.W, H: sz.H})
			PaintChild(cv, it.Icon)
		}
		x += slot
		col := t.Text
		switch {
		case it.Kind == ItemHeader:
			col = t.DisabledText() // the muted ink (TextMuted when set)
		case it.inert() || !IsEnabled(m):
			col = t.DisabledText()
		}
		col = pickc(0, rv, style.PropColor, col)
		baseline := row.Y + (row.H-lineH)/2 + int(m.face.Shape("lg", m.sizePx).Ascent()+0.5)
		sh := m.face.ShapeDir(it.Label, m.sizePx, m.dir)
		if text.RTL(it.Label, m.dir) {
			// A right-to-left label reads from the row's right edge;
			// keep clear of the accelerator and submenu arrow.
			x = rowContent.X + rowContent.W - 4 - int(sh.Advance()+0.5)
			if acc := m.accelText(it); acc != "" {
				x -= int(m.face.Shape(acc, m.sizePx).Advance()+0.5) + 16
			}
			if len(it.Items) > 0 {
				x -= 16
			}
		}
		m.face.Draw(cv, sh, x, baseline, col)
		if ri := m.mnemRunes[i]; ri >= 0 {
			// The mnemonic underline: a strip under the claimed letter,
			// the same 2px underline idiom the composing and link ranges
			// use.
			x0 := x + int(sh.CaretX(ri)+0.5)
			x1 := x + int(sh.CaretX(ri+1)+0.5)
			cv.FillRect(render.Rect{X: x0, Y: baseline + 2, W: max(x1-x0, 1), H: 2}, col)
		}
		if acc := m.accelText(it); acc != "" {
			sh := m.face.Shape(acc, m.sizePx)
			aw := int(sh.Advance() + 0.5)
			m.face.Draw(cv, sh, rowContent.X+rowContent.W-4-aw, baseline, t.TextMuted)
		}
		if len(it.Items) > 0 {
			m.face.Draw(cv, m.face.Shape(">", m.sizePx), rowContent.X+rowContent.W-12, baseline, t.TextMuted)
		}
	}
}

// Role implements Roleer.
func (m *Menu) Role() Role { return RoleMenu }

// HitTest returns the menu when p is inside its bounds.
func (m *Menu) HitTest(p Point) Widget { return m.HitLeaf(m, p) }

// SetHovered clears row hover when the pointer leaves the menu.
func (m *Menu) SetHovered(on bool) {
	if !on && m.hovered != -1 && !m.keyed {
		m.hovered = -1
		m.invalidateState(style.Hover)
		m.syncHovered()
	}
}

// HoverMove tracks the hovered row as the pointer moves inside.
// Disabled menus highlight nothing.
func (m *Menu) HoverMove(p Point) {
	if !IsEnabled(m) {
		return
	}
	i := m.itemAt(p)
	if i < 0 && m.keyed {
		return
	}
	if i >= 0 {
		m.keyed = false
	}
	if i != m.hovered {
		m.moveHover(i)
		if i >= 0 && m.OnHover != nil {
			m.OnHover(i)
		}
	}
}

// RowBounds is row i's rect in the menu's coordinates: where a
// submenu anchors beside it. Empty for an index out of range.
func (m *Menu) RowBounds(i int) render.Rect {
	if i < 0 || i >= len(m.items) {
		return render.Rect{}
	}
	in := m.contentsInset()
	return render.Rect{X: m.bounds.X, Y: m.bounds.Y + in.Top + i*m.stride(), W: m.bounds.W, H: m.bandH()}
}

// Items returns the menu's rows.
func (m *Menu) Items() []MenuItem { return m.items }

// contentsInset is the row area's inset inside the card: the contents
// node's box (the 4px pad unstyled).
func (m *Menu) contentsInset() render.Insets {
	cw := m.contents.style(&m.contents)
	return boxOf(cw, menuContentsPad).outer()
}

// stride is the row stride: a band plus the 2px gap.
func (m *Menu) stride() int { return m.bandH() + 2 }

// itemAt maps a root-space point to an item index, -1 outside.
func (m *Menu) itemAt(p Point) int {
	if p.X < m.bounds.X || p.X >= m.bounds.X+m.bounds.W {
		return -1
	}
	i := (p.Y - m.bounds.Y - m.contentsInset().Top) / m.stride()
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
	m.moveHover(i)
	m.activate(i)
}

// moveHover moves the highlight to row i and mirrors it into the row
// nodes' :hover.
func (m *Menu) moveHover(i int) {
	if i == m.hovered {
		return
	}
	m.hovered = i
	m.syncHovered()
	m.Invalidate()
}

// activate runs row i's action: submenus open through OnSubmenu,
// checkboxes and radios flip with radio-group exclusivity, plain
// actions fire and dismiss, and rows with no action do nothing.
func (m *Menu) activate(i int) {
	item := m.items[i]
	switch {
	case item.Kind == ItemSeparator, item.Disabled:
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
		m.syncRowStates()
		m.Invalidate()
	case item.Kind == ItemRadio:
		for j := range m.items {
			if m.items[j].Kind == ItemRadio && m.items[j].Group == item.Group {
				m.items[j].Checked = false
			}
		}
		m.items[i].Checked = true
		m.syncRowStates()
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
			m.moveHover(i)
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
	case KeyDown, KeyUp, KeyHome, KeyEnd:
		m.keyed = true
	}
	switch a {
	case KeyDown:
		if m.hovered < 0 {
			m.moveHover(m.nextSelectable(-1, 1))
			return
		}
		m.step(1)
	case KeyUp:
		if m.hovered < 0 {
			m.moveHover(m.nextSelectable(len(m.items), -1))
			return
		}
		m.step(-1)
	case KeyHome:
		m.moveHover(m.nextSelectable(-1, 1))
	case KeyEnd:
		m.moveHover(m.nextSelectable(len(m.items), -1))
	case KeyEnter:
		if m.hovered >= 0 {
			m.activate(m.hovered)
		}
	case KeyDismiss, KeyLeft:
		m.dismiss()
	case KeyRight:
		if m.selectable(m.hovered) && len(m.items[m.hovered].Items) > 0 && m.OnSubmenu != nil {
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
	m.moveHover(i)
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
	m.moveHover(m.itemAt(p))
}

// dropMnemonics clears every row's Alt-letter and its underline.
func (m *Menu) dropMnemonics() {
	m.mnemonics = nil
	for i := range m.mnemRunes {
		m.mnemRunes[i] = -1
	}
}

// styleChildren is the contents, its row nodes, and the rows' icons
// (styleKids); the icons hang off the menu for theme-swap
// invalidation.
func (m *Menu) styleChildren() []Widget {
	out := make([]Widget, 0, len(m.rows)+1)
	out = append(out, &m.contents)
	for i := range m.rows {
		out = append(out, &m.rows[i])
	}
	for _, it := range m.items {
		if it.Icon != nil {
			out = append(out, it.Icon)
		}
	}
	return out
}
