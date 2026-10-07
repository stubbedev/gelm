package widget

import "testing"

// cssSwatch is a 60x40 box styled by class c.
func cssSwatch(c string) *Box {
	b := NewBox(Column, 0, 0)
	b.Append(NewSpacer(60, 40), false)
	b.AddClass(c)
	return b
}

// TestGoldenGradientForms pins every CSS gradient form through the
// cascade: linear, radial (ellipse, circle at an offset center),
// conic, and the three repeating forms.
func TestGoldenGradientForms(t *testing.T) {
	root := NewBox(Column, 6, 0)
	root.AttachStylesheet(NewStylesheet(`
		.lin  { background-image: linear-gradient(45deg, #1e66f5, #f5c2e7 50%, #a6e3a1); border-radius: 8px; }
		.rad  { background-image: radial-gradient(#f9e2af, #fab387 60%, #1e1e2e); }
		.circ { background-image: radial-gradient(circle closest-side at 30% 40%, #ffffff, #89b4fa 70%, #11111b); }
		.con  { background-image: conic-gradient(from 45deg, #f38ba8, #f9e2af, #a6e3a1, #89b4fa, #f38ba8); border-radius: 20px; }
		.rlin { background-image: repeating-linear-gradient(90deg, #313244, #313244 10%, #cba6f7 10%, #cba6f7 20%); }
		.rrad { background-image: repeating-radial-gradient(circle 6px, #94e2d5, #1e1e2e); }
		.rcon { background-image: repeating-conic-gradient(#f5e0dc 0deg, #f5e0dc 15deg, #45475a 15deg, #45475a 30deg); }
	`, StylePriorityUser))
	rows := [][]string{{"lin", "rad", "circ", "con"}, {"rlin", "rrad", "rcon"}}
	for _, r := range rows {
		row := NewBox(Row, 6, 0)
		for _, c := range r {
			row.Append(cssSwatch(c), false)
		}
		root.Append(row, false)
	}
	NewGolden(t, root, "gradient-forms", goldenTheme(DarkTheme()))
}
