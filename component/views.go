package component

import (
	"fmt"
	"slices"

	"github.com/stubbedev/gelm/widget"
)

// FactoryView places a Factory's item widgets in a container, relm4's
// FactoryView. Its container holds the factory's items and nothing
// else, so positions line up. Implement it to render into any
// container.
type FactoryView[C any] interface {
	Insert(i int, item C, w widget.Widget)
	Remove(i int, item C, w widget.Widget)
	Move(from, to int, item C, w widget.Widget)
}

type boxView[C any] struct {
	box    *widget.Box
	expand bool
}

// BoxView renders items as the children of a Box (or a list of rows: a
// Column inside a Scroll), each expanding on the box's axis when expand
// is set.
func BoxView[C any](b *widget.Box, expand bool) FactoryView[C] {
	return boxView[C]{box: b, expand: expand}
}

func (v boxView[C]) Insert(i int, _ C, w widget.Widget) { v.box.InsertAt(i, w, v.expand) }
func (v boxView[C]) Remove(i int, _ C, _ widget.Widget) { v.box.RemoveAt(i) }
func (v boxView[C]) Move(from, to int, _ C, _ widget.Widget) {
	v.box.Move(from, to)
}

type flowBoxView[C any] struct{ flow *widget.FlowBox }

// FlowBoxView renders items as the children of a FlowBox; selection
// follows moved items.
func FlowBoxView[C any](f *widget.FlowBox) FactoryView[C] { return flowBoxView[C]{flow: f} }

func (v flowBoxView[C]) Insert(i int, _ C, w widget.Widget) { v.flow.Insert(i, w) }
func (v flowBoxView[C]) Remove(i int, _ C, _ widget.Widget) { v.flow.RemoveAt(i) }
func (v flowBoxView[C]) Move(from, to int, _ C, _ widget.Widget) {
	v.flow.Move(from, to)
}

type stackView[C any] struct {
	stack *widget.Stack
	name  func(C) string
	names map[widget.Widget]string
}

// StackView renders items as the pages of a Stack, each under the
// name name returns for it when inserted. Names must be unique.
func StackView[C any](s *widget.Stack, name func(C) string) FactoryView[C] {
	return &stackView[C]{stack: s, name: name, names: map[widget.Widget]string{}}
}

func (v *stackView[C]) Insert(i int, item C, w widget.Widget) {
	name := v.name(item)
	if slices.Contains(v.stack.Order(), name) {
		panic(fmt.Sprintf("component: StackView page name %q is already taken", name))
	}
	v.names[w] = name
	v.stack.Insert(i, name, w)
}

func (v *stackView[C]) Remove(_ int, _ C, w widget.Widget) {
	v.stack.Remove(v.names[w])
	delete(v.names, w)
}

func (v *stackView[C]) Move(_, to int, _ C, w widget.Widget) { v.stack.Move(v.names[w], to) }

type notebookView[C any] struct {
	notebook *widget.Notebook
	title    func(C) string
}

// NotebookView renders items as the tabs of a Notebook, each titled by
// title when inserted.
func NotebookView[C any](n *widget.Notebook, title func(C) string) FactoryView[C] {
	return notebookView[C]{notebook: n, title: title}
}

func (v notebookView[C]) Insert(i int, item C, w widget.Widget) {
	v.notebook.InsertTab(i, v.title(item), w)
}
func (v notebookView[C]) Remove(i int, _ C, _ widget.Widget) { v.notebook.CloseTabAt(i) }
func (v notebookView[C]) Move(from, to int, _ C, _ widget.Widget) {
	v.notebook.MoveTab(from, to)
}

type gridView[C any] struct {
	grid  *widget.Grid
	place func(i int) (col, row, colSpan, rowSpan int)
	cells []widget.Widget
}

// GridView renders items into a Grid, item i at the cell place(i)
// returns; items re-place in place as positions shift.
func GridView[C any](g *widget.Grid, place func(i int) (col, row, colSpan, rowSpan int)) FactoryView[C] {
	return &gridView[C]{grid: g, place: place}
}

func (v *gridView[C]) Insert(i int, _ C, w widget.Widget) {
	v.cells = slices.Insert(v.cells, i, w)
	v.replace(i+1, len(v.cells))
	col, row, cs, rs := v.place(i)
	v.grid.Attach(w, col, row, cs, rs)
}

func (v *gridView[C]) Remove(i int, _ C, w widget.Widget) {
	v.grid.Remove(w)
	v.cells = slices.Delete(v.cells, i, i+1)
	v.replace(i, len(v.cells))
}

func (v *gridView[C]) Move(from, to int, _ C, w widget.Widget) {
	v.cells = slices.Insert(slices.Delete(v.cells, from, from+1), to, w)
	v.replace(min(from, to), max(from, to)+1)
}

func (v *gridView[C]) replace(from, to int) {
	for i := from; i < to; i++ {
		col, row, cs, rs := v.place(i)
		v.grid.Place(v.cells[i], col, row, cs, rs)
	}
}
