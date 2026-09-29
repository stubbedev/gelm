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

// FontFallback wraps a face in the glyph fallback chain, so glyphs the
// primary family lacks still render.
func FontFallback(face *render.Typeface) *render.Chain {
	return sysfont.Fallback(face)
}
