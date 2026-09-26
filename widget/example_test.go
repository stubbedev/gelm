// Package widget_test holds the godoc examples for the widget package.
// Entry, TextArea, and Menu are plain in-memory objects, so their
// examples really run under go test; the Output blocks pin the API's
// behavior, not just its shape.
package widget_test

import (
	"fmt"
	"log"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

func face() *render.Typeface {
	tf, err := render.LoadFont(goregular.TTF)
	if err != nil {
		log.Fatal(err)
	}
	return tf
}

func ExampleEntry() {
	// An entry is an editing state machine: callbacks see every
	// change, whatever produced it (typing, editing keys, clipboard,
	// SetText).
	name := widget.NewEntry(face(), 14, widget.Current().Text)
	name.SetPlaceholder("who goes there?")
	name.OnChanged = func(s string) { fmt.Println("changed:", s) }

	name.InsertRune('h')
	name.InsertRune('i')
	fmt.Println("text:", name.Text())
	// Output:
	// changed: h
	// changed: hi
	// text: hi
}

func ExampleTextArea() {
	// A text area holds logical lines and soft-wraps them at the
	// offered width by default (SetWrap(false) restores one row per
	// line). Editing stays in logical coordinates; Text reports them.
	notes := widget.NewTextArea(face(), 14, widget.Current().Text)
	notes.SetText("wrap me softly\nline two")

	fmt.Println(notes.Text())
	fmt.Println("--")

	// Place the caret on the second line, after "two", and type.
	notes.SetCursor(1, 8)
	notes.InsertRune('!')
	fmt.Println(notes.Text())
	// Output:
	// wrap me softly
	// line two
	// --
	// wrap me softly
	// line two!
}

func ExampleMenu() {
	// Menus are vertical action lists for popups: rows carry an
	// accelerator label (display-only), check and radio state, and
	// separators that keyboard motion skips.
	wrap := false
	menu := widget.NewMenu(face(), 13,
		widget.MenuItem{Label: "Cut", Accel: "Ctrl+X", OnClick: func() { fmt.Println("cut") }},
		widget.MenuItem{Label: "Copy", Accel: "Ctrl+C", OnClick: func() { fmt.Println("copy") }},
		widget.MenuSeparator(),
		widget.MenuItem{Label: "Word wrap", Kind: widget.ItemCheck, OnClick: func() { wrap = !wrap }},
	)

	// Keyboard: the first Down lands on the first selectable row, the
	// second walks past the separator, Enter activates. The host
	// closes the popup through OnDismiss; Esc maps to KeyDismiss.
	menu.KeyAction(widget.KeyDown, 0)
	menu.KeyAction(widget.KeyDown, 0)
	menu.KeyAction(widget.KeyDown, 0)
	menu.KeyAction(widget.KeyEnter, 0)
	fmt.Println("wrap:", wrap)
	// Output:
	// wrap: true
}
