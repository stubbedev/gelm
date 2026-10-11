package style

// border-image (css-backgrounds-3 §6): an image cut into nine regions
// by four slice lines, drawn over the border area grown by the outset —
// the corners scaled into the corners, the edges stretched, repeated,
// rounded or spaced along their side, the middle only under fill. A
// box whose border image loads draws it instead of its border styles.

// BorderImageLenKind is a border-image-width or -outset side's form.
type BorderImageLenKind uint8

// The forms: a multiple of the side's border width (a plain number,
// the initial 1 for widths), a length, a percentage of the border
// image area, and auto (the slice's own size).
const (
	ImageLenNumber BorderImageLenKind = iota
	ImageLenLength
	ImageLenPercent
	ImageLenAuto
)

// BorderImageLen is one side of border-image-width or -outset.
type BorderImageLen struct {
	Kind BorderImageLenKind
	V    float64
}

// BorderImage is the border-image group's computed value. Slice holds
// the four slice offsets (top, right, bottom, left) as image pixels or
// percentages of the image's size.
type BorderImage struct {
	Source ImageSource
	Slice  [4]NumVal
	Fill   bool
	Width  [4]BorderImageLen
	Outset [4]BorderImageLen
	Repeat LayerRepeat
}

// borderImageInitial is the group's initial value: none, 100% slices,
// width 1 (the border width) and no outset, stretched.
var borderImageInitial = BorderImage{
	Slice:  [4]NumVal{{V: 100, Pct: true}, {V: 100, Pct: true}, {V: 100, Pct: true}, {V: 100, Pct: true}},
	Width:  [4]BorderImageLen{{V: 1}, {V: 1}, {V: 1}, {V: 1}},
	Repeat: LayerRepeat{TileStretch, TileStretch},
}

// UsedBorderImage is v's border image with every longhand v does not
// set at its initial value.
func (v *Values) UsedBorderImage() BorderImage {
	b := borderImageInitial
	if v.Has(PropBorderImageSource) {
		b.Source = v.BorderImage.Source
	}
	if v.Has(PropBorderImageSlice) {
		b.Slice, b.Fill = v.BorderImage.Slice, v.BorderImage.Fill
	}
	if v.Has(PropBorderImageWidth) {
		b.Width = v.BorderImage.Width
	}
	if v.Has(PropBorderImageOutset) {
		b.Outset = v.BorderImage.Outset
	}
	if v.Has(PropBorderImageRepeat) {
		b.Repeat = v.BorderImage.Repeat
	}
	return b
}

// Equal reports whether two border images are the same.
func (b *BorderImage) Equal(o *BorderImage) bool {
	return b.Source.equal(o.Source) && b.Slice == o.Slice && b.Fill == o.Fill && b.Width == o.Width &&
		b.Outset == o.Outset && b.Repeat == o.Repeat
}

// fourSides expands one to four values onto top, right, bottom, left,
// the CSS box-side shorthand rule.
func fourSides[T any](vals []T) ([4]T, bool) {
	switch len(vals) {
	case 1:
		return [4]T{vals[0], vals[0], vals[0], vals[0]}, true
	case 2:
		return [4]T{vals[0], vals[1], vals[0], vals[1]}, true
	case 3:
		return [4]T{vals[0], vals[1], vals[2], vals[1]}, true
	case 4:
		return [4]T{vals[0], vals[1], vals[2], vals[3]}, true
	}
	return [4]T{}, false
}

// borderImageSliceOf reads border-image-slice: one to four numbers or
// percentages, non-negative, and the fill keyword before or after.
func borderImageSliceOf(comps [][]token, cx *ctx) ([4]NumVal, bool, bool) {
	var vals []NumVal
	fill := false
	for i, c := range comps {
		if len(c) == 1 && c[0].ident("fill") && !fill && (i == 0 || i == len(comps)-1) {
			fill = true
			continue
		}
		n, ok := evalNumeric(c, cx)
		if !ok || n.v < 0 || n.kind != numNumber && n.kind != numPercent {
			return [4]NumVal{}, false, false
		}
		vals = append(vals, NumVal{V: n.v, Pct: n.kind == numPercent})
	}
	s, ok := fourSides(vals)
	return s, fill, ok
}

// borderImageLensOf reads border-image-width (auto allowed) or
// -outset: one to four non-negative numbers, lengths or percentages
// (outsets take no percentages).
func borderImageLensOf(comps [][]token, cx *ctx, outset bool) ([4]BorderImageLen, bool) {
	var vals []BorderImageLen
	for _, c := range comps {
		if len(c) == 1 && c[0].ident("auto") && !outset {
			vals = append(vals, BorderImageLen{Kind: ImageLenAuto})
			continue
		}
		n, ok := evalNumeric(c, cx)
		if !ok || n.v < 0 {
			return [4]BorderImageLen{}, false
		}
		switch n.kind {
		case numNumber:
			vals = append(vals, BorderImageLen{Kind: ImageLenNumber, V: n.v})
		case numLength:
			vals = append(vals, BorderImageLen{Kind: ImageLenLength, V: n.v})
		case numPercent:
			if outset {
				return [4]BorderImageLen{}, false
			}
			vals = append(vals, BorderImageLen{Kind: ImageLenPercent, V: n.v})
		default:
			return [4]BorderImageLen{}, false
		}
	}
	return fourSides(vals)
}

// borderImageRepeatOf reads border-image-repeat: one or two of
// stretch, repeat, round and space (horizontal, then vertical).
func borderImageRepeatOf(comps [][]token) (LayerRepeat, bool) {
	names := map[string]TileKind{"stretch": TileStretch, "repeat": TileRepeat, "round": TileRound, "space": TileSpace}
	var out LayerRepeat
	if len(comps) < 1 || len(comps) > 2 {
		return out, false
	}
	for i := range out {
		s, ok := identOf([][]token{comps[min(i, len(comps)-1)]})
		k, known := names[s]
		if !ok || !known {
			return out, false
		}
		out[i] = k
	}
	return out, true
}

func parseBorderImageSource(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 {
		return false
	}
	img, ok := imageOf(comps[0], cx)
	if ok {
		v.BorderImage.Source = img
	}
	return ok
}

func parseBorderImageSlice(ts []token, cx *ctx, v *Values) bool {
	s, fill, ok := borderImageSliceOf(components(ts), cx)
	if ok {
		v.BorderImage.Slice, v.BorderImage.Fill = s, fill
	}
	return ok
}

func parseBorderImageWidth(ts []token, cx *ctx, v *Values) bool {
	w, ok := borderImageLensOf(components(ts), cx, false)
	if ok {
		v.BorderImage.Width = w
	}
	return ok
}

func parseBorderImageOutset(ts []token, cx *ctx, v *Values) bool {
	o, ok := borderImageLensOf(components(ts), cx, true)
	if ok {
		v.BorderImage.Outset = o
	}
	return ok
}

func parseBorderImageRepeat(ts []token, _ *ctx, v *Values) bool {
	r, ok := borderImageRepeatOf(components(ts))
	if ok {
		v.BorderImage.Repeat = r
	}
	return ok
}

// parseBorderImage parses the shorthand (css-backgrounds-3 §6.8): the
// source, the slice with an optional `/ width` and `/ outset`, and the
// repeat, in any order; every longhand resets to its initial value
// first.
func parseBorderImage(ts []token, cx *ctx, v *Values) bool {
	b := borderImageInitial
	comps := components(ts)
	var haveSrc, haveSlice, haveRep bool
	for i := 0; i < len(comps); {
		if !haveSrc {
			if img, ok := imageOf(comps[i], cx); ok {
				b.Source, haveSrc = img, true
				i++
				continue
			}
		}
		if !haveRep {
			if n := min(2, len(comps)-i); n > 0 {
				if r, ok := borderImageRepeatOf(comps[i : i+n]); ok {
					b.Repeat, haveRep = r, true
					i += n
					continue
				}
				if r, ok := borderImageRepeatOf(comps[i : i+1]); ok {
					b.Repeat, haveRep = r, true
					i++
					continue
				}
			}
		}
		if haveSlice {
			return false
		}
		end := i
		for end < len(comps) && !isSlash(comps[end]) && !isImageOrRepeat(comps[end], cx) {
			end++
		}
		s, fill, ok := borderImageSliceOf(comps[i:end], cx)
		if !ok {
			return false
		}
		b.Slice, b.Fill, haveSlice = s, fill, true
		i = end
		for part := 0; part < 2 && i < len(comps) && isSlash(comps[i]); part++ {
			i++
			end := i
			for end < len(comps) && !isSlash(comps[end]) && !isImageOrRepeat(comps[end], cx) {
				end++
			}
			if end == i {
				if part == 1 {
					return false
				}
				continue
			}
			lens, ok := borderImageLensOf(comps[i:end], cx, part == 1)
			if !ok {
				return false
			}
			if part == 0 {
				b.Width = lens
			} else {
				b.Outset = lens
			}
			i = end
		}
	}
	v.BorderImage = b
	return true
}

// isSlash reports a lone `/` component.
func isSlash(c []token) bool { return len(c) == 1 && c[0].is('/') }

// isImageOrRepeat reports a component that starts the shorthand's
// source or repeat, ending a slice or width run.
func isImageOrRepeat(c []token, cx *ctx) bool {
	if _, ok := borderImageRepeatOf([][]token{c}); ok {
		return true
	}
	if len(c) == 1 && c[0].ident("none") {
		return true
	}
	_, ok := imageOf(c, cx)
	return ok
}
