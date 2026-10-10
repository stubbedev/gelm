package component_test

import (
	"slices"
	"testing"

	"github.com/stubbedev/gelm/component"
	"github.com/stubbedev/gelm/component/componenttest"
	"github.com/stubbedev/gelm/widget"
)

func TestKeyedFactoryAddressesItemsByKey(t *testing.T) {
	var loop componenttest.Loop
	box := widget.NewBox(widget.Column, 0, 0)
	f := component.NewKeyedFactory[string](&loop, component.BoxView[*row](box, false))
	var stopped []string
	var heard []string
	f.Forward(func(k string, o rowOut) { heard = append(heard, k) })
	for _, k := range []string{"carol", "alice", "bob"} {
		f.Insert(k, &row{face: face(t), name: k, stopped: &stopped})
	}
	order := func() []string {
		var out []string
		for k := range f.All() {
			out = append(out, k)
		}
		return out
	}
	if got := order(); !slices.Equal(got, []string{"carol", "alice", "bob"}) {
		t.Fatalf("insertion order %v", got)
	}

	f.Insert("alice", &row{face: face(t), name: "alice2", stopped: &stopped})
	if got := order(); !slices.Equal(got, []string{"carol", "alice", "bob"}) || f.Get("alice").name != "alice2" {
		t.Errorf("replacing alice: order %v, item %q", got, f.Get("alice").name)
	}
	if !slices.Equal(stopped, []string{"alice"}) {
		t.Errorf("the replaced item was not shut down: %v", stopped)
	}

	component.Sort(f)
	if got := order(); !slices.Equal(got, []string{"alice", "bob", "carol"}) {
		t.Errorf("sorted order %v", got)
	}
	kids := box.Children()
	if kids[0] != widget.Widget(f.Get("alice").label) || kids[2] != widget.Widget(f.Get("carol").label) {
		t.Error("the box does not follow the sorted order")
	}
	if f.Index("carol").Current() != 2 {
		t.Errorf("carol at %d after the sort, want 2", f.Index("carol").Current())
	}

	f.Send("bob", rowDelete)
	loop.Settle()
	if !slices.Equal(heard, []string{"bob"}) {
		t.Errorf("outputs forwarded with keys %v, want [bob]", heard)
	}
	f.Remove("bob")
	if f.Has("bob") || f.Len() != 2 || len(box.Children()) != 2 {
		t.Errorf("after removing bob: has=%v len=%d children=%d", f.Has("bob"), f.Len(), len(box.Children()))
	}
	defer func() {
		if recover() == nil {
			t.Error("Get on a missing key did not panic")
		}
	}()
	f.Get("bob")
}
