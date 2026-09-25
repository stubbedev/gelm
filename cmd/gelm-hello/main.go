// Command gelm-hello is the M5 windowing demo: a real xdg_toplevel
// window with a counter button, drag-to-move on the background, and
// Escape to close — proving the window stack end to end.
package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/internal/popup"
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
	progress := widget.NewProgressBar(0)
	button := widget.NewButton(
		widget.NewBox(widget.Row, 8, 0).
			Append(widget.NewLabel(tf, "click me", 15, widget.Current().Text), false),
		10, 8)
	button.OnClick = func() {
		count++
		countLabel.SetText(fmt.Sprintf("clicked %d times", count))
		progress.SetValue(0)
		anim.Start(600*time.Millisecond, func(t float64) {
			progress.SetValue(t)
		})
	}
	root := widget.NewBox(widget.Column, 12, 16)
	root.Append(widget.NewLabel(tf, "gelm window", 18, widget.Current().Accent), false)
	root.Append(button, false)
	root.Append(progress, false)
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
	if err := win.Decorate(sess.DecorationManager()); err != nil {
		log.Printf("gelm-hello: server decorations unavailable: %v", err)
	}
	log.Printf("gelm-hello: mapped at %dx%d", w, h)

	sess.OnWmBasePing = win.Pong

	posX, posY := 0, 0
	onPress := func(btn, serial uint32, over widget.Widget) {
		switch {
		case btn == widget.BTNLeft && over != button:
			// A left press on the background starts an interactive
			// move, handled entirely by the compositor.
			_ = win.Toplevel.Move(sess.Seat(), serial)
		case btn == widget.BTNRight:
			items := []widget.MenuItem{
				{Label: "Say hello", OnClick: func() {
					count++
					countLabel.SetText(fmt.Sprintf("clicked %d times", count))
				}},
				{Label: "Light theme", OnClick: func() { widget.SetTheme(widget.LightTheme()) }},
				{Label: "Dark theme", OnClick: func() { widget.SetTheme(widget.DarkTheme()) }},
				{},
				{Label: "Close window", OnClick: win.Close},
			}
			menu := widget.NewMenu(tf, 13, items...)
			mSize := menu.Measure(widget.Constraints{Max: widget.Size{W: 200, H: 400}})
			p, err := popup.New(sess, popup.Config{
				Parent: win.XdgSurface,
				X:      posX, Y: posY,
				Width: mSize.W, Height: mSize.H,
				Serial: serial,
			})
			if err != nil {
				log.Printf("gelm-hello: popup: %v", err)
				return
			}
			menu.OnDismiss = p.Close
			_ = popup.Run(sess, p, 1, menu, widget.Current().Surface)
		}
	}

	if err := app.Run(app.Config{
		Session:       sess,
		Host:          win,
		Scale:         1,
		Root:          root,
		Background:    widget.Current().Bg,
		OnPress:       onPress,
		OnPointerMove: func(x, y float64) { posX, posY = int(x), int(y) },
		OnKey: func(_ *widget.Router, code uint32, _ wlsession.Mods) {
			if sess.KeySym(code) == xkb.KeyEscape {
				win.Close()
			}
		},
	}); err != nil && !errors.Is(err, app.ErrClosed) {
		return err
	}
	return nil
}
