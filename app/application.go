// Application runs many windows — toplevels and layer surfaces — on one
// session and one parked event loop. It is gelm's counterpart of
// relm4's Application/Window model (see docs/application-model.md): the
// process owns one connection, windows are created declaratively, and
// every window joins the same parked loop with its own dirty state,
// frame pacing, and tooltip bookkeeping.
package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/internal/clipboard"
	"github.com/stubbedev/gelm/internal/layersurface"
	"github.com/stubbedev/gelm/internal/window"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// Application is a set of windows sharing one session.
type Application struct {
	sess        *wlsession.Session
	clip        *clipboard.Clipboard
	tooltipFace *render.Typeface
	onKey       func(r *widget.Router, keycode uint32, mods wlsession.Mods)

	windows []*hostWindow
	quit    bool
	rep     *keyRepeater
	kicker  *loopKicker
}

// NewApplication binds an application to a connected session.
func NewApplication(sess *wlsession.Session) *Application {
	return &Application{
		sess:   sess,
		rep:    newKeyRepeater(sess.RepeatInfo()),
		kicker: &loopKicker{},
	}
}

// SetClipboard enables ctrl+c/x/v on every window's focused widget.
func (a *Application) SetClipboard(c *clipboard.Clipboard) { a.clip = c }

// SetTooltipFace enables hover tooltips rendered with the given face.
func (a *Application) SetTooltipFace(f *render.Typeface) { a.tooltipFace = f }

// OnKey receives every key press on the focused window, after widget
// routing, for app-level keybindings.
func (a *Application) OnKey(fn func(r *widget.Router, keycode uint32, mods wlsession.Mods)) {
	a.onKey = fn
}

// Quit ends Run at the next loop check, regardless of open windows.
func (a *Application) Quit() { a.quit = true }

// WindowConfig declares a toplevel window.
type WindowConfig struct {
	// Title and AppID identify the window to the compositor.
	Title, AppID string
	// Width and Height request the initial size; zero lets the
	// compositor pick.
	Width, Height uint32
	// Scale is the integer output scale the surface renders at; zero
	// means 1.
	Scale int
	// Root is the window's widget tree.
	Root widget.Widget
	// Background fills the frame before the tree paints.
	Background render.Color
	// OnPress fires after the router recorded a press (chrome drag,
	// context menus).
	OnPress func(button uint32, serial uint32, over widget.Widget)
	// OnPointerMove receives raw pointer positions in surface pixels.
	OnPointerMove func(x, y float64)
	// OnKey receives every key press routed to this window, after
	// widget routing, for app-level keybindings.
	OnKey func(r *widget.Router, keycode uint32, mods wlsession.Mods)
	// OnClosed runs when the window closed (compositor request accepted,
	// client Close, or output gone).
	OnClosed func()
}

// LayerConfig declares a layer surface: the bar/panel/launcher shape.
// Anchors, margins, exclusive zone, and keyboard interactivity are
// declarative — set once at creation, per gtk4-layer-shell's model.
type LayerConfig struct {
	// Output pins the surface to one output; nil lets the compositor
	// choose.
	Output *wlsession.Output
	// Layer selects the stack layer (background through overlay).
	Layer layersurface.Layer
	// Anchor selects the anchored edges.
	Anchor layersurface.Anchor
	// Margin distances the surface from the anchored edges.
	Margin layersurface.Margins
	// Width and Height: an auto (zero) axis needs both edges of that
	// axis anchored.
	Width, Height uint32
	// ExclusiveZone reserves space along anchored edges; negative
	// values distance instead.
	ExclusiveZone int32
	// Keyboard selects keyboard interactivity (none, exclusive,
	// on-demand) — a launcher overlay grabs with exclusive.
	Keyboard layersurface.KeyboardMode
	// Namespace tags the surface for compositor-side rules.
	Namespace string
	// Scale is the integer output scale; zero derives it from Output.
	Scale int
	// Root is the window's widget tree.
	Root widget.Widget
	// Background fills the frame before the tree paints.
	Background render.Color
	// OnPress, OnPointerMove, OnKey mirror WindowConfig.
	OnPress       func(button uint32, serial uint32, over widget.Widget)
	OnPointerMove func(x, y float64)
	OnKey         func(r *widget.Router, keycode uint32, mods wlsession.Mods)
	// OnClosed runs when the surface closed or its output went away.
	OnClosed func()
}

// NewWindow creates a toplevel window from a declarative config and
// adds it to the application. The window joins the loop on the next
// Run iteration (or immediately when Run is already running).
func (a *Application) NewWindow(cfg WindowConfig) (*Window, error) {
	if a.sess.WmBase() == nil {
		return nil, errors.New("app: compositor has no xdg_wm_base; windows unsupported")
	}
	surf, err := a.sess.Compositor().CreateSurface()
	if err != nil {
		return nil, fmt.Errorf("app: create surface: %w", err)
	}
	win, err := window.New(a.sess.WmBase(), surf, window.Config{
		Title: cfg.Title, AppID: cfg.AppID, Width: cfg.Width, Height: cfg.Height,
	})
	if err != nil {
		return nil, err
	}
	scale := cfg.Scale
	if scale == 0 {
		scale = 1
	}
	a.newWindow(win, scale, cfg.Root, windowHooks{
		background: cfg.Background,
		onPress:    cfg.OnPress,
		onMove:     cfg.OnPointerMove,
		onKey:      cfg.OnKey,
	}, cfg.OnClosed)
	if err := surf.Commit(); err != nil {
		return nil, fmt.Errorf("app: initial commit: %w", err)
	}
	return &Window{app: a, win: win}, nil
}

// NewLayer creates a layer surface from a declarative config — the
// bar/panel/launcher shape: anchors, margins, exclusive zone, and
// keyboard interactivity declared up front.
func (a *Application) NewLayer(cfg LayerConfig) (*LayerWindow, error) {
	shell := a.sess.LayerShell()
	if shell == nil {
		return nil, errors.New("app: compositor has no zwlr_layer_shell_v1")
	}
	surf, err := a.sess.Compositor().CreateSurface()
	if err != nil {
		return nil, fmt.Errorf("app: create surface: %w", err)
	}
	scale := cfg.Scale
	if scale == 0 && cfg.Output != nil {
		scale = cfg.Output.Scale
	}
	if scale == 0 {
		scale = 1
	}
	ls, err := layersurface.New(shell, surf, outputWire(cfg.Output), layersurface.Config{
		Layer:         cfg.Layer,
		Anchor:        cfg.Anchor,
		Width:         cfg.Width,
		Height:        cfg.Height,
		Margin:        cfg.Margin,
		ExclusiveZone: cfg.ExclusiveZone,
		Keyboard:      cfg.Keyboard,
		Namespace:     cfg.Namespace,
	})
	if err != nil {
		return nil, err
	}
	a.newWindow(&layerHost{ls: ls, out: cfg.Output}, scale, cfg.Root, windowHooks{
		background: cfg.Background,
		onPress:    cfg.OnPress,
		onMove:     cfg.OnPointerMove,
		onKey:      cfg.OnKey,
	}, cfg.OnClosed)
	if err := surf.Commit(); err != nil {
		return nil, fmt.Errorf("app: initial commit: %w", err)
	}
	return &LayerWindow{app: a, ls: ls}, nil
}

// layerHost adapts a layer surface to Host: for automatic (zero) axes
// it falls back to the output mode, which is what anchoring both edges
// of an axis means on the wire.
type layerHost struct {
	ls  *layersurface.Surface
	out *wlsession.Output
}

func (h *layerHost) EnsureUsable() error      { return h.ls.EnsureUsable() }
func (h *layerHost) Closed() bool             { return h.ls.Closed() }
func (h *layerHost) HostSurface() *wl.Surface { return h.ls.HostSurface() }

// Size returns the configured size, substituting the output mode for
// automatic axes so the buffer is never zero-sized.
func (h *layerHost) Size() (int, int) {
	w, ht := h.ls.Size()
	if w == 0 && h.out != nil {
		w = h.out.ModeW
	}
	if ht == 0 && h.out != nil {
		ht = h.out.ModeH
	}
	return w, ht
}

// newWindow builds the loop state for one created host and joins it to
// the application. Lifecycle is owned by the loop from here on.
func (a *Application) newWindow(host Host, scale int, root widget.Widget, hooks windowHooks, onClosed func()) {
	hooks.onClosed = onClosed
	w := newHostWindow(a.sess, host, scale, root, hooks)
	a.windows = append(a.windows, w)
	// Wake the parked loop so a new window paints promptly.
	a.sess.WakeAfter(0)
}

// outputWire maps an application-level output to its wl_output; nil
// lets the compositor choose.
func outputWire(o *wlsession.Output) *wl.Output {
	if o == nil {
		return nil
	}
	return o.WL
}

// Run drives every window until the application quits (Quit) or every
// window has closed. It returns ErrClosed in both cases.
func (a *Application) Run() error {
	a.sess.OnKey = a.routeKey
	a.sess.OnKeyUp = a.rep.release
	for {
		if a.quit || len(a.windows) == 0 {
			return ErrClosed
		}
		now := time.Now()

		if code, mods, ok := a.rep.tick(); ok {
			a.deliverKey(code, mods)
		}
		if anim.Active() {
			anim.Tick(now)
			for _, w := range a.windows {
				w.dirty = true
			}
		}
		a.updateTips(now)

		kept := a.windows[:0]
		for _, w := range a.windows {
			if w.host.Closed() {
				w.release()
				if w.cfg.onClosed != nil {
					w.cfg.onClosed()
				}
				continue
			}
			kept = append(kept, w)
		}
		a.windows = kept
		if a.quit || len(a.windows) == 0 {
			return ErrClosed
		}

		for _, w := range a.windows {
			if w.frameReady {
				w.frameReady = false
				w.framePending = false
			}
			if w.host.EnsureUsable() != nil {
				// Not configured yet; the configure event wakes the park.
				continue
			}
			// Draw when something changed and pacing allows: either
			// the previous frame's callback returned, or an animation
			// keeps producing frames even when the compositor stopped
			// scheduling them (occlusion). An input-only redraw always
			// goes out; if a frame is still pending the loop waits for
			// its event.
			if w.dirty && (!w.framePending || anim.Active()) {
				w.dirty = false
				if !w.draw() {
					return w.drawErr
				}
			}
		}
		if a.quit {
			return ErrClosed
		}

		var tipNext, repNext, animEnd time.Time
		for _, w := range a.windows {
			if t, ok := w.tip.next(); ok && (tipNext.IsZero() || t.Before(tipNext)) {
				tipNext = t
			}
		}
		if t, ok := a.rep.nextDeadline(); ok {
			repNext = t
		}
		if t, ok := anim.Next(); ok {
			animEnd = t
		}
		wakeAt, ok := nextWake(repNext, animEnd, tipNext, now)
		if ok {
			a.kicker.schedule(a.sess, wakeAt)
		}
		if err := a.sess.Step(); err != nil {
			return fmt.Errorf("app: dispatch: %w", err)
		}
	}
}

// updateTips advances every window's tooltip state machine.
func (a *Application) updateTips(now time.Time) {
	for _, w := range a.windows {
		w.tip.update(w.router, now, func(h widget.Widget, text string) tooltipWindow {
			if a.tooltipFace == nil {
				return nil
			}
			return openTooltip(w.sess, w.host, &Config{
				Session:     w.sess,
				Host:        w.host,
				Scale:       w.input.scale,
				TooltipFace: a.tooltipFace,
			}, int(w.input.x), int(w.input.y), text)
		})
	}
}

// routeKey dispatches one key press to the window holding keyboard
// focus, falling back to the first window.
func (a *Application) routeKey(keycode uint32, mods wlsession.Mods) {
	a.rep.press(keycode, mods)
	a.deliverKey(keycode, mods)
	for _, w := range a.windows {
		w.dirty = true
	}
}

// deliverKey routes one key press through the focused window's router.
func (a *Application) deliverKey(keycode uint32, mods wlsession.Mods) {
	target := a.focused()
	if target == nil {
		return
	}
	routeKey(a.sess, target.router, keycode, mods, a.clip, a.onKey)
}

// focused picks the window that should receive keyboard input: the one
// under the compositor's keyboard focus, else the first window.
func (a *Application) focused() *hostWindow {
	focus := a.sess.KeyboardFocus()
	if focus != nil {
		for _, w := range a.windows {
			if w.host.HostSurface() == focus {
				return w
			}
		}
	}
	if len(a.windows) > 0 {
		return a.windows[0]
	}
	return nil
}

// Window is the application's handle on one toplevel window.
type Window struct {
	app *Application
	win *window.Window
}

// Close closes the window from the client side; the close-request veto
// does not apply to explicit closes.
func (w *Window) Close() { w.win.Close() }

// Closed reports whether the window closed.
func (w *Window) Closed() bool { return w.win.Closed() }

// SetCloseRequest installs a veto: return false to keep the window
// open when the compositor asks it to close.
func (w *Window) SetCloseRequest(veto func() bool) { w.win.SetCloseRequest(veto) }

// LayerWindow is the application's handle on one layer surface.
type LayerWindow struct {
	app *Application
	ls  *layersurface.Surface
}

// Close destroys the layer surface from the client side.
func (l *LayerWindow) Close() { l.ls.Close() }

// Closed reports whether the compositor or client closed the surface.
func (l *LayerWindow) Closed() bool { return l.ls.Closed() }
