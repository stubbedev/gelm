package component

import "github.com/stubbedev/gelm/app"

// DialogResponder is implemented by components run as a dialog that
// react to its response (a button, Esc, Enter): OnResponse runs on the
// loop before the dialog closes and the component shuts down, so it can
// still emit outputs.
type DialogResponder[In, Out any] interface {
	OnResponse(cx *Context[In, Out], response string)
}

// Dialog runs c as the content of a dialog over parent. The component
// shuts down when the dialog closes, and shutting the component down
// closes the dialog.
func Dialog[In, Out any](a *app.Application, parent app.DialogParent, cfg app.DialogConfig, c Component[In, Out]) (*Controller[In, Out], *app.Dialog, error) {
	if cfg.Content != nil {
		return nil, nil, errContentSet
	}
	ctrl := Launch(a, c)
	cx := ctrl.cx
	cfg.Content = ctrl.Widget()
	respond := cfg.OnResponse
	cfg.OnResponse = func(response string) {
		if r, ok := c.(DialogResponder[In, Out]); ok {
			r.OnResponse(cx, response)
		}
		if respond != nil {
			respond(response)
		}
	}
	cfg.OnClosed = chain(ctrl.Shutdown, cfg.OnClosed)
	d, err := a.NewDialog(parent, cfg)
	if err != nil {
		ctrl.Shutdown()
		return nil, nil, err
	}
	cx.OnShutdown(func() {
		if !d.Closed() {
			d.Close()
		}
	})
	return ctrl, d, nil
}

// Popover runs c as the content of a popover over host. The component
// shuts down when the popover closes, and shutting the component down
// dismisses the popover.
func Popover[In, Out any](a *app.Application, host app.Host, cfg app.PopoverConfig, c Component[In, Out]) (*Controller[In, Out], *app.Popover, error) {
	if cfg.Content != nil {
		return nil, nil, errContentSet
	}
	ctrl := Launch(a, c)
	cfg.Content = ctrl.Widget()
	cfg.OnClosed = chain(ctrl.Shutdown, cfg.OnClosed)
	p, err := a.OpenPopover(host, cfg)
	if err != nil {
		ctrl.Shutdown()
		return nil, nil, err
	}
	ctrl.cx.OnShutdown(func() {
		if !p.Closed() {
			p.Dismiss()
		}
	})
	return ctrl, p, nil
}
