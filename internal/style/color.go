package style

import (
	"math"
	"strings"

	"github.com/stubbedev/gelm/render"
)

// rgbaf is a straight-alpha color in [0,1] floats, the working space for
// color functions; render.Color is premultiplied bytes.
type rgbaf struct{ r, g, b, a float64 }

func (c rgbaf) color() render.Color {
	clamp := func(x float64) float64 { return math.Min(1, math.Max(0, x)) }
	a := clamp(c.a)
	to := func(x float64) uint8 { return uint8(math.Round(clamp(x) * a * 255)) }
	return render.Color(uint32(math.Round(a*255))<<24 | uint32(to(c.r))<<16 | uint32(to(c.g))<<8 | uint32(to(c.b)))
}

// fromColor unpremultiplies a render.Color.
func fromColor(c render.Color) rgbaf {
	a := float64(c.A()) / 255
	if a == 0 {
		return rgbaf{}
	}
	return rgbaf{
		r: float64(c.R()) / 255 / a,
		g: float64(c.G()) / 255 / a,
		b: float64(c.B()) / 255 / a,
		a: a,
	}
}

// colorVal is a parsed color: a concrete value, or currentColor, which
// resolves against the node's computed color after the cascade.
type colorVal struct {
	c       render.Color
	current bool
}

// parseColor parses one color component: hex, a named color,
// transparent, currentColor, rgb()/rgba(), hsl()/hsla(), and
// color-mix(in srgb, ...). GTK's alpha(), shade(), and mix() are
// accepted too, since GTK stylesheets use them.
func parseColor(ts []token, cx *ctx) (colorVal, bool) {
	ts = trimWS(ts)
	if len(ts) == 0 {
		return colorVal{}, false
	}
	t := ts[0]
	switch t.kind {
	case tkHash:
		if len(ts) != 1 {
			return colorVal{}, false
		}
		c, ok := hexColor(t.s)
		return colorVal{c: c}, ok
	case tkIdent:
		if len(ts) != 1 {
			return colorVal{}, false
		}
		name := strings.ToLower(t.s)
		if name == "currentcolor" {
			return colorVal{current: true}, true
		}
		if name == "transparent" {
			return colorVal{}, true
		}
		c, ok := namedColors[name]
		return colorVal{c: c}, ok
	case tkFunc:
		if blockEnd(ts, 0) != len(ts) {
			return colorVal{}, false
		}
		args := funcArgs(ts)
		switch t.s {
		case "rgb", "rgba":
			c, ok := rgbFunc(args, cx)
			return colorVal{c: c}, ok
		case "hsl", "hsla":
			c, ok := hslFunc(args, cx)
			return colorVal{c: c}, ok
		case "color-mix":
			return colorMix(args, cx)
		case "alpha":
			return gtkAlpha(args, cx)
		case "shade":
			return gtkShade(args, cx)
		case "mix":
			return gtkMix(args, cx)
		}
	}
	return colorVal{}, false
}

// hexColor decodes #rgb, #rgba, #rrggbb, or #rrggbbaa (straight alpha)
// into a premultiplied color.
func hexColor(h string) (render.Color, bool) {
	nib := func(c byte) (uint8, bool) {
		switch {
		case c >= '0' && c <= '9':
			return c - '0', true
		case c >= 'a' && c <= 'f':
			return c - 'a' + 10, true
		case c >= 'A' && c <= 'F':
			return c - 'A' + 10, true
		}
		return 0, false
	}
	var ch [4]uint8
	ch[3] = 0xff
	switch len(h) {
	case 3, 4:
		for i := range len(h) {
			v, ok := nib(h[i])
			if !ok {
				return 0, false
			}
			ch[i] = v * 0x11
		}
	case 6, 8:
		for i := range len(h) / 2 {
			hi, ok1 := nib(h[2*i])
			lo, ok2 := nib(h[2*i+1])
			if !ok1 || !ok2 {
				return 0, false
			}
			ch[i] = hi<<4 | lo
		}
	default:
		return 0, false
	}
	return render.RGBA(ch[0], ch[1], ch[2], ch[3]), true
}

// funcNumbers splits color function arguments into numeric components,
// accepting both the legacy comma form and the space form with an
// optional `/ alpha`.
func funcNumbers(args []token, cx *ctx) ([]numeric, bool) {
	var parts [][]token
	if len(splitTop(args, tkComma)) > 1 {
		parts = splitTop(args, tkComma)
	} else {
		for _, c := range components(args) {
			if len(c) == 1 && c[0].is('/') {
				continue
			}
			parts = append(parts, c)
		}
	}
	out := make([]numeric, 0, len(parts))
	for _, p := range parts {
		n, ok := evalNumeric(trimWS(p), cx)
		if !ok {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// channel reads an rgb() channel: a number 0..255 or a percentage.
func channel(n numeric) (float64, bool) {
	switch n.kind {
	case numNumber:
		return n.v / 255, true
	case numPercent:
		return n.v / 100, true
	}
	return 0, false
}

// alphaOf reads an alpha component: a number 0..1 or a percentage.
func alphaOf(n numeric) (float64, bool) {
	switch n.kind {
	case numNumber:
		return n.v, true
	case numPercent:
		return n.v / 100, true
	}
	return 0, false
}

func rgbFunc(args []token, cx *ctx) (render.Color, bool) {
	ns, ok := funcNumbers(args, cx)
	if !ok || len(ns) < 3 || len(ns) > 4 {
		return 0, false
	}
	var c rgbaf
	c.a = 1
	var ok1, ok2, ok3 bool
	c.r, ok1 = channel(ns[0])
	c.g, ok2 = channel(ns[1])
	c.b, ok3 = channel(ns[2])
	if !ok1 || !ok2 || !ok3 {
		return 0, false
	}
	if len(ns) == 4 {
		a, ok := alphaOf(ns[3])
		if !ok {
			return 0, false
		}
		c.a = a
	}
	return c.color(), true
}

func hslFunc(args []token, cx *ctx) (render.Color, bool) {
	ns, ok := funcNumbers(args, cx)
	if !ok || len(ns) < 3 || len(ns) > 4 {
		return 0, false
	}
	h := ns[0].v
	if ns[0].kind == numAngle {
		h = ns[0].v // already degrees
	} else if ns[0].kind != numNumber {
		return 0, false
	}
	if ns[1].kind != numPercent || ns[2].kind != numPercent {
		return 0, false
	}
	s, l := ns[1].v/100, ns[2].v/100
	a := 1.0
	if len(ns) == 4 {
		v, ok := alphaOf(ns[3])
		if !ok {
			return 0, false
		}
		a = v
	}
	h = math.Mod(math.Mod(h, 360)+360, 360) / 360
	hue := func(p, q, t float64) float64 {
		t = math.Mod(t+1, 1)
		switch {
		case t < 1.0/6:
			return p + (q-p)*6*t
		case t < 0.5:
			return q
		case t < 2.0/3:
			return p + (q-p)*(2.0/3-t)*6
		}
		return p
	}
	var q float64
	if l < 0.5 {
		q = l * (1 + s)
	} else {
		q = l + s - l*s
	}
	p := 2*l - q
	return rgbaf{r: hue(p, q, h+1.0/3), g: hue(p, q, h), b: hue(p, q, h-1.0/3), a: a}.color(), true
}

// colorMix is CSS Color 5's color-mix(in srgb, A [p%], B [q%]): the
// percentages normalize to sum 100, the interpolation runs on
// premultiplied channels, and a sum under 100 scales the result's alpha.
// Any color space other than srgb (and srgb-linear, mixed linearly
// here too) is rejected.
func colorMix(args []token, cx *ctx) (colorVal, bool) {
	parts := splitTop(args, tkComma)
	if len(parts) != 3 {
		return colorVal{}, false
	}
	space := components(parts[0])
	if len(space) != 2 || !space[0][0].ident("in") ||
		(!space[1][0].ident("srgb") && !space[1][0].ident("srgb-linear")) {
		return colorVal{}, false
	}
	type side struct {
		c   colorVal
		pct float64
		has bool
	}
	parseSide := func(ts []token) (side, bool) {
		comps := components(ts)
		var s side
		var colorToks []token
		for _, c := range comps {
			if len(c) == 1 && c[0].kind == tkPercent {
				if s.has {
					return side{}, false
				}
				s.pct, s.has = c[0].num, true
				continue
			}
			if c[0].kind == tkFunc && c[0].s == "calc" {
				if n, ok := evalNumeric(c, cx); ok && n.kind == numPercent {
					if s.has {
						return side{}, false
					}
					s.pct, s.has = n.v, true
					continue
				}
			}
			if colorToks != nil {
				return side{}, false
			}
			colorToks = c
		}
		cv, ok := parseColor(colorToks, cx)
		if !ok {
			return side{}, false
		}
		s.c = cv
		return s, true
	}
	a, ok1 := parseSide(parts[1])
	b, ok2 := parseSide(parts[2])
	if !ok1 || !ok2 {
		return colorVal{}, false
	}
	// currentColor inside a mix resolves against the node's color, which
	// the context carries once the color property is computed.
	ca, cb := cx.resolve(a.c), cx.resolve(b.c)
	pa, pb := a.pct, b.pct
	switch {
	case !a.has && !b.has:
		pa, pb = 50, 50
	case !a.has:
		pa = 100 - pb
	case !b.has:
		pb = 100 - pa
	}
	if pa < 0 || pb < 0 {
		return colorVal{}, false
	}
	sum := pa + pb
	if sum == 0 {
		return colorVal{}, false
	}
	return colorVal{c: mixPremul(ca, cb, pa/sum, math.Min(sum, 100)/100)}, true
}

// mixPremul interpolates premultiplied channels: wa is the first color's
// weight; scale multiplies the result (color-mix's under-100% alpha).
func mixPremul(a, b render.Color, wa, scale float64) render.Color {
	var out uint32
	for shift := uint(0); shift <= 24; shift += 8 {
		ca := float64((uint32(a) >> shift) & 0xff)
		cb := float64((uint32(b) >> shift) & 0xff)
		v := math.Round((ca*wa + cb*(1-wa)) * scale)
		out |= uint32(math.Min(255, math.Max(0, v))) << shift
	}
	return render.Color(out)
}

// gtkAlpha is GTK's alpha(color, factor): the alpha scaled.
func gtkAlpha(args []token, cx *ctx) (colorVal, bool) {
	parts := splitTop(args, tkComma)
	if len(parts) != 2 {
		return colorVal{}, false
	}
	cv, ok := parseColor(parts[0], cx)
	n, ok2 := evalNumeric(trimWS(parts[1]), cx)
	if !ok || !ok2 || n.kind != numNumber {
		return colorVal{}, false
	}
	f := fromColor(cx.resolve(cv))
	f.a *= n.v
	return colorVal{c: f.color()}, true
}

// gtkShade is GTK's shade(color, factor): lightness scaled in HLS.
func gtkShade(args []token, cx *ctx) (colorVal, bool) {
	parts := splitTop(args, tkComma)
	if len(parts) != 2 {
		return colorVal{}, false
	}
	cv, ok := parseColor(parts[0], cx)
	n, ok2 := evalNumeric(trimWS(parts[1]), cx)
	if !ok || !ok2 || n.kind != numNumber {
		return colorVal{}, false
	}
	f := fromColor(cx.resolve(cv))
	h, l, s := toHLS(f)
	l = math.Min(1, math.Max(0, l*n.v))
	s = math.Min(1, math.Max(0, s*n.v))
	out := fromHLS(h, l, s)
	out.a = f.a
	return colorVal{c: out.color()}, true
}

// gtkMix is GTK's mix(a, b, factor): straight interpolation toward b.
func gtkMix(args []token, cx *ctx) (colorVal, bool) {
	parts := splitTop(args, tkComma)
	if len(parts) != 3 {
		return colorVal{}, false
	}
	a, ok1 := parseColor(parts[0], cx)
	b, ok2 := parseColor(parts[1], cx)
	n, ok3 := evalNumeric(trimWS(parts[2]), cx)
	if !ok1 || !ok2 || !ok3 || n.kind != numNumber {
		return colorVal{}, false
	}
	fa, fb := fromColor(cx.resolve(a)), fromColor(cx.resolve(b))
	t := n.v
	lerp := func(x, y float64) float64 { return x + (y-x)*t }
	return colorVal{c: rgbaf{lerp(fa.r, fb.r), lerp(fa.g, fb.g), lerp(fa.b, fb.b), lerp(fa.a, fb.a)}.color()}, true
}

func toHLS(c rgbaf) (h, l, s float64) {
	mx := math.Max(c.r, math.Max(c.g, c.b))
	mn := math.Min(c.r, math.Min(c.g, c.b))
	l = (mx + mn) / 2
	if mx == mn {
		return 0, l, 0
	}
	d := mx - mn
	if l <= 0.5 {
		s = d / (mx + mn)
	} else {
		s = d / (2 - mx - mn)
	}
	switch mx {
	case c.r:
		h = (c.g - c.b) / d
	case c.g:
		h = 2 + (c.b-c.r)/d
	default:
		h = 4 + (c.r-c.g)/d
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, l, s
}

func fromHLS(h, l, s float64) rgbaf {
	if s == 0 {
		return rgbaf{l, l, l, 1}
	}
	var m2 float64
	if l <= 0.5 {
		m2 = l * (1 + s)
	} else {
		m2 = l + s - l*s
	}
	m1 := 2*l - m2
	f := func(hue float64) float64 {
		hue = math.Mod(hue+360, 360)
		switch {
		case hue < 60:
			return m1 + (m2-m1)*hue/60
		case hue < 180:
			return m2
		case hue < 240:
			return m1 + (m2-m1)*(240-hue)/60
		}
		return m1
	}
	return rgbaf{f(h + 120), f(h), f(h - 120), 1}
}

// namedColors is the CSS named color table.
var namedColors = func() map[string]render.Color {
	src := map[string]uint32{
		"aliceblue": 0xf0f8ff, "antiquewhite": 0xfaebd7, "aqua": 0x00ffff, "aquamarine": 0x7fffd4,
		"azure": 0xf0ffff, "beige": 0xf5f5dc, "bisque": 0xffe4c4, "black": 0x000000,
		"blanchedalmond": 0xffebcd, "blue": 0x0000ff, "blueviolet": 0x8a2be2, "brown": 0xa52a2a,
		"burlywood": 0xdeb887, "cadetblue": 0x5f9ea0, "chartreuse": 0x7fff00, "chocolate": 0xd2691e,
		"coral": 0xff7f50, "cornflowerblue": 0x6495ed, "cornsilk": 0xfff8dc, "crimson": 0xdc143c,
		"cyan": 0x00ffff, "darkblue": 0x00008b, "darkcyan": 0x008b8b, "darkgoldenrod": 0xb8860b,
		"darkgray": 0xa9a9a9, "darkgreen": 0x006400, "darkgrey": 0xa9a9a9, "darkkhaki": 0xbdb76b,
		"darkmagenta": 0x8b008b, "darkolivegreen": 0x556b2f, "darkorange": 0xff8c00, "darkorchid": 0x9932cc,
		"darkred": 0x8b0000, "darksalmon": 0xe9967a, "darkseagreen": 0x8fbc8f, "darkslateblue": 0x483d8b,
		"darkslategray": 0x2f4f4f, "darkslategrey": 0x2f4f4f, "darkturquoise": 0x00ced1, "darkviolet": 0x9400d3,
		"deeppink": 0xff1493, "deepskyblue": 0x00bfff, "dimgray": 0x696969, "dimgrey": 0x696969,
		"dodgerblue": 0x1e90ff, "firebrick": 0xb22222, "floralwhite": 0xfffaf0, "forestgreen": 0x228b22,
		"fuchsia": 0xff00ff, "gainsboro": 0xdcdcdc, "ghostwhite": 0xf8f8ff, "gold": 0xffd700,
		"goldenrod": 0xdaa520, "gray": 0x808080, "green": 0x008000, "greenyellow": 0xadff2f,
		"grey": 0x808080, "honeydew": 0xf0fff0, "hotpink": 0xff69b4, "indianred": 0xcd5c5c,
		"indigo": 0x4b0082, "ivory": 0xfffff0, "khaki": 0xf0e68c, "lavender": 0xe6e6fa,
		"lavenderblush": 0xfff0f5, "lawngreen": 0x7cfc00, "lemonchiffon": 0xfffacd, "lightblue": 0xadd8e6,
		"lightcoral": 0xf08080, "lightcyan": 0xe0ffff, "lightgoldenrodyellow": 0xfafad2, "lightgray": 0xd3d3d3,
		"lightgreen": 0x90ee90, "lightgrey": 0xd3d3d3, "lightpink": 0xffb6c1, "lightsalmon": 0xffa07a,
		"lightseagreen": 0x20b2aa, "lightskyblue": 0x87cefa, "lightslategray": 0x778899, "lightslategrey": 0x778899,
		"lightsteelblue": 0xb0c4de, "lightyellow": 0xffffe0, "lime": 0x00ff00, "limegreen": 0x32cd32,
		"linen": 0xfaf0e6, "magenta": 0xff00ff, "maroon": 0x800000, "mediumaquamarine": 0x66cdaa,
		"mediumblue": 0x0000cd, "mediumorchid": 0xba55d3, "mediumpurple": 0x9370db, "mediumseagreen": 0x3cb371,
		"mediumslateblue": 0x7b68ee, "mediumspringgreen": 0x00fa9a, "mediumturquoise": 0x48d1cc, "mediumvioletred": 0xc71585,
		"midnightblue": 0x191970, "mintcream": 0xf5fffa, "mistyrose": 0xffe4e1, "moccasin": 0xffe4b5,
		"navajowhite": 0xffdead, "navy": 0x000080, "oldlace": 0xfdf5e6, "olive": 0x808000,
		"olivedrab": 0x6b8e23, "orange": 0xffa500, "orangered": 0xff4500, "orchid": 0xda70d6,
		"palegoldenrod": 0xeee8aa, "palegreen": 0x98fb98, "paleturquoise": 0xafeeee, "palevioletred": 0xdb7093,
		"papayawhip": 0xffefd5, "peachpuff": 0xffdab9, "peru": 0xcd853f, "pink": 0xffc0cb,
		"plum": 0xdda0dd, "powderblue": 0xb0e0e6, "purple": 0x800080, "rebeccapurple": 0x663399,
		"red": 0xff0000, "rosybrown": 0xbc8f8f, "royalblue": 0x4169e1, "saddlebrown": 0x8b4513,
		"salmon": 0xfa8072, "sandybrown": 0xf4a460, "seagreen": 0x2e8b57, "seashell": 0xfff5ee,
		"sienna": 0xa0522d, "silver": 0xc0c0c0, "skyblue": 0x87ceeb, "slateblue": 0x6a5acd,
		"slategray": 0x708090, "slategrey": 0x708090, "snow": 0xfffafa, "springgreen": 0x00ff7f,
		"steelblue": 0x4682b4, "tan": 0xd2b48c, "teal": 0x008080, "thistle": 0xd8bfd8,
		"tomato": 0xff6347, "turquoise": 0x40e0d0, "violet": 0xee82ee, "wheat": 0xf5deb3,
		"white": 0xffffff, "whitesmoke": 0xf5f5f5, "yellow": 0xffff00, "yellowgreen": 0x9acd32,
	}
	out := make(map[string]render.Color, len(src))
	for k, v := range src {
		out[k] = render.Color(0xff000000 | v)
	}
	return out
}()
