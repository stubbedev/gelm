// Package sysfont resolves system fonts through fontscan's CSS font
// selection rules, replacing per-app guesswork. Family matching here
// understands generic families, so "sans-serif" can never resolve to a
// serif face.
package sysfont

import (
	"fmt"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/fontscan"

	"github.com/stubbedev/gelm/render"
)

// quietLogger discards fontscan warnings.
type quietLogger struct{}

// Printf implements fontscan.Logger.
func (quietLogger) Printf(string, ...any) {}

// Sans returns the system's default sans-serif face, resolved with the
// same rules a browser applies to font-family: sans-serif.
func Sans() (*render.Typeface, error) {
	return resolve(fontscan.SansSerif)
}

// Monospace returns the system's default monospace face.
func Monospace() (*render.Typeface, error) {
	return resolve(fontscan.Monospace)
}

// Serif returns the system's default serif face.
func Serif() (*render.Typeface, error) {
	return resolve(fontscan.Serif)
}

// Variant resolves a bold, italic, or bold-italic face of the same
// family base was resolved from, with the CSS matcher's fallback: a
// family without the requested face returns its closest face, usually
// the regular one. RichLabel wiring compares identity with base and
// downgrades to plain rendering when they match.
func Variant(base *render.Typeface, bold, italic bool) (*render.Typeface, error) {
	aspect := font.Aspect{Style: font.StyleNormal, Weight: font.WeightNormal}
	if bold {
		aspect.Weight = font.WeightBold
	}
	if italic {
		aspect.Style = font.StyleItalic
	}
	fm := fontscan.NewFontMap(quietLogger{})
	// An empty cache path lets fontscan pick its platform default.
	if err := fm.UseSystemFonts(""); err != nil {
		return nil, fmt.Errorf("sysfont: scan system fonts: %w", err)
	}
	fm.SetQuery(fontscan.Query{Families: []string{base.Family()}, Aspect: aspect})
	face := fm.ResolveFace('x')
	tf, err := render.NewTypeface(face)
	if err != nil {
		return nil, fmt.Errorf("sysfont: no %s variant (bold=%v, italic=%v) found", base.Family(), bold, italic)
	}
	return tf, nil
}

// resolve asks the font map for the best regular face of a generic
// family; the matcher applies CSS fallback rules, so the result honors
// the user's font configuration.
func resolve(family string) (*render.Typeface, error) {
	fm := fontscan.NewFontMap(quietLogger{})
	// An empty cache path lets fontscan pick its platform default.
	if err := fm.UseSystemFonts(""); err != nil {
		return nil, fmt.Errorf("sysfont: scan system fonts: %w", err)
	}
	fm.SetQuery(fontscan.Query{
		Families: []string{family},
		Aspect: font.Aspect{
			Style:  font.StyleNormal,
			Weight: font.WeightNormal,
		},
	})
	face := fm.ResolveFace('x')
	tf, err := render.NewTypeface(face)
	if err != nil {
		return nil, fmt.Errorf("sysfont: no %s face found", family)
	}
	return tf, nil
}
