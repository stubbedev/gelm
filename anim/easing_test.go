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
		"Spring":         Spring(0.4, 8),
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
		for i, v := range sample(Spring(1, 2*math.Pi), 100) {
			if v < prev {
				t.Fatalf("Spring(zeta 1) dipped at %d/100: %v after %v", i, v, prev)
			}
			prev = v
		}
	})
	t.Run("underdamped spring wobbles and settles", func(t *testing.T) {
		e := Spring(0.2, 8)
		peak := 0.0
		for _, v := range sample(e, 200) {
			peak = max(peak, v)
		}
		if peak <= 1.05 {
			t.Errorf("underdamped spring peak = %v, want a wobble past 1.05", peak)
		}
	})
	t.Run("overdamped spring crawls without crossing", func(t *testing.T) {
		e := Spring(2, 2*math.Pi)
		prev := 0.0
		for i, v := range sample(e, 100) {
			if v < prev || v > 1 {
				t.Fatalf("Spring(zeta 2) misbehaved at %d/100: %v after %v", i, v, prev)
			}
			prev = v
		}
	})
}
