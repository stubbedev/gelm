package widget

import (
	"bytes"
	"log/slog"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/stubbedev/gelm/internal/logutil"
	"github.com/stubbedev/gelm/render"
)

func paintSwitchTrack(t *testing.T) render.Color {
	t.Helper()
	data := make([]byte, render.Stride(40)*22)
	cv := render.New(data, render.Stride(40), 40, 22)
	s := NewSwitch(false)
	s.Measure(Constraints{Max: Size{W: 100, H: 100}})
	s.Arrange(render.Rect{X: 0, Y: 0, W: 40, H: 22})
	s.Paint(cv)
	// (38, 11) is inside the track, clear of the knob.
	return render.ColorFromBytes(data[11*render.Stride(40)+38*4:])
}

func TestThemeRestyles(t *testing.T) {
	defer SetTheme(DarkTheme())

	t.Run("swapping the theme repaints controls", func(t *testing.T) {
		SetTheme(DarkTheme())
		dark := paintSwitchTrack(t)
		SetTheme(LightTheme())
		light := paintSwitchTrack(t)
		SetTheme(DarkTheme())
		again := paintSwitchTrack(t)

		if dark == light {
			t.Errorf("switch track identical under dark and light themes: %v", dark)
		}
		if again != dark {
			t.Errorf("restoring the theme did not restore colors: %v != %v", again, dark)
		}
	})

	t.Run("track follows theme surface, not a hardcoded color", func(t *testing.T) {
		SetTheme(DarkTheme())
		if got := paintSwitchTrack(t); got != DarkTheme().Surface {
			t.Errorf("track = %v, want theme surface", got)
		}
	})
}

func TestThemeExplicitColorWins(t *testing.T) {
	defer SetTheme(DarkTheme())

	green := render.RGB(0, 200, 0)
	b := NewButton(newStub(0, 0), 0, 0)
	b.Bg = green
	b.Measure(Constraints{Max: Size{W: 100, H: 100}})
	b.Arrange(render.Rect{X: 0, Y: 0, W: 20, H: 20})

	paint := func() render.Color {
		data := make([]byte, render.Stride(20)*20)
		cv := render.New(data, render.Stride(20), 20, 20)
		b.Hovered, b.Pressed = false, false
		b.Paint(cv)
		return render.ColorFromBytes(data[10*render.Stride(20)+10*4:])
	}

	SetTheme(DarkTheme())
	dark := paint()
	SetTheme(LightTheme())
	light := paint()

	t.Run("explicit colors survive a theme swap", func(t *testing.T) {
		if dark != green || light != green {
			t.Errorf("explicit Bg = %v (dark), %v (light), want %v", dark, light, green)
		}
	})
}

func TestThemeDefaults(t *testing.T) {
	t.Run("SetTheme nil restores dark", func(t *testing.T) {
		SetTheme(LightTheme())
		SetTheme(nil)
		if Current().Bg != DarkTheme().Bg {
			t.Errorf("bg = %v, want the dark theme bg", Current().Bg)
		}
	})
}

func TestThemePresetsPinned(t *testing.T) {
	// Literal pins: wayle composes from these palettes, so a silent
	// value change would restyle every existing user.
	dark, light := DarkTheme(), LightTheme()
	wantDark := Theme{
		Bg:             render.RGB(0x1E, 0x1E, 0x2E),
		Surface:        render.RGB(0x18, 0x18, 0x24),
		SurfaceHover:   render.RGB(0x22, 0x22, 0x33),
		SurfacePressed: render.RGB(0x11, 0x11, 0x1B),
		Text:           render.RGB(0xCD, 0xD6, 0xF4),
		TextMuted:      render.RGB(0xA6, 0xAD, 0xC3),
		Accent:         render.RGB(0x89, 0xB4, 0xFA),
		OnAccent:       render.RGB(0x11, 0x11, 0x1B),
		Border:         render.RGB(0x58, 0x5B, 0x70),
		ShadowColor:    render.RGBA(0, 0, 0, 120),
		ShadowBlur:     16,
		Radius:         6,
		Spacing:        8,
		Padding:        8,
		TextSize:       14,
		Animations:     true,
	}
	wantLight := Theme{
		Bg:             render.RGB(0xEF, 0xF1, 0xF5),
		Surface:        render.RGB(0xFF, 0xFF, 0xFF),
		SurfaceHover:   render.RGB(0xE8, 0xEC, 0xF4),
		SurfacePressed: render.RGB(0xD3, 0xDA, 0xE8),
		Text:           render.RGB(0x2A, 0x2C, 0x38),
		TextMuted:      render.RGB(0x5C, 0x63, 0x76),
		Accent:         render.RGB(0x1E, 0x66, 0xF5),
		OnAccent:       render.RGB(0xFF, 0xFF, 0xFF),
		Border:         render.RGB(0xB8, 0xC0, 0xD4),
		ShadowColor:    render.RGBA(0, 0, 0, 70),
		ShadowBlur:     16,
		Radius:         6,
		Spacing:        8,
		Padding:        8,
		TextSize:       14,
		Animations:     true,
	}
	if !reflect.DeepEqual(*dark, wantDark) {
		t.Errorf("DarkTheme drifted: %+v", *dark)
	}
	if !reflect.DeepEqual(*light, wantLight) {
		t.Errorf("LightTheme drifted: %+v", *light)
	}
}

func TestThemeWithChainingIsImmutable(t *testing.T) {
	green := render.RGB(0xa6, 0xe3, 0xa1)
	base := DarkTheme()

	accented := base.WithAccent(green)
	if accented == base {
		t.Fatal("WithAccent returned the receiver; chaining must copy")
	}
	if accented.Accent != green {
		t.Errorf("WithAccent accent = %v, want %v", accented.Accent, green)
	}
	if base.Accent != DarkTheme().Accent {
		t.Errorf("WithAccent mutated the receiver: accent %v", base.Accent)
	}

	padded := accented.WithPadding(12)
	if padded.Padding != 12 || accented.Padding != DarkTheme().Padding {
		t.Errorf("WithPadding: padded = %d, prior link = %d, want %d and %d",
			padded.Padding, accented.Padding, 12, DarkTheme().Padding)
	}
	if padded.Accent != green {
		t.Errorf("chain dropped the accent: %v, want %v", padded.Accent, green)
	}
	if !reflect.DeepEqual(*base, *DarkTheme()) {
		t.Errorf("the original theme changed: %+v", *base)
	}

	// Every With* pairs with a field, including the metrics the wayle
	// config drives and the shadow knobs.
	composed := DarkTheme().WithBg(green).WithSurface(green).WithText(green).
		WithTextMuted(green).WithOnAccent(green).WithBorder(green).
		WithSurfaceHover(green).WithSurfacePressed(green).
		WithRadius(3).WithSpacing(4).WithPadding(5).WithTextSize(11).
		WithShadowColor(green).WithShadowBlur(9)
	if composed.Bg != green || composed.Surface != green || composed.Text != green ||
		composed.TextMuted != green || composed.OnAccent != green || composed.Border != green ||
		composed.SurfaceHover != green || composed.SurfacePressed != green ||
		composed.Radius != 3 || composed.Spacing != 4 || composed.Padding != 5 ||
		composed.TextSize != 11 || composed.ShadowColor != green || composed.ShadowBlur != 9 {
		t.Errorf("a With* did not set its field: %+v", composed)
	}

	// ShadowBlur zero is the disable switch; the presets ship it on.
	if DarkTheme().ShadowBlur == 0 || LightTheme().ShadowBlur == 0 {
		t.Error("a preset ships with shadows disabled")
	}
}

func TestParseColor(t *testing.T) {
	t.Run("the markup hex forms parse", func(t *testing.T) {
		for s, want := range map[string]render.Color{
			"#a6e3a1":   render.RGBA(0xa6, 0xe3, 0xa1, 0xff),
			"#A6E3A1":   render.RGBA(0xa6, 0xe3, 0xa1, 0xff),
			"#f0a":      render.RGBA(0xff, 0x00, 0xaa, 0xff),
			"#ffffff80": render.RGBA(0xff, 0xff, 0xff, 0x80),
		} {
			got, err := ParseColor(s)
			if err != nil {
				t.Errorf("ParseColor(%q) = %v", s, err)
				continue
			}
			if got != want {
				t.Errorf("ParseColor(%q) = %v, want %v", s, got, want)
			}
		}
	})

	t.Run("malformed strings error instead of defaulting", func(t *testing.T) {
		for _, s := range []string{"", "a6e3a1", "#12", "#12345", "#a6e3a11", "#a6e3g1", "green"} {
			c, err := ParseColor(s)
			if err == nil {
				t.Errorf("ParseColor(%q) = %v, want an error", s, c)
			} else if !strings.Contains(err.Error(), s) {
				t.Errorf("ParseColor(%q) error %q does not name the input", s, err)
			}
		}
	})

	t.Run("WithAccentHex errors without touching the theme", func(t *testing.T) {
		base := DarkTheme()
		got, err := base.WithAccentHex("#zzz")
		if err == nil {
			t.Fatal("WithAccentHex(#zzz) = nil error, want an error")
		}
		if got != nil {
			t.Errorf("WithAccentHex(#zzz) returned a theme with accent %v; want nil so no default can leak", got.Accent)
		}
		if !reflect.DeepEqual(*base, *DarkTheme()) {
			t.Errorf("the failed parse mutated the base: %+v", *base)
		}
	})

	t.Run("WithAccentHex parses valid input", func(t *testing.T) {
		got, err := DarkTheme().WithAccentHex("#a6e3a1")
		if err != nil {
			t.Fatal(err)
		}
		if got.Accent != render.RGBA(0xa6, 0xe3, 0xa1, 0xff) {
			t.Errorf("accent = %v, want #a6e3a1", got.Accent)
		}
	})
}

func TestThemeDerivations(t *testing.T) {
	// A palette composed from scratch: no explicit state shades.
	dark := &Theme{
		Bg:      render.RGB(0x1E, 0x1E, 0x2E),
		Surface: render.RGB(0x18, 0x18, 0x24),
		Text:    render.RGB(0xCD, 0xD6, 0xF4),
		Accent:  render.RGB(0x89, 0xB4, 0xFA),
	}
	light := &Theme{
		Surface: render.RGB(0xFF, 0xFF, 0xFF),
		Text:    render.RGB(0x2A, 0x2C, 0x38),
		Accent:  render.RGB(0x1E, 0x66, 0xF5),
	}

	t.Run("hover shifts toward text, pressed toward black", func(t *testing.T) {
		// Pinned arithmetic: mix(Surface, Text, 0.08) and
		// mix(Surface, black, 0.20) on the dark surface above.
		wantHover := render.RGB(38, 39, 53)
		wantPressed := render.RGB(19, 19, 29)
		if got := dark.HoverSurface(); got != wantHover {
			t.Errorf("dark hover = %v, want %v", got, wantHover)
		}
		if got := dark.PressedSurface(); got != wantPressed {
			t.Errorf("dark pressed = %v, want %v", got, wantPressed)
		}
		lum := func(c render.Color) float64 {
			s, g, b := float64(c.R()), float64(c.G()), float64(c.B())
			return s + g + b
		}
		if lum(dark.HoverSurface()) <= lum(dark.Surface) {
			t.Errorf("dark hover %v did not lighten the surface", dark.HoverSurface())
		}
		if lum(light.HoverSurface()) >= lum(light.Surface) {
			t.Errorf("light hover %v did not darken the surface", light.HoverSurface())
		}
		if lum(dark.PressedSurface()) >= lum(dark.Surface) {
			t.Errorf("dark pressed %v did not recede below the surface", dark.PressedSurface())
		}
		if lum(light.PressedSurface()) >= lum(light.Surface) {
			t.Errorf("light pressed %v did not recede below the surface", light.PressedSurface())
		}
	})

	t.Run("derivation is a pure function of the palette", func(t *testing.T) {
		for _, th := range []*Theme{dark, light, DarkTheme(), LightTheme()} {
			if a, b := th.HoverSurface(), th.HoverSurface(); a != b {
				t.Errorf("HoverSurface unstable: %v then %v", a, b)
			}
			if a, b := th.PressedSurface(), th.PressedSurface(); a != b {
				t.Errorf("PressedSurface unstable: %v then %v", a, b)
			}
			if a, b := th.DisabledText(), th.DisabledText(); a != b {
				t.Errorf("DisabledText unstable: %v then %v", a, b)
			}
			if a, b := th.DisabledSurface(), th.DisabledSurface(); a != b {
				t.Errorf("DisabledSurface unstable: %v then %v", a, b)
			}
			if a, b := th.DisabledAccent(), th.DisabledAccent(); a != b {
				t.Errorf("DisabledAccent unstable: %v then %v", a, b)
			}
		}
	})

	t.Run("disabled surface recedes halfway toward the background", func(t *testing.T) {
		// Pinned arithmetic: mix(Surface, Bg, 0.5) on the dark palette above.
		want := render.RGB(27, 27, 41)
		if got := dark.DisabledSurface(); got != want {
			t.Errorf("DisabledSurface = %v, want %v", got, want)
		}
	})

	t.Run("disabled accent fades accent like disabled text fades text", func(t *testing.T) {
		want := Color(uint32(float64(dark.Accent) * 0.45))
		if got := dark.DisabledAccent(); got != want {
			t.Errorf("DisabledAccent = %v, want accent at 45%% alpha: %v", got, want)
		}
	})

	t.Run("explicit disabled surface wins over derivation", func(t *testing.T) {
		explicit := render.RGB(9, 8, 7)
		if got := dark.WithSurfaceDisabled(explicit).DisabledSurface(); got != explicit {
			t.Errorf("DisabledSurface = %v, want the explicit %v", got, explicit)
		}
	})

	t.Run("explicit state shades win over derivation", func(t *testing.T) {
		hover, pressed, muted := render.RGB(1, 2, 3), render.RGB(4, 5, 6), render.RGB(7, 8, 9)
		th := dark.WithSurfaceHover(hover).WithSurfacePressed(pressed).WithTextMuted(muted)
		if got := th.HoverSurface(); got != hover {
			t.Errorf("HoverSurface = %v, want the explicit %v", got, hover)
		}
		if got := th.PressedSurface(); got != pressed {
			t.Errorf("PressedSurface = %v, want the explicit %v", got, pressed)
		}
		if got := th.DisabledText(); got != muted {
			t.Errorf("DisabledText = %v, want the explicit %v", got, muted)
		}
		// Presets pin their shades, so they pass through untouched.
		if got := DarkTheme().HoverSurface(); got != DarkTheme().SurfaceHover {
			t.Errorf("dark preset hover = %v, want the pinned %v", got, DarkTheme().SurfaceHover)
		}
	})

	t.Run("hover accent is the accent wash at a fixed alpha", func(t *testing.T) {
		for _, th := range []*Theme{dark, light, DarkTheme(), LightTheme()} {
			got := th.HoverAccent()
			if got.A() != 70 {
				t.Errorf("HoverAccent alpha = %d, want 70", got.A())
			}
			if want := render.RGBA(th.Accent.R(), th.Accent.G(), th.Accent.B(), 70); got != want {
				t.Errorf("HoverAccent = %v, want the accent at hover alpha: %v", got, want)
			}
		}
	})

	t.Run("disabled text fades text when no muted role is set", func(t *testing.T) {
		want := Color(uint32(float64(dark.Text) * 0.45))
		if got := dark.DisabledText(); got != want {
			t.Errorf("DisabledText = %v, want text at 45%% alpha: %v", got, want)
		}
		if dark.DisabledText() == dark.Text {
			t.Error("DisabledText ignored the fade")
		}
	})
}

func TestThemeContrastRatio(t *testing.T) {
	white, black := render.RGB(0xff, 0xff, 0xff), render.RGB(0, 0, 0)
	if got := contrastRatio(white, black); math.Abs(got-21) > 0.01 {
		t.Errorf("contrastRatio(white, black) = %.3f, want 21", got)
	}
	if got := contrastRatio(white, black); got != contrastRatio(black, white) {
		t.Error("contrastRatio is not symmetric")
	}
	// The classic AA boundary: #777 on white just misses 4.5:1.
	if got := contrastRatio(white, render.RGB(0x77, 0x77, 0x77)); got >= 4.5 {
		t.Errorf("#777 on white = %.3f, want below 4.5", got)
	}
	if got := contrastRatio(white, render.RGB(0x76, 0x76, 0x76)); got < 4.5 {
		t.Errorf("#767676 on white = %.3f, want at or above 4.5", got)
	}
}

func TestThemeContrastGuard(t *testing.T) {
	defer SetTheme(DarkTheme())
	SetTheme(DarkTheme())

	var warns []string
	orig := themeWarn
	themeWarn = func(msg string) { warns = append(warns, msg) }
	defer func() { themeWarn = orig }()

	t.Run("clean palettes stay silent", func(t *testing.T) {
		warns = nil
		SetTheme(DarkTheme())
		SetTheme(LightTheme())
		SetTheme(nil)
		if len(warns) != 0 {
			t.Errorf("preset palettes warned: %v", warns)
		}
	})

	t.Run("text below AA warns but still applies", func(t *testing.T) {
		warns = nil
		low := DarkTheme().WithText(DarkTheme().Surface) // text on its own surface color
		SetTheme(low)
		if len(warns) != 1 || !strings.Contains(warns[0], "Text/Bg") || !strings.Contains(warns[0], "WCAG AA") {
			t.Fatalf("warns = %v, want one Text/Bg WCAG AA warning", warns)
		}
		if Current() != low {
			t.Error("the warning rejected the theme; the guard must warn, not fail")
		}
	})

	t.Run("muted text below AA warns", func(t *testing.T) {
		warns = nil
		SetTheme(DarkTheme().WithTextMuted(DarkTheme().Surface))
		if len(warns) != 1 || !strings.Contains(warns[0], "TextMuted/Bg") {
			t.Fatalf("warns = %v, want one TextMuted/Bg warning", warns)
		}
	})

	t.Run("unset roles are skipped", func(t *testing.T) {
		warns = nil
		SetTheme(&Theme{Bg: render.RGB(0x10, 0x10, 0x10)}) // Text unset: falls back at paint time
		if len(warns) != 0 {
			t.Errorf("unset roles warned: %v", warns)
		}
	})
}

// TestThemeContrastWarnsThroughLogger: the default themeWarn sink is
// the injected library logger at Warn — visible only when the
// application opted into gelm's logging, silent otherwise (the
// discarding default).
func TestThemeContrastWarnsThroughLogger(t *testing.T) {
	defer SetTheme(DarkTheme())

	var buf bytes.Buffer
	logutil.Set(slog.New(slog.NewTextHandler(&buf, nil)))
	defer logutil.Set(nil)

	SetTheme(DarkTheme().WithText(DarkTheme().Surface))
	if got := buf.String(); !strings.Contains(got, "level=WARN") || !strings.Contains(got, "Text/Bg") {
		t.Errorf("contrast warning missing from the injected logger:\n%s", got)
	}

	buf.Reset()
	SetTheme(DarkTheme())
	if buf.Len() != 0 {
		t.Errorf("clean palette logged:\n%s", buf.String())
	}
}

func TestSetThemeComposedThemeRepaints(t *testing.T) {
	defer SetTheme(DarkTheme())

	SetTheme(DarkTheme())
	before := paintSwitchTrack(t)
	gen := themeGen

	green := render.RGB(0, 200, 0)
	SetTheme(DarkTheme().WithSurface(green))

	if themeGen != gen+1 {
		t.Errorf("themeGen = %d after SetTheme, want %d: a theme swap must bump the generation so damage repaints", themeGen, gen+1)
	}
	if got := paintSwitchTrack(t); got != green {
		t.Errorf("switch track = %v after the composed theme, want the custom surface %v", got, green)
	}
	if got := paintSwitchTrack(t); got == before {
		t.Errorf("track unchanged after the composed theme: %v", got)
	}
}
