package app

import (
	"fmt"

	"github.com/stubbedev/lys-ui/internal/debug"
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget"
)

// popupHost opens the popups a surface's widgets ask for
// (widget.OpenMenu): menu popovers over host, nested under parent when
// the surface is a popover.
type popupHost struct {
	a      *Application
	host   Host
	parent *Popover
}

// OpenMenu implements widget.PopupHost.
func (p popupHost) OpenMenu(anchor widget.Boundser, face render.Font, sizePx float64, items []widget.MenuItem) {
	cfg := MenuPopoverConfig{Anchor: anchor, Face: face, SizePx: sizePx, Items: items, Parent: p.parent}
	if _, err := p.a.OpenMenuPopover(p.host, cfg); err != nil {
		p.a.reportPopup(fmt.Errorf("app: open a widget's menu: %w", err))
	}
}

// SetPopupErrorHandler installs fn to hear popups a widget asked for
// (widget.OpenMenu) that could not open, so the app can show a toast.
// fn runs on the loop goroutine.
func (a *Application) SetPopupErrorHandler(fn func(err error)) { a.onPopupError = fn }

// reportPopup hands a failed popup to the installed handler.
func (a *Application) reportPopup(err error) {
	debug.Log("popup", "%v", err)
	if a.onPopupError != nil {
		a.onPopupError(err)
	}
}
