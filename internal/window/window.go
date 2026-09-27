// Package window maps the xdg-shell role onto a wl_surface: real
// toplevel windows with titles, the configure handshake, the
// compositor-confirmed state set, ping/pong liveness, and the close
// signal.
package window

import (
	"errors"
	"fmt"
	"strings"

	xdeco "github.com/neurlang/wayland/unstable/xdg-decoration-v1"
	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/xdg"

	"github.com/stubbedev/gelm/internal/debug"
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
	// MinWidth and MinHeight constrain how far the compositor may
	// resize the window down; zero axes are unconstrained. The app
	// clamps layout to the same limits (SizeLimits) in case a
	// compositor configures outside them.
	MinWidth, MinHeight uint32
	// MaxWidth and MaxHeight constrain how far the compositor may
	// resize the window up; zero axes are unconstrained.
	MaxWidth, MaxHeight uint32
}

// State is the compositor-confirmed toplevel state: the state array of
// the most recent configure event, never the set of requests the client
// sent. A request (Maximize and friends) is a hint the compositor may
// refuse, delay, or undo in a later configure; only what arrives in the
// state array is true, so UI chrome reads this, not the requests.
//
// There is no minimized bit: the protocol reports no state for a
// minimized (unmapped) window, and State keeps reporting what was last
// confirmed before it.
type State struct {
	Activated  bool // keyboard focus
	Maximized  bool
	Fullscreen bool
	Resizing   bool // an interactive resize (Resize grab) is streaming
	Suspended  bool // the compositor stopped the window (not visible)
	// The tiled_* bits: the compositor snapped the window to an edge.
	TiledLeft, TiledRight, TiledTop, TiledBottom bool
}

// String renders the confirmed state set ("activated+maximized"),
// "normal" when it is empty. The order is fixed so traces and dumps
// are comparable.
func (s State) String() string {
	parts := make([]string, 0, 9)
	add := func(on bool, name string) {
		if on {
			parts = append(parts, name)
		}
	}
	add(s.Activated, "activated")
	add(s.Maximized, "maximized")
	add(s.Fullscreen, "fullscreen")
	add(s.Resizing, "resizing")
	add(s.Suspended, "suspended")
	add(s.TiledLeft, "tiled-left")
	add(s.TiledRight, "tiled-right")
	add(s.TiledTop, "tiled-top")
	add(s.TiledBottom, "tiled-bottom")
	if len(parts) == 0 {
		return "normal"
	}
	return strings.Join(parts, "+")
}

// parseStates decodes a configure event's state array (one 32-bit
// xdg_toplevel.state value per entry) into the confirmed set. Unknown
// values are ignored: newer compositors may carry states this build
// does not know.
func parseStates(states []int32) State {
	var s State
	for _, v := range states {
		switch uint32(v) {
		case xdg.ToplevelStateActivated:
			s.Activated = true
		case xdg.ToplevelStateMaximized:
			s.Maximized = true
		case xdg.ToplevelStateFullscreen:
			s.Fullscreen = true
		case xdg.ToplevelStateResizing:
			s.Resizing = true
		case xdg.ToplevelStateSuspended:
			s.Suspended = true
		case xdg.ToplevelStateTiledLeft:
			s.TiledLeft = true
		case xdg.ToplevelStateTiledRight:
			s.TiledRight = true
		case xdg.ToplevelStateTiledTop:
			s.TiledTop = true
		case xdg.ToplevelStateTiledBottom:
			s.TiledBottom = true
		}
	}
	return s
}

// Window is one toplevel window and its configure handshake.
type Window struct {
	WLSurface  *wl.Surface
	XdgSurface *xdg.Surface
	Toplevel   *xdg.Toplevel

	wmBase     *xdg.WmBase
	decoration *xdeco.ZxdgToplevelDecorationV1

	// onCloseRequest vetoes the compositor's close request when it
	// returns false (an unsaved-changes prompt, for instance); nil
	// means every close request is accepted.
	onCloseRequest func() bool

	closed     bool
	configured bool
	width      uint32
	height     uint32
	minW, minH uint32
	maxW, maxH uint32
	// state is the compositor-confirmed set from the last configure;
	// requests never write it.
	state State
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
		minW: cfg.MinWidth, minH: cfg.MinHeight,
		maxW: cfg.MaxWidth, maxH: cfg.MaxHeight,
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
	if cfg.MinWidth != 0 || cfg.MinHeight != 0 {
		if err := tl.SetMinSize(int32(cfg.MinWidth), int32(cfg.MinHeight)); err != nil {
			return nil, fmt.Errorf("window: set_min_size: %w", err)
		}
	}
	if cfg.MaxWidth != 0 || cfg.MaxHeight != 0 {
		if err := tl.SetMaxSize(int32(cfg.MaxWidth), int32(cfg.MaxHeight)); err != nil {
			return nil, fmt.Errorf("window: set_max_size: %w", err)
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
// previous choice, per the protocol. The state array is authoritative:
// it replaces the confirmed set wholesale, so a configure without a
// state the client asked for reports the request as refused (or
// withdrawn).
func (w *Window) HandleToplevelConfigure(ev xdg.ToplevelConfigureEvent) {
	if ev.Width != 0 {
		w.width = uint32(ev.Width)
	}
	if ev.Height != 0 {
		w.height = uint32(ev.Height)
	}
	state := parseStates(ev.States)
	if state != w.state {
		w.state = state
		debug.Log("shell", "toplevel state %s, %dx%d", state, w.width, w.height)
	}
}

// State returns the compositor-confirmed state: what the last
// configure's state array said, not what the client asked for.
func (w *Window) State() State { return w.state }

// Maximize asks the compositor to maximize the window. Like every
// state request it is a hint: it becomes visible only through a
// configure carrying the maximized state, which the compositor is free
// to refuse - judge the outcome through State. A wireless window
// (tests) sends nothing.
func (w *Window) Maximize() error {
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.SetMaximized()
}

// Unmaximize asks the compositor to restore the window. See Maximize.
func (w *Window) Unmaximize() error {
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.UnsetMaximized()
}

// Fullscreen asks the compositor to show the window fullscreen; a nil
// output lets the compositor pick (the usual choice). The confirmed
// fullscreen state arrives with the output-sized configure. See
// Maximize.
func (w *Window) Fullscreen(output *wl.Output) error {
	if w.Toplevel == nil {
		return nil
	}
	if output == nil {
		// The protocol's NULL: the compositor picks the output. A plain
		// typed nil cannot ride SendRequest (its ordering pass calls
		// Id() on every proxy argument and panics on a nil receiver), so
		// the zero-value stand-in writes the 0 object word instead.
		output = &wl.Output{}
	}
	return w.Toplevel.SetFullscreen(output)
}

// Unfullscreen asks the compositor to leave fullscreen. See Maximize.
func (w *Window) Unfullscreen() error {
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.UnsetFullscreen()
}

// SetMinimized asks the compositor to minimize the window. The
// protocol defines no minimized state event, so State keeps reporting
// whatever was confirmed before; treat the window as invisible from
// here on. See Maximize.
func (w *Window) SetMinimized() error {
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.SetMinimized()
}

// SetCloseRequest installs a veto callback: returning false keeps the
// window open when the compositor asks it to close (an unsaved-changes
// prompt decides itself when to call Window.Close). Nil accepts every
// close request.
func (w *Window) SetCloseRequest(veto func() bool) { w.onCloseRequest = veto }

// SetMinSize publishes the smallest size the compositor may resize the
// window to; zero axes are unconstrained. The limits are also tracked
// client-side (SizeLimits) so layout clamps to them when a compositor
// configures outside them anyway. A wireless window (tests) records
// the limits without sending anything.
func (w *Window) SetMinSize(width, height uint32) error {
	w.minW, w.minH = width, height
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.SetMinSize(int32(width), int32(height))
}

// SetMaxSize publishes the largest size the compositor may resize the
// window to; zero axes are unconstrained. See SetMinSize.
func (w *Window) SetMaxSize(width, height uint32) error {
	w.maxW, w.maxH = width, height
	if w.Toplevel == nil {
		return nil
	}
	return w.Toplevel.SetMaxSize(int32(width), int32(height))
}

// SizeLimits returns the current min/max size in surface (logical)
// pixels; a zero axis is unconstrained.
func (w *Window) SizeLimits() (minW, minH, maxW, maxH int) {
	return int(w.minW), int(w.minH), int(w.maxW), int(w.maxH)
}

// Resize hands the pointer grab to the compositor for an interactive
// resize: it streams configure events as the user drags, and this
// window repaints at every configured size. seat and serial come from
// the button press that started the gesture; edges is a combination of
// the xdg_toplevel.resize_edge bits. A none (zero) edges value or a
// missing seat is a no-op: there is nothing to resize with.
func (w *Window) Resize(seat *wl.Seat, serial uint32, edges uint32) error {
	if w.Toplevel == nil || seat == nil || edges == xdg.ToplevelResizeEdgeNone {
		return nil
	}
	return w.Toplevel.Resize(seat, serial, edges)
}

// ServerDecorated reports whether server-side decorations were
// requested: the compositor then owns the window's borders and resize
// handles, and the client keeps its own edge handles passive. The
// answer reflects the request; the compositor may still decide
// otherwise (mode events are not tracked).
func (w *Window) ServerDecorated() bool { return w.decoration != nil }

// SetParent parents this toplevel to another, for transient dialogs.
// A nil parent clears the association.
func (w *Window) SetParent(parent *Window) {
	if parent == nil || parent.Toplevel == nil {
		return
	}
	_ = w.Toplevel.SetParent(parent.Toplevel)
}

// HandleToplevelClose marks the window closed unless a close-request
// veto rejects it.
func (w *Window) HandleToplevelClose(xdg.ToplevelCloseEvent) {
	if w.onCloseRequest != nil && !w.onCloseRequest() {
		return
	}
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

// HostSurface returns the underlying wl_surface.
func (w *Window) HostSurface() *wl.Surface { return w.WLSurface }

// TooltipSurface returns the xdg_surface hover tooltips anchor to.
func (w *Window) TooltipSurface() *xdg.Surface { return w.XdgSurface }

// Decorate requests server-side title bars and borders through the
// xdg-decoration manager. A nil manager (compositor without the
// global) is a silent no-op: the window simply stays undecorated.
func (w *Window) Decorate(mgr *xdeco.ZxdgDecorationManagerV1) error {
	if mgr == nil {
		return nil
	}
	dec, err := mgr.GetToplevelDecoration(w.Toplevel)
	if err != nil {
		return err
	}
	w.decoration = dec
	return dec.SetMode(xdeco.ZxdgToplevelDecorationV1ModeServerSide)
}
