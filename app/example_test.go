// Package app_test holds the godoc examples for the app package. The
// examples need a live Wayland compositor to actually run, so they
// carry no Output comment: go test compiles them against the real API
// without executing.
package app_test

import (
	"errors"
	"fmt"
	"log"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/internal/sysfont"
	"github.com/stubbedev/gelm/internal/window"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/widget"
)

// ExampleRun is the whole minimal app: one toplevel window with a
// button that counts clicks, closed by the compositor's close request.
// It mirrors cmd/gelm-hello's structure - connect, pick a font, create
// the surface and window, complete the xdg configure handshake, then
// hand the tree to app.Run - so the README's quickstart compiles
// against the real API. It needs a live compositor, so go test only
// compile-checks it (no Output comment).
func ExampleRun() {
	sess := must(wlsession.Connect())
	defer sess.Close()
	tf := must(sysfont.Sans())
	surf := must(sess.Compositor().CreateSurface())
	win := must(window.New(sess.WmBase(), surf, window.Config{
		Title: "hello", AppID: "dev.stubbe.gelm.hello", Width: 320, Height: 120,
	}))
	die(surf.Commit())
	// The configure events can land after a sync callback completes,
	// so dispatch until the handshake finishes.
	for range 20 {
		if win.EnsureUsable() == nil {
			break
		}
		die(sess.Roundtrip())
	}

	clicks := 0
	count := widget.NewLabel(tf, 15, "clicked 0 times", widget.Current().Text)
	button := widget.NewButton(count, 10, 8)
	button.OnClick = func() {
		clicks++
		count.SetText(fmt.Sprintf("clicked %d times", clicks))
	}
	sess.OnWmBasePing = win.Pong

	err := app.Run(app.Config{Session: sess, Host: win, Root: button, Background: widget.Current().Bg})
	if err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

// must unwraps a (value, error) pair or dies; die checks an error.
// They keep the example down to its essentials.
func must[T any](v T, err error) T {
	if err != nil {
		log.Fatal(err)
	}
	return v
}

func die(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
