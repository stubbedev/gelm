package component_test

import (
	"context"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stubbedev/lys-ui/component"
	"github.com/stubbedev/lys-ui/component/componenttest"
	"github.com/stubbedev/lys-ui/widget"
)

type loadingRow struct {
	name  string
	label *widget.Spacer
	inits int
	got   []string
}

func (r *loadingRow) Loading() widget.Widget { return widget.NewSpacer(2, 2) }

func (r *loadingRow) Load(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
	}
}

func (r *loadingRow) Init(*component.Context[string, struct{}]) widget.Widget {
	r.inits++
	r.label = widget.NewSpacer(5, 5)
	return r.label
}

func (r *loadingRow) Update(_ *component.Context[string, struct{}], msg string) {
	r.got = append(r.got, r.name+":"+msg)
}

func slotShowing(t *testing.T, w widget.Widget) string {
	t.Helper()
	s, ok := w.(*widget.Stack)
	if !ok {
		t.Fatalf("item root = %T, want a loading slot", w)
	}
	return s.Visible()
}

func finishLoading() {
	time.Sleep(time.Second)
	synctest.Wait()
}

func TestFactoryAsyncItemsShowLoadingThenSwapAndReleaseHeldInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		box := widget.NewBox(widget.Column, 0, 0)
		f := component.NewFactory[*loadingRow, string, struct{}](&loop, component.BoxView[*loadingRow](box, false))
		a, b := &loadingRow{name: "a"}, &loadingRow{name: "b"}
		f.PushBack(a)
		f.PushBack(b)
		kids := box.Children()
		if slotShowing(t, kids[0]) != "loading" || slotShowing(t, kids[1]) != "loading" {
			t.Fatalf("slots show %q and %q, want loading first", slotShowing(t, kids[0]), slotShowing(t, kids[1]))
		}

		f.Send(0, "one")
		f.Broadcast("two")
		loop.Settle()
		if a.inits+b.inits != 0 || len(a.got)+len(b.got) != 0 {
			t.Fatalf("before Load returned: inits=%d updates=%v", a.inits+b.inits, append(slices.Clone(a.got), b.got...))
		}

		finishLoading()
		loop.Settle()
		if a.inits != 1 || b.inits != 1 {
			t.Fatalf("inits a=%d b=%d, want one each", a.inits, b.inits)
		}
		kids = box.Children()
		if slotShowing(t, kids[0]) != "ready" || slotShowing(t, kids[1]) != "ready" {
			t.Errorf("slots show %q and %q, want ready", slotShowing(t, kids[0]), slotShowing(t, kids[1]))
		}
		if !slices.Equal(a.got, []string{"a:one", "a:two"}) || !slices.Equal(b.got, []string{"b:two"}) {
			t.Errorf("held input delivered a=%v b=%v, want [a:one a:two] and [b:two]", a.got, b.got)
		}
	})
}

func TestFactoryLoadingItemRemovedNeverInits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		box := widget.NewBox(widget.Column, 0, 0)
		f := component.NewFactory[*loadingRow, string, struct{}](&loop, component.BoxView[*loadingRow](box, false))
		a, b := &loadingRow{name: "a"}, &loadingRow{name: "b"}
		f.PushBack(a)
		f.PushBack(b)
		f.Remove(0)
		finishLoading()
		loop.Settle()
		if a.inits != 0 {
			t.Errorf("a removed mid-load ran Init %d times", a.inits)
		}
		if b.inits != 1 || len(box.Children()) != 1 {
			t.Errorf("after the load: b inits=%d children=%d, want 1 and the surviving item", b.inits, len(box.Children()))
		}
	})
}

func TestFactoryShutdownWhileLoadingNeverInitsItems(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		f := component.NewFactory[*loadingRow, string, struct{}](&loop, component.BoxView[*loadingRow](widget.NewBox(widget.Column, 0, 0), false))
		a, b := &loadingRow{name: "a"}, &loadingRow{name: "b"}
		f.PushBack(a)
		f.PushBack(b)
		f.Shutdown()
		finishLoading()
		loop.Settle()
		if a.inits+b.inits != 0 {
			t.Errorf("items shut down mid-load ran Init %d times", a.inits+b.inits)
		}
	})
}

func TestFactoryMoveAndSwapOfLoadingItemKeepsItsSlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		box := widget.NewBox(widget.Column, 0, 0)
		f := component.NewFactory[*loadingRow, string, struct{}](&loop, component.BoxView[*loadingRow](box, false))
		a, b, c := &loadingRow{name: "a"}, &loadingRow{name: "b"}, &loadingRow{name: "c"}
		ax := f.PushBack(a)
		f.PushBack(b)
		f.PushBack(c)
		before := slices.Clone(box.Children())

		f.Move(0, 2)
		f.Swap(0, 2)
		moved := slices.Clone(box.Children())
		if moved[0] != before[0] || moved[1] != before[2] || moved[2] != before[1] {
			t.Fatalf("Move and Swap did not keep the slots: %v", moved)
		}
		if ax.Current() != 0 {
			t.Errorf("a at %d after Move(0,2) and Swap(0,2), want 0", ax.Current())
		}
		for i, w := range moved {
			if got := slotShowing(t, w); got != "loading" {
				t.Fatalf("slot %d shows %q after the edits, want loading", i, got)
			}
		}

		finishLoading()
		loop.Settle()
		for i, w := range box.Children() {
			if w != moved[i] {
				t.Errorf("slot %d was rebuilt on load", i)
			}
			if got := slotShowing(t, w); got != "ready" {
				t.Errorf("slot %d shows %q after the load, want ready", i, got)
			}
		}
		if a.inits != 1 || b.inits != 1 || c.inits != 1 {
			t.Errorf("inits a=%d b=%d c=%d, want one each", a.inits, b.inits, c.inits)
		}
	})
}

func TestKeyedFactoryAsyncItemsLoadRemoveAndHoldInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		box := widget.NewBox(widget.Column, 0, 0)
		f := component.NewKeyedFactory[string, *loadingRow](
			&loop, component.BoxView[*loadingRow](box, false))
		a, b := &loadingRow{name: "a"}, &loadingRow{name: "b"}
		f.Insert("a", a)
		f.Insert("b", b)
		kids := box.Children()
		if slotShowing(t, kids[0]) != "loading" || slotShowing(t, kids[1]) != "loading" {
			t.Fatalf("slots show %q and %q, want loading first", slotShowing(t, kids[0]), slotShowing(t, kids[1]))
		}

		f.Send("a", "one")
		loop.Settle()
		if a.inits != 0 || len(a.got) != 0 {
			t.Fatalf("before Load returned: inits=%d updates=%v", a.inits, a.got)
		}

		finishLoading()
		loop.Settle()
		kids = box.Children()
		if slotShowing(t, kids[0]) != "ready" || slotShowing(t, kids[1]) != "ready" {
			t.Errorf("slots show %q and %q, want ready", slotShowing(t, kids[0]), slotShowing(t, kids[1]))
		}
		if !slices.Equal(a.got, []string{"a:one"}) {
			t.Errorf("held input delivered %v, want [a:one]", a.got)
		}

		c := &loadingRow{name: "c"}
		f.Insert("c", c)
		f.Remove("c")
		finishLoading()
		loop.Settle()
		if c.inits != 0 {
			t.Errorf("c removed mid-load ran Init %d times", c.inits)
		}
		if f.Len() != 2 || len(box.Children()) != 2 {
			t.Errorf("after removing c mid-load: len=%d children=%d, want 2", f.Len(), len(box.Children()))
		}
	})
}
