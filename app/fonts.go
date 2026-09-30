package app

import (
	"github.com/stubbedev/gelm/internal/sysfont"
	"github.com/stubbedev/gelm/render"
)

// Font resolves a system font family by name at the given logical
// pixel size. Malformed family names fall back per fontconfig; an
// unresolvable name is an error.
func Font(family string, sizePx float64) (*render.Typeface, error) {
	return sysfont.Best(family, sizePx)
}

// FontWeighted resolves a system font family at a CSS numeric weight
// (400 regular, 700 bold, ...) and style. A family without that face
// resolves to its closest one, as a CSS font matcher does.
func FontWeighted(family string, sizePx float64, weight int, italic bool) (*render.Typeface, error) {
	return sysfont.Weighted(family, sizePx, weight, italic)
}

// FontVariant resolves the bold/italic face of base's family, falling
// back to the closest face the family has.
func FontVariant(base *render.Typeface, bold, italic bool) (*render.Typeface, error) {
	return sysfont.Variant(base, bold, italic)
}

// FontFallback wraps a face in the glyph fallback chain, so glyphs the
// primary family lacks still render.
func FontFallback(face *render.Typeface) *render.Chain {
	return sysfont.Fallback(face)
}
