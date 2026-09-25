// Package app owns the shared event loop: buffer pooling, input routing
// into the widget tree, frame-callback pacing, and idle dispatch. A Host
// abstracts over layer surfaces and toplevel windows so either can carry
// a widget tree.
package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"

	"github.com/stubbedev/gelm/internal/buffer"
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
	// OnPress, when set, fires after the router recorded a press; a
	// window can use the serial for interactive move.
	OnPress func(serial uint32, over widget.Widget)
	// OnKey, when set, receives every key press together with the
	// router, for apps that map keycodes to typing or actions.
	OnKey func(r *widget.Router, keycode uint32, shift bool)
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

	sess := cfg.Session
	sess.OnPointerMove = func(x, y float64) {
		pointer.x, pointer.y = x, y
		router.Move(widget.Point{X: int(x) * cfg.Scale, Y: int(y) * cfg.Scale})
		request()
	}
	sess.OnPointerButton = func(button, state, serial uint32) {
		p := widget.Point{X: int(pointer.x) * cfg.Scale, Y: int(pointer.y) * cfg.Scale}
		if state == 1 {
			router.Press(button, p)
			if cfg.OnPress != nil {
				cfg.OnPress(serial, router.Hovered())
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
	sess.OnKey = func(keycode uint32, shift bool) {
		if cfg.OnKey != nil {
			cfg.OnKey(router, keycode, shift)
		}
		request()
	}

	idle := cfg.IdleWait
	if idle == 0 {
		idle = 50 * time.Millisecond
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

		if !waitInput(sess, redraw, host, idle) {
			break
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

// waitInput polls the connection until more input arrives or the deadline
// passes, and reports whether the loop should continue.
func waitInput(sess *wlsession.Session, redraw chan struct{}, host Host, idle time.Duration) bool {
	deadline := time.Now().Add(idle)
	for time.Now().Before(deadline) && !host.Closed() {
		select {
		case <-redraw:
			return true
		default:
		}
		if err := sess.Roundtrip(); err != nil {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
	return !host.Closed()
}
