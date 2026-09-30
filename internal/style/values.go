package style

import (
	"slices"

	"github.com/stubbedev/gelm/render"
)

// Prop names one longhand property of the docs/css.md table. Shorthands
// (padding, margin, border, border-radius, background, outline,
// transition, all) expand onto these at parse time; the cascade runs
// per longhand, exactly like CSS.
type Prop uint8

// The longhand properties, as bit indexes into PropSet.
const (
	PropColor Prop = iota
	PropBackgroundColor
	PropBackgroundImage
	PropOpacity
	PropFilter
	PropPaddingTop
	PropPaddingRight
	PropPaddingBottom
	PropPaddingLeft
	PropMarginTop
	PropMarginRight
	PropMarginBottom
	PropMarginLeft
	PropBorderTopWidth
	PropBorderRightWidth
	PropBorderBottomWidth
	PropBorderLeftWidth
	PropBorderTopStyle
	PropBorderRightStyle
	PropBorderBottomStyle
	PropBorderLeftStyle
	PropBorderTopColor
	PropBorderRightColor
	PropBorderBottomColor
	PropBorderLeftColor
	PropBorderTopLeftRadius
	PropBorderTopRightRadius
	PropBorderBottomRightRadius
	PropBorderBottomLeftRadius
	PropBoxShadow
	PropOutlineWidth
	PropOutlineStyle
	PropOutlineColor
	PropOutlineOffset
	PropMinWidth
	PropMinHeight
	PropBorderSpacing
	PropFontFamily
	PropFontSize
	PropFontWeight
	PropFontStyle
	PropLetterSpacing
	PropTextTransform
	PropIconSize
	PropTransitionProperty
	PropTransitionDuration
	PropTransitionTiming
	PropTransitionDelay
	numProps
)

// PropSet is a set of longhands.
type PropSet uint64

// Has reports whether p is in the set.
func (s PropSet) Has(p Prop) bool { return s&(1<<p) != 0 }

// setOf builds a PropSet from its members.
func setOf(ps ...Prop) PropSet {
	var s PropSet
	for _, p := range ps {
		s |= 1 << p
	}
	return s
}

// allProps is every longhand: the reach of the `all` shorthand.
const allProps = PropSet(1)<<numProps - 1

// inheritedProps is the CSS (and GTK) inherited set: color, the font
// group, letter-spacing, text-transform, and -gtk-icon-size. Custom
// properties inherit too; they live outside the longhand table.
var inheritedProps = setOf(PropColor, PropFontFamily, PropFontSize, PropFontWeight,
	PropFontStyle, PropLetterSpacing, PropTextTransform, PropIconSize)

// Sides is a per-side length set, in logical pixels, in CSS's
// top-right-bottom-left order.
type Sides struct {
	Top, Right, Bottom, Left int
}

// Uniform reports whether every side is v.
func (s Sides) Uniform(v int) bool { return s == Sides{v, v, v, v} }

// Horizontal is left + right.
func (s Sides) Horizontal() int { return s.Left + s.Right }

// Vertical is top + bottom.
func (s Sides) Vertical() int { return s.Top + s.Bottom }

// Add sums two side sets.
func (s Sides) Add(o Sides) Sides {
	return Sides{s.Top + o.Top, s.Right + o.Right, s.Bottom + o.Bottom, s.Left + o.Left}
}

// Corners is a per-corner radius set, clockwise from top-left.
type Corners struct {
	TopLeft, TopRight, BottomRight, BottomLeft int
}

// BorderStyle is a border or outline line style. The subset draws solid
// lines; every other visible style strokes solid, and none/hidden
// zero the width, per the CSS computed-value rule.
type BorderStyle uint8

// Border styles.
const (
	BorderNone BorderStyle = iota
	BorderHidden
	BorderSolid
	BorderDashed
	BorderDotted
	BorderDouble
	BorderGroove
	BorderRidge
	BorderInset
	BorderOutset
)

// Visible reports whether the style draws a line.
func (b BorderStyle) Visible() bool { return b != BorderNone && b != BorderHidden }

// Shadow is one box-shadow layer.
type Shadow struct {
	X, Y, Blur, Spread int
	Color              render.Color
	Inset              bool
}

// MaxShadows bounds the box-shadow list; later layers past it are
// dropped with a warning. Four covers every stacked-elevation idiom.
const MaxShadows = 4

// Shadows is a fixed-capacity box-shadow list, comparable so a restyle
// can diff it.
type Shadows struct {
	N      int
	Layers [MaxShadows]Shadow
}

// List returns the active layers, front (first declared) first.
func (s *Shadows) List() []Shadow { return s.Layers[:s.N] }

// MaxStops bounds a gradient's color stops.
const MaxStops = 8

// GradientStop is one color stop; Pos is a fraction of the gradient
// line, resolved at parse time (auto positions spread evenly).
type GradientStop struct {
	Pos   float64
	Color render.Color
}

// Gradient is a linear-gradient background image. N zero means none.
// Angle is in degrees, CSS convention: 0 points up, 90 right.
type Gradient struct {
	Angle float64
	N     int
	Stops [MaxStops]GradientStop
}

// TextTransform is the text-transform keyword.
type TextTransform uint8

// Text transforms.
const (
	TransformNone TextTransform = iota
	TransformUppercase
	TransformLowercase
	TransformCapitalize
)

// Timing is a transition timing function, reduced to the named curves
// and cubic-bezier control points.
type Timing struct {
	X1, Y1, X2, Y2 float64
	// Steps is non-zero for steps(n); the bezier is then unused.
	Steps int
}

// Transition is the computed transition-* group: which properties
// animate (All covers every animatable one), for how long, after what
// delay, along which curve. Duration zero means no transition.
type Transition struct {
	All      bool
	Props    PropSet
	Duration float64 // seconds
	Delay    float64 // seconds
	Timing   Timing
}

// Covers reports whether a change of p animates.
func (t *Transition) Covers(p Prop) bool {
	return t.Duration > 0 && (t.All || t.Props.Has(p))
}

// Values is the cascade result for one node: the winning declaration
// per longhand, computed (var() substituted, calc() evaluated, units
// resolved to pixels), with Set recording which longhands the
// stylesheet provided, directly or through inheritance. Consumers layer
// programmatic widget values above it and the theme under it. Values is
// comparable, so a restyle diffs old against new with ==.
type Values struct {
	Set PropSet

	Color      render.Color
	Background render.Color
	Image      Gradient
	Opacity    float64
	Brightness float64

	Padding     Sides
	Margin      Sides
	BorderWidth Sides
	BorderStyle [4]BorderStyle // top, right, bottom, left
	BorderColor [4]render.Color
	Radius      Corners
	Shadow      Shadows

	OutlineWidth  int
	OutlineStyle  BorderStyle
	OutlineColor  render.Color
	OutlineOffset int

	MinWidth, MinHeight int
	// BorderSpacing is GTK's border-spacing: the gap a box leaves between
	// its children, horizontal and vertical.
	BorderSpacingH, BorderSpacingV int

	FontFamily    string
	FontSize      float64
	FontWeight    int
	Italic        bool
	LetterSpacing float64
	TextTransform TextTransform
	IconSize      int

	Transition Transition

	// Vars is the node's custom-property environment: its own
	// declarations over the inherited chain. Nodes that declare none
	// share their parent's pointer, so equality stays cheap.
	Vars *Vars
}

// Has reports whether the property was set for this node.
func (v *Values) Has(p Prop) bool { return v.Set.Has(p) }

// HasAny reports whether any of the properties was set.
func (v *Values) HasAny(ps ...Prop) bool {
	return slices.ContainsFunc(ps, v.Set.Has)
}

// EffBorder returns the used border widths: a side whose style draws no
// line has zero width, the CSS computed-value rule.
func (v *Values) EffBorder() Sides {
	b := v.BorderWidth
	if !v.BorderStyle[0].Visible() {
		b.Top = 0
	}
	if !v.BorderStyle[1].Visible() {
		b.Right = 0
	}
	if !v.BorderStyle[2].Visible() {
		b.Bottom = 0
	}
	if !v.BorderStyle[3].Visible() {
		b.Left = 0
	}
	return b
}

// EffOutline returns the used outline width: zero unless the style
// draws a line.
func (v *Values) EffOutline() int {
	if !v.OutlineStyle.Visible() {
		return 0
	}
	return v.OutlineWidth
}

// Var returns the computed value of the custom property name (with its
// leading dashes) as source text, and whether it is defined.
func (v *Values) Var(name string) (string, bool) {
	ts, ok := v.Vars.lookup(name)
	if !ok {
		return "", false
	}
	return tokensText(ts), true
}

// initialValues returns each longhand's initial value: what `initial`,
// `unset` on a non-inherited property, and `all: unset` compute to.
func initialValues() Values {
	return Values{
		Opacity:    1,
		Brightness: 1,
		FontWeight: 400,
	}
}

// Vars is one node's custom-property environment: its own computed
// declarations, falling back to the parent's.
type Vars struct {
	parent *Vars
	own    map[string][]token
}

// lookup finds name, own declarations first, then up the chain.
func (vs *Vars) lookup(name string) ([]token, bool) {
	for ; vs != nil; vs = vs.parent {
		if ts, ok := vs.own[name]; ok {
			return ts, ts != nil
		}
	}
	return nil, false
}

// equalOwn reports whether two environments hold the same own
// declarations over the same parent, so a recompute can keep the old
// pointer.
func (vs *Vars) equalOwn(o *Vars) bool {
	if vs == nil || o == nil || vs.parent != o.parent || len(vs.own) != len(o.own) {
		return false
	}
	for k, a := range vs.own {
		b, ok := o.own[k]
		if !ok || !tokensEqual(a, b) {
			return false
		}
	}
	return true
}

// tokensEqual compares two token lists by content, ignoring positions.
func tokensEqual(a, b []token) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].kind != b[i].kind || a[i].s != b[i].s || a[i].num != b[i].num {
			return false
		}
	}
	return true
}
