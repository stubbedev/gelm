package ui_test

import (
	"testing"

	"github.com/stubbedev/lys-ui/ui"
	"github.com/stubbedev/lys-ui/widget"
)

func TestListBoxBuilderBuildsTheSameTree(t *testing.T) {
	activated := -1
	var built *widget.ListBox
	root, _ := ui.Build(env(t), ui.ListBox().Ref(&built).
		Append(ui.Label("one")).
		Insert(0, ui.Label("zero")).
		SelectionMode(widget.SelectionMultiple).
		SingleClickActivate(false).
		Placeholder(ui.Label("empty")).
		OnActivate(func(i int) { activated = i }))
	if root != widget.Widget(built) {
		t.Fatalf("root = %T, want the list itself", root)
	}
	manual := widget.NewListBox()
	manual.Append(widget.NewLabel(env(t).Face, env(t).Size, "one", widget.Current().Text))
	manual.Insert(0, widget.NewLabel(env(t).Face, env(t).Size, "zero", widget.Current().Text))
	manual.SetSelectionMode(widget.SelectionMultiple)
	manual.SetSingleClickActivate(false)
	manual.SetPlaceholder(widget.NewLabel(env(t).Face, env(t).Size, "empty", widget.Current().Text))
	if built.Len() != manual.Len() || built.SelectionMode() != manual.SelectionMode() {
		t.Fatalf("built len=%d mode=%v, want %d %v", built.Len(), built.SelectionMode(), manual.Len(), manual.SelectionMode())
	}
	if a, b := built.RowAt(0).Child(), manual.RowAt(0).Child(); a.(*widget.Label).Text() != b.(*widget.Label).Text() {
		t.Fatalf("row 0: %q vs %q", a.(*widget.Label).Text(), b.(*widget.Label).Text())
	}
	if a, b := built.RowAt(1).Child(), manual.RowAt(1).Child(); a.(*widget.Label).Text() != b.(*widget.Label).Text() {
		t.Fatalf("row 1: %q vs %q", a.(*widget.Label).Text(), b.(*widget.Label).Text())
	}
	built.OnActivate(2)
	if activated != 2 {
		t.Fatalf("the built OnActivate hook: %d", activated)
	}
	if !built.HasClass("boxed-list") || built.Len() != 2 {
		t.Fatal("the builder's list lost its classes or rows")
	}
}
