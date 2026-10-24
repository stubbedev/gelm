package ui_test

import (
	"testing"

	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

// typedFactory builds stub rows the builders can fill.
func typedFactory() widget.TypedFactory[int] {
	return widget.TypedFactory[int]{
		Setup: func() widget.Widget { return widget.NewBox(widget.Row, 0, 0) },
		Bind:  func(item int, row widget.Widget) {},
	}
}

// TestTypedListBuilderBuildsTheView pins the typed list's builder: the
// constructor, a sort, a filter and the typed selection hooks all land
// on the built view.
func TestTypedListBuilderBuildsTheView(t *testing.T) {
	var built *widget.TypedList[int]
	var selected []int
	var activated int
	root, _ := ui.Build(env(t), ui.TypedList[int](widget.NewTypedItems(3, 1, 2), typedFactory(), 20).Ref(&built).
		Sort(func(a, b int) int { return a - b }).
		AddFilter(func(i int) bool { return i != 1 }).
		SelectionMode(widget.SelectionMultiple).
		OnSelectionChanged(func(items []int) { selected = items }).
		OnActivate(func(item int) { activated = item }))
	if root != widget.Widget(built) {
		t.Fatalf("root = %T, want the list itself", root)
	}
	if built.Rows() != 2 || built.Row(0) != 2 || built.Row(1) != 3 {
		t.Fatalf("built rows: %d wide, row0 = %d", built.Rows(), built.Row(0))
	}
	built.Select(0)
	if len(selected) != 1 || selected[0] != 2 {
		t.Fatalf("typed selection reported %v, want [2]", selected)
	}
	built.Select(1)
	built.KeyAction(widget.KeyEnter, 0)
	if activated != 3 {
		t.Fatalf("typed activation reported %d, want 3", activated)
	}
}

// TestTypedGridBuilderBuildsTheView pins the typed grid's builder.
func TestTypedGridBuilderBuildsTheView(t *testing.T) {
	var built *widget.TypedGrid[int]
	root, _ := ui.Build(env(t), ui.TypedGrid[int](widget.NewTypedItems(1, 2, 3), typedFactory(), 50, 40).Ref(&built).
		MinColumns(2).
		MaxColumns(4))
	if root != widget.Widget(built) {
		t.Fatalf("root = %T, want the grid itself", root)
	}
	if built.Rows() != 3 || built.MinColumns() != 2 || built.MaxColumns() != 4 {
		t.Fatalf("built grid: %d rows, columns [%d,%d]", built.Rows(), built.MinColumns(), built.MaxColumns())
	}
	if built.Element() != "gridview" {
		t.Errorf("element %q, want gridview", built.Element())
	}
}
