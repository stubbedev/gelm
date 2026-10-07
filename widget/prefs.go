package widget

import (
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
	composite
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
	column := NewBox(Column, 6, 0)
	if title != "" {
		column.Append(g.head, false)
	}
	column.Append(g.rows, false)
	g.initComposite(g, column)
	g.fillWidth = true
	g.surfaceRadius = 8
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

// Paint draws the boxed-list card around the rows with hairlines
// between them.
func (g *PreferencesGroup) Paint(cv *render.Canvas) {
	th := Current()
	g.paintSurface(cv, g.rows.Bounds(), th.Surface)
	PaintChild(cv, g.root)
	for i, row := range g.rows.Children() {
		if child, ok := row.(Boundser); ok && i > 0 {
			b := child.Bounds()
			cv.FillRect(render.Rect{X: g.rows.Bounds().X + 8, Y: b.Y, W: g.rows.Bounds().W - 16, H: 1}, th.Border)
		}
	}
}

// PreferencesPage is the scrolling stack of groups: a column of
// groups in a vertical Scroll, clamped to a readable width - the adw
// page.
type PreferencesPage struct {
	composite
	column *Box
	scroll *Scroll
}

// NewPreferencesPage returns an empty page.
func NewPreferencesPage() *PreferencesPage {
	p := &PreferencesPage{}
	p.column = NewBox(Column, 24, 12)
	// The page scrolls vertically only, so the groups lay out at the
	// viewport's width, clamped readable (adw's clamp).
	p.scroll = NewScroll(NewClamp(600, p.column))
	p.scroll.VerticalOnly = true
	p.initComposite(p, p.scroll)
	return p
}

// Add appends a group (or any widget) to the page.
func (p *PreferencesPage) Add(w Widget) {
	p.column.Append(w, false)
	p.InvalidateLayout()
}

// ScrollBy forwards wheel and gesture scrolling to the page's scroll.
func (p *PreferencesPage) ScrollBy(dx, dy int) { p.scroll.ScrollBy(dx, dy) }
