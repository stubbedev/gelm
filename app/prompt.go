package app

import (
	"github.com/stubbedev/gelm/widget"
)

// PromptDialog opens a one-entry question dialog - the rename-prompt
// shape: message, a single text entry prefilled with initial, Enter
// confirms with the entry's text (onText, exactly once, on the loop),
// Escape cancels. Empty text refuses to close the dialog - prompts
// answer with something - unless allowEmpty opts that policy off.
func (a *Application) PromptDialog(parent *Window, title, message, initial string, allowEmpty bool, onText func(string)) (*Dialog, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, ErrNoDialogFace
	}
	th := widget.Current()
	entry := widget.NewEntry(face, 14, th.Text)
	entry.SetText(initial)
	body := widget.NewBox(widget.Column, 10, 4)
	if message != "" {
		body.Append(widget.NewLabel(face, 14, message, th.Text), false)
	}
	body.Append(entry, true)
	var d *Dialog
	d, err := a.NewDialog(parent, DialogConfig{
		Title:   title,
		Width:   360,
		Height:  150,
		Content: body,
		Modal:   true,
		Buttons: []DialogButton{
			{Label: widget.Tr("Cancel"), Response: "cancel", Role: ButtonRoleCancel},
			{Label: widget.Tr("OK"), Response: "ok", Role: ButtonRoleDefault},
		},
		DefaultResponse: "ok",
		CancelResponse:  "cancel",
		ValidateResponse: func(resp string) bool {
			if resp == "ok" && !allowEmpty && entry.Text() == "" {
				return false
			}
			return true
		},
		OnResponse: func(resp string) {
			if resp == "ok" && onText != nil {
				onText(entry.Text())
			}
		},
	})
	return d, err
}

// Progress reports a ProgressDialog's progress to its caller: the
// fraction path drives a determinate bar, Pulse switches to the busy
// spinner.
type Progress struct {
	bar     *widget.ProgressBar
	spinner *widget.Spinner
}

// SetFraction moves the determinate bar to f (clamped to [0, 1]);
// building on a pulsing dialog is a no-op.
func (p *Progress) SetFraction(f float64) {
	if p.bar != nil {
		p.bar.SetValue(f)
	}
}

// ProgressDialog opens a modal progress dialog: a text line over a
// determinate bar the caller drives through the returned Progress (or
// a busy spinner, with pulsing true). Cancelling - the button, or
// Escape - responds "cancel" through OnCancel and leaves closing to
// the caller: long work keeps the dialog up by holding the returned
// Dialog open until it Responds.
func (a *Application) ProgressDialog(parent *Window, title, text string, pulsing bool, onCancel func()) (*Dialog, *Progress, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, nil, ErrNoDialogFace
	}
	th := widget.Current()
	body := widget.NewBox(widget.Column, 12, 4)
	if text != "" {
		body.Append(widget.NewLabel(face, 14, text, th.Text), false)
	}
	p := &Progress{}
	if pulsing {
		p.spinner = widget.NewSpinner(28)
		p.spinner.SetSpinning(true)
		body.Append(p.spinner, false)
	} else {
		p.bar = widget.NewProgressBar(0)
		body.Append(p.bar, false)
	}
	buttons := []DialogButton{}
	if onCancel != nil {
		buttons = append(buttons, DialogButton{Label: widget.Tr("Cancel"), Response: "cancel", Role: ButtonRoleCancel})
	}
	d, err := a.NewDialog(parent, DialogConfig{
		Title:          title,
		Width:          360,
		Height:         140,
		Content:        body,
		Modal:          true,
		Buttons:        buttons,
		CancelResponse: "cancel",
		OnResponse: func(resp string) {
			if resp == "cancel" {
				onCancel()
			}
		},
	})
	if err != nil {
		return nil, nil, err
	}
	return d, p, nil
}
