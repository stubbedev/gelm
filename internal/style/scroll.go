package style

// The CSS scroll properties: scroll-behavior, scroll-margin,
// scroll-padding, the scroll-snap group, overscroll-behavior, and the
// scrollbar-color/width/gutter trio. The container properties
// (behavior, padding, snap-type, overscroll, gutter) style the Scroll
// viewport; the child properties (margin, snap-align, snap-stop) style
// what it scrolls. The widget layer reads them where it reveals,
// snaps, and chains overscroll; nothing here touches layout.

import (
	"strings"
)

// SnapAxis is scroll-snap-type's axis half: which axes the container
// snaps. Block and Inline map onto the vertical and horizontal axes of
// gelm's horizontal-tb world.
type SnapAxis uint8

// The scroll-snap-type axes.
const (
	SnapAxisNone SnapAxis = iota
	SnapAxisX
	SnapAxisY
	SnapAxisBlock
	SnapAxisInline
	SnapAxisBoth
)

// SnapType is the computed scroll-snap-type: the axes that snap and
// how strictly. The zero value snaps nothing.
type SnapType struct {
	Axis SnapAxis
	// Mandatory is the mandatory strictness; proximity otherwise.
	Mandatory bool
}

// Vertical reports whether the y axis snaps.
func (t SnapType) Vertical() bool {
	return t.Axis == SnapAxisY || t.Axis == SnapAxisBlock || t.Axis == SnapAxisBoth
}

// Horizontal reports whether the x axis snaps.
func (t SnapType) Horizontal() bool {
	return t.Axis == SnapAxisX || t.Axis == SnapAxisInline || t.Axis == SnapAxisBoth
}

// SnapAlignK is one scroll-snap-align edge.
type SnapAlignK uint8

// The scroll-snap-align edges.
const (
	SnapNone SnapAlignK = iota
	SnapStart
	SnapEnd
	SnapCenter
)

// SnapAlign is the computed scroll-snap-align: the block (vertical)
// and inline (horizontal) alignments of one scrolled child.
type SnapAlign struct {
	Block, Inline SnapAlignK
}

// Overscroll is one axis's overscroll-behavior.
type Overscroll uint8

// The overscroll-behavior values.
const (
	OverscrollAuto Overscroll = iota
	OverscrollContain
	OverscrollNone
)

// The scrollbar-width values.
const (
	BarWidthAuto uint8 = iota
	BarWidthThin
	BarWidthNone
)

// The scrollbar-gutter flags: Stable reserves the bar's space even
// without overflow, BothEdges reserves a matching space on the
// opposite edge too.
const (
	GutterStable uint8 = 1 << iota
	GutterBothEdges
)

// parseScrollBehavior parses scroll-behavior: auto or smooth.
func parseScrollBehavior(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 || comps[0][0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(comps[0][0].s) {
	case "auto":
		v.ScrollSmooth = false
	case "smooth":
		v.ScrollSmooth = true
	default:
		return false
	}
	return true
}

// parseScrollMargin parses the scroll-margin shorthand, negative
// lengths allowed like margin.
func parseScrollMargin(ts []token, cx *ctx, v *Values) bool {
	return sidesInto(func(v *Values) *Sides { return &v.ScrollMargin }, true)(ts, cx, v)
}

// parseScrollPadding parses the scroll-padding shorthand.
func parseScrollPadding(ts []token, cx *ctx, v *Values) bool {
	return sidesInto(func(v *Values) *Sides { return &v.ScrollPad }, false)(ts, cx, v)
}

// parseScrollSnapType parses scroll-snap-type: none, or an axis
// (x, y, block, inline, both) with an optional strictness
// (proximity, mandatory).
func parseScrollSnapType(ts []token, cx *ctx, v *Values) bool {
	st := SnapType{}
	sawAxis := false
	for _, c := range components(ts) {
		if c[0].kind != tkIdent {
			return false
		}
		switch strings.ToLower(c[0].s) {
		case "none":
			if sawAxis {
				return false
			}
		case "x", "inline":
			if sawAxis {
				return false
			}
			st.Axis, sawAxis = SnapAxisX, true
		case "y", "block":
			if sawAxis {
				return false
			}
			st.Axis, sawAxis = SnapAxisY, true
		case "both":
			if sawAxis {
				return false
			}
			st.Axis, sawAxis = SnapAxisBoth, true
		case "proximity":
			st.Mandatory = false
		case "mandatory":
			st.Mandatory = true
		default:
			return false
		}
	}
	v.Snap = st
	return true
}

// parseScrollSnapAlign parses scroll-snap-align: one value for both
// axes, or the block then inline value.
func parseScrollSnapAlign(ts []token, cx *ctx, v *Values) bool {
	var ks []SnapAlignK
	for _, c := range components(ts) {
		if c[0].kind != tkIdent {
			return false
		}
		switch strings.ToLower(c[0].s) {
		case "none":
			ks = append(ks, SnapNone)
		case "start":
			ks = append(ks, SnapStart)
		case "end":
			ks = append(ks, SnapEnd)
		case "center":
			ks = append(ks, SnapCenter)
		default:
			return false
		}
	}
	switch len(ks) {
	case 1:
		v.SnapAlign = SnapAlign{Block: ks[0], Inline: ks[0]}
	case 2:
		v.SnapAlign = SnapAlign{Block: ks[0], Inline: ks[1]}
	default:
		return false
	}
	return true
}

// parseScrollSnapStop parses scroll-snap-stop: normal or always.
func parseScrollSnapStop(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 || comps[0][0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(comps[0][0].s) {
	case "normal":
		v.SnapStopAlways = false
	case "always":
		v.SnapStopAlways = true
	default:
		return false
	}
	return true
}

// parseOverscroll parses overscroll-behavior or one axis longhand:
// auto, contain or none, one value or the x then y pair.
func parseOverscroll(ts []token, _ *ctx, v *Values, axis int) bool {
	var vals []Overscroll
	for _, c := range components(ts) {
		if c[0].kind != tkIdent {
			return false
		}
		switch strings.ToLower(c[0].s) {
		case "auto":
			vals = append(vals, OverscrollAuto)
		case "contain":
			vals = append(vals, OverscrollContain)
		case "none":
			vals = append(vals, OverscrollNone)
		default:
			return false
		}
	}
	if len(vals) < 1 || len(vals) > 2 {
		return false
	}
	if axis == 0 {
		// The shorthand: one value covers both axes, the second is y.
		v.Overscroll[0] = vals[0]
		v.Overscroll[1] = vals[len(vals)-1]
	} else {
		v.Overscroll[1] = vals[0]
	}
	return true
}

// parseScrollbarColor parses scrollbar-color: auto or two colors
// (thumb then track).
func parseScrollbarColor(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) == 1 && comps[0][0].kind == tkIdent && strings.EqualFold(comps[0][0].s, "auto") {
		v.BarColorAuto = true
		v.BarThumb, v.BarTrack = 0, 0
		return true
	}
	if len(comps) != 2 {
		return false
	}
	thumb, ok := parseColor(comps[0], cx)
	if !ok {
		return false
	}
	track, ok := parseColor(comps[1], cx)
	if !ok {
		return false
	}
	v.BarColorAuto = false
	v.BarThumb, v.BarTrack = cx.resolve(thumb), cx.resolve(track)
	return true
}

// parseScrollbarWidth parses scrollbar-width: auto, thin or none.
func parseScrollbarWidth(ts []token, cx *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 || comps[0][0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(comps[0][0].s) {
	case "auto":
		v.BarWidth = BarWidthAuto
	case "thin":
		v.BarWidth = BarWidthThin
	case "none":
		v.BarWidth = BarWidthNone
	default:
		return false
	}
	return true
}

// parseScrollbarGutter parses scrollbar-gutter: auto, or stable with
// an optional both-edges, in either order.
func parseScrollbarGutter(ts []token, cx *ctx, v *Values) bool {
	var g uint8
	saw := false
	for _, c := range components(ts) {
		if c[0].kind != tkIdent {
			return false
		}
		switch strings.ToLower(c[0].s) {
		case "auto":
			if saw {
				return false
			}
		case "stable":
			if g&GutterStable != 0 {
				return false
			}
			g |= GutterStable
			saw = true
		case "both-edges":
			if g&GutterBothEdges != 0 {
				return false
			}
			g |= GutterBothEdges
			saw = true
		default:
			return false
		}
	}
	if g&GutterStable == 0 {
		g = 0
	}
	v.BarGutter = g
	return true
}
