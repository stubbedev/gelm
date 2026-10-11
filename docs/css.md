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
  `widget.SetFaceResolver` (for example over `app.FontWeighted`).
  Without a resolver they keep the constructor face. A variable face
  still takes `font-weight` on its wght axis.
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
- `@import "file.css";` (or `url(...)`, with an optional trailing
  media query) loads and inlines its target relative to the importing
  stylesheet's path — file loads resolve beside the file, string
  stylesheets relative to the working directory. Cycles are detected
  per import chain and reported precisely
  (`@import cycle: a.css -> b.css -> a.css`); a missing file or a
  malformed import warns and drops that import only. `layer()` and
  `supports()` qualifiers are not supported yet.
- `@layer` orders cascade layers (below).
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
  with compound lists and Level 4 specificity.
- **Pseudo-elements.** `::before` and `::after` cascade their own
  style (inherited from the element's) with `content:` of strings,
  `attr(name)` (the built-ins and data attributes), `counter(name)`
  and `counters(name, sep)`; `counter-reset` and
  `counter-increment` count in document order (through previous
  siblings). The pseudo-element's box layers and content paint over
  the widget's background — content at the content start (`::before`)
  and end (`::after`), in the pseudo-element's own color and
  font-size — wherever the widget paints box layers (`PaintBoxLayers`)
  or shapes text (labels and entries today). `widget.PseudoStyle` and
  `widget.PseudoContent` read the computed values. `::placeholder`,
  `::selection` and `::marker` desugar onto the existing part
  machinery: `entry::placeholder` styles the entry's `placeholder`
  part (as a descendant part selector) and `::selection` its
  `selection` part; `::marker` matches a `marker` part.
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
  vmax (the media viewport the `@media` size features read), pt, pc,
  in, cm, mm, %, deg, grad, rad, turn, s and ms. Container-query
  units (cqw/cqh/…) are not supported yet — container queries are a
  separate item.
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
  - `padding-*`, `margin-*`, `min-width`, `min-height`
  - `border[-side][-width|-style|-color]` (a width draws only with a
    visible style), `border-*-radius`
  - `box-shadow` (lists, inset, spread and blur)
  - `outline[-width|-style|-color|-offset]`
  - `border-spacing` (the Box gap)
- **Effects.** `opacity`, `filter: brightness()`, `transform` and
  `transform-origin`.
- **Motion.**
  - `transition`, `transition-property`, `-duration`, `-delay` and
    `-timing-function`, tweened on the animation clock
  - `animation` and its longhands, including `animation-play-state`,
    driving `@keyframes`
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

## Cascade

The origins, from highest to lowest precedence:

1. programmatic per-widget colors (`Button.Bg`, `Entry.SetColor`, ...)
2. stylesheet declarations, ordered by `!important` first (an
   important declaration at any priority beats every normal one;
   among importants the normal order applies), then priority, then
   cascade layer (unlayered rules outrank every `@layer`; among
   layers, the later-declared wins — `@layer a, b;` fixes the order,
   `@layer name { ... }` styles in it), then specificity `(ids,
   classes+states, elements)`, then source order
3. the `Theme` and its derived state colors

`revert` rolls back to the theme origin — it behaves as `unset`, the
next origin down. `revert-layer` rolls to the next cascade layer
down within the same origin (one `revert-layer` chain step; a
revert-layer beneath a revert-layer behaves as `unset`), and as
`unset` when no lower layer declares the property.

Constructor metrics (a button's padding, a label's size) are
defaults that a stylesheet overrides. `color`, the `font-*` group,
`cursor`, and the CSS Text properties inherit. Box properties, and
`vertical-align`, `line-clamp`, `text-overflow`, `unicode-bidi` and
`font-synthesis`, do not.

## Media state

`@media` conditions evaluate per compute against the tree's media
state:

- **Viewport.** `widget.SetMediaViewport(w, h)` — the app feeds it
  from the window's resize callback (the last resized window wins);
  width, height, aspect-ratio and orientation features read it.
  Until set, the viewport is 0×0, so min-conditions never match.
- **Appearance.** `prefers-color-scheme` derives from the active
  theme's polarity (`Theme.IsDark`) and `prefers-reduced-motion`
  from its animation gate, so `SetTheme` re-evaluates them;
  `prefers-contrast` comes from `widget.SetMediaContrast` (the
  desktop's contrast preference, which no Theme field carries), and
  `resolution` from `widget.SetMediaResolution` (dpi, 96 by default).

A viewport or preference change restyles the whole tree — but only
  when some loaded stylesheet carries `@media` rules; without one,
  the setters record the state and style nothing.

## Performance

Style is cached per widget and invalidated, never re-matched per paint:

- Rules are bucketed by their rightmost simple selector.
- `Sheet.Match` allocates nothing (`TestValuesNoAllocationPerMatch`);
  the new selector forms keep that: attribute tests read the filled
  target, nested `:is()`/`:not()` reuse a fixed buffer pool per
  nesting level, and `:has()` is the only downward walk — budgeted
  (above).
- A stylesheet load bumps a generation, and widgets recompute lazily by
  stamp compare, so nothing walks the tree.
- A class, id or state flip restyles the subtree only when a loaded
  selector tests that fact on an ancestor, and siblings only when
  sibling selectors are loaded.
- The steady-state frame reads the computed struct and makes no style
  calls.

`BenchmarkStyleGallery` (73 widgets, 25 rules, i5-10400F):

| Operation | Time | Allocations |
| --- | --- | --- |
| full-tree restyle | 11.4 µs | 9 (all in the benchmark's walk) |
| single-widget class toggle | about 170 ns | 0 |
| full paint, stylesheet loaded | unchanged against the unstyled paint | — |
