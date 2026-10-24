package widget

import (
	"testing"

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
