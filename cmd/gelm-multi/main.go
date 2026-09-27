// Command gelm-multi exercises the application model: one process
// running a declarative layer-shell bar on every output plus toplevel
// windows created on demand, with a close-request veto. Click the
// bar's "+ window" button to spawn a window; a window's first close
// request is vetoed - only the second one closes it. Escape quits the
// whole application.
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/internal/layersurface"
	"github.com/stubbedev/gelm/internal/sysfont"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

func run() error {
	sess, err := wlsession.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	tf, err := sysfont.Sans()
	if err != nil {
		return err
	}

	application := app.NewApplication(sess)
	application.SetTooltipFace(tf)
	application.OnKey(func(_ *widget.Router, code uint32, mods wlsession.Mods) {
		if mods&wlsession.ModAlt == 0 && sess.KeySym(code) == xkb.KeyEscape {
			log.Printf("gelm-multi: escape quits the application")
			application.Quit()
		}
	})

	spawn := func() {
		w, err := newVetoWindow(application, tf)
		if err != nil {
			log.Printf("gelm-multi: window: %v", err)
			return
		}
		_ = w
	}

	barFor := func(out *wlsession.Output) {
		if _, err := application.NewLayer(newBarConfig(tf, out, spawn)); err != nil {
			log.Printf("gelm-multi: bar: %v", err)
		}
	}
	for _, out := range sess.Outputs() {
		barFor(out)
	}
	// Hotplug: a bar spawns on every output that appears later.
	sess.OnOutputAdded = barFor

	spawn()
	return application.Run()
}

// newBarConfig declares a 28px top bar with an exclusive zone: the
// compositor keeps regular windows out of the strip it covers.
func newBarConfig(tf *render.Typeface, out *wlsession.Output, spawn func()) app.LayerConfig {
	label := widget.NewLabel(tf, 12, "gelm-multi bar", widget.Current().Text)
	plus := widget.NewButton(
		widget.NewBox(widget.Row, 6, 0).
			Append(widget.NewLabel(tf, 12, "+ window", widget.Current().Text), false),
		8, 4)
	plus.OnClick = spawn

	row := widget.NewBox(widget.Row, 12, 4)
	row.Append(label, false)
	row.Append(plus, false)

	return app.LayerConfig{
		Output:        out,
		Layer:         layersurface.LayerTop,
		Anchor:        layersurface.AnchorTop | layersurface.AnchorLeft | layersurface.AnchorRight,
		Height:        28,
		ExclusiveZone: 28,
		Namespace:     "gelm-multi-bar",
		Root:          row,
		Background:    widget.Current().Surface,
	}
}

// newVetoWindow opens a toplevel whose first close request is vetoed;
// the status label says so and the second request closes for real.
func newVetoWindow(application *app.Application, tf *render.Typeface) (*app.Window, error) {
	status := widget.NewLabel(tf, 13, "close requests: 0", widget.Current().Text)
	hint := widget.NewLabel(tf, 12, "the first close is vetoed; close again to win", widget.Current().TextMuted)
	root := widget.NewBox(widget.Column, 12, 12)
	root.Append(hint, false)
	root.Append(status, false)

	w, err := application.NewWindow(app.WindowConfig{
		Title:      "gelm-multi window",
		AppID:      "dev.stubbe.gelm.multi",
		Width:      380,
		Height:     180,
		Root:       root,
		Background: widget.Current().Bg,
		OnClosed:   func() { log.Printf("gelm-multi: window closed") },
	})
	if err != nil {
		return nil, err
	}
	count := 0
	w.SetCloseRequest(func() bool {
		count++
		status.SetText(fmt.Sprintf("close requests: %d", count))
		if count < 2 {
			log.Printf("gelm-multi: close request vetoed")
			return false
		}
		return true
	})
	return w, nil
}
