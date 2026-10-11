package style

import (
	"slices"

	"github.com/stubbedev/gelm/anim"
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
	PropCursor
	PropPointerEvents
	PropUserSelect
	PropCaretShape
	PropAccentColor
	PropAppearance
	PropTouchAction
	PropTextAlign
	PropTextIndent
	PropWhiteSpace
	PropWordBreak
	PropLineBreak
	PropHyphens
	PropWordSpacing
	PropVerticalAlign
	PropLineClamp
	PropTextOverflow
	PropFontVariantCaps
	PropFontVariantNumeric
	PropFontVariantLigatures
	PropFontVariantEastAsian
	PropFontVariantPosition
	PropFontVariantAlternates
	PropFontSynthesis
	PropFontOpticalSizing
	PropDirection
	PropUnicodeBidi
	PropWritingMode
	PropBackdropFilter
	PropMixBlendMode
	PropIsolation
	PropClipPath
	PropMaskImage
	PropMaskPosition
	PropMaskSize
	PropMaskRepeat
	PropMaskMode
	PropPerspective
	PropTransformStyle
	PropBackfaceVisibility
	PropScrollBehavior
	PropScrollMarginTop
	PropScrollMarginRight
	PropScrollMarginBottom
	PropScrollMarginLeft
	PropScrollPaddingTop
	PropScrollPaddingRight
	PropScrollPaddingBottom
	PropScrollPaddingLeft
	PropScrollSnapType
	PropScrollSnapAlign
	PropScrollSnapStop
	PropOverscrollX
	PropOverscrollY
	PropScrollbarColor
	PropScrollbarWidth
	PropScrollbarGutter
	PropTransitionBehavior
	PropAnimComposition
	numProps
)

// PropSet is a set of longhands, two words wide (the longhand table
// passed sixty-four).
type PropSet struct{ lo, hi uint64 }

// Has reports whether p is in the set.
func (s PropSet) Has(p Prop) bool {
	if p < propWordBits {
		return s.lo&(1<<p) != 0
	}
	return s.hi&(1<<(p-propWordBits)) != 0
}

// Empty reports whether the set has no member.
func (s PropSet) Empty() bool { return s == PropSet{} }

// With returns the set plus p.
func (s PropSet) With(p Prop) PropSet {
	if p < propWordBits {
		s.lo |= 1 << p
	} else {
		s.hi |= 1 << (p - propWordBits)
	}
	return s
}

// Without returns the set minus p.
func (s PropSet) Without(p Prop) PropSet {
	if p < propWordBits {
		s.lo &^= 1 << p
	} else {
		s.hi &^= 1 << (p - propWordBits)
	}
	return s
}

// Or returns the union of the sets.
func (s PropSet) Or(o PropSet) PropSet {
	s.lo |= o.lo
	s.hi |= o.hi
	return s
}

// And returns the intersection of the sets.
func (s PropSet) And(o PropSet) PropSet {
	s.lo &= o.lo
	s.hi &= o.hi
	return s
}

// AndNot is s minus o.
func (s PropSet) AndNot(o PropSet) PropSet {
	s.lo &^= o.lo
	s.hi &^= o.hi
	return s
}

// propWordBits is the props per set word.
const propWordBits = 64

// setOf builds a PropSet from its members.
func setOf(ps ...Prop) PropSet {
	var s PropSet
	for _, p := range ps {
		s = s.With(p)
	}
	return s
}

// SetOf builds a PropSet from its members; the exported form for the
// consumers outside the engine.
func SetOf(ps ...Prop) PropSet { return setOf(ps...) }

// allProps is every longhand: the reach of the `all` shorthand.
var allProps = func() (s PropSet) {
	for p := range numProps {
		s = s.With(p)
	}
	return s
}()

// inheritedProps is the CSS (and GTK) inherited set: color, the font
// group, letter-spacing, text-transform, line-height, and
// -gtk-icon-size. Custom properties inherit too; they live outside the
// longhand table.
var inheritedProps = setOf(PropColor, PropFontFamily, PropFontSize, PropFontWeight,
	PropFontStyle, PropLetterSpacing, PropTextTransform, PropLineHeight,
	PropFontFeatures, PropFontVariations, PropIconSize, PropCursor,
	PropTextAlign, PropTextIndent,
	PropWhiteSpace, PropWordBreak, PropLineBreak, PropHyphens, PropWordSpacing,
	PropFontVariantCaps, PropFontVariantNumeric, PropFontVariantLigatures,
	PropFontVariantEastAsian, PropFontVariantPosition, PropFontVariantAlternates,
	PropFontOpticalSizing, PropDirection, PropWritingMode,
	PropScrollbarColor, PropScrollbarWidth)

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

// TextAlign is the text-align keyword.
type TextAlign uint8

// Text alignments. Start and End resolve against the resolved base
// direction; Left and Right are absolute; Justify distributes a
// wrapped row's leftover width across its spaces.
const (
	AlignStart TextAlign = iota
	AlignEnd
	AlignLeft
	AlignRight
	AlignCenter
	AlignJustify
)

// WhiteSpace is the white-space keyword.
type WhiteSpace uint8

// White-space keywords.
const (
	WhiteSpaceNormal WhiteSpace = iota
	WhiteSpacePre
	WhiteSpaceNowrap
	WhiteSpacePreWrap
	WhiteSpacePreLine
	WhiteSpaceBreakSpaces
)

// WordBreak is the word-break keyword.
type WordBreak uint8

// Word-break keywords.
const (
	WordBreakNormal WordBreak = iota
	WordBreakBreakAll
	WordBreakKeepAll
)

// LineBreakK is the line-break keyword (K for the line-break clash
// with the render-side breaker).
type LineBreakK uint8

// Line-break keywords.
const (
	LineBreakAuto LineBreakK = iota
	LineBreakLoose
	LineBreakNormal
	LineBreakStrict
	LineBreakAnywhere
)

// HyphensK is the hyphens keyword.
type HyphensK uint8

// Hyphens keywords.
const (
	HyphensNone HyphensK = iota
	HyphensManual
	HyphensAuto
)

// TextOverflowK is the text-overflow keyword.
type TextOverflowK uint8

// Text-overflow keywords.
const (
	TextOverflowClip TextOverflowK = iota
	TextOverflowEllipsis
)

// VerticalAlignK is the vertical-align alignment kind.
type VerticalAlignK uint8

// Vertical-align kinds. Length is VerticalAlignLength with the shift
// in Values.VerticalAlignPx.
const (
	VerticalAlignBaseline VerticalAlignK = iota
	VerticalAlignSub
	VerticalAlignSuper
	VerticalAlignTextTop
	VerticalAlignTextBottom
	VerticalAlignTop
	VerticalAlignMiddle
	VerticalAlignBottom
	VerticalAlignLength
)

// FontVariantCaps is the font-variant-caps keyword.
type FontVariantCaps uint8

// font-variant-caps keywords.
const (
	CapsNormal FontVariantCaps = iota
	CapsSmallCaps
	CapsAllSmallCaps
	CapsPetiteCaps
	CapsAllPetiteCaps
	CapsUnicase
	CapsTitlingCaps
)

// FontVariantNumeric is font-variant-numeric's flag set.
type FontVariantNumeric uint16

// font-variant-numeric flags.
const (
	NumLining FontVariantNumeric = 1 << iota
	NumOldStyle
	NumProportional
	NumTabular
	NumDiagonalFractions
	NumStackedFractions
	NumOrdinal
	NumSlashedZero
)

// FontVariantLigatures is font-variant-ligatures' flag set; a flag's
// absence means its no-* counterpart.
type FontVariantLigatures uint16

// font-variant-ligatures flags.
const (
	LigCommon FontVariantLigatures = 1 << iota
	LigDiscretionary
	LigHistorical
	LigContextual
)

// FontVariantEastAsian is font-variant-east-asian's flag set.
type FontVariantEastAsian uint16

// font-variant-east-asian flags.
const (
	EAJis78 FontVariantEastAsian = 1 << iota
	EAJis83
	EAJis90
	EAJis2004
	EASimplified
	EATraditional
	EAFullWidth
	EAProportional
	EARuby
)

// FontVariantPosition is the font-variant-position keyword.
type FontVariantPosition uint8

// font-variant-position keywords.
const (
	PositionNormal FontVariantPosition = iota
	PositionSub
	PositionSuper
)

// FontVariantAlternates is the font-variant-alternates keyword.
type FontVariantAlternates uint8

// font-variant-alternates keywords.
const (
	AlternatesNormal FontVariantAlternates = iota
	AlternatesHistorical
)

// FontSynthesis is font-synthesis' flag set: which face qualities the
// engine may synthesize when the chosen face lacks them.
type FontSynthesis uint8

// font-synthesis flags.
const (
	SynthWeight FontSynthesis = 1 << iota
	SynthStyle
	SynthSmallCaps
)

// UnicodeBidi is the unicode-bidi keyword.
type UnicodeBidi uint8

// unicode-bidi keywords.
const (
	BidiNormal UnicodeBidi = iota
	BidiEmbed
	BidiIsolate
	BidiOverride
	BidiIsolateOverride
	BidiPlaintext
)

// WritingMode is the writing-mode keyword.
type WritingMode uint8

// writing-mode keywords.
const (
	WritingHorizontalTB WritingMode = iota
	WritingVerticalRL
	WritingVerticalLR
)

// Timing is a transition or animation timing function: the named
// curves and cubic-bezier control points, steps() with its jump
// position, or the linear() piecewise list. Exactly one form is live:
// LinN > 0 is linear(), else Steps > 0 is steps(), else the bezier.
type Timing struct {
	X1, Y1, X2, Y2 float64
	// Steps is non-zero for steps(n); the bezier is then unused.
	Steps int
	// StepPos is steps' jump position (JumpEnd's value is the default,
	// so the zero Timing keeps the legacy meaning).
	StepPos uint8
	// LinN is the number of live linear() points in Lin.
	LinN int
	// Lin is the linear() point list, capped at MaxLinearPts.
	Lin [MaxLinearPts]anim.LinearPt
}

// MaxLinearPts bounds a linear() point list; points past it are
// dropped with a warning, like the gradient stop budget.
const MaxLinearPts = 8

// Transition is the computed transition-* group: which properties
// animate (All covers every animatable one), for how long, after what
// delay, along which curve, and whether discrete properties may flip
// through the tween instead of snapping. Duration zero means no
// transition.
type Transition struct {
	All      bool
	Props    PropSet
	Duration float64 // seconds
	Delay    float64 // seconds
	Timing   Timing
	// Discrete is transition-behavior: allow-discrete.
	Discrete bool
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

	// Filter is the computed filter list; Brightness stays the
	// brightness-only fast path's product, kept in sync by the
	// parser so the tween engines keep their channel.
	Filter render.Filters
	// Backdrop is the computed backdrop-filter list, applied to what
	// paints behind the widget rather than the widget itself.
	Backdrop render.Filters
	// Blend is the computed mix-blend-mode; BlendNormal composites as
	// plain source-over.
	Blend render.BlendMode
	// Isolate is isolation: isolate — the subtree becomes its own
	// compositing group.
	Isolate bool
	// Clip is the computed clip-path; a zero Kind clips nothing.
	Clip ClipShape
	// Mask is the computed mask-image group (position, size, repeat,
	// mode).
	Mask Mask
	// Perspective is the perspective distance in px; zero is none.
	Perspective float64
	// Preserve3D is transform-style: preserve-3d; flat otherwise. The
	// painter flattens either way today — see docs/css.md.
	Preserve3D bool
	// BackfaceHidden is backface-visibility: hidden.
	BackfaceHidden bool

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
	// Starting is the node's computed @starting-style block, nil when
	// no matched rule carries one: the values a first style's
	// transitions start from (the entry transition). Only the
	// properties its rules declare are meaningful.
	Starting *Values
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
	// Decoration is text-decoration: its lines, style, and color.
	Decoration render.Decoration
	// Cursor is cursor: the hover shape a stylesheet asks for, a
	// keyword or a url() file cursor with its hotspot. The zero value
	// is auto; it inherits.
	Cursor CursorSpec
	// PointerEvents is pointer-events: none takes the widget out of
	// hit testing.
	PointerEvents PointerEvents
	// UserSelect is user-select on the text surfaces that select.
	UserSelect UserSelect
	// CaretShape is caret-shape: the caret's painted form.
	CaretShape CaretShape
	// AccentColor is accent-color over the theme accent; AccentAuto
	// marks the auto keyword falling back to the theme.
	AccentColor render.Color
	AccentAuto  bool
	// Appearance is appearance: parsed only, none's chrome stripping
	// is deferred (docs/css.md).
	Appearance Appearance
	// TouchAction is touch-action: the pan axes a touch scroll may
	// move.
	TouchAction TouchAction

	// The scroll-container group: scroll-behavior gates animated
	// programmatic scrolls, scroll-padding insets the viewport for
	// reveal and snap math, scroll-snap-type turns snapping on,
	// overscroll-behavior gates chaining to the parent scroll, and
	// scrollbar-gutter reserves the bar's space in measure. The
	// scrollbar-color/width pair is inherited and reaches the bars.
	ScrollSmooth bool
	ScrollPad    Sides
	Snap         SnapType
	Overscroll   [2]Overscroll
	BarThumb     render.Color
	BarTrack     render.Color
	// BarColorAuto marks scrollbar-color: auto (the default look).
	BarColorAuto bool
	BarWidth     uint8
	BarGutter    uint8

	// The scrolled-child group: scroll-margin insets the revealed rect,
	// scroll-snap-align names the child a snap position, and
	// scroll-snap-stop makes one the gesture cannot skip past.
	ScrollMargin   Sides
	SnapAlign      SnapAlign
	SnapStopAlways bool

	// The animation-composition slot, zipped onto the group like the
	// other animation longhands.
	AnimComposition []uint8

	// The CSS Text 3 properties. TextAlign, TextIndent, WhiteSpace,
	// WordBreak, LineBreak, Hyphens, WordSpacing and the font-variant
	// family inherit; VerticalAlign, LineClamp, TextOverflow,
	// UnicodeBidi and FontSynthesis do not.
	TextAlign       TextAlign
	TextIndent      float64
	WhiteSpace      WhiteSpace
	WordBreak       WordBreak
	LineBreak       LineBreakK
	Hyphens         HyphensK
	WordSpacing     float64
	VerticalAlign   VerticalAlignK
	VerticalAlignPx float64
	// VerticalAlignPct marks VerticalAlignPx a fraction of the line
	// box instead of pixels.
	VerticalAlignPct bool
	LineClamp        int
	Overflow         TextOverflowK
	VariantCaps      FontVariantCaps
	VariantNumeric   FontVariantNumeric
	VariantLig       FontVariantLigatures
	VariantEA        FontVariantEastAsian
	VariantPos       FontVariantPosition
	VariantAlt       FontVariantAlternates
	Synthesis        FontSynthesis
	OpticalSizing    bool
	// Direction is the CSS direction: 1 ltr, 2 rtl (0 cannot be set -
	// direction takes no auto keyword).
	Direction   uint8
	UnicodeBidi UnicodeBidi
	WritingMode WritingMode

	// Content is a pseudo-element's resolved content (::before,
	// ::after): the strings, attr() values, and counter readings joined;
	// HasContent reports a content declaration won at all.
	Content    string
	HasContent bool
	// Counters is the node's counter environment (counter-reset,
	// counter-increment): nil when no sheet counts anything.
	Counters *CounterScope

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
