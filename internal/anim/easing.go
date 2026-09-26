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
