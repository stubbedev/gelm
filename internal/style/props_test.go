package style

import "testing"

// A shadow or border with its color written out reads nothing from the
// node, so it parses once at load; only an omitted color (currentColor)
// makes it node-dependent (#193).
func TestShadowAndBorderColorsKeepTheParseCache(t *testing.T) {
	for css, cached := range map[string]bool{
		`a { box-shadow: inset 0 0 0 1px #ff0000; }`: true,
		`a { box-shadow: 0 1px 2px; }`:               false,
		`a { border: 1px solid #ff0000; }`:           true,
		`a { border: 1px solid; }`:                   false,
		`a { outline: 2px solid #00ff00; }`:          true,
	} {
		d := Parse(css).rules[0].block.decls[0]
		if got := d.pre != nil; got != cached {
			t.Errorf("%s: parse cached %v, want %v", css, got, cached)
		}
	}
}
