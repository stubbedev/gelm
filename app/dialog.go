package app

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/surfx"
	"github.com/stubbedev/gelm/internal/sysfont"
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
// may be empty to disable that key). Width and Height are the
// dialog's initial size in logical pixels, like every size in the
// toolkit; zero lets the compositor pick.
type DialogConfig struct {
	Title           string
	Width, Height   uint32
	Content         widget.Widget
	Buttons         []DialogButton
	DefaultResponse string
	CancelResponse  string
	OnResponse      func(response string)
	// Bare makes Content the whole window: no button row, no card,
	// Background as the window's clear color. The content carries its
	// own buttons (styled by the application's stylesheet) and answers
	// through Dialog.Respond; Esc and Enter still map to the cancel and
	// default responses.
	Bare       bool
	Background render.Color
}

// Dialog is a modal child window with a response callback. While it
// is open every other window's input is blocked at the application
// level, and — when the compositor offers xdg-dialog-v1 and the dialog
// has a parent — at the window level too: the compositor blocks input
// to the parent itself. Respond fires OnResponse exactly once and
// closes the dialog.
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
	var d *Dialog
	respond := func(response string) {
		if d != nil {
			d.Respond(response)
		}
	}
	root, background, err := a.dialogRoot(cfg, respond)
	if err != nil {
		return nil, err
	}

	winCfg := WindowConfig{
		Title: cfg.Title, Width: cfg.Width, Height: cfg.Height,
		Root: root, Background: background,
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
		// Window-level modality where the compositor offers it: the hint
		// rides the dialog's first commit, and the compositor blocks input
		// to the parent itself. Without the protocol this is a silent
		// no-op and the application-level block above stays the whole
		// story — the floor every compositor gets.
		if err := w.win.SetModal(a.sess.DialogManager(), true); err != nil {
			debug.Log("shell", "dialog modality hint: %v", err)
		}
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

// customPalette is the process session's color picks: every color a
// ColorChooserDialog confirms lands here (deduped, most recent first,
// capped), and every later dialog offers them back as swatches.
var customPalette []render.Color

// paletteCap bounds the session palette.
const paletteCap = 16

// rememberPick records a confirmed color in the session palette.
func rememberPick(col render.Color) {
	for i, c := range customPalette {
		if c == col {
			customPalette = append(customPalette[:i], customPalette[i+1:]...)
			break
		}
	}
	customPalette = append([]render.Color{col}, customPalette...)
	if len(customPalette) > paletteCap {
		customPalette = customPalette[:paletteCap]
	}
}

// ColorChooserDialog opens a modal color picker (#72): an SV square,
// hue and alpha strips, a hex entry, theme-derived presets, and the
// session's custom palette. Confirming responds "ok" and fires
// onColor with the exact picked color (and the pick joins the session
// palette); Esc responds "cancel".
func (a *Application) ColorChooserDialog(parent *Window, initial render.Color, onColor func(render.Color)) (*Dialog, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, errors.New("app: color chooser text face unavailable: no configured tooltip face and the system has no sans font")
	}
	chooser := widget.NewColorChooser(face, 13, initial)
	chooser.SetPaletteSource(func() []render.Color { return customPalette }, rememberPick)
	return a.NewDialog(parent, DialogConfig{
		Title:   "Pick a color",
		Width:   300,
		Height:  310,
		Content: chooser,
		Buttons: []DialogButton{
			{Label: "Cancel", Response: "cancel"},
			{Label: "Select", Response: "ok"},
		},
		DefaultResponse: "ok",
		CancelResponse:  "cancel",
		OnResponse: func(resp string) {
			if resp == "ok" {
				rememberPick(chooser.Color())
				if onColor != nil {
					onColor(chooser.Color())
				}
			}
		},
	})
}

// fontFilter narrows a family list by the chooser's controls: query
// is a case-insensitive substring over the family name, monoOnly keeps
// families whose regular face is fixed-width (isMono classifies,
// cached by the caller). Deterministic: the result keeps the input
// order.
func fontFilter(families []string, query string, monoOnly bool, isMono func(string) bool) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	out := families[:0:0]
	for _, fam := range families {
		if query != "" && !strings.Contains(fam, query) {
			continue
		}
		if monoOnly && !isMono(fam) {
			continue
		}
		out = append(out, fam)
	}
	return out
}

// fontRowModel serves the chooser's family list: rows lazily render in
// their own family's face (the virtualized list only builds the rows
// it shows, so only the visible faces ever parse).
type fontRowModel struct {
	face render.Font
	fams []string
}

func (m *fontRowModel) Len() int { return len(m.fams) }

func (m *fontRowModel) Row(i int) widget.Widget {
	return widget.NewLabel(m.face, 13, sysfont.FamilyDisplay(m.fams[i]), widget.Current().Text)
}

// FontChooserDialog opens a modal font picker (#74): a searchable,
// virtualized family list (each row lazily rendered in its own
// family's face), a size entry with a synced slider, a monospace
// filter toggle, and a live preview line shaped through the same
// fallback chain production text uses. Confirming responds "ok" and
// fires onFont with the family name sysfont.Best accepts and the
// chosen pixel size; Esc responds "cancel". The monospace toggle
// classifies families by their fixed-width flag on first use (cached;
// face parses ride the shared face cache).
func (a *Application) FontChooserDialog(parent *Window, initialFamily string, initialSize float64, onFont func(family string, size float64)) (*Dialog, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, errors.New("app: font chooser text face unavailable: no configured tooltip face and the system has no sans font")
	}
	families, err := sysfont.Families()
	if err != nil {
		return nil, fmt.Errorf("app: font chooser: %w", err)
	}
	if initialSize <= 0 {
		initialSize = 13
	}

	th := widget.Current()
	monoCache := map[string]bool{}
	isMono := func(fam string) bool {
		if v, ok := monoCache[fam]; ok {
			return v
		}
		v := false
		if tf, err := sysfont.Best(fam, 13); err == nil {
			v = tf.IsMonospace()
		}
		monoCache[fam] = v
		return v
	}

	model := &fontRowModel{face: face, fams: slices.Clone(families)}
	list := widget.NewList(model, 26)
	if idx := slices.Index(model.fams, strings.ToLower(strings.TrimSpace(initialFamily))); idx >= 0 {
		list.Select(idx)
	}

	search := widget.NewEntry(face, 13, th.Text)
	search.SetPlaceholder("search families")
	mono := widget.NewCheckButton(false)
	sizeEntry := widget.NewEntry(face, 13, th.Text)
	sizeEntry.SetText(strconv.FormatFloat(initialSize, 'f', -1, 64))
	size := widget.NewSlider(6, 72, 1, initialSize)

	// The preview rebuilds its face through the production chain on
	// every applied family or size change: what you see is what ships.
	preview := widget.NewLabel(face, initialSize, "The quick brown fox jumps over the lazy dog 0123", th.Text)
	chosenSize := func() float64 {
		if v, err := strconv.ParseFloat(strings.TrimSpace(sizeEntry.Text()), 64); err == nil && v > 0 && v <= 512 {
			return v
		}
		return size.Value()
	}
	refreshPreview := func() {
		fam := ""
		if i := list.Selected(); i >= 0 && i < len(model.fams) {
			fam = model.fams[i]
		}
		if tf, err := sysfont.Best(fam, 13); err == nil {
			preview.SetFace(sysfont.Fallback(tf))
		}
		preview.SetSizePx(chosenSize())
	}

	refilter := func() {
		model.fams = fontFilter(families, search.Text(), mono.Checked(), isMono)
		list.Changed()
		refreshPreview()
	}
	search.OnChanged = func(string) { refilter() }
	mono.OnChanged = func(bool) { refilter() }
	size.OnChanged = func(v float64) {
		sizeEntry.SetText(strconv.FormatFloat(v, 'f', -1, 64))
		refreshPreview()
	}
	sizeEntry.OnChanged = func(string) { refreshPreview() }
	list.OnSelect = func(int) { refreshPreview() }

	controls := widget.NewBox(widget.Row, 8, 0)
	controls.Append(search, true)
	controls.Append(mono, false)
	sizeRow := widget.NewBox(widget.Row, 8, 0)
	sizeRow.Append(widget.NewLabel(face, 12, "size", th.TextMuted), false)
	sizeRow.Append(size, true)
	sizeRow.Append(sizeEntry, false)
	root := widget.NewBox(widget.Column, 8, 8)
	root.Append(controls, false)
	root.Append(list, true)
	root.Append(sizeRow, false)
	root.Append(preview, false)
	refreshPreview()

	return a.NewDialog(parent, DialogConfig{
		Title:   "Pick a font",
		Width:   380,
		Height:  420,
		Content: root,
		Buttons: []DialogButton{
			{Label: "Cancel", Response: "cancel"},
			{Label: "Select", Response: "ok"},
		},
		DefaultResponse: "ok",
		CancelResponse:  "cancel",
		OnResponse: func(resp string) {
			if resp != "ok" || onFont == nil {
				return
			}
			fam := ""
			if i := list.Selected(); i >= 0 && i < len(model.fams) {
				fam = model.fams[i]
			}
			onFont(fam, chosenSize())
		},
	})
}

// CalendarDialog opens a modal calendar picker: a widget.Calendar
// starting at initial, responding "select" with the chosen date
// through OnResponse when a day is picked (the dialog closes) and
// "cancel" on Esc. onSelect is the Calendar's own hook for apps that
// want every applied selection, not just the closing one.
func (a *Application) CalendarDialog(parent *Window, initial time.Time, onSelect func(time.Time)) (*Dialog, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, errors.New("app: calendar dialog text face unavailable: no configured tooltip face and the system has no sans font")
	}
	cal := widget.NewCalendar(face, 13, initial)
	var d *Dialog
	cal.OnSelect = func(t time.Time) {
		if onSelect != nil {
			onSelect(t)
		}
		if d != nil {
			d.Respond("select")
		}
	}
	var err error
	d, err = a.NewDialog(parent, DialogConfig{
		Title:          "Select date",
		Width:          260,
		Height:         240,
		Content:        cal,
		Buttons:        []DialogButton{{Label: "Cancel", Response: "cancel"}},
		CancelResponse: "cancel",
	})
	return d, err
}

// MessageBox opens the standard preset: a colored icon slot by kind, a
// text body, and a button row.
func (a *Application) MessageBox(parent *Window, kind MessageKind, title, text string, buttons []DialogButton) (*Dialog, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, errors.New("app: message box text face unavailable: no configured tooltip face and the system has no sans font")
	}
	body := widget.NewBox(widget.Row, 16, 0)
	body.Append(newMessageGlyph(kind, 40), false)
	body.Append(widget.NewLabel(face, 14, text, widget.Current().Text), true)
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

// dialogRoot builds the dialog window's root: the content in the
// toolkit's card above a button row, or, Bare, the content alone.
func (a *Application) dialogRoot(cfg DialogConfig, respond func(string)) (widget.Widget, render.Color, error) {
	if cfg.Bare {
		return cfg.Content, cfg.Background, nil
	}
	if len(cfg.Buttons) == 0 {
		cfg.Buttons = []DialogButton{{Label: "OK", Response: "ok"}}
	}
	// The button row's labels need a face; under the toolkit-wide
	// nil-face contract a nil face into a constructor panics, so a
	// system without any usable font surfaces as an error here.
	face := a.resolveFace(nil)
	if face == nil {
		return nil, 0, errors.New("app: dialog text face unavailable: no configured tooltip face and the system has no sans font")
	}
	buttons := widget.NewBox(widget.Row, 8, 0)
	buttons.Append(widget.NewSpacer(0, 0), true)
	for _, b := range cfg.Buttons {
		resp := b.Response
		btn := widget.NewButton(
			widget.NewBox(widget.Row, 6, 0).
				Append(widget.NewLabel(face, 13, b.Label, widget.Current().Text), false),
			10, 6)
		btn.OnClick = func() { respond(resp) }
		buttons.Append(btn, false)
	}
	content := widget.NewBox(widget.Column, 12, 12)
	content.Append(cfg.Content, true)
	content.Append(buttons, false)
	root, background := dialogCard(content)
	return root, background, nil
}
