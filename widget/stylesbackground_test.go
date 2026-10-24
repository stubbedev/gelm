package widget

import "testing"

// A host's default plate yields only to a declared background:
// transparent counts, an unstyled widget does not.
func TestStylesBackgroundIsTheDeclaration(t *testing.T) {
	loadCSS(t, `.clear { background: transparent; } .tinted { background-color: #123456; } .ringed { border: 2px solid red; }`)
	for class, want := range map[string]bool{"clear": true, "tinted": true, "ringed": true, "plain": false} {
		b := NewBox(Column, 0, 0)
		b.AddClass(class)
		arrangeTree(t, b, 10, 10)
		if got := StylesBackground(b); got != want {
			t.Errorf(".%s: StylesBackground = %v, want %v", class, got, want)
		}
	}
}
