// Command gelm-i18n proves the message catalog (#89): the same app,
// two languages. Every built-in string - dialog buttons, the file
// picker's places and statuses - flows through widget.Tr, so the
// Norwegian catalog below localizes gelm's chrome without touching
// app code. Toggle the language with the button and open the dialogs:
// Escape quits.
package main

import (
	"errors"
	"log"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/internal/sysfont"
	"github.com/stubbedev/gelm/internal/wlsession"
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
	greeting := widget.NewLabel(tf, 16, widget.Tr("greeting"), theme.Text)
	hint := widget.NewLabel(tf, 12, "the buttons and dialogs follow the catalog; Escape quits", theme.TextMuted)

	norwegian := false
	toggle := widget.NewButton(widget.NewLabel(tf, 14, "norsk / english", theme.OnAccent), 12, 6)
	toggle.OnClick = func() {
		norwegian = !norwegian
		if norwegian {
			widget.SetMessageCatalog(func(s string) string { return norsk[s] })
		} else {
			widget.SetMessageCatalog(nil)
		}
		// The greeting rebuilds through Tr; the dialog chrome (opened
		// next) is the proof for the toolkit's own strings.
		greeting.SetText(widget.Tr("greeting"))
	}
	ask := widget.NewButton(widget.NewLabel(tf, 14, widget.Tr("question"), theme.OnAccent), 12, 6)
	ask.OnClick = func() {
		_, _ = application.MessageBox(nil, app.Question,
			widget.Tr("question"), widget.Tr("greeting"),
			[]app.DialogButton{
				{Label: widget.Tr("Cancel"), Response: "cancel", Role: app.ButtonRoleCancel},
				{Label: widget.Tr("OK"), Response: "ok", Role: app.ButtonRoleDefault},
			})
	}

	root := widget.NewBox(widget.Column, 12, 24)
	root.Append(greeting, false)
	root.Append(toggle, false)
	root.Append(ask, false)
	root.Append(hint, false)

	application.OnKey(func(_ *widget.Router, code uint32, mods wlsession.Mods) {
		if mods&wlsession.ModAlt == 0 && sess.KeySym(code) == xkb.KeyEscape {
			application.Quit()
		}
	})
	if _, err := application.NewWindow(app.WindowConfig{
		Title: "gelm i18n",
		AppID: "dev.stubbe.gelm.i18n",
		Width: 380, Height: 220,
		Root:       root,
		Background: theme.Bg,
	}); err != nil {
		return err
	}
	return application.Run()
}
