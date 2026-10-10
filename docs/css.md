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
- **Combinators.** ` `, `>`, `+` and `~`.
- **State pseudo-classes.**
  - `:hover` matches the hovered widget and its ancestors (GTK's rule).
  - `:active`, `:focus`, `:focus-visible` and `:focus-within`.
  - `:disabled`, `:checked`, `:selected`, `:indeterminate` and
    `:backdrop`. These are app-driven through `SetState`.
- **Structural pseudo-classes.** `:first-child`, `:last-child`,
  `:only-child`, `:nth-child()`, `:nth-last-child()` (over visible
  siblings), `:root`, and `:not()` with compound lists and Level 4
  specificity.
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
  unresolvable `var()` acts as `unset`.
- **Math.** `calc()`, `min()`, `max()` and `clamp()`.
- **Units.** px, rem, em, pt, pc, in, cm, mm, %, deg, s and ms.
- **Colors.**
  - Hex (3/4/6/8 digits), named colors, `transparent` and
    `currentColor`.
  - `rgb[a]()`, `hsl[a]()` and `color-mix(in srgb, …)`.
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
  - `letter-spacing`, `line-height`, `text-transform`
  - `text-decoration` (underline, overline and line-through in solid,
    double, dotted, dashed or wavy, with a color), `caret-color`
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

The box model applies to the containers, controls and text widgets,
painted in GTK's order: outer shadows, background, image, inset
shadows, border, content, outline. Opacity and filter apply over the
subtree, and ink outside the border box joins the damage rect.

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
defaults that a stylesheet overrides. `color` and `font-*` inherit.
Box properties do not.

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
- `Sheet.Match` allocates nothing (`TestValuesNoAllocationPerMatch`).
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
