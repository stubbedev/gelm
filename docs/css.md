# gelm CSS

A GTK-flavored CSS layer above the typed `Theme`. The palette stays the
source of truth and works with no stylesheet loaded. Stylesheets
override individual properties where rules match. Real GTK application
stylesheets load as they are: wayle's compiled SCSS, about 1800 rules,
parses in about 6 ms with no warnings.

## API

```go
s, err := widget.AddStylesheetFile("theme.css", widget.StylePriorityTheme) // its own sheet, hot-reloaded
widget.LoadStylesheetFile("app.css")             // the application slot, hot-reloaded
widget.LoadStylesheet(`.title { color: #c00 }`) // or from a string; "" removes it
s := widget.AddStylesheet(css, widget.StylePrioritySettings)
s.Load(newCSS)                                  // replace its rules in place
s.Remove()

b := widget.NewButton(child, 10, 8)
b.AddClass("destructive")
b.SetID("save-button")
b.SetState(widget.StateSelected, true)          // :selected, :checked, :backdrop, ...
b.SetInlineStyle("padding: 4px")                // a widget-scoped provider
b.SetAttr("data-kind", "danger")                // [data-kind="danger"]
box.AttachStylesheet(widget.NewStylesheet(css, widget.StylePriorityApplication))
```

- **Priorities** follow GTK's provider priorities: `StylePriorityFallback`,
  `Theme` (regenerated on `SetTheme`), `Settings`, `Application` (the
  `LoadStylesheet` slot), and `User` (inline styles).
- **Hot reload.** `AddStylesheetFile` and `LoadStylesheetFile` stat each
  loaded file once a second on the application's timer and reload it on
  change; one poller covers every file, starts with the first file load,
  and each sheet's `Remove` (or `Load`) stops its file. A file that
  vanishes or stops parsing keeps its last good rules and is reported
  through `widget.SetParseWarn`; a load that cannot read or parse it
  fails with an error carrying the path.
- **Fonts.** `font-family` and `font-weight` resolve through
  `widget.SetFaceResolver` (for example over `app.FontWeighted`),
  with `@font-face` families consulted first (above). Without a
  resolver they keep the constructor face. A variable face still
  takes `font-weight` on its wght axis.
- **Warnings.** Parse warnings go to the library logger, or to
  `widget.SetParseWarn`.
- **Units.** `widget.SetRootFontSize` sets what 1rem is (16 by default).

## Syntax

- A CSS Syntax 3 tokenizer.
- `@keyframes` (and `@-gtk-keyframes`) compile into the sheet.
- `@media` evaluates against the tree's media state (below) — size
  features (`width`, `height`, `aspect-ratio`, `orientation`,
  `resolution`, with `min-`/`max-` and Level 4 range syntax like
  `(400px <= width <= 800px)`), `prefers-color-scheme`,
  `prefers-contrast`, `prefers-reduced-motion`, the `and`/`not`
  combinators, a comma list (OR), media types (`screen`/`all` true),
  and nesting (the inner conditions intersect). A `@media` block
  inside a rule scopes that rule's remaining declarations.
- `@supports` evaluates at parse time against the engine's actual
  property and selector support: `(prop: value)`, `selector(...)`,
  `not`, `and`, `or`, and parens. A false block's rules never
  compile; unknown properties are false.
- `@import "file.css";` (or `url(...)`, with the optional `layer`
  and `layer(name)` qualifiers — dotted names nest — the optional
  `supports(...)` qualifier evaluated at parse time like `@supports`,
  and an optional trailing media query) loads and inlines its target
  relative to the importing stylesheet's path — file loads resolve
  beside the file, string stylesheets relative to the working
  directory. Cycles are detected per import chain and reported
  precisely (`@import cycle: a.css -> b.css -> a.css`); a missing
  file, a false `supports()`, or a malformed import warns and drops
  that import only.
- `@layer` orders cascade layers (below). Dotted names nest:
  `@layer a.b` is a layer below `a` (the parent outranks its nested
  layers), `@layer a { @layer b { ... } }` means `a.b`, and a name
  statement registers the dotted prefixes it names.
- `@container` evaluates per compute against the nearest query
  container — a widget with `container-type` set in the node's
  ancestry (below).
- `@font-face` declares app fonts: `font-family`, `src: url(...)`
  with `format()` hints (truetype/opentype load; an unsupported
  format warns and falls through to the next src), the `font-weight`
  range, and `font-style`. The face loads on the next relayer and
  the font-family resolver serves it before the app's own; a src
  that cannot be resolved or read reports through the stylesheet
  error path (a file load fails with its path). `font-stretch` and
  the other descriptors parse and are unused — the loader has no
  matching knob.
- `@property` registers typed custom properties: `syntax` (the
  known type names and `*`, pipe lists with the trailing `+`),
  `inherits`, and an `initial-value` validated against the syntax.
  A registered name's initial value answers var() wherever nothing
  declares it; `inherits: false` resolves the name to its initial
  value on nodes that do not declare it, however their ancestors
  set it. Animating a registered property is deferred: the
  transition engine keys on the longhand table, which custom
  properties sit outside.
- `@scope (root) to (upper)? { ... }` scopes the block's rules:
  each subject must descend from an element matching the root
  without crossing the upper boundary, and `:scope` in a selector
  names that root. Scoping proximity ranks above specificity — an
  inner scope beats an outer one at equal specificity — and below
  provider priority.
- Native nesting: a nested block desugars `&` to the parent selector
  (`&:hover`, `& > .child`); a nested selector without `&` is the
  parent's descendant, comma lists cross with the parent list, and
  nesting nests.
- `!important` wins the cascade over every normal declaration
  (below).
- A malformed declaration skips that declaration, and a malformed
  selector skips its rule. Everything around it still applies.

## Selectors

- **Simple selectors.** Element, `.class`, `#id` and `*`.
- **Attribute selectors.** `[attr]`, `[attr=v]`, `~=`, `|=`, `^=`,
  `$=`, `*=`, each with the trailing `i`/`s` case flag. The built-in
  attributes are `id`, `class` (the class list, so `[class~=x]` and
  `[class|=x]` work) and `role` (the a11y role's name, `button`,
  `entry`, `label`, ...); any other name reads an app-set data
  attribute (`SetAttr`).
- **Combinators.** ` `, `>`, `+` and `~`.
- **State pseudo-classes.**
  - `:hover` matches the hovered widget and its ancestors (GTK's rule).
  - `:active`, `:focus`, `:focus-visible` and `:focus-within`.
  - `:disabled`, `:checked`, `:selected`, `:indeterminate` and
    `:backdrop`. These are app-driven through `SetState`.
  - `:enabled` (the negation of `:disabled`), `:read-only` and
    `:read-write` (an entry's `SetReadOnly`), and
    `:placeholder-shown` (an empty entry with a placeholder).
- **Functional pseudo-classes.** `:is()` and `:where()` with full
  selector lists — `:is()` counts its most specific argument's
  specificity, `:where()` counts nothing — and `:not()` with complex
  selector lists (Level 4). `:has()` takes relative selectors
  (`:has(.a)`, `:has(> .a)`, `:has(+ .a)`, `:has(~ .a)`, with complex
  arguments too) and counts its most specific argument; one compute
  spends at most 4096 node visits on `:has()` walks, after which a
  `:has()` conservatively stops matching — the budget keeps relational
  matching linear instead of quadratic in tree size, and a sheet that
  loads `:has()` selectors also makes descendant class/state changes
  restyle the ancestors that read them.
- **Structural pseudo-classes.** `:first-child`, `:last-child`,
  `:only-child`, `:nth-child()`, `:nth-last-child()` (over visible
  siblings), `:nth-of-type()`, `:nth-last-of-type()`,
  `:first-of-type`, `:last-of-type`, `:only-of-type` (over visible
  siblings of the same element name), `:root`, `:empty`, and `:not()`
  with compound lists and Level 4 specificity. `:nth-child(an+b of
  S)` counts only the siblings matching the selector list S.
- **Pseudo-elements.** `::before` and `::after` cascade their own
  style (inherited from the element's) with `content:` of strings,
  `attr(name)` (the built-ins and data attributes), `counter(name)`
  and `counters(name, sep)`; `counter-reset` and
  `counter-increment` count in document order (through previous
  siblings); `content: url(...)` paints an image as the generated
  content, beside the text, and the quote keywords
  `open-quote`/`close-quote`/`no-open-quote`/`no-close-quote` emit
  the typographic quote pairs over the inherited quote nesting
  counter. The pseudo-element's box layers and content paint over
  the widget's background — content at the content start (`::before`)
  and end (`::after`), in the pseudo-element's own color and
  font-size — wherever the widget paints box layers (`PaintBoxLayers`)
  or shapes text (labels and entries today). `widget.PseudoStyle` and
  `widget.PseudoContent` read the computed values. `::placeholder`,
  `::selection` and `::marker` desugar onto the existing part
  machinery: `entry::placeholder` styles the entry's `placeholder`
  part (as a descendant part selector) and `::selection` its
  `selection` part; `::marker` matches a `marker` part.
  `::first-letter` cascades its own pseudo style and paints on
  `Label`'s single-line draw: the first grapheme cluster in its own
  color, font-family, weight, and size, the remainder in the
  label's style. `::first-line` parses but never matches: line
  fragmentation happens inside the text shaper at paint time, and a
  widget's text is one shaped run with no per-line box the cascade
  could address — styling the first line would need the wrapper to
  expose wrap rows to the cascade.
- **Element names.** These are GTK's CSS node names where gelm has the
  GTK widget (`button`, `label`, `entry`, `scale`, `scrolledwindow`,
  `textview`, `listview`, `row`, `gridview`, `child`, `flowboxchild`, ...), and otherwise
  the Go type lowercased. Sub-nodes are named too: `entry > text`,
  `checkbutton > check`, `notebook > header > tabs > tab`,
  `progressbar > trough > progress`, `paned > separator`, and menus as
  `popover.menu` with `modelbutton` rows. The app surfaces are
  `dialog`, `popover`, `tooltip` and `toast`. `TestCSSElementNames`
  pins the list.

## Values

- **Custom properties.** `--x` is inherited and computed where it is
  declared, and cycles are invalid. `var()` takes fallbacks, and an
  unresolvable `var()` acts as `unset`. `var()` works everywhere a
  value can appear, including inside shorthand expansion
  (`border: var(--w) solid var(--c)`).
- **Math.** `calc()`, `min()`, `max()` and `clamp()`, plus the CSS
  Values 4 set: `round()` (with the `nearest`/`up`/`down`/`to-zero`
  strategies), `mod()`, `rem()`, `abs()`, `sign()`, `sin()`, `cos()`,
  `tan()`, `asin()`, `acos()`, `atan()`, `atan2()` (number arguments
  are radians, the inverse functions return angles), `pow()`,
  `sqrt()`, `hypot()`, `log()` (with an optional base) and `exp()`,
  all nestable inside `calc()`.
- **Units.** px, rem, em, ch, ex, cap, ic, lh and rlh (the
  font-relative family: ch/ex/cap/ic resolve against the default
  face's metrics scaled by the node's font size — `render.Font.Metrics`
  — with the CSS fallback ratios when the face reports none; lh is
  the node's computed line-height, rlh the root's), vw, vh, vmin and
  vmax (the media viewport the `@media` size features read), cqw,
  cqh, cqi, cqb, cqmin and cqmax (the container-query units, against
  the nearest query container; without one they resolve to zero,
  the same unset-viewport reading the vw family takes at 0×0), pt,
  pc, in, cm, mm, %, deg, grad, rad, turn, s and ms.
- **env()** answers the safe-area and keyboard inset family
  (`env(safe-area-inset-top)`, `env(keyboard-inset-height)`, ...) as
  zero lengths, the desktop value; unknown names are invalid.
- **attr()** reads the node's attributes (the optional `StyleAttr`
  hook a widget implements) as typed values — `attr(name px)`, a
  unitless number, or `attr(name color)` — wherever a length or color
  is taken. A missing attribute behaves as `unset`. This is the
  engine's one attr() implementation.
- **Colors.**
  - Hex (3/4/6/8 digits), named colors, `transparent` and
    `currentColor` (valid wherever a color is, including inside every
    function below).
  - `rgb[a]()`, `hsl[a]()`, `hwb()`, `lab()`, `lch()`, `oklab()`,
    `oklch()` and `color()` — the predefined spaces `srgb`,
    `srgb-linear`, `display-p3`, `a98-rgb`, `prophoto-rgb`, `rec2020`,
    `xyz`, `xyz-d65` and `xyz-d50`, mapped to sRGB (out-of-gamut
    colors clamp).
  - Relative color syntax: `rgb(from <color> r g b / a)`, and the
    same `from` form for hsl, hwb, lab, lch, oklab and oklch, each
    channel an expression over the origin's channel keywords (`l`,
    `c`, `h`, `r`, `alpha`, ...) in `calc()`.
  - `color-mix(in <space>, A, B)` in every interpolation space
    (`srgb`, `srgb-linear`, `lab`, `lch`, `oklab`, `oklch`, `hsl`,
    `hwb`) with the hue methods `shorter` (default), `longer`,
    `increasing` and `decreasing`.
  - `light-dark(a, b)` follows the active theme's color scheme.
  - System colors — `Canvas`, `CanvasText`, `LinkText`, `VisitedText`,
    `GrayText`, `Highlight`, `HighlightText`, `Mark`, `MarkText`,
    `ButtonFace`, `ButtonText`, `ButtonBorder`, `Field`, `FieldText` —
    map onto the active theme (background, text, accent, borders) and
    re-derive on `SetTheme`.
  - GTK's `alpha()`, `shade()` and `mix()`.
  - `@define-color name value;` with `@name` references, which compile
    to `var(--name)`, so a subtree's `--name` overrides them. The
    theme layer defines all of libadwaita's named colors from the
    palette.
- **Keywords.** `inherit`, `initial`, `unset`, `all`, `revert` and
  `revert-layer`.

## Properties

Each longhand is listed with its shorthand.

- **Text.**
  - `color`
  - `font`, `font-family`, `font-size`, `font-weight`, `font-style`
  - `font-feature-settings`, `font-variation-settings`
  - `letter-spacing`, `word-spacing`, `line-height`, `text-transform`
  - `text-align` (start, end, left, right, center, justify — justify
    spreads a wrapped row's leftover width across its spaces;
    everything but the final row), `text-indent` (the first row's
    start edge), `white-space` (normal, nowrap, pre, pre-wrap,
    pre-line, break-spaces), `word-break` (normal, break-all,
    keep-all — break-all and keep-all step by grapheme cluster and
    the UAX #14 class rules), `line-break` (auto, loose, normal,
    strict, anywhere — anywhere breaks at every cluster; the CJK
    strictness profiles below), `hyphens` (none hides U+00AD, manual
    breaks at it and renders the hyphen where the break landed, auto
    reads as manual), `text-overflow` (clip, ellipsis), `line-clamp`
    (and `-webkit-line-clamp`), `vertical-align` (baseline, sub,
    super, a length, or a percentage of the line box, on every text
    surface that reports a baseline — labels, buttons, rows)
  - `font-variant` and its longhands — caps (`small-caps`,
    `all-small-caps`, `petite-caps`, `all-petite-caps`, `unicase`,
    `titling-caps`), numeric (`lining-nums`, `oldstyle-nums`,
    `proportional-nums`, `tabular-nums`, `diagonal-fractions`,
    `stacked-fractions`, `ordinal`, `slashed-zero`), ligatures
    (`common-ligatures`, `discretionary-ligatures`,
    `historical-ligatures`, `contextual` — enabling only, the `no-*`
    forms are the fonts' default-off state), east-asian (`jis78` …
    `jis2004`, `simplified`, `traditional`, `full-width`,
    `proportional-width`, `ruby`), position (`sub`, `super`) and
    alternates (`historical-forms`) — all mapping onto the OpenType
    feature tags, with `font-feature-settings` winning per tag
  - `font-synthesis` (none/weight/style/small-caps — the engine
    never synthesizes a face quality, so this parses as the guard it
    is; there is nothing to gate), `font-optical-sizing` (auto feeds
    the pixel size to the `opsz` axis in points, none keeps it)
  - `direction` (ltr/rtl — it forces a label's or entry's base
    direction), `unicode-bidi` (the overrides force the base the same
    way), `writing-mode` (parsed; see below)
  - `text-decoration` (underline, overline and line-through in solid,
    double, dotted, dashed or wavy, with a color), `caret-color`,
    `caret-shape` (auto, bar, block, underscore — the entry and text
    area caret's painted form)
  - `user-select` (none, text, all) on the surfaces that select:
    Entry, TextArea, RichLabel. `none` keeps a press a caret move and
    a drag from extending a selection; `all` selects everything on a
    press. Plain Label carries no selection machinery, so the value
    changes nothing there.
- **Text-surface scope and deferrals.** The CSS Text properties
  apply to the styled text surfaces: `Label` (and the buttons, rows,
  and menus that hold one), `Entry` (direction), and the pseudo
  content. Deferred, honestly: `text-align`/`justify` on `Entry` and
  `TextArea` (editing widgets own their layout for caret and
  selection correctness — they clip and scroll rather than justify);
  `RichLabel` composes runs itself and takes the font group only;
  CJK justify (rows without spaces stay start-aligned — inter-cluster
  distribution is unbuilt); `hyphens: auto` (no hyphenation
  dictionary ships; without one, auto would guess); the `line-break`
  strictness profiles for CJK punctuation (the typesetting segmenter
  computes one UAX #14 profile — auto/normal/strict read the same,
  loose and anywhere differ as noted); `unicode-bidi`'s embed and
  isolate embeddings (internal/text resolves one paragraph per
  widget — there is no embedding level to open); `vertical-align`'s
  top/middle/bottom/text-top/text-bottom (they need the inline
  formatting context's box, which a lone widget baseline has not);
  `writing-mode`'s vertical forms (vertical-rl/vertical-lr parse and
  compute but paint horizontally — the whole paint stack, caret
  math, wrapping, and ellipsizing is horizontal); `word-spacing` and
  `text-indent` percentages (they need the containing block the
  per-declaration parse cache cannot see; lengths and — for
  vertical-align — line-box fractions resolve at paint).
- **Box.**
  - `background`, `background-color`, `background-image` (the linear,
    radial and conic gradients and their `repeating-` forms)
  - `padding-*`, `margin-*` (`auto` too: the CSS layouts below hand
    it the free space; programmatic containers read it as 0)
  - `border[-side][-width|-style|-color]` (a width draws only with a
    visible style), `border-*-radius`
  - `box-shadow` (lists, inset, spread and blur)
  - `outline[-width|-style|-color|-offset]`
  - `border-spacing` (the Box gap)
- **Layout.** `display`, `width`, `height`, `min-width`, `min-height`,
  `max-width`, `max-height`, `box-sizing`, `aspect-ratio`, `position`,
  `top`/`right`/`bottom`/`left`/`inset`, `z-index`, the flexbox group
  (`flex`, `flex-grow`, `flex-shrink`, `flex-basis`, `flex-direction`,
  `flex-wrap`, `flex-flow`, `order`, `justify-content`,
  `align-items`, `align-self`, `align-content`, `place-content`,
  `gap`, `row-gap`, `column-gap`), `overflow`/`overflow-x`/
  `overflow-y`, `visibility`, `float` and `clear` — see
  [Layout](#layout).
- **Effects.** `opacity`, `filter`, `backdrop-filter`,
  `mix-blend-mode`, `isolation`, `clip-path`, `mask` (and the
  `mask-image`/`-position`/`-size`/`-repeat`/`-mode` longhands),
  `transform`, `transform-origin`, `perspective`, `transform-style`
  and `backface-visibility`.
- **Motion.**
  - `transition`, `transition-property`, `-duration`, `-delay`,
    `-timing-function` and `-behavior`, tweened on the animation
    clock. `transition-behavior: allow-discrete` flips a discrete
    property (`isolation`, `mix-blend-mode`, `backface-visibility`,
    `appearance`, `caret-shape`) through the tween — the from value
    holds to halfway, the to value from halfway on.
  - `animation` and its longhands, including `animation-play-state`
    and `animation-composition` (`replace`, `add`, `accumulate`),
    driving `@keyframes`. The composition joins each frame onto the
    underlying cascade value: add sums the scalars and colors
    (clamped) and post-composes the transforms; accumulate reads the
    same here — the animation channels carry scalars and composed
    maps, where the two modes agree.
  - Every timing function slot takes `steps(n)` with its jump
    position (`jump-start`, `jump-end`, `jump-none`, `jump-both`,
    `start`, `end`) and `linear(...)`, the piecewise list of values
    with optional percent stops.
  - `@starting-style` inside a rule names the values a newly styled
    widget's transitions start from — the entry transition.
- **Icons.** `-gtk-icon-size`, `-gtk-icon-source`, `-gtk-icon-palette`
  and `-gtk-icon-transform`.
- **Interaction.**
  - `cursor`: every CSS keyword (default, pointer, crosshair, text,
    wait, progress, help, move, the n/s/e/w/ne/nw/se/sw/ns/ew/nesw/
    nwse-resize family, zoom-in, zoom-out, grab, grabbing, none,
    context-menu, cell, vertical-text, alias, copy, no-drop,
    not-allowed, col-resize, row-resize, all-scroll) maps onto the
    cursor-shape-v1 vocabulary, with the xcursor theme as the
    fallback for compositors without the protocol (the theme path
    carries the classic core names). `cursor: url("file.cur") x y`
    loads the named xcursor file and pins the CSS hotspot over the
    file's own; the first frame shows. The property inherits — one
    declaration covers a subtree — and outranks a widget's built-in
    shape (text fields' caret, buttons' hand); `auto` defers to it.
  - `pointer-events`: `none` takes the widget out of hit testing; set
    on a container it covers the children too, and the press falls
    through to whatever is behind (a Stack's lower child, the parent).
    Widgets with hand-written HitTests outside the shared HitLeaf do
    not honor it. `auto` is the default.
  - `accent-color`: a color or `auto` (the theme accent). The checked
    and accent surfaces follow it: the switch's on track, the
    checkbutton and radio fill, the progressbar and levelbar fill,
    the slider's highlight. An explicit background on the part still
    wins.
  - `touch-action`: `pan-x`, `pan-y`, `none` (and the direction forms
    `pan-left/right/up/down`, which gate their axis) gate the touch
    scroll path — the two-finger pan and the kinetic glide after it.
    The wheel path is never gated (touch-action speaks to touch
    manipulation only) and the touchpad's pixel scroll shares the
    gated path. `manipulation` parses as both pan axes; the pinch
    half of its meaning is deferred below.
  - `appearance`: `none` and `auto` parse and compute. `none`'s chrome
    stripping is deferred: the widgets' default look is programmatic
    painting against the theme, not CSS pseudo-element chrome, so
    stripping it means redesigning each widget's unstyled paint; a
    stylesheet already restyles every chrome surface through the box
    model (`background`, `border`, the part selectors).
- **Scrolling.** The properties style the `Scroll` viewport
  (`scrolledwindow`) and what it scrolls.
  - `scroll-behavior`: `smooth` animates programmatic scrolls —
    `RevealRect` and friends — over an eased 300 ms; `auto` (and the
    wheel, always) lands at once, as the spec scopes smooth scrolling
    to programmatic scrolls.
  - `scroll-margin` (all four sides, negatives allowed) grows a
    widget's revealed rect; `scroll-padding` insets the viewport the
    reveal and snap math use.
  - `scroll-snap-type` (`none`, or `x`/`y`/`block`/`inline`/`both`
    with `proximity` or `mandatory`) turns snapping on for the
    container; `scroll-snap-align` (`start`/`end`/`center`, block and
    inline) on the scrolled children names the snap areas, and
    `scroll-snap-stop: always` makes one a gesture cannot skip past.
    Mandatory snapping re-settles through a relayout or resize;
    proximity takes a position when the resting offset lies within
    half a viewport of it (the UA freedom the spec leaves).
  - `overscroll-behavior` (and `-x`/`-y`; `auto`, `contain`, `none`):
    `auto` chains motion a clamp swallowed to the parent scroll — the
    root scroll, with none above, takes the edge glow — `contain`
    keeps the glow here without chaining, `none` shows nothing.
  - `scrollbar-color` (`auto` or thumb then track) and
    `scrollbar-width` (`auto`, `thin`, `none`) reach the bars —
    Scroll's own and the standalone Scrollbar — and inherit, so one
    declaration covers a subtree. `scrollbar-gutter` (`auto`,
    `stable`, `stable both-edges`) reserves the bar's column in
    measure even without overflow (a matching one on the near edge
    under `both-edges`), so content does not reflow when a bar
    appears.

The box model applies to the containers, controls and text widgets,
painted in GTK's order: outer shadows, background, image, inset
shadows, border, content, outline. Opacity and filter apply over the
subtree, and ink outside the border box joins the damage rect.

### Deferred

- `appearance: none` paints nothing (parses and computes): widget
  chrome here is programmatic theme painting, not CSS pseudo-element
  chrome, so `none` needs a per-widget unstyled redesign.
- `touch-action`'s pinch semantics (`manipulation`'s zoom half,
  `pinch-zoom`) are deferred: the gesture layer exposes pinch only as
  a claimed GesturePinch a widget opts into, not an axis the scroll
gates can mask; `manipulation` computes to both pan axes meanwhile.

## Layout

The widget tree is the document. Every widget is one box; a Box (and
any type embedding one) is the container the CSS layouts run in, the
way a `div` is in a browser.

**The interaction rule.** Where the stylesheet sets a layout property,
CSS wins; where it does not, the programmatic layout stands. A Box
without a `display` declaration keeps its GTK box layout (axis,
spacing, expand, AppendAligned, homogeneous), and inside a CSS layout
an undeclared property falls back to the programmatic setting: the
box axis for `flex-direction` (a column box is a column flexbox), the
box spacing for `gap`, Append's expand (or SetHExpand/SetVExpand) for
`flex-grow`, AppendAligned and SetHAlign/SetVAlign across for
`align-self` when `align-items` is not declared either. The CSS-wide
keywords (`initial`, `unset`, `revert`, and `all`) return a layout
longhand to undeclared, so a GTK-style `* { all: unset; }` reset keeps
the toolkit's layout; `min-width`/`min-height` keep their reading of
it as a zero floor, and `visibility` inherits as CSS says.

**display.** `block`, `inline`, `inline-block`, `flow-root`, `flex`,
`inline-flex`, `contents`, `none`, `grid` and `inline-grid`, and the
two-keyword forms (`block flow`, `inline flow-root`, `inline flex`,
...). On a Box the inner display picks the layout of its children:
normal flow for the block and inline values, flexbox for the flex
values. On every widget the outer display decides how it sits in a CSS
flow parent: block-level, or inline-level for `inline`,
`inline-block`, `inline-flex` and `inline-grid`; undeclared, a widget
is block-level. `none` takes the widget out of layout, paint, hit
testing, Tab focus and the accessibility snapshot, exactly like
SetVisible(false), in any container. `contents` dissolves a Box into
its parent's CSS layout: its children become the parent's items while
staying its children (they inherit from it, and it paints them); it
draws no box of its own and takes no hits, and its bounds cover its
children.

**Normal flow.** Block-level children stack down the content box, as
wide as the band beside the floats unless a width says otherwise;
`margin: auto` centers a narrower block, a single auto side pushes it,
and an RTL box aligns it right. Adjacent block siblings' vertical
margins collapse (the largest positive plus the most negative).
Inline-level children fill line boxes, breaking when the next does not
fit, aligned by `text-align` (start, end, left, right, center) and on
the line by `vertical-align` (the baseline family — sub, super and
lengths shift the widget's own baseline — or top, middle, bottom); a
widget without a baseline sits on its bottom margin edge, as an
inline-block does. RTL lines run right to left. `float: left`, `right`,
`inline-start` and `inline-end` pull a child to that side as high as
the content before it allows — onto the current line when it fits
beside its inline content — with blocks and line boxes flowing beside
the floats and moving down when they do not fit; `clear` (`left`,
`right`, `both`, and the logical sides) moves a block or a float below
them. The box's height includes its floats.

**Flexbox.** The CSS Flexible Box Layout algorithm (§9): flex base
sizes from `flex-basis` (a length, a percentage of a definite main
size, `auto` deferring to `width`/`height`, `content` and the content
keywords), hypothetical sizes clamped by the min/max sizes with the
automatic minimum (`min-width: auto` is the content's min-content size,
capped by a definite size, and zero for an item whose overflow clips),
greedy line breaking with the gaps, the flexible-length resolution
with its freeze loop (a flex-grow or flex-shrink sum below one
distributes only that fraction; shrinking scales by the inner base
size), cross sizes at the used main sizes, single-line containers
sized to a definite cross size, baseline groups, `align-content` over
the lines (`normal` and `stretch` stretch them; the distributed values
fall back as CSS says), `align-self` per item with `stretch` honoring
max sizes and auto cross margins, then the main axis: auto margins
take the free space first, else `justify-content`. Every keyword is
honored: `flex-start`, `flex-end`, `start`, `end`, `left`, `right`,
`center`, `normal`, `stretch`, `space-between`, `space-around`,
`space-evenly`, `self-start`, `self-end`, `baseline`, `first`/`last
baseline`, with `safe` falling back to start on overflow and `unsafe`
(the default) overflowing both ways. `row-reverse`, `column-reverse`,
`wrap-reverse` and an RTL `direction` flip the axes the CSS way (start
and end stay with the writing mode, flex-start and flex-end follow the
flow). `order` reorders layout and paint alike. `visibility: collapse`
removes an item's main size but keeps its cross size as a strut on its
line. Gap percentages resolve against the content box.

**Sizing.** `width`, `height`, `min-*` and `max-*` take lengths,
percentages, `calc()` sums and products mixing the two (`calc(100% -
2em)`), `auto` (`none` for max-*), `min-content`, `max-content`,
`fit-content` and `fit-content(<length-percentage>)`; `box-sizing`
picks the content or border box they measure; `aspect-ratio` (`auto`,
`<ratio>`, `auto <ratio>`) transfers a definite size, or the measured
width when both are auto, to the other axis. They apply to every
widget, in any container: a definite size or a bound holds the slot a
container offers (a column box no longer stretches a 100px-wide child,
which sits at the start, or by SetHAlign), and a min size widens it.
Percentages resolve against the containing block — the content box a
CSS layout hands its items, and in a programmatic container the space
it offers in the measure constraint. min-content is the widget's
floor (what it can give up as a Shrinker or WidthShrinker, lowered to
a MinSizer's floor — a wrapping label's widest word); max-content is
its unbounded natural size; in the block axis the content keywords are
auto, as CSS reads them. A plain-pixel `min-width`/`min-height` stays
each widget's own floor, GTK's reading (a switch's knob, a slider's
handle).

**Positioning.** `relative` shifts a widget by its offsets (left over
right, top over bottom, percentages of the parent's content box)
without moving its siblings. `sticky` keeps a widget inside the
nearest Scroll's visible viewport, inset by its declared sides, until
its parent's content box runs out. `absolute` and `fixed` leave the
flow — the container measures nothing for them — and resolve against
their containing block: the padding box of the nearest positioned
ancestor for `absolute`, the tree root's box (the window) for `fixed`
and for an absolute without one. Each axis follows CSS: a definite
size wins; both insets set stretch an auto size between them and
center a definite one under auto margins; otherwise the widget takes
its shrink-to-fit size from the set inset, or from its static position
— where the flow reached — with neither. `z-index` (auto stacks as 0)
orders a container's children in the CSS painting layers: negative
z-index, in-flow blocks, floats, inline-level boxes, positioned boxes
at 0, positive z-index; it applies to positioned widgets and to flex
items. Paint and hit testing read the one order, so the topmost
painted child is the one the pointer hits.

**overflow and visibility.** `overflow`, `overflow-x` and `overflow-y`
(`visible`, `hidden`, `clip`, `scroll`, `auto`) clip a Box's children
to its padding box on each clipping axis, for paint and hit testing
alike. `visibility: hidden` keeps the widget's space but paints
nothing, takes no hits and no focus, and leaves the accessibility
snapshot; a `visibility: visible` child of a hidden Box still shows.

### Layout deferrals

- Grid layout: `display: grid` and `inline-grid` parse, compute and
  report supported to `@supports`, and lay a Box's children out as
  block flow (`flow-root`) until the grid tranche lands; the
  `grid-template-*`, `grid-auto-*`, `grid-row`/`grid-column`/`grid-area`
  and `place-items`/`place-self` properties are not parsed yet (an
  unknown-property warning).
- Inner display on containers other than Box: Grid, CenterBox, Stack,
  Paned and the other built-in containers keep their own layout
  algorithms — their placement is their API — so `display: flex` or
  `block` there only sets how they sit in a CSS parent, and
  `display: contents` dissolves only a Box (elsewhere the widget keeps
  its box).
- Inline formatting is of whole widgets: a widget is an atomic inline
  box, so `display: inline` lays out as `inline-block` and a label's
  text never fragments across its parent's line boxes;
  `text-align: justify` on line boxes reads as start, since there are
  no spaces between widgets to stretch.
- Every CSS-laid-out box roots its own block formatting context
  (`block` reads as `flow-root`): floats never reach into a nested
  box, and margins never collapse through a parent and its first or
  last child or through an empty box.
- Stacking contexts are per container: `z-index` orders a widget
  among its siblings and never lifts it above its container's
  siblings, and a dissolved (`contents`) box's children paint with
  it. Absolute and fixed widgets paint in their parent's pass, so an
  ancestor's clip (`overflow`, a Scroll viewport) still clips them,
  and a transform does not make a containing block.
- The static position of an absolute child of a flex container is the
  content box's start corner; CSS aligns it as the sole flex item.
- `overflow: scroll` and `auto` clip without scrollbars — wrap the
  content in a Scroll for scrolling — and overflow has no effect on
  widgets other than Box, whose content already stays inside their
  box (labels ellipsize or clip, a Scroll scrolls by its own policy).
- `min()`, `max()` and `clamp()` mixing percentages with lengths are
  invalid: the comparison needs the layout-time basis, which the
  compute-time evaluator never sees; `calc()` sums and products of
  them work.
- `last baseline` aligns on a widget's single baseline (widgets report
  one), and baseline alignment in a column flex container falls back
  to start, as CSS's does.
- A `visibility: visible` descendant shows inside a hidden Box only; a
  hidden widget of another type hides its whole subtree.
- Content-box sizes count the built-in padding of a widget that keeps
  it outside the CSS box (Entry's and TextArea's text inset) as
  content.
- The window's root widget takes the window size; CSS sizes on it do
  not apply.
- Layout is horizontal-tb: vertical writing modes compute but lay out
  horizontally, as their paint does.

## Rendering effects

The effect properties apply to a widget's painted result, per frame.
A stylesheet that declares none of them keeps the exact old paint
path — opacity through the canvas's alpha stack, brightness through
its brightness stack — with no offscreen buffer and no allocation
(the budget tests pin both).

- **`filter`**: `blur()`, `brightness()`, `contrast()`, `grayscale()`,
  `hue-rotate()`, `invert()`, `opacity()`, `saturate()`, `sepia()` and
  `drop-shadow()`, in list order. The color functions apply the
  filter-effects matrices (each function's output clamps before the
  next, as the spec defines); `blur()` is a three-pass box
  approximation of the Gaussian (within a couple of counts of it);
  `drop-shadow()` blurs and tints the painted silhouette and composites
  it under the element. A `brightness()`-only list keeps the inline
  fast path (no layer). Deferred: `url()` filters — they name SVG
  filter graphs, which need a document loader the engine does not
  have; a declaration carrying one is skipped with a warning.
- **`backdrop-filter`**: the same function list, applied to what has
  been painted behind the widget this frame — the canvas region under
  the border box is snapshotted, filtered, clipped to the rounded
  border box, and composited back over the original backdrop. Deferred:
  `url()` filters, for the same reason as `filter`.
- **`mix-blend-mode`**: all sixteen modes — the separable set
  (`multiply`, `screen`, `overlay`, `darken`, `lighten`, `color-dodge`,
  `color-burn`, `hard-light`, `soft-light`, `difference`, `exclusion`)
  and the non-separable set (`hue`, `saturation`, `color`,
  `luminosity`) — blended per the compositing spec, then composited
  source-over. **`isolation`**: `isolate` makes the subtree its own
  compositing group (an offscreen layer), so descendants' blend modes
  blend against the group instead of everything below; `auto` blends
  against the canvas. A blending element that also carries a transform
  composites through the transform first (the projective composite is
  source-over only).
- **`clip-path`**: `inset()` (with `round` corners), `circle()`,
  `ellipse()` (keywords and percentages resolve against the border
  box), `polygon()` (the fill-rule keyword is accepted; every path
  fills nonzero) and `path()` — the SVG `d` grammar with arcs
  subdivided into cubics. The shape's coverage multiplies the
  element's painted result.
- **`mask`**: `mask-image` as a gradient or a `url()` image file, with
  `mask-position`, `mask-size` (`auto`, `cover`, `contain`, or
  lengths/percentages), `mask-repeat` (`repeat`/`no-repeat`; `round`
  and `space` read as `repeat`) and `mask-mode` (`alpha`, `luminance`,
  `match-source`). Alpha masks read the mask's alpha, luminance masks
  its WCAG luminance. Deferred: multiple mask layers (a comma list
  rejects the declaration — the engine composites one mask layer), and
  mask-border.
- **3-D transforms**: `perspective()` (as a transform function),
  `rotateX()`, `rotateY()` and `translateZ()` compose as true 4x4
  matrices restricted to the element's plane — a homography — so a
  tilt foreshortens under perspective and a `translateZ` zooms. The
  `perspective` property takes the same distance and composes outside
  the transform (the engine applies every transform to the node
  itself, GTK's model, so `perspective` affects the element's own
  composed map rather than only its children). A perspective with a
  purely 2-D transform is a no-op — a flat plane at z=0 projects onto
  itself, which is CSS's own math. **`backface-visibility`**: `hidden`
  hides the element when its composed map flips the plane (the
  flat-pipeline reading: a negative determinant of the plane map,
  which is what `rotateY(180deg)` and `scaleX(-1)` produce).
  **Deferred**: `transform-style: preserve-3d` parses and computes but
  paints as `flat` — the engine composites each subtree as one
  offscreen layer with no z-interaction between siblings, so a shared
  3-D rendering context (interpenetrating children, per-fragment
  sorting) has no compositor to run on; `matrix3d()`, `rotate3d()`,
  `scale3d()` and `translate3d()` are rejected for the same reason
  (only the plane-restricted functions above are honest on this
  pipeline).

## Cascade

The origins, from highest to lowest precedence:

1. programmatic per-widget colors (`Button.Bg`, `Entry.SetColor`, ...)
2. stylesheet declarations, ordered by `!important` first (an
   important declaration at any priority beats every normal one;
   among importants the normal order applies), then priority, then
   @scope proximity (an inner scope beats an outer one at equal
   specificity), then cascade layer (unlayered rules outrank every
   `@layer`; among layers, the later-declared wins — `@layer a, b;`
   fixes the order, `@layer name { ... }` styles in it, and a dotted
   name's parent outranks it), then specificity `(ids,
   classes+states, elements)`, then source order
3. the `Theme` and its derived state colors

`revert` rolls back to the theme origin — it behaves as `unset`, the
next origin down. `revert-layer` rolls to the next cascade layer
down within the same origin, and walks the chain: a revert-layer
beneath a revert-layer rolls another layer down, until a concrete
declaration or the bottom (then `unset`).

Constructor metrics (a button's padding, a label's size) are
defaults that a stylesheet overrides. `color`, the `font-*` group,
`cursor`, the CSS Text properties and `visibility` inherit. Box and
layout properties, and `vertical-align`, `line-clamp`,
`text-overflow`, `unicode-bidi` and `font-synthesis`, do not.

## Media state

`@media` conditions evaluate per compute against the tree's media
state:

- **Viewport.** `widget.SetMediaViewportFor(root, w, h)` — the app
  layer feeds it from each window's resize callback, and every
  window's tree computes against its own size (one stylesheet, two
  windows, two results). `widget.SetMediaViewport` remains the
  process-wide fallback for trees whose root reported nothing (the
  last value set wins there); width, height, aspect-ratio and
  orientation features read the tree's own viewport. Until set, the
  viewport is 0×0, so min-conditions never match.
- **Appearance.** `prefers-color-scheme` derives from the active
  theme's polarity (`Theme.IsDark`) and `prefers-reduced-motion`
  from its animation gate, so `SetTheme` re-evaluates them;
  `prefers-contrast` comes from `widget.SetMediaContrast` (the
  desktop's contrast preference, which no Theme field carries), and
  `resolution` from `widget.SetMediaResolution` (dpi, 96 by default).

A viewport or preference change restyles the whole tree — but only
  when some loaded stylesheet carries `@media` rules; without one,
  the setters record the state and style nothing.

## Container queries

`container-type: inline-size | size | normal` and `container-name:`
make a widget a query container; `@container <name>? (query)` rules
evaluate per compute against the nearest container in the node's
ancestry (its arranged bounds), and the container units resolve
against the same container. A `size` container queries both axes;
an `inline-size` container's block axis — and the two-axis features,
`aspect-ratio` and `orientation` — read as unknown (false, negated
under `not`). A name filter matches the container's `container-name`
list, and nested `@container` blocks intersect. A container's
resize restyles its subtree like a viewport change does, so queries
re-evaluate on window resize (and on any relayout that moves a
container's bounds when a sheet reads them).

## Performance

Style is cached per widget and invalidated, never re-matched per paint:

- Rules are bucketed by their rightmost simple selector.
- `Sheet.Match` allocates nothing (`TestValuesNoAllocationPerMatch`);
  the new selector forms keep that: attribute tests read the filled
  target, nested `:is()`/`:not()` reuse a fixed buffer pool per
  nesting level, `:nth-child(of S)` fills a per-depth sibling buffer,
  and `:has()` is the only downward walk — budgeted (above). The
  revert-layer chain records declarations only when a loaded sheet
  carries a `revert-layer` value.
- A stylesheet load bumps a generation, and widgets recompute lazily by
  stamp compare, so nothing walks the tree.
- A class, id or state flip restyles the subtree only when a loaded
  selector tests that fact on an ancestor, and siblings only when
  sibling selectors are loaded.
- The steady-state frame reads the computed struct and makes no style
  calls. Rendering effects (a filter, a blend mode, a clip path, a
  mask, a backdrop filter) paint the widget into an offscreen layer
  cached on the widget across frames — allocated once per size — and
  composite it back through the effect chain; the no-effect frame
  never allocates (the budget tests pin it).
- A tree that declares no layout property takes the programmatic
  layout unchanged. The CSS layouts run their passes in per-depth
  buffers reused across frames, so a static flex and flow tree
  measures and arranges without allocating
  (`TestCSSLayoutSteadyFrameAllocatesNothing`); a layout property's
  change relayouts only the branch it sits in.

`BenchmarkStyleGallery` (73 widgets, 25 rules, i5-10400F):

| Operation | Time | Allocations |
| --- | --- | --- |
| full-tree restyle | 11.4 µs | 9 (all in the benchmark's walk) |
| single-widget class toggle | about 170 ns | 0 |
| full paint, stylesheet loaded | unchanged against the unstyled paint | — |
