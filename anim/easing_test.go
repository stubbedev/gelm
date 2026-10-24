package anim

import (
	"math"
	"testing"
)

func TestEasingEndpoints(t *testing.T) {
	curves := map[string]Easing{
		"Linear":         Linear,
		"EaseInQuad":     EaseInQuad,
		"EaseOutQuad":    EaseOutQuad,
		"EaseInOutQuad":  EaseInOutQuad,
		"EaseInCubic":    EaseInCubic,
		"EaseOutCubic":   EaseOutCubic,
		"EaseInOutCubic": EaseInOutCubic,
		"EaseOutBack":    EaseOutBack,
		"SpringEasing":   SpringEasing(0.4, 8),
	}
	for name, e := range curves {
		if got := e(0); got != 0 {
			t.Errorf("%s(0) = %v, want 0", name, got)
		}
		if got := e(1); got != 1 {
			t.Errorf("%s(1) = %v, want 1", name, got)
		}
	}
}

func TestEasingSpotValues(t *testing.T) {
	t.Run("cubic family", func(t *testing.T) {
		if got := EaseOutCubic(0.5); got != 0.875 {
			t.Errorf("EaseOutCubic(0.5) = %v, want 0.875", got)
		}
		if got := EaseInCubic(0.5); got != 0.125 {
			t.Errorf("EaseInCubic(0.5) = %v, want 0.125", got)
		}
		if got := EaseInOutCubic(0.25); got != 0.0625 {
			t.Errorf("EaseInOutCubic(0.25) = %v, want 0.0625", got)
		}
	})
	t.Run("quad family", func(t *testing.T) {
		if got := EaseOutQuad(0.5); got != 0.75 {
			t.Errorf("EaseOutQuad(0.5) = %v, want 0.75", got)
		}
		if got := EaseInQuad(0.5); got != 0.25 {
			t.Errorf("EaseInQuad(0.5) = %v, want 0.25", got)
		}
		if got := EaseInOutQuad(0.5); got != 0.5 {
			t.Errorf("EaseInOutQuad(0.5) = %v, want 0.5", got)
		}
	})
	t.Run("in and out meet in the middle", func(t *testing.T) {
		for name, e := range map[string]Easing{
			"Linear":         Linear,
			"EaseInOutQuad":  EaseInOutQuad,
			"EaseInOutCubic": EaseInOutCubic,
		} {
			if got := e(0.5); got != 0.5 {
				t.Errorf("%s(0.5) = %v, want 0.5", name, got)
			}
		}
	})
}

// sample walks a curve across its domain.
func sample(e Easing, n int) []float64 {
	out := make([]float64, 0, n+1)
	for i := range n + 1 {
		out = append(out, e(float64(i)/float64(n)))
	}
	return out
}

func TestEasingShapes(t *testing.T) {
	t.Run("plain curves never overshoot", func(t *testing.T) {
		for name, e := range map[string]Easing{
			"Linear":         Linear,
			"EaseInQuad":     EaseInQuad,
			"EaseOutQuad":    EaseOutQuad,
			"EaseInOutQuad":  EaseInOutQuad,
			"EaseInCubic":    EaseInCubic,
			"EaseOutCubic":   EaseOutCubic,
			"EaseInOutCubic": EaseInOutCubic,
		} {
			for i, v := range sample(e, 100) {
				if v < 0 || v > 1 {
					t.Fatalf("%s left [0,1] at %d/100: %v", name, i, v)
				}
			}
		}
	})
	t.Run("EaseOutBack overshoots then lands", func(t *testing.T) {
		peak := 0.0
		for _, v := range sample(EaseOutBack, 200) {
			peak = max(peak, v)
		}
		if peak <= 1 {
			t.Errorf("EaseOutBack peak = %v, want an overshoot past 1", peak)
		}
	})
	t.Run("critically damped spring never overshoots", func(t *testing.T) {
		prev := 0.0
		for i, v := range sample(SpringEasing(1, 2*math.Pi), 100) {
			if v < prev {
				t.Fatalf("Spring(zeta 1) dipped at %d/100: %v after %v", i, v, prev)
			}
			prev = v
		}
	})
	t.Run("underdamped spring wobbles and settles", func(t *testing.T) {
		e := SpringEasing(0.2, 8)
		peak := 0.0
		for _, v := range sample(e, 200) {
			peak = max(peak, v)
		}
		if peak <= 1.05 {
			t.Errorf("underdamped spring peak = %v, want a wobble past 1.05", peak)
		}
	})
	t.Run("overdamped spring crawls without crossing", func(t *testing.T) {
		e := SpringEasing(2, 2*math.Pi)
		prev := 0.0
		for i, v := range sample(e, 100) {
			if v < prev || v > 1 {
				t.Fatalf("Spring(zeta 2) misbehaved at %d/100: %v after %v", i, v, prev)
			}
			prev = v
		}
	})
}

// The steps() jump positions against the spec's worked curves.
func TestStepsTiming(t *testing.T) {
	end := Steps(2, JumpEnd)
	if got := []float64{end(0), end(0.25), end(0.5), end(0.99), end(1)}; got[0] != 0 || got[1] != 0 || got[2] != 0.5 || got[3] != 0.5 || got[4] != 1 {
		t.Errorf("steps(2, jump-end) = %v, want 0,0,0,0.5,1", got)
	}
	// jump-start jumps at the start of each interval, so progress 0
	// already reads the first step (CSS Easing 2 §2.3).
	start := Steps(2, JumpStart)
	if got := []float64{start(0), start(0.25), start(0.5), start(0.75), start(1)}; got[0] != 0.5 || got[1] != 0.5 || got[2] != 1 || got[3] != 1 || got[4] != 1 {
		t.Errorf("steps(2, jump-start) = %v, want 0.5,0.5,1,1,1", got)
	}
	// jump-none over 2 steps makes one jump, from 0 to 1 at the
	// midpoint.
	none := Steps(2, JumpNone)
	if got := []float64{none(0), none(0.24), none(0.25), none(0.5), none(0.74), none(1)}; got[0] != 0 || got[1] != 0 || got[2] != 0 || got[3] != 1 || got[4] != 1 || got[5] != 1 {
		t.Errorf("steps(2, jump-none) = %v, want 0,0,0,1,1,1", got)
	}
	both := Steps(2, JumpBoth)
	if got := []float64{both(0), both(0.25), both(0.5), both(0.75), both(1)}; got[0] != 1.0/3 || got[1] != 1.0/3 || got[2] != 2.0/3 || got[3] != 2.0/3 || got[4] != 1 {
		t.Errorf("steps(2, jump-both) = %v, want 1/3,1/3,2/3,2/3,1", got)
	}
	// steps(1, jump-none) is the two-point round; steps(1, jump-end)
	// holds then lands.
	if lin := Steps(1, JumpNone); lin(0.25) != 0 || lin(0.5) != 1 || lin(0.75) != 1 {
		t.Errorf("steps(1, jump-none) = %v %v %v, want 0 0.5 1", lin(0.25), lin(0.5), lin(0.75))
	}
	if end1 := Steps(1, JumpEnd); end1(0.99) != 0 || end1(1) != 1 {
		t.Errorf("steps(1, jump-end) at 0.99,1 = %v,%v, want 0,1", end1(0.99), end1(1))
	}
}

// linear() interpolates between its points, spreading the ones
// without positions and extrapolating outside the bracket.
func TestPiecewiseEasing(t *testing.T) {
	ramp := Piecewise([]LinearPt{{Value: 0}, {Value: 0.5, Pos: 0.5, HasPos: true}, {Value: 1}})
	if got := ramp(0.25); got != 0.25 {
		t.Errorf("ramp(0.25) = %v, want 0.25", got)
	}
	if got := ramp(0.75); got != 0.75 {
		t.Errorf("ramp(0.75) = %v, want 0.75", got)
	}
	// Outside the points the outer segments extrapolate (CSS Easing 2
	// §2.1.2).
	if got := ramp(-1); got != -1 {
		t.Errorf("ramp(-1) = %v, want the extrapolated -1", got)
	}
	if got := ramp(2); got != 2 {
		t.Errorf("ramp(2) = %v, want the extrapolated 2", got)
	}

	// Points without positions spread evenly: 0, 1, 0 land at 0, 0.5, 1.
	bounce := Piecewise([]LinearPt{{Value: 0}, {Value: 1}, {Value: 0}})
	if got := bounce(0.25); got != 0.5 {
		t.Errorf("bounce(0.25) = %v, want 0.5", got)
	}
	if got := bounce(0.5); got != 1 {
		t.Errorf("bounce(0.5) = %v, want 1", got)
	}
	if got := bounce(0.75); got != 0.5 {
		t.Errorf("bounce(0.75) = %v, want 0.5", got)
	}

	// An interior explicit stop brackets the spread around it: the
	// free point lands at 0.375, halfway from 0 to the 75% stop.
	mixed := Piecewise([]LinearPt{{Value: 0}, {Value: 1}, {Value: 0.5, Pos: 0.75, HasPos: true}, {Value: 1}})
	if got := mixed(0.25); math.Abs(got-2.0/3) > 1e-9 {
		t.Errorf("mixed(0.25) = %v, want 2/3 up to the free point at 0.375", got)
	}
	if got := mixed(0.5); math.Abs(got-5.0/6) > 1e-9 {
		t.Errorf("mixed(0.5) = %v, want 5/6 down to the 75%% stop", got)
	}
}
