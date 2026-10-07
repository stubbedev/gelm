package widget

import (
	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// Client-side window chrome (#91): HeaderBar, ActionBar, and
// WindowControls - the pieces a CSD window draws itself, matching
// adw::HeaderBar's shape. The bar reports window-move grabs through
// the WindowMover interface and the app routes the press into
// xdg_toplevel.move; the control buttons are hooks the app wires to
// its window (the widget never reaches the compositor).

// WindowMover is implemented by widgets whose background area is a
// drag handle for moving the window: a press that resolves into the
// widget (not a deeper control) starts the compositor's move grab.
// HeaderBar carries it; a stylesheet's custom header can too.
type WindowMover interface {
	// WindowMoveGrab reports whether a press on the widget itself (not
	// its children) should move the window.
	WindowMoveGrab() bool
}

// WindowDoubleClickable is the optional second half of WindowMover:
// a double press inside the grab area maximizes, GTK header style.
type WindowDoubleClickable interface {
	// WindowDoubleClick fires on the second press inside the grab
	// interval; the widget routes it where it wants (HeaderBar's
	// OnDoubleClick hook).
	WindowDoubleClick()
}

// DescendsFrom reports whether w is ancestor or a descendant of it -
// the walk the input pipeline uses to find a drag handle under a press
// that resolved into a child control.
func DescendsFrom(w, ancestor Widget) bool {
	for cur := w; cur != nil; {
		if cur == ancestor {
			return true
		}
		cur = parentOf(cur)
	}
	return false
}

// HeaderBar is the CSD title bar: a start pack (left), the window
// title and subtitle (center), and an end pack with the window
// controls (right). Pressing the bar's own background moves the window
// (the app routes it to xdg_toplevel.move); a double press maximizes -
// both delivered through the WindowMover contract, so the bar works in
// any host that honors it. Styled as the `headerbar` element, its
// title as `headerbar title` (docs/css.md).
type HeaderBar struct {
	node
	face   render.Font
	sizePx float64

	root     *Box
	start    *Box
	center   *Box
	end      *Box
	title    *Label
	sub      *Label
	controls *WindowControls

	// OnDoubleClick fires on a second press inside the grab interval -
	// the app wires it to maximize/restore.
	OnDoubleClick func()

	height int
}

// NewHeaderBar returns a bar sized for face/sizePx content.
func NewHeaderBar(face render.Font, sizePx float64) *HeaderBar {
	face = requireFace("widget.NewHeaderBar", face)
	h := &HeaderBar{face: face, sizePx: sizePx, height: int(sizePx) + 22}
	th := Current()
	h.SetElement("headerbar")
	h.title = NewLabel(face, sizePx, "", th.Text)
	h.sub = NewLabel(face, sizePx-3, "", th.TextMuted)
	h.title.SetElement("title")
	h.sub.SetElement("subtitle")
	h.center = NewBox(Column, 1, 0)
	h.center.Append(h.title, false)
	h.center.Append(h.sub, false)
	h.start = NewBox(Row, 6, 0)
	h.end = NewBox(Row, 6, 0)
	h.root = NewBox(Row, 8, 6)
	h.root.Append(h.start, false)
	h.root.Append(NewSpacer(0, 0), true)
	h.root.AppendAligned(h.center, false, AlignCenter)
	h.root.Append(NewSpacer(0, 0), true)
	h.root.Append(h.end, false)
	return h
}

// SetTitle sets the window title; SetSubtitle the line under it (an
// empty subtitle hides the row).
func (h *HeaderBar) SetTitle(t string) { h.title.SetText(t) }

// SetSubtitle sets the subtitle; empty hides it.
func (h *HeaderBar) SetSubtitle(s string) { h.sub.SetText(s) }

// PackStart adds w to the leading pack, PackEnd to the trailing pack
// (before the window controls).
func (h *HeaderBar) PackStart(w Widget) { h.start.Append(w, false); h.InvalidateLayout() }

// PackEnd adds w to the trailing pack, before the controls.
func (h *HeaderBar) PackEnd(w Widget) { h.end.InsertAt(0, w, false); h.InvalidateLayout() }

// Controls returns the window controls, wiring them is the app's job:
//
//	bar.Controls().ShowClose(true)
//	bar.Controls().OnClose = window.Close
func (h *HeaderBar) Controls() *WindowControls {
	if h.controls == nil {
		h.controls = NewWindowControls(h.face, h.sizePx)
		h.end.Append(h.controls, false)
	}
	return h.controls
}

// WindowMoveGrab implements WindowMover: presses on the bar's own
// chrome move the window.
func (h *HeaderBar) WindowMoveGrab() bool { return true }

// WindowDoubleClick implements WindowDoubleClickable: the second
// press fires the app-wired hook (maximize/restore).
func (h *HeaderBar) WindowDoubleClick() {
	if h.OnDoubleClick != nil {
		h.OnDoubleClick()
	}
}

// Measure measures the packs and wants at least the chrome height,
// the full width it is given.
func (h *HeaderBar) Measure(con Constraints) Size {
	if sz, ok := h.measureHit(con); ok {
		return sz
	}
	inner := h.root.Measure(con)
	height := max(h.height, inner.H)
	return h.measureStore(con, clampSize(Size{W: con.Max.W, H: height}, con))
}

// Arrange fills the bar and lays its packs out: start left, center
// centered, end right.
func (h *HeaderBar) Arrange(r render.Rect) {
	h.node.Arrange(r)
	h.root.Arrange(r)
	setParents(h, h.root)
}

// Paint draws the bar's box then its content.
func (h *HeaderBar) Paint(cv *render.Canvas) {
	th := Current()
	v := h.style(h)
	paintBoxBehind(cv, v, h.bounds, radiusOr(v, 0), borderOf(v), pickc(0, v, style.PropBackgroundColor, th.Surface))
	PaintChild(cv, h.root)
}

// HitTest resolves into the composed row with the CSD rule applied:
// a press that lands on a button stays a button press; anything else
// on the bar - background, title, spacers - is the move surface and
// resolves to the bar itself.
func (h *HeaderBar) HitTest(p Point) Widget {
	if !h.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if hit := h.root.HitTest(p); hit != nil {
		for cur := hit; cur != nil; cur = parentOf(cur) {
			if b, ok := cur.(*Button); ok {
				return b
			}
			if cur == Widget(h.root) {
				break
			}
		}
	}
	return h
}

// styleChildren is the composed row and title parts (styleKids).
func (h *HeaderBar) styleChildren() []Widget {
	kids := []Widget{h.root}
	return kids
}

// WindowControls are the CSD frame buttons: close, minimize, maximize.
// Each is shown only when asked - the compositor's capabilities decide
// which an app offers - and each fires its hook on click.
type WindowControls struct {
	node
	face   render.Font
	sizePx float64
	root   *Box

	// OnClose, OnMinimize, and OnMaximize fire for the corresponding
	// button; the app wires them to its window.
	OnClose    func()
	OnMinimize func()
	OnMaximize func()

	closeBtn *Button
	minBtn   *Button
	maxBtn   *Button
}

// NewWindowControls returns a controls cluster with every button
// hidden; Show* turns them on.
func NewWindowControls(face render.Font, sizePx float64) *WindowControls {
	face = requireFace("widget.NewWindowControls", face)
	c := &WindowControls{face: face, sizePx: sizePx}
	c.root = NewBox(Row, 4, 0)
	return c
}

// ShowClose shows the close button (a cross), wired to OnClose.
func (c *WindowControls) ShowClose(on bool) { c.toggle(&c.closeBtn, "✕", &c.OnClose, on) }

// ShowMinimize shows the minimize button (an underscore), wired to
// OnMinimize.
func (c *WindowControls) ShowMinimize(on bool) { c.toggle(&c.minBtn, "—", &c.OnMinimize, on) }

// ShowMaximize shows the maximize button (a square), wired to
// OnMaximize.
func (c *WindowControls) ShowMaximize(on bool) { c.toggle(&c.maxBtn, "□", &c.OnMaximize, on) }

// toggle adds or removes one button.
func (c *WindowControls) toggle(slot **Button, glyph string, hook *func(), on bool) {
	if on == (*slot != nil) {
		return
	}
	if !on {
		if i := c.rootChildIndex(*slot); i >= 0 {
			c.root.RemoveAt(i)
		}
		*slot = nil
		c.InvalidateLayout()
		return
	}
	th := Current()
	btn := NewButton(NewLabel(c.face, c.sizePx-2, glyph, th.Text), 6, 3)
	btn.OnClick = func() {
		if *hook != nil {
			(*hook)()
		}
	}
	*slot = btn
	c.root.Append(btn, false)
	c.InvalidateLayout()
}

// childIndex finds a child's slot; buttons keep no other state.
func (c *WindowControls) rootChildIndex(w Widget) int {
	for i, child := range c.root.Children() {
		if child == w {
			return i
		}
	}
	return -1
}

// Measure wants the buttons' natural row.
func (c *WindowControls) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	return c.measureStore(con, c.root.Measure(con))
}

// Arrange fills and lays the row.
func (c *WindowControls) Arrange(r render.Rect) {
	c.node.Arrange(r)
	c.root.Arrange(r)
	setParents(c, c.root)
}

// Paint draws the row.
func (c *WindowControls) Paint(cv *render.Canvas) { PaintChild(cv, c.root) }

// HitTest resolves into the row.
func (c *WindowControls) HitTest(p Point) Widget {
	if !c.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if hit := c.root.HitTest(p); hit != nil {
		return hit
	}
	return c
}

// styleChildren is the row (styleKids).
func (c *WindowControls) styleChildren() []Widget { return []Widget{c.root} }

// ActionBar is the bottom bar: one pack, centered content optional -
// the gtk ActionBar shape. PackStart/PackEnd as on HeaderBar.
type ActionBar struct {
	node
	root  *Box
	start *Box
	end   *Box
}

// NewActionBar returns an empty bottom bar.
func NewActionBar() *ActionBar {
	a := &ActionBar{}
	a.SetElement("actionbar")
	a.start = NewBox(Row, 6, 0)
	a.end = NewBox(Row, 6, 0)
	a.root = NewBox(Row, 8, 6)
	a.root.Append(a.start, false)
	a.root.Append(NewSpacer(0, 0), true)
	a.root.Append(a.end, false)
	return a
}

// PackStart adds w to the leading pack.
func (a *ActionBar) PackStart(w Widget) { a.start.Append(w, false); a.InvalidateLayout() }

// PackEnd adds w to the trailing pack.
func (a *ActionBar) PackEnd(w Widget) { a.end.Append(w, false); a.InvalidateLayout() }

// Measure wants its content's height, full width.
func (a *ActionBar) Measure(con Constraints) Size {
	if sz, ok := a.measureHit(con); ok {
		return sz
	}
	inner := a.root.Measure(con)
	return a.measureStore(con, clampSize(Size{W: con.Max.W, H: inner.H}, con))
}

// Arrange fills and lays the row.
func (a *ActionBar) Arrange(r render.Rect) {
	a.node.Arrange(r)
	a.root.Arrange(r)
	setParents(a, a.root)
}

// Paint draws the bar's surface then the row.
func (a *ActionBar) Paint(cv *render.Canvas) {
	th := Current()
	v := a.style(a)
	paintBoxBehind(cv, v, a.bounds, radiusOr(v, 0), borderOf(v), pickc(0, v, style.PropBackgroundColor, th.Surface))
	PaintChild(cv, a.root)
}

// HitTest resolves into the row.
func (a *ActionBar) HitTest(p Point) Widget {
	if !a.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if hit := a.root.HitTest(p); hit != nil {
		return hit
	}
	return a
}

// styleChildren is the row (styleKids).
func (a *ActionBar) styleChildren() []Widget { return []Widget{a.root} }
