package style

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/stubbedev/gelm/render"
)

// propDef describes one property name: the longhands it sets and the
// parser that writes them into a scratch Values.
type propDef struct {
	covers PropSet
	parse  func(ts []token, cx *ctx, v *Values) bool
}

// ignoredProps are real GTK/CSS properties the engine accepts and
// drops without a warning: they parse in every GTK stylesheet but have
// no painter here (animations, icon transforms, text decoration, ...).
var ignoredProps = map[string]bool{
	"animation-duration": true, "animation-timing-function": true,
	"animation-iteration-count": true, "animation-direction": true,
	"animation-delay": true, "animation-fill-mode": true,
	"-gtk-icon-shadow": true, "-gtk-icon-style": true,
	"-gtk-icon-filter": true, "-gtk-dpi": true, "-gtk-secondary-caret-color": true,
	"text-shadow": true, "text-decoration-line": true,
	"text-decoration-color": true, "text-decoration-style": true,
	"font-variant-numeric": true, "font-variant": true,
	"font-stretch": true, "font-kerning": true,
	"background-size": true, "background-position": true,
	"background-repeat": true, "background-clip": true, "background-origin": true,
	"background-blend-mode": true, "border-image": true, "border-image-source": true,
	"border-image-slice": true, "border-image-width": true, "border-image-repeat": true,
	"outline-radius": true, "-gtk-outline-radius": true, "text-overflow": true,
}

// propTable maps every supported property name to its definition.
var propTable map[string]propDef

func init() {
	propTable = map[string]propDef{
		"color":            {setOf(PropColor), colorInto(func(v *Values) *render.Color { return &v.Color })},
		"background-color": {setOf(PropBackgroundColor), colorInto(func(v *Values) *render.Color { return &v.Background })},
		"background-image": {setOf(PropBackgroundImage), parseBackgroundImage},
		"background":       {setOf(PropBackgroundColor, PropBackgroundImage), parseBackground},
		"opacity":          {setOf(PropOpacity), parseOpacity},
		"filter":           {setOf(PropFilter), parseFilter},

		"padding":        {setOf(PropPaddingTop, PropPaddingRight, PropPaddingBottom, PropPaddingLeft), sidesInto(func(v *Values) *Sides { return &v.Padding }, false)},
		"padding-top":    {setOf(PropPaddingTop), sideInto(func(v *Values) *int { return &v.Padding.Top }, false)},
		"padding-right":  {setOf(PropPaddingRight), sideInto(func(v *Values) *int { return &v.Padding.Right }, false)},
		"padding-bottom": {setOf(PropPaddingBottom), sideInto(func(v *Values) *int { return &v.Padding.Bottom }, false)},
		"padding-left":   {setOf(PropPaddingLeft), sideInto(func(v *Values) *int { return &v.Padding.Left }, false)},
		"margin":         {setOf(PropMarginTop, PropMarginRight, PropMarginBottom, PropMarginLeft), sidesInto(func(v *Values) *Sides { return &v.Margin }, true)},
		"margin-top":     {setOf(PropMarginTop), sideInto(func(v *Values) *int { return &v.Margin.Top }, true)},
		"margin-right":   {setOf(PropMarginRight), sideInto(func(v *Values) *int { return &v.Margin.Right }, true)},
		"margin-bottom":  {setOf(PropMarginBottom), sideInto(func(v *Values) *int { return &v.Margin.Bottom }, true)},
		"margin-left":    {setOf(PropMarginLeft), sideInto(func(v *Values) *int { return &v.Margin.Left }, true)},

		"border-width":        {borderWidths, sidesInto(func(v *Values) *Sides { return &v.BorderWidth }, false)},
		"border-top-width":    {setOf(PropBorderTopWidth), sideInto(func(v *Values) *int { return &v.BorderWidth.Top }, false)},
		"border-right-width":  {setOf(PropBorderRightWidth), sideInto(func(v *Values) *int { return &v.BorderWidth.Right }, false)},
		"border-bottom-width": {setOf(PropBorderBottomWidth), sideInto(func(v *Values) *int { return &v.BorderWidth.Bottom }, false)},
		"border-left-width":   {setOf(PropBorderLeftWidth), sideInto(func(v *Values) *int { return &v.BorderWidth.Left }, false)},
		"border-style":        {borderStyles, parseBorderStyles},
		"border-top-style":    {setOf(PropBorderTopStyle), borderStyleInto(0)},
		"border-right-style":  {setOf(PropBorderRightStyle), borderStyleInto(1)},
		"border-bottom-style": {setOf(PropBorderBottomStyle), borderStyleInto(2)},
		"border-left-style":   {setOf(PropBorderLeftStyle), borderStyleInto(3)},
		"border-color":        {borderColors, parseBorderColors},
		"border-top-color":    {setOf(PropBorderTopColor), colorInto(func(v *Values) *render.Color { return &v.BorderColor[0] })},
		"border-right-color":  {setOf(PropBorderRightColor), colorInto(func(v *Values) *render.Color { return &v.BorderColor[1] })},
		"border-bottom-color": {setOf(PropBorderBottomColor), colorInto(func(v *Values) *render.Color { return &v.BorderColor[2] })},
		"border-left-color":   {setOf(PropBorderLeftColor), colorInto(func(v *Values) *render.Color { return &v.BorderColor[3] })},
		"border":              {borderWidths | borderStyles | borderColors, borderSideInto(0b1111)},
		"border-top":          {setOf(PropBorderTopWidth, PropBorderTopStyle, PropBorderTopColor), borderSideInto(0b0001)},
		"border-right":        {setOf(PropBorderRightWidth, PropBorderRightStyle, PropBorderRightColor), borderSideInto(0b0010)},
		"border-bottom":       {setOf(PropBorderBottomWidth, PropBorderBottomStyle, PropBorderBottomColor), borderSideInto(0b0100)},
		"border-left":         {setOf(PropBorderLeftWidth, PropBorderLeftStyle, PropBorderLeftColor), borderSideInto(0b1000)},

		"border-radius":              {borderRadii, parseBorderRadius},
		"border-top-left-radius":     {setOf(PropBorderTopLeftRadius), cornerInto(func(v *Values) *int { return &v.Radius.TopLeft })},
		"border-top-right-radius":    {setOf(PropBorderTopRightRadius), cornerInto(func(v *Values) *int { return &v.Radius.TopRight })},
		"border-bottom-right-radius": {setOf(PropBorderBottomRightRadius), cornerInto(func(v *Values) *int { return &v.Radius.BottomRight })},
		"border-bottom-left-radius":  {setOf(PropBorderBottomLeftRadius), cornerInto(func(v *Values) *int { return &v.Radius.BottomLeft })},

		"box-shadow": {setOf(PropBoxShadow), parseBoxShadow},

		"outline":        {setOf(PropOutlineWidth, PropOutlineStyle, PropOutlineColor), parseOutline},
		"outline-width":  {setOf(PropOutlineWidth), lengthInto(func(v *Values) *int { return &v.OutlineWidth }, false)},
		"outline-style":  {setOf(PropOutlineStyle), outlineStyleInto},
		"outline-color":  {setOf(PropOutlineColor), colorInto(func(v *Values) *render.Color { return &v.OutlineColor })},
		"outline-offset": {setOf(PropOutlineOffset), lengthInto(func(v *Values) *int { return &v.OutlineOffset }, true)},

		"min-width":      {setOf(PropMinWidth), lengthInto(func(v *Values) *int { return &v.MinWidth }, false)},
		"min-height":     {setOf(PropMinHeight), lengthInto(func(v *Values) *int { return &v.MinHeight }, false)},
		"border-spacing": {setOf(PropBorderSpacing), parseBorderSpacing},

		"font-family":             {setOf(PropFontFamily), parseFontFamily},
		"font-size":               {setOf(PropFontSize), parseFontSize},
		"font-weight":             {setOf(PropFontWeight), parseFontWeight},
		"font-style":              {setOf(PropFontStyle), parseFontStyle},
		"font":                    {setOf(PropFontFamily, PropFontSize, PropFontWeight, PropFontStyle), parseFont},
		"letter-spacing":          {setOf(PropLetterSpacing), parseLetterSpacing},
		"text-transform":          {setOf(PropTextTransform), parseTextTransform},
		"line-height":             {setOf(PropLineHeight), parseLineHeight},
		"transform":               {setOf(PropTransform), parseTransform},
		"transform-origin":        {setOf(PropTransformOrigin), parseTransformOrigin},
		"font-feature-settings":   {setOf(PropFontFeatures), parseFontFeatures},
		"font-variation-settings": {setOf(PropFontVariations), parseFontVariations},
		"-gtk-icon-size":          {setOf(PropIconSize), lengthInto(func(v *Values) *int { return &v.IconSize }, false)},

		"transition":                 {transitionProps, parseTransition},
		"transition-property":        {setOf(PropTransitionProperty), parseTransitionProperty},
		"transition-duration":        {setOf(PropTransitionDuration), timeInto(func(v *Values) *float64 { return &v.Transition.Duration })},
		"transition-delay":           {setOf(PropTransitionDelay), timeInto(func(v *Values) *float64 { return &v.Transition.Delay })},
		"transition-timing-function": {setOf(PropTransitionTiming), parseTransitionTiming},

		"animation":            {animationProps, parseAnimation},
		"animation-name":       {setOf(PropAnimation), parseAnimationName},
		"animation-duration":   {setOf(PropAnimDuration), animTimeInto(func(v *Values, secs float64) { v.AnimDuration = []float64{secs} })},
		"animation-delay":      {setOf(PropAnimDelay), animTimeInto(func(v *Values, secs float64) { v.AnimDelay = []float64{secs} })},
		"animation-direction":  {setOf(PropAnimDirection), parseAnimationDirection},
		"animation-fill-mode":  {setOf(PropAnimFill), parseAnimationFillMode},
		"animation-play-state": {setOf(PropAnimationPlayState), parseAnimationPlayState},
		"-gtk-icon-transform":  {setOf(PropIconTransform), parseIconTransform},
		"-gtk-icon-source":     {setOf(PropIconSource), parseIconSource},
		"-gtk-icon-palette":    {setOf(PropIconPalette), parseIconPalette},
		"caret-color":          {setOf(PropCaretColor), parseCaretColor},
		"text-decoration":      {setOf(PropTextDecoration), parseTextDecoration},
	}
}

var (
	borderWidths = setOf(PropBorderTopWidth, PropBorderRightWidth, PropBorderBottomWidth, PropBorderLeftWidth)
	borderStyles = setOf(PropBorderTopStyle, PropBorderRightStyle, PropBorderBottomStyle, PropBorderLeftStyle)
	borderColors = setOf(PropBorderTopColor, PropBorderRightColor, PropBorderBottomColor, PropBorderLeftColor)
	borderRadii  = setOf(PropBorderTopLeftRadius, PropBorderTopRightRadius, PropBorderBottomRightRadius,
		PropBorderBottomLeftRadius)
	transitionProps = setOf(PropTransitionProperty, PropTransitionDuration, PropTransitionTiming, PropTransitionDelay)
	animationProps  = setOf(PropAnimation, PropAnimationPlayState)
)

// animTimeInto applies a time longhand across the computed animations.
func animTimeInto(set func(v *Values, secs float64)) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		secs, ok := timeOf(components(splitTop(ts, tkComma)[0])[0], cx)
		if !ok {
			return false
		}
		set(v, secs)
		return true
	}
}

// colorInto parses a single color into the slot the getter returns.
func colorInto(get func(v *Values) *render.Color) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		cv, ok := parseColor(ts, cx)
		if !ok {
			return false
		}
		*get(v) = cx.resolve(cv)
		return true
	}
}

// lengthInto parses one length into an int field.
func lengthInto(get func(v *Values) *int, negative bool) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		comps := components(ts)
		if len(comps) != 1 {
			return false
		}
		l, ok := lengthOf(comps[0], cx)
		if !ok || l < 0 && !negative {
			return false
		}
		*get(v) = px(l)
		return true
	}
}

// sideInto parses one side length (padding-top, margin-left, ...).
func sideInto(get func(v *Values) *int, negative bool) func([]token, *ctx, *Values) bool {
	return lengthInto(get, negative)
}

// cornerInto parses one corner radius (a second, elliptical radius is
// accepted and dropped: corners are circular here).
func cornerInto(get func(v *Values) *int) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		comps := components(ts)
		if len(comps) < 1 || len(comps) > 2 {
			return false
		}
		r, ok := radiusOf(comps[0], cx)
		if !ok {
			return false
		}
		*get(v) = r
		return true
	}
}

// radiusOf reads a corner radius; a percentage has no box to resolve
// against at compute time, so 50% (the pill idiom) maps to a very large
// radius the painters clamp to half the short side, and other
// percentages are rejected.
func radiusOf(ts []token, cx *ctx) (int, bool) {
	n, ok := evalNumeric(ts, cx)
	if !ok {
		return 0, false
	}
	switch n.kind {
	case numLength, numNumber:
		if n.v < 0 {
			return 0, false
		}
		return px(n.v), true
	case numPercent:
		if n.v >= 50 {
			return 1 << 14, true
		}
	}
	return 0, false
}

// expandSides applies the CSS 1-to-4 value expansion.
func expandSides(vals []int) (Sides, bool) {
	switch len(vals) {
	case 1:
		return Sides{vals[0], vals[0], vals[0], vals[0]}, true
	case 2:
		return Sides{vals[0], vals[1], vals[0], vals[1]}, true
	case 3:
		return Sides{vals[0], vals[1], vals[2], vals[1]}, true
	case 4:
		return Sides{vals[0], vals[1], vals[2], vals[3]}, true
	}
	return Sides{}, false
}

// sidesInto parses a 1-4 length shorthand (padding, margin,
// border-width).
func sidesInto(get func(v *Values) *Sides, negative bool) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		var vals []int
		for _, c := range components(ts) {
			l, ok := lengthOf(c, cx)
			if !ok || l < 0 && !negative {
				return false
			}
			vals = append(vals, px(l))
		}
		s, ok := expandSides(vals)
		if !ok {
			return false
		}
		*get(v) = s
		return true
	}
}

// parseBorderRadius parses 1-4 radii (an elliptical `/` part is
// accepted and ignored).
func parseBorderRadius(ts []token, cx *ctx, v *Values) bool {
	var vals []int
	for _, c := range components(ts) {
		if len(c) == 1 && c[0].is('/') {
			break
		}
		r, ok := radiusOf(c, cx)
		if !ok {
			return false
		}
		vals = append(vals, r)
	}
	s, ok := expandSides(vals)
	if !ok {
		return false
	}
	v.Radius = Corners{TopLeft: s.Top, TopRight: s.Right, BottomRight: s.Bottom, BottomLeft: s.Left}
	return true
}

// borderStyleOf parses a line style keyword.
func borderStyleOf(ts []token) (BorderStyle, bool) {
	if len(ts) != 1 || ts[0].kind != tkIdent {
		return 0, false
	}
	switch strings.ToLower(ts[0].s) {
	case "none":
		return BorderNone, true
	case "hidden":
		return BorderHidden, true
	case "solid":
		return BorderSolid, true
	case "dashed":
		return BorderDashed, true
	case "dotted":
		return BorderDotted, true
	case "double":
		return BorderDouble, true
	case "groove":
		return BorderGroove, true
	case "ridge":
		return BorderRidge, true
	case "inset":
		return BorderInset, true
	case "outset":
		return BorderOutset, true
	}
	return 0, false
}

func borderStyleInto(side int) func([]token, *ctx, *Values) bool {
	return func(ts []token, _ *ctx, v *Values) bool {
		comps := components(ts)
		if len(comps) != 1 {
			return false
		}
		st, ok := borderStyleOf(comps[0])
		if !ok {
			return false
		}
		v.BorderStyle[side] = st
		return true
	}
}

func parseBorderStyles(ts []token, _ *ctx, v *Values) bool {
	var vals []int
	for _, c := range components(ts) {
		st, ok := borderStyleOf(c)
		if !ok {
			return false
		}
		vals = append(vals, int(st))
	}
	s, ok := expandSides(vals)
	if !ok {
		return false
	}
	v.BorderStyle = [4]BorderStyle{BorderStyle(s.Top), BorderStyle(s.Right), BorderStyle(s.Bottom), BorderStyle(s.Left)}
	return true
}

func parseBorderColors(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) < 1 || len(comps) > 4 {
		return false
	}
	var cs [4]render.Color
	for i, c := range comps {
		cv, ok := parseColor(c, cx)
		if !ok {
			return false
		}
		cs[i] = cx.resolve(cv)
	}
	switch len(comps) {
	case 1:
		cs[1], cs[2], cs[3] = cs[0], cs[0], cs[0]
	case 2:
		cs[2], cs[3] = cs[0], cs[1]
	case 3:
		cs[3] = cs[1]
	}
	v.BorderColor = cs
	return true
}

// lineParts parses the `<width> || <style> || <color>` grammar of the
// border and outline shorthands. Omitted parts take their initial
// values: width 0 (GTK's initial border width), style none, color
// currentColor.
func lineParts(ts []token, cx *ctx) (width int, style BorderStyle, color render.Color, ok bool) {
	color = cx.cur()
	var haveW, haveS, haveC bool
	for _, c := range components(ts) {
		if !haveS {
			if st, ok := borderStyleOf(c); ok {
				style, haveS = st, true
				continue
			}
		}
		if !haveW {
			if l, ok := lengthOf(c, cx); ok && l >= 0 {
				width, haveW = px(l), true
				continue
			}
			if len(c) == 1 && c[0].kind == tkIdent {
				switch strings.ToLower(c[0].s) {
				case "thin":
					width, haveW = 1, true
					continue
				case "medium":
					width, haveW = 3, true
					continue
				case "thick":
					width, haveW = 5, true
					continue
				}
			}
		}
		if !haveC {
			if cv, ok := parseColor(c, cx); ok {
				color, haveC = cx.resolve(cv), true
				continue
			}
		}
		return 0, 0, 0, false
	}
	return width, style, color, haveW || haveS || haveC
}

// borderSideInto parses the border / border-<side> shorthands; mask
// selects the sides (bit 0 top, clockwise).
func borderSideInto(mask int) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		w, st, col, ok := lineParts(ts, cx)
		if !ok {
			return false
		}
		widths := [4]*int{&v.BorderWidth.Top, &v.BorderWidth.Right, &v.BorderWidth.Bottom, &v.BorderWidth.Left}
		for i := range 4 {
			if mask&(1<<i) == 0 {
				continue
			}
			*widths[i] = w
			v.BorderStyle[i] = st
			v.BorderColor[i] = col
		}
		return true
	}
}

func parseOutline(ts []token, cx *ctx, v *Values) bool {
	w, st, col, ok := lineParts(ts, cx)
	if !ok {
		return false
	}
	v.OutlineWidth, v.OutlineStyle, v.OutlineColor = w, st, col
	return true
}

func outlineStyleInto(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	st, ok := borderStyleOf(comps[0])
	if !ok {
		return false
	}
	v.OutlineStyle = st
	return true
}

// parseBorderSpacing parses one or two lengths: horizontal, vertical.
func parseBorderSpacing(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) < 1 || len(comps) > 2 {
		return false
	}
	h, ok := lengthOf(comps[0], cx)
	if !ok || h < 0 {
		return false
	}
	vv := h
	if len(comps) == 2 {
		if vv, ok = lengthOf(comps[1], cx); !ok || vv < 0 {
			return false
		}
	}
	v.BorderSpacingH, v.BorderSpacingV = px(h), px(vv)
	return true
}

// parseOpacity parses a number or percentage, clamped to [0,1].
func parseOpacity(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok {
		return false
	}
	switch n.kind {
	case numNumber:
	case numPercent:
		n.v /= 100
	default:
		return false
	}
	v.Opacity = math.Min(1, math.Max(0, n.v))
	return true
}

// parseFilter parses `none` or a list of brightness()/opacity()
// functions; brightness multiplies, opacity folds into the brightness
// channel's companion, Values.Opacity is left alone (CSS applies the
// filter's opacity on top of the property). Other filter functions are
// rejected.
func parseFilter(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) == 1 && comps[0][0].ident("none") {
		v.Brightness = 1
		return true
	}
	b := 1.0
	for _, c := range comps {
		if c[0].kind != tkFunc || c[0].s != "brightness" {
			return false
		}
		n, ok := evalNumeric(funcArgs(c), cx)
		if !ok {
			return false
		}
		switch n.kind {
		case numNumber:
		case numPercent:
			n.v /= 100
		default:
			return false
		}
		if n.v < 0 {
			return false
		}
		b *= n.v
	}
	v.Brightness = b
	return len(comps) > 0
}

// parseBackground parses the background shorthand: at most one color
// and at most one image (none or a linear gradient). Position, size,
// and repeat keywords are out of the subset and reject the declaration.
func parseBackground(ts []token, cx *ctx, v *Values) bool {
	v.Background = 0
	v.Image = Gradient{}
	v.BgImageURL = ""
	var haveC, haveI bool
	for _, c := range components(ts) {
		if !haveI {
			if c[0].ident("none") {
				haveI = true
				continue
			}
			if u, ok := backgroundImageURL(c); ok {
				v.BgImageURL, haveI = u, true
				continue
			}
			if g, ok := gradientOf(c, cx); ok {
				v.Image, haveI = g, true
				continue
			}
		}
		if !haveC {
			if cv, ok := parseColor(c, cx); ok {
				v.Background, haveC = cx.resolve(cv), true
				continue
			}
		}
		return false
	}
	return haveC || haveI
}

func parseBackgroundImage(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	if comps[0][0].ident("none") {
		v.Image = Gradient{}
		v.BgImageURL = ""
		return true
	}
	if u, ok := backgroundImageURL(comps[0]); ok {
		v.Image = Gradient{}
		v.BgImageURL = u
		return true
	}
	g, ok := gradientOf(comps[0], cx)
	if !ok {
		return false
	}
	v.Image = g
	v.BgImageURL = ""
	return true
}

// backgroundImageURL parses url(...) — a quoted string — to a local
// file path: a file:// URL strips the scheme, anything else is the
// path itself. gelm loads no network images; a bare (unquoted) path
// tokenizes piecemeal and is not accepted, the way producers write
// quoted urls anyway.
func backgroundImageURL(ts []token) (string, bool) {
	if len(ts) == 0 || ts[0].kind != tkFunc || ts[0].s != "url" {
		return "", false
	}
	args := funcArgs(ts)
	if len(args) != 1 || args[0].kind != tkString {
		return "", false
	}
	path := strings.Trim(strings.TrimSpace(args[0].s), "\"'")
	path = strings.TrimPrefix(path, "file://")
	if path == "" {
		return "", false
	}
	return path, true
}

// gradientOf parses linear-gradient([<angle> | to <side-or-corner>,]
// <color> [<pos>]#). Corner directions use the square-box angle (45°
// multiples); positions are percentages, auto ones spread evenly
// between their neighbors.
func gradientOf(ts []token, cx *ctx) (Gradient, bool) {
	if len(ts) == 0 || ts[0].kind != tkFunc || ts[0].s != "linear-gradient" {
		return Gradient{}, false
	}
	args := splitTop(funcArgs(ts), tkComma)
	g := Gradient{Angle: 180}
	first := components(args[0])
	switch {
	case len(first) >= 2 && first[0][0].ident("to"):
		var dx, dy int
		for _, c := range first[1:] {
			switch {
			case c[0].ident("top"):
				dy = -1
			case c[0].ident("bottom"):
				dy = 1
			case c[0].ident("left"):
				dx = -1
			case c[0].ident("right"):
				dx = 1
			default:
				return Gradient{}, false
			}
		}
		g.Angle = math.Mod(math.Atan2(float64(dx), float64(-dy))*180/math.Pi+360, 360)
		args = args[1:]
	case len(first) == 1:
		if n, ok := evalNumeric(first[0], cx); ok && n.kind == numAngle {
			g.Angle = n.v
			args = args[1:]
		} else if ok && n.kind == numNumber && n.v == 0 {
			g.Angle = 0
			args = args[1:]
		}
	}
	if len(args) < 2 || len(args) > MaxStops {
		return Gradient{}, false
	}
	pos := make([]float64, len(args))
	for i, a := range args {
		comps := components(a)
		if len(comps) < 1 || len(comps) > 2 {
			return Gradient{}, false
		}
		cv, ok := parseColor(comps[0], cx)
		if !ok {
			return Gradient{}, false
		}
		g.Stops[i].Color = cx.resolve(cv)
		pos[i] = math.NaN()
		if len(comps) == 2 {
			n, ok := evalNumeric(comps[1], cx)
			if !ok || n.kind != numPercent {
				return Gradient{}, false
			}
			pos[i] = n.v / 100
		}
	}
	if math.IsNaN(pos[0]) {
		pos[0] = 0
	}
	if last := len(pos) - 1; math.IsNaN(pos[last]) {
		pos[last] = 1
	}
	for i := 1; i < len(pos); i++ {
		if !math.IsNaN(pos[i]) {
			pos[i] = math.Max(pos[i], pos[i-1]) // stops never move backwards
			continue
		}
		j := i
		for math.IsNaN(pos[j]) {
			j++
		}
		step := (pos[j] - pos[i-1]) / float64(j-i+1)
		for k := i; k < j; k++ {
			pos[k] = pos[k-1] + step
		}
	}
	for i := range pos {
		g.Stops[i].Pos = pos[i]
	}
	g.N = len(args)
	return g, true
}

// parseBoxShadow parses `none` or a comma list of shadows: `inset? <x>
// <y> [<blur> [<spread>]] <color>?`, in any order of the inset keyword
// and the color relative to the lengths. An omitted color is
// currentColor.
func parseBoxShadow(ts []token, cx *ctx, v *Values) bool {
	v.Shadow = Shadows{}
	layers := splitTop(ts, tkComma)
	if len(layers) == 1 {
		if comps := components(layers[0]); len(comps) == 1 && comps[0][0].ident("none") {
			return true
		}
	}
	for _, layer := range layers {
		var sh Shadow
		sh.Color = cx.cur()
		var lens []int
		haveC := false
		for _, c := range components(layer) {
			if c[0].ident("inset") && !sh.Inset {
				sh.Inset = true
				continue
			}
			if l, ok := lengthOf(c, cx); ok {
				lens = append(lens, px(l))
				continue
			}
			if !haveC {
				if cv, ok := parseColor(c, cx); ok {
					sh.Color, haveC = cx.resolve(cv), true
					continue
				}
			}
			return false
		}
		switch len(lens) {
		case 1:
			// gelm's pre-CSS-3 one-shadow form, `COLOR BLUR`: a centered
			// glow. Kept for existing stylesheets; standard CSS always
			// carries two offsets.
			if !haveC || lens[0] < 0 || len(layers) != 1 {
				return false
			}
			sh.Blur = lens[0]
		case 4:
			sh.Spread = lens[3]
			fallthrough
		case 3:
			if lens[2] < 0 {
				return false
			}
			sh.Blur = lens[2]
			fallthrough
		case 2:
			sh.X, sh.Y = lens[0], lens[1]
		default:
			return false
		}
		if v.Shadow.N == MaxShadows {
			warnf("box-shadow: layers past %d dropped", MaxShadows)
			break
		}
		v.Shadow.Layers[v.Shadow.N] = sh
		v.Shadow.N++
	}
	return true
}

// parseFontFamily keeps the head of the family list: the face the
// consumer shapes with (fallback runs are the font store's job).
func parseFontFamily(ts []token, _ *ctx, v *Values) bool {
	fams := splitTop(ts, tkComma)
	var head string
	for i, f := range fams {
		f = trimWS(f)
		if len(f) == 0 {
			return false
		}
		var name string
		if len(f) == 1 && f[0].kind == tkString {
			name = f[0].s
		} else {
			parts := make([]string, 0, len(f))
			for _, t := range f {
				switch t.kind {
				case tkIdent:
					parts = append(parts, t.s)
				case tkWS:
				default:
					return false
				}
			}
			name = strings.Join(parts, " ")
		}
		if i == 0 {
			head = name
		}
	}
	if head == "" {
		return false
	}
	v.FontFamily = head
	return true
}

// parseFontSize parses a length (em against the parent's size) or a
// keyword; percentages scale the parent's size.
func parseFontSize(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	if comps[0][0].kind == tkIdent {
		scale, ok := map[string]float64{
			"xx-small": 3.0 / 5, "x-small": 3.0 / 4, "small": 8.0 / 9, "medium": 1,
			"large": 6.0 / 5, "x-large": 3.0 / 2, "xx-large": 2, "smaller": 1 / 1.2, "larger": 1.2,
		}[strings.ToLower(comps[0][0].s)]
		if !ok {
			return false
		}
		cx.usedRem = true
		base := cx.rem
		if s := strings.ToLower(comps[0][0].s); s == "smaller" || s == "larger" {
			base = cx.emPx()
		}
		v.FontSize = base * scale
		return true
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok {
		return false
	}
	switch n.kind {
	case numLength, numNumber:
	case numPercent:
		n.v = cx.emPx() * n.v / 100
	default:
		return false
	}
	if n.v <= 0 || n.v > 1<<10 {
		return false
	}
	v.FontSize = n.v
	return true
}

// parseLineHeight parses `normal` (0, the font's own line box) or a
// length, percentage, or unitless number resolved against the computed
// font-size into pixels.
func parseLineHeight(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	if comps[0][0].ident("normal") {
		v.LineHeight = 0
		return true
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok {
		return false
	}
	switch n.kind {
	case numNumber:
		n.v = cx.emPx() * n.v
	case numPercent:
		n.v = cx.emPx() * n.v / 100
	case numLength:
	default:
		return false
	}
	if n.v < 0 || n.v > 1<<14 {
		return false
	}
	v.LineHeight = n.v
	return true
}

// parseTransform parses the full 2-D transform list: none, matrix,
// translate / translateX / translateY, scale / scaleX / scaleY,
// rotate, and skew / skewX / skewY, lengths resolving vars against the
// computed font size and angles taking deg, grad, rad, and turn. The
// functions compose left to right into the affine the widget paints
// through - exactly GTK's own set, lengths and all, since GTK CSS
// accepts no percentages in transforms.
func parseTransform(ts []token, cx *ctx, v *Values) bool {
	x, ok := composeTransform(ts, cx)
	if !ok {
		return false
	}
	v.Transform = x
	return true
}

// composeTransform walks one transform property's function list into
// an Xform: the composed affine plus the primitive list the tweens
// interpolate function by function. Each function maps points through
// the ones before it, the CSS order, so the composition is left
// multiplication.
func composeTransform(ts []token, cx *ctx) (Xform, bool) {
	x := XformIdentity
	comps := components(ts)
	if len(comps) == 1 && comps[0][0].ident("none") {
		return x, true
	}
	for _, comp := range comps {
		if len(comp) == 0 || comp[0].kind != tkFunc {
			return Xform{}, false
		}
		affine, part, ok := transformPrim(comp[0].s, splitTop(funcArgs(comp), tkComma), cx)
		if !ok {
			return Xform{}, false
		}
		x.M = x.M.Mul(affine)
		x.Parts = append(x.Parts, part)
	}
	return x, true
}

// transformPrim builds one transform function's affine: the name (the
// tokenizer lowercases function names) over its comma-separated args.
// Length and number args resolve per slot; angle args are the
// functions' own.
func transformPrim(name string, args [][]token, cx *ctx) (render.Affine, XformPart, bool) {
	num := func(i int) (float64, bool) {
		if i >= len(args) {
			return 0, false
		}
		comps := components(args[i])
		if len(comps) != 1 {
			return 0, false
		}
		n, ok := evalNumeric(comps[0], cx)
		if !ok {
			return 0, false
		}
		switch n.kind {
		case numLength, numNumber:
		default:
			return 0, false
		}
		return n.v, true
	}
	angle := func(i int) (float64, bool) {
		if i >= len(args) {
			return 0, false
		}
		comps := components(args[i])
		if len(comps) != 1 {
			return 0, false
		}
		return angleOf(comps[0][0])
	}
	switch name {
	case "scale":
		if len(args) == 0 || len(args) > 2 {
			return render.Affine{}, XformPart{}, false
		}
		sx, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		sy := sx
		if len(args) == 2 {
			if sy, ok = num(1); !ok {
				return render.Affine{}, XformPart{}, false
			}
		}
		return render.Scale(sx, sy), XformPart{Op: XformScale, N: [6]float64{sx, sy}}, true
	case "scalex":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		sx, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Scale(sx, 1), XformPart{Op: XformScale, N: [6]float64{sx, 1}}, true
	case "scaley":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		sy, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Scale(1, sy), XformPart{Op: XformScale, N: [6]float64{1, sy}}, true
	case "translate":
		if len(args) == 0 || len(args) > 2 {
			return render.Affine{}, XformPart{}, false
		}
		dx, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		dy := 0.0
		if len(args) == 2 {
			if dy, ok = num(1); !ok {
				return render.Affine{}, XformPart{}, false
			}
		}
		return render.Translate(dx, dy), XformPart{Op: XformTranslate, N: [6]float64{dx, dy}}, true
	case "translatex":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		dx, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Translate(dx, 0), XformPart{Op: XformTranslate, N: [6]float64{dx, 0}}, true
	case "translatey":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		dy, ok := num(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Translate(0, dy), XformPart{Op: XformTranslate, N: [6]float64{0, dy}}, true
	case "rotate":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		deg, ok := angle(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return render.Rotate(deg), XformPart{Op: XformRotate, N: [6]float64{deg}}, true
	case "skew":
		if len(args) == 0 || len(args) > 2 {
			return render.Affine{}, XformPart{}, false
		}
		x, ok := angle(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		y := 0.0
		if len(args) == 2 {
			if y, ok = angle(1); !ok {
				return render.Affine{}, XformPart{}, false
			}
		}
		return skewAffine(x, y), XformPart{Op: XformSkew, N: [6]float64{x, y}}, true
	case "skewx":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		x, ok := angle(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return skewAffine(x, 0), XformPart{Op: XformSkew, N: [6]float64{x, 0}}, true
	case "skewy":
		if len(args) != 1 {
			return render.Affine{}, XformPart{}, false
		}
		y, ok := angle(0)
		if !ok {
			return render.Affine{}, XformPart{}, false
		}
		return skewAffine(0, y), XformPart{Op: XformSkew, N: [6]float64{0, y}}, true
	case "matrix":
		if len(args) != 6 {
			return render.Affine{}, XformPart{}, false
		}
		var nums [6]float64
		for i := range nums {
			var ok bool
			if nums[i], ok = num(i); !ok {
				return render.Affine{}, XformPart{}, false
			}
		}
		return render.Affine{A: nums[0], B: nums[1], C: nums[2], D: nums[3], E: nums[4], F: nums[5]}, XformPart{Op: XformMatrix, N: nums}, true
	}
	return render.Affine{}, XformPart{}, false
}

// skewAffine is the shear by the two angles: x leans by tan(ax), y by
// tan(ay).
func skewAffine(ax, ay float64) render.Affine {
	return render.Affine{
		A: 1,
		B: math.Tan(ay * math.Pi / 180),
		C: math.Tan(ax * math.Pi / 180),
		D: 1,
	}
}

// parseTransformOrigin parses transform-origin: one or two of the
// edge keywords, lengths, and percentages, lengths resolving against
// the computed font size. One value centers the other axis; keywords
// land in either order (left top == top left).
func parseTransformOrigin(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) == 0 || len(comps) > 2 {
		return false
	}
	var fx, fy, px, py float64
	seenX, seenY := false, false
	for _, comp := range comps {
		t := comp[0]
		if len(comp) != 1 {
			return false
		}
		if t.kind == tkIdent {
			switch strings.ToLower(t.s) {
			case "left", "right":
				if seenX {
					return false
				}
				seenX = true
				if strings.EqualFold(t.s, "left") {
					fx = 0
				} else {
					fx = 1
				}
				continue
			case "top", "bottom":
				if seenY {
					return false
				}
				seenY = true
				if strings.EqualFold(t.s, "top") {
					fy = 0
				} else {
					fy = 1
				}
				continue
			case "center":
				if !seenX {
					seenX, fx = true, 0.5
				} else if !seenY {
					seenY, fy = true, 0.5
				} else {
					return false
				}
				continue
			}
			return false
		}
		n, ok := evalNumeric(comp, cx)
		if !ok {
			return false
		}
		switch n.kind {
		case numLength:
			if !seenX {
				seenX, px = true, n.v
			} else if !seenY {
				seenY, py = true, n.v
			} else {
				return false
			}
		case numPercent:
			f := n.v / 100
			if !seenX {
				seenX, fx = true, f
			} else if !seenY {
				seenY, fy = true, f
			} else {
				return false
			}
		default:
			return false
		}
	}
	if !seenY {
		fy = 0.5
	}
	v.OriginFrac, v.OriginPx = [2]float64{fx, fy}, [2]float64{px, py}
	return true
}

// parseFontVariations parses `normal` or the axis list: each "tag" (a
// quoted four-character axis tag) and its number. The list lands in
// canonical sorted `tag=value` form so the comparable Values can hold
// it.
func parseFontVariations(ts []token, _ *ctx, v *Values) bool {
	if first := components(ts)[0]; first[0].ident("normal") {
		v.Variations = ""
		return true
	}
	var parts []string
	for _, comp := range splitTop(ts, tkComma) {
		comps := components(comp)
		if len(comps) != 2 {
			return false
		}
		tag := strings.Trim(comps[0][0].s, "\"'")
		if len(tag) != 4 {
			return false
		}
		n, ok := evalNumeric(comps[1], &ctx{})
		if !ok || n.kind != numNumber {
			return false
		}
		parts = append(parts, fmt.Sprintf("%s=%g", tag, n.v))
	}
	slices.Sort(parts)
	v.Variations = strings.Join(parts, ";")
	return true
}

// parseFontFeatures parses `normal` or the feature-tag list: each
// "tag" (a quoted or bare four-character tag), with an optional value
// (a number, or on/off) defaulting to 1. The list lands in canonical
// sorted `tag=value` form so the comparable Values can hold it.
func parseFontFeatures(ts []token, _ *ctx, v *Values) bool {
	if first := components(ts)[0]; first[0].ident("normal") {
		v.Features = ""
		return true
	}
	type fv struct {
		tag string
		val int
	}
	var feats []fv
	for _, comp := range splitTop(ts, tkComma) {
		comps := components(comp)
		if len(comps) == 0 {
			return false
		}
		tag := strings.ToLower(strings.Trim(comps[0][0].s, "\"'"))
		if len(comps[0][0].s) < 3 || len(tag) == 0 || len(tag) > 4 {
			return false
		}
		val := 1
		if len(comps) > 1 {
			switch {
			case comps[1][0].ident("on"):
			case comps[1][0].ident("off"):
				val = 0
			default:
				n, ok := evalNumeric(comps[1], &ctx{})
				if !ok || n.kind != numNumber {
					return false
				}
				val = int(math.Round(n.v))
			}
		}
		feats = append(feats, fv{fmt.Sprintf("%-4s", tag), val})
	}
	slices.SortFunc(feats, func(a, b fv) int { return strings.Compare(a.tag, b.tag) })
	parts := make([]string, len(feats))
	for i, f := range feats {
		parts[i] = fmt.Sprintf("%s=%d", f.tag, f.val)
	}
	v.Features = strings.Join(parts, ";")
	return true
}

// parseFontWeight parses normal/bold/bolder/lighter or 1..1000.
func parseFontWeight(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	if comps[0][0].kind == tkIdent {
		switch strings.ToLower(comps[0][0].s) {
		case "normal":
			v.FontWeight = 400
		case "bold":
			v.FontWeight = 700
		case "bolder":
			v.FontWeight = min(900, cx.wt()+300)
		case "lighter":
			v.FontWeight = max(100, cx.wt()-300)
		default:
			return false
		}
		return true
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok || n.kind != numNumber || n.v < 1 || n.v > 1000 {
		return false
	}
	v.FontWeight = int(math.Round(n.v))
	return true
}

func parseFontStyle(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) < 1 || comps[0][0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(comps[0][0].s) {
	case "normal":
		v.Italic = false
	case "italic", "oblique":
		v.Italic = true
	default:
		return false
	}
	return true
}

// parseFont parses the font shorthand: [style] [weight] size family.
func parseFont(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	v.Italic, v.FontWeight = false, 400
	i := 0
	for ; i < len(comps); i++ {
		if parseFontStyle(comps[i], cx, v) && !comps[i][0].ident("normal") {
			continue
		}
		if parseFontWeight(comps[i], cx, v) {
			continue
		}
		break
	}
	if i >= len(comps) || !parseFontSize(comps[i], cx, v) {
		return false
	}
	// The family is everything after the size component.
	var fam []token
	for j, t := range ts {
		if t.pos >= comps[i][len(comps[i])-1].end {
			fam = ts[j:]
			break
		}
	}
	return parseFontFamily(trimWS(fam), cx, v)
}

func parseLetterSpacing(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	if comps[0][0].ident("normal") {
		v.LetterSpacing = 0
		return true
	}
	n, ok := evalNumeric(comps[0], cx)
	if !ok || n.kind != numLength && n.kind != numNumber {
		return false
	}
	v.LetterSpacing = n.v
	return true
}

func parseTextTransform(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 || comps[0][0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(comps[0][0].s) {
	case "none":
		v.TextTransform = TransformNone
	case "uppercase":
		v.TextTransform = TransformUppercase
	case "lowercase":
		v.TextTransform = TransformLowercase
	case "capitalize":
		v.TextTransform = TransformCapitalize
	default:
		return false
	}
	return true
}

// timeOf reads a time value in seconds.
func timeOf(ts []token, cx *ctx) (float64, bool) {
	n, ok := evalNumeric(ts, cx)
	if !ok || n.kind != numTime && !(n.kind == numNumber && n.v == 0) {
		return 0, false
	}
	return n.v, true
}

func timeInto(get func(v *Values) *float64) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		// Only the first entry of a list is kept: one transition per node.
		first := components(splitTop(ts, tkComma)[0])
		if len(first) != 1 {
			return false
		}
		t, ok := timeOf(first[0], cx)
		if !ok || t < 0 {
			return false
		}
		*get(v) = t
		return true
	}
}

// timingOf parses a timing function keyword or cubic-bezier()/steps().
func timingOf(ts []token, cx *ctx) (Timing, bool) {
	if ts[0].kind == tkIdent {
		switch strings.ToLower(ts[0].s) {
		case "linear":
			return Timing{0, 0, 1, 1, 0}, true
		case "ease":
			return Timing{0.25, 0.1, 0.25, 1, 0}, true
		case "ease-in":
			return Timing{0.42, 0, 1, 1, 0}, true
		case "ease-out":
			return Timing{0, 0, 0.58, 1, 0}, true
		case "ease-in-out":
			return Timing{0.42, 0, 0.58, 1, 0}, true
		}
		return Timing{}, false
	}
	if ts[0].kind != tkFunc {
		return Timing{}, false
	}
	args := splitTop(funcArgs(ts), tkComma)
	switch ts[0].s {
	case "cubic-bezier":
		if len(args) != 4 {
			return Timing{}, false
		}
		var f [4]float64
		for i, a := range args {
			n, ok := evalNumeric(a, cx)
			if !ok || n.kind != numNumber {
				return Timing{}, false
			}
			f[i] = n.v
		}
		return Timing{f[0], f[1], f[2], f[3], 0}, true
	case "steps":
		n, ok := evalNumeric(args[0], cx)
		if !ok || n.kind != numNumber || n.v < 1 {
			return Timing{}, false
		}
		return Timing{Steps: int(n.v)}, true
	}
	return Timing{}, false
}

func parseTransitionTiming(ts []token, cx *ctx, v *Values) bool {
	first := components(splitTop(ts, tkComma)[0])
	if len(first) != 1 {
		return false
	}
	t, ok := timingOf(first[0], cx)
	if !ok {
		return false
	}
	v.Transition.Timing = t
	return true
}

// transitionTarget maps a transition-property name to the longhands it
// animates; all and none are handled by the callers.
func transitionTarget(name string) (PropSet, bool) {
	name = strings.ToLower(name)
	if def, ok := propTable[name]; ok {
		return def.covers, true
	}
	if ignoredProps[name] {
		return 0, true
	}
	return 0, false
}

func parseTransitionProperty(ts []token, _ *ctx, v *Values) bool {
	v.Transition.All, v.Transition.Props = false, 0
	for _, item := range splitTop(ts, tkComma) {
		comps := components(item)
		if len(comps) != 1 || comps[0][0].kind != tkIdent {
			return false
		}
		name := comps[0][0].s
		switch strings.ToLower(name) {
		case "all":
			v.Transition.All = true
			continue
		case "none":
			continue
		}
		ps, ok := transitionTarget(name)
		if !ok {
			return false
		}
		v.Transition.Props |= ps
	}
	return true
}

// parseTransition parses the shorthand's comma list; each item is
// `[property] [duration] [timing] [delay]` in any order, the first time
// the duration and the second the delay.
func parseTransition(ts []token, cx *ctx, v *Values) bool {
	v.Transition = Transition{Timing: Timing{0.25, 0.1, 0.25, 1, 0}}
	for _, item := range splitTop(ts, tkComma) {
		var props PropSet
		all := false
		haveP, haveD, haveDelay, haveT := false, false, false, false
		var dur, delay float64
		timing := Timing{0.25, 0.1, 0.25, 1, 0}
		for _, c := range components(item) {
			if t, ok := timeOf(c, cx); ok {
				switch {
				case !haveD:
					dur, haveD = t, true
				case !haveDelay:
					delay, haveDelay = t, true
				default:
					return false
				}
				continue
			}
			if !haveT {
				if tm, ok := timingOf(c, cx); ok {
					timing, haveT = tm, true
					continue
				}
			}
			if !haveP && c[0].kind == tkIdent {
				switch strings.ToLower(c[0].s) {
				case "all":
					all, haveP = true, true
					continue
				case "none":
					haveP = true
					continue
				}
				if ps, ok := transitionTarget(c[0].s); ok {
					props, haveP = ps, true
					continue
				}
			}
			return false
		}
		if !haveP {
			all = true // the property defaults to all
		}
		v.Transition.All = v.Transition.All || all
		v.Transition.Props |= props
		// One duration per node: the longest wins, which is what a
		// multi-property transition looks like on screen.
		if dur > v.Transition.Duration {
			v.Transition.Duration = dur
			v.Transition.Delay = delay
			v.Transition.Timing = timing
		}
	}
	return true
}
