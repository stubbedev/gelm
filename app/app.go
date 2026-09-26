// Package app owns the shared event loop: buffer pooling, input routing
// into the widget tree, frame-callback pacing, and idle dispatch. A Host
// abstracts over layer surfaces and toplevel windows so either can carry
// a widget tree.
package app

import (
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/internal/buffer"
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
	// Scale is the integer output scale the surface renders at.
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
// surface.
type surfaceInput struct {
	sess    *wlsession.Session
	surf    *wl.Surface
	scale   int
	router  *widget.Router
	tip     *tooltipCtl
	onPress func(button uint32, serial uint32, over widget.Widget)
	onMove  func(x, y float64)

	x, y       float64
	lastCursor string
	// request schedules a repaint; installed by Run once the redraw
	// channel exists.
	request func()
}

// HandlePointerEnter implements wlsession.SurfacePointerHandler.
func (in *surfaceInput) HandlePointerEnter(x, y float64) {
	in.move(x, y)
}

// HandlePointerMotion implements wlsession.SurfacePointerHandler.
func (in *surfaceInput) HandlePointerMotion(x, y float64) {
	in.move(x, y)
}

// move feeds one pointer position to the router and updates the
// cursor shape and hover bookkeeping.
func (in *surfaceInput) move(x, y float64) {
	in.x, in.y = x, y
	in.router.Move(widget.Point{X: int(x) * in.scale, Y: int(y) * in.scale})
	debug.Log("input", "route move (%.1f,%.1f) hit %T", x, y, in.router.Hovered())
	if debug.Enabled {
		if bs, ok := in.router.Hovered().(widget.Boundser); ok {
			fb := bs.Bounds()
			debug.Log("input", "hit bounds (%d,%d)+%dx%d", fb.X, fb.Y, fb.W, fb.H)
		}
	}
	if shape := cursorFor(in.router.Hovered()); shape != in.lastCursor {
		in.lastCursor = shape
		if err := in.sess.SetCursor(shape); err != nil {
			in.lastCursor = ""
		}
	}
	if in.onMove != nil {
		in.onMove(x, y)
	}
	in.request()
}

// HandlePointerButton implements wlsession.SurfacePointerHandler.
func (in *surfaceInput) HandlePointerButton(button, state, serial uint32) {
	p := widget.Point{X: int(in.x) * in.scale, Y: int(in.y) * in.scale}
	debug.Log("input", "route button %d %s at (%.1f,%.1f) over %T",
		button, buttonStateName(state), in.x, in.y, in.router.Hovered())
	if state == 1 {
		// Any press dismisses a tooltip, like every toolkit.
		if in.tip.open != nil {
			in.tip.open.Close()
		}
		in.router.Press(button, p)
		if in.onPress != nil {
			in.onPress(button, serial, in.router.Hovered())
		}
	} else {
		in.router.Release(button, p)
	}
	in.request()
}

// HandlePointerAxis implements wlsession.SurfacePointerHandler.
func (in *surfaceInput) HandlePointerAxis(dy float64) {
	steps := int(dy / 10)
	if dy != 0 && steps == 0 {
		steps = 1
		if dy < 0 {
			steps = -1
		}
	}
	debug.Log("input", "route axis dy=%.1f steps=%d hover %T", dy, steps, in.router.Hovered())
	in.router.Axis(float64(steps))
	in.request()
}

// HandlePointerLeave implements wlsession.SurfacePointerHandler.
func (in *surfaceInput) HandlePointerLeave() {
	debug.Log("input", "route leave")
	in.router.Leave()
	in.request()
}

// Run drives the host until it closes: acquire a buffer, measure and
// arrange the tree, paint, commit full-frame damage, pace on the frame
// callback, and dispatch input into a router meanwhile. The loop parks
// in Session.Step between events: timers (key repeat, animation, tooltip
// dwell) schedule wakeups, and with nothing pending the process holds no
// CPU — even when the compositor stops sending frame callbacks for an
// occluded surface. It returns ErrClosed when the surface ended.
func Run(cfg Config) error {
	host := cfg.Host
	if err := host.EnsureUsable(); err != nil {
		return fmt.Errorf("app: %w", err)
	}

	surf := host.HostSurface()
	create := func() (*buffer.Buffer, error) {
		bw, bh := host.Size()
		return buffer.NewFile(cfg.Session.Shm(), bw*cfg.Scale, bh, cfg.Scale)
	}
	pool := buffer.New(create, 3)

	router := &widget.Router{Root: cfg.Root}
	dirty := true
	markDirty := func() { dirty = true }
	lastW, lastH := host.Size()
	tip := &tooltipCtl{since: time.Now()}

	input := &surfaceInput{
		sess: cfg.Session, surf: surf, scale: cfg.Scale,
		router: router, tip: tip,
		onPress: cfg.OnPress, onMove: cfg.OnPointerMove,
		request: markDirty,
	}
	cfg.Session.SetSurfaceInput(surf, input)
	defer cfg.Session.SetSurfaceInput(surf, nil)

	sess := cfg.Session

	// Key repeat: the compositor tells us its rate and delay; repeats
	// fire from timer wakeups, not loop polling.
	routeKey := func(keycode uint32, mods wlsession.Mods) {
		routeKey(sess, router, keycode, mods, cfg.Clipboard, cfg.OnKey)
	}
	rep := newKeyRepeater(sess.RepeatInfo())
	sess.OnKey = func(keycode uint32, mods wlsession.Mods) {
		rep.press(keycode, mods)
		routeKey(keycode, mods)
		markDirty()
	}
	sess.OnKeyUp = rep.release

	frameReady := false
	framePending := false
	var drawErr error
	kicker := &loopKicker{}
	opener := func(h widget.Widget, text string) tooltipWindow {
		return openTooltip(sess, host, &cfg, int(input.x), int(input.y), text)
	}

	draw := func() bool {
		// Resize before acquiring: Resize destroys every buffered
		// wl_buffer, so running it after Acquire would hand back a
		// destroyed buffer and the compositor kills the connection on
		// the attach. A late configure therefore also marks the frame
		// dirty even without input.
		bw, bh := host.Size()
		if bw != lastW || bh != lastH {
			lastW, lastH = bw, bh
			pool.Resize(create)
			dirty = true
		}
		b, err := pool.Acquire()
		if errors.Is(err, buffer.ErrBusy) {
			// The release event wakes the park below; stay dirty.
			dirty = true
			return true
		}
		if err != nil {
			return false
		}
		wlclient.BufferAddListener(b.WL, buffer.ReleaseHandler{B: b})

		cfg.Root.Measure(widget.Constraints{Max: widget.Size{W: bw, H: bh}})
		cfg.Root.Arrange(render.Rect{X: 0, Y: 0, W: bw, H: bh})

		cv := render.New(b.Data, b.Stride, b.Width, b.Height)
		cv.Clear(cv.Rect(), cfg.Background)
		cfg.Root.Paint(cv)

		// Keyboard focus ring around the focused widget.
		if f := router.Focused(); f != nil {
			if bs, ok := f.(widget.Boundser); ok {
				if fb := bs.Bounds(); fb.W > 0 && fb.H > 0 {
					cv.BorderRect(render.Rect{X: fb.X - 2, Y: fb.Y - 2, W: fb.W + 4, H: fb.H + 4},
						2, widget.Current().Accent)
				}
			}
		}

		if err := surf.Attach(b.WL, 0, 0); err != nil {
			drawErr = fmt.Errorf("app: attach: %w", err)
			return false
		}
		debug.Log("frame", "main surface %d attached", surf.Id())
		if err := surf.DamageBuffer(0, 0, int32(b.Width), int32(b.Height)); err != nil {
			drawErr = fmt.Errorf("app: damage: %w", err)
			return false
		}
		if err := surf.Commit(); err != nil {
			drawErr = fmt.Errorf("app: commit: %w", err)
			return false
		}

		cb, err := surf.Frame()
		if err != nil {
			drawErr = fmt.Errorf("app: frame callback: %w", err)
			return false
		}
		wlclient.CallbackAddListener(cb, frameDone{ready: &frameReady})
		debug.Log("frame", "frame committed, waiting for callback")
		framePending = true
		return true
	}

	for !host.Closed() {
		now := time.Now()

		if frameReady {
			frameReady = false
			framePending = false
		}
		if code, mods, ok := rep.tick(); ok {
			routeKey(code, mods)
			dirty = true
		}
		if anim.Active() {
			anim.Tick(now)
			dirty = true
		}
		tip.update(router, now, opener)

		// Draw when something changed and pacing allows: either the
		// previous frame's callback returned, or an animation keeps
		// producing frames even when the compositor stopped scheduling
		// them (occlusion). An input-only redraw always goes out; if a
		// frame is still pending the loop simply waits for its event.
		if dirty && (!framePending || anim.Active()) {
			dirty = false
			if !draw() {
				return drawErr
			}
		}

		if host.Closed() {
			break
		}

		tipNext, _ := tip.next()
		repNext, repOK := rep.nextDeadline()
		animEnd, animOK := anim.Next()
		if !repOK {
			repNext = time.Time{}
		}
		if !animOK {
			animEnd = time.Time{}
		}
		wakeAt, ok := nextWake(repNext, animEnd, tipNext, now)
		if ok {
			kicker.schedule(sess, wakeAt)
		}
		if err := sess.Step(); err != nil {
			return fmt.Errorf("app: dispatch: %w", err)
		}
	}
	return ErrClosed
}

// loopKicker coalesces timer wakeups: one outstanding WakeAfter per
// deadline, skipped while an earlier kick still covers the requested
// time.
type loopKicker struct {
	until time.Time
}

// schedule arms a wakeup for at unless one is already pending that
// covers it.
func (k *loopKicker) schedule(sess *wlsession.Session, at time.Time) {
	now := time.Now()
	if !k.until.After(now) || at.Add(-2*time.Millisecond).Before(k.until) {
		// A stale or already-covered kick: rearm.
		k.until = at
		sess.WakeAfter(at.Sub(now))
	}
}

// nextWake computes the earliest timer deadline the parked loop must
// wake for; false means nothing is pending and the loop may sleep until
// the next compositor event. Deadlines already due return false: the
// next loop iteration handles them, and parking for zero duration would
// only burn a cycle.
func nextWake(repeat, animEnd, tipNext time.Time, now time.Time) (time.Time, bool) {
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
	consider(animEnd)
	consider(tipNext)
	return wake, found
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
		if mods&wlsession.ModShift != 0 {
			router.FocusPrev()
		} else {
			router.FocusNext()
		}
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
	switch sym {
	case xkb.KeyBackSpace:
		return widget.KeyBackspace, true
	case xkb.KeyDelete:
		return widget.KeyDelete, true
	case xkb.KeyLeft:
		return widget.KeyLeft, true
	case xkb.KeyRight:
		return widget.KeyRight, true
	case xkb.KeyUp:
		return widget.KeyUp, true
	case xkb.KeyDown:
		return widget.KeyDown, true
	case xkb.KeyHome:
		return widget.KeyHome, true
	case xkb.KeyEnd:
		return widget.KeyEnd, true
	case xkb.KeyReturn, xkb.KeyKPEnter:
		return widget.KeyEnter, true
	}
	return 0, false
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
