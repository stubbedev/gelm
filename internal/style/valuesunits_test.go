package style

import (
	"math"
	"testing"
)

// computeEnv computes n under a custom environment.
func computeEnv(s *Sheet, n Node, env Env) Values {
	var sc Scratch
	var v Values
	Compute([]Layer{{Sheet: s, Priority: PriorityApplication}}, n, nil, nil, env, &sc, &v)
	return v
}

// testAttrNode carries attributes attr() resolves.
type testAttrNode struct {
	node
	attrs map[string]string
}

func (n *testAttrNode) StyleAttr(name string) (string, bool) {
	v, ok := n.attrs[name]
	return v, ok
}

func TestViewportUnits(t *testing.T) {
	s := parseOne(t, `
		a { padding: 10vw; margin: 10vh; min-width: 50vmin; min-height: 50vmax; }
		b { padding: calc(5vw + 5px); }
	`)
	env := Env{Rem: 16, Media: Media{Width: 500, Height: 300, DPI: 96}}
	v := computeEnv(s, el("a"), env)
	if v.Padding.Top != 50 || v.Margin.Top != 30 || v.MinWidth != 150 || v.MinHeight != 250 {
		t.Errorf("vw/vh/vmin/vmax: %+v %d %d %d", v.Padding, v.Margin.Top, v.MinWidth, v.MinHeight)
	}
	v = computeEnv(s, el("b"), env)
	if v.Padding.Top != 30 {
		t.Errorf("vw in calc: %d, want 30", v.Padding.Top)
	}
	if v := computeEnv(s, el("a"), Env{Rem: 16}); v.Padding.Top != 0 {
		t.Errorf("vw with no viewport: %d, want 0", v.Padding.Top)
	}
}

func TestFontRelativeUnits(t *testing.T) {
	s := parseOne(t, `
		a { font-size: 20px; padding: 10ch; margin: 10ex; min-width: 10cap; min-height: 10ic; letter-spacing: 1lh; }
		b { font-size: 20px; line-height: 2; padding: 3lh; margin-top: 3rlh; }
		c { font-size: 20px; line-height: normal; padding: 2lh; }
	`)
	env := Env{Rem: 16, Font: FontMetrics{Ch: 0.6, Ex: 0.25, Cap: 0.7, Ic: 1.25, LineHeight: 1.5}, RootLineHeight: 20}
	v := computeEnv(s, el("a"), env)
	if v.Padding.Top != 120 || v.Margin.Top != 50 || v.MinWidth != 140 || v.MinHeight != 250 || v.LetterSpacing != 30 {
		t.Errorf("ch/ex/cap/ic/lh: %+v %+v %d %d %v", v.Padding, v.Margin, v.MinWidth, v.MinHeight, v.LetterSpacing)
	}
	v = computeEnv(s, el("b"), env)
	if v.Padding.Top != 120 || v.Margin.Top != 60 {
		t.Errorf("lh reads the computed line-height, rlh the root's: %d %d", v.Padding.Top, v.Margin.Top)
	}
	v = computeEnv(s, el("c"), env)
	if v.Padding.Top != 60 {
		t.Errorf("lh under line-height normal: %d, want the font's natural 1.5em", v.Padding.Top)
	}
}

func TestFontRelativeUnitFallbacks(t *testing.T) {
	s := parseOne(t, `a { font-size: 20px; padding: 10ch; margin: 10ex; min-width: 10cap; min-height: 10ic; }`)
	v := computeEnv(s, el("a"), Env{Rem: 16})
	if v.Padding.Top != 100 || v.Margin.Top != 100 || v.MinWidth != 100 || v.MinHeight != 200 {
		t.Errorf("CSS fallback ratios: %+v %+v %d %d", v.Padding, v.Margin, v.MinWidth, v.MinHeight)
	}
}

func TestAngleUnits(t *testing.T) {
	s := parseOne(t, `
		a { transform: rotate(0.25turn); }
		b { transform: rotate(200grad); }
		c { transform: rotate(1.5707963267948966rad); }
		d { transform: rotate(-360deg); }
	`)
	deg := func(n *node) float64 {
		v := computeTree(s, n)
		m := v.Transform.M
		return math.Round(math.Atan2(m.B, m.A) * 180 / math.Pi)
	}
	if got := deg(el("a")); got != 90 {
		t.Errorf("turn: %v, want 90", got)
	}
	if got := deg(el("b")); got != 180 {
		t.Errorf("grad: %v, want 180", got)
	}
	if got := deg(el("c")); got != 90 {
		t.Errorf("rad: %v, want 90", got)
	}
	if got := deg(el("d")); got != 0 {
		t.Errorf("deg: %v, want 0", got)
	}
}

func TestMathFunctions(t *testing.T) {
	s := parseOne(t, `
		a { padding: round(5px); margin: round(nearest, 12px, 5px); min-width: round(up, 5px, 12px); min-height: round(down, 5px, 12px); border-spacing: round(to-zero, 5px, 12px); }
		b { padding: mod(14px, 5px); margin: mod(-1px, 5px); min-width: calc(abs(rem(-1px, 5px))); margin-top: rem(14px, 5px); }
		c { opacity: abs(-0.25); letter-spacing: sign(-2px); }
		d { padding: calc(sin(30deg) * 1px); margin: calc(hypot(3, 4) * 1px); min-width: calc(sqrt(81) * 1px); }
		e { min-height: calc(round(pow(2, 10) / 1024 * 4px) + log(8, 2) * 1px + exp(0) * 1px); }
		f { transform: rotate(calc(asin(0.5) + atan2(1, 1) - 60deg)); }
		g { opacity: calc(max(0.5, min(0.7, 0.9)) * cos(0deg)); }
	`)
	v := computeTree(s, el("a"))
	if v.Padding.Top != 5 || v.Margin.Top != 10 || v.MinWidth != 12 || v.MinHeight != 0 || v.BorderSpacingH != 0 {
		t.Errorf("round: %d %d %d %d %d", v.Padding.Top, v.Margin.Top, v.MinWidth, v.MinHeight, v.BorderSpacingH)
	}
	v = computeTree(s, el("b"))
	if v.Padding.Top != 4 || v.Margin.Top != 4 || v.MinWidth != 1 || v.Margin.Bottom != 4 {
		t.Errorf("mod/rem: %d %d %d %d", v.Padding.Top, v.Margin.Top, v.MinWidth, v.Margin.Bottom)
	}
	v = computeTree(s, el("c"))
	if v.Opacity != 0.25 || v.LetterSpacing != -1 {
		t.Errorf("abs/sign: %v %v", v.Opacity, v.LetterSpacing)
	}
	v = computeTree(s, el("d"))
	if v.Padding.Top != 0 || v.Margin.Top != 5 || v.MinWidth != 9 {
		t.Errorf("sin/hypot/sqrt: %d %d %d", v.Padding.Top, v.Margin.Top, v.MinWidth)
	}
	v = computeTree(s, el("e"))
	if v.MinHeight != 8 {
		t.Errorf("pow/log/exp: %d, want 8", v.MinHeight)
	}
	v = computeTree(s, el("f"))
	ang := math.Atan2(v.Transform.M.B, v.Transform.M.A) * 180 / math.Pi
	if math.Abs(ang-15) > 1e-6 {
		t.Errorf("asin/atan2: %v, want 15deg", ang)
	}
	v = computeTree(s, el("g"))
	if v.Opacity != 0.7 {
		t.Errorf("nested min/max/cos: %v, want 0.7", v.Opacity)
	}
}

func TestMathFunctionTypeErrors(t *testing.T) {
	warns := capture(t)
	Parse(`
		a { padding: round(up, 5px, 1); }
		b { padding: mod(1px, 0); }
		c { opacity: sin(1px); }
		d { min-height: pow(2px, 3); }
	`)
	if len(*warns) != 4 {
		t.Errorf("type errors: %d warnings, want 4: %v", len(*warns), *warns)
	}
}

func TestEnvFunction(t *testing.T) {
	s := parseOne(t, `
		a { padding: env(safe-area-inset-top); margin: calc(4px + env(safe-area-inset-left)); min-height: env(keyboard-inset-height); }
	`)
	v := computeTree(s, el("a"))
	if v.Padding.Top != 0 || v.Margin.Left != 4 || v.MinHeight != 0 {
		t.Errorf("env insets zero on desktop: %+v %+v %d", v.Padding, v.Margin, v.MinHeight)
	}
	warns := capture(t)
	Parse(`a { padding: env(nonsense); }`)
	if len(*warns) != 1 {
		t.Errorf("unknown env: %d warnings, want 1", len(*warns))
	}
}

func TestAttrFunction(t *testing.T) {
	s := parseOne(t, `
		a { min-width: attr(data-w px); color: attr(data-c color); padding: attr(data-g); }
	`)
	n := &testAttrNode{attrs: map[string]string{
		"data-w": "7",
		"data-c": "#ff0000",
		"data-g": "3px",
	}}
	n.target = Target{Element: "a"}
	v := computeEnv(s, n, Env{Rem: 16})
	if v.MinWidth != 7 || v.Color != 0xffff0000 || v.Padding.Top != 3 {
		t.Errorf("typed attr(): %d %08x %+v", v.MinWidth, uint32(v.Color), v.Padding)
	}
	plain := el("a")
	if v := computeTree(s, plain); v.Has(PropColor) || v.Padding.Top != 0 || v.MinWidth != 0 {
		t.Errorf("missing attributes act as unset: color set %v padding %d min-width %d", v.Has(PropColor), v.Padding.Top, v.MinWidth)
	}
}

func TestVarInShorthands(t *testing.T) {
	s := parseOne(t, `
		:root { --p: 4px 8px; --c: #ff0000; --b: 2px solid; }
		a { padding: var(--p); border: var(--b) var(--c); }
		b { margin: var(--missing, 1px 2px); }
	`)
	v := computeTree(s, el("a"))
	if v.Padding != (Sides{4, 8, 4, 8}) || v.BorderWidth.Top != 2 || v.BorderStyle[0] != BorderSolid || v.BorderColor[0] != 0xffff0000 {
		t.Errorf("var() through shorthands: %+v %+v %+v %08x", v.Padding, v.BorderWidth, v.BorderStyle, uint32(v.BorderColor[0]))
	}
	if v := computeTree(s, el("b")); v.Margin != (Sides{1, 2, 1, 2}) {
		t.Errorf("var() fallback through a shorthand: %+v", v.Margin)
	}
}
