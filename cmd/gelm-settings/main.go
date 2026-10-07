// Command gelm-settings is the settings application example: typed
// persisted keys (app/settings.go) bound straight to widgets through
// Binding connectors, so every edit writes through to
// <config>/dev.stubbe.gelm.settings/settings.json and every restart
// comes back exactly where it left off. Change something, quit, run it
// again.
package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/internal/sysfont"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/widget"
)

var (
	nameKey   = app.Key[string]{Name: "name", Default: "someone"}
	volumeKey = app.Key[float64]{Name: "volume", Default: 0.5}
	notifyKey = app.Key[bool]{Name: "notifications", Default: true}
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
	settings := app.NewSettings(application, "dev.stubbe.gelm.settings")
	theme := widget.Current()

	name := widget.NewEntry(tf, 14, theme.Text)
	defer name.BindText(nameKey.Binding(settings))()
	volume := widget.NewSlider(0, 100, 1, 0)
	defer volume.BindValue(scaleBinding(settings))()
	notify := widget.NewSwitch(false)
	defer notify.BindOn(notifyKey.Binding(settings))()

	path := "settings.json"
	if dir, err := os.UserConfigDir(); err == nil {
		path = filepath.Join(dir, "dev.stubbe.gelm.settings", "settings.json")
	}
	hint := widget.NewLabel(tf, 12, "edits persist to "+path+"; restart to see them; Escape quits", theme.TextMuted)

	row := func(label string, control widget.Widget) *widget.Box {
		box := widget.NewBox(widget.Row, 12, 0)
		box.Append(widget.NewLabel(tf, 14, label, theme.Text), false)
		box.Append(control, false)
		return box
	}
	root := widget.NewBox(widget.Column, 14, 24)
	root.Append(row("name", name), false)
	root.Append(row("volume", volume), false)
	root.Append(row("notifications", notify), false)
	root.Append(hint, false)

	application.OnKey(func(_ *widget.Router, code uint32, mods wlsession.Mods) {
		if mods&wlsession.ModAlt == 0 && sess.KeySym(code) == xkb.KeyEscape {
			application.Quit()
		}
	})

	if _, err := application.NewWindow(app.WindowConfig{
		Title: "gelm settings",
		AppID: "dev.stubbe.gelm.settings",
		Width: 420, Height: 220,
		Root:       root,
		Background: theme.Bg,
	}); err != nil {
		return err
	}
	return application.Run()
}

// scaleBinding adapts the 0..1 persisted volume to the slider's
// percentage scale: a Binding over the derived value, written back
// through the key.
func scaleBinding(s *app.Settings) *widget.Binding[float64] {
	vol := volumeKey.Get(s)
	b := widget.NewBinding(vol * 100)
	b.Subscribe(func(pct float64) { volumeKey.Set(s, pct/100) })
	volumeKey.Subscribe(s, func(v float64) { b.Set(v * 100) })
	return b
}
