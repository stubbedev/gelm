package app

import (
	"testing"
	"time"

	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// spyRoot records the Arrange rect, standing in for a window's root
// widget.
type spyRoot struct {
	arranged render.Rect
	nat      widget.Size
	painted  int
}

func (s *spyRoot) Measure(widget.Constraints) widget.Size { return s.nat }

func (s *spyRoot) Arrange(r render.Rect) { s.arranged = r }

func (s *spyRoot) Paint(*render.Canvas) { s.painted++ }

func (s *spyRoot) HitTest(widget.Point) widget.Widget { return nil }

func newToastLayerForTest(root *spyRoot) *toastLayer {
	return &toastLayer{root: root, margin: toastMargin, max: toastMaxStack}
}

func TestToastLayerDropsOldestPastCap(t *testing.T) {
	l := newToastLayerForTest(&spyRoot{nat: widget.Size{W: 50, H: 40}})
	l.max = 2
	toasts := make([]*widget.Toast, 3)
	for i := range toasts {
		toasts[i] = widget.NewToast(nil, "toast", time.Second)
		toasts[i].OnDismissed = func() { l.remove(toasts[i]) }
		l.add(toasts[i])
	}

	t.Run("the cap holds and the oldest drops", func(t *testing.T) {
		if len(l.slots) != 2 {
			t.Fatalf("stack holds %d toasts, want 2", len(l.slots))
		}
		if !toasts[0].Closed() {
			t.Error("the oldest toast survived the cap")
		}
		if toasts[1].Closed() || toasts[2].Closed() {
			t.Error("a live toast was dropped instead of the oldest")
		}
		if l.slots[0] != toasts[1] || l.slots[1] != toasts[2] {
			t.Error("stack order lost: want oldest live first, newest last")
		}
	})
}

func TestToastLayerArrange(t *testing.T) {
	root := &spyRoot{nat: widget.Size{W: 50, H: 40}}
	l := newToastLayerForTest(root)

	t.Run("the wrapped root keeps the whole window", func(t *testing.T) {
		full := render.Rect{X: 0, Y: 0, W: 300, H: 200}
		if got := l.Measure(widget.Constraints{Max: widget.Size{W: full.W, H: full.H}}); got != root.nat {
			t.Errorf("Measure = %v, want the root's natural %v", got, root.nat)
		}
		l.Arrange(full)
		if root.arranged != full {
			t.Errorf("root arranged into %v, want %v", root.arranged, full)
		}
	})

	first := widget.NewToast(nil, "one", 0)
	l.add(first)
	l.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 200})

	t.Run("a single toast sits bottom-center inside the margin", func(t *testing.T) {
		b := first.Bounds()
		if b.Empty() {
			t.Fatal("toast never arranged")
		}
		if b.Y+b.H != 200-toastMargin {
			t.Errorf("toast bottom edge at %d, want %d", b.Y+b.H, 200-toastMargin)
		}
		if b.X != (300-b.W)/2 {
			t.Errorf("toast x = %d, want centered %d", b.X, (300-b.W)/2)
		}
		if b.W > 300-2*toastMargin {
			t.Errorf("toast width %d past the margin box", b.W)
		}
	})

	second := widget.NewToast(nil, "two", 0)
	l.add(second)
	l.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 200})

	t.Run("the newest hugs the edge, older stacks above", func(t *testing.T) {
		top, bottom := first.Bounds(), second.Bounds()
		if top.Y+top.H+toastSpacing != bottom.Y {
			t.Errorf("older toast bottom %d + spacing != newer top %d", top.Y+top.H, bottom.Y)
		}
		if bottom.Y+bottom.H != 200-toastMargin {
			t.Errorf("newest toast not at the edge: bottom %d", bottom.Y+bottom.H)
		}
	})

	l.pos = ToastTop
	l.Arrange(render.Rect{X: 0, Y: 0, W: 300, H: 200})

	t.Run("a top stack mirrors downward", func(t *testing.T) {
		newest, older := second.Bounds(), first.Bounds()
		if newest.Y != toastMargin {
			t.Errorf("newest toast top = %d, want the margin %d", newest.Y, toastMargin)
		}
		if older.Y != newest.Y+newest.H+toastSpacing {
			t.Errorf("older toast top = %d, want below the newest", older.Y)
		}
	})
}

func TestToastLayerRemoveUnwraps(t *testing.T) {
	root := &spyRoot{nat: widget.Size{W: 50, H: 40}}
	l := newToastLayerForTest(root)
	a := widget.NewToast(nil, "a", 0)
	b := widget.NewToast(nil, "b", 0)
	a.OnDismissed = func() { l.remove(a) }
	b.OnDismissed = func() { l.remove(b) }
	l.add(a)
	l.add(b)

	// A dismissed toast detaches; the survivor stays.
	a.OnDismissed()
	if len(l.slots) != 1 || l.slots[0] != b {
		t.Fatalf("stack = %v, want just b", l.slots)
	}
	if !a.Closed() {
		t.Error("removed toast not closed")
	}

	// The last removal tears the layer down and detaches it from the
	// window.
	hw := &hostWindow{}
	hw.router = &widget.Router{Root: widget.Widget(l)}
	l.hw = hw
	b.OnDismissed()
	if len(l.slots) != 0 {
		t.Errorf("stack holds %d toasts after the last removal", len(l.slots))
	}
	if hw.router.Root != widget.Widget(root) {
		t.Errorf("window root = %T, want the bare root back", hw.router.Root)
	}
	if l.hw != nil {
		t.Error("layer still attached to its window")
	}

	// Closing an already-detached layer is inert.
	l.close()
}

func TestShowToastWithoutAWindowIsNil(t *testing.T) {
	a := &Application{}
	if got := a.ShowToast("hi", time.Second, nil); got != nil {
		t.Errorf("ShowToast without windows = %v, want nil", got)
	}
	if got := a.ShowToast("hi", time.Second, &ToastConfig{}); got != nil {
		t.Errorf("ShowToast with a default config = %v, want nil", got)
	}
}
