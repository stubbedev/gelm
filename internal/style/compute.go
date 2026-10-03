package style

import (
	"strconv"
	"strings"

	"github.com/stubbedev/gelm/render"
)

// Env is the unit environment a compute resolves against.
type Env struct {
	// Rem is the root font size in logical pixels: what 1rem is.
	Rem float64
	// FontPx is the font size a node computes when nothing in its
	// ancestry sets one (CSS's `medium`); zero means Rem.
	FontPx float64
}

// maxVarDepth bounds var() expansion: a chain deeper than this is
// treated as a cycle.
const maxVarDepth = 32

// Compute runs the cascade for n: match every layer (and n's inline
// block), resolve the node's custom properties over parent's, substitute
// var() and parse the winning declarations, then inherit the unset
// inherited longhands from parent. parent is nil at the root. old, when
// non-nil, is the node's previous result: an unchanged custom-property
// environment keeps its old pointer so equality stays cheap.
func Compute(layers []Layer, n Node, parent, old *Values, env Env, sc *Scratch, v *Values) {
	*v = Values{}
	if env.Rem <= 0 {
		env.Rem = 16
	}
	if env.FontPx <= 0 {
		env.FontPx = env.Rem
	}
	sc.collect(layers, n)

	var pv *Values
	if parent != nil {
		pv = parent
	} else {
		pv = &Values{}
	}
	v.Vars = sc.resolveVars(pv.Vars)
	if old != nil && old.Vars != v.Vars && v.Vars.equalOwn(old.Vars) {
		v.Vars = old.Vars
	}

	// The context lives in the scratch: the parsers are function values,
	// so a stack ctx would escape and allocate per compute.
	sc.cx = ctx{rem: env.Rem, em: env.FontPx, weight: 400}
	cx := &sc.cx
	if pv.Has(PropFontSize) {
		cx.em = pv.FontSize
	}
	if pv.Has(PropFontWeight) {
		cx.weight = pv.FontWeight
	}
	if pv.Has(PropColor) {
		cx.color = pv.Color
	}
	sc.parsed = sc.parsed[:0]

	// font-size first: em lengths everywhere else resolve against it.
	// color next: currentColor everywhere else resolves against it.
	var blocked PropSet
	blocked |= sc.computeProp(PropFontSize, v, pv, cx)
	if v.Has(PropFontSize) {
		cx.em = v.FontSize
	}
	blocked |= sc.computeProp(PropColor, v, pv, cx)
	if v.Has(PropColor) {
		cx.color = v.Color
	}
	for p := range numProps {
		if p == PropFontSize || p == PropColor {
			continue
		}
		blocked |= sc.computeProp(p, v, pv, cx)
	}

	// Inheritance: unset inherited longhands take the parent's value,
	// unless an explicit `initial` blocked it.
	v.Own = v.Set
	for p := range numProps {
		if inheritedProps.Has(p) && !v.Has(p) && !blocked.Has(p) && pv.Has(p) {
			copyProp(v, pv, p)
			v.Set |= 1 << p
		}
	}
	resolveKeyframes(&v.Animation, layers)
}

// computeProp computes one longhand from its winning declaration. It
// returns p's bit when an explicit `initial` must keep the inherited
// value out.
func (sc *Scratch) computeProp(p Prop, v, parent *Values, cx *ctx) PropSet {
	d := sc.win[p]
	if d == nil {
		return 0
	}
	wide := d.wide
	if wide == wideNone {
		src, ok := sc.parse(d, v.Vars, cx)
		if ok {
			copyProp(v, src, p)
			v.Set |= 1 << p
			return 0
		}
		// Invalid at computed-value time: the declaration acts as unset.
		wide = wideUnset
	}
	if wide == wideUnset {
		if inheritedProps.Has(p) {
			wide = wideInherit
		} else {
			wide = wideInitial
		}
	}
	switch wide {
	case wideInherit:
		if parent.Has(p) {
			copyProp(v, parent, p)
			v.Set |= 1 << p
		} else if !inheritedProps.Has(p) {
			sc.setInitial(p, v, cx)
		}
	case wideInitial:
		// An inherited property's initial value is the theme's (the
		// consumer's default), so it stays unset; the rest take the CSS
		// initial value, which is what strips theme styling under
		// `all: unset`.
		if !inheritedProps.Has(p) {
			sc.setInitial(p, v, cx)
			return 0
		}
		return 1 << p
	}
	return 0
}

// setInitial writes p's initial value. Border and outline colors start
// as currentColor.
func (sc *Scratch) setInitial(p Prop, v *Values, cx *ctx) {
	init := initialValues()
	init.BorderColor = [4]render.Color{cx.color, cx.color, cx.color, cx.color}
	init.OutlineColor = cx.color
	copyProp(v, &init, p)
	v.Set |= 1 << p
}

// parse substitutes and parses d once per compute.
func (sc *Scratch) parse(d *decl, vars *Vars, cx *ctx) (*Values, bool) {
	if d.pre != nil && (d.preRem == 0 || d.preRem == cx.rem) {
		return d.pre, true
	}
	for i := range sc.parsed {
		if sc.parsed[i].d == d {
			return &sc.parsed[i].v, sc.parsed[i].ok
		}
	}
	ts := d.val
	ok := true
	if d.hasVar {
		ts, ok = substitute(d.val, func(name string) ([]token, bool) { return vars.lookup(name) }, 0)
		ts = trimWS(ts)
		ok = ok && len(ts) > 0
	}
	sc.parsed = append(sc.parsed, parsedDecl{d: d})
	e := &sc.parsed[len(sc.parsed)-1]
	e.v = initialValues()
	if ok {
		e.ok = d.parse(ts, cx, &e.v)
	}
	return &e.v, e.ok
}

// resolveVars computes the node's custom-property environment: the
// winning declarations with their own var() references substituted
// (against each other and the inherited chain), cycles made invalid.
func (sc *Scratch) resolveVars(parent *Vars) *Vars {
	if len(sc.customs) == 0 {
		return parent
	}
	own := make(map[string][]token, len(sc.customs))
	const (
		pending = iota
		resolving
		done
	)
	state := make(map[string]uint8, len(sc.customs))
	var resolve func(name string, depth int) ([]token, bool)
	resolve = func(name string, depth int) ([]token, bool) {
		i, mine := sc.byName[name]
		if !mine {
			return parent.lookup(name)
		}
		switch state[name] {
		case done:
			ts := own[name]
			return ts, ts != nil
		case resolving:
			return nil, false // a cycle: every member is invalid
		}
		if depth > maxVarDepth {
			return nil, false
		}
		state[name] = resolving
		d := sc.customs[i].d
		var ts []token
		ok := true
		switch d.wide {
		case wideInherit, wideUnset:
			ts, ok = parent.lookup(name)
		case wideInitial:
			ok = false
		default:
			ts = d.val
			if d.hasVar {
				ts, ok = substitute(d.val, func(n string) ([]token, bool) { return resolve(n, depth+1) }, depth)
				ts = trimWS(ts)
			}
		}
		state[name] = done
		if !ok {
			own[name] = nil
			return nil, false
		}
		if ts == nil {
			ts = []token{} // an empty value is valid and distinct from invalid
		}
		own[name] = ts
		return ts, true
	}
	for _, w := range sc.customs {
		resolve(w.d.name, 0)
	}
	return &Vars{parent: parent, own: own}
}

// substitute replaces every var(--name[, fallback]) in ts with the named
// value (or the substituted fallback). ok is false when a reference
// cannot resolve: the declaration is then invalid at computed-value
// time.
func substitute(ts []token, lookup func(string) ([]token, bool), depth int) ([]token, bool) {
	if depth > maxVarDepth {
		return nil, false
	}
	var out []token
	for i := 0; i < len(ts); i++ {
		t := ts[i]
		if t.kind != tkFunc || t.s != "var" {
			out = append(out, t)
			continue
		}
		end := blockEnd(ts, i)
		args := trimWS(funcArgs(ts[i:end]))
		if len(args) == 0 || args[0].kind != tkIdent || !strings.HasPrefix(args[0].s, "--") {
			return nil, false
		}
		name := args[0].s
		rest := trimWS(args[1:])
		val, found := lookup(name)
		switch {
		case found:
			out = append(out, val...)
		case len(rest) > 0 && rest[0].kind == tkComma:
			fb, ok := substitute(rest[1:], lookup, depth+1)
			if !ok {
				return nil, false
			}
			out = append(out, trimWS(fb)...)
		default:
			return nil, false
		}
		i = end - 1
	}
	return out, true
}

// copyProp copies longhand p's computed value from src to dst.
func copyProp(dst, src *Values, p Prop) {
	switch p {
	case PropColor:
		dst.Color = src.Color
	case PropBackgroundColor:
		dst.Background = src.Background
	case PropBackgroundImage:
		dst.Image = src.Image
	case PropOpacity:
		dst.Opacity = src.Opacity
	case PropFilter:
		dst.Brightness = src.Brightness
	case PropPaddingTop:
		dst.Padding.Top = src.Padding.Top
	case PropPaddingRight:
		dst.Padding.Right = src.Padding.Right
	case PropPaddingBottom:
		dst.Padding.Bottom = src.Padding.Bottom
	case PropPaddingLeft:
		dst.Padding.Left = src.Padding.Left
	case PropMarginTop:
		dst.Margin.Top = src.Margin.Top
	case PropMarginRight:
		dst.Margin.Right = src.Margin.Right
	case PropMarginBottom:
		dst.Margin.Bottom = src.Margin.Bottom
	case PropMarginLeft:
		dst.Margin.Left = src.Margin.Left
	case PropBorderTopWidth:
		dst.BorderWidth.Top = src.BorderWidth.Top
	case PropBorderRightWidth:
		dst.BorderWidth.Right = src.BorderWidth.Right
	case PropBorderBottomWidth:
		dst.BorderWidth.Bottom = src.BorderWidth.Bottom
	case PropBorderLeftWidth:
		dst.BorderWidth.Left = src.BorderWidth.Left
	case PropBorderTopStyle:
		dst.BorderStyle[0] = src.BorderStyle[0]
	case PropBorderRightStyle:
		dst.BorderStyle[1] = src.BorderStyle[1]
	case PropBorderBottomStyle:
		dst.BorderStyle[2] = src.BorderStyle[2]
	case PropBorderLeftStyle:
		dst.BorderStyle[3] = src.BorderStyle[3]
	case PropBorderTopColor:
		dst.BorderColor[0] = src.BorderColor[0]
	case PropBorderRightColor:
		dst.BorderColor[1] = src.BorderColor[1]
	case PropBorderBottomColor:
		dst.BorderColor[2] = src.BorderColor[2]
	case PropBorderLeftColor:
		dst.BorderColor[3] = src.BorderColor[3]
	case PropBorderTopLeftRadius:
		dst.Radius.TopLeft = src.Radius.TopLeft
	case PropBorderTopRightRadius:
		dst.Radius.TopRight = src.Radius.TopRight
	case PropBorderBottomRightRadius:
		dst.Radius.BottomRight = src.Radius.BottomRight
	case PropBorderBottomLeftRadius:
		dst.Radius.BottomLeft = src.Radius.BottomLeft
	case PropBoxShadow:
		dst.Shadow = src.Shadow
	case PropOutlineWidth:
		dst.OutlineWidth = src.OutlineWidth
	case PropOutlineStyle:
		dst.OutlineStyle = src.OutlineStyle
	case PropOutlineColor:
		dst.OutlineColor = src.OutlineColor
	case PropOutlineOffset:
		dst.OutlineOffset = src.OutlineOffset
	case PropMinWidth:
		dst.MinWidth = src.MinWidth
	case PropMinHeight:
		dst.MinHeight = src.MinHeight
	case PropBorderSpacing:
		dst.BorderSpacingH, dst.BorderSpacingV = src.BorderSpacingH, src.BorderSpacingV
	case PropFontFamily:
		dst.FontFamily = src.FontFamily
	case PropFontSize:
		dst.FontSize = src.FontSize
	case PropFontWeight:
		dst.FontWeight = src.FontWeight
	case PropFontStyle:
		dst.Italic = src.Italic
	case PropLetterSpacing:
		dst.LetterSpacing = src.LetterSpacing
	case PropTextTransform:
		dst.TextTransform = src.TextTransform
	case PropIconSize:
		dst.IconSize = src.IconSize
	case PropTransitionProperty:
		dst.Transition.All, dst.Transition.Props = src.Transition.All, src.Transition.Props
	case PropTransitionDuration:
		dst.Transition.Duration = src.Transition.Duration
	case PropTransitionTiming:
		dst.Transition.Timing = src.Transition.Timing
	case PropTransitionDelay:
		dst.Transition.Delay = src.Transition.Delay
	case PropAnimation:
		dst.Animation = src.Animation
	case PropAnimationPlayState:
		dst.Animation.Running = src.Animation.Running
	case PropIconTransform:
		dst.Rotation = src.Rotation
	}
}

// tokensText serializes tokens back to CSS source text.
func tokensText(ts []token) string {
	var b strings.Builder
	for _, t := range ts {
		switch t.kind {
		case tkIdent:
			b.WriteString(t.s)
		case tkFunc:
			b.WriteString(t.s)
			b.WriteByte('(')
		case tkAt:
			b.WriteByte('@')
			b.WriteString(t.s)
		case tkHash:
			b.WriteByte('#')
			b.WriteString(t.s)
		case tkString:
			b.WriteString(strconv.Quote(t.s))
		case tkNumber:
			b.WriteString(strconv.FormatFloat(t.num, 'f', -1, 64))
		case tkPercent:
			b.WriteString(strconv.FormatFloat(t.num, 'f', -1, 64))
			b.WriteByte('%')
		case tkDim:
			b.WriteString(strconv.FormatFloat(t.num, 'f', -1, 64))
			b.WriteString(t.s)
		case tkWS:
			b.WriteByte(' ')
		default:
			b.WriteString(t.s)
		}
	}
	return b.String()
}
