package style

import (
	"slices"
	"strings"
)

// Background and mask image layers (css-backgrounds-3 §3, css-images-4
// §2, css-masking-1 §7): an image source, and how a layer sizes,
// positions, repeats and clips it. A background is a stack of layers,
// its longhands lists matched by index against the image list; a mask
// is one layer of the same parts.

// ImageSource is one <image>: a gradient, a url() file, or none (both
// zero). image-set() resolves to one of its candidates when it
// computes.
type ImageSource struct {
	Gradient *Gradient
	URL      string
	// Grain is lys-ui's -lys-grain(): a fine noise at this opacity
	// (0 to 1) over the layer's area, the etching of frosted glass.
	Grain float64
}

// None reports an image that paints nothing.
func (s ImageSource) None() bool { return s.Gradient == nil && s.URL == "" && s.Grain == 0 }

// equal compares two sources, gradients by value.
func (s ImageSource) equal(o ImageSource) bool {
	if s.URL != o.URL || s.Grain != o.Grain || (s.Gradient == nil) != (o.Gradient == nil) {
		return false
	}
	return s.Gradient == nil || *s.Gradient == *o.Gradient
}

// LayerSizeKind is a layer size's form.
type LayerSizeKind uint8

// The layer size forms: auto (the image's natural size, a gradient
// filling the positioning area), cover, contain, and explicit widths
// and heights.
const (
	SizeAuto LayerSizeKind = iota
	SizeCover
	SizeContain
	SizeExplicit
)

// LayerLength is one explicit size dimension: a length or percentage
// of the positioning area, or auto.
type LayerLength struct {
	Auto bool
	Len  NumVal
}

// LayerSize is background-size or mask-size.
type LayerSize struct {
	Kind LayerSizeKind
	W, H LayerLength
}

// TileKind is one axis of background-repeat or mask-repeat.
type TileKind uint8

// The repeat styles.
const (
	TileRepeat TileKind = iota
	TileNoRepeat
	TileSpace
	TileRound
	// TileStretch scales one tile over the whole length: border-image only.
	TileStretch
)

// LayerRepeat is a layer's horizontal then vertical repeat style.
type LayerRepeat [2]TileKind

// LayerBox is a box a layer positions in or clips to.
type LayerBox uint8

// The layer boxes.
const (
	BoxBorder LayerBox = iota
	BoxPadding
	BoxContent
)

// LayerAttachment is background-attachment.
type LayerAttachment uint8

// The attachments: scroll and local move with the box (no box here
// scrolls its own background), fixed positions against the viewport.
const (
	AttachScroll LayerAttachment = iota
	AttachFixed
	AttachLocal
)

// ImageLayer is one zipped background layer.
type ImageLayer struct {
	Image  ImageSource
	Size   LayerSize
	Pos    [2]AxisPos
	Repeat LayerRepeat
	Clip   LayerBox
	Origin LayerBox
	Attach LayerAttachment
}

// Background is the background-* longhands' lists as declared; a nil
// list is the initial value (none, auto, 0% 0%, repeat, border-box,
// padding-box, scroll).
type Background struct {
	Images      []ImageSource
	Sizes       []LayerSize
	Positions   [][2]AxisPos
	Repeats     []LayerRepeat
	Clips       []LayerBox
	Origins     []LayerBox
	Attachments []LayerAttachment
}

// Len is the layer count: one per image, at least one.
func (b *Background) Len() int { return max(1, len(b.Images)) }

// Layer zips layer i, top first: each list matched by index and
// repeating.
func (b *Background) Layer(i int) ImageLayer {
	return ImageLayer{
		Image:  listAt(b.Images, i, ImageSource{}),
		Size:   listAt(b.Sizes, i, LayerSize{}),
		Pos:    listAt(b.Positions, i, [2]AxisPos{}),
		Repeat: listAt(b.Repeats, i, LayerRepeat{}),
		Clip:   listAt(b.Clips, i, BoxBorder),
		Origin: listAt(b.Origins, i, BoxPadding),
		Attach: listAt(b.Attachments, i, AttachScroll),
	}
}

// Painted reports whether any layer has an image.
func (b *Background) Painted() bool {
	return slices.ContainsFunc(b.Images, func(s ImageSource) bool { return !s.None() })
}

// Equal reports whether two backgrounds hold the same lists.
func (b *Background) Equal(o *Background) bool {
	return slices.EqualFunc(b.Images, o.Images, ImageSource.equal) && slices.Equal(b.Sizes, o.Sizes) &&
		slices.Equal(b.Positions, o.Positions) && slices.Equal(b.Repeats, o.Repeats) &&
		slices.Equal(b.Clips, o.Clips) && slices.Equal(b.Origins, o.Origins) &&
		slices.Equal(b.Attachments, o.Attachments)
}

// Mask is the computed mask group (css-masking-1 §7): one image layer
// — its image, size, position and repeat, positioned and clipped in
// the border box — and whether it reads luminance instead of alpha.
type Mask struct {
	Image     ImageSource
	Size      LayerSize
	Pos       [2]AxisPos
	Repeat    LayerRepeat
	Luminance bool
}

// Equal reports whether two masks are the same.
func (m *Mask) Equal(o *Mask) bool {
	return m.Image.equal(o.Image) && m.Size == o.Size && m.Pos == o.Pos && m.Repeat == o.Repeat &&
		m.Luminance == o.Luminance
}

// parseMaskImage parses mask-image: one <image>. A list of layers is
// rejected: the engine composites one mask layer (docs/css.md).
func parseMaskImage(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	img, ok := imageOf(comps[0], cx)
	if ok {
		v.Mask.Image = img
	}
	return ok
}

// parseMaskPosition parses mask-position as a CSS <position>.
func parseMaskPosition(ts []token, cx *ctx, v *Values) bool {
	pos, ok := parsePosition(components(ts), cx)
	if ok {
		v.Mask.Pos = pos
	}
	return ok
}

// parseMaskSize parses mask-size, background-size's grammar.
func parseMaskSize(ts []token, cx *ctx, v *Values) bool {
	s, ok := layerSizeOf(components(ts), cx)
	if ok {
		v.Mask.Size = s
	}
	return ok
}

// parseMaskRepeat parses mask-repeat, background-repeat's grammar.
func parseMaskRepeat(ts []token, _ *ctx, v *Values) bool {
	r, ok := layerRepeatOf(components(ts))
	if ok {
		v.Mask.Repeat = r
	}
	return ok
}

// parseMaskMode parses mask-mode: match-source, alpha or luminance.
func parseMaskMode(ts []token, _ *ctx, v *Values) bool {
	s, ok := identOf(components(ts))
	switch {
	case !ok:
		return false
	case s == "match-source" || s == "alpha":
		v.Mask.Luminance = false
	case s == "luminance":
		v.Mask.Luminance = true
	default:
		return false
	}
	return true
}

// imageOf parses one <image>: none, url(), a gradient, or image-set().
func imageOf(ts []token, cx *ctx) (ImageSource, bool) {
	if len(ts) == 1 && ts[0].ident("none") {
		return ImageSource{}, true
	}
	if u, ok := backgroundImageURL(ts); ok {
		return ImageSource{URL: u}, true
	}
	if g, ok := gradientOf(ts, cx); ok {
		return ImageSource{Gradient: &g}, true
	}
	if len(ts) > 0 && ts[0].kind == tkFunc && strings.EqualFold(ts[0].s, "-lys-grain") {
		comps := components(funcArgs(ts))
		if len(comps) != 1 {
			return ImageSource{}, false
		}
		n, ok := evalNumeric(comps[0], cx)
		if !ok || (n.kind != numNumber && n.kind != numPercent) {
			return ImageSource{}, false
		}
		if n.kind == numPercent {
			n.v /= 100
		}
		if n.v <= 0 || n.v > 1 {
			return ImageSource{}, false
		}
		return ImageSource{Grain: n.v}, true
	}
	if len(ts) > 0 && ts[0].kind == tkFunc && (strings.EqualFold(ts[0].s, "image-set") || strings.EqualFold(ts[0].s, "-webkit-image-set")) {
		return imageSetOf(funcArgs(ts), cx)
	}
	return ImageSource{}, false
}

// imageSetOf resolves image-set() (css-images-4 §2.2): each candidate
// an image (a bare string is a url) with an optional resolution
// (1x by default) and type(); the candidate chosen is the one with the
// lowest resolution at or above the device's, else the highest.
// Candidates of an unsupported type() drop.
func imageSetOf(args []token, cx *ctx) (ImageSource, bool) {
	cx.dep = true
	var best ImageSource
	bestRes, found := 0.0, false
	for _, item := range splitTop(args, tkComma) {
		comps := components(trimWS(item))
		if len(comps) == 0 {
			return ImageSource{}, false
		}
		var img ImageSource
		if len(comps[0]) == 1 && comps[0][0].kind == tkString {
			img = ImageSource{URL: strings.TrimPrefix(comps[0][0].s, "file://")}
		} else if s, ok := imageOf(comps[0], cx); ok && !s.None() {
			img = s
		} else {
			return ImageSource{}, false
		}
		res, typed := 1.0, true
		for _, c := range comps[1:] {
			if r, ok := mediaResolution(c); ok {
				res = r / 96
				continue
			}
			if c[0].kind == tkFunc && strings.EqualFold(c[0].s, "type") {
				typed = imageTypeSupported(funcArgs(c))
				continue
			}
			return ImageSource{}, false
		}
		if !typed {
			continue
		}
		if !found || betterResolution(res, bestRes, cx.dppx()) {
			best, bestRes, found = img, res, true
		}
	}
	return best, found
}

// betterResolution reports whether a candidate of resolution res beats
// the current pick cur for a device at dppx: the lowest at or above the
// device, else the highest below it.
func betterResolution(res, cur, dppx float64) bool {
	switch {
	case res >= dppx && cur >= dppx:
		return res < cur
	case res >= dppx:
		return true
	case cur >= dppx:
		return false
	}
	return res > cur
}

// imageTypeSupported reports whether image-set()'s type("mime") names
// a format the image loader decodes.
func imageTypeSupported(args []token) bool {
	if len(args) != 1 || args[0].kind != tkString {
		return false
	}
	switch strings.ToLower(args[0].s) {
	case "image/png", "image/jpeg", "image/gif", "image/svg+xml":
		return true
	}
	return false
}

// layerSizeOf parses one background-size or mask-size value.
func layerSizeOf(comps [][]token, cx *ctx) (LayerSize, bool) {
	if s, ok := identOf(comps); ok && len(comps) == 1 {
		switch s {
		case "cover":
			return LayerSize{Kind: SizeCover}, true
		case "contain":
			return LayerSize{Kind: SizeContain}, true
		}
	}
	if len(comps) < 1 || len(comps) > 2 {
		return LayerSize{}, false
	}
	var dims [2]LayerLength
	for i := range dims {
		c := comps[min(i, len(comps)-1)]
		if i == 1 && len(comps) == 1 {
			dims[1] = LayerLength{Auto: true}
			break
		}
		if len(c) == 1 && c[0].ident("auto") {
			dims[i] = LayerLength{Auto: true}
			continue
		}
		n, ok := numValOf(c, cx)
		if !ok || n.V < 0 {
			return LayerSize{}, false
		}
		dims[i] = LayerLength{Len: n}
	}
	if dims[0].Auto && dims[1].Auto {
		return LayerSize{}, true
	}
	return LayerSize{Kind: SizeExplicit, W: dims[0], H: dims[1]}, true
}

// repeatNames are the per-axis repeat keywords.
var repeatNames = map[string]TileKind{
	"repeat": TileRepeat, "no-repeat": TileNoRepeat, "space": TileSpace, "round": TileRound,
}

// layerRepeatOf parses one <repeat-style>: repeat-x, repeat-y, or one
// or two per-axis keywords.
func layerRepeatOf(comps [][]token) (LayerRepeat, bool) {
	var words []string
	for _, c := range comps {
		s, ok := identOf([][]token{c})
		if !ok {
			return LayerRepeat{}, false
		}
		words = append(words, s)
	}
	switch {
	case len(words) == 1 && words[0] == "repeat-x":
		return LayerRepeat{TileRepeat, TileNoRepeat}, true
	case len(words) == 1 && words[0] == "repeat-y":
		return LayerRepeat{TileNoRepeat, TileRepeat}, true
	case len(words) == 1:
		k, ok := repeatNames[words[0]]
		return LayerRepeat{k, k}, ok
	case len(words) == 2:
		x, okx := repeatNames[words[0]]
		y, oky := repeatNames[words[1]]
		return LayerRepeat{x, y}, okx && oky
	}
	return LayerRepeat{}, false
}

// boxNames are the layer box keywords.
var boxNames = map[string]LayerBox{"border-box": BoxBorder, "padding-box": BoxPadding, "content-box": BoxContent}

// attachNames are background-attachment's keywords.
var attachNames = map[string]LayerAttachment{"scroll": AttachScroll, "fixed": AttachFixed, "local": AttachLocal}

// layerList parses a comma list of layer values through one.
func layerList[T any](ts []token, one func(comps [][]token) (T, bool)) ([]T, bool) {
	var out []T
	for _, item := range splitTop(ts, tkComma) {
		v, ok := one(components(trimWS(item)))
		if !ok {
			return nil, false
		}
		out = append(out, v)
	}
	return out, true
}

// layerKeyword adapts a keyword table to layerList.
func layerKeyword[T any](names map[string]T) func(comps [][]token) (T, bool) {
	return func(comps [][]token) (T, bool) {
		var zero T
		s, ok := identOf(comps)
		if !ok || len(comps) != 1 {
			return zero, false
		}
		v, ok := names[s]
		return v, ok
	}
}

func parseBackgroundImage(ts []token, cx *ctx, v *Values) bool {
	l, ok := layerList(ts, func(comps [][]token) (ImageSource, bool) {
		if len(comps) != 1 {
			return ImageSource{}, false
		}
		return imageOf(comps[0], cx)
	})
	if ok {
		v.Bg.Images = l
	}
	return ok
}

func parseBackgroundSize(ts []token, cx *ctx, v *Values) bool {
	l, ok := layerList(ts, func(comps [][]token) (LayerSize, bool) { return layerSizeOf(comps, cx) })
	if ok {
		v.Bg.Sizes = l
	}
	return ok
}

func parseBackgroundPosition(ts []token, cx *ctx, v *Values) bool {
	l, ok := layerList(ts, func(comps [][]token) ([2]AxisPos, bool) { return parsePosition(comps, cx) })
	if ok {
		v.Bg.Positions = l
	}
	return ok
}

func parseBackgroundRepeat(ts []token, _ *ctx, v *Values) bool {
	l, ok := layerList(ts, layerRepeatOf)
	if ok {
		v.Bg.Repeats = l
	}
	return ok
}

func parseBackgroundClip(ts []token, _ *ctx, v *Values) bool {
	l, ok := layerList(ts, layerKeyword(boxNames))
	if ok {
		v.Bg.Clips = l
	}
	return ok
}

func parseBackgroundOrigin(ts []token, _ *ctx, v *Values) bool {
	l, ok := layerList(ts, layerKeyword(boxNames))
	if ok {
		v.Bg.Origins = l
	}
	return ok
}

func parseBackgroundAttachment(ts []token, _ *ctx, v *Values) bool {
	l, ok := layerList(ts, layerKeyword(attachNames))
	if ok {
		v.Bg.Attachments = l
	}
	return ok
}

// parseBackground parses the background shorthand (css-backgrounds-3
// §3.10): a comma list of layers, each any order of an image, a
// position with an optional `/ size`, a repeat style, an attachment
// and one or two boxes (the first the origin, the second the clip,
// one box both), and the color in the final layer only. Every
// longhand list takes one entry per layer; the color resets to
// transparent unless the final layer names one.
func parseBackground(ts []token, cx *ctx, v *Values) bool {
	items := splitTop(ts, tkComma)
	var b Background
	v.Background = 0
	for i, item := range items {
		haveCol := false
		l, ok := layerOf(components(trimWS(item)), cx, func(c []token) bool {
			cv, ok := parseColor(c, cx)
			if !ok || haveCol {
				return false
			}
			v.Background, haveCol = cx.resolve(cv), true
			return true
		})
		if !ok || haveCol && i != len(items)-1 {
			return false
		}
		b.Images = append(b.Images, l.Image)
		b.Sizes = append(b.Sizes, l.Size)
		b.Positions = append(b.Positions, l.Pos)
		b.Repeats = append(b.Repeats, l.Repeat)
		b.Clips = append(b.Clips, l.Clip)
		b.Origins = append(b.Origins, l.Origin)
		b.Attachments = append(b.Attachments, l.Attach)
	}
	v.Bg = b
	return true
}

// layerOf parses one layer of the background or mask shorthand: any
// order of an image, a position with an optional `/ size`, a repeat
// style, an attachment and one or two boxes (the first the origin, the
// second the clip, one box both). extra claims the shorthand's own
// components — background's final-layer color, mask's mode — and
// reports whether it took c.
func layerOf(comps [][]token, cx *ctx, extra func(c []token) bool) (ImageLayer, bool) {
	l := ImageLayer{Origin: BoxPadding}
	var haveImg, havePos, haveRep, haveAtt bool
	var boxes []LayerBox
	for i := 0; i < len(comps); {
		c := comps[i]
		if !haveImg {
			if img, ok := imageOf(c, cx); ok {
				l.Image, haveImg = img, true
				i++
				continue
			}
		}
		if !havePos {
			if n := positionRun(comps[i:], cx); n > 0 {
				l.Pos, _ = parsePosition(comps[i:i+n], cx)
				havePos = true
				i += n
				if i < len(comps) && len(comps[i]) == 1 && comps[i][0].is('/') {
					n := sizeRun(comps[i+1:], cx)
					if n == 0 {
						return ImageLayer{}, false
					}
					l.Size, _ = layerSizeOf(comps[i+1:i+1+n], cx)
					i += 1 + n
				}
				continue
			}
		}
		if !haveRep {
			if r, ok := layerRepeatOf(comps[i : i+1]); ok {
				l.Repeat, haveRep = r, true
				if i+1 < len(comps) {
					if r2, ok := layerRepeatOf(comps[i : i+2]); ok && !strings.HasPrefix(rawName(comps[i]), "repeat-") {
						l.Repeat = r2
						i++
					}
				}
				i++
				continue
			}
		}
		if s, ok := identOf([][]token{c}); ok {
			if a, ok := attachNames[s]; ok && !haveAtt {
				l.Attach, haveAtt = a, true
				i++
				continue
			}
			if bx, ok := boxNames[s]; ok && len(boxes) < 2 {
				boxes = append(boxes, bx)
				i++
				continue
			}
		}
		if extra(c) {
			i++
			continue
		}
		return ImageLayer{}, false
	}
	switch len(boxes) {
	case 1:
		l.Origin, l.Clip = boxes[0], boxes[0]
	case 2:
		l.Origin, l.Clip = boxes[0], boxes[1]
	}
	return l, true
}

// parseMask parses the mask shorthand (css-masking-1 §7.2): one layer
// of the background shorthand's grammar with the mask mode in place of
// the color. The engine positions and clips a mask in the border box.
func parseMask(ts []token, cx *ctx, v *Values) bool {
	m := Mask{}
	l, ok := layerOf(components(ts), cx, func(c []token) bool {
		s, ok := identOf([][]token{c})
		switch {
		case !ok:
			return false
		case s == "alpha" || s == "match-source":
			m.Luminance = false
		case s == "luminance":
			m.Luminance = true
		default:
			return false
		}
		return true
	})
	if !ok || l.Attach != AttachScroll {
		return false
	}
	m.Image, m.Size, m.Pos, m.Repeat = l.Image, l.Size, l.Pos, l.Repeat
	v.Mask = m
	return true
}

// positionRun is how many leading components form the longest valid
// <position>, zero for none.
func positionRun(comps [][]token, cx *ctx) int {
	for n := min(4, len(comps)); n > 0; n-- {
		if len(comps[n-1]) == 1 && comps[n-1][0].is('/') {
			continue
		}
		if _, ok := parsePosition(comps[:n], cx); ok {
			return n
		}
	}
	return 0
}

// sizeRun is how many leading components form a valid layer size.
func sizeRun(comps [][]token, cx *ctx) int {
	for n := min(2, len(comps)); n > 0; n-- {
		if _, ok := layerSizeOf(comps[:n], cx); ok {
			return n
		}
	}
	return 0
}
