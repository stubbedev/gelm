// Command gelm-messages demonstrates the typed messaging layer
// (app/message.go): one SharedState counter shared across two windows -
// either window's buttons move both labels - and one Stream broker that
// any depth of code can reach without threading senders: both windows
// subscribe, a button publishes, every window toasts. The click handler
// itself is a Component: clicks cross onto the loop as typed messages.
package main

import (
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/internal/sysfont"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/widget"
)

// clickMsg is the Component message: which window was clicked.
type clickMsg struct{ window string }

// announceMsg is the broker message: a toast line for every window.
type announceMsg struct{ text string }

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
	theme := widget.Current()

	// The broker: a handle any nesting depth can hold, no senders
	// threaded through. Both windows subscribe; publishing is one Send.
	broker := app.NewStream[announceMsg](application)
	broker.Subscribe(func(m announceMsg) {
		application.ShowToast(m.text, 3*time.Second, nil)
	})

	// The shared state: one counter, every window's label subscribes, so
	// no per-window copy can fall out of sync.
	counter := app.NewSharedState(application, 0)

	// The component: clicks become typed messages and update on the
	// loop goroutine.
	lastClick := widget.NewLabel(tf, 12, "last click: --", theme.TextMuted)
	clicks := app.NewComponent(application, func(m clickMsg) {
		counter.Update(func(n *int) { *n++ })
		lastClick.SetText("last click: " + m.window)
	})

	window := func(title string, pingText string) *widget.Box {
		count := widget.NewLabel(tf, 28, "0", theme.Text)
		counter.Subscribe(func(n int) {
			count.SetText(strconv.Itoa(n))
		})
		plus := widget.NewButton(widget.NewLabel(tf, 14, "increment", theme.OnAccent), 10, 6)
		plus.OnClick = func() { clicks.Send(clickMsg{window: title}) }
		ping := widget.NewButton(widget.NewLabel(tf, 14, "ping both", theme.OnAccent), 10, 6)
		ping.OnClick = func() { broker.Send(announceMsg{text: pingText}) }
		hint := widget.NewLabel(tf, 12, "either window's increment moves both counters; Escape quits", theme.TextMuted)
		box := widget.NewBox(widget.Column, 10, 24)
		box.Append(count, false)
		box.Append(plus, false)
		box.Append(ping, false)
		box.Append(hint, false)
		return box
	}

	application.OnKey(func(_ *widget.Router, code uint32, mods wlsession.Mods) {
		if mods&wlsession.ModAlt == 0 && sess.KeySym(code) == xkb.KeyEscape {
			application.Quit()
		}
	})

	rootA := window("gelm messages a", "ping from the left window")
	rootA.Append(lastClick, false)
	rootB := window("gelm messages b", "ping from the right window")
	for _, cfg := range []app.WindowConfig{
		{Title: "gelm messages a", AppID: "dev.stubbe.gelm.messages", Width: 380, Height: 300, Root: rootA, Background: theme.Bg},
		{Title: "gelm messages b", AppID: "dev.stubbe.gelm.messages", Width: 380, Height: 260, Root: rootB, Background: theme.Bg},
	} {
		if _, err := application.NewWindow(cfg); err != nil {
			return err
		}
	}
	return application.Run()
}
