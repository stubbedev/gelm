// Echo masking for Entry: password fields render one dot per rune
// while the contents, caret, click mapping, and selection stay logical.
// Because masking is one-to-one per rune, every display-space index is
// also the logical rune index, so nothing about the editing model
// changes — only the pixels do.
package widget

import "strings"

// Echo selects how an Entry shows its contents.
type Echo uint8

const (
	// EchoNormal shows the runes as typed.
	EchoNormal Echo = iota
	// EchoPassword shows one dot per rune; the display reveals nothing
	// about the contents, not even a selection's extent.
	EchoPassword
	// EchoNone shows nothing at all.
	EchoNone
)

// passwordDot is the glyph shown per rune in EchoPassword mode.
const passwordDot = "•"

// SetEcho sets the display mode. Contents and caret/selection math
// stay logical whatever the mode.
func (e *Entry) SetEcho(mode Echo) {
	if e.echo == mode {
		return
	}
	e.echo = mode
	e.InvalidateLayout()
}

// Echo returns the display mode.
func (e *Entry) Echo() Echo { return e.echo }

// EchoReveal shows the real contents regardless of the echo mode while
// on. The reveal toggle is the app's job — a peek button, a
// hold-to-reveal key — and the entry only renders what it is told.
func (e *Entry) EchoReveal(on bool) {
	if e.reveal == on {
		return
	}
	e.reveal = on
	e.InvalidateLayout()
}

// Revealing reports whether the reveal override is on.
func (e *Entry) Revealing() bool { return e.reveal }

// displayText is the string on show: the display runes (contents with
// any composing text spliced in) with echo masking applied. Masked
// modes keep the rune count — one dot per rune, or none — so caret,
// click, and selection math operate on the same indices as ever. The
// unmasked, not-composing read — every steady frame — is the string
// setRunes keeps fresh, not a fresh conversion.
func (e *Entry) displayText() string {
	rs := e.displayRunes()
	switch {
	case e.echo == EchoNormal || e.reveal:
		if !e.composing() {
			return e.disp
		}
		return string(rs)
	case e.echo == EchoNone:
		return ""
	default:
		return strings.Repeat(passwordDot, len(rs))
	}
}
