package style

import (
	"testing"
)

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
	if kf.Frames[1].Rotation == nil || *kf.Frames[1].Rotation != 90 {
		t.Errorf("the 50%% stop rotation %v, want 90deg", kf.Frames[1].Rotation)
	}
	if kf.Frames[3].Rotation == nil || *kf.Frames[3].Rotation != 180 {
		t.Errorf("the to stop rotation %v, want 0.5turn = 180", kf.Frames[3].Rotation)
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
	a := v.Animation
	if a.Name != "spin" || a.Keyframes == nil || len(a.Keyframes.Frames) != 2 {
		t.Fatalf("animation %+v, want spin resolved to its keyframes", a)
	}
	if a.Duration != 1 || !a.Infinite || !a.Alternate || a.Timing.Steps != 20 {
		t.Errorf("shorthand fields %+v, want 1s steps(20) infinite alternate", a)
	}
	if !a.Active() {
		t.Error("the animation is not active")
	}

	v = run(`@keyframes spin { to { opacity: 0.5; } }
		box { animation: spin 2s linear infinite; animation-play-state: paused; }`)
	if !v.Animation.Running == false || v.Animation.Duration != 2 {
		t.Errorf("play-state did not merge: %+v", v.Animation)
	}
	if v.Animation.Running {
		t.Error("paused lost to the shorthand's default")
	}

	v = run(`box { animation: spin 1s infinite; animation: none; }`)
	if v.Animation.Name != "" || v.Animation.Keyframes != nil {
		t.Errorf("animation: none left %+v", v.Animation)
	}

	v = run(`box { animation: nosuch 1s infinite; }`)
	if v.Animation.Keyframes != nil || v.Animation.Active() {
		t.Errorf("an unknown keyframes name resolved: %+v", v.Animation)
	}
}

// Keyframes interpolate between the stops that declare a channel and
// hold the nearest declared value outside them.
func TestKeyframesInterpolate(t *testing.T) {
	kf := &Keyframes{Name: "k", Frames: []Keyframe{
		{Offset: 0, Opacity: new(1.0)},
		{Offset: 0.5, Opacity: new(0.4), Rotation: new(0.0)},
		{Offset: 1, Rotation: new(360.0)},
	}}
	at := func(p float64) AnimValues { return kf.At(p) }
	if v := at(0); v.Opacity != 1 {
		t.Errorf("at 0: %v, want the from stop", v)
	}
	if v := at(0.25); v.Opacity != 0.7 {
		t.Errorf("at 0.25: %v, want halfway 1 to 0.4", v)
	}
	if v := at(0.75); v.Rotation != 180 || v.Opacity != 0.4 {
		t.Errorf("at 0.75: %v, want rotation halfway and opacity held", v)
	}
	if v := at(1.5); v.Rotation != 360 {
		t.Errorf("past the end: %v, want the to stop", v)
	}
}
