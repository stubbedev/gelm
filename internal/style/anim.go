package style

// CSS animations: @keyframes rules, the animation shorthand, and
// animation-play-state. A keyframes rule declares named stops over the
// animation's progress; the shorthand points a node at one and gives
// the duration, curve, iteration count, and direction. The animated
// channels are the ones the stylesheet animates - opacity and the icon
// transform's rotation - and the widget layer runs the tween against
// its style cache, the same channel the transitions use.

import (
	"math"
	"slices"
	"sort"
	"strings"
)

// Keyframes is one @keyframes rule: named stops over the animation's
// progress.
type Keyframes struct {
	Name   string
	Frames []Keyframe
}

// Keyframe is one stop of a keyframes rule: its offset along the
// animation's progress and the values it animates. A nil channel is
// not animated at that stop - the property runs between the stops
// that declare it and holds the nearest declared value outside them,
// exactly like CSS's missing-keyframe rule.
type Keyframe struct {
	Offset   float64 // 0..1; `from` is 0, `to` is 1
	Opacity  *float64
	Rotation *float64 // degrees; -gtk-icon-transform's rotate()
}

// Animation is the computed animation-* group: which @keyframes rule
// runs, for how long, along which curve, how many times (Infinite, or
// Iteration counts), alternating direction or not, and running or
// paused. A nil Keyframes (an unknown name, or `animation: none`) runs
// nothing.
type Animation struct {
	Name      string
	Duration  float64 // seconds
	Timing    Timing
	Infinite  bool
	Iteration float64
	Alternate bool
	Running   bool
	Keyframes *Keyframes
}

// Active reports whether an animation should be running: named, timed,
// resolved, and not paused.
func (a Animation) Active() bool {
	return a.Keyframes != nil && a.Name != "" && a.Duration > 0 && a.Running
}

// AnimValues is one interpolated animation frame: the animated
// channels' values at a phase of the animation.
type AnimValues struct {
	Opacity  float64
	Rotation float64 // degrees
}

// AnimatesOpacity reports whether any stop animates the opacity.
func (kf *Keyframes) AnimatesOpacity() bool {
	for _, f := range kf.Frames {
		if f.Opacity != nil {
			return true
		}
	}
	return false
}

// AnimatesRotation reports whether any stop animates the icon
// transform's rotation.
func (kf *Keyframes) AnimatesRotation() bool {
	for _, f := range kf.Frames {
		if f.Rotation != nil {
			return true
		}
	}
	return false
}

// At interpolates the keyframes at phase (0..1).
func (kf *Keyframes) At(phase float64) AnimValues {
	frames := kf.Frames
	out := AnimValues{}
	if len(frames) == 0 {
		return out
	}
	if phase <= frames[0].Offset {
		return frameValues(frames[0], out)
	}
	last := frames[len(frames)-1]
	if phase >= last.Offset {
		return frameValues(last, out)
	}
	for i := 0; i+1 < len(frames); i++ {
		a, b := frames[i], frames[i+1]
		if phase < a.Offset || phase > b.Offset {
			continue
		}
		t := (phase - a.Offset) / (b.Offset - a.Offset)
		lerp(&out, a.Opacity, b.Opacity, t, lerpedOpacity)
		lerp(&out, a.Rotation, b.Rotation, t, lerpedRotation)
		return out
	}
	return frameValues(last, out)
}

// frameValues copies the channels a single stop declares.
func frameValues(k Keyframe, out AnimValues) AnimValues {
	if k.Opacity != nil {
		out.Opacity = *k.Opacity
	}
	if k.Rotation != nil {
		out.Rotation = *k.Rotation
	}
	return out
}

// lerp channel setters.
func lerpedOpacity(out *AnimValues, v float64)  { out.Opacity = v }
func lerpedRotation(out *AnimValues, v float64) { out.Rotation = v }

func lerp(out *AnimValues, a, b *float64, t float64, set func(*AnimValues, float64)) {
	switch {
	case a != nil && b != nil:
		set(out, *a+(*b-*a)*t)
	case a != nil:
		set(out, *a)
	case b != nil:
		set(out, *b)
	}
}

// parseAnimation parses the animation shorthand: a flexible-order run
// of the name (the first ident that is no keyword, or `none`), a
// duration, a timing function, an iteration count (a number or
// `infinite`), and `alternate`. The shorthand resets play-state and
// iteration; animation-play-state merges after it. The stylesheet
// writes `name var(--dur, 1s) linear infinite [alternate]`.
func parseAnimation(ts []token, cx *ctx, v *Values) bool {
	a := &v.Animation
	duration := -1.0
	name := ""
	timing, timingOK := Timing{}, false
	count, countOK := 0.0, false
	alternate := false
	sawNone := false
	for _, part := range components(ts) {
		switch {
		case part[0].kind == tkFunc && part[0].s == "var":
			return false // a var() value computes first; never direct
		case part[0].kind == tkFunc:
			tm, ok := timingOf(part, cx)
			if !ok {
				return false
			}
			timing, timingOK = tm, true
		default:
			if secs, ok := timeOf(part, cx); ok && secs >= 0 {
				if duration < 0 {
					duration = secs
				}
				continue
			}
			if n, ok := evalNumeric(part, cx); ok && n.kind == numNumber && n.v >= 0 {
				count, countOK = n.v, true
				continue
			}
			if part[0].kind != tkIdent {
				return false
			}
			switch strings.ToLower(part[0].s) {
			case "none":
				sawNone = true
			case "infinite":
				count, countOK, a.Infinite = 0, true, true
			case "alternate":
				alternate = true
			case "linear", "ease", "ease-in", "ease-out", "ease-in-out":
				tm, _ := timingOf(part, cx)
				timing, timingOK = tm, true
			default:
				name = strings.ToLower(part[0].s)
			}
		}
	}
	if sawNone {
		*a = Animation{}
		return true
	}
	if name == "" {
		return false
	}
	a.Name = name
	if duration >= 0 {
		a.Duration = duration
	}
	if timingOK {
		a.Timing = timing
	}
	if countOK {
		a.Iteration = count
	}
	a.Alternate = alternate
	a.Running = true
	return true
}

// parseAnimationPlayState parses `running` or `paused` without
// disturbing the rest of the animation group.
func parseAnimationPlayState(ts []token, cx *ctx, v *Values) bool {
	first := splitTop(ts, tkComma)[0]
	if len(first) != 1 || first[0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(first[0].s) {
	case "running":
		v.Animation.Running = true
	case "paused":
		v.Animation.Running = false
	default:
		return false
	}
	return true
}

// parseAnimationName parses the animation-name longhand.
func parseAnimationName(ts []token, cx *ctx, v *Values) bool {
	first := splitTop(ts, tkComma)[0]
	if len(first) != 1 || first[0].kind != tkIdent {
		return false
	}
	if strings.EqualFold(first[0].s, "none") {
		v.Animation = Animation{}
		return true
	}
	v.Animation.Name = strings.ToLower(first[0].s)
	v.Animation.Running = true
	return true
}

// parseIconTransform parses -gtk-icon-transform's rotate(): the icon's
// turn in degrees clockwise. `none` and a non-rotate transform zero it.
func parseIconTransform(ts []token, cx *ctx, v *Values) bool {
	first := splitTop(ts, tkComma)[0]
	if len(first) == 1 && first[0].ident("none") {
		v.Rotation = 0
		return true
	}
	deg, ok := rotateOf(first)
	if !ok {
		return false
	}
	v.Rotation = deg
	return true
}

// parseIconSource parses -gtk-icon-source: -gtk-icontheme("name") or
// none.
func parseIconSource(ts []token, cx *ctx, v *Values) bool {
	first := splitTop(ts, tkComma)[0]
	if len(first) == 1 && first[0].ident("none") {
		v.IconSource = ""
		return true
	}
	if len(first) == 0 || first[0].kind != tkFunc || first[0].s != "-gtk-icontheme" {
		return false
	}
	args := splitTop(funcArgs(first), tkComma)
	if len(args) != 1 || len(args[0]) != 1 || args[0][0].kind != tkString {
		return false
	}
	v.IconSource = args[0][0].s
	return true
}

// parseIconPalette parses -gtk-icon-palette's `success <color>` entry
// into the palette tint; other channels' entries keep their defaults.
func parseIconPalette(ts []token, cx *ctx, v *Values) bool {
	sawChannel := false
	for _, part := range splitTop(ts, tkComma) {
		words := components(part)
		if len(words) != 2 || words[0][0].kind != tkIdent {
			return false
		}
		cv, ok := parseColor(words[1], cx)
		if !ok {
			return false
		}
		if strings.EqualFold(words[0][0].s, "success") {
			v.PaletteTint = cx.resolve(cv)
		}
		sawChannel = true
	}
	return sawChannel
}

// parseCaretColor parses caret-color: a color or auto (the text
// color).
func parseCaretColor(ts []token, cx *ctx, v *Values) bool {
	first := splitTop(ts, tkComma)[0]
	if len(first) == 1 && first[0].ident("auto") {
		v.CaretColor = 0
		return true
	}
	cv, ok := parseColor(first, cx)
	if !ok {
		return false
	}
	v.CaretColor = cx.resolve(cv)
	return true
}

// parseTextDecoration parses text-decoration's line keywords: the
// subset takes underline and none (line-through and overline parse to
// an error, no painter draws them).
func parseTextDecoration(ts []token, cx *ctx, v *Values) bool {
	saw := false
	for _, part := range components(ts) {
		if len(part) != 1 || part[0].kind != tkIdent {
			return false
		}
		switch strings.ToLower(part[0].s) {
		case "none":
			v.Underline = false
			saw = true
		case "underline":
			v.Underline = true
			saw = true
		default:
			return false
		}
	}
	return saw
}

// resolveKeyframes points the computed animation at the named
// @keyframes rule, the highest-priority layer that declares it
// winning. An unknown name runs nothing.
func resolveKeyframes(a *Animation, layers []Layer) {
	if a.Name == "" {
		*a = Animation{}
		return
	}
	for _, layer := range slices.Backward(layers) {
		if kf := layer.Sheet.Keyframes(a.Name); kf != nil {
			a.Keyframes = kf
			return
		}
	}
	a.Keyframes = nil
}

// sortFrames orders a keyframes rule's stops by offset, stable so
// equal offsets keep source order (CSS: the last wins).
func sortFrames(kf *Keyframes) {
	sort.SliceStable(kf.Frames, func(i, j int) bool { return kf.Frames[i].Offset < kf.Frames[j].Offset })
}

// rotateOf parses a transform rotate() angle: deg, grad, rad, or turn,
// returned in degrees clockwise. A non-rotate transform (scale,
// translate) parses to no rotation.
func rotateOf(ts []token) (float64, bool) {
	if len(ts) == 0 || ts[0].kind != tkFunc {
		return 0, false
	}
	args := splitTop(funcArgs(ts), tkComma)
	if len(args) != 1 || len(args[0]) != 1 {
		return 0, false
	}
	t := args[0][0]
	if t.kind != tkDim {
		if t.kind == tkNumber && t.num == 0 {
			return 0, true
		}
		return 0, false
	}
	switch t.s {
	case "deg":
		return t.num, true
	case "grad":
		return t.num * 0.9, true
	case "rad":
		return t.num * 180 / math.Pi, true
	case "turn":
		return t.num * 360, true
	}
	return 0, false
}
