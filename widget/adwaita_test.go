package widget

import (
	"testing"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget/css"
)

// TestThemeSheetParsesClean pins the generated layer: every named
// color and class rule parses without a warning, for each preset.
func TestThemeSheetParsesClean(t *testing.T) {
	for _, th := range []*Theme{DarkTheme(), LightTheme(), HighContrastTheme()} {
		var warns []string
		style.SetParseWarn(func(msg string) { warns = append(warns, msg) })
		style.Parse(themeCSS(th))
		style.SetParseWarn(nil)
		if len(warns) != 0 {
			t.Errorf("theme sheet warned: %v", warns)
		}
	}
}

// TestNamedColorsFollowTheme pins the theme layer end to end: a class
// styled through @accent_bg_color paints the current theme's accent,
// re-derives on SetTheme, and an application stylesheet outranks it.
func TestNamedColorsFollowTheme(t *testing.T) {
	defer SetTheme(nil)
	face := chromeFace(t)
	btn := NewButton(NewLabel(face, 14, "go", DarkTheme().Text), 6, 4)
	btn.AddClass(css.SuggestedAction)
	root := NewBox(Row, 0, 0)
	root.Append(btn, false)
	bg := func() render.Color {
		arrangeTree(t, root, 200, 40)
		return btn.style(btn).Background
	}

	SetTheme(DarkTheme())
	if got := bg(); got != DarkTheme().Accent {
		t.Errorf("dark: suggested-action fill = %08x, want the accent", uint32(got))
	}
	SetTheme(LightTheme())
	if got := bg(); got != LightTheme().Accent {
		t.Errorf("light: fill = %08x, want the light accent", uint32(got))
	}
	loadCSS(t, `@define-color accent_bg_color #102030;`)
	if got := bg(); got != render.RGB(0x10, 0x20, 0x30) {
		t.Errorf("app override: fill = %08x, want the application define", uint32(got))
	}
}

// TestSystemColorsFollowTheme pins the CSS system color keywords
// against the active theme: Canvas tracks the theme background across
// SetTheme, and Highlight the accent, the same re-derivation the named
// colors get.
func TestSystemColorsFollowTheme(t *testing.T) {
	defer SetTheme(nil)
	face := chromeFace(t)
	lbl := NewLabel(face, 14, "x", DarkTheme().Text)
	root := NewBox(Row, 0, 0)
	root.Append(lbl, false)
	loadCSS(t, `label { background-color: Canvas; color: Highlight; }`)
	paint := func() (render.Color, render.Color) {
		arrangeTree(t, root, 200, 40)
		v := lbl.style(lbl)
		return v.Background, v.Color
	}

	SetTheme(LightTheme())
	bg, hl := paint()
	if bg != LightTheme().Bg {
		t.Errorf("Canvas = %08x, want the theme bg %08x", uint32(bg), uint32(LightTheme().Bg))
	}
	if hl != LightTheme().Accent {
		t.Errorf("Highlight = %08x, want the accent %08x", uint32(hl), uint32(LightTheme().Accent))
	}
	SetTheme(DarkTheme())
	if bg, _ = paint(); bg != DarkTheme().Bg {
		t.Errorf("dark Canvas = %08x, want the theme bg %08x", uint32(bg), uint32(DarkTheme().Bg))
	}
}

// TestStatusColorsDerive pins the status fallbacks: explicit values
// win, zero derives the Adwaita default for the palette's polarity.
func TestStatusColorsDerive(t *testing.T) {
	dark, light := DarkTheme(), LightTheme()
	if !dark.IsDark() || light.IsDark() {
		t.Fatal("preset polarity misread")
	}
	if dark.DestructiveColor() == light.DestructiveColor() {
		t.Error("destructive does not follow polarity")
	}
	custom := dark.with(func(n *Theme) { n.Success = render.RGB(1, 2, 3) })
	if custom.SuccessColor() != render.RGB(1, 2, 3) {
		t.Error("an explicit status color lost to the default")
	}
}
