package component

import (
	"errors"

	"github.com/stubbedev/gelm/app"
)

var _ Loop = (*app.Application)(nil)

var errRootSet = errors.New("component: the config's Root is set; the component provides the root")

// Window launches c as the root of a new toplevel window: the
// counterpart of relm4's RelmApp::run. The component shuts down when the
// window closes; a window that fails to open shuts it down at once.
func Window[In, Out any](a *app.Application, cfg app.WindowConfig, c Component[In, Out]) (*Controller[In, Out], *app.Window, error) {
	if cfg.Root != nil {
		return nil, nil, errRootSet
	}
	ctrl := Launch(a, c)
	cfg.Root = ctrl.Widget()
	cfg.OnClosed = chain(ctrl.Shutdown, cfg.OnClosed)
	w, err := a.NewWindow(cfg)
	if err != nil {
		ctrl.Shutdown()
		return nil, nil, err
	}
	return ctrl, w, nil
}

// Layer launches c as the root of a new layer surface, like Window.
func Layer[In, Out any](a *app.Application, cfg app.LayerConfig, c Component[In, Out]) (*Controller[In, Out], *app.LayerWindow, error) {
	if cfg.Root != nil {
		return nil, nil, errRootSet
	}
	ctrl := Launch(a, c)
	cfg.Root = ctrl.Widget()
	cfg.OnClosed = chain(ctrl.Shutdown, cfg.OnClosed)
	l, err := a.NewLayer(cfg)
	if err != nil {
		ctrl.Shutdown()
		return nil, nil, err
	}
	return ctrl, l, nil
}

func chain(first, then func()) func() {
	if then == nil {
		return first
	}
	return func() {
		first()
		then()
	}
}
