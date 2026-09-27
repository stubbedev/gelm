package app

import (
	"errors"
	"fmt"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/internal/surfx"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// DialogButton is one button in a dialog's button row.
type DialogButton struct {
	Label    string
	Response string
}

// DialogConfig declares a dialog: content widget plus a button row.
// Esc responds with CancelResponse, Enter with DefaultResponse (either
// may be empty to disable that key).
type DialogConfig struct {
	Title           string
	Width, Height   uint32
	Content         widget.Widget
	Buttons         []DialogButton
	DefaultResponse string
	CancelResponse  string
	OnResponse      func(response string)
}

// Dialog is a modal child window with a response callback. While it is
// open every other window's input is blocked (xdg_shell has no modal
// bit, so the block is application-level). Respond fires OnResponse
// exactly once and closes the dialog.
type Dialog struct {
	app       *Application
	win       *Window
	cfg       DialogConfig
	responded bool
}

// NewDialog opens a dialog parented to a window of this application.
// A nil parent makes an application-level dialog that blocks every
// other window while open.
func (a *Application) NewDialog(parent *Window, cfg DialogConfig) (*Dialog, error) {
	if cfg.Content == nil {
		return nil, errors.New("app: dialog needs content")
	}
	if len(cfg.Buttons) == 0 {
		cfg.Buttons = []DialogButton{{Label: "OK", Response: "ok"}}
	}

	var d *Dialog
	respond := func(response string) {
		if d != nil {
			d.Respond(response)
		}
	}

	buttons := widget.NewBox(widget.Row, 8, 0)
	buttons.Append(widget.NewSpacer(0, 0), true)
	for _, b := range cfg.Buttons {
		resp := b.Response
		btn := widget.NewButton(
			widget.NewBox(widget.Row, 6, 0).
				Append(widget.NewLabel(a.tooltipFace, b.Label, 13, widget.Current().Text), false),
			10, 6)
		btn.OnClick = func() { respond(resp) }
		buttons.Append(btn, false)
	}

	root := widget.NewBox(widget.Column, 12, 12)
	root.Append(cfg.Content, true)
	root.Append(buttons, false)

	winCfg := WindowConfig{
		Title: cfg.Title, Width: cfg.Width, Height: cfg.Height,
		Root: root, Background: widget.Current().Bg,
		OnKey: func(_ *widget.Router, code uint32, _ wlsession.Mods) {
			if resp, ok := dialogResponseForKey(cfg, a.sess.KeySym(code)); ok {
				respond(resp)
			}
		},
		OnClosed: func() {
			for i, x := range a.dialogs {
				if x == d {
					a.dialogs = append(a.dialogs[:i], a.dialogs[i+1:]...)
					break
				}
			}
			if len(a.dialogs) == 0 {
				a.blockAll(false)
			}
		},
	}
	// Dialogs animate: KindDialog gives the child window a fade-in on
	// open and an exit tween on Respond, during which the modal block
	// stays on (the unblock happens in OnClosed, once the tween's
	// destroy has landed and the loop reaped the window).
	w, err := a.newWindowWindow(winCfg, surfx.KindDialog)
	if err != nil {
		return nil, fmt.Errorf("app: dialog window: %w", err)
	}
	// Dialog windows are exempt from their own blocking.
	for _, hw := range a.windows {
		if hw.win == w {
			hw.isDialog = true
			hw.blocked = false
		}
	}
	d = &Dialog{app: a, win: w, cfg: cfg}
	a.dialogs = append(a.dialogs, d)
	a.blockAll(true)
	if parent != nil && parent.win != nil {
		w.win.SetParent(parent.win)
	}
	return d, nil
}

// Respond fires OnResponse once and closes the dialog. Further calls
// are ignored, so a double event cannot produce two responses.
func (d *Dialog) Respond(response string) {
	if d.responded {
		return
	}
	d.responded = true
	if d.cfg.OnResponse != nil {
		d.cfg.OnResponse(response)
	}
	d.win.Close()
}

// Closed reports whether the dialog already produced a response or was
// closed.
func (d *Dialog) Closed() bool { return d.responded || d.win.Closed() }

// dialogResponseForKey maps Escape and Enter to the dialog's cancel
// and default responses; an empty response disables the key.
func dialogResponseForKey(cfg DialogConfig, sym xkb.Keysym) (string, bool) {
	switch sym {
	case xkb.KeyEscape:
		return cfg.CancelResponse, cfg.CancelResponse != ""
	case xkb.KeyReturn, xkb.KeyKPEnter:
		return cfg.DefaultResponse, cfg.DefaultResponse != ""
	}
	return "", false
}

// blockAll flips the input-blocked flag; dialog windows are exempt
// from their own blocking.
func (a *Application) blockAll(blocked bool) {
	dialogWins := make(map[*Window]bool, len(a.dialogs))
	for _, d := range a.dialogs {
		dialogWins[d.win] = true
	}
	for _, w := range a.windows {
		if w.win == nil || dialogWins[w.win] {
			continue
		}
		w.blocked = blocked
	}
}

// MessageBox opens the standard preset: a colored icon slot by kind, a
// text body, and a button row.
func (a *Application) MessageBox(parent *Window, kind MessageKind, title, text string, buttons []DialogButton) (*Dialog, error) {
	body := widget.NewBox(widget.Row, 16, 0)
	body.Append(newMessageGlyph(kind, 40), false)
	body.Append(widget.NewLabel(a.tooltipFace, text, 14, widget.Current().Text), true)
	cfg := DialogConfig{
		Title:           title,
		Width:           420,
		Height:          160,
		Content:         body,
		Buttons:         buttons,
		DefaultResponse: defaultResponse(buttons),
		CancelResponse:  cancelResponse(buttons),
	}
	return a.NewDialog(parent, cfg)
}

// MessageKind selects the icon mood of a MessageBox.
type MessageKind uint8

// Message kinds.
const (
	Info MessageKind = iota
	Warning
	Error
)

// messageColors maps each kind to its icon color.
var messageColors = map[MessageKind]render.Color{
	Info:    render.RGB(0x50, 0xa0, 0xe0),
	Warning: render.RGB(0xf0, 0xc0, 0x40),
	Error:   render.RGB(0xe0, 0x50, 0x50),
}

// newMessageGlyph paints a colored disc marking the message kind.
func newMessageGlyph(kind MessageKind, size int) widget.Widget {
	return &messageGlyph{size: size, color: messageColors[kind]}
}

// messageGlyph paints a filled disc in the message color.
type messageGlyph struct {
	size   int
	color  render.Color
	bounds render.Rect
}

func (g *messageGlyph) Measure(con widget.Constraints) widget.Size {
	w, h := g.size, g.size
	if con.Max.W < w {
		w = con.Max.W
	}
	if con.Max.H < h {
		h = con.Max.H
	}
	return widget.Size{W: w, H: h}
}

func (g *messageGlyph) Arrange(r render.Rect) { g.bounds = r }

func (g *messageGlyph) Paint(cv *render.Canvas) {
	cv.FillRect(g.bounds, g.color)
}

func (g *messageGlyph) HitTest(p widget.Point) widget.Widget {
	if g.bounds.Contains(p.X, p.Y) {
		return g
	}
	return nil
}

func (g *messageGlyph) Bounds() render.Rect { return g.bounds }

// defaultResponse is the response of the first button.
func defaultResponse(buttons []DialogButton) string {
	if len(buttons) > 0 {
		return buttons[0].Response
	}
	return ""
}

// cancelResponse is the response of the last button.
func cancelResponse(buttons []DialogButton) string {
	if len(buttons) > 0 {
		return buttons[len(buttons)-1].Response
	}
	return ""
}
