package widget

import (
	"github.com/stubbedev/gelm/render"
)

// dropdownMaxListH caps the open item list's height; taller lists clip
// (the app caps popovers at the same 600px).
const dropdownMaxListH = 600

// Dropdown is a combobox: a closed face showing the current selection
// that expands into a themed list of items beneath it.
//
// The list is an inline expansion, not an app.Popover: the widget
// package cannot import app (the dependency points the other way), and
// a self-contained popup keeps the dropdown usable in plain widget
// trees, in tests, and under hosts with no popover plumbing. While
// open the list is a real child (Children) arranged just below the
// face, so hit testing, damage, and focus traversal all see it; hosts
// that want true popup semantics can embed the dropdown in a popover
// of their own. The trade-off: nothing closes it from outside its own
// subtree — click-away dismissal needs the popover grab, so it closes
// on a selection, Esc, or a second click on the face.
//
// Keyboard: Enter, Space, or Down opens while focused, arrows move the
// highlighted row (Home/End jump), Enter picks, Esc cancels without
// changing the selection. Type-ahead is deliberately not implemented —
// menus have no mnemonics either, and the first character of type-ahead
// would collide with Space-to-open in the text router.
//
// NewDropdown takes no face because hosts usually build widgets before
// fonts settle; call SetFace right after construction. Without a face
// the face still paints (fixed size) and simply never opens — there is
// nothing to shape the rows with.
type Dropdown struct {
	node
	face     *render.Typeface
	sizePx   float64
	items    []string
	selected int

	menu    *Menu
	open    bool
	hovered bool
	enabled bool

	// OnSelect fires exactly once per selection change, whatever
	// produced it: a click, Enter, or SetSelected. Re-picking the
	// selected row is not a change and fires nothing.
	OnSelect func(i int)
}

// NewDropdown returns a dropdown showing items with the selected index
// clamped into range (-1 when there are no items).
func NewDropdown(items []string, selected int) *Dropdown {
	sel := -1
	if len(items) > 0 {
		sel = min(max(selected, 0), len(items)-1)
	}
	return &Dropdown{items: items, selected: sel, enabled: true}
}

// SetFace sets the typeface and pixel size the closed face and the
// item rows shape with. Call it once, right after construction, before
// the first open: the item list builds lazily from the face and is not
// rebuilt afterwards.
func (d *Dropdown) SetFace(face *render.Typeface, sizePx float64) {
	d.face = face
	d.sizePx = sizePx
	d.InvalidateLayout()
}

// Selected returns the selected item index, -1 when empty.
func (d *Dropdown) Selected() int { return d.selected }

// Selection returns the selected item's label, empty when none.
func (d *Dropdown) Selection() string {
	if d.selected < 0 || d.selected >= len(d.items) {
		return ""
	}
	return d.items[d.selected]
}

// SetSelected moves the selection to i (clamped) and fires OnSelect
// when the index changed.
func (d *Dropdown) SetSelected(i int) { d.selectIndex(i) }

// selectIndex clamps i, repaints, and fires OnSelect once on change.
func (d *Dropdown) selectIndex(i int) {
	if len(d.items) == 0 {
		return
	}
	i = min(max(i, 0), len(d.items)-1)
	if i == d.selected {
		return
	}
	d.selected = i
	d.Invalidate()
	if d.OnSelect != nil {
		d.OnSelect(i)
	}
}

// Opened reports whether the item list is showing.
func (d *Dropdown) Opened() bool { return d.open }

// SetEnabled turns the dropdown on or off. Disabling closes the list;
// a disabled dropdown ignores clicks and keys and paints muted, the
// way Menu paints rows without an action.
func (d *Dropdown) SetEnabled(enabled bool) {
	if d.enabled == enabled {
		return
	}
	d.enabled = enabled
	if !enabled {
		d.Close()
	}
	d.Invalidate()
}

// Enabled reports whether the dropdown accepts input.
func (d *Dropdown) Enabled() bool { return d.enabled }

// list lazily builds the item menu from the items; it needs the face,
// which may arrive after construction through SetFace.
func (d *Dropdown) list() *Menu {
	if d.menu != nil || d.face == nil {
		return d.menu
	}
	items := make([]MenuItem, len(d.items))
	for i, label := range d.items {
		items[i] = MenuItem{Label: label, OnClick: func() { d.selectIndex(i) }}
	}
	d.menu = NewMenu(d.face, d.sizePx, items...)
	// Menu activation dismisses before the row fires, so the list is
	// closed (and its pixels invalidated) by the time OnSelect runs.
	d.menu.OnDismiss = d.Close
	return d.menu
}

// Open shows the item list beneath the face, highlighted on the
// current selection. A disabled dropdown, an empty one, and one with
// no face stay closed.
func (d *Dropdown) Open() {
	if d.open || !d.enabled || len(d.items) == 0 {
		return
	}
	m := d.list()
	if m == nil {
		return
	}
	d.open = true
	m.hovered = d.selected
	d.arrangeList()
	m.Invalidate()
	d.Invalidate()
}

// Close hides the item list without touching the selection; the list's
// pixels are invalidated as an extra rect, because the damage walk no
// longer descends into the unexposed child.
func (d *Dropdown) Close() {
	if !d.open {
		return
	}
	d.open = false
	d.InvalidateRect(d.menu.Bounds())
	d.Invalidate()
}

// arrangeList places the item menu directly below the face, at least
// as wide as the face and capped in height.
func (d *Dropdown) arrangeList() {
	m := d.menu
	nat := m.Measure(Constraints{Max: Size{W: 1 << 24, H: dropdownMaxListH}})
	m.Arrange(render.Rect{
		X: d.bounds.X,
		Y: d.bounds.Y + d.bounds.H,
		W: max(d.bounds.W, nat.W),
		H: nat.H,
	})
	setParents(d, m)
}

// Measure wants the widest item plus arrow and padding, and one row
// height; the closed face keeps its width whatever the selection, so a
// change never relayouts the surrounding tree. Without a face it wants
// a fixed 80x28 placeholder.
func (d *Dropdown) Measure(con Constraints) Size {
	if sz, ok := d.measureHit(con); ok {
		return sz
	}
	w, h := 80, 28
	if d.face != nil {
		h = d.face.Shape("lg", d.sizePx).LineHeight() + 12
		for _, label := range d.items {
			if adv := int(d.face.Shape(label, d.sizePx).Advance()+0.5) + 14 + 24; adv > w {
				w = adv
			}
		}
	}
	return d.measureStore(con, clampSize(Size{W: w, H: h}, con))
}

// Arrange records the face's rect and, while open, keeps the list
// pinned beneath it.
func (d *Dropdown) Arrange(r render.Rect) {
	d.node.Arrange(r)
	if d.open {
		d.arrangeList()
	}
}

// Paint draws the closed face — surface, selection, chevron — and, on
// top of the frame beneath it, the open item list.
func (d *Dropdown) Paint(cv *render.Canvas) {
	t := Current()
	bg := t.Surface
	switch {
	case d.open:
		bg = t.SurfacePressed
	case d.hovered:
		bg = t.SurfaceHover
	}
	cv.RoundedRect(d.bounds, t.Radius, bg)

	col := t.Text
	if !d.enabled {
		col = t.TextMuted
	}
	if d.face != nil {
		lineH := d.face.Shape("lg", d.sizePx).LineHeight()
		baseline := d.bounds.Y + (d.bounds.H-lineH)/2 + int(d.face.Shape("lg", d.sizePx).Ascent()+0.5)
		d.face.Draw(cv, d.face.Shape(d.Selection(), d.sizePx), d.bounds.X+8, baseline, col)
	}
	// The chevron: two strokes forming a v at the face's right edge.
	cx, cy := d.bounds.X+d.bounds.W-16, d.bounds.Y+d.bounds.H/2
	cv.Line(cx-4, cy-2, cx, cy+2, 1, col)
	cv.Line(cx, cy+2, cx+4, cy-2, 1, col)

	if d.open {
		d.menu.Paint(cv)
	}
}

// Role implements Roleer.
func (d *Dropdown) Role() Role { return RoleComboBox }

// HitTest returns the item list while a point is inside it — the list
// hangs outside the face's bounds — else the dropdown inside its own.
func (d *Dropdown) HitTest(p Point) Widget {
	if d.open {
		if hit := d.menu.HitTest(p); hit != nil {
			return hit
		}
	}
	return d.HitLeaf(d, p)
}

// Children exposes the open item list for focus traversal (and the
// damage walk, which shares the interface); closed, there is nothing
// to traverse into.
func (d *Dropdown) Children() []Widget {
	if d.open {
		return []Widget{d.menu}
	}
	return nil
}

// ClickAt toggles the list; the Router invokes it when a press and
// release land on the face. Presses on the list go to the list.
func (d *Dropdown) ClickAt(Point) {
	if !d.enabled {
		return
	}
	if d.open {
		d.Close()
		return
	}
	d.Open()
}

// SetHovered implements HoverSetter; the face's hover shade repaints.
func (d *Dropdown) SetHovered(on bool) {
	if d.hovered == on {
		return
	}
	d.hovered = on
	d.Invalidate()
}

// KeyAction implements KeyActionHandler: closed, Enter or Down opens;
// open, everything but Esc goes to the item list, so arrows move the
// highlight and Enter picks. Esc closes without changing anything.
func (d *Dropdown) KeyAction(a KeyAction, _ Mods) {
	if !d.enabled {
		return
	}
	if d.open {
		if a == KeyDismiss {
			d.Close()
			return
		}
		d.menu.KeyAction(a, 0)
		return
	}
	switch a {
	case KeyEnter, KeyDown:
		d.Open()
	}
}

// InsertRune implements RuneHandler: Space opens, the other half of
// the activation pair (Enter is the first). Open, Space behaves like
// Enter and activates the highlighted row.
func (d *Dropdown) InsertRune(r rune) {
	if r == ' ' {
		d.KeyAction(KeyEnter, 0)
	}
}

// DropdownItem pairs a display label with the typed value picking it
// reports.
type DropdownItem[T any] struct {
	Label string
	Value T
}

// DropdownOf is a Dropdown whose rows carry typed values: the labels
// feed the visible list, and a change comes back with the value behind
// the picked row. It is a Dropdown in every other respect — embed it in
// a tree as-is.
type DropdownOf[T any] struct {
	*Dropdown
	values []T

	// OnSelect fires exactly once per selection change with the new
	// row's index and value. It shadows the wrapped Dropdown's OnSelect
	// (a func(int)); set this one.
	OnSelect func(i int, value T)
}

// NewDropdownOf builds a typed dropdown from labeled values, showing
// the selected index clamped into range.
func NewDropdownOf[T any](items []DropdownItem[T], selected int) *DropdownOf[T] {
	labels := make([]string, len(items))
	values := make([]T, len(items))
	for i, it := range items {
		labels[i], values[i] = it.Label, it.Value
	}
	d := &DropdownOf[T]{Dropdown: NewDropdown(labels, selected), values: values}
	d.Dropdown.OnSelect = func(i int) {
		if d.OnSelect != nil && i >= 0 && i < len(d.values) {
			d.OnSelect(i, d.values[i])
		}
	}
	return d
}

// Value returns the picked row's value, the zero T when nothing is
// selected.
func (d *DropdownOf[T]) Value() T {
	var zero T
	i := d.Selected()
	if i < 0 || i >= len(d.values) {
		return zero
	}
	return d.values[i]
}
