package widget

import (
	"slices"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget/css"
)

type ListBox struct {
	node
	selection
	rows        []*ListBoxRow
	filterFn    func(*ListBoxRow) bool
	sortFn      func(a, b *ListBoxRow) int
	headerFn    func(*ListBoxRow) Widget
	order       []int
	hidden      []bool
	ph          Widget
	singleClick bool

	OnSelectionChanged func(rows []int)
	OnActivate         func(i int)
}

type ListBoxRow struct {
	node
	box         *ListBox
	child       Widget
	header      Widget
	nat         Size
	headNat     Size
	selectable  bool
	activatable bool
}

func NewListBox() *ListBox {
	l := &ListBox{singleClick: true}
	l.SetElement("list")
	l.AddClass(css.BoxedList)
	l.initSelection(l, SelectionSingle)
	return l
}

func (l *ListBox) SetSingleClickActivate(on bool) { l.singleClick = on }

func (l *ListBox) count() int { return len(l.rows) }

func (l *ListBox) selectionSync() {
	for i, r := range l.rows {
		r.SetState(StateSelected, l.isSelected(i))
	}
	l.Invalidate()
}

func (l *ListBox) selectionNotify() {
	if l.OnSelectionChanged != nil {
		l.OnSelectionChanged(l.Selection())
	}
}

func (l *ListBox) selectionSingle(int) {}

func (l *ListBox) reveal(i int) {
	if r := l.RowAt(i); r != nil {
		RevealRect(r, r.bounds)
	}
}

func (l *ListBox) activate(i int) {
	if r := l.RowAt(i); r != nil && r.activatable && l.OnActivate != nil {
		l.OnActivate(i)
	}
}

func (l *ListBox) Append(w Widget) *ListBoxRow { return l.Insert(len(l.rows), w) }

func (l *ListBox) Insert(i int, w Widget) *ListBoxRow {
	r := &ListBoxRow{box: l, child: w, selectable: true, activatable: true}
	r.SetElement("row")
	r.AddClass(css.Activatable)
	r.cursorName = "pointer"
	i = min(max(i, 0), len(l.rows))
	l.rows = append(l.rows, nil)
	copy(l.rows[i+1:], l.rows[i:])
	l.rows[i] = r
	l.hidden = append(l.hidden, false)
	copy(l.hidden[i+1:], l.hidden[i:])
	l.hidden[i] = false
	l.inserted(i)
	setParents(l, r)
	setParents(r, w)
	l.recompute()
	restyleChildren(l)
	return r
}

func (l *ListBox) RemoveAt(i int) {
	if i < 0 || i >= len(l.rows) {
		return
	}
	r := l.rows[i]
	l.rows = append(l.rows[:i], l.rows[i+1:]...)
	l.hidden = append(l.hidden[:i], l.hidden[i+1:]...)
	l.removed(i)
	if r.header != nil {
		clearParents(r.header)
	}
	notifyRemoved(r.child)
	clearParents(r, r.child)
	r.box = nil
	l.recompute()
	restyleChildren(l)
}

func (l *ListBox) Move(from, to int) {
	if from < 0 || from >= len(l.rows) {
		return
	}
	to = min(max(to, 0), len(l.rows)-1)
	if from == to {
		return
	}
	r := l.rows[from]
	l.rows = slices.Insert(slices.Delete(l.rows, from, from+1), to, r)
	h := l.hidden[from]
	l.hidden = slices.Insert(slices.Delete(l.hidden, from, from+1), to, h)
	l.selection.moved(from, to)
	l.recompute()
	restyleChildren(l)
}

func (l *ListBox) Len() int { return len(l.rows) }

func (l *ListBox) RowAt(i int) *ListBoxRow {
	if i < 0 || i >= len(l.rows) {
		return nil
	}
	return l.rows[i]
}

func (l *ListBox) Rows() []Widget {
	out := make([]Widget, len(l.rows))
	for i, r := range l.rows {
		out[i] = r
	}
	return out
}

func (l *ListBox) SetPlaceholder(w Widget) {
	if l.ph != nil {
		clearParents(l.ph)
	}
	l.ph = w
	if w != nil {
		setParents(l, w)
	}
	l.InvalidateLayout()
}

func (l *ListBox) SetFilterFunc(keep func(*ListBoxRow) bool) {
	l.filterFn = keep
	l.recompute()
}

func (l *ListBox) SetSortFunc(cmp func(a, b *ListBoxRow) int) {
	l.sortFn = cmp
	l.recompute()
}

func (l *ListBox) SetHeaderFunc(fn func(*ListBoxRow) Widget) {
	l.headerFn = fn
	l.rerunHeaders()
}

func (l *ListBox) InvalidateFilter() { l.recompute() }

func (l *ListBox) InvalidateSort() { l.recompute() }

func (l *ListBox) InvalidateHeaders() { l.rerunHeaders() }

func (l *ListBox) recompute() {
	order := make([]int, 0, len(l.rows))
	for i, r := range l.rows {
		keep := l.filterFn == nil || l.filterFn(r)
		l.hidden[i] = !keep
		if keep {
			order = append(order, i)
		}
	}
	if l.sortFn != nil {
		slices.SortStableFunc(order, func(a, b int) int { return l.sortFn(l.rows[a], l.rows[b]) })
	}
	l.order = order
	l.dropHiddenSelection()
	l.InvalidateLayout()
}

func (l *ListBox) dropHiddenSelection() {
	before := l.Selection()
	switch l.mode {
	case SelectionMultiple:
		set := make(map[int]struct{}, len(l.multi))
		for i := range l.multi {
			if !l.hidden[i] {
				set[i] = struct{}{}
			}
		}
		l.applyMulti(set)
	case SelectionBrowse:
		if l.sel >= 0 && l.hidden[l.sel] {
			l.sel = -1
			for _, i := range l.order {
				l.sel = i
				break
			}
		}
	case SelectionSingle:
		if l.sel >= 0 && l.hidden[l.sel] {
			l.sel = -1
		}
	}
	l.selectionSync()
	if after := l.Selection(); !slices.Equal(before, after) {
		l.notify()
	}
}

func (l *ListBox) rerunHeaders() {
	for _, r := range l.rows {
		if r.header != nil {
			clearParents(r.header)
		}
		r.header = nil
		if l.headerFn != nil {
			if h := l.headerFn(r); h != nil {
				r.header = h
				setParents(r, h)
			}
		}
	}
	l.InvalidateLayout()
}

func (l *ListBox) anyVisible() bool {
	for _, i := range l.order {
		if IsVisible(l.rows[i]) {
			return true
		}
	}
	return false
}

func (l *ListBox) placeholderShown() bool {
	return l.ph != nil && !l.anyVisible() && IsVisible(l.ph)
}

func (l *ListBox) indexAt(p Point) int {
	for _, i := range l.order {
		if IsVisible(l.rows[i]) && l.rows[i].bounds.Contains(p.X, p.Y) {
			return i
		}
	}
	return -1
}

func (l *ListBox) rowClick(i int) {
	r := l.RowAt(i)
	if r == nil {
		return
	}
	if r.selectable {
		l.click(i)
	}
	if l.singleClick && l.mode != SelectionMultiple && r.activatable {
		l.activate(i)
	}
}

func (l *ListBox) KeyAction(a KeyAction, mods Mods) {
	if IsEnabled(l) {
		l.key(a, mods, l.navigate)
	}
}

func (l *ListBox) navigate(from int, a KeyAction) (int, bool) {
	if a != KeyUp && a != KeyDown {
		return 0, false
	}
	navigable := func(i int) bool { return !l.hidden[i] && l.rows[i].selectable }
	pos := slices.Index(l.order, from)
	if pos < 0 {
		for _, i := range l.order {
			if navigable(i) {
				return i, true
			}
		}
		return 0, false
	}
	step := -1
	if a == KeyDown {
		step = 1
	}
	for p := pos + step; p >= 0 && p < len(l.order); p += step {
		if navigable(l.order[p]) {
			return l.order[p], true
		}
	}
	return from, true
}

func (l *ListBox) Measure(con Constraints) Size {
	if sz, ok := l.measureHit(con); ok {
		return sz
	}
	v := l.style(l)
	box := boxOf(v, render.Insets{})
	return l.measureStore(con, measureBox(v, box, con, func(inner Constraints) Size {
		var out Size
		for _, i := range l.order {
			r := l.rows[i]
			r.nat = measureChild(l, r, Constraints{Max: inner.Max})
			if IsVisible(r) {
				out.W = max(out.W, r.nat.W)
				out.H += r.nat.H
			}
		}
		if l.placeholderShown() {
			out = measureChild(l, l.ph, Constraints{Max: inner.Max})
		}
		return clampSize(out, inner)
	}))
}

func (l *ListBox) Arrange(r render.Rect) {
	border, inner := boxRects(boxOf(l.style(l), render.Insets{}), r)
	l.node.Arrange(border)
	y := inner.Y
	for _, i := range l.order {
		r := l.rows[i]
		if !IsVisible(r) {
			continue
		}
		r.Arrange(render.Rect{X: inner.X, Y: y, W: inner.W, H: r.nat.H})
		setParents(l, r)
		y += r.nat.H
	}
	if l.placeholderShown() {
		arrangeChild(l.ph, inner)
	}
}

func (l *ListBox) Paint(cv *render.Canvas) {
	paintCSSBox(cv, l.style(l), l.bounds)
	th := Current()
	prev := false
	for _, i := range l.order {
		r := l.rows[i]
		if !IsVisible(r) {
			continue
		}
		if prev && (r.header == nil || !IsVisible(r.header)) {
			b := r.bounds
			cv.FillRect(render.Rect{X: b.X + 8, Y: b.Y, W: b.W - 16, H: 1}, th.Border)
		}
		prev = true
		PaintChild(cv, r)
	}
	if l.placeholderShown() {
		PaintChild(cv, l.ph)
	}
}

func (l *ListBox) HitTest(p Point) Widget {
	for _, i := range l.order {
		r := l.rows[i]
		if !IsVisible(r) {
			continue
		}
		if hit := r.HitTest(p); hit != nil {
			return hit
		}
	}
	if l.placeholderShown() {
		return l.HitLeaf(l.ph, p)
	}
	return l.HitLeaf(l, p)
}

func (r *ListBoxRow) Child() Widget { return r.child }

func (r *ListBoxRow) Header() Widget { return r.header }

func (r *ListBoxRow) Index() int {
	if r.box != nil {
		return slices.Index(r.box.rows, r)
	}
	return -1
}

func (r *ListBoxRow) SetSelectable(on bool) { r.selectable = on }

func (r *ListBoxRow) Selectable() bool { return r.selectable }

func (r *ListBoxRow) SetActivatable(on bool) {
	if r.activatable == on {
		return
	}
	r.activatable = on
	r.cursorName = ""
	if on {
		r.cursorName = "pointer"
		r.AddClass(css.Activatable)
	} else {
		r.RemoveClass(css.Activatable)
	}
	r.Invalidate()
}

func (r *ListBoxRow) Activatable() bool { return r.activatable }

func (r *ListBoxRow) Measure(con Constraints) Size {
	if sz, ok := r.measureHit(con); ok {
		return sz
	}
	v := r.style(r)
	box := boxOf(v, render.Insets{})
	return r.measureStore(con, measureBox(v, box, con, func(inner Constraints) Size {
		var out Size
		r.headNat = Size{}
		if r.header != nil {
			r.headNat = measureChild(r, r.header, Constraints{Max: inner.Max})
			if !IsVisible(r.header) {
				r.headNat.H = 0
			}
			out.W = max(out.W, r.headNat.W)
			out.H += r.headNat.H
		}
		c := measureChild(r, r.child, inner)
		out.W = max(out.W, c.W)
		out.H += c.H
		return clampSize(out, inner)
	}))
}

func (r *ListBoxRow) Arrange(rct render.Rect) {
	border, inner := boxRects(boxOf(r.style(r), render.Insets{}), rct)
	r.node.Arrange(border)
	if r.header != nil && r.headNat.H > 0 {
		arrangeChild(r.header, render.Rect{X: inner.X, Y: inner.Y, W: inner.W, H: r.headNat.H})
		inner.Y += r.headNat.H
		inner.H -= r.headNat.H
	}
	arrangeChild(r.child, inner)
}

func (r *ListBoxRow) Paint(cv *render.Canvas) {
	v := r.style(r)
	if r.HasState(StateSelected) && !v.Declares(style.PropBackgroundColor) {
		cv.RoundedRect(r.bounds, radiusOr(v, 6).TopLeft, selectedTint())
	}
	paintCSSBox(cv, v, r.bounds)
	if r.header != nil && r.headNat.H > 0 {
		PaintChild(cv, r.header)
	}
	PaintChild(cv, r.child)
}

func (r *ListBoxRow) HitTest(p Point) Widget {
	if hit := r.child.HitTest(p); hit != nil && (r.box == nil || r.box.mode == SelectionNone || !r.selectable || interactiveWithin(hit, r)) {
		return hit
	}
	if r.header != nil && r.headNat.H > 0 {
		if hit := r.header.HitTest(p); hit != nil {
			return hit
		}
	}
	return r.HitLeaf(r, p)
}

func (r *ListBoxRow) ClickAt(Point) { r.box.rowClick(r.Index()) }

func (r *ListBoxRow) DoubleClickAt(Point) {
	if !r.box.singleClick && r.box.mode != SelectionNone && r.activatable {
		r.box.activate(r.Index())
	}
}

func (r *ListBoxRow) SetPressed(on bool) {
	if on {
		r.box.hold()
	}
}

func (r *ListBoxRow) DragMove(p Point) {
	b := r.box
	if b.mode != SelectionSingle && b.mode != SelectionBrowse {
		return
	}
	if at := b.indexAt(p); at >= 0 && at != b.sel {
		b.selectOne(at)
	}
}

func (r *ListBoxRow) PressEnd() { r.box.release() }

func (r *ListBoxRow) KeyAction(a KeyAction, mods Mods) { r.box.KeyAction(a, mods) }

func (r *ListBoxRow) SelectAll() { r.box.SelectAll() }

func (r *ListBoxRow) Children() []Widget {
	if r.header == nil {
		return []Widget{r.child}
	}
	return []Widget{r.header, r.child}
}
