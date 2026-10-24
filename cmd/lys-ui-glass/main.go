// Command lys-ui-glass shows the theme's glass surfaces over busy,
// moving content: a sidebar, a floating card and a bottom bar carry the
// .glass class over a striped backdrop whose bands keep sliding, so
// the backdrop blur re-samples every frame. t toggles light and dark,
// Escape closes.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/lys-ui/app"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
	"github.com/stubbedev/lys-ui/widget/css"
)

func main() {
	err := run()
	switch {
	case err == nil, errors.Is(err, app.ErrClosed):
	case errors.Is(err, app.ErrDisconnected):
		os.Exit(app.DisconnectExitCode)
	default:
		log.Fatal(err)
	}
}

// stripes is the busy backdrop: diagonal bands whose phase the loop
// advances, so the glass above has something moving to blur.
const stripes = `.stripes { background-image: repeating-linear-gradient(%ddeg, #e05561 0px, #e05561 18px, #3fb68b 18px, #3fb68b 36px, #4f7cff 36px, #4f7cff 54px, #f0c040 54px, #f0c040 72px); }`

func run() error {
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	face, err := app.UIFont(14)
	if err != nil {
		return err
	}
	application := app.NewApplication(sess)

	backdrop := widget.NewBox(widget.Column, 0, 0)
	backdrop.AddClass("stripes")
	widget.LoadStylesheet(fmt.Sprintf(stripes, 45))

	panel := func(min string, rows ...string) *widget.Box {
		b := widget.NewBox(widget.Column, 6, 16)
		b.AddClass(css.Glass)
		for i, text := range rows {
			size := 13.0
			if i == 0 {
				size = 16
			}
			b.Append(widget.NewLabel(face, size, text, 0), false)
		}
		b.SetInlineStyle(min)
		return b
	}
	sidebar := panel("min-width: 180px;", "Library", "Recent", "Favorites", "Shared", "Trash")
	card := panel("min-width: 260px;", "Clear glass", "The backdrop blurs and saturates", "under a translucent card.")
	bar := panel("min-height: 40px;", "Playing - the bands keep moving underneath")

	stage := widget.NewOverlay().
		Append(backdrop).
		AppendAligned(sidebar, widget.AlignStart, widget.AlignFill).
		AppendAligned(card, widget.AlignCenter, widget.AlignCenter).
		AppendAligned(bar, widget.AlignFill, widget.AlignEnd)

	phase := 45
	application.Every(33*time.Millisecond, func() {
		phase = (phase + 1) % 360
		widget.LoadStylesheet(fmt.Sprintf(stripes, phase))
	})

	_, err = application.NewWindow(app.WindowConfig{
		Title:      "lys-ui glass",
		AppID:      "dev.stubbe.lys.ui.glass",
		Width:      820,
		Height:     520,
		Root:       stage,
		Background: render.RGB(0, 0, 0),
		OnKey: func(_ *widget.Router, code uint32, _ app.Mods) {
			switch application.KeySym(code) {
			case xkb.KeyEscape:
				application.Quit()
			case xkb.Keysym('t'):
				if widget.Current().IsDark() {
					widget.SetTheme(widget.LightTheme())
				} else {
					widget.SetTheme(widget.DarkTheme())
				}
			}
		},
	})
	if err != nil {
		return err
	}
	return application.Run()
}
