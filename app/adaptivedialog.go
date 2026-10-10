package app

import (
	"errors"

	"github.com/stubbedev/gelm/widget"
)

// DialogPresentation is how an AdaptiveDialog shows.
type DialogPresentation uint8

const (
	// PresentAuto floats the dialog when the parent is wide enough and
	// shows it as a bottom sheet inside the parent otherwise.
	PresentAuto DialogPresentation = iota
	// PresentFloating always shows a parented dialog window.
	PresentFloating
	// PresentBottomSheet always shows a sheet inside the parent window.
	PresentBottomSheet
)

const defaultSheetBelow = 450

// AdaptiveDialogConfig declares an AdaptiveDialog: a DialogConfig plus
// how to present it.
type AdaptiveDialogConfig struct {
	DialogConfig
	// Presentation picks the presentation; PresentAuto by default.
	Presentation DialogPresentation
	// SheetBelow is the parent width (logical pixels) under which
	// PresentAuto uses the bottom sheet; zero means 450, libadwaita's
	// narrow breakpoint.
	SheetBelow int
}

// AdaptiveDialog is libadwaita's adaptive AdwDialog: over a wide
// parent it is a regular window-modal dialog; over a narrow one it is
// a bottom sheet sliding up inside the parent window, closed by a
// response, Esc, a click on the scrim or a drag down. The presentation
// is decided when the dialog opens.
type AdaptiveDialog struct {
	app       *Application
	cfg       AdaptiveDialogConfig
	floating  *Dialog
	sheet     *widget.BottomSheet
	host      *hostWindow
	presented DialogPresentation
	responded bool
	closed    bool
}

// AdaptiveDialog opens a dialog over parent, floating or as a bottom
// sheet per cfg.Presentation.
func (a *Application) AdaptiveDialog(parent *Window, cfg AdaptiveDialogConfig) (*AdaptiveDialog, error) {
	if parent == nil {
		return nil, errors.New("app: an adaptive dialog needs a parent window")
	}
	d := &AdaptiveDialog{app: a, cfg: cfg, presented: cfg.Presentation}
	if d.presented == PresentAuto {
		below := cfg.SheetBelow
		if below <= 0 {
			below = defaultSheetBelow
		}
		d.presented = PresentFloating
		if w, _ := parent.Size(); w > 0 && w < below {
			d.presented = PresentBottomSheet
		}
	}
	if d.presented == PresentFloating {
		inner := cfg.DialogConfig
		inner.OnClosed = d.chainClosed(inner.OnClosed)
		f, err := a.NewDialog(parent, inner)
		if err != nil {
			return nil, err
		}
		d.floating = f
		return d, nil
	}
	hw := a.hostOf(parent)
	if hw == nil {
		return nil, errors.New("app: the dialog's parent window is not open")
	}
	root, _, err := a.dialogRoot(cfg.DialogConfig, d.Respond)
	if err != nil {
		return nil, err
	}
	d.host = hw
	d.sheet = widget.NewBottomSheet(hw.router.Root, root)
	d.sheet.OnClosed = d.sheetClosed
	hw.router.Root = d.sheet
	hw.dirty = true
	d.sheet.SetOpen(true)
	return d, nil
}

func (d *AdaptiveDialog) chainClosed(then func()) func() {
	return func() {
		d.closed = true
		if then != nil {
			then()
		}
	}
}

// Presentation reports how the dialog was presented.
func (d *AdaptiveDialog) Presentation() DialogPresentation { return d.presented }

// Respond answers the dialog with response, as its buttons, Esc and
// Enter do: ValidateResponse may refuse, OnResponse hears it, and the
// dialog closes.
func (d *AdaptiveDialog) Respond(response string) {
	if d.floating != nil {
		d.floating.Respond(response)
		return
	}
	if d.responded || d.closed {
		return
	}
	if v := d.cfg.ValidateResponse; v != nil && !v(response) {
		return
	}
	d.responded = true
	if d.cfg.OnResponse != nil {
		d.cfg.OnResponse(response)
	}
	d.sheet.SetOpen(false)
}

// Close closes the dialog without a response.
func (d *AdaptiveDialog) Close() {
	if d.floating != nil {
		d.floating.Close()
		return
	}
	d.responded = true
	d.sheet.SetOpen(false)
}

// Closed reports whether the dialog has closed or is closing.
func (d *AdaptiveDialog) Closed() bool {
	if d.floating != nil {
		return d.floating.Closed()
	}
	return d.closed || d.responded
}

func (d *AdaptiveDialog) sheetClosed() {
	if d.closed {
		return
	}
	if !d.responded && d.cfg.CancelResponse != "" && d.cfg.OnResponse != nil {
		d.cfg.OnResponse(d.cfg.CancelResponse)
	}
	d.responded, d.closed = true, true
	if d.host.router.Root == widget.Widget(d.sheet) {
		d.host.router.Root = d.sheet.Content()
		d.host.dirty = true
	}
	if d.cfg.OnClosed != nil {
		d.cfg.OnClosed()
	}
}
