# gelm icons

How gelm resolves and paints themed icons (stubbedev/gelm#20).

## Pieces

- `internal/icons` — the freedesktop icon theme spec in pure Go:
  `index.theme` parsing (sections, `Inherits`, `Directories` /
  `ScaledDirectories`, per-directory `Size` / `Type` / `MinSize` /
  `MaxSize` / `Threshold` / `Scale`), the spec's lookup algorithm
  (exact-size pass, nearest-size pass, inheritance chain with hicolor
  last, unthemed base-directory fallback), `-symbolic` name fallback,
  and a raster cache keyed by (name, logical size, 120-based scale).
- `widget.Icon` — four constructors: `NewIcon` (a raster the caller
  made), `NewThemeIcon` (theme name), `NewFileIcon` (png / svg / svgz
  path), `NewSVGIcon` (embedded bytes, for apps that ship their own
  glyphs). Icons rasterize at the canvas device scale, so a fractional
  rescale (#14) re-rasterizes crisply.
- `render.Icon.Tint` — the recoloring primitive.

## Symbolic recoloring

Symbolic sources are `-symbolic` theme files and SVGs painting with
`currentColor` (oksvg rejects `currentColor` in strict mode, so
`internal/icons` rewrites it to opaque white before parsing). Recoloring
is a **mask pass**: the source is decoded once, and the tinted variant
keeps every pixel's alpha while taking the tint's color. This is what
GTK does for symbolic icons, and it survives opacity exactly
(anti-aliasing and `fill-opacity` carry into the result).

The honest limits of the mask approach: ink comes out one flat color.
Gradient hue variation collapses to its alpha ramp, and multi-color art
goes monochrome — which is the symbolic-icon contract, not a bug.
Gradients that encode their shape in *alpha* (the common symbolic idiom,
currentColor → transparent) survive unchanged.

A symbolic `widget.Icon` follows `widget.Current().Accent` automatically;
`SetTint` pins a different color (zero un-pins). Non-symbolic sources
keep their colors unless a tint is pinned.

## Theme changes are explicit (deferred scope)

There is **no live theme-change signal**. Following the desktop setting
would pull xsettings (or a settings-daemon protocol) in as a new
dependency, so #20 defers it deliberately. Instead:

- `icons.Cache.SetTheme` / `InvalidateTheme` drop every cached raster
  and bump `Generation`.
- `widget.Icon` compares its resolved generation against the cache in
  its damage drain, so the next frame after an explicit switch repaints
  the new theme's icons.

An app that wants to follow the desktop theme calls
`icons.Default().InvalidateTheme()` whenever it learns about a switch
(how it learns stays the app's business). If a live signal lands later,
it plugs in at exactly that call site.
