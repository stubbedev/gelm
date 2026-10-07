package widget

import (
	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// PreferencesGroup and PreferencesPage (#93): the labeled, bordered
// boxed-list a settings page is made of, and the scrolling page that
// stacks groups. Both are plain composites over Box and Scroll - the
// group paints the boxed-list look (surface card, rounded border, a
// hairline between rows) that a stylesheet can restyle through the
// `rows` element.

// PreferencesGroup is a labeled boxed list of rows: a header line
// (title plus an optional suffix, the adw description slot), the
// bordered card, and rows separated by hairlines. Add appends rows;
// any widget works, ActionRow and friends are the point.
type PreferencesGroup struct {
	node
	root   *Box
	head   *Box
	title  *Label
	suffix *Label
	rows   *Box
}

// NewPreferencesGroup returns an empty group; title empty skips the
// header line.
func NewPreferencesGroup(face render.Font, sizePx float64, title string) *PreferencesGroup {
	face = requireFace("widget.NewPreferencesGroup", face)
	g := &PreferencesGroup{}
	g.SetElement("rows")
	th := Current()
	g.title = NewLabel(face, sizePx-1, title, th.TextMuted)
	g.suffix = NewLabel(face, sizePx-1, "", th.Accent)
	g.head = NewBox(Row, 8, 0)
	g.head.Append(NewSpacer(0, 0), true)
	g.head.Append(g.suffix, false)
	g.head.InsertAt(0, g.title, false)
	g.rows = NewBox(Column, 0, 0)
	g.root = NewBox(Column, 6, 0)
	if title != "" {
		g.root.Append(g.head, false)
	}
	g.root.Append(g.rows, false)
	return g
}

// SetTitle sets the header title.
func (g *PreferencesGroup) SetTitle(t string) { g.title.SetText(t) }

// SetSuffix sets the header's trailing note (the adw description
// slot); empty hides it.
func (g *PreferencesGroup) SetSuffix(s string) { g.suffix.SetText(s) }

// Add appends a row to the card.
func (g *PreferencesGroup) Add(w Widget) {
	g.rows.Append(w, false)
	g.InvalidateLayout()
}

// Measure delegates to the composed column, cached like every widget.
func (g *PreferencesGroup) Measure(con Constraints) Size {
	if sz, ok := g.measureHit(con); ok {
		return sz
	}
	inner := g.root.Measure(con)
	return g.measureStore(con, clampSize(Size{W: con.Max.W, H: inner.H}, con))
}

// Arrange fills and lays the group.
func (g *PreferencesGroup) Arrange(r render.Rect) {
	g.node.Arrange(r)
	g.root.Arrange(r)
	setParents(g, g.root)
}

// Paint draws the boxed-list card around the rows with hairlines
// between them.
func (g *PreferencesGroup) Paint(cv *render.Canvas) {
	th := Current()
	v := g.style(g)
	paintBoxBehind(cv, v, g.rows.Bounds(), radiusOr(v, 8), borderOf(v), pickc(0, v, style.PropBackgroundColor, th.Surface))
	prev := cv.PushClip(g.rows.Bounds())
	PaintChild(cv, g.root)
	cv.PopClip(prev)
	first, last := g.rowRange()
	for i := first + 1; i < last; i++ {
		if child, ok := g.rows.Children()[i].(Boundser); ok {
			b := child.Bounds()
			cv.FillRect(render.Rect{X: g.rows.Bounds().X + 8, Y: b.Y, W: g.rows.Bounds().W - 16, H: 1}, th.Border)
		}
	}
}

// rowRange is the first and one-past-last row index for separators.
func (g *PreferencesGroup) rowRange() (int, int) {
	return 0, len(g.rows.Children())
}

// HitTest resolves into the composed column.
func (g *PreferencesGroup) HitTest(p Point) Widget {
	if !g.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if hit := g.root.HitTest(p); hit != nil {
		return hit
	}
	return g
}

// styleChildren is the composed column (styleKids).
func (g *PreferencesGroup) styleChildren() []Widget { return []Widget{g.root} }

// PreferencesPage is the scrolling stack of groups: a column of
// groups inside a Scroll, capped wide enough to read - the adw page
// without the viewport ceremony beyond the Scroll widget itself.
type PreferencesPage struct {
	node
	root   *Box
	column *Box
	scroll *Scroll
}

// NewPreferencesPage returns an empty page.
func NewPreferencesPage() *PreferencesPage {
	p := &PreferencesPage{}
	p.column = NewBox(Column, 24, 0)
	p.root = NewBox(Row, 0, 0)
	p.root.Append(NewSpacer(0, 0), true)
	p.root.AppendAligned(p.column, false, AlignStart)
	p.root.Append(NewSpacer(0, 0), true)
	p.scroll = NewScroll(p.root)
	return p
}

// Add appends a group (or any widget) to the page.
func (p *PreferencesPage) Add(w Widget) {
	p.column.Append(w, false)
	p.InvalidateLayout()
}

// Measure delegates to the scroll, cached like every widget.
func (p *PreferencesPage) Measure(con Constraints) Size {
	if sz, ok := p.measureHit(con); ok {
		return sz
	}
	return p.measureStore(con, p.scroll.Measure(con))
}

// Arrange fills and lays the page.
func (p *PreferencesPage) Arrange(r render.Rect) {
	p.node.Arrange(r)
	p.scroll.Arrange(r)
	setParents(p, p.scroll)
}

// Paint draws the scroll.
func (p *PreferencesPage) Paint(cv *render.Canvas) { PaintChild(cv, p.scroll) }

// HitTest resolves into the scroll.
func (p *PreferencesPage) HitTest(pnt Point) Widget {
	if !p.bounds.Contains(pnt.X, pnt.Y) {
		return nil
	}
	if hit := p.scroll.HitTest(pnt); hit != nil {
		return hit
	}
	return p
}

// ScrollBy forwards wheel and gesture scrolling to the page's scroll.
func (p *PreferencesPage) ScrollBy(dx, dy int) { p.scroll.ScrollBy(dx, dy) }

// styleChildren is the scroll (styleKids).
func (p *PreferencesPage) styleChildren() []Widget { return []Widget{p.scroll} }
