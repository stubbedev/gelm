// Command gelm-states is the window-state client the headless state
// suite (internal/headlesstest) drives: one toplevel whose keyboard
// chords issue the xdg_toplevel state requests - m maximize, n
// unmaximize, f fullscreen, g unfullscreen, i minimize, p poll the
// reported state, Escape close - while a status label mirrors only
// what the compositor CONFIRMS. Requests and confirmed states are
// traced separately, so the harness can tell a refusal (request
// traced, no confirming configure) from a confirmation.
package main

import (
	"errors"
	"log"
	"os"
	"time"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/sysfont"
	"github.com/stubbedev/gelm/internal/window"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/widget"
)

func main() {
	err := run()
	switch {
	case err == nil, errors.Is(err, app.ErrClosed):
		// Normal close: the window ended the loop.
	case errors.Is(err, app.ErrDisconnected):
		// The compositor went away; exit with the supervisor code.
		os.Exit(app.DisconnectExitCode)
	default:
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
	// GELM_DEMO_RECONNECT rebuilds the window on a restarted compositor
	// instead of exiting: the headless reconnect test's probe.
	if os.Getenv("GELM_DEMO_RECONNECT") != "" {
		application.SetReconnect(&app.ReconnectOptions{
			OnReconnected: func() { debug.Log("demo", "reconnected") },
		})
	}

	hint := widget.NewLabel(tf, 11, "m maximize  n unmaximize  f fullscreen  g unfullscreen  i minimize  esc close", widget.Current().TextMuted)
	status := widget.NewLabel(tf, 15, "pending configure", widget.Current().Text)
	root := widget.NewBox(widget.Column, 12, 12)
	root.Append(hint, false)
	root.Append(status, false)

	var w *app.Window
	w, err = application.NewWindow(app.WindowConfig{
		Title:      "gelm states",
		AppID:      "dev.stubbe.gelm.states",
		Width:      420,
		Height:     280,
		Root:       root,
		Background: widget.Current().Bg,
		OnClosed:   func() { debug.Log("demo", "closed") },
		OnKey: func(_ *widget.Router, code uint32, mods wlsession.Mods) {
			if mods&wlsession.ModAlt != 0 {
				return
			}
			// Every chord traces the request it sends, so the harness can
			// tell a refused request (no confirming configure follows) from
			// one it never issued.
			request := func(name string, fn func()) {
				debug.Log("demo", "requested %s", name)
				fn()
			}
			switch application.KeySym(code) {
			case xkb.Keysym('m'):
				request("maximize", w.Maximize)
			case xkb.Keysym('n'):
				request("unmaximize", w.Unmaximize)
			case xkb.Keysym('f'):
				request("fullscreen", w.Fullscreen)
			case xkb.Keysym('g'):
				request("unfullscreen", w.Unfullscreen)
			case xkb.Keysym('i'):
				request("minimize", w.SetMinimized)
			case xkb.Keysym('p'):
				// Read the reported state on demand: the poller only traces
				// changes, so an unchanged state is silent until asked.
				st := w.State()
				sw, sh := w.Size()
				debug.Log("demo", "polled maximized=%v fullscreen=%v %dx%d",
					st.Maximized, st.Fullscreen, sw, sh)
			case xkb.KeyEscape:
				w.Close()
			}
		},
	})
	if err != nil {
		return err
	}

	// Mirror the compositor-confirmed truth: the label (and the trace
	// the harness asserts on) follow Window.State - what the last
	// configure confirmed, never what a chord asked for. Traces fire on
	// state or size change, so a transition produces one line.
	var last window.State
	lastW, lastH := -1, -1
	application.Every(50*time.Millisecond, func() {
		if w.EnsureUsable() != nil {
			return // nothing is confirmed before the first configure
		}
		st := w.State()
		sw, sh := w.Size()
		if st == last && sw == lastW && sh == lastH {
			return
		}
		last, lastW, lastH = st, sw, sh
		status.SetText(st.String())
		// Flag-form (not State's joined string) so the harness can match
		// one state bit regardless of the tiled_* and activated bits the
		// compositor carries alongside it.
		debug.Log("demo", "state maximized=%v fullscreen=%v %dx%d",
			st.Maximized, st.Fullscreen, sw, sh)
	})

	return application.Run()
}
