package widget

import (
	"io"
	"slices"

	"github.com/stubbedev/gelm/render"
)

// ColumnView is the virtualized table (#90): typed columns over a
// ColumnModel, a header row with sort indicators, sort and filter in
// one stable permutation, tree indentation through the tree helpers,
// and optional row drag reordering. List underneath does what List is
// for - virtualization, selection, keyboard - through the same model
// adapter, so a million rows cost about what twenty do.
//
// The model layer is the GIO list-model analog: sorting and filtering
// never touch the source model, they live in a permutation of its row
// indices - stable (equal rows keep source order), recomputed on
// SortBy/SetFilter/SetModel, and the source's own order is one call
// away.
type ColumnView[T any] struct {
	composite
	face   render.Font
	sizePx float64

	cols   []TableColumn[T]
	model  ColumnModel[T]
	filter func(T) bool
	sortBy int
	sortUp bool
	perm   rowPermutation

	header *Box
	list   *List
	rows   *columnRows[T]

	// OnRowReorder, when set, receives drag-and-drop row moves: from
	// and to are visible indices (post sort/filter), fired on the loop
	// goroutine. The application performs the move in its model - the
	// view never mutates data it does not own.
	OnRowReorder func(from, to int)
	dragRows     bool
}

// TableColumn is one typed table column: how a cell renders, how wide
// it sits, and whether the header sorts by it.
type TableColumn[T any] struct {
	Title string
	// Width is the cell's minimum width (the header's too, so columns
	// line up); Expand shares leftover table width across the
	// expanding columns, Box-style.
	Width  int
	Expand bool
	// Cell builds the cell's widget for one row value; nil renders the
	// zero label.
	Cell func(T) Widget
	// Sort, when non-nil, makes the header clickable and orders rows:
	// negative puts a first. Equal values keep source order (stable).
	Sort func(a, b T) int
}

// ColumnModel is the table's data source, ListModel's typed sibling.
type ColumnModel[T any] interface {
	Len() int
	Row(i int) T
}

// SliceModel adapts a slice as a ColumnModel.
type SliceModel[T any] []T

// Len implements ColumnModel.
func (s SliceModel[T]) Len() int { return len(s) }

// Row implements ColumnModel.
func (s SliceModel[T]) Row(i int) T { return s[i] }

// NewColumnView returns a view over model with the given columns.
func NewColumnView[T any](face render.Font, sizePx float64, model ColumnModel[T], cols []TableColumn[T]) *ColumnView[T] {
	face = requireFace("widget.NewColumnView", face)
	v := &ColumnView[T]{
		face: face, sizePx: sizePx,
		cols: cols, model: model, sortBy: -1,
	}
	v.rows = &columnRows[T]{view: v}
	v.list = NewList(v.rows, int(sizePx)+12)
	v.header = NewBox(Row, 0, 0)
	column := NewBox(Column, 0, 0)
	column.Append(v.header, false)
	column.Append(v.list, true)
	v.initComposite(v, column)
	v.buildHeader()
	v.recompute()
	return v
}

// List exposes the underlying list - selection, activation, keyboard,
// scrolling - the same surface a plain List gives.
func (v *ColumnView[T]) List() *List { return v.list }

// Row returns the data of visible row i (post sort/filter).
func (v *ColumnView[T]) Row(i int) T {
	if i < 0 || i >= len(v.perm.order) || v.model == nil {
		var zero T
		return zero
	}
	return v.model.Row(v.perm.order[i])
}

// Rows reports the visible row count.
func (v *ColumnView[T]) Rows() int { return len(v.perm.order) }

// SetModel replaces the data source and drops the sort/filter
// permutation back to the source order.
func (v *ColumnView[T]) SetModel(m ColumnModel[T]) {
	v.model = m
	v.sortBy = -1
	v.buildHeader()
	v.recompute()
}

// SortBy sorts by column col (descending with desc); col -1 returns
// to the source order. A column without a Sort function clears the
// sort instead.
func (v *ColumnView[T]) SortBy(col int, desc bool) {
	if col < 0 || col >= len(v.cols) || v.cols[col].Sort == nil {
		v.sortBy = -1
		v.sortUp = false
	} else {
		v.sortBy = col
		v.sortUp = !desc
	}
	v.buildHeader()
	v.recompute()
}

// SortedBy reports the active sort column (-1 when none) and its
// direction.
func (v *ColumnView[T]) SortedBy() (col int, ascending bool) { return v.sortBy, v.sortUp }

// SetFilter narrows the view to rows keep accepts (nil clears); the
// source model is untouched and the sort stays.
func (v *ColumnView[T]) SetFilter(keep func(T) bool) {
	v.filter = keep
	v.recompute()
}

// EnableRowDrag turns row drag-and-drop on: rows become drag sources
// offering the internal reorder mime, and a row dropped on another
// fires OnRowReorder with the two visible indices.
func (v *ColumnView[T]) EnableRowDrag(on bool) {
	v.dragRows = on
	v.recompute()
}

// buildHeader rebuilds the header row: one button per column, the
// active sort column carrying its arrow, every cell held to its
// column's width so rows and headers line up.
func (v *ColumnView[T]) buildHeader() {
	v.header.Clear()
	th := Current()
	for i, col := range v.cols {
		title := NewBox(Row, 4, 0)
		title.AppendAligned(NewLabel(v.face, v.sizePx-1, col.Title, th.TextMuted), false, AlignCenter)
		if i == v.sortBy {
			arrow := SymbolChevronDown
			if v.sortUp {
				arrow = SymbolChevronUp
			}
			title.AppendAligned(NewSymbol(arrow, int(v.sizePx)), false, AlignCenter)
		}
		btn := NewButton(title, headerInset, 2)
		if col.Sort != nil {
			which := i
			btn.OnClick = func() { v.cycleSort(which) }
		}
		v.header.AppendAligned(newSizedCell(btn, col.Width), col.Expand, AlignFill)
	}
}

// cycleSort walks a column through ascending, descending, off.
func (v *ColumnView[T]) cycleSort(col int) {
	switch {
	case v.sortBy != col:
		v.SortBy(col, false)
	case v.sortUp:
		v.SortBy(col, true)
	default:
		v.SortBy(-1, false)
	}
}

// recompute rebuilds the permutation and tells the List.
func (v *ColumnView[T]) recompute() {
	n := 0
	if v.model != nil {
		n = v.model.Len()
	}
	var cmp func(i, j int) int
	if v.sortBy >= 0 && v.sortBy < len(v.cols) && v.cols[v.sortBy].Sort != nil && v.model != nil {
		less := v.cols[v.sortBy].Sort
		desc := !v.sortUp
		cmp = func(i, j int) int {
			c := less(v.model.Row(i), v.model.Row(j))
			if desc {
				return -c
			}
			return c
		}
	}
	var keep func(i int) bool
	if v.filter != nil && v.model != nil {
		f := v.filter
		keep = func(i int) bool { return f(v.model.Row(i)) }
	}
	v.perm.rebuild(n, cmp, keep)
	v.rows.set()
	v.list.Reset()
	v.InvalidateLayout()
}

// rowPermutation is the sort/filter model layer: a stable ordering of
// the source indices - SortListModel's shape without the interface
// ceremony, one slice, rebuilt, indexed.
type rowPermutation struct {
	order []int
}

// rebuild recomputes the visible order of n source rows: keep filters
// (nil keeps all), cmp sorts (nil keeps source order), stably.
func (p *rowPermutation) rebuild(n int, cmp func(i, j int) int, keep func(i int) bool) {
	p.order = p.order[:0]
	for i := range n {
		if keep == nil || keep(i) {
			p.order = append(p.order, i)
		}
	}
	if cmp != nil {
		slices.SortStableFunc(p.order, cmp)
	}
}

// columnRows adapts the view as a ListModel: each visible row builds a
// horizontal Box of cells, draggable when row drag is on.
type columnRows[T any] struct {
	view  *ColumnView[T]
	built []Widget
}

// Len implements ListModel.
func (r *columnRows[T]) Len() int { return r.view.Rows() }

// Row implements ListModel; rows are built once per permutation and
// cached until the next recompute.
func (r *columnRows[T]) Row(i int) Widget {
	if i < 0 || i >= len(r.view.perm.order) {
		return nil
	}
	for len(r.built) < i+1 {
		r.built = append(r.built, nil)
	}
	if r.built[i] == nil {
		r.built[i] = r.view.buildRow(i)
	}
	return r.built[i]
}

// set drops the row cache on recompute.
func (r *columnRows[T]) set() { r.built = r.built[:0] }

// buildRow renders visible row i: the cells in column order, widths
// held by the same spacer trick the header uses, draggable rows
// wrapped in the reorder source.
func (v *ColumnView[T]) buildRow(i int) Widget {
	val := v.Row(i)
	row := NewBox(Row, 0, 0)
	for _, col := range v.cols {
		var cell Widget = NewLabel(v.face, v.sizePx, "", Current().Text)
		if col.Cell != nil {
			cell = col.Cell(val)
		}
		// The inset matches the header button's padding, so a cell's
		// content starts under its column title.
		inner := NewBox(Row, 0, 0)
		inner.Append(NewSpacer(headerInset, 0), false)
		inner.AppendAligned(cell, false, AlignCenter)
		row.AppendAligned(newSizedCell(inner, col.Width), col.Expand, AlignFill)
	}
	if !v.dragRows {
		return row
	}
	return &dragRow[T]{Box: row, view: v, at: i}
}

// headerInset is the header buttons' padding and the row cells' inset.
const headerInset = 6

// rowReorderMime is the internal drag mime for row moves.
const rowReorderMime = "application/x-gelm-row"

// dragRow makes a table row a drag source and drop target for
// reordering: dragging offers the row's visible index, dropping on
// another row fires the view's OnRowReorder.
type dragRow[T any] struct {
	*Box
	view *ColumnView[T]
	at   int
}

// DragContent implements DragSource.
func (d *dragRow[T]) DragContent() *DragContent {
	return &DragContent{
		Mimes: []string{rowReorderMime},
		Write: func(mime string, w io.Writer) error {
			_, err := w.Write([]byte(rowKey(d.at)))
			return err
		},
	}
}

// DragEnter implements DragEnterer.
func (d *dragRow[T]) DragEnter(mimes []string, _ Point) string {
	if slices.Contains(mimes, rowReorderMime) {
		return rowReorderMime
	}
	return ""
}

// Drop implements Dropper: the move lands as visible indices.
func (d *dragRow[T]) Drop(mime string, data []byte, _ Point) {
	if mime != rowReorderMime || d.view.OnRowReorder == nil {
		return
	}
	from := parseRowKey(string(data))
	if from >= 0 {
		d.view.OnRowReorder(from, d.at)
	}
}

// rowKey encodes a visible index as the drag payload.
func rowKey(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [12]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

// parseRowKey decodes one; anything else is -1.
func parseRowKey(s string) int {
	if s == "" {
		return -1
	}
	n := 0
	for i := 0; i < len(s); i++ { //nolint:intrange // the index dance is the point here
		if s[i] < '0' || s[i] > '9' {
			return -1
		}
		n = n*10 + int(s[i]-'0')
	}
	return n
}
