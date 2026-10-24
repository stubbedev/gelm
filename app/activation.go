package app

import (
	"github.com/stubbedev/lys-ui/internal/wlsession"
)

// ErrActivationUnavailable is a token request on a compositor without
// xdg-activation.
var ErrActivationUnavailable = wlsession.ErrActivationUnavailable

// RequestActivationToken mints an xdg-activation token for a launch or
// an activation the application performs itself: a notification
// daemon handing the app whose action was pressed a token
// (org.freedesktop.Notifications.ActivationToken), a portal passing
// one in platform_data. It is anchored to the focused window and the
// latest input event, as a launch is, so it vouches for a real
// interaction. done runs once, on the event loop, with the token, or
// with ErrActivationUnavailable on a compositor without
// xdg-activation, or the request's failure.
func (a *Application) RequestActivationToken(done func(token string, err error)) {
	a.Invoke(func() {
		a.sess.RequestActivationToken(a.focusSurface(), a.sess.InputSerial(), done)
	})
}

// RequestActivationToken is Application.RequestActivationToken
// anchored to this window - a press on it rather than on the focused
// window. A closed window reports ErrClosed.
func (w *Window) RequestActivationToken(done func(token string, err error)) {
	w.app.Invoke(func() {
		if w.win == nil || w.closed || w.win.Closed() {
			done("", ErrClosed)
			return
		}
		w.app.sess.RequestActivationToken(w.win.HostSurface(), w.app.sess.InputSerial(), done)
	})
}

// Activate asks the compositor to raise and focus the window with a
// token another client minted for it - the one in
// XDG_ACTIVATION_TOKEN, a notification action's ActivationToken, a
// portal's platform_data. The compositor decides. Nothing happens
// without xdg-activation, for an empty token, or on a closed window.
func (w *Window) Activate(token string) {
	w.app.Invoke(func() {
		if w.win == nil || w.closed || w.win.Closed() {
			return
		}
		w.app.sess.Activate(w.win.HostSurface(), token)
	})
}
