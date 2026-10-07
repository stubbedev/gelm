package app

import (
	"github.com/stubbedev/gelm/widget"
)

// Client-side window chrome wiring (#91): the widgets live in the
// widget package (HeaderBar, WindowControls, ActionBar, MenuBar);
// this file is the app half - the runtime window operations the
// controls and the header's grab route into, and the one-call
// attach helpers that wire a bar to its window.
//
// Decoration policy: xdg-decoration stays as it is - when the
// compositor offers server-side decoration the window asks for it and
// keeps its passive edge handles; a HeaderBar in the tree is an app's
// explicit opt into client-side chrome, the GTK4 default inverted:
// gelm windows are undecorated unless the app draws chrome or the
// compositor decorates them. Both can coexist (a compositor frame
// around an app-drawn header); the app is always right about what it
// draws.

// SetTitle updates the compositor-visible window title at runtime -
// taskbars, alt-tab, wherever the environment shows it. The app's own
// HeaderBar text is separate and stays the app's to update.
func (w *Window) SetTitle(title string) {
	if w.win != nil {
		_ = w.win.SetTitle(title)
	}
}

// Minimize asks the compositor to minimize the window; a hint
// compositors without a minimized concept ignore. See Maximize for
// the state-request contract.
func (w *Window) Minimize() {
	if w.win != nil {
		_ = w.win.Minimize()
	}
}

// ToggleMaximize flips between maximized and restored by the last
// configure-confirmed state - the header double-press contract.
func (w *Window) ToggleMaximize() {
	if w.stateMaximized() {
		w.Unmaximize()
		return
	}
	w.Maximize()
}

// AttachHeader wires a HeaderBar to its window: close, minimize, and
// maximize controls shown and hooked, the double-press mapped to
// maximize/restore. The press-to-move grab needs no wiring - the input
// pipeline routes any WindowMover press into xdg_toplevel.move. The
// buttons are shown unconditionally: minimize and maximize are hints
// a compositor is free to ignore, and a hidden button cannot be
// discovered while an ignored one simply does nothing - the honest
// degradation GTK ships too.
func (a *Application) AttachHeader(win *Window, bar *widget.HeaderBar) {
	controls := bar.Controls()
	controls.ShowClose(true)
	controls.ShowMinimize(true)
	controls.ShowMaximize(true)
	controls.OnClose = win.Close
	controls.OnMinimize = win.Minimize
	controls.OnMaximize = win.ToggleMaximize
	bar.OnDoubleClick = win.ToggleMaximize
}

// AttachMenuBar wires a MenuBar's roots to their popovers: activating
// root i opens menus[i] below its button, through the same menu
// popover machinery a menu button uses. F10 reaches the bar when the
// app routes the key to it (an accelerator or OnKey handoff).
func (a *Application) AttachMenuBar(host Host, bar *widget.MenuBar, menus [][]widget.MenuItem) {
	face := a.resolveFace(nil)
	bar.OnRoot = func(i int, anchor widget.Boundser) {
		if i < 0 || i >= len(menus) {
			return
		}
		_, _ = a.OpenMenuPopover(host, MenuPopoverConfig{
			Anchor: anchor,
			Face:   face,
			Items:  menus[i],
		})
	}
}

// stateMaximized reports the confirmed maximized state for the
// double-click toggle (tests).
func (w *Window) stateMaximized() bool {
	hw := w.app.hostOf(w)
	if hw == nil || hw.state == nil {
		return false
	}
	return hw.state().Maximized
}
