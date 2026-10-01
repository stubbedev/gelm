package app

import (
	"errors"

	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// MenuPopoverConfig declares a menu tree opened as nested popovers.
type MenuPopoverConfig struct {
	// Anchor is the widget the root menu opens beside.
	Anchor widget.Boundser
	// Gravity picks the root menu's side; submenus open to the right
	// of their row (the compositor flips them left when there is no
	// room).
	Gravity Gravity
	// Face and SizePx paint every level.
	Face   render.Font
	SizePx float64
	// Items is the root level; a row with Items opens a submenu.
	Items []widget.MenuItem
	// Serial is the opening event's serial, for the root's grab.
	Serial uint32
	// OnClosed runs once when the whole menu goes away.
	OnClosed func()
}

// MenuPopover is a menu tree presented as nested popovers: GTK's
// PopoverMenu with the NESTED flag, where each submenu is its own
// popover beside its row instead of sliding in place (MenuStack). A
// submenu opens when its row is hovered, clicked, or entered with
// Right or Enter; hovering a sibling row closes it; Esc or Left closes
// the innermost level only; a leaf row closes the whole chain before
// its action runs.
type MenuPopover struct {
	cfg    MenuPopoverConfig
	open   menuOpener
	frame  *menuFrame
	levels []*menuLevel
}

// menuOpener opens one level's popover: the root under the host, a
// submenu nested under its parent (parent nil for the root).
type menuOpener func(parent *Popover, anchor widget.Boundser, content widget.Widget, onClosed func()) (*Popover, error)

// menuLevel is one open menu and its popover; openRow is the row whose
// submenu is open beside it, -1 for none.
type menuLevel struct {
	menu    *widget.Menu
	pop     *Popover
	openRow int
}

// OpenMenuPopover opens the root level over host.
func (a *Application) OpenMenuPopover(host Host, cfg MenuPopoverConfig) (*MenuPopover, error) {
	return openMenuPopover(cfg, func(parent *Popover, anchor widget.Boundser, content widget.Widget, onClosed func()) (*Popover, error) {
		pc := PopoverConfig{Anchor: anchor, Content: content, OnClosed: onClosed, Parent: parent}
		if parent == nil {
			pc.Gravity, pc.Serial = cfg.Gravity, cfg.Serial
		} else {
			pc.Gravity = GravityRight
		}
		return a.OpenPopover(host, pc)
	})
}

func openMenuPopover(cfg MenuPopoverConfig, open menuOpener) (*MenuPopover, error) {
	if cfg.Anchor == nil || cfg.Face == nil {
		return nil, errors.New("app: a menu popover needs an anchor and a face")
	}
	m := &MenuPopover{cfg: cfg, open: open}
	root := m.newLevel(0, cfg.Items)
	// The root menu sits in a frame so a live update can swap it.
	m.frame = &menuFrame{Box: widget.NewBox(widget.Column, 0, 0), m: m}
	m.frame.Append(root.menu, true)
	pop, err := open(nil, cfg.Anchor, m.frame, func() {
		m.levels = nil
		if cfg.OnClosed != nil {
			cfg.OnClosed()
		}
	})
	if err != nil {
		return nil, err
	}
	root.pop = pop
	m.levels = []*menuLevel{root}
	return m, nil
}

// newLevel builds the menu for one level; depth is its index in the
// chain.
func (m *MenuPopover) newLevel(depth int, items []widget.MenuItem) *menuLevel {
	rows := make([]widget.MenuItem, len(items))
	for i, it := range items {
		if action := it.OnClick; action != nil && len(it.Items) == 0 {
			// A leaf closes the whole chain, then acts.
			it.OnClick = func() {
				m.Dismiss()
				action()
			}
		}
		rows[i] = it
	}
	lvl := &menuLevel{menu: widget.NewMenu(m.cfg.Face, m.cfg.SizePx, rows...), openRow: -1}
	lvl.menu.OnDismiss = func() {
		if lvl.pop != nil {
			lvl.pop.Dismiss()
		}
	}
	lvl.menu.OnSubmenu = func(row int, sub []widget.MenuItem) { m.openSubmenu(depth, row, sub) }
	lvl.menu.OnHover = func(row int) {
		if sub := lvl.menu.Items()[row].Items; len(sub) > 0 && !lvl.menu.Items()[row].Disabled {
			m.openSubmenu(depth, row, sub)
			return
		}
		m.closeBelow(depth)
	}
	return lvl
}

// rowAnchor anchors a submenu to its row.
type rowAnchor struct{ r render.Rect }

func (a rowAnchor) Bounds() render.Rect { return a.r }

// openSubmenu opens row's submenu beside it at depth+1, closing what
// was open deeper; the row whose submenu is already open stays as is.
func (m *MenuPopover) openSubmenu(depth, row int, items []widget.MenuItem) {
	if depth >= len(m.levels) {
		return
	}
	parent := m.levels[depth]
	if parent.openRow == row && depth+1 < len(m.levels) {
		return
	}
	m.closeBelow(depth)
	lvl := m.newLevel(depth+1, items)
	pop, err := m.open(parent.pop, rowAnchor{parent.menu.RowBounds(row)}, lvl.menu, func() {
		// Closed on its own (Esc, Left) or with its parent: the chain
		// ends above it.
		if depth+1 < len(m.levels) && m.levels[depth+1] == lvl {
			m.levels = m.levels[:depth+1]
			parent.openRow = -1
		}
	})
	if err != nil {
		return
	}
	lvl.pop = pop
	parent.openRow = row
	m.levels = append(m.levels, lvl)
}

// closeBelow closes every level deeper than depth.
func (m *MenuPopover) closeBelow(depth int) {
	if depth >= len(m.levels) {
		return
	}
	m.levels[depth].pop.closeChildren()
	if len(m.levels) > depth+1 {
		m.levels = m.levels[:depth+1]
	}
	m.levels[depth].openRow = -1
}

// SetItems replaces the tree while it is open, closing any open
// submenu: the shape a live model update takes (a tray application
// re-publishing its menu).
func (m *MenuPopover) SetItems(items ...widget.MenuItem) {
	if m.Closed() {
		return
	}
	m.closeBelow(0)
	root := m.newLevel(0, items)
	root.pop = m.levels[0].pop
	m.levels[0] = root
	m.frame.Clear()
	m.frame.Append(root.menu, true)
}

// Depth is the number of open levels: 1 with no submenu open, 0 once
// closed.
func (m *MenuPopover) Depth() int { return len(m.levels) }

// Level returns the open menu at depth (0 is the root), nil past the
// chain.
func (m *MenuPopover) Level(depth int) *widget.Menu {
	if depth < 0 || depth >= len(m.levels) {
		return nil
	}
	return m.levels[depth].menu
}

// Dismiss closes the whole chain.
func (m *MenuPopover) Dismiss() {
	if len(m.levels) > 0 {
		m.levels[0].pop.Dismiss()
	}
}

// Closed reports whether the menu has gone away.
func (m *MenuPopover) Closed() bool { return len(m.levels) == 0 || m.levels[0].pop.Closed() }

// menuFrame holds the root menu so a live update can swap it, and
// hands it the popover's keys (arrows, Enter, Right into a submenu):
// the popover forwards key actions to its content only.
type menuFrame struct {
	*widget.Box
	m *MenuPopover
}

// KeyAction implements widget.KeyActionHandler.
func (f *menuFrame) KeyAction(a widget.KeyAction, mods widget.Mods) {
	if root := f.m.Level(0); root != nil {
		root.KeyAction(a, mods)
	}
}
