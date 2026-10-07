package style

// CSS animations: @keyframes rules, the animation shorthand and its
// longhands, and animation-play-state. A keyframes rule declares named
// stops over the animation's progress; the shorthand points a node at
// one and gives the duration, delay, curve, iteration count, direction,
// and fill mode, and a comma list runs several at once. The animated
// channels are the interpolatable computed values - opacity, the text
// and background colors, the filter's brightness, the transform, and
// the icon transform - and the widget layer runs the tweens against its
// style cache, the same channels the transitions use.

import (
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/stubbedev/gelm/render"
)

// Chan is one animatable channel, the bit a Keyframe declares and a
// running animation overrides in the style cache.
type Chan uint8

// The channels, one bit each.
const (
	// ChOpacity is the opacity channel.
	ChOpacity Chan = 1 << iota
	ChColor
	ChBackground
	ChBrightness
	ChTransform
	ChIconXform
)

// Keyframes is one @keyframes rule: named stops over the animation's
// progress. Mask records the union of channels the stops declare, so
// the runner knows which cache fields a phase read owns.
type Keyframes struct {
	Name   string
	Frames []Keyframe
	Mask   uint32
}

// Keyframe is one stop of a keyframes rule: its offset along the
// animation's progress and the channels it animates. A nil channel is
// not animated at that stop - the property runs between the stops that
// declare it and holds the nearest declared value outside them,
// exactly like CSS's missing-keyframe rule.
type Keyframe struct {
	Offset     float64 // 0..1; `from` is 0, `to` is 1
	Opacity    *float64
	Color      *render.Color
	Background *render.Color
	Brightness *float64
	Transform  *Xform
	IconXform  *Xform
}

// Animation is one computed animation: which @keyframes rule runs, for
// how long after what delay, along which curve, how many times, in
// which direction, and what holds when it is not running. A nil
// Keyframes (an unknown name, or `animation: none`) runs nothing.
type Animation struct {
	Name      string
	Duration  float64 // seconds
	Delay     float64 // seconds; negative starts mid-flight
	Timing    Timing
	Infinite  bool
	Iteration float64
	Direction uint8 // DirNormal..DirAlternateReverse
	Fill      uint8 // FillNone..FillBoth
	Running   bool
	Keyframes *Keyframes
}

// The animation-direction values.
const (
	DirNormal uint8 = iota
	DirReverse
	DirAlternate
	DirAlternateReverse
)

// The animation-fill-mode values.
const (
	FillNone uint8 = iota
	FillForwards
	FillBackwards
	FillBoth
)

// Active reports whether an animation should be running: named, timed,
// resolved, and not paused.
func (a Animation) Active() bool {
	return a.Keyframes != nil && a.Name != "" && a.Duration > 0 && a.Running
}

// AnimValues is one interpolated animation frame: the animated
// channels' values at a phase of the animation. Channels the rule
// never declares read zero and are masked off by Keyframes.Mask.
type AnimValues struct {
	Opacity    float64
	Color      render.Color
	Background render.Color
	Brightness float64
	Transform  Xform
	IconXform  Xform
}

// mask computes the union of the stops' channels.
func (kf *Keyframes) mask() uint32 {
	var m uint32
	for i := range kf.Frames {
		f := &kf.Frames[i]
		if f.Opacity != nil {
			m |= uint32(ChOpacity)
		}
		if f.Color != nil {
			m |= uint32(ChColor)
		}
		if f.Background != nil {
			m |= uint32(ChBackground)
		}
		if f.Brightness != nil {
			m |= uint32(ChBrightness)
		}
		if f.Transform != nil {
			m |= uint32(ChTransform)
		}
		if f.IconXform != nil {
			m |= uint32(ChIconXform)
		}
	}
	return m
}

// At interpolates the keyframes at phase (0..1) over the channels the
// rule declares.
func (kf *Keyframes) At(phase float64) AnimValues {
	frames := kf.Frames
	if len(frames) == 0 {
		return AnimValues{}
	}
	if phase <= frames[0].Offset {
		return frameValues(frames[0])
	}
	last := frames[len(frames)-1]
	if phase >= last.Offset {
		return frameValues(last)
	}
	for i := 0; i+1 < len(frames); i++ {
		a, b := frames[i], frames[i+1]
		if phase < a.Offset || phase > b.Offset {
			continue
		}
		t := (phase - a.Offset) / (b.Offset - a.Offset)
		return mixFrames(a, b, t)
	}
	return frameValues(last)
}

// frameValues copies the channels one stop declares.
func frameValues(k Keyframe) AnimValues {
	out := AnimValues{}
	if k.Opacity != nil {
		out.Opacity = *k.Opacity
	}
	if k.Color != nil {
		out.Color = *k.Color
	}
	if k.Background != nil {
		out.Background = *k.Background
	}
	if k.Brightness != nil {
		out.Brightness = *k.Brightness
	}
	if k.Transform != nil {
		out.Transform = *k.Transform
	}
	if k.IconXform != nil {
		out.IconXform = *k.IconXform
	}
	return out
}

// mixFrames interpolates every channel between two stops by the
// missing-keyframe rule: a channel one side omits holds the declared
// side's value; the transforms walk their decompositions.
func mixFrames(a, b Keyframe, t float64) AnimValues {
	out := AnimValues{}
	out.Opacity = lerpPtr(a.Opacity, b.Opacity, t)
	out.Color = mixPtrColor(a.Color, b.Color, t)
	out.Background = mixPtrColor(a.Background, b.Background, t)
	out.Brightness = lerpPtr(a.Brightness, b.Brightness, t)
	out.Transform = lerpPtrXform(a.Transform, b.Transform, t)
	out.IconXform = lerpPtrXform(a.IconXform, b.IconXform, t)
	return out
}

func lerpPtr(a, b *float64, t float64) float64 {
	switch {
	case a != nil && b != nil:
		return *a + (*b-*a)*t
	case a != nil:
		return *a
	case b != nil:
		return *b
	}
	return 0
}

func mixPtrColor(a, b *render.Color, t float64) render.Color {
	switch {
	case a != nil && b != nil:
		return mixAnimColor(*a, *b, t)
	case a != nil:
		return *a
	case b != nil:
		return *b
	}
	return 0
}

func lerpPtrXform(a, b *Xform, t float64) Xform {
	switch {
	case a != nil && b != nil:
		return LerpXform(*a, *b, t)
	case a != nil:
		return *a
	case b != nil:
		return *b
	}
	return XformIdentity
}

// mixAnimColor interpolates two premultiplied colors per channel.
func mixAnimColor(a, b render.Color, t float64) render.Color {
	mix := func(x, y uint8) uint8 {
		return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5)
	}
	return render.Color(uint32(mix(a.A(), b.A()))<<24 |
		uint32(mix(a.R(), b.R()))<<16 |
		uint32(mix(a.G(), b.G()))<<8 |
		uint32(mix(a.B(), b.B())))
}

// parseAnimation parses the animation shorthand, one entry per comma
// group: a flexible-order run of the name (the first ident that is no
// keyword, or `none`), a duration, a delay (the second time; negative
// starts mid-flight), a timing function, an iteration count (a number
// or `infinite`), and the direction and fill keywords. The shorthand
// resets play-state and iteration; animation-play-state merges after
// it. The stylesheet writes
// `name var(--dur, 1s) linear infinite [alternate]`.
func parseAnimation(ts []token, cx *ctx, v *Values) bool {
	v.Animation = nil
	for _, grp := range splitTop(ts, tkComma) {
		a, none, ok := parseOneAnimation(grp, cx)
		if !ok {
			return false
		}
		if none {
			continue
		}
		v.Animation = append(v.Animation, a)
	}
	return true
}

// parseOneAnimation parses one comma group of the shorthand; none
// reports a `none` group (kept out of the list).
func parseOneAnimation(ts []token, cx *ctx) (a Animation, none, ok bool) {
	a = Animation{Running: true}
	duration, delay := -1.0, 0.0
	named := false
	for _, part := range components(ts) {
		switch {
		case part[0].kind == tkFunc && part[0].s == "var":
			return Animation{}, false, false // a var() value computes first; never direct
		case part[0].kind == tkFunc:
			tm, pok := timingOf(part, cx)
			if !pok {
				return Animation{}, false, false
			}
			a.Timing = tm
		default:
			if secs, pok := timeOf(part, cx); pok {
				if duration < 0 {
					duration = secs
				} else {
					delay = secs
				}
				continue
			}
			if n, pok := evalNumeric(part, cx); pok && n.kind == numNumber && n.v >= 0 {
				a.Iteration = n.v
				continue
			}
			if part[0].kind != tkIdent {
				return Animation{}, false, false
			}
			switch strings.ToLower(part[0].s) {
			case "none":
				return Animation{}, true, true
			case "infinite":
				a.Infinite = true
			case "alternate":
				a.Direction = DirAlternate
			case "alternate-reverse":
				a.Direction = DirAlternateReverse
			case "reverse":
				a.Direction = DirReverse
			case "forwards":
				a.Fill = FillForwards
			case "backwards":
				a.Fill = FillBackwards
			case "both":
				a.Fill = FillBoth
			case "linear", "ease", "ease-in", "ease-out", "ease-in-out":
				a.Timing, _ = timingOf(part, cx)
			default:
				if named {
					return Animation{}, false, false
				}
				named = true
				a.Name = strings.ToLower(part[0].s)
			}
		}
	}
	if duration >= 0 {
		a.Duration = duration
	}
	a.Delay = delay
	return a, false, true
}

// animZip applies one longhand's comma list across the computed
// animations, extending the list when the longhand is longer: CSS
// zips each animation-* list against the name list.
func animZip(v *Values, apply func(a *Animation)) {
	if len(v.Animation) == 0 {
		v.Animation = append(v.Animation, Animation{})
	}
	for i := range v.Animation {
		apply(&v.Animation[i])
	}
}

// mergeAnimSlots zips the longhand slots onto the shorthand's group;
// the slots carry their first value, so a single longhand retimes
// every entry, the zip CSS gives a one-entry list.
func mergeAnimSlots(v *Values) {
	zipF := func(slot []float64, set func(a *Animation, x float64)) {
		if len(slot) == 0 {
			return
		}
		for i := range v.Animation {
			set(&v.Animation[i], slot[0])
		}
	}
	zipU := func(slot []uint8, set func(a *Animation, x uint8)) {
		if len(slot) == 0 {
			return
		}
		for i := range v.Animation {
			set(&v.Animation[i], slot[0])
		}
	}
	zipF(v.AnimDuration, func(a *Animation, x float64) { a.Duration = x })
	zipF(v.AnimDelay, func(a *Animation, x float64) { a.Delay = x })
	zipU(v.AnimDirection, func(a *Animation, x uint8) { a.Direction = x })
	zipU(v.AnimFill, func(a *Animation, x uint8) { a.Fill = x })
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
		animZip(v, func(a *Animation) { a.Running = true })
	case "paused":
		animZip(v, func(a *Animation) { a.Running = false })
	default:
		return false
	}
	return true
}

// parseAnimationName parses the animation-name longhand: one name per
// comma group, or `none`.
func parseAnimationName(ts []token, cx *ctx, v *Values) bool {
	v.Animation = nil
	for _, grp := range splitTop(ts, tkComma) {
		comps := components(grp)
		if len(comps) != 1 || comps[0][0].kind != tkIdent {
			return false
		}
		if strings.EqualFold(comps[0][0].s, "none") {
			continue
		}
		v.Animation = append(v.Animation, Animation{
			Name:    strings.ToLower(comps[0][0].s),
			Running: true,
		})
	}
	return true
}

// parseAnimationDirection parses animation-direction.
func parseAnimationDirection(ts []token, cx *ctx, v *Values) bool {
	first := splitTop(ts, tkComma)[0]
	if len(first) != 1 || first[0].kind != tkIdent {
		return false
	}
	var dir uint8
	switch strings.ToLower(first[0].s) {
	case "normal":
		dir = DirNormal
	case "reverse":
		dir = DirReverse
	case "alternate":
		dir = DirAlternate
	case "alternate-reverse":
		dir = DirAlternateReverse
	default:
		return false
	}
	v.AnimDirection = []uint8{dir}
	return true
}

// parseAnimationFillMode parses animation-fill-mode.
func parseAnimationFillMode(ts []token, cx *ctx, v *Values) bool {
	first := splitTop(ts, tkComma)[0]
	if len(first) != 1 || first[0].kind != tkIdent {
		return false
	}
	var fill uint8
	switch strings.ToLower(first[0].s) {
	case "none":
		fill = FillNone
	case "forwards":
		fill = FillForwards
	case "backwards":
		fill = FillBackwards
	case "both":
		fill = FillBoth
	default:
		return false
	}
	v.AnimFill = []uint8{fill}
	return true
}

// parseIconTransform parses -gtk-icon-transform over the same full
// 2-D set as transform, composed into the affine icons draw through.
func parseIconTransform(ts []token, cx *ctx, v *Values) bool {
	m, ok := composeTransform(ts, cx)
	if !ok {
		return false
	}
	v.IconXform = m
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

// parseTextDecoration parses the text-decoration shorthand: none, or
// any of the lines (underline, overline, line-through), a style
// (solid, double, dotted, dashed, wavy), and a color, in any order.
func parseTextDecoration(ts []token, cx *ctx, v *Values) bool {
	var d render.Decoration
	none, lines, style, color := false, false, false, false
	for _, part := range components(ts) {
		if len(part) == 1 && part[0].kind == tkIdent {
			name := strings.ToLower(part[0].s)
			if line, ok := decorationLines[name]; ok {
				d.Lines |= line
				lines = true
				continue
			}
			if st, ok := decorationStyles[name]; ok && !style {
				d.Style, style = st, true
				continue
			}
			if name == "none" && !none {
				none = true
				continue
			}
		}
		cv, ok := parseColor(part, cx)
		if !ok || color {
			return false
		}
		d.Color, color = cx.resolve(cv), true
	}
	if none && lines {
		return false
	}
	if !none && !lines && !style && !color {
		return false
	}
	v.Decoration = d
	return true
}

// decorationLines and decorationStyles are text-decoration's keywords.
var (
	decorationLines = map[string]render.DecorationLine{
		"underline": render.Underline, "overline": render.Overline, "line-through": render.LineThrough,
	}
	decorationStyles = map[string]render.DecorationStyle{
		"solid": render.DecorationSolid, "double": render.DecorationDouble, "dotted": render.DecorationDotted,
		"dashed": render.DecorationDashed, "wavy": render.DecorationWavy,
	}
)

// resolveKeyframes points the computed animations at the named
// @keyframes rules, the highest-priority layer that declares each
// winning, and stamps each rule's channel mask. An unknown name runs
// nothing.
func resolveKeyframes(v *Values, layers []Layer) {
	for i := range v.Animation {
		a := &v.Animation[i]
		if a.Name == "" {
			*a = Animation{}
			continue
		}
		for _, layer := range slices.Backward(layers) {
			if kf := layer.Sheet.Keyframes(a.Name); kf != nil {
				a.Keyframes = kf
				break
			}
		}
		if a.Keyframes != nil && a.Keyframes.Mask == 0 {
			a.Keyframes.Mask = a.Keyframes.mask()
		}
	}
}

// sortFrames orders a keyframes rule's stops by offset, stable so
// equal offsets keep source order (CSS: the last wins), and stamps the
// channel mask the stops declare.
func sortFrames(kf *Keyframes) {
	sort.SliceStable(kf.Frames, func(i, j int) bool { return kf.Frames[i].Offset < kf.Frames[j].Offset })
	kf.Mask = kf.mask()
}

// angleOf parses one angle token: deg, grad, rad, or turn, returned in
// degrees clockwise; a bare 0 is allowed.
func angleOf(t token) (float64, bool) {
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
