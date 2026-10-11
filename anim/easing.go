package anim

import "math"

// Easing maps linear progress t in [0, 1] to eased progress. Most
// curves keep the result inside [0, 1]; springs may overshoot past 1
// (or dip below 0) on their way to settling, so consumers must clamp
// where a value cannot overshoot (alpha, positions in bounds).
type Easing func(t float64) float64

// Linear runs at constant speed.
func Linear(t float64) float64 { return t }

// EaseInQuad starts slow and accelerates.
func EaseInQuad(t float64) float64 { return t * t }

// EaseOutQuad starts fast and decelerates.
func EaseOutQuad(t float64) float64 { return 1 - (1-t)*(1-t) }

// EaseInOutQuad accelerates through the first half, decelerates
// through the second.
func EaseInOutQuad(t float64) float64 {
	if t < 0.5 {
		return 2 * t * t
	}
	u := -2*t + 2
	return 1 - u*u/2
}

// EaseInCubic starts slow and accelerates hard.
func EaseInCubic(t float64) float64 { return t * t * t }

// EaseOutCubic is gelm's default curve: fast start, gentle landing.
func EaseOutCubic(t float64) float64 { return 1 - (1-t)*(1-t)*(1-t) }

// EaseInOutCubic eases both ends of the tween.
func EaseInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := -2*t + 2
	return 1 - u*u*u/2
}

// EaseOutBack overshoots the target by a fixed fraction and settles
// back - the classic popover landing. The overshoot peaks around 1.10
// near t = 0.7.
func EaseOutBack(t float64) float64 {
	const c1 = 1.70158
	const c3 = c1 + 1
	if t <= 0 || t >= 1 {
		// Clamp the polynomial: its endpoints only land on 0 and 1 up
		// to float noise, and the contract is exact.
		return min(max(t, 0), 1)
	}
	u := t - 1
	return 1 + c3*u*u*u + c1*u*u
}

// CubicBezier returns the CSS cubic-bezier(x1, y1, x2, y2) timing
// function: x is time, y progress, both solved on the parametric curve
// (Newton where it converges, bisection where it does not). x1 and x2
// are clamped to [0, 1] as CSS requires; the y controls are free.
func CubicBezier(x1, y1, x2, y2 float64) Easing {
	ax, ay, bx, by, cx, cy := 1-(3*x1), 1-(3*y1), 3*x1-2*(3*x1), 3*y1-2*(3*y1), 3*x1, 3*y1
	sampleX := func(t float64) float64 { return ((ax*t+bx)*t + cx) * t }
	sampleY := func(t float64) float64 { return ((ay*t+by)*t + cy) * t }
	slopeX := func(t float64) float64 { return (3*ax*t+2*bx)*t + cx }
	solveX := func(x float64) float64 {
		t := x
		for range 8 {
			dx := sampleX(t) - x
			if dx > -1e-6 && dx < 1e-6 {
				return t
			}
			d := slopeX(t)
			if d > 1e-6 || d < -1e-6 {
				t -= dx / d
			} else {
				break
			}
		}
		lo, hi := 0.0, 1.0
		t = x
		for range 24 {
			xc := sampleX(t)
			if xc < x {
				lo = t
			} else {
				hi = t
			}
			t = (lo + hi) / 2
		}
		return t
	}
	return func(t float64) float64 {
		switch {
		case t <= 0:
			return 0
		case t >= 1:
			return 1
		}
		return sampleY(solveX(t))
	}
}

// StepPos is where a steps() curve jumps: at the start of each
// interval, the end, both edges, or neither.
type StepPos uint8

// The steps() jump positions. JumpEnd is the CSS default (and the
// legacy `end` keyword); JumpStart is the legacy `start`.
const (
	JumpEnd StepPos = iota
	JumpStart
	JumpNone
	JumpBoth
)

// Steps returns the steps(n, jump) timing function: progress held in
// n equal intervals with a jump at each interval boundary per jump.
// steps(1, JumpEnd) is the discrete flip browsers give un-animated
// toggles; JumpNone spreads n+1 points with no jump at either end,
// so steps(1, JumpNone) rounds at the midpoint.
func Steps(n int, jump StepPos) Easing {
	if n < 1 {
		n = 1
	}
	return func(t float64) float64 {
		switch jump {
		case JumpStart:
			return math.Ceil(t*float64(n)) / float64(n)
		case JumpNone:
			return math.Floor(t*float64(n)+0.5) / float64(n)
		case JumpBoth:
			return (math.Floor(t*float64(n)) + 1) / float64(n+1)
		}
		return math.Floor(t*float64(n)) / float64(n)
	}
}

// LinearPt is one point of a linear() easing: the output value and,
// when HasPos, its input position as a fraction of the timeline.
type LinearPt struct {
	Value float64
	Pos   float64
	// HasPos marks an explicit percentage stop; the rest spread
	// evenly between their bracketing explicit stops.
	HasPos bool
}

// Piecewise returns the linear(...) easing: output interpolated
// linearly between the points, held at the first and last outside
// them. Points without positions spread evenly between the nearest
// explicit stops (0 and 1 implied at the ends), the CSS rule.
func Piecewise(pts []LinearPt) Easing {
	n := len(pts)
	if n == 0 {
		return Linear
	}
	pos := make([]float64, n)
	for i := range pts {
		if pts[i].HasPos {
			pos[i] = pts[i].Pos
		}
	}
	if !pts[0].HasPos {
		pos[0] = 0
	}
	if !pts[n-1].HasPos {
		pos[n-1] = 1
	}
	// Interior points without positions: evenly spaced between the
	// previous and next explicit stops.
	for i := 0; i < n; {
		if pts[i].HasPos || i == 0 || i == n-1 {
			i++
			continue
		}
		j := i
		for j < n-1 && !pts[j].HasPos {
			j++
		}
		lo, hi := pos[i-1], pos[j]
		for k := i; k < j; k++ {
			pos[k] = lo + (hi-lo)*float64(k-i+1)/float64(j-i+1)
		}
		i = j
	}
	return func(t float64) float64 {
		switch {
		case t <= pos[0]:
			return pts[0].Value
		case t >= pos[n-1]:
			return pts[n-1].Value
		}
		i := 1
		for ; i < n && pos[i] < t; i++ {
		}
		a, b := pos[i-1], pos[i]
		if b <= a {
			return pts[i].Value
		}
		u := (t - a) / (b - a)
		return pts[i-1].Value + (pts[i].Value-pts[i-1].Value)*u
	}
}

// Spring returns a damped-spring easing: the value leaves 0 with zero
// velocity and settles at 1, oscillating on the way when underdamped.
// zeta is the damping ratio - below 1 overshoots (0.3 to 0.6 give a
// lively wobble, 1 is critically damped with none), above 1 is
// overdamped and sluggish. omega is the undamped angular frequency in
// radians across the normalized tween: 2*pi crosses one full
// oscillation over the duration, so popovers want roughly 6 to 12.
func Spring(zeta, omega float64) Easing {
	return func(t float64) float64 {
		switch {
		case t <= 0:
			return 0
		case t >= 1:
			return 1
		case zeta < 1:
			wd := omega * math.Sqrt(1-zeta*zeta)
			e := math.Exp(-zeta * omega * t)
			return 1 - e*(math.Cos(wd*t)+(zeta*omega/wd)*math.Sin(wd*t))
		case zeta == 1:
			return 1 - (1+omega*t)*math.Exp(-omega*t)
		default:
			// Overdamped: two real decay rates, no oscillation.
			s := math.Sqrt(zeta*zeta - 1)
			r1 := omega * (-zeta + s)
			r2 := omega * (-zeta - s)
			return 1 + (r2*math.Exp(r1*t)-r1*math.Exp(r2*t))/(r1-r2)
		}
	}
}
