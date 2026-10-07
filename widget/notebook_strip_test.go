package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// stripNotebook is a 250px notebook over n tabs named a, b, c, ...
func stripNotebook(t *testing.T, n int) *Notebook {
	nb := NewNotebook(chromeFace(t))
	for i := range n {
		nb.AppendTab(string(rune('a'+i)), NewSpacer(10, 10))
	}
	nb.Measure(Constraints{Max: Size{W: 250, H: 100}})
	nb.Arrange(render.Rect{W: 250, H: 100})
	return nb
}

// TestNotebookReorder pins the drag: a reorderable tab moves into the
// slot under the pointer as it crosses, the selection following its
// page; a fixed strip ignores the drag.
func TestNotebookReorder(t *testing.T) {
	nb := stripNotebook(t, 2)
	nb.Reorderable = true
	var moves []string
	nb.OnReorder = func(name string, i int) { moves = append(moves, name+string(rune('0'+i))) }
	a := nb.tabRect(0)
	b := nb.tabRect(1)
	nb.PressAt(Point{X: a.X + 10, Y: a.Y + 5})
	nb.DragMove(Point{X: b.X + 10, Y: b.Y + 5})
	nb.PressEnd()
	if nb.tabs[0].name != "b" || nb.tabs[1].name != "a" || nb.SelectedTab() != "a" || nb.selected != 1 {
		t.Errorf("after the drag: %s %s, selected %q at %d", nb.tabs[0].name, nb.tabs[1].name, nb.SelectedTab(), nb.selected)
	}
	if len(moves) != 1 || moves[0] != "a1" {
		t.Errorf("OnReorder = %v", moves)
	}
	nb.Reorderable = false
	nb.PressAt(Point{X: a.X + 10, Y: a.Y + 5})
	nb.DragMove(Point{X: b.X + 10, Y: b.Y + 5})
	if nb.tabs[0].name != "b" {
		t.Error("a fixed strip reordered")
	}
}

// TestNotebookOverflow pins the scrolling strip: arrows appear only on
// overflow, step a tab, selecting reveals, the wheel scrolls over the
// strip and passes on elsewhere, and ctrl+PageDown cycles.
func TestNotebookOverflow(t *testing.T) {
	if r := stripNotebook(t, 2).arrowRects(); !r[0].Empty() {
		t.Error("arrows on a strip that fits")
	}
	nb := stripNotebook(t, 5) // 480px of tabs in 250
	arrows := nb.arrowRects()
	if arrows[0].Empty() || nb.stripRange().Max() != 480-210 {
		t.Fatalf("overflow: arrows=%v max=%d", arrows, nb.stripRange().Max())
	}
	nb.ClickAt(Point{X: arrows[1].X + 5, Y: 5})
	if nb.scroll != tabWidth {
		t.Errorf("right arrow scrolled to %d", nb.scroll)
	}
	nb.SelectTab("a")
	if nb.scroll != 0 {
		t.Errorf("selecting the first tab left the strip at %d", nb.scroll)
	}
	nb.KeyAction(KeyPriorPage, ModCtrl)
	if nb.SelectedTab() != "e" || nb.tabRect(4).X+tabWidth > nb.tabViewport().X+nb.tabViewport().W {
		t.Errorf("ctrl+PageUp wrapped to %q at x=%d, want e revealed", nb.SelectedTab(), nb.tabRect(4).X)
	}
	nb.scrollStrip(0)
	nb.HoverMove(Point{X: 100, Y: 5})
	if !nb.ScrollInput(1) || nb.scroll != tabWidth/3 {
		t.Errorf("wheel over the strip: consumed? scroll=%d", nb.scroll)
	}
	nb.SetHovered(false)
	if nb.ScrollInput(1) {
		t.Error("the wheel off the strip was consumed")
	}
}

// TestGoldenNotebookStrip pins an overflowing, closable strip.
func TestGoldenNotebookStrip(t *testing.T) {
	th := DarkTheme()
	nb := NewNotebook(goldenFace(t))
	nb.Closable = true
	for _, s := range []string{"One", "Two", "Three", "Four"} {
		nb.AppendTab(s, NewSpacer(10, 10))
	}
	nb.SelectTab("Two")
	NewGolden(t, nb, "notebook-strip", goldenTheme(th), goldenFrame(260, 60))
}
