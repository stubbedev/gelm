// hostWindow is one hosted widget tree inside a running loop: the
// per-window bookkeeping (router, buffer pool, frame pacing, dirty
// state, tooltips) shared by the single-window Run convenience and the
// multi-window Application. It implements the wlsession pointer
// handler for its surface through surfaceInput.
//
// Frames are damage-tracked: widgets invalidated since the last frame
// (widget.CollectDamage) define the repaint region, painting is clipped
// to it, and only that region goes out as wl_surface.damage_buffer. The
// pool's stale bookkeeping keeps partial repaints correct when the
// acquired buffer's content is older than the last frame; a fresh,
// resized, or rescaled buffer is fully stale and falls back to a full
// repaint.
//
// Coordinates: the widget tree lives in logical (surface) pixels end to
// end - layout, input routing, popovers, IME rects. The device scale
// (frac120, 120-based; 240 is 2x, 150 is 1.25) appears only in buffer
// sizes, the paint canvas, and the damage rects sent on the wire.
package app

import (
	"errors"
	"fmt"
	"time"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"

	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/dragdrop"
	"github.com/stubbedev/gelm/internal/scale"
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

	pool *buffer.Pool
	// surf is the wire half of the host surface; production wraps the
	// wl_surface, tests substitute a recorder.
	surf surfaceHandle
	// sc is the surface-scale wire state: viewport destination or
	// integer set_buffer_scale, plus the output transform. Production
	// wraps internal/scale.Controller; nil in wire-free tests.
	sc scaleWire
	// newBuffer allocates one pooled buffer at the current logical size
	// and device scale; a field so wire-free tests substitute fake
	// buffers and rescale keeps using the substitute.
	newBuffer func() (*buffer.Buffer, error)
	// frac120 is the device scale as a 120-based fraction (120 is 1x,
	// 150 is 1.25, 240 is 2): buffers are ceil(logical * frac120/120)
	// device pixels in both axes. scale is the rounded-up integer
	// fallback the wire sees without the viewporter.
	frac120 uint32
	scale   int
	// scaleApplied records that the wire scale state went out at least
	// once, so the first draw publishes it when the surface had no size
	// to scale at creation time.
	scaleApplied bool
	// dnd is the application's drag-and-drop controller; its Bind
	// lifetime matches the window's.
	dnd          *dragdrop.Controller
	router       *widget.Router
	input        *surfaceInput
	tip          *tooltipCtl
	lastW, lastH int
	// lastFocus is the widget the focus ring was last drawn around; a
	// change adds both rings' rects to the damage union.
	lastFocus widget.Widget
	// pendingRects accumulates drained invalidations until they are
	// painted into an acquired buffer; it survives pool-busy retries.
	pendingRects []render.Rect
	dirty        bool
	frameReady   bool
	framePending bool
	drawErr      error
	// paintedPixels is the pixel count the last frame actually wrote;
	// the paint-count tests pin it.
	paintedPixels int
	// lastDamage mirrors the rects the last frame damaged; tests and
	// traces read it.
	lastDamage []render.Rect
}

// windowHooks are the app-visible callbacks one window carries.
type windowHooks struct {
	background render.Color
	onPress    func(button uint32, serial uint32, over widget.Widget)
	onMove     func(x, y float64)
	onKey      func(r *widget.Router, keycode uint32, mods wlsession.Mods)
	onClosed   func()
}

// focusRingPad is how far the keyboard focus ring extends beyond the
// focused widget's bounds.
const focusRingPad = 2

func newHostWindow(sess *wlsession.Session, host Host, initialScale int, root widget.Widget, hooks windowHooks, dnd *dragdrop.Controller) *hostWindow {
	w0, h0 := host.Size()
	if initialScale < 1 {
		initialScale = 1
	}
	w := &hostWindow{
		host: host, sess: sess, cfg: hooks,
		surf:    wireSurface{wl: host.HostSurface()},
		frac120: uint32(initialScale * 120),
		scale:   initialScale,
		router:  &widget.Router{Root: root},
		dnd:     dnd,
		tip:     &tooltipCtl{since: time.Now()},
		lastW:   w0, lastH: h0,
		dirty: true,
	}
	w.newBuffer = w.create
	w.pool = buffer.New(w.allocator(), 3)
	// Wire the scale state before anything can commit: with the
	// fractional protocols the compositor's preferred_scale events (the
	// initial one included) land in rescale; without them the window
	// keeps the integer scale it was created with.
	w.sc = scale.New(sess, host.HostSurface(), w.rescale)
	input := &surfaceInput{
		sess: sess, surf: host.HostSurface(),
		router: w.router, tip: w.tip,
		onPress: hooks.onPress, onMove: hooks.onMove,
		dnd:     dnd,
		request: func() { w.dirty = true },
		blocked: func() bool { return w.blocked },
		frac:    func() uint32 { return w.frac120 },
	}
	w.input = input
	sess.SetSurfaceInput(host.HostSurface(), input)
	if dnd != nil {
		dnd.Bind(host.HostSurface(), input)
	}
	return w
}

// release unregisters the window's input handlers; called when the
// loop drops the window.
func (w *hostWindow) release() {
	w.sess.SetSurfaceInput(w.host.HostSurface(), nil)
	if w.dnd != nil {
		w.dnd.Bind(w.host.HostSurface(), nil)
	}
}

// create builds one buffer at the window's current logical size and
// device scale and wires its release event into the pool, once per
// buffer lifetime.
func (w *hostWindow) create() (*buffer.Buffer, error) {
	bw, bh := w.host.Size()
	b, err := buffer.NewFile(w.sess.Shm(),
		scale.DeviceSize(bw, w.frac120), scale.DeviceSize(bh, w.frac120), w.scale)
	if err != nil {
		return nil, err
	}
	wlclient.BufferAddListener(b.WL, buffer.ReleaseHandler{B: b})
	return b, nil
}

// scaleWire is the surface-scale seam beside surfaceHandle: the requests
// a rescale performs on the wire. Production wraps
// internal/scale.Controller; tests record. Nil is allowed and skips the
// wire traffic (wire-free tests).
type scaleWire interface {
	// Apply publishes device scale frac120 for a surface of logical
	// size w x h: the viewport destination in fractional mode, the
	// rounded-up set_buffer_scale in integer mode.
	Apply(frac120 uint32, w, h int) error
	// SetTransform publishes the output transform the buffers are
	// submitted for (rotated outputs).
	SetTransform(t int32) error
}

// allocator is the pool's buffer source: it always consults the
// newBuffer field, so a rescale's pool rebuild keeps any test
// substitute in place.
func (w *hostWindow) allocator() func() (*buffer.Buffer, error) {
	return func() (*buffer.Buffer, error) { return w.newBuffer() }
}

// devNum is frac120 as the numerator the render pipeline multiplies
// by, normalizing the zero value of wire-free test windows to 1x.
func (w *hostWindow) devNum() int {
	if w.frac120 == 0 {
		return scale.Denom
	}
	return int(w.frac120)
}

// rescale switches the window to a new device scale in place: the same
// hostWindow, widget tree, router, focus, and pool survive; only the
// buffers are rebuilt. The fresh buffers are fully stale, so the next
// frame falls back to a full repaint at the new device size, exactly as
// a resize does. frac120 is the 120-based preferred scale (150 is 1.25);
// zero is not a scale and is ignored.
func (w *hostWindow) rescale(frac120 uint32) {
	if frac120 == 0 || frac120 == w.frac120 {
		return
	}
	w.frac120 = frac120
	w.scale = scale.IntegerScale(frac120)
	bw, bh := w.host.Size()
	if w.sc != nil {
		_ = w.sc.Apply(frac120, bw, bh)
	}
	w.scaleApplied = true
	w.pool.Resize(w.allocator())
	// The logical layout is unchanged, so widgets owe no damage - but
	// the screen must still be fully repainted at the new device size.
	// Queue the whole window; the fresh buffers' full staleness covers
	// the device-space remainder on the next frame.
	w.pendingRects = append(w.pendingRects, render.Rect{W: bw, H: bh})
	w.dirty = true
	debug.Log("frame", "rescale %d/120: buffers %d px per logical px",
		frac120, frac120)
}

// surfaceHandle is the wire-facing half of the host surface: the
// double-buffered operations one frame performs. Production wraps the
// wl_surface; tests substitute fakes so the paint pipeline runs without
// a wayland connection.
type surfaceHandle interface {
	// Attach presents b on the surface.
	Attach(b *buffer.Buffer) error
	// Damage marks rects, in buffer pixels, as changed since the last
	// frame.
	Damage(rects []render.Rect) error
	// Commit applies attach and damage atomically.
	Commit() error
	// Frame arms the compositor callback that fires ready once the
	// frame may be followed by another.
	Frame(ready *bool) error
}

// wireSurface is the production surfaceHandle over a wl_surface.
type wireSurface struct{ wl *wl.Surface }

// Attach implements surfaceHandle.
func (s wireSurface) Attach(b *buffer.Buffer) error { return s.wl.Attach(b.WL, 0, 0) }

// Damage implements surfaceHandle: one damage_buffer call per rect, in
// buffer pixels.
func (s wireSurface) Damage(rects []render.Rect) error {
	for _, r := range rects {
		if err := s.wl.DamageBuffer(int32(r.X), int32(r.Y), int32(r.W), int32(r.H)); err != nil {
			return err
		}
	}
	return nil
}

// Commit implements surfaceHandle.
func (s wireSurface) Commit() error { return s.wl.Commit() }

// Frame implements surfaceHandle.
func (s wireSurface) Frame(ready *bool) error {
	cb, err := s.wl.Frame()
	if err != nil {
		return err
	}
	wlclient.CallbackAddListener(cb, frameDone{ready: ready})
	return nil
}

// draw paints one frame: resize check, acquire, cached measure, arrange,
// damage collection, region-clipped paint, commit, and frame-callback
// arming. It reports whether the loop may continue; drawErr carries the
// failure.
func (w *hostWindow) draw() bool {
	// Resize before acquiring: Resize destroys every buffered
	// wl_buffer, so running it after Acquire would hand back a
	// destroyed buffer and the compositor kills the connection on the
	// attach. A late configure therefore also marks the frame dirty
	// even without input, and the fresh buffers are fully stale, so
	// the frame repaints everything.
	bw, bh := w.host.Size()
	if bw != w.lastW || bh != w.lastH {
		w.lastW, w.lastH = bw, bh
		w.pool.Resize(w.allocator())
		w.dirty = true
	}
	// Publish the wire scale state on the first draw with a real size:
	// a surface created unconfigured has nothing to scale yet. Fractional
	// windows have already applied theirs through rescale.
	if w.sc != nil && !w.scaleApplied && bw > 0 && bh > 0 {
		if err := w.sc.Apply(w.frac120, bw, bh); err == nil {
			w.scaleApplied = true
		}
	}

	// Measure (cached per widget: a static tree costs nothing) and
	// arrange; Arrange invalidates the old rect of anything that moved,
	// so the drain below sees moves from this frame too. All tree
	// coordinates are logical pixels.
	w.router.Root.Measure(widget.Constraints{Max: widget.Size{W: bw, H: bh}})
	w.router.Root.Arrange(render.Rect{X: 0, Y: 0, W: bw, H: bh})

	// Drain invalidations into the pending region. The pending list
	// survives busy retries (the flags are already drained) and is what
	// lets a frame with nothing paintable skip the buffer entirely.
	// Rects are clipped to the window here: a widget may owe pixels
	// outside the buffer (an overflowing scroll child), and such rects
	// must not keep a frame alive.
	bufBounds := render.Rect{W: bw, H: bh}
	if rects, _ := widget.CollectDamage(w.router.Root); len(rects) > 0 {
		for _, r := range rects {
			if r = r.Intersect(bufBounds); !r.Empty() {
				w.pendingRects = append(w.pendingRects, r)
			}
		}
	}
	w.pendingRects = append(w.pendingRects, w.focusChangeRects()...)
	if len(w.pendingRects) == 0 {
		debug.Log("frame", "draw skipped: nothing damaged")
		w.paintedPixels = 0
		w.lastDamage = nil
		return true
	}

	b, err := w.pool.Acquire()
	if errors.Is(err, buffer.ErrBusy) {
		// The release event wakes the park below; stay dirty. The
		// pending region is kept for the retry.
		debug.Log("frame", "draw deferred: pool busy")
		w.dirty = true
		return true
	}
	if err != nil {
		w.drawErr = fmt.Errorf("app: acquire buffer: %w", err)
		return false
	}

	// A buffer with stale content must repaint everything that changed
	// since its content was last on screen, not just this frame's
	// damage. Fresh buffers are fully stale: the full-repaint fallback
	// for the first frame, after resizes, and after rescales.
	// Everything past this point is in device pixels: pending logical
	// damage maps outward, stale is tracked in buffer pixels.
	bufBounds = render.Rect{W: b.Width, H: b.Height}
	rects := make([]render.Rect, 0, len(w.pendingRects)+1)
	for _, r := range w.pendingRects {
		if r = render.MapRect(r, w.devNum(), scale.Denom).Intersect(bufBounds); !r.Empty() {
			rects = append(rects, r)
		}
	}
	// changed is what actually differs on screen after this commit; it
	// drives the other buffers' staleness. It deliberately excludes
	// b.Stale: initializing a fresh buffer paints the whole window but
	// changes nothing the screen was not already showing.
	changed := render.UnionAll(rects)
	if sb := b.Stale.Intersect(bufBounds); !sb.Empty() {
		rects = append(rects, sb)
	}
	region := render.UnionAll(rects)
	w.pendingRects = w.pendingRects[:0]
	debug.Log("frame", "draw %dx%d at %d/120: %d damage rect(s), region %dx%d",
		bw, bh, w.frac120, len(rects), region.W, region.H)

	cv := render.NewScaled(b.Data, b.Stride, b.Width, b.Height, w.devNum(), scale.Denom)
	prev := cv.PushClip(region)
	cv.ClearDevice(region, w.cfg.background)
	w.router.Root.Paint(cv)
	if ring := w.focusRingRect(); !ring.Empty() {
		cv.BorderRect(ring, focusRingPad, widget.Current().Accent)
	}
	cv.PopClip(prev)
	w.paintedPixels = cv.Touched()
	w.lastDamage = rects

	if err := w.surf.Attach(b); err != nil {
		w.drawErr = fmt.Errorf("app: attach: %w", err)
		return false
	}
	if debug.Enabled {
		debug.Log("frame", "surface %d attached", w.host.HostSurface().Id())
	}
	if err := w.surf.Damage(rects); err != nil {
		w.drawErr = fmt.Errorf("app: damage: %w", err)
		return false
	}
	if err := w.surf.Commit(); err != nil {
		w.drawErr = fmt.Errorf("app: commit: %w", err)
		return false
	}
	// The commit just went to the screen through b: b is current now,
	// and every other pooled buffer lags by what changed here.
	w.pool.Presented(b, changed)

	if err := w.surf.Frame(&w.frameReady); err != nil {
		w.drawErr = fmt.Errorf("app: frame callback: %w", err)
		return false
	}
	debug.Log("frame", "frame committed, waiting for callback")
	w.framePending = true
	return true
}

// focusRingRect returns the rect the keyboard focus ring occupies, or
// an empty rect when nothing is focused.
func (w *hostWindow) focusRingRect() render.Rect {
	f := w.router.Focused()
	if f == nil {
		return render.Rect{}
	}
	bs, ok := f.(widget.Boundser)
	if !ok {
		return render.Rect{}
	}
	fb := bs.Bounds()
	if fb.W <= 0 || fb.H <= 0 {
		return render.Rect{}
	}
	return ringRect(fb)
}

// focusChangeRects returns both rings' rects when the focused widget
// changed since the last frame: the old ring's area must be repainted
// to erase it, the new one to draw it.
func (w *hostWindow) focusChangeRects() []render.Rect {
	f := w.router.Focused()
	if f == w.lastFocus {
		return nil
	}
	var rects []render.Rect
	for _, fw := range []widget.Widget{w.lastFocus, f} {
		bs, ok := fw.(widget.Boundser)
		if !ok {
			continue
		}
		if fb := bs.Bounds(); fb.W > 0 && fb.H > 0 {
			rects = append(rects, ringRect(fb))
		}
	}
	w.lastFocus = f
	return rects
}

// ringRect is the focus ring's rect around a widget's bounds.
func ringRect(fb render.Rect) render.Rect {
	return render.Rect{X: fb.X - focusRingPad, Y: fb.Y - focusRingPad, W: fb.W + 2*focusRingPad, H: fb.H + 2*focusRingPad}
}
