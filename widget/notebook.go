package widget

import (
	"slices"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// Notebook is a tabbed container: a painted tab bar on top and exactly
// one visible page beneath it, the GtkNotebook model. Tabs carry a
// name; selecting one shows its page and fires OnSelect. With Closable
// set, each tab paints a close box and a click on it fires OnTabClose
// instead of selecting - hosts remove the page themselves. With
// Reorderable set, dragging a tab along the strip moves it (OnReorder
// reports the new index). Tabs that overflow the strip scroll: arrows
// appear at its ends, the wheel over it scrolls, and the selected tab
// is kept in view.
type Notebook struct {
	node
	face     render.Font
	tabs     []notebookTab
	selected int
	header   notebookHeader

	// scroll is the tab strip's horizontal offset; arrows are the
	// overflow chevrons; overHeader tracks the pointer for the wheel.
	scroll     int
	arrows     [2]*Symbol
	overHeader bool
	// revealing asks the next arranged frame to bring the selected tab
	// into view (a selection before the first arrange has no strip).
	revealing bool
	// drag is a tab reorder in flight: the dragged tab's index.
	drag    dragGesture
	dragTab int

	OnSelect    func(name string)
	OnTabClose  func(name string)
	OnReorder   func(name string, index int)
	Closable    bool
	Reorderable bool
}

// tabArrowW is an overflow arrow's strip width.
const tabArrowW = 20

// notebookHeader is the tab bar's node tree (`notebook > header >
// tabs > tab`): the header strip, the tab row, one tab part per page
// (its :checked marks the selected one).
type notebookHeader struct {
	stylePart
	tabs notebookTabs
}

type notebookTabs struct {
	stylePart
	tab []stylePart
}

func (t *notebookTabs) styleChildren() []Widget {
	out := make([]Widget, len(t.tab))
	for i := range t.tab {
		out[i] = &t.tab[i]
	}
	return out
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
// given face; a render.Chain adds mixed-script fallback. A nil face
// panics here (see requireFace) instead of failing later, in shaping.
func NewNotebook(face render.Font) *Notebook {
	n := &Notebook{face: requireFace("widget.NewNotebook", face)}
	n.arrows = [2]*Symbol{NewSymbol(SymbolChevronLeft, 14), NewSymbol(SymbolChevronRight, 14)}
	n.header.SetElement("header")
	n.header.tabs.SetElement("tabs")
	n.syncTabs()
	return n
}

// syncTabs rebuilds the tab parts over the current pages and links the
// header chain below the notebook.
func (n *Notebook) syncTabs() {
	n.header.tabs.tab = make([]stylePart, len(n.tabs))
	for i := range n.header.tabs.tab {
		n.header.tabs.tab[i].SetElement("tab")
	}
	setParents(n, &n.header)
	setParents(&n.header, &n.header.tabs)
	setParents(&n.header.tabs, n.header.tabs.styleChildren()...)
	n.syncSelectedTab()
}

// syncSelectedTab mirrors the selection into the tab parts' :checked.
func (n *Notebook) syncSelectedTab() {
	for i := range n.header.tabs.tab {
		n.header.tabs.tab[i].SetState(StateChecked, i == n.selected)
	}
}

// styleChildren is the header and the visible page (styleKids).
func (n *Notebook) styleChildren() []Widget {
	out := []Widget{&n.header}
	if n.selected < len(n.tabs) {
		out = append(out, n.tabs[n.selected].w)
	}
	return out
}

// AppendTab adds a page under name; the first page added becomes the
// selected one.
func (n *Notebook) AppendTab(name string, w Widget) { n.InsertTab(len(n.tabs), name, w) }

// InsertTab adds a page under name at index i (clamped); the selected
// page stays selected.
func (n *Notebook) InsertTab(i int, name string, w Widget) {
	i = min(max(i, 0), len(n.tabs))
	if len(n.tabs) > 0 && i <= n.selected {
		n.selected++
	}
	n.tabs = slices.Insert(n.tabs, i, notebookTab{name: name, w: w})
	n.syncTabs()
	n.InvalidateLayout()
}

// stripRange is the tab strip as a scroll range: the offset into the
// tabs' total width, of which the viewport between the arrows shows.
func (n *Notebook) stripRange() ScrollRange {
	total := len(n.tabs) * tabWidth
	page := n.bounds.W
	if total > page {
		page -= 2 * tabArrowW
	}
	return ScrollRange{Offset: n.scroll, Page: max(page, 0), Total: total}
}

// tabViewport is the strip area tabs paint in: inside the arrows while
// they overflow.
func (n *Notebook) tabViewport() render.Rect {
	r := render.Rect{X: n.bounds.X, Y: n.bounds.Y, W: n.bounds.W, H: tabBarHeight}
	if n.stripRange().Overflows() {
		r.X += tabArrowW
		r.W -= 2 * tabArrowW
	}
	return r
}

// scrollStrip moves the strip to offset, clamped.
func (n *Notebook) scrollStrip(offset int) {
	offset = min(max(offset, 0), n.stripRange().Max())
	if offset != n.scroll {
		n.scroll = offset
		n.Invalidate()
	}
}

// applyReveal brings the selected tab into view once there is a strip.
func (n *Notebook) applyReveal() {
	if n.revealing && n.bounds.W > 0 && n.selected < len(n.tabs) {
		n.revealing = false
		n.revealTab(n.selected)
	}
}

// revealTab scrolls the strip so tab i is whole in view.
func (n *Notebook) revealTab(i int) {
	vp, rect := n.tabViewport(), n.tabRect(i)
	switch {
	case rect.X < vp.X:
		n.scrollStrip(n.scroll - (vp.X - rect.X))
	case rect.X+rect.W > vp.X+vp.W:
		n.scrollStrip(n.scroll + rect.X + rect.W - vp.X - vp.W)
	}
}

// MoveTab moves tab from to index to, the selection following its
// page, and fires OnReorder.
func (n *Notebook) MoveTab(from, to int) {
	if from == to || from < 0 || to < 0 || from >= len(n.tabs) || to >= len(n.tabs) {
		return
	}
	selected := n.tabs[n.selected].name
	t := n.tabs[from]
	n.tabs = append(n.tabs[:from], n.tabs[from+1:]...)
	n.tabs = append(n.tabs[:to], append([]notebookTab{t}, n.tabs[to:]...)...)
	for i, k := range n.tabs {
		if k.name == selected {
			n.selected = i
		}
	}
	n.syncSelectedTab()
	n.Invalidate()
	if n.OnReorder != nil {
		n.OnReorder(t.name, to)
	}
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

// selectIndex switches pages (revealing the tab), invalidates the
// notebook, and fires OnSelect.
func (n *Notebook) selectIndex(i int) {
	changed := i != n.selected
	n.selected = i
	n.revealing = true
	n.applyReveal()
	if !changed {
		return
	}
	n.syncSelectedTab()
	n.Invalidate()
	if n.OnSelect != nil {
		n.OnSelect(n.tabs[i].name)
	}
}

// CloseTab removes the page under name; the following page becomes
// visible. Unknown names are a no-op. Reports whether a page was
// removed. The closed page detaches like every tree mutation: parent
// link cleared, removal hook fired.
func (n *Notebook) CloseTab(name string) bool {
	for i, t := range n.tabs {
		if t.name != name {
			continue
		}
		notifyRemoved(t.w)
		n.tabs = append(n.tabs[:i], n.tabs[i+1:]...)
		if n.selected >= len(n.tabs) {
			n.selected = len(n.tabs) - 1
		}
		if n.selected < 0 {
			n.selected = 0
		}
		n.scrollStrip(n.scroll)
		clearParents(t.w)
		n.syncTabs()
		n.InvalidateLayout()
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

// appendChildren appends the selected tab, matching Children.
func (n *Notebook) appendChildren(buf []Widget) []Widget {
	if n.selected < len(n.tabs) {
		return append(buf, n.tabs[n.selected].w)
	}
	return buf
}

// SetEnabled turns the notebook's subtree on or off through the
// per-query enable walk, like Box. Like Stack, hidden pages stay
// untouched: they are not painted, so they need no repaint.
func (n *Notebook) SetEnabled(enabled bool) {
	n.node.SetEnabled(enabled)
	invalidateTree(n)
}

// Measure reports the tab bar height plus the largest page.
func (n *Notebook) Measure(con Constraints) Size {
	if sz, ok := n.measureHit(con); ok {
		return sz
	}
	best := Size{}
	for _, t := range n.tabs {
		s := measureChild(n, t.w, con)
		best.W = max(best.W, s.W)
		best.H = max(best.H, s.H)
	}
	best.H += tabBarHeight
	return n.measureStore(con, clampSize(best, con))
}

// Arrange lays out the tab strip and the visible page beneath it.
func (n *Notebook) Arrange(r render.Rect) {
	n.node.Arrange(r)
	n.scrollStrip(n.scroll) // a wider strip may have less to scroll
	n.applyReveal()
	page := render.Rect{X: r.X, Y: r.Y + tabBarHeight, W: r.W, H: max(0, r.H-tabBarHeight)}
	if n.selected < len(n.tabs) {
		n.tabs[n.selected].w.Arrange(page)
		setParents(n, n.tabs[n.selected].w)
	}
}

// tabRect returns the strip occupied by tab i, scrolled.
func (n *Notebook) tabRect(i int) render.Rect {
	return render.Rect{X: n.tabViewport().X + i*tabWidth - n.scroll, Y: n.bounds.Y, W: tabWidth, H: tabBarHeight}
}

// tabAt is the tab under p, -1 off the visible strip.
func (n *Notebook) tabAt(p Point) int {
	if !n.tabViewport().Contains(p.X, p.Y) {
		return -1
	}
	for i := range n.tabs {
		if n.tabRect(i).Contains(p.X, p.Y) {
			return i
		}
	}
	return -1
}

// arrowRects are the overflow arrows' strips, empty without overflow.
func (n *Notebook) arrowRects() [2]render.Rect {
	if !n.stripRange().Overflows() {
		return [2]render.Rect{}
	}
	y := n.bounds.Y
	return [2]render.Rect{
		{X: n.bounds.X, Y: y, W: tabArrowW, H: tabBarHeight},
		{X: n.bounds.X + n.bounds.W - tabArrowW, Y: y, W: tabArrowW, H: tabBarHeight},
	}
}

// Paint draws the tab bar through its nodes — the notebook's box, the
// header strip, and each tab (the selected one :checked: its own
// background and label color, the accent underline as the theme's
// fallback) — and the visible page.
func (n *Notebook) Paint(cv *render.Canvas) {
	th := Current()
	v := n.style(n)
	fx := pushEffects(cv, v)
	radii := radiusOr(v, th.Radius)
	paintBoxBehind(cv, v, n.bounds, radii, borderOf(v), pickc(0, v, style.PropBackgroundColor, th.Surface))
	paintOutline(cv, v, n.bounds, radii)
	fx.pop(cv)

	header := render.Rect{X: n.bounds.X, Y: n.bounds.Y, W: n.bounds.W, H: tabBarHeight}
	hv := n.header.style(&n.header)
	hfx := pushEffects(cv, hv)
	n.header.Arrange(header)
	n.header.tabs.Arrange(header)
	hradii := radiusOr(hv, 0)
	paintBoxBehind(cv, hv, header, hradii, borderOf(hv), pickc(0, hv, style.PropBackgroundColor, 0))
	paintOutline(cv, hv, header, hradii)
	hfx.pop(cv)

	strip := cv.PushClip(n.tabViewport())
	for i, t := range n.tabs {
		rect := n.tabRect(i)
		tp := &n.header.tabs.tab[i]
		tp.Arrange(rect)
		tv := tp.style(tp)
		var fill, underline render.Color
		if i == n.selected {
			fill, underline = th.SurfaceHover, th.Accent
		}
		tfx := pushEffects(cv, tv)
		tradii := radiusOr(tv, 0)
		paintBoxBehind(cv, tv, rect, tradii, borderOf(tv), pickc(0, tv, style.PropBackgroundColor, fill))
		if i == n.selected && !tv.Has(style.PropBoxShadow) {
			cv.FillRect(render.Rect{X: rect.X, Y: rect.Y + tabBarHeight - 3, W: rect.W, H: 3}, underline)
		}
		tfx.pop(cv)
		if n.face != nil {
			color := th.TextMuted
			if i == n.selected {
				color = th.Text
			}
			color = pickc(0, tv, style.PropColor, color)
			n.face.DrawAligned(cv, t.name, rect, 12, color, render.AlignCenter)
		}
		if n.Closable {
			strokeCross(cv, rect.X+rect.W-13, rect.Y+tabBarHeight/2, 4, th.TextMuted)
		}
	}
	cv.PopClip(strip)
	for k, r := range n.arrowRects() {
		if !r.Empty() {
			n.arrows[k].Arrange(r)
			setParents(n, n.arrows[k])
			PaintChild(cv, n.arrows[k])
		}
	}
	if n.selected < len(n.tabs) {
		prev := cv.PushClip(render.Rect{
			X: n.bounds.X, Y: n.bounds.Y + tabBarHeight,
			W: n.bounds.W, H: max(0, n.bounds.H-tabBarHeight),
		})
		PaintChild(cv, n.tabs[n.selected].w)
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
	arrows := n.arrowRects()
	switch {
	case arrows[0].Contains(p.X, p.Y):
		n.scrollStrip(n.scroll - tabWidth)
		return
	case arrows[1].Contains(p.X, p.Y):
		n.scrollStrip(n.scroll + tabWidth)
		return
	}
	i := n.tabAt(p)
	if i < 0 {
		return
	}
	if rect := n.tabRect(i); n.Closable && p.X > rect.X+rect.W-20 {
		if n.OnTabClose != nil {
			n.OnTabClose(n.tabs[i].name)
		}
		return
	}
	n.selectIndex(i)
}

// PressAt picks up a tab for reordering.
func (n *Notebook) PressAt(p Point) {
	if i := n.tabAt(p); n.Reorderable && i >= 0 {
		n.drag.begin(p)
		n.dragTab = i
	}
}

// DragMove moves the dragged tab into the slot under the pointer, so
// the strip reorders live as the pointer crosses its neighbors.
func (n *Notebook) DragMove(p Point) {
	if _, _, ok := n.drag.travel(p); !ok {
		return
	}
	if i := n.tabAt(Point{X: p.X, Y: n.bounds.Y + 1}); i >= 0 && i != n.dragTab {
		n.MoveTab(n.dragTab, i)
		n.dragTab = i
	}
}

// PressEnd drops the tab.
func (n *Notebook) PressEnd() { n.drag.end() }

// HoverMove tracks whether the pointer is over the tab strip.
func (n *Notebook) HoverMove(p Point) { n.overHeader = p.Y < n.bounds.Y+tabBarHeight }

// SetHovered forgets the strip when the pointer leaves the notebook
// itself (into a page, or away).
func (n *Notebook) SetHovered(on bool) {
	if !on {
		n.overHeader = false
	}
}

// ScrollInput scrolls an overflowing strip under the pointer, a third
// of a tab per wheel step; anywhere else the step passes on to the
// scrollers outside.
func (n *Notebook) ScrollInput(dy int) bool {
	if !n.overHeader || !n.stripRange().Overflows() {
		return false
	}
	n.scrollStrip(n.scroll + dy*tabWidth/3)
	return true
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
