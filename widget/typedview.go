package widget

import (
	"slices"
)

// TypedItems is the typed item model the typed views sit on, relm4's
// DynamicIndex analog: a slice of T with CRUD, insertion in sort order,
// and change notification - wholesale through SetRefresh/Notify, one
// edited item in place through SetItemRefresh/NotifyItem. It is a
// ColumnModel, so it feeds TypedList, TypedGrid and ColumnView alike.
type TypedItems[T any] struct {
	items       []T
	refresh     func()
	itemRefresh func(int)
}

// NewTypedItems returns a model over items.
func NewTypedItems[T any](items ...T) *TypedItems[T] {
	return &TypedItems[T]{items: items}
}

// Len implements ColumnModel.
func (m *TypedItems[T]) Len() int { return len(m.items) }

// Row implements ColumnModel.
func (m *TypedItems[T]) Row(i int) T { return m.items[i] }

// Get returns item i.
func (m *TypedItems[T]) Get(i int) T { return m.items[i] }

// Set replaces item i in place and notifies the item refresh, so a
// visible row rebinds to the new value without rebuilding the view.
func (m *TypedItems[T]) Set(i int, item T) {
	m.items[i] = item
	m.NotifyItem(i)
}

// Append adds item at the end.
func (m *TypedItems[T]) Append(item T) { m.Insert(len(m.items), item) }

// Insert adds item at i and notifies the refresh.
func (m *TypedItems[T]) Insert(i int, item T) {
	m.items = slices.Insert(m.items, i, item)
	m.Notify()
}

// InsertSorted adds item at the position cmp keeps the slice sorted
// at (the first among equals) and returns where it landed.
func (m *TypedItems[T]) InsertSorted(item T, cmp func(a, b T) int) int {
	i, _ := slices.BinarySearchFunc(m.items, item, cmp)
	m.Insert(i, item)
	return i
}

// RemoveAt drops item i and notifies the refresh.
func (m *TypedItems[T]) RemoveAt(i int) {
	m.items = slices.Delete(m.items, i, i+1)
	m.Notify()
}

// Clear drops every item and notifies the refresh.
func (m *TypedItems[T]) Clear() {
	m.items = m.items[:0]
	m.Notify()
}

// SetRefresh installs the wholesale-change hook Notify runs.
func (m *TypedItems[T]) SetRefresh(fn func()) { m.refresh = fn }

// Notify reports the model changed; the view recomputes - resorting,
// refiltering and rebinding in the process.
func (m *TypedItems[T]) Notify() {
	if m.refresh != nil {
		m.refresh()
	}
}

// SetItemRefresh installs the per-item hook NotifyItem runs.
func (m *TypedItems[T]) SetItemRefresh(fn func(i int)) { m.itemRefresh = fn }

// NotifyItem reports item i was edited in place; a visible row rebinds
// to the new value, and nothing else moves.
func (m *TypedItems[T]) NotifyItem(i int) {
	if m.itemRefresh != nil {
		m.itemRefresh(i)
	}
}

// ViewFilter is one filter in a viewModel's stack: a predicate over
// items, on or off. AddFilter hands one out; SetFilterStatus toggles
// it without dropping it.
type ViewFilter[T any] struct {
	keep func(T) bool
	on   bool
}

// Enabled reports whether the filter currently applies.
func (f *ViewFilter[T]) Enabled() bool { return f.on }

// viewModel is the typed model layer every typed view shares, #166's
// generalization of the table's permutation: a stable sort and a stack
// of toggleable filters over a ColumnModel, materialized as an order
// of the source's row indices. Sorting and filtering never touch the
// source.
type viewModel[T any] struct {
	src     ColumnModel[T]
	sortFn  func(a, b T) int
	filters []*ViewFilter[T]
	order   []int
}

// setModel replaces the data source.
func (v *viewModel[T]) setModel(m ColumnModel[T]) { v.src = m }

// setSort orders the view by cmp (nil returns to the source order);
// equal items keep source order.
func (v *viewModel[T]) setSort(cmp func(a, b T) int) { v.sortFn = cmp }

// addFilter appends a filter, on, and returns its handle.
func (v *viewModel[T]) addFilter(keep func(T) bool) *ViewFilter[T] {
	f := &ViewFilter[T]{keep: keep, on: true}
	v.filters = append(v.filters, f)
	return f
}

// setFilterStatus turns a filter on or off in place.
func (v *viewModel[T]) setFilterStatus(f *ViewFilter[T], on bool) {
	if f != nil {
		f.on = on
	}
}

// popFilter drops the most recently added filter.
func (v *viewModel[T]) popFilter() {
	if n := len(v.filters); n > 0 {
		v.filters = v.filters[:n-1]
	}
}

// clearFilters drops every filter.
func (v *viewModel[T]) clearFilters() { v.filters = nil }

// len is the visible item count.
func (v *viewModel[T]) len() int { return len(v.order) }

// at returns the item at visible index i.
func (v *viewModel[T]) at(i int) T {
	var zero T
	if v.src == nil || i < 0 || i >= len(v.order) {
		return zero
	}
	return v.src.Row(v.order[i])
}

// source maps a visible index to its source index, -1 when out of
// range.
func (v *viewModel[T]) source(i int) int {
	if i < 0 || i >= len(v.order) {
		return -1
	}
	return v.order[i]
}

// visible maps a source index to its visible index, -1 when the item
// is filtered out or gone.
func (v *viewModel[T]) visible(src int) int {
	return slices.Index(v.order, src)
}

// keeps reports whether the source's item i passes every enabled
// filter.
func (v *viewModel[T]) keeps(i int) bool {
	item := v.src.Row(i)
	for _, f := range v.filters {
		if f.on && !f.keep(item) {
			return false
		}
	}
	return true
}

// rebuild recomputes the visible order: filters narrow, the sort
// orders what is left, stably.
func (v *viewModel[T]) rebuild() {
	v.order = v.order[:0]
	if v.src == nil {
		return
	}
	n := v.src.Len()
	v.order = slices.Grow(v.order, n)
	for i := range n {
		if v.keeps(i) {
			v.order = append(v.order, i)
		}
	}
	if v.sortFn != nil {
		slices.SortStableFunc(v.order, func(i, j int) int {
			return v.sortFn(v.src.Row(i), v.src.Row(j))
		})
	}
}

// TypedFactory is how a typed view materializes rows, relm4's
// RelmListItem/RelmGridItem: Setup builds a row once, Bind fills it
// for one item every time it scrolls into view, Unbind releases it
// when it scrolls out. Rows recycle: only the visible ones exist, and
// Setup runs about as often as the viewport holds rows.
type TypedFactory[T any] struct {
	Setup func() Widget
	Bind  func(item T, row Widget)
	// Unbind, when set, runs when a row's item scrolls out; the row
	// goes back to the pool and is Bound to whatever scrolls in next.
	Unbind func(item T, row Widget)
}

// typedRow is one recycled row: the widget Setup built and the item it
// is currently bound to.
type typedRow[T any] struct {
	w    Widget
	item T
}

// typedRows adapts a viewModel to ListModel with recycling: Row hands
// out pool rows bound to the asked item, unbind returns them to the
// pool when the List evicts their index.
type typedRows[T any] struct {
	core *typedCore[T]
	live map[int]*typedRow[T]
	pool []*typedRow[T]
}

// Len implements ListModel.
func (r *typedRows[T]) Len() int { return r.core.vm.len() }

// Row implements ListModel: a pooled (or fresh) row bound to item i.
func (r *typedRows[T]) Row(i int) Widget {
	if row, ok := r.live[i]; ok {
		return row.w
	}
	var row *typedRow[T]
	if n := len(r.pool); n > 0 {
		row, r.pool = r.pool[n-1], r.pool[:n-1]
	} else {
		row = &typedRow[T]{w: r.core.factory.Setup()}
	}
	row.item = r.core.vm.at(i)
	r.core.factory.Bind(row.item, row.w)
	r.live[i] = row
	return row.w
}

// unbind implements the List's recycling hook: the row leaves the
// viewport, Unbind hears its item, and it joins the pool.
func (r *typedRows[T]) unbind(i int) {
	row, ok := r.live[i]
	if !ok {
		return
	}
	delete(r.live, i)
	if r.core.factory.Unbind != nil {
		r.core.factory.Unbind(row.item, row.w)
	}
	r.pool = append(r.pool, row)
}

// typedCore is what TypedList and TypedGrid share: the view model, the
// recycling adapter over it, the underlying List, and a typed
// selection tracked by source index so it survives sorting and
// filtering.
type typedCore[T any] struct {
	vm        viewModel[T]
	factory   TypedFactory[T]
	rows      *typedRows[T]
	list      *List
	sel       map[int]struct{}
	syncing   bool
	changed   func(items []T)
	activated func(item T)
}

// init binds the core, its recycling adapter and the list together.
func (c *typedCore[T]) init(l *List, model ColumnModel[T], factory TypedFactory[T], rowHeight int) {
	c.vm.setModel(model)
	c.factory = factory
	c.rows = &typedRows[T]{core: c, live: map[int]*typedRow[T]{}}
	c.list = l
	c.sel = map[int]struct{}{}
	initList(l, c.rows, rowHeight)
	l.OnSelectionChanged = c.listSelectionChanged
	l.OnActivate = c.listActivated
	c.vm.rebuild()
}

// recompute rebuilds the order and everything under it, carrying the
// typed selection across: the selected items keep their rows, or lose
// them when a filter hides them.
func (c *typedCore[T]) recompute() {
	c.syncing = true
	had := c.sources()
	n := 0
	if c.vm.src != nil {
		n = c.vm.src.Len()
	}
	c.list.Reset()
	c.vm.rebuild()
	c.reapply(n)
	c.list.Changed()
	c.syncing = false
	if after := c.sources(); !slices.Equal(had, after) && c.changed != nil {
		c.changed(c.items())
	}
}

// reapply translates the tracked source selection back into visible
// rows after a rebuild: hidden items drop out, the rest stay selected
// wherever they landed.
func (c *typedCore[T]) reapply(n int) {
	kept := make(map[int]struct{}, len(c.sel))
	for src := range c.sel {
		if src < n {
			kept[src] = struct{}{}
		}
	}
	c.sel = kept
	vis := make(map[int]struct{}, len(c.sel))
	for src := range c.sel {
		if i := c.vm.visible(src); i >= 0 {
			vis[i] = struct{}{}
		}
	}
	sel := -1
	for i := range vis {
		if sel < 0 || i < sel {
			sel = i
		}
	}
	s := &c.list.selection
	if s.mode == SelectionMultiple {
		s.multi = vis
		s.sel, s.cursor = sel, sel
		c.sel = map[int]struct{}{}
		for i := range vis {
			c.sel[c.vm.source(i)] = struct{}{}
		}
	} else {
		s.sel, s.cursor = -1, -1
		c.sel = map[int]struct{}{}
		if sel >= 0 {
			s.sel, s.cursor = sel, sel
			c.sel[c.vm.source(sel)] = struct{}{}
		}
	}
	c.list.selectionSync()
}

// sources is the tracked selection as ascending source indices.
func (c *typedCore[T]) sources() []int {
	out := make([]int, 0, len(c.sel))
	for src := range c.sel {
		out = append(out, src)
	}
	slices.Sort(out)
	return out
}

// items is the tracked selection as items, in visible order.
func (c *typedCore[T]) items() []T {
	out := make([]T, 0, len(c.sel))
	for _, src := range c.vm.order {
		if _, ok := c.sel[src]; ok {
			out = append(out, c.vm.src.Row(src))
		}
	}
	return out
}

// listSelectionChanged tracks the List's selection in source indices
// and reports it as items; the sync around a recompute is the view's
// own, not the user's, and stays silent.
func (c *typedCore[T]) listSelectionChanged(vis []int) {
	if c.syncing {
		return
	}
	c.sel = map[int]struct{}{}
	for _, i := range vis {
		if src := c.vm.source(i); src >= 0 {
			c.sel[src] = struct{}{}
		}
	}
	if c.changed != nil {
		c.changed(c.items())
	}
}

// listActivated reports a row activation as its item.
func (c *typedCore[T]) listActivated(i int) {
	if c.activated == nil || i < 0 || i >= c.vm.len() {
		return
	}
	c.activated(c.vm.at(i))
}

// itemChanged rebinds the row showing an item edited in place, when
// one is showing.
func (c *typedCore[T]) itemChanged(src int) {
	i := c.vm.visible(src)
	if i < 0 {
		return
	}
	if row, ok := c.rows.live[i]; ok {
		row.item = c.vm.at(i)
		c.factory.Bind(row.item, row.w)
	}
}

// TypedList is a virtualized list over typed items, relm4's
// TypedListView: the model layer sorts and filters without touching
// the source, rows are built once by the factory's Setup and filled
// per item by Bind (Unbind on the way out), so only the visible rows
// exist, and selection is typed - it reports items and follows them
// through sorting and filtering. List underneath does what List is
// for: SetSelectionMode, Select, keyboard and scrolling come from it
// and act on visible rows.
type TypedList[T any] struct {
	List
	core typedCore[T]
	// OnSelectionChanged fires with the selected items, in visible
	// order, once per gesture.
	OnSelectionChanged func(items []T)
	// OnActivate fires with the activated row's item.
	OnActivate func(item T)
}

// NewTypedList returns a list over model whose rows the factory builds
// and fills; rowHeight is the uniform row height in pixels, zero to
// derive it from the first row.
func NewTypedList[T any](model ColumnModel[T], factory TypedFactory[T], rowHeight int) *TypedList[T] {
	l := &TypedList[T]{}
	l.core.init(&l.List, model, factory, rowHeight)
	l.core.changed = func(items []T) {
		if l.OnSelectionChanged != nil {
			l.OnSelectionChanged(items)
		}
	}
	l.core.activated = func(item T) {
		if l.OnActivate != nil {
			l.OnActivate(item)
		}
	}
	return l
}

// Rows reports the visible item count, post sort/filter.
func (l *TypedList[T]) Rows() int { return l.core.vm.len() }

// Row returns the data of visible row i, post sort/filter.
func (l *TypedList[T]) Row(i int) T { return l.core.vm.at(i) }

// SetModel replaces the data source, keeping the sort and the filters.
func (l *TypedList[T]) SetModel(m ColumnModel[T]) {
	l.core.vm.setModel(m)
	l.core.recompute()
}

// Refresh re-reads the model: the order is rebuilt under the current
// sort and filters and the visible rows rebind. The hook a
// TypedItems model reports wholesale changes through.
func (l *TypedList[T]) Refresh() { l.core.recompute() }

// ItemChanged rebinds the row showing source item i, when one is
// showing. The hook a TypedItems model reports an in-place edit
// through.
func (l *TypedList[T]) ItemChanged(i int) { l.core.itemChanged(i) }

// SetSort orders the view by cmp (nil returns to the source order);
// equal items keep source order. The sort applies now and to every
// later rebuild, so items appended or inserted land where cmp puts
// them.
func (l *TypedList[T]) SetSort(cmp func(a, b T) int) {
	l.core.vm.setSort(cmp)
	l.core.recompute()
}

// AddFilter appends a filter to the stack and returns its handle;
// every enabled filter must accept an item for it to show.
func (l *TypedList[T]) AddFilter(keep func(T) bool) *ViewFilter[T] {
	f := l.core.vm.addFilter(keep)
	l.core.recompute()
	return f
}

// SetFilterStatus turns a filter AddFilter returned on or off without
// dropping it.
func (l *TypedList[T]) SetFilterStatus(f *ViewFilter[T], on bool) {
	l.core.vm.setFilterStatus(f, on)
	l.core.recompute()
}

// PopFilter drops the most recently added filter.
func (l *TypedList[T]) PopFilter() {
	l.core.vm.popFilter()
	l.core.recompute()
}

// ClearFilters drops every filter.
func (l *TypedList[T]) ClearFilters() {
	l.core.vm.clearFilters()
	l.core.recompute()
}

// SelectedItems returns the selection as items, in visible order.
func (l *TypedList[T]) SelectedItems() []T { return l.core.items() }

// SelectedItem returns the selected item; in multiple mode the anchor
// the last gesture settled on, false when nothing is selected.
func (l *TypedList[T]) SelectedItem() (item T, ok bool) {
	items := l.core.items()
	if len(items) == 0 {
		var zero T
		return zero, false
	}
	return items[0], true
}

// TypedGrid is a virtualized grid over typed items, relm4's
// TypedGridView: a TypedList flowing left to right in as many columns
// of the cell width as the width fits, recycling, sorting, filters and
// typed selection all inherited. It styles as `gridview` with `child`
// cells.
type TypedGrid[T any] struct {
	TypedList[T]
}

// NewTypedGrid returns a grid over model with cells of at least
// cellWidth by cellHeight pixels, between 1 and 7 columns (GTK's
// defaults).
func NewTypedGrid[T any](model ColumnModel[T], factory TypedFactory[T], cellWidth, cellHeight int) *TypedGrid[T] {
	g := &TypedGrid[T]{}
	g.core.init(&g.List, model, factory, cellHeight)
	g.SetElement("gridview")
	g.rowElement = "child"
	g.cellW = max(1, cellWidth)
	g.minCols, g.maxCols = 1, 7
	return g
}

// SetMinColumns sets the fewest columns the grid lays out, even when
// cells end up narrower than their width.
func (g *TypedGrid[T]) SetMinColumns(n int) {
	if n != g.minCols {
		g.minCols = max(1, n)
		g.InvalidateLayout()
	}
}

// SetMaxColumns sets the most columns the grid lays out; zero lifts
// the cap.
func (g *TypedGrid[T]) SetMaxColumns(n int) {
	if n != g.maxCols {
		g.maxCols = max(0, n)
		g.InvalidateLayout()
	}
}

// MinColumns returns the column floor.
func (g *TypedGrid[T]) MinColumns() int { return g.minCols }

// MaxColumns returns the column cap, zero for none.
func (g *TypedGrid[T]) MaxColumns() int { return g.maxCols }

// Columns returns the column count of the last layout.
func (g *TypedGrid[T]) Columns() int { return g.columns() }
