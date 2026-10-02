package widget

// The CSS golden set (docs/css.md exit criterion 4): every property of
// the table shown applied on a consumer widget. New properties add a
// shot here; the pre-CSS goldens must stay untouched — the zero-style
// case is today's pixels.

import (
	"testing"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// cssShot pins the palette, loads css for the shot, and hands the
// rest to NewGolden.
func cssShot(t *testing.T, css string, w Widget, name string, opts ...goldenOption) {
	t.Helper()
	loadCSS(t, css)
	NewGolden(t, w, name, opts...)
}

func TestGoldenCSSButtonProperties(t *testing.T) {
	face := goldenFace(t)
	// background-color, border-radius, padding, border-width,
	// border-color in one rule on one consumer.
	btn := NewButton(NewLabel(face, 14, "Save", render.RGB(0xff, 0xff, 0xff)), 10, 6)
	btn.AddClass("primary")
	cssShot(t, `
		.primary {
			background-color: #7b3fa0;
			border-radius: 14;
			padding: 16;
			border-width: 3;
			border-style: solid;
			border-color: #30d5c8;
		}
	`, btn, "css-button-properties")
}

func TestGoldenCSSColorAndInheritance(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	// color on a label directly, and color inherited from a styled box
	// into a zero-color label — the inheritance half of the set.
	direct := NewLabel(face, 14, "direct color", 0)
	inherited := NewBox(Column, 8, 8)
	inherited.Append(NewLabel(face, 14, "inherited", 0), false)
	cssShot(t, `label { color: #e05561; }`, direct, "css-label-color", goldenTheme(th))
	cssShot(t, `box { color: #3fb68b; }`, inherited, "css-label-color-inherited", goldenTheme(th))
}

func TestGoldenCSSFontSize(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	lbl := NewLabel(face, 12, "sized by the sheet", th.Text)
	cssShot(t, `label { font-size: 20; }`, lbl, "css-label-font-size", goldenTheme(th))
}

// installFixtureResolver wires the family/weight lookup to the two
// fixture faces: the Hebrew face stands in for both the "hebrew"
// family and the bold variant, since the fixture set carries two
// regular faces. It also records the lookups, so tests can pin that
// the declaration reached the resolver verbatim.
func installFixtureResolver(t *testing.T) *[]string {
	t.Helper()
	hebrew, err := render.NewFixtureHebrewTypeface()
	if err != nil {
		t.Fatalf("load hebrew fixture: %v", err)
	}
	var calls []string
	SetFaceResolver(func(family string, weight int) (render.Font, bool) {
		calls = append(calls, family+"/"+itoa(weight))
		if family == "hebrew" || weight >= 600 {
			return hebrew, true
		}
		return nil, false
	})
	t.Cleanup(func() { SetFaceResolver(nil) })
	return &calls
}

func TestGoldenCSSFontFamily(t *testing.T) {
	face := goldenFace(t)
	resolverCalls := installFixtureResolver(t)
	// Cantarell covers no Hebrew: without the sheet the label is
	// .notdef boxes; with font-family the resolver's face shapes it.
	lbl := NewLabel(face, 20, "אבג home", render.RGB(0xff, 0xff, 0xff))
	loadCSS(t, `label { font-family: "hebrew"; }`)
	NewGolden(t, lbl, "css-label-font-family")
	if len(*resolverCalls) == 0 || (*resolverCalls)[0] != "hebrew/0" {
		t.Errorf("resolver calls = %v, want hebrew/0", *resolverCalls)
	}
}

func TestGoldenCSSFontWeight(t *testing.T) {
	face := goldenFace(t)
	resolverCalls := installFixtureResolver(t)
	lbl := NewLabel(face, 20, "אבג home", render.RGB(0xff, 0xff, 0xff))
	loadCSS(t, `label { font-weight: bold; }`)
	NewGolden(t, lbl, "css-label-font-weight")
	if len(*resolverCalls) == 0 || (*resolverCalls)[0] != "/700" {
		t.Errorf("resolver calls = %v, want /700", *resolverCalls)
	}
}

func TestGoldenCSSEntryMinAndBorder(t *testing.T) {
	face := goldenFace(t)
	// min-width floors the field well past its natural size, and the
	// border-width/border-color ring strokes it; background-color
	// fills between.
	entry := NewEntry(face, 14, render.RGB(0xff, 0xff, 0xff))
	entry.SetText("typed")
	cssShot(t, `
		entry {
			min-width: 260;
			border-width: 2;
			border-style: solid;
			border-color: #f5a623;
			background-color: #1b2a34;
		}
	`, entry, "css-entry-min-border")
}

func TestGoldenCSSTextAreaBackground(t *testing.T) {
	face := goldenFace(t)
	area := NewTextArea(face, 13, render.RGB(0xff, 0xff, 0xff))
	area.SetText("styled area\nsecond line")
	cssShot(t, `textview { background-color: #123456; }`, area, "css-textarea-background")
}

func TestGoldenCSSToastShadow(t *testing.T) {
	face := goldenFace(t)
	toast := NewToast(face, "saved", 0)
	cssShot(t, `
		toast {
			box-shadow: #a04000 24;
			border-radius: 10;
		}
	`, toast, "css-toast-shadow", goldenAfterArrange(func() {
		toast.progress = 1
	}))
	// The glow is subtle on the card; pin the cascade that painted it.
	v := toast.style(toast)
	if !v.Has(style.PropBoxShadow) || v.Shadow.N != 1 || v.Shadow.Layers[0].Color != render.RGBA(0xa0, 0x40, 0x00, 0xff) || v.Shadow.Layers[0].Blur != 24 {
		t.Errorf("toast shadow cascade = %+v", v.Shadow)
	}
}

func TestGoldenCSSDialogCard(t *testing.T) {
	face := goldenFace(t)
	// The app-level surfaces name their card; dialog styles the
	// elevation's shadow, plate, and radius.
	card := NewElevation(NewLabel(face, 14, "dialog body", render.RGB(0xff, 0xff, 0xff))).
		WithRadius(6).
		WithPlate(DarkTheme().Surface)
	card.SetElement("dialog")
	cssShot(t, `
		dialog {
			box-shadow: #30d5c8 20;
			border-radius: 16;
			background-color: #202233;
		}
	`, card, "css-dialog-card")
}
