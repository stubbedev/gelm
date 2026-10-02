package widget

import (
	"slices"
	"testing"
	"time"

	"github.com/stubbedev/gelm/render"
)

// Every widget that records a parent is reachable from that parent by
// the style walk (styleKids): a subtree restyle - the first-parent
// one, an inherited value changing - must reach everything that
// inherits, whatever container holds it.
func TestStyleWalkReachesEveryParentedWidget(t *testing.T) {
	face := testFace(t)
	leaves := map[string]Widget{}
	leaf := func(name string) Widget {
		l := NewLabel(face, 12, name, 0)
		leaves[name] = l
		return l
	}
	icon := NewThemeIcon("x", 12)
	leaves["menu icon"] = icon
	nb := NewNotebook(face)
	nb.AppendTab("tab", leaf("notebook page"))
	grid := NewGrid(0, 0)
	grid.Attach(leaf("grid cell"), 0, 0, 1, 1)
	stack := NewStack()
	stack.Add("p", leaf("stack page"))
	stack.Show("p")
	drop := NewDropdown(face, 12, []string{"a", "b"}, 0)
	drop.Open()
	sw := NewSwitch(true)
	leaves["switch knob"] = &sw.knob
	root := NewBox(Column, 0, 0)
	exp := NewExpander(face, "e", leaf("expander child"))
	exp.SetOpen(true)
	for _, w := range []Widget{
		NewButton(leaf("button content"), 0, 0),
		NewPaned(Row, leaf("paned start"), leaf("paned end")),
		NewOverlay().Append(leaf("overlay child")),
		NewScroll(leaf("scroll child")),
		NewRevealer(leaf("revealer child")),
		NewFader(leaf("fader child")),
		NewElevation(leaf("elevation child")),
		exp,
		NewList[Widget](staticRows{leaf("list row")}, 0),
		NewMenu(face, 12, MenuItem{Label: "m", Icon: icon}),
		nb, grid, stack, drop,
		sw, NewCalendar(face, 12, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)),
		NewColorChooser(face, 12, render.RGB(1, 2, 3)), NewMenuStack(face, 12, MenuItem{Label: "s"}),
	} {
		root.Append(w, false)
	}
	for range 2 {
		root.Measure(Constraints{Max: Size{W: 400, H: 2000}})
		root.Arrange(render.Rect{W: 400, H: 2000})
		data := make([]byte, render.Stride(400)*2000)
		root.Paint(render.New(data, render.Stride(400), 400, 2000))
	}
	for name, w := range leaves {
		for child := w; child != nil && child != Widget(root); child = parentOf(child) {
			p := parentOf(child)
			if p == nil {
				t.Errorf("%s: %T has no parent below the root", name, child)
				break
			}
			if !slices.Contains(styleKids(p), child) {
				t.Errorf("%s: %T is %T's child but not in its style walk", name, child, p)
			}
		}
	}
}

// TestStyleWalkReachesComposites checks the composites' internal
// parts: every widget under the root whose parent is set is in that
// parent's style walk, found by walking Children and styleChildren.
func TestStyleWalkReachesComposites(t *testing.T) {
	face := testFace(t)
	root := NewBox(Column, 0, 0)
	root.Append(NewCalendar(face, 12, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)), false)
	root.Append(NewColorChooser(face, 12, render.RGB(1, 2, 3)), false)
	for range 2 {
		root.Measure(Constraints{Max: Size{W: 400, H: 1200}})
		root.Arrange(render.Rect{W: 400, H: 1200})
		data := make([]byte, render.Stride(400)*1200)
		root.Paint(render.New(data, render.Stride(400), 400, 1200))
	}
	walkTree(root, 0, func(w Widget, _ int) {
		p := parentOf(w)
		if p == nil || w == Widget(root) {
			return
		}
		if slices.Contains(styleKids(p), w) {
			return
		}
		t.Errorf("%T is %T's child but not in its style walk", w, p)
	})
}
