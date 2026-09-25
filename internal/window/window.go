// Package window maps the xdg-shell role onto a wl_surface: real
// toplevel windows with titles, the configure handshake, ping/pong
// liveness, and the close signal.
package window

import (
	"errors"
	"fmt"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/xdg"
)

// ErrNotConfigured reports a draw attempt before the first configure
// event. The compositor rejects buffers committed before it.
var ErrNotConfigured = errors.New("window: not configured yet")

// ErrClosed reports that the window was closed; the objects must not be
// used again.
var ErrClosed = errors.New("window: closed")

// Config describes the window before the first commit.
type Config struct {
	// Title and AppID identify the window to the compositor.
	Title, AppID string
	// Width and Height in surface (logical) pixels. Zeros let the
	// compositor pick.
	Width, Height uint32
}

// Window is one toplevel window and its configure handshake.
type Window struct {
	WLSurface  *wl.Surface
	XdgSurface *xdg.Surface
	Toplevel   *xdg.Toplevel

	wmBase     *xdg.WmBase
	closed     bool
	configured bool
	width      uint32
	height     uint32
}

// New assigns the xdg_toplevel role and sends the initial state. The
// caller must still commit the wl_surface; the compositor answers with
// the first configure round.
func New(wmBase *xdg.WmBase, surf *wl.Surface, cfg Config) (*Window, error) {
	xdgSurf, err := wmBase.GetSurface(surf)
	if err != nil {
		return nil, fmt.Errorf("window: get_xdg_surface: %w", err)
	}
	tl, err := xdgSurf.GetToplevel()
	if err != nil {
		return nil, fmt.Errorf("window: get_toplevel: %w", err)
	}
	w := &Window{
		WLSurface: surf, XdgSurface: xdgSurf, Toplevel: tl, wmBase: wmBase,
		width: cfg.Width, height: cfg.Height,
	}
	xdgSurf.AddConfigureHandler(w)
	tl.AddConfigureHandler(w)
	tl.AddCloseHandler(w)

	if err := tl.SetTitle(cfg.Title); err != nil {
		return nil, fmt.Errorf("window: set_title: %w", err)
	}
	if err := tl.SetAppId(cfg.AppID); err != nil {
		return nil, fmt.Errorf("window: set_app_id: %w", err)
	}
	if cfg.Width != 0 && cfg.Height != 0 {
		if err := tl.SetMinSize(int32(cfg.Width), int32(cfg.Height)); err != nil {
			return nil, fmt.Errorf("window: set_min_size: %w", err)
		}
	}
	return w, nil
}

// Pong answers the compositor's liveness ping with the pinged serial.
// Wire Session.OnWmBasePing to this; a window that fails to pong is
// killed.
func (w *Window) Pong(serial uint32) {
	if w.wmBase != nil {
		_ = w.wmBase.Pong(serial)
	}
}

// HandleSurfaceConfigure completes the configure round: it marks the
// window configured and acknowledges the serial. Without a wire surface
// (tests) only the state changes.
func (w *Window) HandleSurfaceConfigure(ev xdg.SurfaceConfigureEvent) {
	w.configured = true
	if w.XdgSurface != nil {
		_ = w.XdgSurface.AckConfigure(ev.Serial)
	}
}

// HandleToplevelConfigure records the new size; a zero axis keeps the
// previous choice, per the protocol.
func (w *Window) HandleToplevelConfigure(ev xdg.ToplevelConfigureEvent) {
	if ev.Width != 0 {
		w.width = uint32(ev.Width)
	}
	if ev.Height != 0 {
		w.height = uint32(ev.Height)
	}
}

// HandleToplevelClose marks the window closed.
func (w *Window) HandleToplevelClose(xdg.ToplevelCloseEvent) {
	w.closed = true
}

// EnsureUsable gates drawing: the window must have completed the first
// configure round and must not be closed.
func (w *Window) EnsureUsable() error {
	if w.closed {
		return ErrClosed
	}
	if !w.configured {
		return ErrNotConfigured
	}
	return nil
}

// Closed reports whether the window was closed.
func (w *Window) Closed() bool { return w.closed }

// Close marks the window closed from the client side (a keybinding, for
// instance). The next loop check exits.
func (w *Window) Close() { w.closed = true }

// Size returns the last configured size in surface (logical) pixels. An
// unconfigured window reports zeros.
func (w *Window) Size() (int, int) {
	return int(w.width), int(w.height)
}
