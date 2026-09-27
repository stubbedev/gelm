package widget

import (
	"fmt"
	"math"

	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/render"
)

// Theme holds the palette and metrics widgets paint with. Colors left at
// zero on a widget fall back to the theme at paint time, so replacing the
// theme restyles everything on the next frame.
//
// A Theme is an immutable value: the With* methods return a fresh copy
// and never touch the receiver, so chains compose from any preset —
//
//	widget.SetTheme(widget.DarkTheme().WithAccent(green).WithPadding(10))
//
// Colors arrive from config files as hex strings; parse them with
// ParseColor or set them with WithAccentHex, which errors on malformed
// input instead of defaulting silently.
type Theme struct {
	// Bg is the window background; Surface fills controls at rest.
	Bg, Surface Color
	// SurfaceHover and SurfacePressed restyle controls under the
	// pointer or while pressed. Leave them unset and HoverSurface
	// and PressedSurface derive them from Surface.
	SurfaceHover, SurfacePressed Color
	// Text paints primary labels; TextMuted groups and hints. Leave
	// TextMuted unset and DisabledText derives it from Text.
	Text, TextMuted Color
	// Accent drives fills that express state: progress, switches on,
	// sliders. HoverAccent derives the translucent hover wash from it.
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
// The next frame repaints every widget with the new palette. In debug
// builds (gelmdebug tag) any palette pair whose contrast falls below
// WCAG AA traces a warning on the "theme" category; the check compiles
// out of prod builds.
func SetTheme(t *Theme) {
	if t == nil {
		t = DarkTheme()
	}
	current = t
	themeGen++
	for _, w := range t.contrastWarnings() {
		themeWarn(w)
	}
}

// themeWarn is the contrast-warning sink: debug builds route to the
// trace log, where GOELM_DEBUG admits categories; without the
// gelmdebug tag debug.Log is an empty function. Tests swap it to
// capture warnings.
var themeWarn = func(msg string) { debug.Log("theme", "%s", msg) }

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

// ParseColor reads a hex color — #rgb, #rrggbb, or #rrggbbaa, the same
// forms the markup parser accepts — into a premultiplied Color. It
// reports malformed input instead of defaulting, so a config file's
// mistake fails loudly at theme-construction time.
func ParseColor(s string) (Color, error) {
	c, ok := parseHexColor(s)
	if !ok {
		return 0, fmt.Errorf("gelm: invalid color %q: want #rgb, #rrggbb, or #rrggbbaa", s)
	}
	return c, nil
}

// with copies t and applies f to the copy. Every With* goes through
// here, so chaining is immutable by construction: the receiver keeps
// its values and each link in a chain is a distinct theme.
func (t *Theme) with(f func(*Theme)) *Theme {
	n := *t
	f(&n)
	return &n
}

// WithBg returns a copy with the window background set.
func (t *Theme) WithBg(c Color) *Theme { return t.with(func(n *Theme) { n.Bg = c }) }

// WithSurface returns a copy with the control fill set.
func (t *Theme) WithSurface(c Color) *Theme { return t.with(func(n *Theme) { n.Surface = c }) }

// WithSurfaceHover returns a copy with the hover shade set; unset, the
// copy derives one (HoverSurface).
func (t *Theme) WithSurfaceHover(c Color) *Theme {
	return t.with(func(n *Theme) { n.SurfaceHover = c })
}

// WithSurfacePressed returns a copy with the pressed shade set; unset,
// the copy derives one (PressedSurface).
func (t *Theme) WithSurfacePressed(c Color) *Theme {
	return t.with(func(n *Theme) { n.SurfacePressed = c })
}

// WithText returns a copy with the primary text color set.
func (t *Theme) WithText(c Color) *Theme { return t.with(func(n *Theme) { n.Text = c }) }

// WithTextMuted returns a copy with the secondary text color set.
func (t *Theme) WithTextMuted(c Color) *Theme {
	return t.with(func(n *Theme) { n.TextMuted = c })
}

// WithAccent returns a copy with the accent color set.
func (t *Theme) WithAccent(c Color) *Theme { return t.with(func(n *Theme) { n.Accent = c }) }

// WithAccentHex returns a copy with the accent parsed from a hex
// string (#rgb, #rrggbb, #rrggbbaa). A malformed string is an error,
// never a silent default — theme construction is where a config
// mistake must surface.
func (t *Theme) WithAccentHex(s string) (*Theme, error) {
	c, err := ParseColor(s)
	if err != nil {
		return nil, err
	}
	return t.WithAccent(c), nil
}

// WithOnAccent returns a copy with the on-accent glyph color set.
func (t *Theme) WithOnAccent(c Color) *Theme { return t.with(func(n *Theme) { n.OnAccent = c }) }

// WithBorder returns a copy with the outline color set.
func (t *Theme) WithBorder(c Color) *Theme { return t.with(func(n *Theme) { n.Border = c }) }

// WithRadius returns a copy with the default corner rounding set.
func (t *Theme) WithRadius(px int) *Theme { return t.with(func(n *Theme) { n.Radius = px }) }

// WithSpacing returns a copy with the default layout gap set.
func (t *Theme) WithSpacing(px int) *Theme { return t.with(func(n *Theme) { n.Spacing = px }) }

// WithPadding returns a copy with the default control padding set.
func (t *Theme) WithPadding(px int) *Theme { return t.with(func(n *Theme) { n.Padding = px }) }

// WithTextSize returns a copy with the default label size set.
func (t *Theme) WithTextSize(px float64) *Theme { return t.with(func(n *Theme) { n.TextSize = px }) }

// Derived state colors. The presets pin explicit shades; themes
// composed from scratch get the same states by derivation, so widgets
// read state colors through these methods instead of mixing their own.
// All derivations are pure functions of the palette: same palette in,
// same color out, on every widget.

// HoverSurface returns the control fill under the pointer: the
// explicit SurfaceHover when set, otherwise Surface mixed 8% toward
// Text. The text pole carries the palette's polarity, so the shift
// lightens dark palettes and darkens light ones.
func (t *Theme) HoverSurface() Color {
	if t.SurfaceHover != 0 {
		return t.SurfaceHover
	}
	return mix(t.Surface, t.Text, 0.08)
}

// PressedSurface returns the control fill while pressed: the explicit
// SurfacePressed when set, otherwise Surface mixed 20% toward black —
// the surface recedes under a press on any palette.
func (t *Theme) PressedSurface() Color {
	if t.SurfacePressed != 0 {
		return t.SurfacePressed
	}
	return mix(t.Surface, render.RGB(0, 0, 0), 0.20)
}

// HoverAccent returns the translucent accent wash rows and items paint
// under the pointer: Accent at alpha 70, the same derivation every
// highlighter uses.
func (t *Theme) HoverAccent() Color {
	return render.RGBA(t.Accent.R(), t.Accent.G(), t.Accent.B(), 70)
}

// DisabledText returns the color for rows and labels that ignore
// input: the explicit TextMuted when set, otherwise Text faded to 45%
// alpha.
func (t *Theme) DisabledText() Color {
	if t.TextMuted != 0 {
		return t.TextMuted
	}
	return scaleAlpha(t.Text, 0.45)
}

// wcagAA is the WCAG 2.x AA contrast ratio for normal text.
const wcagAA = 4.5

// contrastWarnings reports one line per role whose contrast against
// the background falls below WCAG AA. Roles left unset (transparent)
// are skipped — they fall back to the presets at paint time. SetTheme
// traces the result in debug builds; it never fails the swap.
func (t *Theme) contrastWarnings() []string {
	var out []string
	pair := func(role string, fg Color) {
		if fg.A() != 255 || t.Bg.A() != 255 {
			return
		}
		if r := contrastRatio(fg, t.Bg); r < wcagAA {
			out = append(out, fmt.Sprintf(
				"theme %s/Bg contrast %.2f:1 is below WCAG AA (%.1f:1)", role, r, wcagAA))
		}
	}
	pair("Text", t.Text)
	pair("TextMuted", t.TextMuted)
	return out
}

// contrastRatio reports the WCAG 2.x contrast ratio of two opaque
// colors, (L1+0.05)/(L2+0.05) with L the relative luminance.
func contrastRatio(a, b Color) float64 {
	la, lb := wcagLuminance(a), wcagLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// wcagLuminance computes the WCAG 2.x relative luminance of an opaque
// sRGB color: the linearized channels under the perceptual weights.
func wcagLuminance(c Color) float64 {
	lin := func(p uint8) float64 {
		s := float64(p) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c.R()) + 0.7152*lin(c.G()) + 0.0722*lin(c.B())
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

// mix interpolates two premultiplied colors at p in [0, 1] — the same
// arithmetic the renderer uses for gradients: premultiplied channels
// interpolate correctly.
func mix(a, b Color, p float64) Color {
	ch := func(x, y uint8) uint32 {
		return uint32(float64(x) + (float64(y)-float64(x))*p + 0.5)
	}
	return Color(ch(a.A(), b.A())<<24 |
		ch(a.R(), b.R())<<16 |
		ch(a.G(), b.G())<<8 |
		ch(a.B(), b.B()))
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
