package widget

import (
	"github.com/stubbedev/gelm/render"
)

// ListModel supplies rows to a List. Row may return a fresh widget per
// call; the List caches the ones it is currently showing. Len and Row
// are re-queried on List.Changed.
type ListModel[W Widget] interface {
	Len() int
	Row(i int) W
}

// List is a virtualized single-select list view: only the rows inside
// the viewport are requested from the model, measured, and painted, so
// ten thousand rows cost about the same as ten. Selection is pointer
// and keyboard driven and clamps at the ends; Enter activates.
type List struct {
	node
	model listModel
	rowH  int

	viewW, viewH int
	offY         int
	sel          int
	hover        int
	rows         map[int]Widget
	dirty        bool

	OnSelect   func(i int)
	OnActivate func(i int)
}

// listModel erases the model's type parameter; the List only needs
// widgets out of it.
type listModel interface {
	len() int
	row(i int) Widget
}

// modelAdapter binds a typed model to the erased interface.
type modelAdapter[W Widget] struct {
	m ListModel[W]
}

func (a modelAdapter[W]) len() int { return a.m.Len() }

func (a modelAdapter[W]) row(i int) Widget { return a.m.Row(i) }

// NewList wraps a model with a uniform row height in pixels; zero
// derives the height from measuring the first row.
func NewList[W Widget](model ListModel[W], rowHeight int) *List {
	return &List{
		model: modelAdapter[W]{m: model},
		rowH:  rowHeight,
		sel:   -1,
		hover: -1,
		rows:  make(map[int]Widget),
	}
}

// Select moves the selection to row i (clamped, -1 clears) and fires
// OnSelect on change.
func (l *List) Select(i int) {
	n := l.model.len()
	i = min(max(i, -1), n-1)
	if i == l.sel {
		return
	}
	l.sel = i
	l.scrollTo(i)
	l.dirty = true
	l.Invalidate()
	if l.OnSelect != nil {
		l.OnSelect(i)
	}
}

// Selected returns the selected row, or -1.
func (l *List) Selected() int { return l.sel }

// Changed re-queries the model after its data changed, keeping cached
// row widgets whose indices still exist.
func (l *List) Changed() {
	n := l.model.len()
	for i := range l.rows {
		if i >= n {
			delete(l.rows, i)
		}
	}
	l.offY = min(l.offY, max(0, n*l.rowH-l.viewH))
	if l.sel >= n {
		l.sel = n - 1
	}
	l.dirty = true
	l.InvalidateLayout()
}

// scrollTo nudges the offset so row i is inside the viewport.
func (l *List) scrollTo(i int) {
	top, bottom := l.offY, l.offY+l.viewH
	y := i * l.rowH
	if y < top {
		l.offY = y
	} else if y+l.rowH > bottom {
		l.offY = y + l.rowH - l.viewH
	}
}

// measureRowH derives the row height from the first row once.
func (l *List) measureRowH() int {
	if l.rowH > 0 {
		return l.rowH
	}
	if l.model.len() == 0 {
		return 1
	}
	s := l.model.row(0).Measure(Constraints{Max: Size{W: 1 << 20, H: 1 << 20}})
	l.rowH = max(1, s.H)
	return l.rowH
}

// Measure reports the offered width by the full content height,
// clamped to the constraints: the list is the viewport, not the
// content.
func (l *List) Measure(con Constraints) Size {
	if sz, ok := l.measureHit(con); ok {
		return sz
	}
	h := l.measureRowH() * l.model.len()
	return l.measureStore(con, clampSize(Size{W: 120, H: h}, con))
}

// Arrange lays out the viewport.
func (l *List) Arrange(r render.Rect) {
	l.node.Arrange(r)
	l.viewW, l.viewH = r.W, r.H
	l.measureRowH()
	l.offY = min(l.offY, max(0, l.model.len()*l.rowH-l.viewH))
}

// visible returns the half-open range of rows inside the viewport and
// caches their widgets, evicting everything off-screen.
func (l *List) visible() (first, last int) {
	n := l.model.len()
	first = min(l.offY/l.rowH, max(0, n-1))
	last = min(n, first+l.viewH/l.rowH+1)
	for i := range l.rows {
		if i < first || i >= last {
			delete(l.rows, i)
		}
	}
	for i := first; i < last; i++ {
		if _, ok := l.rows[i]; !ok {
			l.rows[i] = l.model.row(i)
		}
	}
	return first, last
}

// Paint draws the viewport background, the selection and hover bands,
// and exactly the visible rows.
func (l *List) Paint(cv *render.Canvas) {
	th := Current()
	prev := cv.PushClip(l.bounds)
	cv.FillRect(l.bounds, th.Surface)

	first, last := l.visible()
	for i := first; i < last; i++ {
		w := l.rows[i]
		y := l.bounds.Y + i*l.rowH - l.offY
		w.Arrange(render.Rect{X: l.bounds.X, Y: y, W: l.viewW, H: l.rowH})
		setParents(l, w)
		switch i {
		case l.sel:
			hl := th.Accent
			cv.FillRect(render.Rect{X: l.bounds.X, Y: y, W: l.viewW, H: l.rowH},
				render.RGBA(hl.R(), hl.G(), hl.B(), 70))
		case l.hover:
			tint := th.SurfaceHover
			cv.FillRect(render.Rect{X: l.bounds.X, Y: y, W: l.viewW, H: l.rowH},
				render.RGBA(tint.R(), tint.G(), tint.B(), 120))
		}
		w.Paint(cv)
	}
	cv.PopClip(prev)
}

// Role implements Roleer.
func (l *List) Role() Role { return RoleList }

// HitTest returns the row widget under p, or the list itself.
func (l *List) HitTest(p Point) Widget {
	if !l.bounds.Contains(p.X, p.Y) {
		return nil
	}
	i := l.rowAt(p)
	if w, ok := l.rows[i]; ok {
		return w
	}
	return l
}

// rowAt maps a point to a row index.
func (l *List) rowAt(p Point) int {
	return min(max((p.Y-l.bounds.Y+l.offY)/l.rowH, 0), l.model.len()-1)
}

// HoverMove implements HoverMover: tracks the hovered row for styling
// and per-row tooltips. Disabled lists highlight nothing.
func (l *List) HoverMove(p Point) {
	if !IsEnabled(l) {
		return
	}
	i := l.rowAt(p)
	if i == l.hover {
		return
	}
	l.hover = i
	l.dirty = true
	l.Invalidate()
}

// ScrollBy implements ScrollHandler. Disabled lists do not scroll.
func (l *List) ScrollBy(dx, dy int) {
	if !IsEnabled(l) {
		return
	}
	scrollMax := max(0, l.model.len()*l.rowH-l.viewH)
	l.offY = min(max(0, l.offY+dy*40), scrollMax)
	l.dirty = true
	l.Invalidate()
}

// KeyAction implements KeyActionHandler: selection motion clamps at
// the ends (pinned), PageUp/PageDown move a viewport of rows, and
// Enter activates the selection exactly once per press. Disabled
// lists ignore keys.
func (l *List) KeyAction(a KeyAction, mods Mods) {
	if !IsEnabled(l) {
		return
	}
	n := l.model.len()
	if n == 0 {
		return
	}
	page := max(1, l.viewH/l.rowH)
	switch a {
	case KeyUp:
		l.Select(max(0, l.sel-1))
	case KeyDown:
		l.Select(min(n-1, l.sel+1))
	case KeyHome:
		l.Select(0)
	case KeyEnd:
		l.Select(n - 1)
	case KeyPriorPage:
		l.Select(max(0, l.sel-page))
	case KeyNextPage:
		l.Select(min(n-1, l.sel+page))
	case KeyEnter:
		if l.sel >= 0 && l.sel < n && l.OnActivate != nil {
			l.OnActivate(l.sel)
		}
	}
}
