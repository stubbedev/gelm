package style

import (
	"slices"
	"strings"
)

// Transition is the computed transition-* group as its longhand lists
// (css-transitions-1 §2): the property list, and the durations, delays,
// timing functions and behaviors matched to it by index, a shorter list
// repeating. A nil list is the longhand's initial value: all, 0s, 0s,
// ease, normal.
type Transition struct {
	Props     []TransitionTarget
	Durations []float64
	Delays    []float64
	Timings   []Timing
	// Discrete is transition-behavior per entry: allow-discrete.
	Discrete []bool
}

// TransitionTarget is one transition-property entry: all, the
// longhands one property name covers, or nothing (none, or a name the
// engine does not know, which CSS keeps in the list).
type TransitionTarget struct {
	All   bool
	Props PropSet
}

// TransitionTiming is how one longhand's change transitions.
type TransitionTiming struct {
	Duration, Delay float64
	Timing          Timing
	Discrete        bool
}

// For is the timing a change of p transitions with: the last entry of
// the property list naming p (or all), with a positive combined
// duration (css-transitions-1 §3). ok is false when p does not
// transition.
func (t *Transition) For(p Prop) (TransitionTiming, bool) {
	props := t.Props
	if props == nil {
		props = []TransitionTarget{{All: true}}
	}
	for i, tg := range slices.Backward(props) {
		if !tg.All && !tg.Props.Has(p) {
			continue
		}
		tt := TransitionTiming{
			Duration: max(0, listAt(t.Durations, i, 0)),
			Delay:    listAt(t.Delays, i, 0),
			Timing:   listAt(t.Timings, i, TimingEase),
			Discrete: listAt(t.Discrete, i, false),
		}
		return tt, tt.Duration+tt.Delay > 0
	}
	return TransitionTiming{}, false
}

// Active reports whether any entry can run: a positive duration
// somewhere in the list.
func (t *Transition) Active() bool {
	return slices.ContainsFunc(t.Durations, func(d float64) bool { return d > 0 }) ||
		slices.ContainsFunc(t.Delays, func(d float64) bool { return d > 0 })
}

// Equal reports whether two transition groups hold the same lists.
func (t *Transition) Equal(o *Transition) bool {
	return slices.Equal(t.Props, o.Props) && slices.Equal(t.Durations, o.Durations) &&
		slices.Equal(t.Delays, o.Delays) && slices.Equal(t.Timings, o.Timings) &&
		slices.Equal(t.Discrete, o.Discrete)
}

// listAt is entry i of a longhand list matched by index against
// another, repeating (css-transitions-1 §2, css-animations-1 §3); an
// empty list reads def, the initial value.
func listAt[T any](l []T, i int, def T) T {
	if len(l) == 0 {
		return def
	}
	return l[i%len(l)]
}

// timeListInto parses a comma list of times into the slot get names;
// negative values only where neg allows (delays).
func timeListInto(get func(*Values) *[]float64, neg bool) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		var out []float64
		for _, item := range splitTop(ts, tkComma) {
			t, ok := timeOf(trimWS(item), cx)
			if !ok || t < 0 && !neg {
				return false
			}
			out = append(out, t)
		}
		*get(v) = out
		return true
	}
}

// timingListInto parses a comma list of timing functions.
func timingListInto(get func(*Values) *[]Timing) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		var out []Timing
		for _, item := range splitTop(ts, tkComma) {
			tm, ok := timingOf(trimWS(item), cx)
			if !ok {
				return false
			}
			out = append(out, tm)
		}
		*get(v) = out
		return true
	}
}

// keywordListInto parses a comma list of one-keyword entries through
// names.
func keywordListInto(names map[string]uint8, get func(*Values) *[]uint8) func([]token, *ctx, *Values) bool {
	return func(ts []token, _ *ctx, v *Values) bool {
		var out []uint8
		for _, item := range splitTop(ts, tkComma) {
			s, ok := identOf(components(item))
			if !ok {
				return false
			}
			k, ok := names[s]
			if !ok {
				return false
			}
			out = append(out, k)
		}
		*get(v) = out
		return true
	}
}

// parseTransitionBehavior parses transition-behavior: a list of normal
// and allow-discrete.
func parseTransitionBehavior(ts []token, _ *ctx, v *Values) bool {
	var out []bool
	for _, item := range splitTop(ts, tkComma) {
		s, ok := identOf(components(item))
		switch {
		case !ok:
			return false
		case s == "normal":
			out = append(out, false)
		case s == "allow-discrete":
			out = append(out, true)
		default:
			return false
		}
	}
	v.Transition.Discrete = out
	return true
}

// transitionTargetOf reads one transition-property name: all, a known
// property, or another custom ident, kept as a target naming nothing.
func transitionTargetOf(name string) (TransitionTarget, bool) {
	switch name = strings.ToLower(name); name {
	case "all":
		return TransitionTarget{All: true}, true
	case "none", "initial", "inherit", "unset", "default":
		return TransitionTarget{}, false
	}
	if def, ok := propTable[name]; ok {
		return TransitionTarget{Props: def.covers}, true
	}
	return TransitionTarget{}, true
}

// parseTransitionProperty parses transition-property: none, or a list
// of property names and all.
func parseTransitionProperty(ts []token, _ *ctx, v *Values) bool {
	items := splitTop(ts, tkComma)
	if s, ok := identOf(components(items[0])); ok && s == "none" && len(items) == 1 {
		v.Transition.Props = []TransitionTarget{}
		return true
	}
	var out []TransitionTarget
	for _, item := range items {
		comps := components(item)
		if len(comps) != 1 || len(comps[0]) != 1 || comps[0][0].kind != tkIdent {
			return false
		}
		tg, ok := transitionTargetOf(comps[0][0].s)
		if !ok {
			return false
		}
		out = append(out, tg)
	}
	v.Transition.Props = out
	return true
}

// parseTransition parses the shorthand (css-transitions-1 §2.5): a
// comma list of `[property] [duration] [timing] [delay] [behavior]` in
// any order, the first time the duration and the second the delay;
// `none` names no property and is valid only in a one-item list. Every
// longhand list takes one entry per item.
func parseTransition(ts []token, cx *ctx, v *Values) bool {
	items := splitTop(ts, tkComma)
	t := Transition{
		Props:     make([]TransitionTarget, 0, len(items)),
		Durations: make([]float64, 0, len(items)),
		Delays:    make([]float64, 0, len(items)),
		Timings:   make([]Timing, 0, len(items)),
		Discrete:  make([]bool, 0, len(items)),
	}
	for _, item := range items {
		target, haveP := TransitionTarget{All: true}, false
		var times []float64
		timing, haveT := TimingEase, false
		discrete, haveB := false, false
		for _, c := range components(item) {
			if secs, ok := timeOf(c, cx); ok && len(times) < 2 {
				times = append(times, secs)
				continue
			}
			if tm, ok := timingOf(c, cx); ok && !haveT {
				timing, haveT = tm, true
				continue
			}
			if s, ok := identOf([][]token{c}); ok && !haveB && (s == "normal" || s == "allow-discrete") {
				discrete, haveB = s == "allow-discrete", true
				continue
			}
			if haveP || len(c) != 1 || c[0].kind != tkIdent {
				return false
			}
			if c[0].ident("none") {
				if len(items) != 1 {
					return false
				}
				target, haveP = TransitionTarget{}, true
				continue
			}
			tg, ok := transitionTargetOf(c[0].s)
			if !ok {
				return false
			}
			target, haveP = tg, true
		}
		if len(times) > 0 && times[0] < 0 {
			return false
		}
		t.Props = append(t.Props, target)
		t.Durations = append(t.Durations, listAt(times, 0, 0))
		delay := 0.0
		if len(times) == 2 {
			delay = times[1]
		}
		t.Delays = append(t.Delays, delay)
		t.Timings = append(t.Timings, timing)
		t.Discrete = append(t.Discrete, discrete)
	}
	v.Transition = t
	return true
}
