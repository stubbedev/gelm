package app

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/logutil"
	"github.com/stubbedev/gelm/internal/recentfiles"
)

// External launching (#86): app.OpenURL and app.OpenPath hand a URL or
// a file to the user's handlers - the browser for https links, the
// file's app for files - the GTK UriLauncher/FileLauncher pair.
//
// Transport: the xdg-desktop-portal OpenURI interface when a portal
// owns the desktop (sandbox-friendly, and the only correct way inside
// flatpaks), xdg-open(1) otherwise, and nothing succeeds without
// either - the error comes back for the app to surface. The portal
// call carries an xdg-activation token requested from the session at
// call time, so the launched app may take focus per the compositor's
// focus-stealing rules; a session without the activation protocol
// launches without one.

// launchTimeout bounds the portal call.
const launchTimeout = 3 * time.Second

const (
	portalOpenName  = "org.freedesktop.portal.Desktop"
	portalOpenPath  = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	portalOpenIface = "org.freedesktop.portal.OpenURI"
	portalOpenCall  = portalOpenIface + ".OpenURI"
)

// launcher is the Application's launch state: the transport seams
// (tests substitute them) and the pending-activation queue. openPortal
// and openExternal are the seams; the defaults are the portal D-Bus
// call and xdg-open.
type launcher struct {
	mu sync.Mutex
	// pending holds URIs waiting for their activation token, oldest
	// first; the token callback launches and shifts. Bounded: a burst
	// beyond the bound launches without waiting for tokens.
	pending []string

	openPortal   func(uri, parentWindow, token string) error
	openExternal func(uri string) error
}

// OpenURL opens url in the user's handler for its scheme. Safe from
// any goroutine; the transport work happens off the caller. The
// compositor decides focus, guided by an activation token when the
// session's xdg-activation support issued one.
func (a *Application) OpenURL(uri string) {
	a.launch(uri)
}

// OpenPath opens a file or directory in the user's handler for it,
// the FileLauncher shape.
func (a *Application) OpenPath(path string) {
	a.launch(pathToFileURI(path))
}

// launch routes one URI: with activation support it waits for an
// issued token (the open rides the token callback), else it opens
// now; a burst beyond the bound opens its oldest without waiting.
func (a *Application) launch(uri string) {
	if uri == "" {
		return
	}
	l := &a.launchState
	canToken := a.sess != nil && a.sess.ActivationAvailable()
	l.mu.Lock()
	l.pending = append(l.pending, uri)
	var now string
	if !canToken || len(l.pending) > 4 {
		now = l.pending[0]
		l.pending = l.pending[1:]
	}
	l.mu.Unlock()
	if now != "" {
		a.launchNow(now, "")
		return
	}
	a.wireActivationToken()
	// Anchored to the focused surface and the latest input serial the
	// session saw, so the token vouches for a real interaction.
	a.sess.RequestActivationToken(a.focusSurface(), a.LastPressSerial(nil))
}

// focusSurface is the focused window's surface, for anchoring the
// activation token.
func (a *Application) focusSurface() *wl.Surface {
	if hw := a.focused(); hw != nil && hw.host != nil {
		return hw.host.HostSurface()
	}
	return nil
}

// wireActivationToken hooks the session's token callback once; later
// tokens launch the pending URIs. An app's own callback, if it set
// one, keeps running.
func (a *Application) wireActivationToken() {
	a.launchOnce.Do(func() {
		if a.sess == nil {
			return
		}
		previous := a.sess.OnActivationToken
		a.sess.OnActivationToken = func(token string) {
			if previous != nil {
				previous(token)
			}
			a.launchOldestWith(token)
		}
	})
}

// launchOldestWith opens the oldest pending URI carrying token.
func (a *Application) launchOldestWith(token string) {
	l := &a.launchState
	l.mu.Lock()
	if len(l.pending) == 0 {
		l.mu.Unlock()
		return
	}
	uri := l.pending[0]
	l.pending = l.pending[1:]
	l.mu.Unlock()
	a.launchNow(uri, token)
}

// launchNow runs the transport off the caller's goroutine: portal
// first, xdg-open when no portal answered, an error logged (Debug)
// when neither worked. The focused-surface read happens on the
// caller's side - loop state never leaves the loop goroutine.
func (a *Application) launchNow(uri, token string) {
	l := &a.launchState
	parent := ""
	if a.sess != nil {
		if hw := a.focused(); hw != nil && hw.host != nil {
			parent = surfacePortalID(hw.host.HostSurface())
		}
	}
	go func() {
		if l.openPortal == nil {
			l.openPortal = portalOpen
		}
		portalErr := l.openPortal(uri, parent, token)
		if portalErr == nil {
			return
		}
		if !errors.Is(portalErr, errNoPortal) {
			debug.Log("shell", "open uri: portal: %v", portalErr)
		}
		if l.openExternal == nil {
			l.openExternal = xdgOpen
		}
		if err := l.openExternal(uri); err != nil {
			logutil.L().Debug("open uri failed", "uri", uri, "portal", portalErr, "fallback", err)
		}
	}()
}

// surfacePortalID formats a surface's wayland proxy id as the
// portal's parent_window string; the compositor-side object id is
// what the portal understands.
func surfacePortalID(surface *wl.Surface) string {
	return fmt.Sprintf("wayland:%d", surface.Id())
}

// errNoPortal reports the absent-portal case (the fallback runs).
var errNoPortal = errors.New("no xdg-desktop-portal")

// portalOpen calls the OpenURI interface on the session bus.
func portalOpen(uri, parentWindow, token string) error {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return errNoPortal
	}
	defer func() { _ = conn.Close() }()
	var owned bool
	if err := conn.Object("org.freedesktop.DBus", "/").CallWithContext(
		conn.Context(), "org.freedesktop.DBus.NameHasOwner", 0,
		portalOpenName).Store(&owned); err != nil || !owned {
		return errNoPortal
	}
	options := map[string]dbus.Variant{}
	if token != "" {
		options["activation_token"] = dbus.MakeVariant(token)
	}
	ctx, cancel := context.WithTimeout(conn.Context(), launchTimeout)
	defer cancel()
	return conn.Object(portalOpenName, portalOpenPath).CallWithContext(
		ctx, portalOpenCall, dbus.FlagNoReplyExpected, parentWindow, uri, options).Err
}

// xdgOpen runs xdg-open(1) detached.
func xdgOpen(uri string) error {
	cmd := exec.Command("xdg-open", uri) //nolint:gosec // the documented external opener; the URI is the user-chosen link
	return cmd.Start()
}

// pathToFileURI encodes a local path as a file:// URI; an empty or
// relative path is returned unchanged for the transport to reject.
func pathToFileURI(path string) string {
	return recentfiles.PathToURI(path)
}

// OpenURLFromLink wires a RichLabel's (or any link-emitting widget's)
// OnLinkClick to OpenURL - the sanctioned one-liner links have been
// missing:
//
//	label.OnLinkClick = application.OpenURLFromLink
func (a *Application) OpenURLFromLink(href string) { a.OpenURL(href) }
