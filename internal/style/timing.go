package style

import (
	"math"
	"strings"

	"github.com/stubbedev/gelm/anim"
)

// Timing is a transition or animation timing function (CSS Easing 2):
// the named curves and cubic-bezier control points, steps() with its
// jump position, or the linear() piecewise list. Exactly one form is
// live: LinN > 0 is linear(), else Steps > 0 is steps(), else the
// bezier.
type Timing struct {
	X1, Y1, X2, Y2 float64
	// Steps is non-zero for steps(n); the bezier is then unused.
	Steps int
	// StepPos is steps' jump position.
	StepPos anim.StepPos
	// LinN is the number of live linear() points in Lin, which hold
	// canonical positions (anim.CanonicalLinear).
	LinN int
	// Lin is the linear() point list, capped at MaxLinearPts.
	Lin [MaxLinearPts]anim.LinearPt
}

// MaxLinearPts bounds a linear() point list; points past it are
// dropped with a warning, like the gradient stop budget.
const MaxLinearPts = 8

// The named timing functions (CSS Easing 2 §2.2, §2.3).
var (
	TimingLinear    = Timing{X1: 0, Y1: 0, X2: 1, Y2: 1}
	TimingEase      = Timing{X1: 0.25, Y1: 0.1, X2: 0.25, Y2: 1}
	TimingEaseIn    = Timing{X1: 0.42, Y1: 0, X2: 1, Y2: 1}
	TimingEaseOut   = Timing{X1: 0, Y1: 0, X2: 0.58, Y2: 1}
	TimingEaseInOut = Timing{X1: 0.42, Y1: 0, X2: 0.58, Y2: 1}
)

// timingKeywords are the timing-function keywords.
var timingKeywords = map[string]Timing{
	"linear":      TimingLinear,
	"ease":        TimingEase,
	"ease-in":     TimingEaseIn,
	"ease-out":    TimingEaseOut,
	"ease-in-out": TimingEaseInOut,
	"step-start":  {Steps: 1, StepPos: anim.JumpStart},
	"step-end":    {Steps: 1, StepPos: anim.JumpEnd},
}

// At evaluates the timing function at input progress t; before is the
// before flag the step functions read (CSS Easing 2 §2.3.1).
func (tm *Timing) At(t float64, before bool) float64 {
	switch {
	case tm.LinN > 0:
		return anim.PiecewiseAt(tm.Lin[:tm.LinN], t)
	case tm.Steps > 0:
		return anim.StepsAt(tm.Steps, tm.StepPos, t, before)
	}
	return anim.CubicBezierAt(tm.X1, tm.Y1, tm.X2, tm.Y2, t)
}

// Easing is the timing function as an anim curve.
func (tm Timing) Easing() anim.Easing {
	return func(t float64) float64 { return tm.At(t, false) }
}

// timingOf parses one <easing-function>: a keyword, cubic-bezier(),
// steps() or linear(), each with CSS Easing 2's validity rules.
func timingOf(ts []token, cx *ctx) (Timing, bool) {
	if len(ts) == 0 {
		return Timing{}, false
	}
	if ts[0].kind == tkIdent {
		tm, ok := timingKeywords[strings.ToLower(ts[0].s)]
		return tm, ok && len(ts) == 1
	}
	if ts[0].kind != tkFunc || blockEnd(ts, 0) != len(ts) {
		return Timing{}, false
	}
	args := splitTop(funcArgs(ts), tkComma)
	switch strings.ToLower(ts[0].s) {
	case "cubic-bezier":
		return parseCubicBezier(args, cx)
	case "steps":
		return parseSteps(args, cx)
	case "linear":
		return parseLinearPoints(args, cx)
	}
	return Timing{}, false
}

// parseCubicBezier reads cubic-bezier(x1, y1, x2, y2): four numbers,
// the x controls within [0, 1] so the curve stays a function of time.
func parseCubicBezier(args [][]token, cx *ctx) (Timing, bool) {
	if len(args) != 4 {
		return Timing{}, false
	}
	var f [4]float64
	for i, a := range args {
		n, ok := evalNumeric(trimWS(a), cx)
		if !ok || n.kind != numNumber {
			return Timing{}, false
		}
		f[i] = n.v
	}
	if f[0] < 0 || f[0] > 1 || f[2] < 0 || f[2] > 1 {
		return Timing{}, false
	}
	return Timing{X1: f[0], Y1: f[1], X2: f[2], Y2: f[3]}, true
}

// parseSteps reads steps(<integer>, <step-position>?): a positive
// integer count, at least 2 under jump-none.
func parseSteps(args [][]token, cx *ctx) (Timing, bool) {
	if len(args) < 1 || len(args) > 2 {
		return Timing{}, false
	}
	n, ok := evalNumeric(trimWS(args[0]), cx)
	if !ok || n.kind != numNumber || n.v != math.Trunc(n.v) || n.v < 1 {
		return Timing{}, false
	}
	t := Timing{Steps: int(n.v)}
	if len(args) == 2 {
		comps := components(args[1])
		if len(comps) != 1 || comps[0][0].kind != tkIdent {
			return Timing{}, false
		}
		jump, ok := timingJump(strings.ToLower(comps[0][0].s))
		if !ok {
			return Timing{}, false
		}
		t.StepPos = jump
	}
	if t.StepPos == anim.JumpNone && t.Steps < 2 {
		return Timing{}, false
	}
	return t, true
}

// timingJump reads the steps() jump-position keyword.
func timingJump(s string) (anim.StepPos, bool) {
	switch s {
	case "jump-start", "start":
		return anim.JumpStart, true
	case "jump-end", "end":
		return anim.JumpEnd, true
	case "jump-none":
		return anim.JumpNone, true
	case "jump-both":
		return anim.JumpBoth, true
	}
	return 0, false
}

// parseLinearPoints parses a linear() argument list (CSS Easing 2
// §2.1): at least two points, each a number with up to two input
// percentages in either order — two percentages stand for two points
// of the same output — capped at MaxLinearPts, then made canonical.
func parseLinearPoints(args [][]token, cx *ctx) (Timing, bool) {
	var t Timing
	add := func(pt anim.LinearPt) {
		if t.LinN >= MaxLinearPts {
			warnf("linear() point skipped: past the %d-point budget", MaxLinearPts)
			return
		}
		t.Lin[t.LinN] = pt
		t.LinN++
	}
	for _, a := range args {
		comps := components(a)
		if len(comps) < 1 || len(comps) > 3 {
			return Timing{}, false
		}
		var value float64
		haveValue := false
		var pos []float64
		for _, c := range comps {
			n, ok := evalNumeric(c, cx)
			switch {
			case !ok:
				return Timing{}, false
			case n.kind == numNumber && !haveValue:
				value, haveValue = n.v, true
			case n.kind == numPercent && len(pos) < 2:
				pos = append(pos, n.v/100)
			default:
				return Timing{}, false
			}
		}
		if !haveValue {
			return Timing{}, false
		}
		if len(pos) == 0 {
			add(anim.LinearPt{Value: value})
			continue
		}
		for _, p := range pos {
			add(anim.LinearPt{Value: value, Pos: p, HasPos: true})
		}
	}
	if t.LinN < 2 {
		return Timing{}, false
	}
	anim.CanonicalLinear(t.Lin[:t.LinN])
	return t, true
}
