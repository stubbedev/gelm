package widget

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/gelm/render"
)

func testFace(t *testing.T) *render.Typeface {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

func TestLabelMeasure(t *testing.T) {
	face := testFace(t)

	t.Run("nonempty text wants positive space", func(t *testing.T) {
		got := NewLabel(face, "gelm", 14, render.RGB(255, 255, 255)).Measure(Constraints{Max: Size{W: 500, H: 100}})
		if got.W <= 0 || got.H <= 0 {
			t.Errorf("measure = %v, want positive", got)
		}
	})

	t.Run("empty text keeps zero width and the font line height", func(t *testing.T) {
		got := NewLabel(face, "", 14, render.RGB(255, 255, 255)).Measure(Constraints{Max: Size{W: 500, H: 100}})
		if got.W != 0 {
			t.Errorf("empty label width = %d, want 0", got.W)
		}
		if got.H <= 0 {
			t.Errorf("empty label height = %d, want the font line height", got.H)
		}
	})

	t.Run("wider text wants more width", func(t *testing.T) {
		short := NewLabel(face, "g", 14, render.RGB(255, 255, 255)).Measure(Constraints{Max: Size{W: 500, H: 100}})
		long := NewLabel(face, "gelmland", 14, render.RGB(255, 255, 255)).Measure(Constraints{Max: Size{W: 500, H: 100}})
		if long.W <= short.W {
			t.Errorf("long %d <= short %d", long.W, short.W)
		}
	})

	t.Run("clamps to the constraint ceiling", func(t *testing.T) {
		got := NewLabel(face, "gelm is great", 14, render.RGB(255, 255, 255)).Measure(Constraints{Max: Size{W: 20, H: 8}})
		if got.W != 20 || got.H != 8 {
			t.Errorf("measure = %v, want clamped to 20x8", got)
		}
	})

	t.Run("SetText retakes the measurement", func(t *testing.T) {
		l := NewLabel(face, "a", 14, render.RGB(255, 255, 255))
		before := l.Measure(Constraints{Max: Size{W: 500, H: 100}})
		l.SetText("a much longer label")
		after := l.Measure(Constraints{Max: Size{W: 500, H: 100}})
		if after.W <= before.W {
			t.Errorf("after SetText width %d <= before %d", after.W, before.W)
		}
		if l.Text() != "a much longer label" {
			t.Errorf("Text() = %q", l.Text())
		}
	})

	t.Run("SetText to the same value is a no-op", func(t *testing.T) {
		l := NewLabel(face, "same", 14, render.RGB(255, 255, 255))
		l.SetText("same")
		if l.Text() != "same" {
			t.Errorf("Text() = %q", l.Text())
		}
	})
}

func TestLabelPaint(t *testing.T) {
	face := testFace(t)

	t.Run("text leaves ink in the arranged rect", func(t *testing.T) {
		data := make([]byte, render.Stride(200)*32)
		cv := render.New(data, render.Stride(200), 200, 32)
		l := NewLabel(face, "ink", 16, render.RGB(255, 255, 255))
		l.Measure(Constraints{Max: Size{W: 500, H: 100}})
		l.Arrange(render.Rect{X: 4, Y: 0, W: 100, H: 32})
		l.Paint(cv)
		ink := 0
		for y := range 32 {
			for x := range 200 {
				if render.ColorFromBytes(data[y*render.Stride(200)+x*4:]).A() > 0 {
					ink++
				}
			}
		}
		if ink == 0 {
			t.Error("painted label produced no ink")
		}
	})
}
