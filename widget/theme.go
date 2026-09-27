package widget

import "github.com/stubbedev/gelm/render"

// Theme holds the palette and metrics widgets paint with. Colors left at
// zero on a widget fall back to the theme at paint time, so replacing the
// theme restyles everything on the next frame.
type Theme struct {
	// Bg is the window background; Surface fills controls at rest.
	Bg, Surface Color
	// SurfaceHover and SurfacePressed restyle controls under the
	// pointer or while pressed.
	SurfaceHover, SurfacePressed Color
	// Text paints primary labels; TextMuted groups and hints.
	Text, TextMuted Color
	// Accent drives fills that express state: progress, switches on,
	// sliders.
	Accent Color
	// OnAccent paints glyphs and knobs on top of Accent.
	OnAccent Color
	// Border strokes control outlines.
	Border Color
	// Radius is the default corner rounding; Spacing and Padding are
	// layout defaults.
	Radius  int
	Spacing int
	Padding int
	// TextSize is the default label size in pixels.
	TextSize float64
}

// Color aliases render.Color so themes read naturally without importing
// render at every call site.
type Color = render.Color

// themeGen bumps on every SetTheme so the damage collector repaints
// each widget once after a restyle (node.takeDamage compares stamps).
var themeGen uint64 = 1

var current = DarkTheme()

// Current returns the active theme.
func Current() *Theme { return current }

// SetTheme replaces the active theme. Passing nil restores DarkTheme.
// The next frame repaints every widget with the new palette.
func SetTheme(t *Theme) {
	if t == nil {
		t = DarkTheme()
	}
	current = t
	themeGen++
}

// DarkTheme returns the default dark palette.
func DarkTheme() *Theme {
	return &Theme{
		Bg:             render.RGB(0x1E, 0x1E, 0x2E),
		Surface:        render.RGB(0x18, 0x18, 0x24),
		SurfaceHover:   render.RGB(0x22, 0x22, 0x33),
		SurfacePressed: render.RGB(0x11, 0x11, 0x1B),
		Text:           render.RGB(0xCD, 0xD6, 0xF4),
		TextMuted:      render.RGB(0xA6, 0xAD, 0xC3),
		Accent:         render.RGB(0x89, 0xB4, 0xFA),
		OnAccent:       render.RGB(0x11, 0x11, 0x1B),
		Border:         render.RGB(0x58, 0x5B, 0x70),
		Radius:         6,
		Spacing:        8,
		Padding:        8,
		TextSize:       14,
	}
}

// LightTheme returns a light palette.
func LightTheme() *Theme {
	return &Theme{
		Bg:             render.RGB(0xEF, 0xF1, 0xF5),
		Surface:        render.RGB(0xFF, 0xFF, 0xFF),
		SurfaceHover:   render.RGB(0xE8, 0xEC, 0xF4),
		SurfacePressed: render.RGB(0xD3, 0xDA, 0xE8),
		Text:           render.RGB(0x2A, 0x2C, 0x38),
		TextMuted:      render.RGB(0x5C, 0x63, 0x76),
		Accent:         render.RGB(0x1E, 0x66, 0xF5),
		OnAccent:       render.RGB(0xFF, 0xFF, 0xFF),
		Border:         render.RGB(0xB8, 0xC0, 0xD4),
		Radius:         6,
		Spacing:        8,
		Padding:        8,
		TextSize:       14,
	}
}

// or picks b when a is the zero color.
func (t *Theme) or(a Color, b Color) Color {
	if a == 0 {
		return b
	}
	return a
}

// resolve maps a widget color onto the theme.
func (t *Theme) resolve(a Color, role Color) Color {
	return t.or(a, role)
}

// scaleAlpha fades a premultiplied color toward transparent by p in
// [0, 1]. Premultiplied channels scale linearly, so multiplying the
// packed value fades alpha, red, green, and blue together.
func scaleAlpha(c Color, p float64) Color {
	if p >= 1 {
		return c
	}
	if p <= 0 {
		return 0
	}
	return Color(uint32(float64(c) * p))
}
