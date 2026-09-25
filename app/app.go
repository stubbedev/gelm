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
	// IdleWait bounds one idle poll before another frame is drawn.
	// Zero defaults to 50ms.
	IdleWait time.Duration
}

// Run drives the host until it closes: acquire a buffer, measure and
// arrange the tree, paint, commit full-frame damage, pace on the frame
// callback, and dispatch input into a router meanwhile. It returns
// ErrClosed when the surface ended.
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
	var pointer struct{ x, y float64 }
	redraw := make(chan struct{}, 1)
	request := func() {
		select {
		case redraw <- struct{}{}:
		default:
		}
	}
	lastW, lastH := host.Size()
	tip := &tooltipCtl{since: time.Now()}
	lastCursor := ""

	sess := cfg.Session
	sess.OnPointerMove = func(x, y float64) {
		pointer.x, pointer.y = x, y
		router.Move(widget.Point{X: int(x) * cfg.Scale, Y: int(y) * cfg.Scale})
		if shape := cursorFor(router.Hovered()); shape != lastCursor {
			lastCursor = shape
			if err := sess.SetCursor(shape); err != nil {
				lastCursor = ""
			}
		}
		if cfg.OnPointerMove != nil {
			cfg.OnPointerMove(x, y)
		}
		request()
	}
	sess.OnPointerButton = func(button, state, serial uint32) {
		p := widget.Point{X: int(pointer.x) * cfg.Scale, Y: int(pointer.y) * cfg.Scale}
		if state == 1 {
			// Any press dismisses a tooltip, like every toolkit.
			if tip.open != nil {
				tip.open.Close()
			}
			router.Press(button, p)
			if cfg.OnPress != nil {
				cfg.OnPress(button, serial, router.Hovered())
			}
		} else {
			router.Release(button, p)
		}
		request()
	}
	sess.OnPointerAxis = func(dy float64) {
		steps := int(dy / 10)
		if dy != 0 && steps == 0 {
			steps = 1
			if dy < 0 {
				steps = -1
			}
		}
		router.Axis(float64(steps))
		request()
	}
	sess.OnPointerLeave = func() {
		router.Leave()
		request()
	}

	idle := cfg.IdleWait
	if idle == 0 {
		idle = 50 * time.Millisecond
	}

	// Key repeat: the compositor tells us its rate and delay; held keys
	// re-fire the route while waitInput polls.
	routeKey := func(keycode uint32, mods wlsession.Mods) {
		routeKey(sess, router, keycode, mods, cfg.Clipboard, cfg.OnKey)
	}
	rep := newKeyRepeater(sess.RepeatInfo())
	sess.OnKey = func(keycode uint32, mods wlsession.Mods) {
		rep.press(keycode, mods)
		routeKey(keycode, mods)
		request()
	}
	sess.OnKeyUp = rep.release
	pump := func() bool {
		code, mods, ok := rep.tick()
		if !ok {
			return false
		}
		routeKey(code, mods)
		return true
	}

	for !host.Closed() {
		b, err := pool.Acquire()
		if errors.Is(err, buffer.ErrBusy) {
			if err := sess.Roundtrip(); err != nil {
				return fmt.Errorf("app: dispatch while busy: %w", err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("app: acquire buffer: %w", err)
		}
		wlclient.BufferAddListener(b.WL, buffer.ReleaseHandler{B: b})

		bw, bh := host.Size()
		if bw != lastW || bh != lastH {
			lastW, lastH = bw, bh
			pool.Resize(create)
		}
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
			return fmt.Errorf("app: attach: %w", err)
		}
		if err := surf.DamageBuffer(0, 0, int32(b.Width), int32(b.Height)); err != nil {
			return fmt.Errorf("app: damage: %w", err)
		}
		if err := surf.Commit(); err != nil {
			return fmt.Errorf("app: commit: %w", err)
		}

		frameReady := false
		cb, err := surf.Frame()
		if err != nil {
			return fmt.Errorf("app: frame callback: %w", err)
		}
		wlclient.CallbackAddListener(cb, frameDone{ready: &frameReady})
		for !frameReady && !host.Closed() {
			if err := sess.Roundtrip(); err != nil {
				return fmt.Errorf("app: frame dispatch: %w", err)
			}
		}

		anim.Tick(time.Now())
		tip.update(router, time.Now(), func(h widget.Widget, text string) tooltipWindow {
			return openTooltip(sess, host, &cfg, int(pointer.x), int(pointer.y), text)
		})
		if !waitInput(sess, redraw, host, idle, pump) {
			break
		}
		if anim.Active() {
			// Animations need the next frame immediately; skip the
			// idle wait.
			continue
		}
	}
	return ErrClosed
}

// frameDone flips ready when the compositor reports the frame as taken.
type frameDone struct {
	ready *bool
}

// HandleCallbackDone implements wl.CallbackDoneHandler.
func (f frameDone) HandleCallbackDone(wl.CallbackDoneEvent) {
	*f.ready = true
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

// waitInput polls the connection until more input arrives or the deadline
// passes, firing due key repeats along the way, and reports whether the
// loop should continue.
func waitInput(sess *wlsession.Session, redraw chan struct{}, host Host, idle time.Duration, pump func() bool) bool {
	deadline := time.Now().Add(idle)
	for time.Now().Before(deadline) && !host.Closed() {
		select {
		case <-redraw:
			return true
		default:
		}
		if pump() {
			return true
		}
		if err := sess.Roundtrip(); err != nil {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
	return !host.Closed()
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
