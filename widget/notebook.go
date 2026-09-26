package widget

import (
	"github.com/stubbedev/gelm/render"
)

// Notebook is a tabbed container: a painted tab bar on top and exactly
// one visible page beneath it, the GtkNotebook model. Tabs carry a
// name; selecting one shows its page and fires OnSelect. With Closable
// set, each tab paints a close box and a click on it fires OnTabClose
// instead of selecting - hosts remove the page themselves.
type Notebook struct {
	node
	face     *render.Typeface
	tabs     []notebookTab
	selected int

	OnSelect   func(name string)
	OnTabClose func(name string)
	Closable   bool
}

type notebookTab struct {
	name string
	w    Widget
}

// tabBarHeight is the reserved strip for the tab row.
const tabBarHeight = 26

// tabWidth is the fixed strip width per tab; a close box lives at its
// right edge when Closable is set.
const tabWidth = 96

// NewNotebook returns an empty notebook with tab labels painted in the
// given face.
func NewNotebook(face *render.Typeface) *Notebook { return &Notebook{face: face} }

// AppendTab adds a page under name; the first page added becomes the
// selected one.
func (n *Notebook) AppendTab(name string, w Widget) {
	n.tabs = append(n.tabs, notebookTab{name: name, w: w})
}

// SelectedTab returns the visible page's name, empty when none.
func (n *Notebook) SelectedTab() string {
	if n.selected < len(n.tabs) {
		return n.tabs[n.selected].name
	}
	return ""
}

// SelectTab shows the page under name; unknown names are a no-op.
func (n *Notebook) SelectTab(name string) {
	for i, t := range n.tabs {
		if t.name == name {
			n.selectIndex(i)
			return
		}
	}
}

// selectIndex switches pages and fires OnSelect.
func (n *Notebook) selectIndex(i int) {
	if i == n.selected {
		return
	}
	n.selected = i
	if n.OnSelect != nil {
		n.OnSelect(n.tabs[i].name)
	}
}

// CloseTab removes the page under name; the following page becomes
// visible. Unknown names are a no-op. Reports whether a page was
// removed.
func (n *Notebook) CloseTab(name string) bool {
	for i, t := range n.tabs {
		if t.name != name {
			continue
		}
		n.tabs = append(n.tabs[:i], n.tabs[i+1:]...)
		if n.selected >= len(n.tabs) {
			n.selected = len(n.tabs) - 1
		}
		if n.selected < 0 {
			n.selected = 0
		}
		return true
	}
	return false
}

// Children exposes only the visible page, so keyboard traversal skips
// hidden pages.
func (n *Notebook) Children() []Widget {
	if n.selected < len(n.tabs) {
		return []Widget{n.tabs[n.selected].w}
	}
	return nil
}

// Measure reports the tab bar height plus the largest page.
func (n *Notebook) Measure(con Constraints) Size {
	best := Size{}
	for _, t := range n.tabs {
		s := t.w.Measure(con)
		best.W = max(best.W, s.W)
		best.H = max(best.H, s.H)
	}
	best.H += tabBarHeight
	return clampSize(best, con)
}

// Arrange lays out the tab strip and the visible page beneath it.
func (n *Notebook) Arrange(r render.Rect) {
	n.node.Arrange(r)
	page := render.Rect{X: r.X, Y: r.Y + tabBarHeight, W: r.W, H: max(0, r.H-tabBarHeight)}
	if n.selected < len(n.tabs) {
		n.tabs[n.selected].w.Arrange(page)
		setParents(n, n.tabs[n.selected].w)
	}
}

// tabRect returns the strip occupied by tab i.
func (n *Notebook) tabRect(i int) render.Rect {
	x := n.bounds.X
	for range i {
		x += tabWidth
	}
	return render.Rect{X: x, Y: n.bounds.Y, W: tabWidth, H: tabBarHeight}
}

// Paint draws the tab bar - the selected tab is distinct through the
// accent underline and full text color - and the visible page.
func (n *Notebook) Paint(cv *render.Canvas) {
	th := Current()
	cv.RoundedRect(n.bounds, th.Radius, th.Surface)

	for i, t := range n.tabs {
		rect := n.tabRect(i)
		if i == n.selected {
			cv.FillRect(rect, th.SurfaceHover)
			cv.FillRect(render.Rect{X: rect.X, Y: rect.Y + tabBarHeight - 3, W: rect.W, H: 3}, th.Accent)
		}
		if n.face != nil {
			color := th.TextMuted
			if i == n.selected {
				color = th.Text
			}
			n.face.DrawAligned(cv, t.name, rect, 12, color, render.AlignCenter)
		}
		if n.Closable {
			cx, cy := rect.X+rect.W-13, rect.Y+tabBarHeight/2
			cv.Line(cx-4, cy-4, cx+4, cy+4, 1, th.TextMuted)
			cv.Line(cx-4, cy+4, cx+4, cy-4, 1, th.TextMuted)
		}
	}
	if n.selected < len(n.tabs) {
		prev := cv.PushClip(render.Rect{
			X: n.bounds.X, Y: n.bounds.Y + tabBarHeight,
			W: n.bounds.W, H: max(0, n.bounds.H-tabBarHeight),
		})
		n.tabs[n.selected].w.Paint(cv)
		cv.PopClip(prev)
	}
}

// Role implements Roleer.
func (n *Notebook) Role() Role { return RoleTabList }

// HitTest resolves the tab strip into the notebook itself (clicks
// select or close) and otherwise delegates to the visible page.
func (n *Notebook) HitTest(p Point) Widget {
	if !n.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if p.Y < n.bounds.Y+tabBarHeight {
		return n
	}
	if n.selected < len(n.tabs) {
		if hit := n.tabs[n.selected].w.HitTest(p); hit != nil {
			return hit
		}
	}
	return n
}

// ClickAt selects the tab under the point, or fires OnTabClose when
// the close box of a tab was pressed.
func (n *Notebook) ClickAt(p Point) {
	for i := range n.tabs {
		rect := n.tabRect(i)
		if !rect.Contains(p.X, p.Y) {
			continue
		}
		if n.Closable && p.X > rect.X+rect.W-20 {
			if n.OnTabClose != nil {
				n.OnTabClose(n.tabs[i].name)
			}
			return
		}
		n.selectIndex(i)
		return
	}
}

// KeyAction implements KeyActionHandler: ctrl+PageUp/PageDown cycle
// through tabs. Other actions are ignored.
func (n *Notebook) KeyAction(a KeyAction, mods Mods) {
	if mods&ModCtrl == 0 || len(n.tabs) == 0 {
		return
	}
	switch a {
	case KeyPriorPage:
		n.selectIndex((n.selected - 1 + len(n.tabs)) % len(n.tabs))
	case KeyNextPage:
		n.selectIndex((n.selected + 1) % len(n.tabs))
	}
}
