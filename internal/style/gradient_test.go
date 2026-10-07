package style

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// TestGradientForms pins the radial, conic and repeating parses: shape,
// size keyword, explicit radii, positions, from-angles, angle stops.
func TestGradientForms(t *testing.T) {
	s := parseOne(t, `
		a { background-image: radial-gradient(#000000, #ffffff); }
		b { background-image: radial-gradient(circle closest-side at left 25%, #000000, #ffffff); }
		c { background-image: radial-gradient(20px 10px at center, #000000, #ffffff); }
		d { background-image: repeating-radial-gradient(circle 8px, #000000, #ffffff 50%); }
		e { background-image: conic-gradient(from 90deg at 30% 70%, #ff0000, #0000ff 90deg, #00ff00); }
		f { background-image: repeating-linear-gradient(45deg, #000000, #ffffff 10%); }
		h { background-image: conic-gradient(#ff0000 0%, #0000ff 0.5turn); }
	`)
	img := func(name string) Gradient { return computeTree(s, el(name)).Image }
	if g := img("a"); g.Kind != render.GradientRadial || g.Circle || g.Size != render.FarthestCorner || g.CenterX != 0.5 || g.N != 2 {
		t.Errorf("default radial: %+v", g)
	}
	if g := img("b"); !g.Circle || g.Size != render.ClosestSide || g.CenterX != 0 || g.CenterY != 0.25 {
		t.Errorf("circle closest-side at left 25%%: %+v", g)
	}
	if g := img("c"); g.Size != render.RadiusExplicit || g.RadiusX != 20 || g.RadiusY != 10 || g.Circle {
		t.Errorf("explicit ellipse: %+v", g)
	}
	if g := img("d"); !g.Repeat || !g.Circle || g.RadiusX != 8 || g.Stops[1].Pos != 0.5 {
		t.Errorf("repeating circle: %+v", g)
	}
	if g := img("e"); g.Kind != render.GradientConic || g.Angle != 90 || g.CenterX != 0.3 || g.CenterY != 0.7 || g.Stops[1].Pos != 0.25 {
		t.Errorf("conic: %+v", g)
	}
	if g := img("f"); g.Kind != render.GradientLinear || !g.Repeat || g.Angle != 45 {
		t.Errorf("repeating linear: %+v", g)
	}
	if g := img("h"); g.N != 2 || g.Stops[1].Pos != 0.5 {
		t.Errorf("conic turn stop: %+v", g)
	}
}

// An unknown radial keyword skips the declaration (and the same path
// accepts a valid form, so the rejections are the keywords').
func TestGradientRejects(t *testing.T) {
	if _, ok := gradientOf(tokenize("radial-gradient(circle 1px, #000000, #ffffff)"), &ctx{}); !ok {
		t.Fatal("the control form did not parse")
	}
	for _, bad := range []string{"radial-gradient(bogus, #000000, #ffffff)", "conic-gradient(from 1px, #000000, #ffffff)", "radial-gradient(circle 1px 2px, #000000, #ffffff)"} {
		var g Gradient
		var ok bool
		toks := tokenize(bad)
		g, ok = gradientOf(toks, &ctx{})
		if ok {
			t.Errorf("%s parsed: %+v", bad, g)
		}
	}
}
