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
	PropLineHeight
	PropFontFeatures
	PropFontVariations
	PropTransform
	PropTransformOrigin
	PropIconSize
	PropTransitionProperty
	PropTransitionDuration
	PropTransitionTiming
	PropTransitionDelay
	PropAnimation
	PropAnimationPlayState
	PropAnimDuration
	PropAnimDelay
	PropAnimDirection
	PropAnimFill
	PropIconTransform
	PropIconSource
	PropIconPalette
	PropCaretColor
	PropTextDecoration
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

// allProps is every longhand: the reach of the `all` shorthand. The
// shift form holds with the 64-bit set full.
const allProps = ^PropSet(0) >> (64 - numProps)

// inheritedProps is the CSS (and GTK) inherited set: color, the font
// group, letter-spacing, text-transform, line-height, and
// -gtk-icon-size. Custom properties inherit too; they live outside the
// longhand table.
var inheritedProps = setOf(PropColor, PropFontFamily, PropFontSize, PropFontWeight,
	PropFontStyle, PropLetterSpacing, PropTextTransform, PropLineHeight,
	PropFontFeatures, PropFontVariations, PropIconSize)

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

// At reads side i by the transition engine's index: 0 top, 1 right,
// 2 bottom, 3 left.
func (s Sides) At(i int) int {
	return [4]int{s.Top, s.Right, s.Bottom, s.Left}[i]
}

// SetAt writes side i.
func (s *Sides) SetAt(i int, v int) {
	switch i {
	case 0:
		s.Top = v
	case 1:
		s.Right = v
	case 2:
		s.Bottom = v
	case 3:
		s.Left = v
	}
}

// Corners is a per-corner radius set, clockwise from top-left.
type Corners struct {
	TopLeft, TopRight, BottomRight, BottomLeft int
}

// At reads corner i, the same clockwise order as Sides.
func (c Corners) At(i int) int {
	return [4]int{c.TopLeft, c.TopRight, c.BottomRight, c.BottomLeft}[i]
}

// SetAt writes corner i.
func (c *Corners) SetAt(i, v int) {
	switch i {
	case 0:
		c.TopLeft = v
	case 1:
		c.TopRight = v
	case 2:
		c.BottomRight = v
	case 3:
		c.BottomLeft = v
	}
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

// Gradient is a gradient background image - linear, radial, or conic,
// optionally repeating - in render's geometry terms. N zero means none.
// Angle is in degrees, CSS convention: 0 points up, 90 right (a conic
// gradient's start). CenterX/Y place a radial or conic center as box
// fractions; RadiusX/Y are explicit radial radii in px.
type Gradient struct {
	Kind             render.GradientKind
	Repeat           bool
	Angle            float64
	CenterX, CenterY float64
	Circle           bool
	Size             render.RadialSize
	RadiusX, RadiusY float64
	N                int
	Stops            [MaxStops]GradientStop
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
	// Own are the properties the node's own rules declared, before
	// inheritance filled in the rest of Set: a part that styles only
	// what a stylesheet names for it (an entry's placeholder color)
	// reads these.
	Own PropSet

	Color      render.Color
	Background render.Color
	Image      Gradient
	// BgImageURL is background-image's url(...): a file path whose
	// image paints as the box's background, cover-fit inside the
	// rounded border (CSS background-size: cover). Empty is none; a
	// gradient and a URL are mutually exclusive, the last declaration
	// wins.
	BgImageURL string
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
	// LineHeight is the computed line-height in pixels; 0 is `normal`,
	// the font's own line box.
	LineHeight float64
	// Features is the font-feature-settings tag list in canonical form
	// (`on=1;ss01=0;tnum=1`, sorted); empty is `normal`. The text
	// shaper honors the tags it knows (tnum today).
	Features string
	// Variations is the font-variation-settings axis list in canonical
	// form (`wdth=80.5;wght=650`, sorted); empty is `normal`.
	Variations string
	// Transform carries the transform property: the composed affine
	// the widget paints its subtree through, with the primitive list
	// the tweens interpolate. The identity means unset; the parser
	// always writes the full Xform, so a declared `none` reads as it.
	Transform Xform
	// TransformOrigin is the transform's pivot: fraction-of-box plus
	// pixel offset per axis; the default is the box's center.
	OriginFrac, OriginPx [2]float64
	IconSize             int

	Transition Transition
	// Animation is the computed animation-* group; not inherited. A
	// comma list runs every entry; one entry is the common case.
	Animation []Animation
	// The animation longhands' slots: the shorthand computes the group
	// and each longhand's slot zips onto it at the end of Compute, so
	// `animation: a 1s; animation-delay: -2s` keeps the name and the
	// delay whoever is declared first.
	AnimDuration, AnimDelay []float64
	AnimDirection, AnimFill []uint8
	// IconXform is -gtk-icon-transform's composed transform; icons
	// draw through it.
	IconXform Xform
	// IconSource is -gtk-icon-source's -gtk-icontheme() name: a themed
	// symbolic icon a widget draws instead of its painted mark (the
	// checkbutton's tick).
	IconSource string
	// PaletteTint is -gtk-icon-palette's recolor: the color the source
	// icon takes (the stylesheet writes `success <color>`).
	PaletteTint render.Color
	// CaretColor is caret-color: an entry's caret takes it over the
	// text color.
	CaretColor render.Color
	// Underline is text-decoration's underline.
	Underline bool

	// Vars is the node's custom-property environment: its own
	// declarations over the inherited chain. Nodes that declare none
	// share their parent's pointer, so equality stays cheap.
	Vars *Vars
}

// Has reports whether the property was set for this node.
func (v *Values) Has(p Prop) bool { return v.Set.Has(p) }

// Declares reports whether the node's own rules set p, not its
// inheritance.
func (v *Values) Declares(p Prop) bool { return v.Own.Has(p) }

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
// Zero would be a visible value for opacity and brightness, and a zero
// scale paints nothing, so those carry their identities; the transform
// origin defaults to the box's center.
func initialValues() Values {
	return Values{
		Opacity:    1,
		Brightness: 1,
		FontWeight: 400,
		Transform:  XformIdentity,
		IconXform:  XformIdentity,
		OriginFrac: [2]float64{0.5, 0.5},
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
