package component_test

import (
	"testing"

	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/component/componenttest"
	"github.com/stubbedev/lys-ui/widget"
)

func TestListBoxViewFollowsEdits(t *testing.T) {
	var loop componenttest.Loop
	lb := widget.NewListBox()
	f := component.NewFactory(&loop, component.ListBoxView[*row](lb))
	rows := fill(t, f, "a", "b", "c")
	if lb.Len() != 3 || lb.RowAt(0).Child() != rows[0].label || lb.RowAt(2).Index() != 2 {
		t.Fatalf("appends: len=%d first=%v", lb.Len(), lb.RowAt(0).Child())
	}
	f.Move(0, 2)
	if lb.RowAt(2).Child() != rows[0].label || lb.RowAt(0).Child() != rows[1].label || lb.RowAt(1).Child() != rows[2].label {
		t.Fatal("the list did not follow the move")
	}
	f.Remove(1)
	if lb.Len() != 2 || lb.RowAt(0).Child() != rows[1].label || lb.RowAt(1).Child() != rows[0].label {
		t.Fatalf("after removing b: %d rows, first=%v", lb.Len(), lb.RowAt(0).Child())
	}
	f.Insert(0, &row{face: face(t), name: "z"})
	if _, ok := lb.RowAt(0).Child().(*widget.Label); !ok || lb.Len() != 3 {
		t.Fatal("Insert through the view")
	}
}
