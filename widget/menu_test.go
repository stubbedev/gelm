package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

func TestMenu(t *testing.T) {
	face := entryFace(t)
	clicked := ""
	menu := NewMenu(face, 14,
		MenuItem{Label: "Cut", OnClick: func() { clicked = "cut" }},
		MenuItem{Label: "Copy", OnClick: func() { clicked = "copy" }},
		MenuItem{Label: "Disabled"},
	)
	menu.Measure(Constraints{Max: Size{W: 300, H: 300}})
	menu.Arrange(render.Rect{X: 10, Y: 20, W: 120, H: 3 * menu.itemH})
	menu.OnDismiss = func() {}

	t.Run("hover tracks the row under the pointer", func(t *testing.T) {
		menu.HoverMove(Point{X: 40, Y: 20 + 4 + menu.itemH + 3})
		if menu.hovered != 1 {
			t.Errorf("hovered = %d, want 1", menu.hovered)
		}
		menu.HoverMove(Point{X: 5, Y: 5})
		if menu.hovered != -1 {
			t.Errorf("hovered outside rows = %d, want -1", menu.hovered)
		}
	})

	t.Run("clicking a row fires it and dismisses", func(t *testing.T) {
		fired := false
		menu.OnDismiss = func() { fired = true }
		menu.ClickAt(Point{X: 40, Y: 20 + 4 + menu.itemH + 3})
		if clicked != "copy" {
			t.Errorf("clicked = %q, want copy", clicked)
		}
		if !fired {
			t.Error("click did not dismiss the menu")
		}
	})

	t.Run("clicking a disabled row neither fires nor dismisses", func(t *testing.T) {
		clicked = ""
		fired := false
		menu.OnDismiss = func() { fired = true }
		menu.ClickAt(Point{X: 40, Y: 20 + 4 + 2*menu.itemH + 3})
		if clicked != "" || fired {
			t.Errorf("disabled row fired (%q, dismiss %v)", clicked, fired)
		}
	})

	t.Run("clicking outside the rows dismisses", func(t *testing.T) {
		fired := false
		menu.OnDismiss = func() { fired = true }
		menu.ClickAt(Point{X: 40, Y: 20 - 50})
		if !fired {
			t.Error("outside click did not dismiss")
		}
	})

	t.Run("painted menu leaves ink on every row", func(t *testing.T) {
		menu.OnDismiss = nil
		data := make([]byte, render.Stride(130)*90)
		cv := render.New(data, render.Stride(130), 130, 90)
		menu.Arrange(render.Rect{X: 0, Y: 0, W: 120, H: 3 * menu.itemH})
		menu.Paint(cv)
		for row := range 3 {
			y := 4 + row*menu.itemH + 10
			ink := 0
			for x := range 120 {
				if render.ColorFromBytes(data[y*render.Stride(130)+x*4:]).A() > 0 {
					ink++
				}
			}
			if ink == 0 {
				t.Errorf("row %d painted nothing", row)
			}
		}
	})
}
