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
	"image"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/internal/appearance"
	"github.com/stubbedev/gelm/internal/clipboard"
	"github.com/stubbedev/gelm/internal/datacontrol"
	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/dragdrop"
	"github.com/stubbedev/gelm/internal/icons"
	"github.com/stubbedev/gelm/internal/inspect"
	"github.com/stubbedev/gelm/internal/layersurface"
	"github.com/stubbedev/gelm/internal/notify"
	"github.com/stubbedev/gelm/internal/popup"
	"github.com/stubbedev/gelm/internal/recentfiles"
	"github.com/stubbedev/gelm/internal/surfx"
	"github.com/stubbedev/gelm/internal/window"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// Application is a set of windows sharing one session.
type Application struct {
	sess        *wlsession.Session
	tooltipFace *render.Typeface
	// tooltipOpts tunes tooltips (SetTooltipOptions).
	tooltipOpts TooltipOptions
	// toastHost is the default toast window (SetToastHost).
	toastHost Host
	onKey     func(r *widget.Router, keycode uint32, mods wlsession.Mods)
	// onDisconnect is the compositor-disconnect policy hook; see
	// app/disconnect.go. disconnectOnce makes it fire exactly once, and
	// step is the dispatch seam (sess.Step in production) tests drive
	// without a compositor.
	onDisconnect   func(DisconnectedEvent)
	disconnectOnce atomic.Bool
	step           func() error
	// accels is the accelerator table: named actions plus keysym+mods
	// bindings, consulted by routeKey before text routing.
	accels *accelTable
	ime    *imeController
	// clip enables ctrl+c/x/v via routeKey; primary carries the
	// primary-selection behavior (middle-click paste, copy-on-select).
	// SetClipboard fills both from one clipboard.
	clip    *clipboard.Clipboard
	primary *primarySelection
	// dnd drives drag-and-drop for every window on this application;
	// inert when the compositor lacks a data device.
	dnd *dragdrop.Controller
	// onTransferError hears failed pastes and drops
	// (SetTransferErrorHandler).
	onTransferError func(error)
	// dataControl is the lazily bound data-control device
	// (datacontrol.go); nil until DataControl first succeeds.
	dataControl *datacontrol.Device

	// inspector state: armed enables the chords (ctrl+shift+i/d), on
	// paints the widget-tree overlay. Armed by GELM_INSPECT=1 or an
	// explicit SetInspect/ToggleInspect; see inspect.go.
	inspectArmed bool
	inspectOn    bool

	windows  []*hostWindow
	dialogs  []*Dialog
	popovers popoverRegistry
	// openPopovers are the popovers the loop drives, oldest first.
	openPopovers []*openPopover
	// toasts tracks each window's toast stack (see toast.go).
	toasts toastRegistry
	// appearance follows the portal's live icon-theme setting (#64) so
	// themed icons re-resolve without an app restart; stopIconFollow
	// unwires at Run's exit. The monitor is icon-lookup plumbing, not a
	// palette swap: the color-scheme preference stays the app's to wire
	// (docs/appearance.md).
	appearance     *appearance.Monitor
	stopIconFollow func()
	// windowIcons (windowicon.go): per-window overrides and the app
	// default posted through xdg-toplevel-icon-v1, plus the icons
	// currently applied to live host windows.
	windowIcons  map[*Window]*postedIcon
	appliedIcons map[*hostWindow]*postedIcon
	defaultIcon  *postedIcon
	// defaultIconSrc and windowIconSrc are the images behind them, what
	// a reconnect posts again.
	defaultIconSrc image.Image
	windowIconSrc  map[*Window]image.Image
	// stopA11y stops the AT-SPI bridge while it serves; stopA11yWatch
	// ends the A11yAuto watch on the desktop's switch; a11yMode is
	// SetAccessibility's choice.
	stopA11y      func()
	stopA11yWatch func()
	a11yMode      A11yMode
	// reconnect is SetReconnect's policy (nil: clean exit);
	// rebuildPlan holds the windows a disconnect planned to rebuild.
	reconnect   *ReconnectOptions
	rebuildPlan []*hostWindow
	// carriedToasts are the planned windows' toasts, set aside for the
	// rebuilt windows.
	carriedToasts map[*hostWindow]movedToasts
	quit          bool
	// recentFiles (filedialog.go) lazily owns the desktop's shared
	// recently-used list; nil until a file dialog with Recents runs.
	recentFiles *recentfiles.Manager
	// palette (dialog.go) is this application's color-chooser picks,
	// scoped per process instead of a process global (#84).
	palette appPalette
	// eyedrop (eyedropper.go): the once-checked screencopy
	// availability for the color chooser's pick-from-screen button.
	eyedropChecked bool
	eyedropOK      bool
	// notify (notification.go): the once-started desktop notifier and
	// the per-notification callback table; the connection dies with
	// the loop.
	notifyOnce  sync.Once
	notifyMu    sync.Mutex
	notifyStore *notify.Notifier
	notifyOpts  map[string]NotifyOptions
	// launcher (launcher.go): the OpenURL/OpenPath transport state.
	launchState launcher
	// shortcuts (globalshortcuts_portal.go): the lazily created portal
	// GlobalShortcuts session, the fallback transport.
	shortcuts portalShortcuts
	// sessionLock is the lock this application holds (sessionlock.go);
	// while it exists the loop runs on with no window mapped.
	sessionLock *SessionLock
	// holds counts unreleased Holds; while any is held the loop runs on
	// with no window mapped.
	holds  int
	rep    *keyRepeater
	kicker *loopKicker
	// queues is the Invoke/Every plumbing and wake the loop-kick call;
	// wake is a field so tests can drive Invoke without a session.
	queues loopQueues
	wake   func(time.Duration)
	// watchers are the loop-owned stoppables: the WatchFD and WatchFiles
	// pollers and the typed messengers of message.go, all stopped with
	// the loop; ended closes when the loop ends, releasing a poller
	// waiting on a dropped invoke.
	watchers  stopSet
	endedOnce sync.Once
	ended     chan struct{}
	endedMu   sync.Mutex
}

// IdleInhibitAvailable reports whether the compositor supports the
// idle-inhibit protocol; false on compositors without it.
func (a *Application) IdleInhibitAvailable() bool { return a.sess.IdleInhibitAvailable() }

// InhibitIdle holds the compositor's idle and suspend for as long as
// the returned release function goes uncalled: while a media player
// runs or a download is in flight, the screen stays on. The inhibitor
// binds to the host's surface. Fails when the protocol is
// unavailable.
func (a *Application) InhibitIdle(host Host) (func(), error) {
	inhibitor, err := a.sess.InhibitIdle(host.HostSurface())
	if err != nil {
		return nil, err
	}
	return inhibitor.Destroy, nil
}

// LastPressSerial returns the wl pointer serial of the most recent
// button press on the host, for xdg_popup.grab. Zero when the host is
// unknown or nothing was pressed yet — callers opening popovers from
// a click always run after a press, so the zero case is headless only.
func (a *Application) LastPressSerial(host Host) uint32 {
	for _, win := range a.windows {
		if win.host != host || win.input == nil {
			continue
		}
		return win.input.pressSerial
	}
	return 0
}

// NewApplication binds an application to a connected session.
func NewApplication(sess *wlsession.Session) *Application {
	a := &Application{
		sess:         sess,
		accels:       newAccelTable(),
		primary:      &primarySelection{},
		dnd:          dragdrop.New(sess),
		rep:          newKeyRepeater(sess.RepeatInfo()),
		kicker:       &loopKicker{},
		ime:          newIMEController(sess),
		wake:         sess.WakeAfter,
		step:         sess.Step,
		windowIcons:  make(map[*Window]*postedIcon),
		appliedIcons: make(map[*hostWindow]*postedIcon),
	}
	// Async image loads (widget.Image file/URL sources) deliver through
	// the loop queue - the only sanctioned bridge (docs/threading.md).
	widget.SetInvoker(a.Invoke)
	// Stylesheet files hot-reload through the loop's timer wheel: the
	// widget package cannot reach it, so the app lends it a scheduler —
	// one stat per second once a file is loaded, nothing before.
	widget.SetStylesheetPoller(func(fn func()) { a.Every(time.Second, fn) })
	// Tree mutations (Box.Remove/RemoveAt/Clear/InsertAt, Stack.Remove,
	// Scroll.SetChild, ...) must not leave a window's router pointing at
	// a detached widget: the hook drops hover, press, focus, and
	// drop-target state for the removed subtree, so no KeyAction reaches
	// a dead widget — and, through router.Hovered(), no tooltip dwells
	// on a ghost (tooltipCtl opens and closes on the router's hover).
	// One hook per process, like SetInvoker; it covers the window
	// routers — popups run transient routers over short-lived trees.
	// Rejected input (a key into a read-only field, a value that does
	// not parse) rings the system bell for the widget's window.
	widget.SetErrorBell(a.ringFor)
	widget.SetRemovedHook(func(w widget.Widget) {
		for _, win := range a.windows {
			win.router.Forget(w)
		}
	})
	// Icon lookups follow the portal's live icon-theme setting: a
	// switch swaps the resolution theme (an empty name keeps the
	// previous one, Warn) and the cache generation moves, so live
	// themed icons re-resolve. Any cache reset (that switch, new search
	// paths, a refresh) is bridged into the loop as the repaint that
	// walks the damage and picks the change up.
	a.appearance = appearance.New()
	a.stopIconFollow = a.appearance.OnIconThemeChange(icons.Default().ApplyIconTheme)
	a.repaintOnIconReset(icons.Default())
	if inspect.Enabled() {
		a.setInspect(true)
	}
	if doctorRequested() {
		fmt.Fprint(os.Stdout, inspect.Doctor(sess, inspect.DoctorOptions{}))
	}
	return a
}

// Clipboard returns the clipboard SetClipboard installed, nil before.
// Its methods touch the Wayland connection: call them on the loop
// goroutine (inside Invoke from elsewhere).
func (a *Application) Clipboard() *Clipboard { return a.clip }

// SetClipboard enables ctrl+c/x/v on every window's focused widget,
// plus the primary-selection behavior (middle-click paste, and
// copy-on-select once SetCopyOnSelect turns it on).
func (a *Application) SetClipboard(c *clipboard.Clipboard) {
	a.clip = c
	a.primary.src = c
}

// SetCopyOnSelect turns X11-style copy-on-select on or off; a finished
// selection claims the primary selection, so a middle click pastes it.
// Default off. Needs a configured clipboard.
func (a *Application) SetCopyOnSelect(on bool) { a.primary.copyOnSelect = on }

// SetTooltipFace enables hover tooltips rendered with the given face.
func (a *Application) SetTooltipFace(f *render.Typeface) { a.tooltipFace = f }

// OnKey observes every key press on the focused window, after widget
// routing and accelerators, for app-level keybindings. It still sees
// keys an accelerator consumed; accelerators only suppress text
// routing and widget actions.
func (a *Application) OnKey(fn func(r *widget.Router, keycode uint32, mods wlsession.Mods)) {
	a.onKey = fn
}

// Quit ends Run at the next loop check, regardless of open windows.
func (a *Application) Quit() { a.quit = true }

// done reports whether Run should return: Quit was called, or every
// window closed with no session lock and no Hold held. A held lock
// keeps the loop alive with nothing mapped — its last output may be
// unplugged, and only this process can unlock the session.
func (a *Application) done() bool {
	return a.quit || (len(a.windows) == 0 && a.sessionLock == nil && a.holds == 0)
}

// Hold keeps Run going with no window mapped, for an application whose
// work is not on screen: a portal serving the clipboard over the
// data-control device, a daemon that maps surfaces only on request.
// The returned release ends the hold (once; later calls do nothing),
// and Run returns at its next check when nothing else keeps it alive.
// Call both on the loop goroutine, or before Run.
func (a *Application) Hold() (release func()) {
	a.holds++
	released := false
	return func() {
		if released {
			return
		}
		released = true
		a.holds--
	}
}

// WindowConfig declares a toplevel window.
type WindowConfig struct {
	// Title and AppID identify the window to the compositor.
	Title, AppID string
	// Width and Height request the initial size in logical pixels
	// (surface coordinates; the device scale is applied once at the
	// buffer boundary); zero lets the compositor pick.
	Width, Height uint32
	// MinWidth and MinHeight constrain how far the compositor may
	// resize the window down (set_min_size on the wire); zero axes are
	// unconstrained. Layout clamps to the same limits client-side.
	MinWidth, MinHeight uint32
	// MaxWidth and MaxHeight constrain how far the compositor may
	// resize the window up (set_max_size); zero axes are unconstrained,
	// in logical pixels like every size in the toolkit.
	MaxWidth, MaxHeight uint32
	// Scale is the initial integer output scale the surface renders at;
	// zero means 1. With the fractional-scale protocol the compositor's
	// preferred scale (1.25 and friends) overrides this live.
	Scale int
	// Root is the window's widget tree.
	Root widget.Widget
	// Background fills the frame before the tree paints. An alpha of
	// 255 sets wl_surface.set_opaque_region automatically; an alpha
	// below 255 leaves the surface translucent for compositor blur
	// (panels: alpha 200-235 - docs/architecture.md, rule 13).
	Background render.Color
	// Opaque forces the opaque region even though Background is
	// translucent; see app.Config.Opaque.
	Opaque bool
	// ContentType hints what the window shows (a photo viewer, a video
	// player, a game) so the compositor may tune for it; ignored where
	// the compositor lacks wp_content_type_v1.
	ContentType ContentType
	// Parent, when set, makes the window transient for it from the
	// first commit (SetTransientFor): a toolbox above its owner.
	Parent *Window
	// OnPress fires after the router recorded a press (chrome drag,
	// context menus).
	OnPress func(button uint32, serial uint32, over widget.Widget)
	// OnPointerMove receives raw pointer positions in surface pixels.
	OnPointerMove func(x, y float64)
	// OnKey receives every key press routed to this window, after
	// widget routing, for app-level keybindings.
	OnKey func(r *widget.Router, keycode uint32, mods wlsession.Mods)
	// KeyCapture sees every key press routed to this window (repeats
	// included) before anything else does: built-in widget handling,
	// accelerators, text routing, and OnKey. Returning true consumes
	// the press. It is GTK's capture phase - a launcher's bound keys
	// (Return, Tab, Up) act on its list while every other key still
	// types into the focused entry. The Accel carries the keysym
	// normalized as accelerators are and the held modifiers with caps
	// lock masked out, so it compares equal to ParseAccel of a
	// binding's name.
	KeyCapture func(Accel) bool
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
	// Width and Height: the surface size in logical pixels (the device
	// scale is applied once at the buffer boundary). An auto (zero)
	// axis needs both edges of that axis anchored.
	Width, Height uint32
	// ExclusiveZone reserves space along anchored edges; negative
	// values distance instead.
	ExclusiveZone int32
	// Keyboard selects keyboard interactivity (none, exclusive,
	// on-demand) — a launcher overlay grabs with exclusive.
	Keyboard layersurface.KeyboardMode
	// Namespace tags the surface for compositor-side rules.
	Namespace string
	// Scale is the initial integer output scale; zero derives it from
	// Output, then 1. With the fractional-scale protocol the
	// compositor's preferred scale overrides this live.
	Scale int
	// Root is the window's widget tree.
	Root widget.Widget
	// Background fills the frame before the tree paints. An alpha of
	// 255 sets wl_surface.set_opaque_region automatically; an alpha
	// below 255 leaves the surface translucent for compositor blur
	// (panels: alpha 200-235 - docs/architecture.md, rule 13).
	Background render.Color
	// Opaque forces the opaque region even though Background is
	// translucent; see app.Config.Opaque.
	Opaque bool
	// ContentType mirrors WindowConfig.
	ContentType ContentType
	// OnPress, OnPointerMove, OnKey, KeyCapture mirror WindowConfig.
	OnPress       func(button uint32, serial uint32, over widget.Widget)
	OnPointerMove func(x, y float64)
	OnKey         func(r *widget.Router, keycode uint32, mods wlsession.Mods)
	KeyCapture    func(Accel) bool
	// OnClosed runs when the surface closed or its output went away.
	OnClosed func()
}

// NewWindow creates a toplevel window from a declarative config and
// adds it to the application. The window joins the loop on the next
// Run iteration (or immediately when Run is already running).
func (a *Application) NewWindow(cfg WindowConfig) (*Window, error) {
	return a.newWindowWindow(cfg, surfx.KindMenu)
}

// newWindowWindow creates the toplevel and its loop state. animKind
// is the surface-animation profile: KindMenu (zero) for plain
// toplevels, which open and close instantly.
func (a *Application) newWindowWindow(cfg WindowConfig, animKind surfx.Kind) (*Window, error) {
	w := &Window{app: a, cfg: cfg, kind: animKind}
	if _, err := a.openWindow(w, cfg); err != nil {
		return nil, err
	}
	return w, nil
}

// openWindow gives handle w a toplevel on the current session, built
// from cfg, with its loop state - the first open, and the rebuild on a
// new connection, which keeps every *Window an app holds valid.
func (a *Application) openWindow(w *Window, cfg WindowConfig) (*hostWindow, error) {
	if a.sess.WmBase() == nil {
		return nil, errors.New("app: compositor has no xdg_wm_base; windows unsupported")
	}
	surf, err := a.sess.Compositor().CreateSurface()
	if err != nil {
		return nil, fmt.Errorf("app: create surface: %w", err)
	}
	win, err := window.New(a.sess.WmBase(), surf, window.Config{
		Title: cfg.Title, AppID: cfg.AppID, Width: cfg.Width, Height: cfg.Height,
		MinWidth: cfg.MinWidth, MinHeight: cfg.MinHeight,
		MaxWidth: cfg.MaxWidth, MaxHeight: cfg.MaxHeight,
	})
	if err != nil {
		return nil, err
	}
	w.win = win
	hw := a.newWindow(win, max(cfg.Scale, 1), cfg.Root, windowHooks{
		background: cfg.Background,
		opaque:     opaqueFor(cfg.Background, cfg.Opaque),
		onPress:    cfg.OnPress,
		onMove:     cfg.OnPointerMove,
		onKey:      cfg.OnKey,
		keyCapture: cfg.KeyCapture,
	}, cfg.OnClosed, w.kind)
	hw.win = w
	if cfg.Parent != nil {
		w.SetTransientFor(cfg.Parent)
	}
	a.initialHints(surf, cfg.ContentType)
	if err := surf.Commit(); err != nil {
		return nil, fmt.Errorf("app: initial commit: %w", err)
	}
	return hw, nil
}

// NewLayer creates a layer surface from a declarative config — the
// bar/panel/launcher shape: anchors, margins, exclusive zone, and
// keyboard interactivity declared up front.
func (a *Application) NewLayer(cfg LayerConfig) (*LayerWindow, error) {
	l := &LayerWindow{app: a, cfg: cfg}
	if _, err := a.openLayer(l, cfg); err != nil {
		return nil, err
	}
	return l, nil
}

// openLayer gives handle l a layer surface on the current session,
// built from cfg - the first open and the rebuild, like openWindow.
func (a *Application) openLayer(l *LayerWindow, cfg LayerConfig) (*hostWindow, error) {
	if a.sess.LayerShell() == nil {
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
	ls, err := layersurface.New(a.sess, surf, outputWire(cfg.Output), layersurface.Config{
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
	l.ls = ls
	hw := a.newWindow(&layerHost{ls: ls, out: cfg.Output}, max(scale, 1), cfg.Root, windowHooks{
		background: cfg.Background,
		opaque:     opaqueFor(cfg.Background, cfg.Opaque),
		onPress:    cfg.OnPress,
		onMove:     cfg.OnPointerMove,
		onKey:      cfg.OnKey,
		keyCapture: cfg.KeyCapture,
	}, cfg.OnClosed, surfx.KindOverlay)
	hw.layer = l
	// A rotated output's transform must be published before the first
	// commit so the compositor maps the buffers correctly.
	if cfg.Output != nil && cfg.Output.Transform != 0 && hw.sc != nil {
		_ = hw.sc.SetTransform(cfg.Output.Transform)
	}
	a.initialHints(surf, cfg.ContentType)
	if err := surf.Commit(); err != nil {
		return nil, fmt.Errorf("app: initial commit: %w", err)
	}
	return hw, nil
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
// the application. animKind picks the surface-animation profile:
// surfx.KindMenu (the zero) means no window-level animation — plain
// toplevels open and close instantly — while KindOverlay (layer
// surfaces) and KindDialog get enter/exit tweens whose exit keeps the
// loop alive until it lands.
func (a *Application) newWindow(host Host, scale int, root widget.Widget, hooks windowHooks, onClosed func(), animKind surfx.Kind) *hostWindow {
	hooks.onClosed = onClosed
	// Every window's tree sits behind the inspector overlay. While it
	// is off the overlay is pure passthrough (measure, arrange, hit
	// test, damage), so wrapping costs nothing visible; toggling it on
	// never changes layout or which widget input lands on.
	// The tree styles as its own root: the overlay and the fader above
	// it are layout plumbing, never a style ancestor.
	widget.SetStyleRoot(root)
	ov := inspect.NewOverlay(root)
	ov.SetOn(a.inspectOn)
	w := newHostWindow(a.sess, host, scale, ov, hooks, a.dnd, a.primary, animKind, hostCloser(host))
	if w.input != nil {
		w.input.transferError = a.reportTransfer
	}
	ov.SetRouter(w.router)
	w.inspector = ov
	a.windows = append(a.windows, w)
	// Wake the parked loop so a new window paints promptly.
	a.sess.WakeAfter(0)
	return w
}

// hostCloser returns the raw wire teardown for a host: the layer
// surface's or the xdg toplevel's destroy, without the app-level
// Window handle's animated-close logic (which would recurse).
func hostCloser(host Host) func() {
	switch h := host.(type) {
	case *layerHost:
		return h.ls.Close
	case *window.Window:
		return h.Close
	case *lockHost:
		return h.s.Close
	}
	return nil
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
// window has closed with no session lock held (a lock keeps the loop
// alive with nothing mapped). It returns ErrClosed in both cases. Run's
// goroutine is the event-loop goroutine of the threading contract
// (docs/threading.md): widget and callback work happens here, and
// other goroutines reach it only through Invoke and Every. Run marks
// the goroutine for the off-loop debug hook, pumps the queued work at
// the top of every pass, and folds the periodic-timer deadlines into
// the wake computation, so a pending poller wakes the park exactly
// once per tick.
func (a *Application) Run() error {
	widget.MarkLoop()
	defer func() {
		// Every exit but a dead connection settles the wire first:
		// requests the loop's last pass made (an unlock, a destroy)
		// must reach the compositor before the process can exit.
		if !a.disconnectOnce.Load() {
			a.settleWire()
		}
		widget.UnmarkLoop()
		// Discard queued invokes and stop the pollers: nothing will run
		// them, and later Invokes drop instead of accumulating. This runs
		// on the disconnect path too — timers and invokes die with the
		// loop, whatever killed it.
		a.queues.shutdown()
		a.watchers.shutdown()
		a.endLoop()
		// The AT-SPI bridge dies with the loop: its samplers hop
		// through Invoke, which shutdown just drained.
		if a.stopA11yWatch != nil {
			a.stopA11yWatch()
		}
		a.stopAccessibility()
		// Unwind the NewApplication icon-follow wiring: stop the cache's
		// subscription, then the monitor's goroutines and connection.
		if a.stopIconFollow != nil {
			a.stopIconFollow()
		}
		if a.appearance != nil {
			a.appearance.Close()
		}
		a.closeNotifier()
		a.shortcuts.shutdown()
	}()
	a.startAccessibility()
	a.wireSession()
	for {
		if a.done() {
			return ErrClosed
		}
		if err := a.tick(a.stepFn(), time.Now()); err != nil {
			// A planned reconnect (SetReconnect) rebuilds on a new
			// session and the loop goes on.
			if a.reconnectAfter(err) {
				continue
			}
			return err
		}
	}
}

// settleWire round-trips the display once as Run ends, so the
// compositor has processed every request the application made before
// the process can exit: libwayland-server destroys a client that hangs
// up without reading what is still in its socket, and an unlock that
// ended the loop (the last lock, no window left) would otherwise be
// lost - leaving the session locked for good, as the protocol demands
// of a lock client that vanished. A closed session has nothing to
// settle.
func (a *Application) settleWire() {
	if a.sess == nil || a.sess.Display == nil || a.sess.Closed() {
		return
	}
	if err := a.sess.Roundtrip(); err != nil {
		debug.Log("wire", "app: final roundtrip: %v", err)
		return
	}
	debug.Log("wire", "app: final roundtrip done")
}

// wireSession installs the application's hooks on its session - at
// Run, and again on each session a reconnect adopts.
func (a *Application) wireSession() {
	a.sess.OnKey = a.routeKey
	a.sess.OnKeyUp = a.rep.release
	a.sess.OnIME = a.imeEvent
	a.sess.OnIMEFocus = a.ime.reset
}

// stepFn is the dispatch step: the test seam when set, else the
// current session's.
func (a *Application) stepFn() func() error {
	if a.step != nil {
		return a.step
	}
	return a.sess.Step
}

// tick runs one loop pass: queued work, repeats, IME sync, animation
// ticks, tooltip state, window draws (paced), wake computation, and
// one dispatch. A non-nil error ends the loop; ErrClosed comes back
// when the application quit or its last window closed mid-pass.
func (a *Application) tick(step func() error, now time.Time) error {
	// Off-loop work queued since the last pass: Invoke fns first,
	// then the periodic timers (pollers) that came due. Everything
	// here runs on this goroutine, the loop goroutine. Work that ran
	// owes every surface a damage check; a window whose tree drained
	// nothing skips its draw.
	worked := a.pump(now)
	if worked {
		for _, w := range a.windows {
			w.dirty = true
		}
	}

	if code, mods, ok := a.rep.tick(); ok {
		a.deliverKey(code, mods)
		// Repeat keys are deliveries too: keep the state fresh.
	}
	// Push the focused widget's text-input state; this is what
	// enables the input method on an editable focus, updates the
	// surrounding text and caret as it types, and disables on
	// focus moving elsewhere. The previous dispatch's changes are
	// picked up here, one event later.
	if r := a.keyboardRouter(); r != nil {
		a.ime.sync(r, true)
	}
	animating := anim.Active()
	if animating {
		// The animation clock ticks on loop wakes: frame callbacks
		// pace it when the compositor answers, the timer Next()
		// schedules pace it when they stop (occlusion). A tick that
		// ran callbacks invalidated widgets, so only then is a
		// frame owed.
		if anim.Tick(now) {
			for _, w := range a.windows {
				w.dirty = true
			}
		}
	}
	a.updateTips(now)

	kept := a.windows[:0]
	var moved []movedToasts
	for _, w := range a.windows {
		// A tween-frame kick from another loop's tick (a popup's
		// nested Run) marks the window for repaint.
		if w.kick.Swap(false) {
			w.dirty = true
		}
		if w.host.Closed() {
			// Its live toasts move to the next toast window once the
			// list is settled; the surface they painted on goes.
			if m, ok := a.toasts.takeHost(w.host); ok {
				moved = append(moved, m)
			}
			w.release()
			if w.cfg.onClosed != nil {
				w.cfg.onClosed()
			}
			continue
		}
		kept = append(kept, w)
	}
	a.windows = kept
	a.rehostToasts(moved)
	a.reapWindowIcons()
	if a.done() {
		return ErrClosed
	}

	for _, w := range a.windows {
		// Pick up a configure-driven size change even when nothing
		// else is dirty: syncSize resizes the pool and schedules the
		// repaint, so the frame below already runs at the new size.
		resized := w.syncSize()
		if w.frameReady {
			w.frameReady = false
			w.framePending = false
		}
		if w.host.EnsureUsable() != nil {
			// Not configured yet; the configure event wakes the park.
			continue
		}
		// Start the enter tween on the first usable pass, before the
		// first draw, so the window's first frame sits at reveal 0.
		w.enterIfDue()
		// Draw when something changed: a configure-driven resize
		// repaint always goes out (a pacing-gated one deadlocks,
		// see shouldDraw), otherwise the frameOwed pacing decides:
		// either the previous frame's callback returned, or an
		// animation's timer deadline passed with the callback
		// still unheard - an occluded surface stops receiving
		// callbacks, and the animation clock keeps it moving at
		// the frame period.
		if shouldDraw(w, animating, now, resized) {
			w.dirty = false
			if !w.draw() {
				// A wire failure mid-frame (attach/commit on a dead
				// socket) classifies exactly like a dispatch one.
				return a.loopError(w.drawErr)
			}
		}
	}
	if err := a.drivePopovers(worked); err != nil {
		return a.loopError(err)
	}
	if a.quit {
		return ErrClosed
	}

	var tipNext, repNext, animFrame time.Time
	for _, w := range a.windows {
		if t, ok := w.tip.next(); ok && (tipNext.IsZero() || t.Before(tipNext)) {
			tipNext = t
		}
	}
	if t, ok := a.rep.nextDeadline(); ok {
		repNext = t
	}
	if t, ok := anim.Next(); ok {
		animFrame = t
	}
	wakeAt, ok := nextWake(repNext, animFrame, tipNext, a.queues.nextDeadline(), now)
	if ok {
		a.kicker.schedule(a.wake, wakeAt)
	}
	if err := step(); err != nil {
		// A dead connection (compositor restart, protocol verdict)
		// runs the disconnect policy here; other dispatch failures
		// come back as they are.
		return a.loopError(fmt.Errorf("app: dispatch: %w", err))
	}
	return nil
}

// updateTips advances every window's tooltip state machine.
func (a *Application) updateTips(now time.Time) {
	for _, w := range a.windows {
		w.tip.delay = a.tooltipOpts.Delay
		w.tip.update(w.router, now, func(h widget.Widget, text string) (tooltipWindow, *popup.Painter) {
			if a.tooltipFace == nil {
				return nil, nil
			}
			// A nil *popup.Popup must not be wrapped: the interface
			// would carry a typed nil that the nil checks below let
			// through to a dereference.
			tp, pc := openTooltip(w.sess, w.host, a.tooltipFace, a.tooltipOpts, w.frac120, int(w.input.x), int(w.input.y), h, text)
			if tp == nil {
				return nil, nil
			}
			return tp, pc
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

// deliverKey routes one key press through the focused window's
// router, then its bindings: the window's own OnKey hook (per-window
// bindings such as Escape closes this window) runs before the
// application-wide one. The hook chain must hang off the FOCUSED
// window - consulting only the app-level hook silently dropped every
// Config.OnKey a client passed to Run, so Escape-to-close never fired.
func (a *Application) deliverKey(keycode uint32, mods wlsession.Mods) {
	if a.cancelDragKey(keycode) {
		return
	}
	if op := a.keyPopover(); op != nil {
		a.deliverPopoverKey(a.sess, op, keycode, mods)
		return
	}
	target := a.focused()
	if target == nil {
		return
	}
	extra := target.cfg.onKey
	if extra == nil {
		extra = a.onKey
	} else if a.onKey != nil {
		windowKey, appKey := extra, a.onKey
		extra = func(r *widget.Router, code uint32, m wlsession.Mods) {
			windowKey(r, code, m)
			appKey(r, code, m)
		}
	}
	// The inspector's chords run before everything else - widget
	// routing, accelerators, and text - but only while it is armed,
	// so unarmed apps never lose their ctrl+shift bindings.
	if a.inspectArmed {
		if act, ok := inspectKey(a.sess.KeySym(keycode), mods); ok {
			switch act {
			case inspectToggle:
				a.ToggleInspect()
			case inspectDump:
				a.dumpTree(target)
			}
			return
		}
	}
	if captureKey(a.sess, target.cfg.keyCapture, keycode, mods) {
		return
	}
	a.reportTransfer(routeKey(a.sess, target.router, keycode, mods, a.clip, a.accels, extra))
}

// captureKey offers one press to a window's KeyCapture hook and
// reports whether the hook consumed it. The hook sees the keysym
// normalized the way accelerators are (letters folded to lowercase)
// and the modifiers without caps lock, which is a latched state rather
// than part of a chord.
func captureKey(sess keyTranslator, capture func(Accel) bool, keycode uint32, mods wlsession.Mods) bool {
	if capture == nil {
		return false
	}
	return capture(Accel{Sym: normalizeSym(sess.KeySym(keycode)), Mods: mods &^ wlsession.ModCapsLock})
}

// imeEvent applies one input-method batch into the focused widget of
// whatever holds the keyboard and repaints it; the controller re-pushes
// state when the batch was current.
func (a *Application) imeEvent(ev wlsession.IMEEvent) {
	r := a.keyboardRouter()
	if r == nil {
		return
	}
	a.ime.deliver(r, ev)
	if op := a.keyPopover(); op != nil {
		op.pop.MarkFrame()
		return
	}
	for _, w := range a.windows {
		w.dirty = true
	}
}

// keyboardRouter is the router keyboard input lands in: the topmost open popover's while one holds the seat
// keyboard (its xdg_popup grab, the same rule deliverKey follows),
// else the focused window's. The input method follows it, so a
// popover Entry (a menu search, a dropdown entry) gets preedit and
// commit like a window's.
func (a *Application) keyboardRouter() *widget.Router {
	if op := a.keyPopover(); op != nil {
		return op.router
	}
	if w := a.focused(); w != nil {
		return w.router
	}
	return nil
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

// windowOf returns the window whose tree holds w, nil when none does
// (a popover's content, a detached widget).
func (a *Application) windowOf(w widget.Widget) *hostWindow {
	root := widget.RootOf(w)
	for _, hw := range a.windows {
		if hw.router != nil && hw.router.Root == root {
			return hw
		}
	}
	return nil
}

// hostOf returns the loop state for a toplevel handle.
func (a *Application) hostOf(w *Window) *hostWindow {
	for _, hw := range a.windows {
		if hw.win == w {
			return hw
		}
	}
	return nil
}

// hostOfLayer returns the loop state for a layer handle.
func (a *Application) hostOfLayer(l *LayerWindow) *hostWindow {
	for _, hw := range a.windows {
		if hw.layer == l {
			return hw
		}
	}
	return nil
}

// Window is the application's handle on one toplevel window.
type Window struct {
	app    *Application
	win    *window.Window
	closed bool
	// cfg and kind are what the window was opened from, the base a
	// rebuild on a new connection starts from (reconnect.go); alpha is
	// the SetOpacity in effect (0: none).
	cfg   WindowConfig
	kind  surfx.Kind
	alpha float64
}

// Close closes the window from the client side; the close-request veto
// does not apply to explicit closes. Animated kinds (dialogs) run the
// exit tween first: Closed flips immediately, input seals, and the
// real destroy waits for the tween to land. Plain toplevels close on
// the spot as before.
func (w *Window) Close() {
	if w.closed {
		return
	}
	if hw := w.app.hostOf(w); hw != nil && hw.beginExit() {
		w.closed = true
		return
	}
	if w.win != nil {
		w.win.Close()
	}
	w.closed = true
}

// Closed reports whether the window closed.
func (w *Window) Closed() bool {
	return w.closed || (w.win != nil && w.win.Closed())
}

// SetCloseRequest installs a veto: return false to keep the window
// open when the compositor asks it to close.
func (w *Window) SetCloseRequest(veto func() bool) {
	if w.win != nil {
		w.win.SetCloseRequest(veto)
	}
}

// SetMinSize constrains how far the compositor may resize the window
// down (set_min_size on the wire); zero axes are unconstrained. The
// limits also clamp layout client-side.
func (w *Window) SetMinSize(width, height uint32) {
	if w.win != nil {
		// A failed request means the connection is dying; the next
		// frame fails with the same cause.
		_ = w.win.SetMinSize(width, height)
	}
}

// SetMaxSize constrains how far the compositor may resize the window
// up (set_max_size); zero axes are unconstrained.
func (w *Window) SetMaxSize(width, height uint32) {
	if w.win != nil {
		_ = w.win.SetMaxSize(width, height)
	}
}

// Maximize asks the compositor to maximize the window. Like every
// state request it is a hint: the compositor may refuse it, and the
// visible truth is whatever a later configure confirms - read that
// through State, never assume the request landed. The confirmed
// maximize usually arrives as an output-sized configure, which rides
// the same relayout path as any other resize.
func (w *Window) Maximize() {
	if w.win != nil {
		// A failed request means the connection is dying; the next
		// frame fails with the same cause.
		_ = w.win.Maximize()
	}
}

// Unmaximize asks the compositor to restore the window. See Maximize.
func (w *Window) Unmaximize() {
	if w.win != nil {
		_ = w.win.Unmaximize()
	}
}

// Fullscreen asks the compositor to show the window fullscreen; the
// compositor picks the output (FullscreenOn names one). See Maximize.
func (w *Window) Fullscreen() { w.FullscreenOn(nil) }

// Unfullscreen asks the compositor to leave fullscreen. See Maximize.
func (w *Window) Unfullscreen() {
	if w.win != nil {
		_ = w.win.Unfullscreen()
	}
}

// SetMinimized asks the compositor to minimize the window. The
// protocol reports no state for a minimized window, so State keeps
// what was confirmed before; treat the window as invisible from here
// on.
func (w *Window) SetMinimized() {
	if w.win != nil {
		_ = w.win.SetMinimized()
	}
}

// State returns the compositor-confirmed window state: the state array
// of the last configure, not the set of requests the client sent. A
// refused request never shows up here.
func (w *Window) State() window.State {
	if w.win == nil {
		return window.State{}
	}
	return w.win.State()
}

// SetFocus moves the window's keyboard focus to target, a widget of its
// tree; see widget.Router.SetFocus for what is ignored.
func (w *Window) SetFocus(target widget.Widget) {
	if hw := w.app.hostOf(w); hw != nil {
		hw.router.SetFocus(target)
		hw.dirty = true
	}
}

// LayerWindow is the application's handle on one layer surface.
type LayerWindow struct {
	app *Application
	ls  *layersurface.Surface
	// cfg is what the surface was opened from (reconnect.go); alpha is
	// the SetOpacity in effect.
	cfg   LayerConfig
	alpha float64
}

// Close closes the layer surface from the client side: the exit tween
// runs first — the launcher/panel-reveal fade-out — with the loop kept
// alive and event-driven until it lands, then the real destroy.
func (l *LayerWindow) Close() {
	if hw := l.app.hostOfLayer(l); hw != nil && hw.beginExit() {
		return
	}
	l.ls.Close()
}

// SetFocus moves the surface's keyboard focus to w, a widget of its
// tree; see widget.Router.SetFocus for what is ignored. The compositor
// still decides whether the surface itself holds the keyboard.
func (l *LayerWindow) SetFocus(w widget.Widget) {
	if hw := l.app.hostOfLayer(l); hw != nil {
		hw.router.SetFocus(w)
		hw.dirty = true
	}
}

// SetSize requests a new surface size in logical pixels, for a surface
// whose content grows or shrinks (a launcher list that fits its rows).
// Like the initial size, an auto (zero) axis needs both of its edges
// anchored; a request that breaks the rule is refused with an error
// before it reaches the wire. The size applies with the next commit,
// and the compositor's configure answer resizes the buffers.
func (l *LayerWindow) SetSize(width, height uint32) error {
	return l.commit(l.ls.SetSize(width, height))
}

// SetAnchor re-anchors the surface (a bar moving edges); the auto-axis
// rule SetSize states applies. Lands with the next commit.
func (l *LayerWindow) SetAnchor(a layersurface.Anchor) error { return l.commit(l.ls.SetAnchor(a)) }

// SetMargin changes the distances from the anchored edges.
func (l *LayerWindow) SetMargin(m layersurface.Margins) error { return l.commit(l.ls.SetMargin(m)) }

// SetExclusiveZone changes the reserved space along the anchored edge
// - an auto-hiding bar handing its zone back.
func (l *LayerWindow) SetExclusiveZone(zone int32) error {
	return l.commit(l.ls.SetExclusiveZone(zone))
}

// SetLayer moves the surface to another stack layer (a panel rising to
// overlay while shown); needs layer shell v2.
func (l *LayerWindow) SetLayer(layer layersurface.Layer) error { return l.commit(l.ls.SetLayer(layer)) }

// commit marks the surface for the commit that applies an accepted
// change; a refused one returns its error untouched.
func (l *LayerWindow) commit(err error) error {
	if err != nil {
		return err
	}
	l.requestFrame()
	return nil
}

// KeyboardMode is the surface's keyboard interactivity.
func (l *LayerWindow) KeyboardMode() KeyboardMode { return l.ls.KeyboardMode() }

// SetKeyboardMode changes the keyboard interactivity: None, Exclusive,
// or OnDemand. Popovers opened on a None surface switch it to OnDemand
// while they are open, so their grab can take the keyboard.
func (l *LayerWindow) SetKeyboardMode(mode KeyboardMode) error {
	if err := l.ls.SetKeyboardMode(mode); err != nil {
		return err
	}
	l.cfg.Keyboard = mode
	return nil
}

// holdKeyboard implements keyboardModer: a popover's temporary mode.
func (l *LayerWindow) holdKeyboard(mode KeyboardMode) error { return l.ls.SetKeyboardMode(mode) }

// Closed reports whether the compositor or client closed the surface.
// A surface running its exit tween reads closed here (the logical
// state flipped at Close) while its last frames still paint.
func (l *LayerWindow) Closed() bool {
	if hw := l.app.hostOfLayer(l); hw != nil && hw.exiting {
		return true
	}
	return l.ls.Closed()
}

// HeldMods is the modifiers held right now (shift, ctrl, alt): what a
// pointer binding reads, since pointer events carry no modifier state
// of their own (a launcher's Ctrl+double-click).
func (a *Application) HeldMods() Mods { return a.sess.Mods() }

// repaintOnIconReset bridges every reset of c into the loop as a
// repaint of every window, so live themed icons re-resolve.
func (a *Application) repaintOnIconReset(c *icons.Cache) {
	c.OnReset(func() {
		a.Invoke(func() {
			for _, w := range a.windows {
				w.dirty = true
			}
		})
	})
}

// SetSurfaceMotion turns gelm's own surface tweens on or off app-wide:
// the fade and slide every layer surface, dialog, popover and tooltip
// plays as it maps and closes. An application that animates its
// surfaces' content itself (through a widget.Revealer) turns them off
// so the two do not stack; surfaces then map and close at once, with
// the same callbacks in the same order.
func (a *Application) SetSurfaceMotion(on bool) { surfx.SetEnabled(on) }

// IdleNotifyAvailable reports whether the compositor can tell the
// application when the user goes idle (ext-idle-notify-v1).
func (a *Application) IdleNotifyAvailable() bool { return a.sess.IdleNotifyAvailable() }

// OnIdle watches for timeout of user inactivity: idle runs once the
// user has been idle that long, resume when they are active again,
// both on the loop goroutine, until the returned stop. Idle inhibitors
// (a video playing) hold the timer like they hold the compositor's
// own. Fails when the protocol is unavailable - an absent protocol
// leaves the application with no idle signal rather than a guess.
func (a *Application) OnIdle(timeout time.Duration, idle, resume func()) (stop func(), err error) {
	n, err := a.sess.IdleNotify(timeout, false)
	if err != nil {
		return nil, err
	}
	n.OnIdle, n.OnResume = idle, resume
	return n.Destroy, nil
}
