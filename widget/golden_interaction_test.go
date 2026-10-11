package widget

// The interaction-property golden set (docs/css.md): caret-color and
// caret-shape reaching the entry's painted caret, accent-color
// reaching the checked/accent surfaces.

import (
	"testing"
)

func TestGoldenCSSCaret(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	entry := NewEntry(face, 14, th.Text)
	entry.SetText("caret here")
	cssShot(t, `
		entry { caret-color: #e05561; caret-shape: block; }
	`, entry, "css-entry-caret-block", goldenTheme(th), goldenFrame(160, 30),
		goldenAfterArrange(func() { entry.focused = true }))
	bar := NewEntry(face, 14, th.Text)
	bar.SetText("bar caret")
	cssShot(t, `
		entry { caret-color: #3fb68b; caret-shape: underscore; }
	`, bar, "css-entry-caret-underscore", goldenTheme(th), goldenFrame(160, 30),
		goldenAfterArrange(func() { bar.focused = true }))
}

func TestGoldenCSSAccentColor(t *testing.T) {
	th := DarkTheme()
	sw := NewSwitch(true)
	cssShot(t, `switch { accent-color: #e05561; }`, sw, "css-accent-switch", goldenTheme(th))
	check := NewCheckButton(true)
	cssShot(t, `checkbutton { accent-color: #3fb68b; }`, check, "css-accent-checkbutton", goldenTheme(th))
	bar := NewProgressBar(0.55)
	cssShot(t, `progressbar { accent-color: #e05561; }`, bar, "css-accent-progressbar", goldenTheme(th))
	level := NewLevelBar(0.55)
	cssShot(t, `levelbar { accent-color: #3fb68b; }`, level, "css-accent-levelbar", goldenTheme(th))
	slider := NewSlider(0, 1, 0, 0.5)
	cssShot(t, `scale { accent-color: #e05561; }`, slider, "css-accent-slider", goldenTheme(th))
}
