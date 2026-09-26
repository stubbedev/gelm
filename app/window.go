// hostWindow is one hosted widget tree inside a running loop: the
// per-window bookkeeping (router, buffer pool, frame pacing, dirty
// state, tooltips) shared by the single-window Run convenience and the
// multi-window Application. It implements the wlsession pointer
// handler for its surface through surfaceInput.
package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/neurlang/wayland/wlclient"

	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// hostWindow couples a Host with everything the loop needs to drive it.
type hostWindow struct {
	host Host
	sess *wlsession.Session
	cfg  windowHooks

	win *Window

	// blocked suppresses pointer input while a modal dialog is open;
	// isDialog marks the exempt dialog windows themselves.
	blocked  bool
	isDialog bool

	pool         *buffer.Pool
	router       *widget.Router
	input        *surfaceInput
	tip          *tooltipCtl
	lastW, lastH int
	dirty        bool
	frameReady   bool
	framePending bool
	drawErr      error
}

// windowHooks are the app-visible callbacks one window carries.
type windowHooks struct {
	background render.Color
	onPress    func(button uint32, serial uint32, over widget.Widget)
	onMove     func(x, y float64)
	onKey      func(r *widget.Router, keycode uint32, mods wlsession.Mods)
	onClosed   func()
}

func newHostWindow(sess *wlsession.Session, host Host, scale int, root widget.Widget, hooks windowHooks) *hostWindow {
	pool := buffer.New(func() (*buffer.Buffer, error) {
		bw, bh := host.Size()
		return buffer.NewFile(sess.Shm(), bw*scale, bh, scale)
	}, 3)
	router := &widget.Router{Root: root}
	w0, h0 := host.Size()
	w := &hostWindow{
		host: host, sess: sess, cfg: hooks,
		pool:   pool,
		router: router,
		tip:    &tooltipCtl{since: time.Now()},
		lastW:  w0,
		lastH:  h0,
		dirty:  true,
	}
	input := &surfaceInput{
		sess: sess, surf: host.HostSurface(), scale: scale,
		router: router, tip: w.tip,
		onPress: hooks.onPress, onMove: hooks.onMove,
		request: func() { w.dirty = true },
		blocked: func() bool { return w.blocked },
	}
	w.input = input
	sess.SetSurfaceInput(host.HostSurface(), input)
	return w
}

// release unregisters the window's input handler; called when the loop
// drops the window.
func (w *hostWindow) release() {
	w.sess.SetSurfaceInput(w.host.HostSurface(), nil)
}

// create builds one buffer at the window's current size and scale.
func (w *hostWindow) create() (*buffer.Buffer, error) {
	bw, bh := w.host.Size()
	return buffer.NewFile(w.sess.Shm(), bw*w.input.scale, bh, w.input.scale)
}

// draw paints one frame: resize check, acquire, layout, paint, focus
// ring, commit, and frame-callback arming. It reports whether the loop
// may continue; drawErr carries the failure.
func (w *hostWindow) draw() bool {
	// Resize before acquiring: Resize destroys every buffered
	// wl_buffer, so running it after Acquire would hand back a
	// destroyed buffer and the compositor kills the connection on the
	// attach. A late configure therefore also marks the frame dirty
	// even without input.
	bw, bh := w.host.Size()
	if bw != w.lastW || bh != w.lastH {
		w.lastW, w.lastH = bw, bh
		w.pool.Resize(w.create)
		w.dirty = true
	}
	debug.Log("frame", "draw %dx%d at scale %d", bw, bh, w.input.scale)
	b, err := w.pool.Acquire()
	if errors.Is(err, buffer.ErrBusy) {
		// The release event wakes the park below; stay dirty.
		w.dirty = true
		return true
	}
	if err != nil {
		w.drawErr = fmt.Errorf("app: acquire buffer: %w", err)
		return false
	}
	wlclient.BufferAddListener(b.WL, buffer.ReleaseHandler{B: b})

	w.router.Root.Measure(widget.Constraints{Max: widget.Size{W: bw, H: bh}})
	w.router.Root.Arrange(render.Rect{X: 0, Y: 0, W: bw, H: bh})

	cv := render.New(b.Data, b.Stride, b.Width, b.Height)
	cv.Clear(cv.Rect(), w.cfg.background)
	w.router.Root.Paint(cv)

	// Keyboard focus ring around the focused widget.
	if f := w.router.Focused(); f != nil {
		if bs, ok := f.(widget.Boundser); ok {
			if fb := bs.Bounds(); fb.W > 0 && fb.H > 0 {
				cv.BorderRect(render.Rect{X: fb.X - 2, Y: fb.Y - 2, W: fb.W + 4, H: fb.H + 4},
					2, widget.Current().Accent)
			}
		}
	}

	surf := w.host.HostSurface()
	if err := surf.Attach(b.WL, 0, 0); err != nil {
		w.drawErr = fmt.Errorf("app: attach: %w", err)
		return false
	}
	debug.Log("frame", "main surface %d attached", surf.Id())
	if err := surf.DamageBuffer(0, 0, int32(b.Width), int32(b.Height)); err != nil {
		w.drawErr = fmt.Errorf("app: damage: %w", err)
		return false
	}
	if err := surf.Commit(); err != nil {
		w.drawErr = fmt.Errorf("app: commit: %w", err)
		return false
	}

	cb, err := surf.Frame()
	if err != nil {
		w.drawErr = fmt.Errorf("app: frame callback: %w", err)
		return false
	}
	wlclient.CallbackAddListener(cb, frameDone{ready: &w.frameReady})
	debug.Log("frame", "frame committed, waiting for callback")
	w.framePending = true
	return true
}
