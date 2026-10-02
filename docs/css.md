# gelm CSS support — design

Status: **implemented** (#76). The section at the end records what
shipped and the measured numbers; the rest of this document is the
reviewed spec (#75) that #76 builds exactly, nothing more. The
numbers in "Performance envelope" come from the prototype in
`bench/css-proto` and are the bar the implementation had to meet.

**v2 (GTK stylesheet parity).** The engine now loads real GTK
application stylesheets (wayle's compiled SCSS, ~1800 rules, parses in
~6 ms with no warnings). It supersedes the "out" lists below where they
disagree:

- **Syntax**: a CSS Syntax 3 tokenizer; `@keyframes`/`@media` blocks
  skip silently, other at-rules skip with a warning; nested (SCSS)
  blocks and `!important` still reject their rule.
- **Selectors**: combinators ` `, `>`, `+`, `~` (with backtracking);
  `:hover` (on the hovered widget *and its ancestors*, GTK's rule),
  `:active`, `:focus`, `:focus-visible` (keyboard focus),
  `:focus-within`, `:disabled`, `:checked`, `:selected`,
  `:indeterminate`, `:backdrop` (the app-driven ones through
  `SetState`), `:first-child`, `:last-child`, `:only-child`,
  `:nth-child()`/`:nth-last-child()` (visible siblings), `:root`,
  `:not()` (compound lists, Level 4 specificity).
- **Values**: custom properties (`--x`, inherited, computed where
  declared, cycles invalid) and `var()` with fallbacks; a declaration
  whose `var()` cannot resolve is invalid at computed-value time and
  acts as `unset`. `calc()`/`min()`/`max()`/`clamp()`; units px, rem
  (`SetRootFontSize`, default 16), em, pt, pc, in, cm, mm, %, deg,
  s/ms. Colors: hex (3/4/6/8), named, `transparent`, `currentColor`,
  `rgb[a]()`, `hsl[a]()`, `color-mix(in srgb, …)` (premultiplied,
  sub-100% sums scale alpha), GTK's `alpha()`, `shade()`, `mix()`.
  CSS-wide `inherit`/`initial`/`unset` and `all`.
- **Properties** (longhands, with their shorthands): `color`,
  `background[-color|-image]` (`linear-gradient` with angles, `to`
  sides and positioned stops), `opacity`, `filter: brightness()`,
  `padding-*`, `margin-*`, `border[-side][-width|-style|-color]`
  (a width draws only with a visible style, the CSS rule),
  `border-*-radius`, `box-shadow` (lists, offsets, spread, blur,
  `inset`), `outline[-width|-style|-color|-offset]`, `min-width`,
  `min-height`, `border-spacing` (Box gap), `font[-family|-size|-weight|-style]`,
  `letter-spacing` (computed, not yet painted), `text-transform`,
  `-gtk-icon-size`, `transition[-*]` (computed; not yet animated).
  Animation, icon-transform, text-decoration and similar GTK properties
  parse and drop silently.
- **Cascade**: prioritized stylesheets (`AddStylesheet`, GTK's provider
  priorities; `LoadStylesheet` is the application slot) and per-widget
  inline declarations (`SetInlineStyle`, a widget-scoped provider at
  `StylePriorityUser`) — priority, then specificity, then order.
- **Box model** (Box, Button, Label, Icon): self-applied margins,
  per-side border and padding, min sizes on the content box, painted in
  GTK's order (outer shadows, background, image, inset shadows, border,
  content, outline), with opacity and filter over the subtree; ink
  outside the border box joins the damage rect.
- **Invalidation**: a state/class/id flip restyles the subtree only
  when a loaded selector tests that fact on an ancestor, and siblings
  only when sibling selectors are loaded.

The maintainer's goal restated: GTK-flavored CSS as an **override
layer on top of the typed `Theme`** — the palette stays the source of
truth and keeps working with no stylesheet loaded; a stylesheet
overrides individual properties where rules match. This supersedes the
"no GtkCss analog" rejection of #32/#36 and, with it, the
"**no per-widget theme overrides**" non-goal
([architecture.md](architecture.md) "Theming" and "Non-goals", both
updated): a stylesheet is a supported way to restyle widgets per class
and state, on top of — not instead of — the palette.

## Selector subset

| Form | Meaning | Example |
| --- | --- | --- |
| element | GTK's CSS node name where gelm has the GTK widget (`button`, `label`, `entry`, `image`, `scale`, `scrolledwindow`, `textview`, `listview`, `flowbox`, `flowboxchild`, ...), else the widget type lowercased (`box`, `listrow`, ...); sub-nodes too (`entry > text`, `scale trough slider`) | `button { }` |
| `.class` | classes carried by the widget (new `AddClass`/`RemoveClass`/`HasClass`) | `.destructive { }` |
| `#id` | widget id (new `SetID`) | `#sidebar { }` |
| `:hover`, `:focus`, `:active`, `:disabled` | the widget state machine the paint path already tracks | `button:hover { }` |
| descendant combinator (space) | any depth | `box listrow:hover { }` |
| child combinator (`>`) | one depth | `box > label { }` |
| grouping (comma) | one rule, several selectors | `button, entry { }` |

Explicitly **out**, each with the reason it can come later if a need
shows up: attribute selectors (no widget attributes to select on),
`:nth-*` (tree order is not stable enough across relayouts to be
meaningful yet), pseudo-elements `::before`/`::after` (no content
model), `@media` and other at-rules other than none (no reflow-driven
querying), `@keyframes` and transitions/animations (the anim package
owns motion; CSS-driven animation is a separate cost/benefit decision),
`!important` (see Cascade), namespaced/multiple shadow DOM constructs.
A selector using only out-of-subset syntax is a parse error and takes
the rule-level error policy below.

Element names come from the widget type (`strings.ToLower` of the Go
type name) plus the few app-level surfaces that paint like widgets:
`dialog`, `popover`, `tooltip`, `toast`. The initial list is pinned by
a test; unknown element names in a stylesheet simply never match —
they are not errors.

## Cascade

- **Origin precedence**, high to low: (1) programmatic per-widget
  color (`button.SetBg`) — application code is the most specific
  intent there is; (2) matching stylesheet declarations; (3) the
  typed `Theme` and its derived-state methods (`HoverSurface`,
  `DisabledText`, ...). With no stylesheet loaded the behavior is
  bit-for-bit today's.
- **Specificity**: `(id, class-or-state, element)` counts, compared
  lexicographically; source order breaks ties (later wins). This is
  GTK/CSS's model minus inline style attributes (a widget's explicit
  colors are the inline style, and they sit above the cascade as (1)).
- **No `!important`.** It exists to fight specificity wars in large
  shared stylesheets; gelm stylesheets are application-local. A
  declaration using it is a rule-level parse error (skipped, warned).
- **Inheritance** follows CSS: `color` and `font-*` inherit from the
  parent widget's computed style (the root inherits from the
  `Theme`); `background-color`, `padding`, `border-*`,
  `border-radius`, `box-shadow`, `min-width`, `min-height` do not.

## Properties

Every property maps onto a parameter the render/theme layer already
consumes, and every property lists its consumer widgets — nothing
ships without a painter:

| Property | Maps to | Consumers | Inherits |
| --- | --- | --- | --- |
| `color` | text ink (canvas text calls) | Label, RichLabel, Entry, TextArea, MenuItem, Dropdown face | yes |
| `background-color` | widget background fill | every widget (Button, Entry, Box, ListRow, panels, Dialog, ...) | no |
| `padding` | content insets | Box, Button, Label, Entry, TextArea, containers | no |
| `font-family` | font chain head (sysfont/Chain) | text widgets | yes |
| `font-size` | text size (px) | text widgets | yes |
| `font-weight` | face weight selection | text widgets | yes |
| `border-radius` | rounded-rect corners | Button, Entry, Switch, ListRow, panels, popovers, toasts, dialogs | no |
| `border-width` | outline stroke width | Button, Entry focus ring | no |
| `border-color` | outline stroke color | Button, Entry focus ring | no |
| `box-shadow` | client shadow color/blur (one shadow: `COLOR BLUR` or `none`) | menus, popovers, tooltips, toasts, dialogs | no |
| `min-width`, `min-height` | MinSizer floors | Entry, TextArea, Button, Paned children | no |

Out for v1, with today's answer recorded: `margin` (layout geometry is
owned by Box/Grid/Spacing, not style), `opacity` (the fader widget owns
transitions), `spacing`/`gap` (same owner as margin), `text-decoration`
(no underline/strike in the text path), `transition`/`animation`
(the anim package owns motion).

## Parsing

**Hand-rolled tokenizer and parser, no dependency.** The subset is
small (one grammar page above); a dependency would drag in a full CSS
object model and a module the flake has to vendor forever, for
capabilities the subset deliberately excludes. The parser is a
byte-oriented state machine in the same style as the markup parser
(widget/markup.go): allocation-light, no regexp on any hot path.

**Error policy is Warn-and-skip-rule** (#55's degraded-but-running
contract): a malformed declaration skips that declaration; a malformed
selector skips that rule; everything around it still applies. A
stylesheet that parses only partially still applies its valid rules.
Warnings go to the injected library logger at Warn, silent by default,
like `SetTheme`'s contrast warnings.

## Performance envelope

Style work is **cached per widget and invalidated, never re-matched
per paint**:

- **Parse once** per stylesheet load: O(stylesheet bytes), producing
  rules with pre-parsed values (colors, lengths) — no strings left on
  the hot path.
- **Match index**: rules bucketed by their rightmost simple selector —
  element buckets, class buckets, id bucket, universal bucket. A
  widget consults only the buckets its own element name, classes, and
  id hit, so per-widget match cost is proportional to the rules that
  can plausibly apply, not to the stylesheet.
- **Computed style per widget**: one small struct, filled once per
  invalidation by walking matched declarations in precedence order and
  layering over the `Theme`-derived defaults. Values reference the
  parsed stylesheet (no per-widget copies of colors/lengths beyond the
  struct).
- **Invalidation granularity**: a class or state change restyles that
  widget and its subtree for inherited properties; a stylesheet
  (re)load and `SetTheme` restyle everything — both ride the existing
  damage walk (the `themeGen` stamp pattern). Layout-affecting
  properties (`padding`, `font-size`, `min-*`) request a relayout of
  the widget's window through the existing path; paint-only ones do
  not.
- **Steady state: zero style cost per frame.** No class/state change,
  no stylesheet change, no style work — the computed struct is read
  directly by the paint path exactly where `Theme` fields are read
  today.

**Prototype numbers** (bench/css-proto, the standard widget gallery
shape: ~200 widgets, 3-deep tree, 27 rules, element/class/id/state
selectors, descendant and child combinators, match+compute exactly as
scoped above; Intel i5-10400F, Go 1.27, recorded 2026-09-28): full
tree restyle **~59 µs**, single-widget restyle on a class toggle
**~274 ns**, steady-frame paint-path read of computed styles
**~880 ns at 0 allocs**. The full restyle is three orders of
magnitude under the 16.6 ms frame budget; the prototype's 261
allocs/op sit in its naive candidate assembly and per-call map,
which the real engine replaces with per-widget buffers and a rank
table - #76's numbers must land at or under these, not above.

**Measurement plan**: `BenchmarkStyleGallery` (widget package, the
#45 harness gallery) measures full-tree match+compute and per-widget
invalidation; CI archives it beside the existing benchmarks
(`just bench`). #76 records its numbers next to the prototype's in
this section; a regression beyond the prototype bar fails review.
Steady-state per-frame cost is asserted zero by construction (no style
calls on the paint path) and by benchmark (the gallery paint benchmark
must not move when a stylesheet is loaded).

## Stylesheet lifecycle

- `widget.LoadStylesheet(css string)` and
  `widget.LoadStylesheetFile(path string)`: parse, install, bump the
  style generation; the next frame repaints. Loading again replaces
  (not merges) the active stylesheet; loading an empty string removes
  styling and returns to theme-only.
- Dev hot reload: `LoadStylesheetFile` polls the file (stat per second
  on the existing app timer — one syscall per tick, no inotify
  dependency) and reloads on change, so a running showcase picks up
  edits for free. Polling starts on first file load and is the only
  recurring cost a stylesheet adds.

## API sketch

```go
widget.LoadStylesheetFile("theme.css")          // app-level, once at startup
widget.LoadStylesheet(`.title { color: #c00 }`) // or from a string

b := widget.NewButton(...)
b.AddClass("destructive")
b.SetID("save-button")                          // #save-button

widget.HasClass(b, "destructive")               // read side (tests, a11y)
```

Classes and ids ride the shared `node` like `SetEnabled`/`SetTooltip`
do; `SetID` is distinct from the accessible name (a11y names stay
semantic text, not style hooks).

## Exit criteria (what #76 must show)

1. Reviewed spec — this document, with the property table complete and
   every consumer named.
2. Selector subset implemented exactly as scoped; out-of-subset
   syntax takes the documented error policy; no silent acceptances.
3. Cascade proven by tests: programmatic color > stylesheet > theme,
   specificity order, source-order tie-break, inheritance set.
4. Every property has a golden test showing it applied on at least one
   consumer widget (render and widget goldens).
5. `BenchmarkStyleGallery` recorded in this document at or under the
   prototype bar; paint-path benchmarks unchanged with a stylesheet
   loaded.
6. docs/css.md gains an "implemented" section; README theming row and
   architecture.md non-goals updated (done for the non-goals in this
   ticket).

## Implemented (#76)

The subset above shipped as scoped. Where the code lives and the two
decisions the spec left open:

- **Engine** — `internal/style`: the hand-rolled byte-oriented parser
  (`parse.go`, the markup-parser style: no regexp, one pass, warnings
  through the library logger at Warn, warn-and-skip-rule throughout)
  and the bucketed matcher/cascader (`match.go`). Rules are indexed by
  their rightmost simple selector (element/class/id/universal
  buckets), specificity packs into one comparable rank
  `(ids, classes+states, elements, source order)`, and the cascade is
  a best-per-property rank compare over a scratch workspace —
  `Sheet.Match` allocates nothing (pinned by
  `TestValuesNoAllocationPerMatch`).
- **Widget side** — `widget/style.go`: `LoadStylesheet`/
  `LoadStylesheetFile` (atomic install, generation bump), the
  `AddClass`/`RemoveClass`/`HasClass`/`SetID` methods on the shared
  node, and the per-widget computed style. The app installs the
  hot-reload poller once (`app` lends its timer wheel via
  `widget.SetStylesheetPoller`; one stat per second after the first
  file load, nothing before).
- **Computed style, where it lives** — one `style.Values` struct per
  node, holding the winning declaration per property with a set mask.
  Consumers read it exactly where they read `Theme` fields today,
  layering programmatic widget color above the cascade above the
  theme (`pickc`/`picki`). The set mask is what keeps the no-stylesheet
  pixels bit-for-bit: nothing set, nothing changes.
- **Invalidation flow** — three stamps, no walks. A stylesheet load
  bumps `styleGen`; the measure caches and the damage collector notice
  per widget by stamp compare (the `themeGen` pattern), and each
  widget's cascade recomputes lazily on first read. Class, id, and
  state changes mark only the widget; the diff at recompute decides
  the rest: repaint self if values moved, `InvalidateLayout` if a
  layout-affecting one did, mark the subtree when an inherited one did
  — the damage collector recomputes pre-order, so an inherited change
  lands in the same drain. Steady-state paint reads the struct; two
  integer compares, zero style calls (the styled paint benchmark sits
  on the unstyled one).
- **Inheritance** — `color` and the `font-*` group inherit from the
  parent's computed style (recomputed on demand up the chain); the
  root's fallback is whatever the widget painted before the cascade
  spoke (its constructor color, the theme at paint), so a stylesheet
  that sets nothing anywhere changes nothing.

### Property consumers as implemented

The spec table's full consumer list is the direction; these are the
widgets that honor each property today, each with a golden:

| Property | Consumer(s) in this change | Golden |
| --- | --- | --- |
| `color` | Label (direct and inherited), Entry, TextArea, Toast ink | `css-label-color`, `css-label-color-inherited` |
| `background-color` | Button, Entry, Box (a bare box fills only when styled), TextArea, Toast, Elevation plate (`dialog`) | `css-button-properties`, `css-entry-min-border`, `css-textarea-background`, `css-dialog-card` |
| `padding` | Button (measure + arrange), Box, Entry text inset, Label content inset | `css-button-properties` |
| `font-family` | Label, through the app-installed face resolver (`widget.SetFaceResolver`; without one the constructor face stays, the same graceful no-op as an unknown element name) | `css-label-font-family` |
| `font-size` | Label (reshape + relayout), Entry (shape, measure, caret) | `css-label-font-size` |
| `font-weight` | Label, through the same resolver (`normal`/`bold`/number) | `css-label-font-weight` |
| `border-radius` | Button, Entry, Toast, Elevation | `css-button-properties`, `css-dialog-card` |
| `border-width`, `border-color` | Button and Entry stroke their outline (the fill shrinks inside the ring) | `css-button-properties`, `css-entry-min-border` |
| `box-shadow` | Toast and Elevation (`COLOR BLUR` or `none`; the damage ring follows the effective blur) | `css-toast-shadow`, `css-dialog-card` |
| `min-width`, `min-height` | Entry and TextArea: `MinSize` floors for negotiating containers, and Measure claims the floor before the constraints clamp | `css-entry-min-border` |

The element-name list: every node-embedding widget type lowercased
(pinned by `TestCSSElementNames`), plus `dialog`, `popover`, `tooltip`
— named by their app constructors onto the widget that paints the
card (pinned in `app/style_test.go`) — and `toast`, a widget type.

### Measured numbers

`BenchmarkStyleGallery` (widget package, the #45 harness gallery
showcase tree, 73 widgets, the 25-rule `galleryCSS` — element/class/
id/state selectors, descendant and child combinators, one id rule,
one universal; Intel i5-10400F, the prototype's machine class,
recorded 2026-09-28):

| Benchmark | Prototype bar | Implemented |
| --- | --- | --- |
| Full-tree restyle | ~59 µs over ~200 nodes (~295 ns/widget), 261 allocs | **11.4 µs over 73 widgets (~156 ns/widget)**, 9 allocs — all in the benchmark's `Children()` walk, none in the engine (`Sheet.Match` is 0 allocs) |
| Single-widget restyle, class toggle | ~274 ns | **~170 ns, 0 allocs** |
| Steady-frame paint, stylesheet loaded | ~880 ns read, 0 allocs | paint benchmark unchanged against the unstyled `BenchmarkShowcasePaint` (1.84 ms vs 1.83 ms full-window paint); the paint path reads the computed struct only |

The full restyle sits two orders of magnitude under the 16.6 ms frame
budget; the engine's per-widget buffers (scratch, reusable ancestor
adapters, rank table) are what replaced the prototype's 261 allocs.

### Deviations from the spec's letter, with reasons

- **Constructor metrics are defaults, not the programmatic layer.**
  The cascade's programmatic layer is colors (the spec's `button.SetBg`
  example). Constructor sizes — a button's padding, a label's
  sizePx — act as the default the stylesheet overrides, or a label's
  `font-size: 20` could never apply to a constructed label. Explicit
  per-widget colors keep their above-the-cascade seat.
- **`:focus` rides the router, `:hover`/`:active` ride the widgets' own
  fields.** The router is the one writer of focus, so it flips the
  style bit (`setFocusStyle`); hover and press stay in the fields the
  router already drives, which the matcher reads per compute — the
  setters mark the cascade stale.
- **`font-family`/`font-weight` resolve through an injectable face
  resolver** (`widget.SetFaceResolver`) rather than reaching into
  `internal/sysfont` directly: face loading is the app's font store's
  job, goldens never consult host fonts, and without a resolver the
  declarations keep the constructor face — degraded but running, like
  an unknown element name.
- **Popover names its content tree** (the card the app hands over is
  the popover surface), and the dialog names its Elevation card — or
  the plain root box when client shadows are off. There is no separate
  popover widget to name.
