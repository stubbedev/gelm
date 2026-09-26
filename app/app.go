// Package app owns the shared event loop: buffer pooling, input routing
// into the widget tree, frame-callback pacing, and idle dispatch. A Host
// abstracts over layer surfaces and toplevel windows so either can carry
// a widget tree.
package app

import (
	"errors"
	"time"
	"unicode/utf8"

	"github.com/neurlang/wayland/wl"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/internal/clipboard"
	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// ErrClosed reports that the surface ended and the loop exited. Callers
// that treat a plain close as success can match it with errors.Is.
var ErrClosed = errors.New("app: surface closed")

// Host is the piece of the shell a widget tree is hosted on: a layer
// surface or a toplevel window. Both types implement it.
type Host interface {
	// EnsureUsable gates drawing until the first configure completed.
	EnsureUsable() error
	// Closed reports whether the compositor or user ended the surface.
	Closed() bool
	// Size returns the current size in surface (logical) pixels.
	Size() (int, int)
	// HostSurface returns the underlying wl_surface.
	HostSurface() *wl.Surface
}

// Config describes an app run.
type Config struct {
	// Session is the wayland connection.
	Session *wlsession.Session
	// Host carries the widget tree.
	Host Host
	// Scale is the initial integer output scale the surface renders at;
	// zero means 1. With the fractional-scale protocol the compositor's
	// preferred scale (1.25 and friends) overrides this live.
	Scale int
	// Root is the widget tree.
	Root widget.Widget
	// Background fills the frame before the tree paints.
	Background render.Color
	// OnPress, when set, fires after the router recorded a press with
	// the button, its serial (for interactive move), and the widget
	// under the pointer.
	OnPress func(button uint32, serial uint32, over widget.Widget)
	// OnPointerMove, when set, receives raw pointer positions in
	// surface (logical) coordinates, for anchoring context menus.
	OnPointerMove func(x, y float64)
	// TooltipFace renders hover tooltips; nil disables tooltips.
	TooltipFace *render.Typeface
	// Clipboard, when set, enables ctrl+c, ctrl+x, and ctrl+v on the
	// focused widget's selection.
	Clipboard *clipboard.Clipboard
	// OnKey, when set, receives every key press (repeats included)
	// together with the router, for apps that map keycodes to typing
	// or actions.
	OnKey func(r *widget.Router, keycode uint32, mods wlsession.Mods)
}

// surfaceInput routes one host surface's pointer events into the
// widget tree: hover tracking, press/release with click detection,
// drags, scroll, cursor shapes, and tooltip state. It implements
// wlsession.SurfacePointerHandler, so events arrive only when the
// compositor's pointer focus (or an active grab) belongs to this
// surface. Pointer coordinates are logical pixels and route into the
// widget tree unchanged - the device scale lives entirely in the paint
// pipeline.
type surfaceInput struct {
	sess    *wlsession.Session
	surf    *wl.Surface
	router  *widget.Router
	tip     *tooltipCtl
	onPress func(button uint32, serial uint32, over widget.Widget)
	onMove  func(x, y float64)
	// dnd, when set, starts drags from press+motion gestures and
	// receives this surface's data-device events (dragdrop.Target).
	dnd dragController
	// frac reports the host window's current 120-based device scale,
	// for one-shot surfaces created from this input (drag icons).
	frac func() uint32
	// resizeAt maps a pointer position to the xdg_toplevel.resize_edge
	// bits under it (0 when interior); set only for toplevel windows.
	resizeAt func(x, y float64) uint32
	// startResize engages the compositor's resize grab; while both are
	// set, an edge press belongs to the window frame, not the widgets.
	startResize func(edges uint32, serial uint32)

	x, y           float64
	lastCursor     string
	pressX, pressY float64
	pressSerial    uint32
	dragStarted    bool
	// request schedules a repaint; installed by Run once the redraw
	// channel exists.
	request func()
	// blocked suppresses all pointer input while a modal dialog is
	// open over this window.
	blocked func() bool
}

// dropInput reports whether a modal dialog over this window currently
// owns the pointer.
func (in *surfaceInput) dropInput() bool { return in.blocked != nil && in.blocked() }

// HandlePointerEnter implements wlsession.SurfacePointerHandler.
func (in *surfaceInput) HandlePointerEnter(x, y float64) {
	if in.dropInput() {
		return
	}
	in.move(x, y)
}

// HandlePointerMotion implements wlsession.SurfacePointerHandler.
func (in *surfaceInput) HandlePointerMotion(x, y float64) {
	if in.dropInput() {
		return
	}
	in.move(x, y)
}

// move feeds one pointer position to the router and updates the
// cursor shape, hover bookkeeping, and the drag gesture. The position
// is logical surface coordinates; the tree is laid out in the same
// space, so it routes 1:1 at every device scale.
func (in *surfaceInput) move(x, y float64) {
	in.x, in.y = x, y
	in.router.Move(widget.Point{X: int(x), Y: int(y)})
	in.startDrag()
	debug.Log("input", "route move (%.1f,%.1f) hit %T", x, y, in.router.Hovered())
	if debug.Enabled {
		if bs, ok := in.router.Hovered().(widget.Boundser); ok {
			fb := bs.Bounds()
			debug.Log("input", "hit bounds (%d,%d)+%dx%d", fb.X, fb.Y, fb.W, fb.H)
		}
	}
	if shape := in.cursorAt(x, y); shape != in.lastCursor {
		in.lastCursor = shape
		if in.sess != nil {
			if err := in.sess.SetCursor(shape); err != nil {
				in.lastCursor = ""
			}
		}
	}
	if in.onMove != nil {
		in.onMove(x, y)
	}
	in.request()
}

// cursorAt resolves the pointer shape at a position: a resize handle's
// resize_* shape wins over the hovered widget's own request - the frame
// owns the edges.
func (in *surfaceInput) cursorAt(x, y float64) string {
	if in.resizeAt != nil {
		if shape := resizeCursor(in.resizeAt(x, y)); shape != "" {
			return shape
		}
	}
	return cursorFor(in.router.Hovered())
}

// HandlePointerButton implements wlsession.SurfacePointerHandler.
func (in *surfaceInput) HandlePointerButton(button, state, serial uint32) {
	if in.dropInput() {
		return
	}
	p := widget.Point{X: int(in.x), Y: int(in.y)}
	debug.Log("input", "route button %d %s at (%.1f,%.1f) over %T",
		button, buttonStateName(state), in.x, in.y, in.router.Hovered())
	if state == 1 {
		// Any press dismisses a tooltip, like every toolkit.
		if in.tip != nil && in.tip.open != nil {
			in.tip.open.Close()
		}
		// A press on a resize handle belongs to the window frame: the
		// xdg_toplevel.resize grab replaces the widget press entirely.
		if in.startResize != nil && in.resizeAt != nil {
			if edges := in.resizeAt(in.x, in.y); edges != 0 {
				debug.Log("input", "edge %d grabbed for resize (serial %d)", edges, serial)
				in.startResize(edges, serial)
				in.request()
				return
			}
		}
		in.router.Press(button, p)
		// Remember the press for the drag gesture: the threshold is
		// measured from here and start_drag wants this serial.
		in.pressX, in.pressY = in.x, in.y
		in.pressSerial = serial
		in.dragStarted = false
		if in.onPress != nil {
			in.onPress(button, serial, in.router.Hovered())
		}
	} else {
		in.router.Release(button, p)
	}
	in.request()
}

// HandlePointerAxis implements wlsession.SurfacePointerHandler: dx
// from the horizontal axis, dy from the vertical, both positive
// downward/rightward.
func (in *surfaceInput) HandlePointerAxis(dx, dy float64) {
	if in.dropInput() {
		return
	}
	dxSteps, dySteps := axisSteps(dx), axisSteps(dy)
	debug.Log("input", "route axis dx=%.1f dy=%.1f steps=%d,%d hover %T", dx, dy, dxSteps, dySteps, in.router.Hovered())
	in.router.Axis(float64(dxSteps), float64(dySteps))
	in.request()
}

// axisSteps converts a smooth axis value to 40px steps, keeping at
// least one step when the value is nonzero.
func axisSteps(v float64) int {
	steps := int(v / 10)
	if v != 0 && steps == 0 {
		steps = 1
		if v < 0 {
			steps = -1
		}
	}
	return steps
}

// HandlePointerLeave implements wlsession.SurfacePointerHandler.
func (in *surfaceInput) HandlePointerLeave() {
	if in.dropInput() {
		return
	}
	debug.Log("input", "route leave")
	in.router.Leave()
	in.request()
}

// Run drives one host until it closes: acquire a buffer, measure and
// arrange the tree, paint, commit full-frame damage, pace on the frame
// callback, and dispatch input into a router meanwhile. It is the
// single-window convenience over Application; the loop parks in
// Session.Step between events, so with nothing pending the process
// holds no CPU — even when the compositor stops sending frame callbacks
// for an occluded surface. It returns ErrClosed when the surface ended.
func Run(cfg Config) error {
	app := NewApplication(cfg.Session)
	app.SetClipboard(cfg.Clipboard)
	app.SetTooltipFace(cfg.TooltipFace)
	app.newWindow(cfg.Host, cfg.Scale, cfg.Root, windowHooks{
		background: cfg.Background,
		onPress:    cfg.OnPress,
		onMove:     cfg.OnPointerMove,
		onKey:      cfg.OnKey,
	}, nil)
	return app.Run()
}

// loopKicker coalesces timer wakeups: one outstanding WakeAfter per
// deadline, skipped while an earlier kick still covers the requested
// time.
type loopKicker struct {
	until time.Time
}

// schedule arms a wakeup for at unless one is already pending that
// covers it. While a tween runs the loop passes here once per frame:
// re-arming a covered deadline would queue another sync per pass and
// the parked loop would stop parking at all.
func (k *loopKicker) schedule(sess *wlsession.Session, at time.Time) {
	now := time.Now()
	if k.covers(at, now) {
		return
	}
	k.until = at
	sess.WakeAfter(at.Sub(now))
}

// covers reports whether the pending kick still satisfies at: it has
// not fired yet, and fires no more than 2ms before at. A pending kick
// that already fired (or lands earlier than that) does not cover.
func (k *loopKicker) covers(at, now time.Time) bool {
	return k.until.After(now) && !at.Add(-2*time.Millisecond).After(k.until)
}

// nextWake computes the earliest timer deadline the parked loop must
// wake for; false means nothing is pending and the loop may sleep until
// the next compositor event. animFrame is the animation clock's next
// tick deadline while tweens run. Deadlines already due return false:
// the next loop iteration handles them, and parking for zero duration
// would only burn a cycle.
func nextWake(repeat, animFrame, tipNext time.Time, now time.Time) (time.Time, bool) {
	wake := time.Time{}
	found := false
	consider := func(t time.Time) {
		if t.IsZero() || !t.After(now) {
			return
		}
		if !found || t.Before(wake) {
			wake, found = t, true
		}
	}
	consider(repeat)
	consider(animFrame)
	consider(tipNext)
	return wake, found
}

// shouldDraw reports whether a dirty window may paint this iteration.
// A configure-driven resize always repaints: compositors hold the
// surface's frame callback until it commits at the configured size, so
// a resize repaint gated on pacing would deadlock the resize - a
// liveness requirement, not politeness. Otherwise frameOwed pacing
// decides: the previous frame's callback returned, or an animation's
// timer deadline passed with the callback still unheard (an occluded
// surface stops getting callbacks and the animation clock keeps it
// moving at the frame period).
func shouldDraw(w *hostWindow, animating bool, now time.Time, resized bool) bool {
	return w.dirty && (resized || w.frameOwed(animating, now))
}

// frameDone flips ready when the compositor reports the frame as taken.
type frameDone struct {
	ready *bool
}

// HandleCallbackDone implements wl.CallbackDoneHandler.
func (f frameDone) HandleCallbackDone(wl.CallbackDoneEvent) {
	debug.Log("frame", "frame callback done")
	*f.ready = true
}

// buttonStateName renders a wl_pointer.button state for traces.
func buttonStateName(state uint32) string {
	if state == 1 {
		return "press"
	}
	return "release"
}

// ctrl reports whether ctrl is held.
func ctrl(m wlsession.Mods) bool { return m&wlsession.ModCtrl != 0 }

// keyTranslator is the keymap-facing slice of the session.
type keyTranslator interface {
	KeyUTF8(code uint32) string
	KeySym(code uint32) xkb.Keysym
}

// routeKey turns one key press into text or a widget action through the
// compositor's keymap, then gives the app the raw event for any custom
// bindings. Alt is never text. Ctrl handles clipboard (c, x, v when a
// clipboard is configured) and select-all; other ctrl combos still act
// on editing keys.
func routeKey(sess keyTranslator, router *widget.Router, keycode uint32, mods wlsession.Mods, clip *clipboard.Clipboard, extra func(*widget.Router, uint32, wlsession.Mods)) {
	sym := sess.KeySym(keycode)
	isCtrl := ctrl(mods)
	switch {
	case isCtrl && clip != nil && (sym == xkb.Keysym('c') || sym == xkb.Keysym('C')):
		copySelection(router, clip)
	case isCtrl && clip != nil && (sym == xkb.Keysym('x') || sym == xkb.Keysym('X')):
		if copySelection(router, clip) {
			router.KeyAction(widget.KeyDelete, widget.Mods(mods))
		}
	case isCtrl && clip != nil && (sym == xkb.Keysym('v') || sym == xkb.Keysym('V')):
		pasteSelection(router, clip)
	case isCtrl && (sym == xkb.Keysym('a') || sym == xkb.Keysym('A')):
		router.SelectAll()
	case !isCtrl && mods&wlsession.ModAlt == 0 && sym == xkb.KeyTab:
		// Tab trap: inside a widget that absorbs tabs (a multi-line
		// text area) a plain Tab indents; ctrl+Tab and shift+Tab move
		// focus.
		if mods&wlsession.ModShift != 0 {
			router.FocusPrev()
			break
		}
		if f := router.Focused(); f != nil {
			if tt, ok := f.(widget.TabTrapper); ok && tt.TrapTab(false) {
				break
			}
		}
		router.FocusNext()
	case !isCtrl && mods&wlsession.ModAlt == 0:
		if txt := sess.KeyUTF8(keycode); txt != "" {
			if r, _ := utf8.DecodeRuneInString(txt); r != utf8.RuneError && r != 0 {
				router.Type(r)
			}
			break
		}
		if a, ok := actionForSym(sym); ok {
			router.KeyAction(a, widget.Mods(mods))
		}
	default:
		if a, ok := actionForSym(sym); ok {
			router.KeyAction(a, widget.Mods(mods))
		}
	}
	if extra != nil {
		extra(router, keycode, mods)
	}
}

// copySelection puts the focused widget's selection on the clipboard
// and reports whether there was one.
func copySelection(router *widget.Router, clip *clipboard.Clipboard) bool {
	f := router.Focused()
	if f == nil {
		return false
	}
	sel, ok := f.(widget.SelectedTexter)
	if !ok {
		return false
	}
	text, has := sel.SelectedText()
	if !has {
		return false
	}
	return clip.WriteText(text) == nil
}

// pasteSelection inserts the clipboard text at the focused widget's
// cursor.
func pasteSelection(router *widget.Router, clip *clipboard.Clipboard) {
	text, err := clip.ReadText()
	if err != nil {
		return
	}
	for _, r := range text {
		router.Type(r)
	}
}

// actionForSym maps editing keysyms to widget actions.
func actionForSym(sym xkb.Keysym) (widget.KeyAction, bool) {
	return widget.KeyActionForSym(sym)
}

// keyRepeater synthesizes repeat presses for a held key, on the
// compositor's schedule: one repeat after the initial delay, then one
// every 1/rate.
type keyRepeater struct {
	rate, delay time.Duration
	held        *heldKey
	next        time.Time
}

type heldKey struct {
	code uint32
	mods wlsession.Mods
}

// newKeyRepeater builds a repeater from the compositor's repeat info;
// zeros fall back to 400ms delay and 20 keys per second.
func newKeyRepeater(rate, delayMs uint32) *keyRepeater {
	if rate == 0 {
		rate = 20
	}
	if delayMs == 0 {
		delayMs = 400
	}
	return &keyRepeater{
		rate:  time.Second / time.Duration(rate),
		delay: time.Duration(delayMs) * time.Millisecond,
	}
}

// press records the key now held; a new press replaces and re-arms the
// previous one, matching physical repeat behavior.
func (r *keyRepeater) press(code uint32, mods wlsession.Mods) {
	r.held = &heldKey{code: code, mods: mods}
	r.next = time.Now().Add(r.delay)
}

// release stops repeating when the held key comes up.
func (r *keyRepeater) release(code uint32) {
	if r.held != nil && r.held.code == code {
		r.held = nil
	}
}

// tick fires the next repeat when one is due and reports whether a key
// press should be delivered.
func (r *keyRepeater) tick() (code uint32, mods wlsession.Mods, ok bool) {
	if r.held == nil || time.Now().Before(r.next) {
		return 0, 0, false
	}
	r.next = time.Now().Add(r.rate)
	return r.held.code, r.held.mods, true
}

// nextDeadline reports when the next repeat would fire; false when no
// key is held. The parked loop wakes for it instead of polling.
func (r *keyRepeater) nextDeadline() (time.Time, bool) {
	if r.held == nil {
		return time.Time{}, false
	}
	return r.next, true
}
