package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

func flowLeaves(n int) []*solidLeaf {
	out := make([]*solidLeaf, n)
	for i := range out {
		out[i] = &solidLeaf{sz: Size{W: 40, H: 20}, col: render.RGB(0, 0, 0xff)}
	}
	return out
}

func TestFlowBoxWraps(t *testing.T) {
	f := NewFlowBox(4, 6)
	leaves := flowLeaves(5)
	for _, l := range leaves {
		f.Append(l)
	}
	if sz := f.Measure(Constraints{Max: Size{W: 1000, H: 1000}}); sz != (Size{W: 5*40 + 4*4, H: 20}) {
		t.Errorf("wide: %v, want one line", sz)
	}
	// 100px fits two 40px children and their gap per line.
	if sz := f.Measure(Constraints{Max: Size{W: 100, H: 1000}}); sz != (Size{W: 84, H: 3*20 + 2*6}) {
		t.Errorf("narrow: %v, want three lines of two", sz)
	}
	// The gaps count: at 82 two children fit but not their gap, at 126
	// three fit but not both gaps.
	if sz := f.Measure(Constraints{Max: Size{W: 82, H: 1000}}); sz.H != 5*20+4*6 {
		t.Errorf("82px: %v, want one per line", sz)
	}
	if sz := f.Measure(Constraints{Max: Size{W: 126, H: 1000}}); sz.H != 3*20+2*6 {
		t.Errorf("126px: %v, want two per line", sz)
	}
	f.Measure(Constraints{Max: Size{W: 100, H: 1000}})
	f.Arrange(render.Rect{X: 10, Y: 10, W: 100, H: 72})
	if leaves[2].bounds != (render.Rect{X: 10, Y: 36, W: 40, H: 20}) || leaves[3].bounds.X != 54 {
		t.Errorf("third child at %v, fourth at %v", leaves[2].bounds, leaves[3].bounds)
	}
	if f.IndexAt(Point{X: 60, Y: 40}) != 3 || f.IndexAt(Point{X: 52, Y: 40}) != -1 || f.ChildAt(3).Child() != leaves[3] {
		t.Error("IndexAt / ChildAt")
	}
	if f.ChildAt(4).Index() != 4 || f.ChildAt(9) != nil {
		t.Error("Index / out-of-range ChildAt")
	}
	// The per-line cap.
	f.SetMaxChildrenPerLine(2)
	if sz := f.Measure(Constraints{Max: Size{W: 1000, H: 1000}}); sz.H != 3*20+2*6 {
		t.Errorf("two per line: %v", sz)
	}
	f.SetMaxChildrenPerLine(0)
	if sz := f.Measure(Constraints{Max: Size{W: 1000, H: 1000}}); sz.H != 5*20+4*6 {
		t.Errorf("a cap below one is one: %v", sz)
	}
	f.SetMaxChildrenPerLine(7)
	// Hidden children take no slot.
	f.ChildAt(1).SetVisible(false)
	f.Measure(Constraints{Max: Size{W: 100, H: 1000}})
	f.Arrange(render.Rect{W: 100, H: 72})
	if leaves[2].bounds.X != 44 || leaves[2].bounds.Y != 0 {
		t.Errorf("after a hidden child the next sits at %v", leaves[2].bounds)
	}
	// Insert, remove, clear.
	extra := &solidLeaf{sz: Size{W: 10, H: 10}}
	f.Insert(0, extra)
	if f.ChildAt(0).Child() != extra || f.Len() != 6 || parentOf(extra) != Widget(f.ChildAt(0)) {
		t.Error("Insert at 0")
	}
	f.RemoveAt(0)
	if f.Len() != 5 || parentOf(extra) != nil || f.ChildAt(0).Child() != leaves[0] {
		t.Error("RemoveAt")
	}
	f.Clear()
	if f.Len() != 0 || len(f.Children()) != 0 || f.Measure(Constraints{Max: Size{W: 100, H: 100}}) != (Size{}) {
		t.Error("Clear")
	}
}

func TestFlowBoxChildNodes(t *testing.T) {
	loadCSS(t, `flowbox > flowboxchild.mark { background-color: #ff0000; padding: 3px; }`)
	f := NewFlowBox(0, 0)
	leaf := &solidLeaf{sz: Size{W: 10, H: 10}, col: render.RGB(0, 0, 0xff)}
	c := f.Append(leaf)
	c.AddClass("mark")
	sz := f.Measure(Constraints{Max: Size{W: 100, H: 100}})
	if sz != (Size{W: 16, H: 16}) {
		t.Errorf("styled child %v, want the padding around it", sz)
	}
	f.Arrange(render.Rect{W: 16, H: 16})
	data := make([]byte, render.Stride(16)*16)
	cv := render.New(data, render.Stride(16), 16, 16)
	f.Paint(cv)
	px := func(x, y int) render.Color { return render.ColorFromBytes(data[y*render.Stride(16)+x*4:]) }
	if px(1, 1) != render.RGB(0xff, 0, 0) || px(8, 8) != render.RGB(0, 0, 0xff) {
		t.Errorf("padding %v, child %v", px(1, 1), px(8, 8))
	}
	if f.HitTest(Point{X: 8, Y: 8}) != Widget(leaf) || f.HitTest(Point{X: 1, Y: 1}) != Widget(c) {
		t.Error("hit test")
	}
}

// dropZone is an app's drop target around a FlowBox: it embeds the
// box and adds the drag interfaces.
type dropZone struct {
	*FlowBox
	entered []Point
	dropped string
}

func (z *dropZone) DragEnter(_ []string, p Point) string {
	z.entered = append(z.entered, p)
	return "text/plain"
}

func (z *dropZone) Drop(_ string, data []byte, _ Point) { z.dropped = string(data) }

// A type embedding a container is what its children's parent links
// name, so ancestor walks find the wrapper's interfaces: a drag over a
// chip inside the zone reaches the zone.
func TestEmbeddingWrapperIsTheParent(t *testing.T) {
	zone := &dropZone{FlowBox: NewFlowBox(0, 0)}
	leaf := &solidLeaf{sz: Size{W: 20, H: 20}}
	c := zone.Append(leaf)
	root := NewBox(Column, 0, 0)
	root.Append(zone, false)
	layoutRoot(root, 100, 100)
	if parentOf(c) != Widget(zone) {
		t.Fatalf("the child's parent is %T, want the wrapper", parentOf(c))
	}
	r := &Router{Root: root}
	if mime := r.DragEnter([]string{"text/plain"}, Point{X: 5, Y: 5}); mime != "text/plain" || len(zone.entered) != 1 {
		t.Fatalf("drag over the chip: mime %q, zone entered %d times", mime, len(zone.entered))
	}
	r.Drop("text/plain", []byte("0:left:1"), Point{X: 5, Y: 5})
	if zone.dropped != "0:left:1" {
		t.Errorf("the zone got %q", zone.dropped)
	}
	// Unwrapped, the parent is the container itself.
	plain := NewFlowBox(0, 0)
	pc := plain.Append(&solidLeaf{sz: Size{W: 5, H: 5}})
	if parentOf(pc) != Widget(plain) {
		t.Errorf("a plain box's child names %T", parentOf(pc))
	}
	// A removed child no longer knows its index.
	zone.RemoveAt(0)
	if c.Index() != -1 {
		t.Errorf("a removed child reports index %d", c.Index())
	}
}
