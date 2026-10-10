package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

func TestColorButtonOpensAndChooses(t *testing.T) {
	b := NewColorButton(render.RGB(1, 2, 3))
	var opened render.Color
	var set []render.Color
	b.OnOpen = func(c render.Color) { opened = c }
	b.OnColorSet = func(c render.Color) { set = append(set, c) }
	b.OnClick()
	if opened != render.RGB(1, 2, 3) {
		t.Errorf("opened at %v", opened)
	}
	b.SetColor(render.RGB(9, 9, 9))
	b.Choose(render.RGB(4, 5, 6))
	if b.Color() != render.RGB(4, 5, 6) || len(set) != 1 {
		t.Errorf("color %v, OnColorSet fired %d times; SetColor must stay silent", b.Color(), len(set))
	}
	if b.Element() != "colorbutton" {
		t.Errorf("element %q", b.Element())
	}
	binding := NewBinding(render.RGB(7, 7, 7))
	b.BindColor(binding)
	b.Choose(render.RGB(8, 8, 8))
	if binding.Get() != render.RGB(8, 8, 8) || b.Color() != render.RGB(8, 8, 8) {
		t.Error("the binding did not follow the pick")
	}
}

func TestFontButtonShowsAndChooses(t *testing.T) {
	b := NewFontButton(testFace(t), 13, FontChoice{Family: "Inter", Size: 13})
	if b.label.Text() != "Inter 13" {
		t.Errorf("label %q", b.label.Text())
	}
	var opened FontChoice
	b.OnOpen = func(f FontChoice) { opened = f }
	b.OnClick()
	b.Choose(FontChoice{Family: "Mono", Size: 11.5})
	if opened.Family != "Inter" || b.Font().Family != "Mono" || b.label.Text() != "Mono 11.5" {
		t.Errorf("opened %+v, now %+v %q", opened, b.Font(), b.label.Text())
	}
}

func TestInscriptionSizesByCountsNotContent(t *testing.T) {
	in := NewInscription(testFace(t), 12, "a very long line of text that would be wide\nsecond\nthird", 0)
	in.SetNatChars(5)
	in.SetNatLines(2)
	short := NewInscription(testFace(t), 12, "x", 0)
	short.SetNatChars(5)
	short.SetNatLines(2)
	a := in.Measure(Constraints{Max: Size{W: 1000, H: 1000}})
	b := short.Measure(Constraints{Max: Size{W: 1000, H: 1000}})
	if a != b || a.W == 0 || a.H != 2*in.lineH {
		t.Errorf("long text measured %v, short %v: the size must come from the counts", a, b)
	}
	in.SetMinChars(8)
	if got := in.Measure(Constraints{Max: Size{W: 1000, H: 1000}}).W; got < in.MinSize().W {
		t.Errorf("natural width %d below the minimum %d", got, in.MinSize().W)
	}
	in.Arrange(render.Rect{W: a.W, H: a.H})
	in.Paint(render.New(make([]uint8, a.W*a.H*4+4096), a.W*4, a.W, a.H))
}
