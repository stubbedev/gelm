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
// box (GTK's rule; under box-sizing: border-box they floor the border
// box instead), and the insets add back. The result lies within con.
func measureBox(v *style.Values, b cssInsets, con Constraints, content func(Constraints) Size) Size {
	o := b.outer()
	h, vv := o.Left+o.Right, o.Top+o.Bottom
	inner := Constraints{
		Min: Size{W: max(0, con.Min.W-h), H: max(0, con.Min.H-vv)},
		Max: Size{W: max(0, con.Max.W-h), H: max(0, con.Max.H-vv)},
	}
	sz := content(inner)
	minW, minH := picki(v, style.PropMinWidth, 0), picki(v, style.PropMinHeight, 0)
	if v.Layout.BoxSizing == style.BoxSizingBorder {
		minW -= b.border.Left + b.border.Right + b.padding.Left + b.padding.Right
		minH -= b.border.Top + b.border.Bottom + b.padding.Top + b.padding.Bottom
	}
	sz.W = max(sz.W, minW)
	sz.H = max(sz.H, minH)
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

// cssGradient is the render gradient a background-image declares,
// painted over box: its stops resolve against the gradient line the
// box gives it, into buf.
func cssGradient(g *style.Gradient, box render.Rect, buf *[style.MaxResolvedStops]render.GradientStop) render.Gradient {
	out := render.Gradient{
		Kind: g.Kind, Angle: g.Angle,
		CenterX: g.CenterX, CenterY: g.CenterY,
		Circle: g.Circle, Size: g.Size, RadiusX: g.RadiusX, RadiusY: g.RadiusY,
	}
	if g.Repeat {
		out.Extend = render.ExtendRepeat
	}
	out.Stops = g.Resolve(out.LineLength(box), buf[:0])
	return out
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
	paintBackground(cv, v, border, radii, bw, bg)
	inner := bw.Shrink(border)
	innerRadii := insetCorners(radii, bw)
	for _, sh := range slices.Backward(shadows) {
		if sh.Inset {
			cv.BoxShadow(inner, innerRadii, toRenderShadow(sh))
		}
	}
	if !paintBorderImage(cv, v, border, bw) && !bw.Zero() {
		cv.RoundedBorderSides(border, radii, bw, cols)
	}
}

// paintOutline strokes the CSS outline around the border box, offset
// outward by outline-offset, above the content.
func paintOutline(cv *render.Canvas, v *style.Values, border render.Rect, radii render.Corners) {
	w := outlineWidth(v)
	if w <= 0 {
		return
	}
	grow := w + v.OutlineOffset
	r := render.UniformInsets(grow).Grow(border)
	outer := render.Corners{
		TopLeft: max(0, radii.TopLeft+grow), TopRight: max(0, radii.TopRight+grow),
		BottomRight: max(0, radii.BottomRight+grow), BottomLeft: max(0, radii.BottomLeft+grow),
	}
	cv.RoundedBorder(r, outer, render.UniformInsets(w), outlineColor(v))
}

// FocusRingWidth is the width of the keyboard focus ring: the window's
// ring around the focused widget, and an outline-style: auto outline
// that declares no width of its own.
const FocusRingWidth = 2

// outlineWidth is the used outline width: zero without a visible
// style, the focus ring's width for an auto outline declaring none.
func outlineWidth(v *style.Values) int {
	w := v.EffOutline()
	if w == 0 && v.OutlineStyle == style.BorderAuto {
		return FocusRingWidth
	}
	return w
}

// outlineColor is the outline's used color (css-ui-4 §3.4): the
// declared color, else for outline-color auto the focus ring's accent
// under an auto style and currentColor under another, the theme accent
// where no color is set at all.
func outlineColor(v *style.Values) render.Color {
	switch {
	case v.Has(style.PropOutlineColor) && !v.OutlineAuto:
		return v.OutlineColor
	case v.OutlineStyle == style.BorderAuto:
		return accentOf(v, Current().Accent)
	}
	return pickc(0, v, style.PropColor, Current().Accent)
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

// hasBoxLayers reports whether the cascade paints anything behind the
// content beyond a background color: an image, shadows, a border, or
// an outline.
func hasBoxLayers(v *style.Values) bool {
	return v.Bg.Painted() || v.Shadow.N > 0 || !borderOf(v).Zero() || outlineWidth(v) > 0 || v.Has(style.PropBorderImageSource)
}
