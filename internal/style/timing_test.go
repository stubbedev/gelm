package style

import "testing"

// -lys-spring(<bounce>) is a duration-filling spring: no bounce lands
// without overshoot, a bounce overshoots, both land exactly at 1;
// -lys-bounce is the preset (#203).
func TestSpringTiming(t *testing.T) {
	s := parseOne(t, `a { transition-timing-function: -lys-spring(); } b { transition-timing-function: -lys-spring(0.4); } c { transition-timing-function: -lys-bounce; }`)
	peak := func(name string) float64 {
		tm := computeTree(s, el(name)).Transition.Timings[0]
		if tm.Zeta <= 0 || tm.At(1, false) != 1 || tm.At(0, false) != 0 {
			t.Fatalf("%s: %+v is not a landing spring", name, tm)
		}
		p := 0.0
		for i := range 100 {
			p = max(p, tm.At(float64(i)/100, false))
		}
		return p
	}
	if p := peak("a"); p > 1.0001 {
		t.Errorf("-lys-spring() overshot to %v", p)
	}
	if p := peak("b"); p <= 1.01 {
		t.Errorf("-lys-spring(0.4) peaked at %v, want an overshoot", p)
	}
	if p := peak("c"); p <= 1.01 {
		t.Errorf("-lys-bounce peaked at %v, want an overshoot", p)
	}
	for _, bad := range []string{`-lys-spring(1)`, `-lys-spring(-0.1)`, `-lys-spring(0.2, 3)`, `-lys-spring(fast)`} {
		s := parseWarns(t, `a { transition-timing-function: `+bad+`; }`)
		if v := computeTree(s, el("a")); v.Has(PropTransitionTiming) {
			t.Errorf("%s parsed", bad)
		}
	}
}

// @-lys-define names any value sheet-wide, read through var() like a
// @define-color, declared on no element.
func TestLysDefine(t *testing.T) {
	s := parseOne(t, `@-lys-define lys-duration-short 120ms; a { transition-duration: var(--lys-duration-short); }`)
	if v := computeTree(s, el("a")); len(v.Transition.Durations) != 1 || v.Transition.Durations[0] != 0.12 {
		t.Errorf("durations %v, want the defined 120ms", v.Transition.Durations)
	}
}
