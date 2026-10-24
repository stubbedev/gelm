package widget

import (
	"fmt"
	"testing"

	"github.com/stubbedev/lys-ui/internal/sysfont"
	"github.com/stubbedev/lys-ui/widget/css"
)

// The shadcn control set in both palettes: every button variant at
// rest, hovered, pressed and disabled, the size steps, and the inputs
// and toggles at rest and focused (#193).
func TestGoldenShadcnControls(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			button := func(label string, classes ...string) *Button {
				b := NewButton(NewLabel(face, 14, label, 0), 6, 4)
				b.AddClass(classes...)
				return b
			}
			grid := NewBox(Column, 8, 0)
			for _, variant := range [][]string{nil, {css.SuggestedAction}, {css.DestructiveAction}, {css.Outline}, {css.Ghost}, {css.Link}} {
				row := NewBox(Row, 8, 0)
				rest, hover, pressed, disabled := button("Button", variant...), button("Hover", variant...), button("Pressed", variant...), button("Disabled", variant...)
				hover.SetHovered(true)
				pressed.Pressed = true
				disabled.SetEnabled(false)
				row.Append(rest, false).Append(hover, false).Append(pressed, false).Append(disabled, false)
				grid.Append(row, false)
			}
			sizes := NewBox(Row, 8, 0)
			sizes.Append(button("Small", css.Small), false).Append(button("Default"), false).Append(button("Large", css.Large), false)
			grid.Append(sizes, false)
			NewGolden(t, grid, "shadcn-buttons-"+tc.name, goldenTheme(tc.th))

			entry := NewEntry(face, 14, tc.th.Text)
			entry.SetPlaceholder("Email")
			focused := NewEntry(face, 14, tc.th.Text)
			focused.SetText("name@example.com")
			inputs := NewBox(Column, 8, 0)
			inputs.Append(entry, false).Append(focused, false).
				Append(NewDropdown(face, 14, []string{"Apple", "Banana"}, 0), false)
			toggles := NewBox(Row, 12, 0)
			toggles.Append(NewCheckButton(false), false).Append(NewCheckButton(true), false).
				Append(NewSwitch(false), false).Append(NewSwitch(true), false)
			inputs.Append(toggles, false).Append(NewSlider(0, 100, 1, 40), false).Append(NewProgressBar(0.6), false)
			h := inputs.Measure(Constraints{Max: Size{W: 260, H: 4096}}).H
			NewGolden(t, inputs, "shadcn-inputs-"+tc.name, goldenTheme(tc.th), goldenFrame(260, h), goldenFocus(focused))
		})
	}
}

// The embedded UI faces render the same on every machine: Geist at
// regular, medium and semibold, and Geist Mono (#194).
func TestGoldenUIFont(t *testing.T) {
	ui, err := sysfont.UI()
	if err != nil {
		t.Fatal(err)
	}
	mono, err := sysfont.UIMono()
	if err != nil {
		t.Fatal(err)
	}
	col := NewBox(Column, 4, 0)
	for _, w := range []int{400, 500, 600} {
		face, err := sysfont.Weighted(ui.Family(), 15, w, false)
		if err != nil {
			t.Fatal(err)
		}
		col.Append(NewLabel(face, 15, fmt.Sprintf("Geist %d - The quick brown fox", w), 0), false)
	}
	col.Append(NewLabel(mono, 14, "Geist Mono: func main() {}", 0), false)
	NewGolden(t, col, "ui-font")
}

// .glass over a busy backdrop, in both palettes: the stripes under the
// card blur and saturate inside its rounded shape and stay sharp
// around it (#195).
func TestGoldenGlass(t *testing.T) {
	face := goldenFace(t)
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			backdrop := NewBox(Column, 0, 0)
			backdrop.AddClass("stripes")
			card := NewBox(Column, 4, 16)
			card.AddClass(css.Glass)
			card.Append(NewLabel(face, 15, "Clear glass", 0), false).
				Append(NewLabel(face, 13, "blurred, saturated backdrop", 0), false)
			stage := NewOverlay().Append(backdrop).AppendAligned(card, AlignCenter, AlignCenter)
			cssShot(t, `.stripes { background-image: repeating-linear-gradient(45deg, #e05561 0px, #e05561 10px, #3fb68b 10px, #3fb68b 20px, #4f7cff 20px, #4f7cff 30px); }`,
				stage, "glass-"+tc.name, goldenTheme(tc.th), goldenFrame(260, 140))
		})
	}
}

// A .glass window root whose compositor blur is not in effect turns
// opaque: the card color instead of the translucent glass (#196).
func TestGlassBlurUnavailableIsOpaque(t *testing.T) {
	SetTheme(DarkTheme())
	defer SetTheme(nil)
	glass := NewBox(Column, 0, 0)
	glass.AddClass(css.Glass)
	if v := glass.style(glass); v.Background == Current().CardColor() {
		t.Fatal("plain .glass is already opaque; the test proves nothing")
	}
	glass.AddClass(css.BlurUnavailable)
	if v := glass.style(glass); v.Background != Current().CardColor() {
		t.Errorf("fallback background %08x, want the opaque card color %08x", uint32(v.Background), uint32(Current().CardColor()))
	}
}
