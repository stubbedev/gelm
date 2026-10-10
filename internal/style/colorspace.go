package style

import (
	"math"
	"strings"

	"github.com/stubbedev/gelm/render"
)

// space is one CSS Color 4 interpolation space: the component layout
// color-mix() interpolates in and the color functions construct with.
type space uint8

// The interpolation spaces. Polar spaces carry a hue angle in the
// channel hueIdx; hsl/hwb hold it first, lch/oklch third.
const (
	spSrgb space = iota
	spSrgbLinear
	spLab
	spLch
	spOklab
	spOklch
	spHsl
	spHwb
)

// hueIdx is the hue channel's index per space, -1 when rectangular.
var hueIdx = [8]int{-1, -1, -1, 2, -1, 2, 0, 0}

// mixSpace is CSS Color 5's color-mix() arithmetic generalized over
// every interpolation space: both colors convert to the space, the
// channels interpolate premultiplied (hue channels interpolate by the
// named hue method), and a percentage sum under 100 scales the alpha.
func mixSpace(a, b rgbaf, wa float64, sp space, hue hueMethod, scale float64) rgbaf {
	ca, cb := toComps(a, sp), toComps(b, sp)
	var out [4]float64
	out[3] = ca[3]*wa + cb[3]*(1-wa)
	for i := range 3 {
		if i == hueIdx[sp] {
			continue
		}
		pa, pb := ca[i]*ca[3], cb[i]*cb[3]
		v := pa*wa + pb*(1-wa)
		if out[3] > 0 {
			v /= out[3]
		}
		out[i] = v
	}
	if hi := hueIdx[sp]; hi >= 0 && ca[3] > 0 && cb[3] > 0 {
		out[hi] = interpHue(ca[hi], cb[hi], 1-wa, hue)
	}
	out[3] *= scale
	out[3] = math.Min(1, math.Max(0, out[3]))
	return fromComps(out, sp)
}

// interpHue interpolates two hue angles by t toward h2 under the CSS
// Color 5 hue interpolation method.
func interpHue(h1, h2, t float64, m hueMethod) float64 {
	d := h2 - h1
	switch m {
	case hueShorter:
		if d > 180 {
			d -= 360
		} else if d < -180 {
			d += 360
		}
	case hueLonger:
		if d > 0 {
			if d <= 180 {
				d -= 360
			}
		} else if d < 0 {
			d += 360
		}
	case hueIncreasing:
		if d < 0 {
			d += 360
		}
	case hueDecreasing:
		if d > 0 {
			d -= 360
		}
	}
	return math.Mod(h1+d*t+360, 360)
}

// hueMethod is color-mix()'s hue interpolation strategy.
type hueMethod uint8

// The hue interpolation methods, CSS Color 5's set; shorter is the
// default when color-mix() names none.
const (
	hueShorter hueMethod = iota
	hueLonger
	hueIncreasing
	hueDecreasing
)

// toComps converts a color into sp's components plus alpha: rgb and
// oklab in 0..1 (lab L in 0..100), polar spaces with the hue in
// degrees and saturation/chroma/lightness in their natural units.
func toComps(c rgbaf, sp space) [4]float64 {
	switch sp {
	case spSrgb:
		return [4]float64{c.r, c.g, c.b, c.a}
	case spSrgbLinear:
		return [4]float64{srgbDecode(c.r), srgbDecode(c.g), srgbDecode(c.b), c.a}
	case spLab:
		l, a, b := xyzToLab(rgbToXYZ(c))
		return [4]float64{l, a, b, c.a}
	case spLch:
		l, a, b := xyzToLab(rgbToXYZ(c))
		l2, ch, h := labToLch(l, a, b)
		return [4]float64{l2, ch, h, c.a}
	case spOklab:
		l, a, b := linearToOklab(rgbaf{srgbDecode(c.r), srgbDecode(c.g), srgbDecode(c.b), 1})
		return [4]float64{l, a, b, c.a}
	case spOklch:
		l, a, b := linearToOklab(rgbaf{srgbDecode(c.r), srgbDecode(c.g), srgbDecode(c.b), 1})
		l2, ch, h := labToLch(l, a, b)
		return [4]float64{l2, ch, h, c.a}
	case spHsl:
		h, l, s := toHLS(c)
		return [4]float64{h, s, l, c.a}
	default:
		h, _, _ := toHLS(c)
		mx := math.Max(c.r, math.Max(c.g, c.b))
		mn := math.Min(c.r, math.Min(c.g, c.b))
		return [4]float64{h, mn, 1 - mx, c.a}
	}
}

// fromComps converts sp's components plus alpha back to a color.
func fromComps(cs [4]float64, sp space) rgbaf {
	var c rgbaf
	c.a = cs[3]
	switch sp {
	case spSrgb:
		c.r, c.g, c.b = cs[0], cs[1], cs[2]
	case spSrgbLinear:
		c.r, c.g, c.b = srgbEncode(cs[0]), srgbEncode(cs[1]), srgbEncode(cs[2])
	case spLab:
		c = xyzToRGB(labToXYZ(cs[0], cs[1], cs[2]))
	case spLch:
		l, a, b := lchToLab(cs[0], cs[1], cs[2])
		c = xyzToRGB(labToXYZ(l, a, b))
	case spOklab:
		lin := oklabToLinear(cs[0], cs[1], cs[2])
		c = rgbaf{srgbEncode(lin.r), srgbEncode(lin.g), srgbEncode(lin.b), 1}
	case spOklch:
		l, a, b := lchToLab(cs[0], cs[1], cs[2])
		lin := oklabToLinear(l, a, b)
		c = rgbaf{srgbEncode(lin.r), srgbEncode(lin.g), srgbEncode(lin.b), 1}
	case spHsl:
		c = fromHLS(cs[0], cs[2], cs[1])
	default:
		c = hwbToRGB(cs[0], cs[1], cs[2])
	}
	c.a = cs[3]
	return c
}

// hwbToRGB converts hwb's hue, whiteness and blackness to rgb.
func hwbToRGB(h, w, bk float64) rgbaf {
	if w+bk >= 1 {
		v := w / (w + bk)
		return rgbaf{v, v, v, 1}
	}
	base := fromHLS(h, 0.5, 1)
	return rgbaf{
		r: base.r*(1-w-bk) + w,
		g: base.g*(1-w-bk) + w,
		b: base.b*(1-w-bk) + w,
		a: 1,
	}
}

func srgbDecode(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func srgbEncode(c float64) float64 {
	if c <= 0.0031308 {
		return c * 12.92
	}
	return 1.055*math.Pow(c, 1/2.4) - 0.055
}

// xyz is a CIE XYZ tristimulus value.
type xyz struct{ x, y, z float64 }

// linSRGBToXYZ and its inverse are CSS Color 4's sRGB reference
// matrices (D65); d65ToD50 and back the Bradford adaptation.
var (
	linSRGBToXYZ = [9]float64{
		0.41239079926595948, 0.35758433938387796, 0.18048078840183429,
		0.21263900587151036, 0.71516867876775593, 0.07219231536073371,
		0.01933081871559185, 0.11919477979462599, 0.95053215224966058,
	}
	xyzToLinSRGB = [9]float64{
		3.2409699419045214, -1.5373831775700935, -0.49861076029300328,
		-0.96924363628087983, 1.8759675015077207, 0.04155505740717561,
		0.05563007969699361, -0.20397695888897657, 1.0569715142428786,
	}
	d65ToD50 = [9]float64{
		1.0479298208405488, 0.022946793341019088, -0.05019222954313557,
		0.029627815688159344, 0.990434484573249, -0.01707382502938514,
		-0.009243058152591178, 0.015055144896577895, 0.7518742899580008,
	}
	d50ToD65 = [9]float64{
		0.9554734527042182, -0.023098536874261423, 0.0632593086610217,
		-0.028369706963208136, 1.0099954580058226, 0.021041398966943008,
		0.012314001688319899, -0.020507696433477912, 1.3303659366080753,
	}
)

func matApply(m [9]float64, v xyz) xyz {
	return xyz{
		x: m[0]*v.x + m[1]*v.y + m[2]*v.z,
		y: m[3]*v.x + m[4]*v.y + m[5]*v.z,
		z: m[6]*v.x + m[7]*v.y + m[8]*v.z,
	}
}

func matMul(a, b [9]float64) [9]float64 {
	var out [9]float64
	for r := range 3 {
		for c := range 3 {
			out[r*3+c] = a[r*3]*b[c] + a[r*3+1]*b[3+c] + a[r*3+2]*b[6+c]
		}
	}
	return out
}

func rgbToXYZ(c rgbaf) xyz {
	return matApply(linSRGBToXYZ, xyz{srgbDecode(c.r), srgbDecode(c.g), srgbDecode(c.b)})
}

func xyzToRGB(v xyz) rgbaf {
	lin := matApply(xyzToLinSRGB, v)
	return rgbaf{srgbEncode(lin.x), srgbEncode(lin.y), srgbEncode(lin.z), 1}
}

// labWhite is the D50 white point scaled to Y=1, lab()'s reference.
var labWhite = xyz{x: 0.9642956764295677, y: 1, z: 0.8251046025104602}

func labF(t float64) float64 {
	if t > 216.0/24389 {
		return math.Cbrt(t)
	}
	return (841.0/108)*t + 4.0/29
}

func labInvF(t float64) float64 {
	t3 := t * t * t
	if t3 > 216.0/24389 {
		return t3
	}
	return (108.0 / 841) * (t - 4.0/29)
}

func xyzToLab(v xyz) (l, a, b float64) {
	v = matApply(d65ToD50, v)
	fx, fy, fz := labF(v.x/labWhite.x), labF(v.y/labWhite.y), labF(v.z/labWhite.z)
	l = 116*fy - 16
	a = 500 * (fx - fy)
	b = 200 * (fy - fz)
	return l, a, b
}

func labToXYZ(l, a, b float64) xyz {
	fy := (l + 16) / 116
	fx := fy + a/500
	fz := fy - b/200
	v := xyz{x: labInvF(fx) * labWhite.x, y: labInvF(fy) * labWhite.y, z: labInvF(fz) * labWhite.z}
	return matApply(d50ToD65, v)
}

func labToLch(l, a, b float64) (l2, c, h float64) {
	c = math.Hypot(a, b)
	h = math.Mod(math.Atan2(b, a)*180/math.Pi+360, 360)
	return l, c, h
}

func lchToLab(l, c, h float64) (l2, a, b float64) {
	rad := h * math.Pi / 180
	return l, c * math.Cos(rad), c * math.Sin(rad)
}

func linearToOklab(c rgbaf) (l, a, b float64) {
	l0 := math.Cbrt(0.4122214708*c.r + 0.5363325363*c.g + 0.0514459929*c.b)
	m := math.Cbrt(0.2119034982*c.r + 0.6806995451*c.g + 0.1073969566*c.b)
	s := math.Cbrt(0.0883024619*c.r + 0.2817188376*c.g + 0.6299787005*c.b)
	return 0.2104542553*l0 + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l0 - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l0 + 0.7827717662*m - 0.8086757660*s
}

func oklabToLinear(l, a, b float64) rgbaf {
	l0 := l + 0.3963377774*a + 0.2158037573*b
	m := l - 0.1055613458*a - 0.0638541728*b
	s := l - 0.0894841775*a - 1.2914855480*b
	l3, m3, s3 := l0*l0*l0, m*m*m, s*s*s
	return rgbaf{
		r: 4.0767416621*l3 - 3.3077115913*m3 + 0.2309699292*s3,
		g: -1.2684380046*l3 + 2.6097574011*m3 - 0.3413193965*s3,
		b: -0.0041960863*l3 - 0.7034186147*m3 + 1.7076147010*s3,
		a: 1,
	}
}

// chanDef describes one color-function channel: whether it is a hue
// angle, what 100% means, and what a unitless number multiplies.
type chanDef struct {
	angle   bool
	pctFull float64
	numUnit float64
}

// colorSpaceDef drives one color() family constructor and its relative
// color syntax: the interpolation space, channel specs, and the
// channel keywords relative channels substitute.
type colorSpaceDef struct {
	space space
	names []string
	chans [4]chanDef
}

// The constructor families. rgb/hsl/hwb channels take their legacy
// scales; oklch's chroma 100% is 0.4, lab's a/b 100% is 125, lch's
// chroma 100% is 150.
var (
	rgbDef = colorSpaceDef{
		space: spSrgb,
		names: []string{"r", "g", "b"},
		chans: [4]chanDef{{pctFull: 1, numUnit: 1.0 / 255}, {pctFull: 1, numUnit: 1.0 / 255}, {pctFull: 1, numUnit: 1.0 / 255}, {pctFull: 1}},
	}
	hslDef = colorSpaceDef{
		space: spHsl,
		names: []string{"h", "s", "l"},
		chans: [4]chanDef{{angle: true}, {pctFull: 1}, {pctFull: 1}, {pctFull: 1}},
	}
	hwbDef = colorSpaceDef{
		space: spHwb,
		names: []string{"h", "w", "b"},
		chans: [4]chanDef{{angle: true}, {pctFull: 1}, {pctFull: 1}, {pctFull: 1}},
	}
	labDef = colorSpaceDef{
		space: spLab,
		names: []string{"l", "a", "b"},
		chans: [4]chanDef{{pctFull: 100}, {pctFull: 125}, {pctFull: 125}, {pctFull: 1}},
	}
	lchDef = colorSpaceDef{
		space: spLch,
		names: []string{"l", "c", "h"},
		chans: [4]chanDef{{pctFull: 100}, {pctFull: 150}, {angle: true}, {pctFull: 1}},
	}
	oklabDef = colorSpaceDef{
		space: spOklab,
		names: []string{"l", "a", "b"},
		chans: [4]chanDef{{pctFull: 1}, {pctFull: 0.4}, {pctFull: 0.4}, {pctFull: 1}},
	}
	oklchDef = colorSpaceDef{
		space: spOklch,
		names: []string{"l", "c", "h"},
		chans: [4]chanDef{{pctFull: 1}, {pctFull: 0.4}, {angle: true}, {pctFull: 1}},
	}
)

// relAlphaRef is the alpha keyword of relative color syntax.
const relAlphaRef = "alpha"

// spaceFunc parses one color-space constructor (and its relative color
// syntax `fn(from <color> ...)`) into a color. refs, when set, resolves
// the channel keywords relative color syntax substitutes.
func spaceFunc(def colorSpaceDef, args []token, cx *ctx, refs map[string]numeric) (colorVal, bool) {
	comps := components(args)
	if refs == nil && len(comps) >= 2 && len(comps[0]) == 1 && comps[0][0].ident("from") {
		return relativeColor(def, comps, cx)
	}
	var vals [4]float64
	i := 0
	for _, c := range comps {
		if len(c) == 1 && c[0].is('/') {
			continue
		}
		if i >= 4 {
			return colorVal{}, false
		}
		v, ok := def.chans[i].value(c, cx, refs, i)
		if !ok {
			return colorVal{}, false
		}
		vals[i] = v
		i++
	}
	if i < 3 {
		return colorVal{}, false
	}
	if vals[3] == 0 && !hasAlpha(comps) && i < 4 {
		vals[3] = 1
	}
	return colorVal{c: fromComps(vals, def.space).color()}, true
}

// hasAlpha reports whether the component list carries an explicit
// `/ alpha` part.
func hasAlpha(comps [][]token) bool {
	for i := range comps {
		if len(comps[i]) == 1 && comps[i][0].is('/') {
			return i+1 < len(comps)
		}
	}
	return false
}

// value evaluates one channel against its spec, substituting relative
// refs when present.
func (d chanDef) value(ts []token, cx *ctx, refs map[string]numeric, i int) (float64, bool) {
	ts = trimWS(ts)
	if len(ts) == 1 && ts[0].ident("none") {
		return 0, true
	}
	p := calcParser{ts: ts, cx: cx, refs: refs}
	n, ok := p.sum()
	if !ok || p.pos != len(ts) {
		return 0, false
	}
	if i == 3 {
		switch n.kind {
		case numNumber, numPercent:
		default:
			return 0, false
		}
		if n.kind == numPercent {
			n = numeric{v: n.v / 100, kind: numNumber}
		}
		return n.v, true
	}
	unit := d.numUnit
	if unit == 0 {
		unit = 1
	}
	switch {
	case d.angle:
		if n.kind != numAngle && n.kind != numNumber {
			return 0, false
		}
		return n.v, true
	case n.kind == numPercent:
		return n.v / 100 * d.pctFull, true
	case n.kind == numNumber:
		return n.v * unit, true
	}
	return 0, false
}

// relativeColor parses the relative color syntax: the origin color
// then three channel expressions (plus optional alpha) over the origin
// channels, each keyword standing for that channel's value in the
// function's own scale. comps[0] is the `from` marker.
func relativeColor(def colorSpaceDef, comps [][]token, cx *ctx) (colorVal, bool) {
	if len(comps) < 2 || len(comps[1]) == 0 {
		return colorVal{}, false
	}
	origin, ok := parseColor(comps[1], cx)
	if !ok {
		return colorVal{}, false
	}
	oc := toComps(fromColor(cx.resolve(origin)), def.space)
	refs := map[string]numeric{}
	for i, name := range def.names {
		refs[name] = refNumeric(def.chans[i], oc[i])
	}
	refs[relAlphaRef] = numeric{v: oc[3], kind: numNumber}
	return spaceFunc(def, joinTokens(comps[2:]), cx, refs)
}

// refNumeric expresses one origin channel value in the numeric form
// its channel expressions read.
func refNumeric(d chanDef, v float64) numeric {
	switch {
	case d.angle:
		return numeric{v: v, kind: numAngle}
	case d.numUnit != 0 && d.numUnit != 1:
		return numeric{v: v / d.numUnit, kind: numNumber}
	}
	return numeric{v: v / d.pctFull * 100, kind: numPercent}
}

// joinTokens concatenates component groups with separating whitespace.
func joinTokens(comps [][]token) []token {
	var out []token
	for i, c := range comps {
		if i > 0 {
			out = append(out, token{kind: tkWS})
		}
		out = append(out, c...)
	}
	return out
}

// predefined is one color() predefined space: the composite matrices
// between its linear form and linear sRGB (white-point adapted), and
// its transfer decode/encode pair.
type predefined struct {
	toLinearSRGB, fromLinearSRGB [9]float64
	decode, encode               func(float64) float64
}

// predefinedSpaces are color()'s spaces, keyed by name; xyz and its
// aliases are linear already.
var predefinedSpaces = map[string]predefined{
	"srgb":         {identity(), identity(), srgbDecode, srgbEncode},
	"srgb-linear":  {identity(), identity(), linearTransfer, linearTransfer},
	"xyz":          {linSRGBToXYZ, xyzToLinSRGB, linearTransfer, linearTransfer},
	"xyz-d65":      {linSRGBToXYZ, xyzToLinSRGB, linearTransfer, linearTransfer},
	"xyz-d50":      {matMul(d65ToD50, linSRGBToXYZ), matMul(xyzToLinSRGB, d50ToD65), linearTransfer, linearTransfer},
	"display-p3":   p3Space(),
	"a98-rgb":      a98Space(),
	"prophoto-rgb": prophotoSpace(),
	"rec2020":      rec2020Space(),
}

func identity() [9]float64 {
	return [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1}
}

func linearTransfer(v float64) float64 { return v }

// p3Space composes linear Display-P3 ↔ XYZ D65 (CSS Color 4) with the
// sRGB reference matrices.
func p3Space() predefined {
	p3ToXYZ := [9]float64{
		0.4865709486482162, 0.26566769316909306, 0.1982172852343630,
		0.2289745640697488, 0.6917385218365064, 0.0792869140937450,
		0, 0.0451133818589026, 1.043944368900976,
	}
	xyzToP3 := [9]float64{
		2.493496911941425, -0.9313836179191239, -0.4027107844507168,
		-0.8294889695615747, 1.7626640603183463, 0.0236246858419435,
		0.0358458302437844, -0.0761723892680418, 0.9568845240076872,
	}
	return predefined{
		toLinearSRGB:   matMul(xyzToLinSRGB, p3ToXYZ),
		fromLinearSRGB: matMul(xyzToP3, linSRGBToXYZ),
		decode:         srgbDecode, encode: srgbEncode,
	}
}

// a98Space composes the known linear sRGB ↔ linear a98-rgb matrices.
func a98Space() predefined {
	return predefined{
		toLinearSRGB: [9]float64{
			2.0415879038107465, -0.9692436362808795, 0.0134442806320311,
			-0.5650069742788597, 1.8759675015077207, -0.1183623922310184,
			-0.3447313507783256, 0.0415550574071756, 1.0151749943912054,
		},
		fromLinearSRGB: [9]float64{
			0.5766690429101305, 0.1855582379065463, 0.1882286462349947,
			0.2973449752505360, 0.6273635662554661, 0.0752914584939979,
			0.0270313613864123, 0.0706888525358272, 0.9913375368376388,
		},
		decode: func(v float64) float64 { return math.Pow(v, 1/2.19921875) },
		encode: func(v float64) float64 { return math.Pow(v, 2.19921875) },
	}
}

// prophotoSpace composes linear prophoto ↔ XYZ D50 (CSS Color 4)
// through the Bradford adaptation, with its 1.8 gamma and digital
// toe.
func prophotoSpace() predefined {
	proToXYZ := [9]float64{
		0.7977604896723027, 0.1351858371757403, 0.0313493495815248,
		0.2880711282292934, 0.7118432178101014, 0.0000856539606053,
		0, 0, 0.8251046025104602,
	}
	xyzToPro := [9]float64{
		1.3457989731028281, -0.2555801000799753, -0.0511062850675340,
		-0.5446224939028347, 1.5082327413132781, 0.0205360323914797,
		0, 0, 1.2119675456389454,
	}
	dec := func(v float64) float64 {
		if v <= 0.03125 {
			return v / 16
		}
		return math.Pow(v, 1.8)
	}
	enc := func(v float64) float64 {
		if v <= 0.001953125 {
			return v * 16
		}
		return math.Pow(v, 1/1.8)
	}
	return predefined{
		toLinearSRGB:   matMul(xyzToLinSRGB, matMul(d50ToD65, proToXYZ)),
		fromLinearSRGB: matMul(xyzToPro, matMul(d65ToD50, linSRGBToXYZ)),
		decode:         dec, encode: enc,
	}
}

// rec2020Space composes linear rec2020 ↔ XYZ D65 (CSS Color 4) with
// its piecewise transfer.
func rec2020Space() predefined {
	recToXYZ := [9]float64{
		0.6369580483012914, 0.1446169035862083, 0.1688809751641721,
		0.2627002120112671, 0.6779980715188708, 0.0593017164698620,
		0, 0.0280726930490874, 1.060985057710791,
	}
	xyzToRec := [9]float64{
		1.716651187971268, -0.355670783776392, -0.253366281373660,
		-0.666684351832489, 1.616481236634939, 0.015768545813911,
		0.017639857445311, -0.042770613257809, 0.942103121235474,
	}
	const (
		alpha = 1.09929682680944
		beta  = 0.018053968510807
	)
	dec := func(v float64) float64 {
		if v < beta*4.5 {
			return v / 4.5
		}
		return math.Pow((v+alpha-1)/alpha, 1/0.45)
	}
	enc := func(v float64) float64 {
		if v < beta {
			return v * 4.5
		}
		return alpha*math.Pow(v, 0.45) - alpha + 1
	}
	return predefined{
		toLinearSRGB:   matMul(xyzToLinSRGB, recToXYZ),
		fromLinearSRGB: matMul(xyzToRec, linSRGBToXYZ),
		decode:         dec, encode: enc,
	}
}

// colorFunc parses color(space, c1 c2 c3 [/ a]): a predefined-space
// color mapped to sRGB, channels numbers 0..1 or percentages.
func colorFunc(args []token, cx *ctx) (colorVal, bool) {
	comps := components(commaToWS(args))
	if len(comps) < 4 {
		return colorVal{}, false
	}
	if len(comps[0]) != 1 || comps[0][0].kind != tkIdent {
		return colorVal{}, false
	}
	sp, ok := predefinedSpaces[strings.ToLower(comps[0][0].s)]
	if !ok {
		return colorVal{}, false
	}
	var vals [4]float64
	i := 0
	for _, c := range comps[1:] {
		if len(c) == 1 && c[0].is('/') {
			continue
		}
		if i >= 4 {
			return colorVal{}, false
		}
		v, ok := chanOf(c, cx)
		if !ok {
			return colorVal{}, false
		}
		vals[i] = v
		i++
	}
	if i < 3 {
		return colorVal{}, false
	}
	if vals[3] == 0 && !hasAlpha(comps[1:]) {
		vals[3] = 1
	}
	lin := matApply(sp.toLinearSRGB, xyz{sp.decode(vals[0]), sp.decode(vals[1]), sp.decode(vals[2])})
	return colorVal{c: rgbaf{srgbEncode(lin.x), srgbEncode(lin.y), srgbEncode(lin.z), vals[3]}.color()}, true
}

// chanOf reads one color() channel: a number 0..1 or a percentage,
// alpha a number 0..1 or a percentage.
func chanOf(ts []token, cx *ctx) (float64, bool) {
	ts = trimWS(ts)
	if len(ts) == 1 && ts[0].ident("none") {
		return 0, true
	}
	n, ok := evalNumeric(ts, cx)
	if !ok {
		return 0, false
	}
	switch n.kind {
	case numPercent:
		return n.v / 100, true
	case numNumber:
		return n.v, true
	}
	return 0, false
}

// lightDark parses light-dark(a, b): a under a light color scheme, b
// under a dark one, following the active theme's polarity.
func lightDark(args []token, cx *ctx) (colorVal, bool) {
	parts := splitTop(args, tkComma)
	if len(parts) != 2 {
		return colorVal{}, false
	}
	cx.dep = true
	cv, ok := parseColor(trimWS(parts[0]), cx)
	if !ok {
		return colorVal{}, false
	}
	if !cx.dark {
		return cv, true
	}
	return parseColor(trimWS(parts[1]), cx)
}

// attrColor parses typed attr(name color), reading the node's
// attribute and parsing it as a color.
func attrColor(args []token, cx *ctx) (colorVal, bool) {
	comps := components(args)
	if len(comps) != 1 && len(comps) != 2 {
		return colorVal{}, false
	}
	if len(comps[0]) != 1 || comps[0][0].kind != tkIdent {
		return colorVal{}, false
	}
	if len(comps) == 2 && (len(comps[1]) != 1 || !comps[1][0].ident("color")) {
		return colorVal{}, false
	}
	val, ok := cx.attrOf(comps[0][0].s)
	if !ok {
		return colorVal{}, false
	}
	return parseColor(tokenize(val), cx)
}

// SystemColors are the CSS system color keywords mapped to the active
// theme; SetSystemColors installs the mapping, so a theme swap
// re-derives every system color the way the theme layer re-derives
// the named colors.
type SystemColors struct {
	Canvas, CanvasText, LinkText, VisitedText, GrayText    render.Color
	Highlight, HighlightText, Mark, MarkText               render.Color
	ButtonFace, ButtonText, ButtonBorder, Field, FieldText render.Color
}

// defaultSystemColors is the neutral mapping SetSystemColors resets to.
var defaultSystemColors = SystemColors{
	Canvas:        render.RGB(0xff, 0xff, 0xff),
	CanvasText:    render.RGB(0x00, 0x00, 0x00),
	LinkText:      render.RGB(0x00, 0x00, 0xee),
	VisitedText:   render.RGB(0x55, 0x1a, 0x8b),
	GrayText:      render.RGBA(0x00, 0x00, 0x00, 0x7f),
	Highlight:     render.RGB(0x00, 0x00, 0xff),
	HighlightText: render.RGB(0xff, 0xff, 0xff),
	Mark:          render.RGB(0xff, 0xff, 0x00),
	MarkText:      render.RGB(0x00, 0x00, 0x00),
	ButtonFace:    render.RGB(0xd4, 0xd0, 0xc8),
	ButtonText:    render.RGB(0x00, 0x00, 0x00),
	ButtonBorder:  render.RGB(0x80, 0x80, 0x80),
	Field:         render.RGB(0xff, 0xff, 0xff),
	FieldText:     render.RGB(0x00, 0x00, 0x00),
}

// sysColors is the installed system color mapping.
var sysColors = defaultSystemColors

// SetSystemColors installs the system color mapping; the zero value
// restores the defaults. One mapping per process, like the theme
// layer.
func SetSystemColors(s SystemColors) {
	if s == (SystemColors{}) {
		s = defaultSystemColors
	}
	sysColors = s
}

// sysColor resolves one system color keyword, marking the node
// dependence.
func (cx *ctx) sysColor(name string) (render.Color, bool) {
	cx.dep = true
	switch name {
	case "canvas":
		return sysColors.Canvas, true
	case "canvastext":
		return sysColors.CanvasText, true
	case "linktext":
		return sysColors.LinkText, true
	case "visitedtext":
		return sysColors.VisitedText, true
	case "graytext":
		return sysColors.GrayText, true
	case "highlight":
		return sysColors.Highlight, true
	case "highlighttext":
		return sysColors.HighlightText, true
	case "mark":
		return sysColors.Mark, true
	case "marktext":
		return sysColors.MarkText, true
	case "buttonface":
		return sysColors.ButtonFace, true
	case "buttontext":
		return sysColors.ButtonText, true
	case "buttonborder":
		return sysColors.ButtonBorder, true
	case "field":
		return sysColors.Field, true
	case "fieldtext":
		return sysColors.FieldText, true
	}
	return 0, false
}
