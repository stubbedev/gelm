// The CSS box model the styled widgets share: a margin box a container
// arranges, the border box the widget owns (its Bounds, hit area, and
// damage), and the content box its children or text fill — with
// padding, per-side borders, min sizes, and the background layers
// painted in GTK's order. Margins are self-applied: a widget measures
// its margin box and insets itself inside whatever rect its container
// hands it, so every container gets margins for free.
package widget

import (
	"slices"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// cssInsets is one widget's resolved box: margin, border, and padding.
type cssInsets struct {
	margin, border, padding render.Insets
}

// boxOf resolves the box for a cascade; defPad is the widget's own
// padding where the stylesheet sets none.
func boxOf(v *style.Values, defPad render.Insets) cssInsets {
	return cssInsets{margin: marginOf(v), border: borderOf(v), padding: paddingOr(v, defPad)}
}

// outer is margin + border + padding: everything between the margin
// box and the content box.
func (b cssInsets) outer() render.Insets {
	return render.Insets{
		Top:    b.margin.Top + b.border.Top + b.padding.Top,
		Right:  b.margin.Right + b.border.Right + b.padding.Right,
		Bottom: b.margin.Bottom + b.border.Bottom + b.padding.Bottom,
		Left:   b.margin.Left + b.border.Left + b.padding.Left,
	}
}

// measureBox measures a CSS box: the content measures under the
// constraints less the insets, min-width/min-height floor the content
// box (GTK's rule), and the insets add back. The result lies within
// con.
func measureBox(v *style.Values, b cssInsets, con Constraints, content func(Constraints) Size) Size {
	o := b.outer()
	h, vv := o.Left+o.Right, o.Top+o.Bottom
	inner := Constraints{
		Min: Size{W: max(0, con.Min.W-h), H: max(0, con.Min.H-vv)},
		Max: Size{W: max(0, con.Max.W-h), H: max(0, con.Max.H-vv)},
	}
	sz := content(inner)
	sz.W = max(sz.W, picki(v, style.PropMinWidth, 0))
	sz.H = max(sz.H, picki(v, style.PropMinHeight, 0))
	return clampSize(Size{W: sz.W + h, H: sz.H + vv}, con)
}

// boxRects splits an arranged margin box into the border box and the
// content box.
func boxRects(b cssInsets, r render.Rect) (border, content render.Rect) {
	border = b.margin.Shrink(r)
	content = b.padding.Shrink(b.border.Shrink(border))
	return border, content
}

// borderColors returns the per-side border colors: the stylesheet's,
// else currentColor, else the theme's border ink.
func borderColors(v *style.Values) [4]render.Color {
	sides := [4]style.Prop{style.PropBorderTopColor, style.PropBorderRightColor, style.PropBorderBottomColor, style.PropBorderLeftColor}
	fallback := pickc(0, v, style.PropColor, Current().Border)
	var out [4]render.Color
	for i, p := range sides {
		if v.Has(p) {
			out[i] = v.BorderColor[i]
		} else {
			out[i] = fallback
		}
	}
	return out
}

// paintBoxBehind paints a CSS box's layers under its content, in GTK's
// order: outer shadows, the background color (bg, resolved by the
// caller from its programmatic, stylesheet, and theme origins), the
// background image, inset shadows, and the border.
func paintBoxBehind(cv *render.Canvas, v *style.Values, border render.Rect, radii render.Corners, bw render.Insets, bg render.Color) {
	paintBoxBehindCol(cv, v, border, radii, bw, bg, borderColors(v))
}

// paintBoxBehindCol is paintBoxBehind with explicit border colors: a
// widget's themed fallback where the cascade names none.
func paintBoxBehindCol(cv *render.Canvas, v *style.Values, border render.Rect, radii render.Corners, bw render.Insets, bg render.Color, cols [4]render.Color) {
	shadows := v.Shadow.List()
	for _, sh := range slices.Backward(shadows) { // the first shadow is on top
		if !sh.Inset {
			cv.BoxShadow(border, radii, toRenderShadow(sh))
		}
	}
	if bg != 0 {
		cv.RoundedRectCorners(border, radii, bg)
	}
	if g := &v.Image; g.N > 0 {
		var buf [style.MaxStops]render.GradientStop
		stops := buf[:g.N]
		for i := range g.N {
			stops[i] = render.GradientStop{Pos: g.Stops[i].Pos, Color: g.Stops[i].Color}
		}
		cv.FillGradient(border, radii, g.Angle, stops)
	}
	if img := bgImageFor(v.BgImageURL); img != nil {
		cv.DrawImageCoverImage(img, border, radii)
	}
	inner := bw.Shrink(border)
	innerRadii := insetCorners(radii, bw)
	for _, sh := range slices.Backward(shadows) {
		if sh.Inset {
			cv.BoxShadow(inner, innerRadii, toRenderShadow(sh))
		}
	}
	if !bw.Zero() {
		cv.RoundedBorderSides(border, radii, bw, cols)
	}
}

// paintOutline strokes the CSS outline around the border box, offset
// outward by outline-offset, above the content.
func paintOutline(cv *render.Canvas, v *style.Values, border render.Rect, radii render.Corners) {
	w := v.EffOutline()
	if w <= 0 {
		return
	}
	grow := w + v.OutlineOffset
	r := render.UniformInsets(grow).Grow(border)
	outer := render.Corners{
		TopLeft: max(0, radii.TopLeft+grow), TopRight: max(0, radii.TopRight+grow),
		BottomRight: max(0, radii.BottomRight+grow), BottomLeft: max(0, radii.BottomLeft+grow),
	}
	col := pickc(0, v, style.PropOutlineColor, pickc(0, v, style.PropColor, Current().Accent))
	cv.RoundedBorder(r, outer, render.UniformInsets(w), col)
}

// insetCorners shrinks radii by the adjacent border widths: the padding
// box's corners.
func insetCorners(c render.Corners, b render.Insets) render.Corners {
	return render.Corners{
		TopLeft:     max(0, c.TopLeft-max(b.Top, b.Left)),
		TopRight:    max(0, c.TopRight-max(b.Top, b.Right)),
		BottomRight: max(0, c.BottomRight-max(b.Bottom, b.Right)),
		BottomLeft:  max(0, c.BottomLeft-max(b.Bottom, b.Left)),
	}
}

func toRenderShadow(sh style.Shadow) render.BoxShadow {
	return render.BoxShadow{X: sh.X, Y: sh.Y, Blur: sh.Blur, Spread: sh.Spread, Color: sh.Color, Inset: sh.Inset}
}

// effects is a pushed opacity/brightness pair, restored by pop.
type effects struct {
	pa, pb float64
	op, br bool
}

// pushEffects applies the subtree effects — opacity and the brightness
// filter — for the matching pop. Both are no-ops unless the stylesheet
// set them.
func pushEffects(cv *render.Canvas, v *style.Values) effects {
	e := effects{
		op: v.Has(style.PropOpacity) && v.Opacity != 1,
		br: v.Has(style.PropFilter) && v.Brightness != 1,
	}
	if e.op {
		e.pa = cv.PushAlpha(v.Opacity)
	}
	if e.br {
		e.pb = cv.PushBrightness(v.Brightness)
	}
	return e
}

// pop restores what pushEffects pushed.
func (e effects) pop(cv *render.Canvas) {
	if e.br {
		cv.PopBrightness(e.pb)
	}
	if e.op {
		cv.PopAlpha(e.pa)
	}
}

// hasBoxLayers reports whether the cascade paints anything behind the
// content beyond a background color: an image, shadows, a border, or
// an outline.
func hasBoxLayers(v *style.Values) bool {
	return v.Image.N > 0 || v.Shadow.N > 0 || !borderOf(v).Zero() || v.EffOutline() > 0
}
