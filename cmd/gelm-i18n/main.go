// Command gelm-i18n proves the message catalog (#89): the same app,
// two languages. Every built-in string - dialog buttons, the file
// picker's places and statuses - flows through widget.Tr, so the
// Norwegian catalog below localizes gelm's chrome without touching
// app code. Toggle the language with the button and open the dialogs:
// Escape quits.
package main

import (
	"log"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/component"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/ui"
	"github.com/stubbedev/gelm/widget"
)

// norsk covers the built-in strings this demo surfaces. An app's own
// strings ride the same catalog - one lookup for chrome and content.
var norsk = map[string]string{
	"OK":       "OK",
	"Cancel":   "Avbryt",
	"Close":    "Lukk",
	"Open":     "Åpne",
	"Save":     "Lagre",
	"Select":   "Velg",
	"Up":       "Opp",
	"Home":     "Hjem",
	"Recent":   "Nylig",
	"Name:":    "Navn:",
	"Filter:":  "Filter:",
	"question": "Norsk eller engelsk?",
	"greeting": "Hello, gelm",
}

type toggleLanguage struct{}

type greeter struct {
	app       *app.Application
	env       ui.Env
	norwegian bool
}

func (g *greeter) Init(cx *component.Context[toggleLanguage, struct{}]) widget.Widget {
	onAccent := widget.Current().OnAccent
	return ui.Mount(cx, g.env, ui.Column(
		ui.Label("").Font(nil, 16).WatchText(func() string { return widget.Tr("greeting") }),
		ui.Button(ui.Label("norsk / english").Ink(onAccent), 12, 6).
			OnClick(func() { cx.Input(toggleLanguage{}) }),
		ui.Button(ui.Label(widget.Tr("question")).Ink(onAccent), 12, 6).OnClick(g.ask),
		ui.Label("the buttons and dialogs follow the catalog; Escape quits").
			Font(nil, 12).Ink(widget.Current().TextMuted),
	).Spacing(12).Padding(render.UniformInsets(24)))
}

func (g *greeter) Update(*component.Context[toggleLanguage, struct{}], toggleLanguage) {
	g.norwegian = !g.norwegian
	if g.norwegian {
		widget.SetMessageCatalog(func(s string) string { return norsk[s] })
	} else {
		widget.SetMessageCatalog(nil)
	}
}

func (g *greeter) ask() {
	_, _ = g.app.MessageBox(nil, app.Question, widget.Tr("question"), widget.Tr("greeting"),
		[]app.DialogButton{
			{Label: widget.Tr("Cancel"), Response: "cancel", Role: app.ButtonRoleCancel},
			{Label: widget.Tr("OK"), Response: "ok", Role: app.ButtonRoleDefault},
		})
}

func main() {
	err := component.Run(app.WindowConfig{
		Title: "gelm i18n", AppID: "dev.stubbe.gelm.i18n",
		Width: 380, Height: 220, Background: widget.Current().Bg,
	}, func(a *app.Application) (*greeter, error) {
		face, err := app.Font("sans-serif", 14)
		if err != nil {
			return nil, err
		}
		if err := a.AddAccel("Escape", widget.NewAction("quit", a.Quit)); err != nil {
			return nil, err
		}
		return &greeter{app: a, env: ui.Env{Face: face, Size: 14}}, nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
