// Command gelm-settings is the settings application example: typed
// persisted keys (app/settings.go) bound straight to widgets through
// Binding connectors, so every edit writes through to
// <config>/dev.stubbe.gelm.settings/settings.json and every restart
// comes back exactly where it left off. Change something, quit, run it
// again.
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/component"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/ui"
	"github.com/stubbedev/gelm/widget"
)

var (
	nameKey   = app.Key[string]{Name: "name", Default: "someone"}
	volumeKey = app.Key[float64]{Name: "volume", Default: 0.5}
	notifyKey = app.Key[bool]{Name: "notifications", Default: true}
)

type prefs struct {
	env      ui.Env
	settings *app.Settings
}

func (p *prefs) Init(cx *component.Context[struct{}, struct{}]) widget.Widget {
	path := "settings.json"
	if dir, err := os.UserConfigDir(); err == nil {
		path = filepath.Join(dir, "dev.stubbe.gelm.settings", "settings.json")
	}
	row := func(label string, control ui.Node) ui.Node {
		return ui.Row(ui.Label(label), control).Spacing(12)
	}
	return ui.Mount(cx, p.env, ui.Column(
		row("name", ui.Entry().BindText(nameKey.Binding(p.settings))),
		row("volume", ui.Slider(0, 100, 1, 0).BindValue(scaleBinding(p.settings))),
		row("notifications", ui.Switch(false).BindOn(notifyKey.Binding(p.settings))),
		ui.Label("edits persist to "+path+"; restart to see them; Escape quits").
			Font(nil, 12).Ink(widget.Current().TextMuted),
	).Spacing(14).Padding(render.UniformInsets(24)))
}

func (p *prefs) Update(*component.Context[struct{}, struct{}], struct{}) {}

func main() {
	err := component.Run(app.WindowConfig{
		Title: "gelm settings", AppID: "dev.stubbe.gelm.settings",
		Width: 420, Height: 220, Background: widget.Current().Bg,
	}, func(a *app.Application) (*prefs, error) {
		face, err := app.Font("sans-serif", 14)
		if err != nil {
			return nil, err
		}
		if err := a.AddAccel("Escape", widget.NewAction("quit", a.Quit)); err != nil {
			return nil, err
		}
		return &prefs{env: ui.Env{Face: face, Size: 14}, settings: app.NewSettings(a, "dev.stubbe.gelm.settings")}, nil
	})
	if err != nil {
		log.Fatal(err)
	}
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
