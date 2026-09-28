# gelm CSS support — design

Status: **spec** (ticket #75). Ticket #76 builds exactly what this
document commits to, nothing more. The numbers in "Performance
envelope" come from the prototype in `bench/css-proto` and are the bar
#76 has to meet, not aspirations.

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
| element | widget type, lowercased (`button`, `label`, `entry`, `textarea`, `box`, `listrow`, `menuitem`, ...) | `button { }` |
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
