package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

func TestEntryClickToCaret(t *testing.T) {
	face := entryFace(t)
	e := NewEntry(face, 13, render.RGB(255, 255, 255))
	e.SetText("hello")
	e.Measure(Constraints{Max: Size{W: 200, H: 100}})
	e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})

	t.Run("clicking past the end puts the caret at the end", func(t *testing.T) {
		textW := int(face.Shape("hello", 13).Advance())
		e.ClickAt(Point{X: e.Bounds().X + 8 + textW + 50, Y: 15})
		if e.Cursor() != 5 {
			t.Errorf("cursor = %d, want 5", e.Cursor())
		}
	})

	t.Run("clicking mid-text splits the caret", func(t *testing.T) {
		sh := face.Shape("hello", 13)
		e.ClickAt(Point{X: e.Bounds().X + 8 + int(sh.CaretX(3)), Y: 15})
		if e.Cursor() != 3 {
			t.Errorf("cursor = %d, want 3", e.Cursor())
		}
	})

	t.Run("clicking before the text puts the caret at the start", func(t *testing.T) {
		e.ClickAt(Point{X: e.Bounds().X + 1, Y: 15})
		if e.Cursor() != 0 {
			t.Errorf("cursor = %d, want 0", e.Cursor())
		}
	})

	t.Run("typing after a click inserts at the caret", func(t *testing.T) {
		sh := face.Shape("hello", 13)
		e.ClickAt(Point{X: e.Bounds().X + 8 + int(sh.CaretX(2)), Y: 15})
		e.InsertRune('X')
		if got := e.Text(); got != "heXllo" {
			t.Errorf("text = %q, want heXllo", got)
		}
	})
}
