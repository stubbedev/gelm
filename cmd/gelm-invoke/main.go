// Command gelm-invoke demonstrates the threading model: a plain
// goroutine "polls a sensor" twice a second and pushes the reading
// into a label through app.Invoke, while app.Every drives a second
// label on the loop's own timer wakes. Between ticks the loop parks -
// run it with GOELM_DEBUG=wake (gelmdebug build tag) and watch it go
// quiet: two wakeups per second, nothing in between, 0% idle CPU.
package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/internal/sysfont"
	"github.com/stubbedev/gelm/internal/wlsession"
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

	theme := widget.Current()
	gauge := widget.NewLabel(tf, 18, "sensor: -- mV", theme.Text)
	cadence := widget.NewLabel(tf, 14, "every: --", theme.Text)
	hint := widget.NewLabel(tf, 12, "a goroutine updates the top label via app.Invoke; Escape quits", theme.TextMuted)
	root := widget.NewBox(widget.Column, 10, 16)
	root.Append(gauge, false)
	root.Append(cadence, false)
	root.Append(hint, false)

	// The poller: an ordinary goroutine. It never touches widgets -
	// updates cross onto the loop goroutine through Invoke, which
	// queues the closure and kicks the parked loop with one coalesced
	// wake. The goroutine outlives Run: after the loop exits its
	// Invokes are dropped, so it just spins down with the process.
	go func() {
		for n := 0; ; n++ {
			time.Sleep(500 * time.Millisecond)
			reading := fmt.Sprintf("sensor: %d mV", 1800+n%400)
			application.Invoke(func() { gauge.SetText(reading) })
		}
	}()

	// Periodic work rides the loop's timer wakes instead of its own
	// ticker goroutine: the park wakes exactly at the deadline.
	every := 0
	stopEvery := application.Every(time.Second, func() {
		every++
		cadence.SetText(fmt.Sprintf("every: %d s on the loop goroutine", every))
	})
	defer stopEvery()

	application.OnKey(func(_ *widget.Router, code uint32, mods wlsession.Mods) {
		if mods&wlsession.ModAlt == 0 && sess.KeySym(code) == xkb.KeyEscape {
			application.Quit()
		}
	})

	if _, err := application.NewWindow(app.WindowConfig{
		Title: "gelm invoke",
		AppID: "dev.stubbe.gelm.invoke",
		Width: 460, Height: 150,
		Root:       root,
		Background: theme.Bg,
	}); err != nil {
		return err
	}
	return application.Run()
}
