package widget

// The rendering-effect paint pipeline (#159): filter,
// backdrop-filter, mix-blend-mode, isolation, clip-path and mask
// applied to a widget's painted result. The no-effect frame keeps the
// old path — opacity through PushAlpha, brightness through
// PushBrightness, no layer — so a stylesheet without effects paints
// exactly as before, allocation-free. Anything heavier paints the
// widget offscreen (a Canvas Layer reused across frames) and
// composites the layer back through the effect chain: filters, then
// the clip-path mask, then the mask-image mask, then the blend mode.

import (
	"image"
	"image/color"
	"math"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// effects is the pushed effect state: the canvas to paint into (the
// layer's when any effect forced one, the caller's otherwise) and
// what pop owes to composite back.
type effects struct {
	cv     *render.Canvas
	dst    *render.Canvas
	n      *node
	layer  *render.Layer
	region render.Rect
	v      *style.Values
	bounds render.Rect
	radii  render.Corners
	pa, pb float64
	op, br bool
	blend  render.BlendMode
}

// effectsHeavy reports whether v's effect declarations need the
// offscreen layer: a filter beyond brightness, a clip-path, a mask, a
// blend mode other than normal, or isolation (whose group the
// descendants' blends then land on).
func effectsHeavy(v *style.Values) bool {
	if v.Has(style.PropFilter) && !v.Filter.Empty() {
		if _, only := v.Filter.BrightnessOnly(); !only {
			return true
		}
	}
	if v.Has(style.PropClipPath) && !v.Clip.Empty() {
		return true
	}
	if v.Has(style.PropMaskImage) && v.Mask.Kind != style.MaskNone {
		return true
	}
	if v.Has(style.PropMixBlendMode) && v.Blend != render.BlendNormal {
		return true
	}
	return v.Has(style.PropIsolation) && v.Isolate
}

// pushEffects applies the subtree effects — opacity, brightness, and
// the rendering effects of #159 — and returns the state pop
// composites with. n is the widget's own node (the layer cache lives
// there); bounds and radii are the border box the effects apply to.
// The returned canvas (fx.cv) is what the caller paints into.
func pushEffects(cv *render.Canvas, n *node, v *style.Values, bounds render.Rect, radii render.Corners) effects {
	e := effects{cv: cv, dst: cv, n: n, v: v, bounds: bounds, radii: radii}
	if v.Has(style.PropOpacity) && v.Opacity != 1 {
		e.op = true
	}
	if v.Has(style.PropFilter) && v.Brightness != 1 {
		if _, only := v.Filter.BrightnessOnly(); only || v.Filter.Empty() {
			e.br = true
		}
	}
	// backface-visibility on the flat pipeline: a transform whose
	// composed map flips the plane hides the widget entirely, painted
	// at zero opacity so the hidden case allocates nothing.
	if v.Has(style.PropBackfaceVisibility) && v.BackfaceHidden &&
		v.Has(style.PropTransform) && v.Transform.Backface {
		e.pa = cv.PushAlpha(0)
		e.op = true
		return e
	}
	// The backdrop filters first — before any early return — because
	// they sample what has been painted behind the widget so far this
	// frame, whether or not the widget itself needs a layer.
	if v.Has(style.PropBackdropFilter) && !v.Backdrop.Empty() {
		paintBackdrop(cv, n, v, bounds, radii)
	}
	if !effectsHeavy(v) {
		e.pushFast(cv, v)
		return e
	}
	e.region = cv.MapRect(v.Filter.Extent().Grow(bounds)).Intersect(cv.Rect())
	e.layer = cv.Layer(n.fx, e.region)
	n.fx = e.layer
	e.cv = e.layer.Canvas()
	e.pushFast(e.cv, v)
	if v.Has(style.PropMixBlendMode) {
		e.blend = v.Blend
	}
	return e
}

// pushFast pushes only the cheap effects — opacity, brightness, and
// the hidden backface — onto cv; popFast restores them. The transform
// pipeline paints through this onto its own layer, then composites
// with finishEffects.
func (e *effects) pushFast(cv *render.Canvas, v *style.Values) {
	e.cv = cv
	if e.op {
		e.pa = cv.PushAlpha(v.Opacity)
	}
	if e.br {
		e.pb = cv.PushBrightness(v.Brightness)
	}
	if v.Has(style.PropBackfaceVisibility) && v.BackfaceHidden &&
		v.Has(style.PropTransform) && v.Transform.Backface {
		e.pa = cv.PushAlpha(0)
		e.op = true
	}
}

// popFast restores pushFast's pushes.
func (e *effects) popFast() {
	if e.br {
		e.cv.PopBrightness(e.pb)
	}
	if e.op {
		e.cv.PopAlpha(e.pa)
	}
}

// pop restores what pushEffects pushed: the fast path pops in place,
// the heavy path filters the layer, applies the clip and mask, and
// blends it back onto the canvas.
func (e *effects) pop() {
	e.popFast()
	if e.layer != nil {
		finishEffects(e.dst, e.n, e.v, e.bounds, e.radii, e.layer, e.region, e.blend, nil)
	}
}

// finishEffects composites a painted layer through the heavy effect
// chain: the filters, the clip-path mask, the mask-image mask, then
// the blend-mode draw — or the projective composite when p3 carries a
// 3-D transform (the perspective divide replaces the one-to-one draw).
func finishEffects(cv *render.Canvas, n *node, v *style.Values, bounds render.Rect, radii render.Corners,
	layer *render.Layer, region render.Rect, mode render.BlendMode, p3 *render.M3,
) {
	if v.BackfaceHidden && v.Has(style.PropTransform) && v.Transform.Backface {
		return
	}
	if !v.Filter.Empty() {
		render.ApplyFilters(layer, region, &v.Filter)
	}
	if v.Has(style.PropClipPath) && !v.Clip.Empty() {
		if p, ok := clipShapePath(&v.Clip, bounds, radii); ok {
			layer.ApplyPathMask(p, region)
		}
	}
	if v.Has(style.PropMaskImage) && v.Mask.Kind != style.MaskNone {
		applyMaskLayer(cv, n, v, layer, region, bounds)
	}
	if p3 != nil {
		cv.CompositeProjective(layer, region, *p3, 1)
		return
	}
	cv.DrawLayer(layer, region, mode)
}

// paintBackdrop applies backdrop-filter: the canvas region under the
// widget's box (grown by the filters' extent) is snapshotted,
// filtered, clipped to the rounded border box, and composited back
// over the unfiltered original — the filtered half replacing the
// backdrop inside the shape, the original showing outside it.
func paintBackdrop(cv *render.Canvas, n *node, v *style.Values, bounds render.Rect, radii render.Corners) {
	region := cv.MapRect(v.Backdrop.Extent().Grow(bounds)).Intersect(cv.Rect())
	if region.Empty() {
		return
	}
	snap := cv.Snapshot(n.fxb, region)
	n.fxb = snap
	filt := cv.Layer(n.fxf, region)
	n.fxf = filt
	filt.Canvas().DrawCanvasDevice(snap.Canvas(), 0, 0)
	render.ApplyFilters(filt, region, &v.Backdrop)
	p := &render.Path{}
	render.RoundedRectPath(p, bounds, radii)
	filt.ApplyPathMask(p, region)
	cv.ClearDevice(region, 0)
	cv.DrawLayer(snap, region, render.BlendNormal)
	cv.DrawLayer(filt, region, render.BlendNormal)
}

// clipShapePath builds the clip shape's geometry against the border
// box (logical pixels): the inset rect with its rounded corners, the
// circle and ellipse, the polygon, or the parsed path() data offset
// onto the box's origin.
func clipShapePath(c *style.ClipShape, bounds render.Rect, radii render.Corners) (*render.Path, bool) {
	p := &render.Path{}
	w, h := float64(bounds.W), float64(bounds.H)
	circleR := func(nv style.NumVal) float64 {
		if nv.Pct {
			return nv.V / 100 * mathSqrtDiag(w, h)
		}
		return nv.V
	}
	switch c.Kind {
	case style.ClipInset:
		t := c.Inset[0].Resolve(h)
		r := c.Inset[1].Resolve(w)
		b := c.Inset[2].Resolve(h)
		l := c.Inset[3].Resolve(w)
		in := render.Rect{
			X: bounds.X + int(math.Round(l)), Y: bounds.Y + int(math.Round(t)),
			W: max(0, bounds.W-int(math.Round(l+r))), H: max(0, bounds.H-int(math.Round(t+b))),
		}
		cr := render.Corners{
			TopLeft: c.InsetRad[0], TopRight: c.InsetRad[1],
			BottomRight: c.InsetRad[2], BottomLeft: c.InsetRad[3],
		}
		if cr == (render.Corners{}) {
			cr = radii
		}
		render.RoundedRectPath(p, in, cr)
	case style.ClipCircle:
		cx := float64(bounds.X) + c.CenterX.Resolve(w)
		cy := float64(bounds.Y) + c.CenterY.Resolve(h)
		render.EllipsePath(p, cx, cy, circleR(c.Rad), circleR(c.Rad))
	case style.ClipEllipse:
		rx, ry := circleR(c.RadX), circleR(c.RadY)
		cx := float64(bounds.X) + c.CenterX.Resolve(w)
		cy := float64(bounds.Y) + c.CenterY.Resolve(h)
		render.EllipsePath(p, cx, cy, rx, ry)
	case style.ClipPolygon:
		for i := 0; i+1 < len(c.Poly); i += 2 {
			x := float64(bounds.X) + c.Poly[i].Resolve(w)
			y := float64(bounds.Y) + c.Poly[i+1].Resolve(h)
			if i == 0 {
				p.MoveTo(x, y)
			} else {
				p.LineTo(x, y)
			}
		}
		p.Close()
	case style.ClipPathData:
		d, ok := render.ParsePathData(c.Path)
		if !ok {
			return nil, false
		}
		p = d.Offset(float64(bounds.X), float64(bounds.Y))
	default:
		return nil, false
	}
	if p.Empty() {
		return nil, false
	}
	return p, true
}

// mathSqrtDiag is sqrt(w²+h²)/sqrt(2): CSS's reference for a
// percentage circle radius.
func mathSqrtDiag(w, h float64) float64 {
	return math.Sqrt(w*w+h*h) / math.Sqrt2
}

// applyMaskLayer paints the mask-image into its own layer over the
// effect region and multiplies the widget layer's pixels by it: the
// alpha channel for mask-mode alpha, the WCAG luminance for
// luminance.
func applyMaskLayer(cv *render.Canvas, n *node, v *style.Values, layer *render.Layer, region, bounds render.Rect) {
	m := &v.Mask
	mask := cv.Layer(n.fxmask, region)
	n.fxmask = mask
	mc := mask.Canvas()
	mc.ClearDevice(region, 0)
	rect := maskRect(v, bounds)
	switch m.Kind {
	case style.MaskGradient:
		g := &m.Image
		var buf [style.MaxStops]render.GradientStop
		stops := buf[:g.N]
		for i := range g.N {
			stops[i] = render.GradientStop{Pos: g.Stops[i].Pos, Color: g.Stops[i].Color}
		}
		rg := cssGradient(g, stops)
		if m.Repeat {
			rg.Extend = render.ExtendRepeat
		}
		prev := mc.PushClipDevice(region)
		mc.PaintGradient(rect, render.Corners{}, rg)
		mc.PopClip(prev)
	case style.MaskURL:
		if ic := bgImageFor(m.URL); ic != nil {
			mc.DrawImageRect(iconAsImage(ic), rect)
		}
	}
	layer.ApplyMaskLayer(mask, region, m.Luminance)
}

// iconAsImage adapts an Icon to image.Image through its At sampler.
func iconAsImage(ic *render.Icon) image.Image {
	w, h := ic.Size()
	return &iconImage{ic: ic, w: w, h: h}
}

// iconImage is Icon's image.Image face.
type iconImage struct {
	ic   *render.Icon
	w, h int
}

// ColorModel implements image.Image.
func (m *iconImage) ColorModel() color.Model { return color.RGBAModel }

// Bounds implements image.Image.
func (m *iconImage) Bounds() image.Rectangle {
	return image.Rect(0, 0, m.w, m.h)
}

// At implements image.Image.
func (m *iconImage) At(x, y int) color.Color {
	if x < 0 || y < 0 || x >= m.w || y >= m.h {
		return color.RGBA{}
	}
	return m.ic.At(x, y)
}

// maskRect resolves the mask's size and position against the border
// box: auto keeps a gradient on the box (an image its natural size),
// cover and contain scale the standard way, and explicit lengths or
// percentages win. The position is the offset of the sized tile from
// the box's origin, percentages of the leftover space.
func maskRect(v *style.Values, bounds render.Rect) render.Rect {
	m := &v.Mask
	w, h := float64(bounds.W), float64(bounds.H)
	switch m.Kind {
	case style.MaskGradient:
		if m.SizeKind == style.MaskSizeAuto && !m.Repeat {
			return bounds
		}
	}
	if m.Kind == style.MaskURL {
		if ic := bgImageFor(m.URL); ic != nil {
			iw, ih := ic.Size()
			nw, nh := float64(iw), float64(ih)
			switch m.SizeKind {
			case style.MaskSizeAuto:
				w, h = nw, nh
			case style.MaskSizeLength:
				w, h = m.SizeW.Resolve(w), m.SizeH.Resolve(h)
			case style.MaskSizeCover:
				s := math.Max(w/nw, h/nh)
				w, h = nw*s, nh*s
			case style.MaskSizeContain:
				s := math.Min(w/nw, h/nh)
				w, h = nw*s, nh*s
			}
		}
	} else if m.SizeKind == style.MaskSizeLength {
		w, h = m.SizeW.Resolve(w), m.SizeH.Resolve(h)
	}
	var x, y float64
	if m.PosX.Pct {
		x = float64(bounds.X) + (float64(bounds.W)-w)*m.PosX.V/100
	} else {
		x = float64(bounds.X) + m.PosX.V
	}
	if m.PosY.Pct {
		y = float64(bounds.Y) + (float64(bounds.H)-h)*m.PosY.V/100
	} else {
		y = float64(bounds.Y) + m.PosY.V
	}
	return render.Rect{X: int(math.Round(x)), Y: int(math.Round(y)), W: max(1, int(math.Round(w))), H: max(1, int(math.Round(h)))}
}
