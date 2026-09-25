// Command gelm-hello is the M5 windowing demo: a real xdg_toplevel
// window with a counter button, drag-to-move on the background, and
// Escape to close — proving the window stack end to end.
package main

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

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	sess, err := wlsession.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	if sess.WmBase() == nil {
		return errors.New("gelm-hello: compositor has no xdg_wm_base; windows unsupported")
	}
	tf, err := sysfont.Sans()
	if err != nil {
		return err
	}

	surf, err := sess.Compositor().CreateSurface()
	if err != nil {
		return fmt.Errorf("gelm-hello: create surface: %w", err)
	}
	win, err := window.New(sess.WmBase(), surf, window.Config{
		Title:  "gelm hello",
		AppID:  "dev.stubbe.gelm.hello",
		Width:  360,
		Height: 240,
	})
	if err != nil {
		return err
	}

	// The widget tree: centered column with a counter button.
	count := 0
	countLabel := widget.NewLabel(tf, "clicked 0 times", 15, widget.Current().Text)
	button := widget.NewButton(
		widget.NewBox(widget.Row, 8, 0).
			Append(widget.NewLabel(tf, "click me", 15, widget.Current().Text), false),
		10, 8)
	button.OnClick = func() {
		count++
		countLabel.SetText(fmt.Sprintf("clicked %d times", count))
	}
	root := widget.NewBox(widget.Column, 12, 16)
	root.Append(widget.NewLabel(tf, "gelm window", 18, widget.Current().Accent), false)
	root.Append(button, false)
	root.Append(countLabel, false)
	root.Append(widget.NewLabel(tf, "drag to move, esc to close", 11, widget.Current().TextMuted), false)

	if err := surf.Commit(); err != nil {
		return fmt.Errorf("gelm-hello: initial commit: %w", err)
	}
	// The configure events can land after a sync callback completes, so
	// dispatch until the handshake finishes.
	for range 20 {
		if win.EnsureUsable() == nil {
			break
		}
		if err := sess.Roundtrip(); err != nil {
			return fmt.Errorf("gelm-hello: configure roundtrip: %w", err)
		}
	}
	if err := win.EnsureUsable(); err != nil {
		return fmt.Errorf("gelm-hello: %w", err)
	}
	w, h := win.Size()
	log.Printf("gelm-hello: mapped at %dx%d", w, h)

	sess.OnWmBasePing = win.Pong
	sess.OnKey = func(keycode uint32, _ bool) {
		if keycode == 1 { // KEY_ESC
			win.Close()
		}
	}

	onPress := func(serial uint32, over widget.Widget) {
		// A press that is not on the counter button starts an
		// interactive move, handled entirely by the compositor.
		if over != button {
			_ = win.Toplevel.Move(sess.Seat(), serial)
		}
	}

	if err := app.Run(app.Config{
		Session:    sess,
		Host:       win,
		Scale:      1,
		Root:       root,
		Background: widget.Current().Bg,
		OnPress:    onPress,
	}); err != nil && !errors.Is(err, app.ErrClosed) {
		return err
	}
	return nil
}
