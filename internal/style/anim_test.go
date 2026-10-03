package style

import (
	"math"
	"testing"

	"github.com/stubbedev/gelm/render"
)

// affineNear compares two transforms' matrices entrywise within tol.
func affineNear(a, b Xform, tol float64) bool {
	return affineM(a.M, b.M, tol)
}

func affineM(a, b render.Affine, tol float64) bool {
	for _, pair := range [][2]float64{{a.A, b.A}, {a.B, b.B}, {a.C, b.C}, {a.D, b.D}, {a.E, b.E}, {a.F, b.F}} {
		if d := pair[0] - pair[1]; d > tol || d < -tol {
			return false
		}
	}
	return true
}

// first returns the shorthand's first animation.
func first(v Values) Animation {
	if len(v.Animation) == 0 {
		return Animation{}
	}
	return v.Animation[0]
}

// A @keyframes rule compiles into the sheet: stops in offset order,
// from/to mapped, percentage groups expanded, and only the animatable
// channels kept.
func TestKeyframesParse(t *testing.T) {
	s := parseOne(t, `@keyframes pulse {
		from { opacity: 1; }
		50%, 60% { opacity: 0.5; -gtk-icon-transform: rotate(90deg); }
		to { opacity: 0.25; -gtk-icon-transform: rotate(0.5turn); }
	}`)
	kf := s.Keyframes("pulse")
	if kf == nil {
		t.Fatal("the sheet lost the pulse keyframes")
	}
	if len(kf.Frames) != 4 {
		t.Fatalf("frames %d, want 4 (the 50/60 group expands)", len(kf.Frames))
	}
	if kf.Frames[0].Offset != 0 || kf.Frames[3].Offset != 1 {
		t.Errorf("endpoints %v %v, want from 0 and to 1", kf.Frames[0].Offset, kf.Frames[3].Offset)
	}
	if kf.Frames[0].Opacity == nil || *kf.Frames[0].Opacity != 1 {
		t.Error("the from stop lost its opacity")
	}
	if kf.Frames[1].IconXform == nil || !affineNear(*kf.Frames[1].IconXform, Xform{M: render.Rotate(90)}, 1e-9) {
		t.Error("the 50% stop icon transform, want 90deg")
	}
	if kf.Frames[3].IconXform == nil || !affineNear(*kf.Frames[3].IconXform, Xform{M: render.Rotate(180)}, 1e-9) {
		t.Error("the to stop icon transform, want 0.5turn = 180")
	}
	if s.Keyframes("absent") != nil {
		t.Error("an unknown name resolved")
	}
}

// The animation shorthand parses in the orders the stylesheet writes
// and resolves its keyframes through the layers; play-state merges
// after it, and `none` clears the group.
func TestAnimationShorthand(t *testing.T) {
	var sc Scratch
	run := func(css string) Values {
		s := Parse(css)
		var root, v Values
		Compute([]Layer{{Sheet: s}}, el("box"), nil, nil, Env{}, &sc, &root)
		Compute([]Layer{{Sheet: s}}, el("box"), nil, nil, Env{}, &sc, &v)
		return v
	}
	v := run(`@keyframes spin { from { -gtk-icon-transform: rotate(0deg); } to { -gtk-icon-transform: rotate(360deg); } }
		box { animation: spin var(--d, 1s) steps(20) infinite alternate; }`)
	a := first(v)
	if a.Name != "spin" || a.Keyframes == nil || len(a.Keyframes.Frames) != 2 {
		t.Fatalf("animation %+v, want spin resolved to its keyframes", a)
	}
	if a.Duration != 1 || !a.Infinite || a.Direction != DirAlternate || a.Timing.Steps != 20 {
		t.Errorf("shorthand fields %+v, want 1s steps(20) infinite alternate", a)
	}
	if !a.Active() {
		t.Error("the animation is not active")
	}

	// A comma list runs both entries.
	v = run(`@keyframes a1 { to { opacity: 0; } } @keyframes a2 { to { opacity: 1; } }
		box { animation: a1 1s linear, a2 2s reverse; }`)
	if len(v.Animation) != 2 || v.Animation[0].Name != "a1" || v.Animation[1].Name != "a2" ||
		v.Animation[1].Direction != DirReverse {
		t.Errorf("the comma list %+v, want a1 and a2 reverse", v.Animation)
	}

	// The shorthand's second time is the delay; the longhand takes
	// negatives.
	v = run(`box { animation: a1 1s 0.5s; }`)
	if first(v).Delay != 0.5 {
		t.Errorf("shorthand delay %v, want 0.5s", first(v).Delay)
	}
	v = run(`box { animation: a1 1s; animation-delay: -0.25s; }`)
	if first(v).Delay != -0.25 {
		t.Errorf("delay longhand %v, want -0.25s", first(v).Delay)
	}
	// Fill modes and directions parse.
	v = run(`box { animation: a1 1s both; }`)
	if first(v).Fill != FillBoth {
		t.Errorf("fill %v, want both", first(v).Fill)
	}
	v = run(`box { animation: a1 1s alternate-reverse; }`)
	if first(v).Direction != DirAlternateReverse {
		t.Errorf("direction %v, want alternate-reverse", first(v).Direction)
	}

	v = run(`@keyframes spin { to { opacity: 0.5; } }
		box { animation: spin 2s linear infinite; animation-play-state: paused; }`)
	if first(v).Duration != 2 || first(v).Running {
		t.Errorf("play-state did not merge: %+v", first(v))
	}

	v = run(`box { animation: spin 1s infinite; animation: none; }`)
	if len(v.Animation) != 0 {
		t.Errorf("animation: none left %+v", v.Animation)
	}

	v = run(`box { animation: nosuch 1s infinite; }`)
	if first(v).Keyframes != nil || first(v).Active() {
		t.Errorf("an unknown keyframes name resolved: %+v", first(v))
	}
}

// Keyframes interpolate between the stops that declare a channel and
// hold the nearest declared value outside them.
func TestKeyframesInterpolate(t *testing.T) {
	kf := &Keyframes{Name: "k", Frames: []Keyframe{
		{Offset: 0, Opacity: new(1.0)},
		{Offset: 0.5, Opacity: new(0.4), IconXform: new(XformIdentity)},
		{Offset: 1, IconXform: new(Xform{M: render.Rotate(180)})},
	}}
	at := func(p float64) AnimValues { return kf.At(p) }
	if v := at(0); v.Opacity != 1 {
		t.Errorf("at 0: %v, want the from stop", v)
	}
	if v := at(0.25); v.Opacity != 0.7 {
		t.Errorf("at 0.25: %v, want halfway 1 to 0.4", v)
	}
	v := at(0.75)
	if !affineNear(v.IconXform, Xform{M: render.Rotate(90)}, 1e-6) || v.Opacity != 0.4 {
		t.Errorf("at 0.75: %v, want the turn halfway and opacity held", v)
	}
	if v := at(1.5); !affineNear(v.IconXform, Xform{M: render.Rotate(180)}, 1e-6) {
		t.Errorf("past the end: %v, want the to stop", v)
	}
	// The transform channel turns, not shrinks: halfway 0deg to 180deg
	// maps (1,0) up, not through a collapsed matrix.
	tkf := &Keyframes{Frames: []Keyframe{
		{Offset: 0, Transform: new(XformIdentity)},
		{Offset: 1, Transform: new(Xform{M: render.Rotate(180)})},
	}}
	px, py := tkf.At(0.5).Transform.M.Apply(1, 0)
	if math.Abs(px) > 1e-9 || math.Abs(py-1) > 1e-9 {
		t.Errorf("mid transform maps (1,0) to (%v,%v), want (0,1)", px, py)
	}
}
