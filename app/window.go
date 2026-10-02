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
	"sync/atomic"
	"time"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/dragdrop"
	"github.com/stubbedev/gelm/internal/inspect"
	"github.com/stubbedev/gelm/internal/scale"
	"github.com/stubbedev/gelm/internal/surfx"
	"github.com/stubbedev/gelm/internal/window"
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

	// Exit/enter animation state. fx is nil for kinds that do not
	// animate (plain toplevels open and close instantly). fader wraps
	// the app's tree while fx exists; destroy is the raw wire teardown
	// the coordinator's exit lands into. exiting flips at Dismiss, so
	// input goes dead before the first fade frame and the loop keeps
	// the window mapped and drawing until the tween finishes.
	fx           *surfx.Coordinator
	fader        *widget.Fader
	enterStarted bool
	exiting      bool
	destroy      func()
	// kick carries MarkFrame's repaint request from a tween callback
	// (possibly on another goroutine) into the loop pass.
	kick atomic.Bool

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
	// limits reports the host's min/max size in logical pixels (zero
	// axes unconstrained); sizes the compositor configures outside the
	// limits are clamped before layout and buffers see them. Nil for
	// hosts without limits (layer surfaces).
	limits func() (minW, minH, maxW, maxH int)
	// startResize engages the compositor's interactive resize grab;
	// nil for hosts without edges (layer surfaces).
	startResize func(edges uint32, serial uint32)
	// decorated reports compositor-owned decorations, which disables
	// the client's own edge handles; nil means never decorated.
	decorated func() bool
	// state reports the compositor-confirmed toplevel state, so the
	// edge handles also go passive while maximized/fullscreen; nil for
	// hosts without states (layer surfaces).
	state func() window.State
	// inspector is the window's debug overlay (internal/inspect) the
	// tree wraps in; nil only in wire-free tests that build hostWindow
	// by hand. The app toggles it through setInspect.
	inspector *inspect.Overlay
	// lastFocus is the widget the focus ring was last drawn around; a
	// change adds both rings' rects to the damage union.
	lastFocus widget.Widget
	// pendingRects accumulates drained invalidations until they are
	// painted into an acquired buffer; it survives pool-busy retries.
	pendingRects []render.Rect
	dirty        bool
	frameReady   bool
	framePending bool
	// frameArmedAt is when the pending frame callback was armed; the
	// animation clock judges a callback dead (occluded surface) once
	// it stays unanswered past frameStaleAfter.
	frameArmedAt time.Time
	drawErr      error
	// paintedPixels is the pixel count the last frame actually wrote;
	// the paint-count tests pin it.
	paintedPixels int
	// lastDamage mirrors the rects the last frame damaged; tests and
	// traces read it.
	lastDamage []render.Rect
	// opaqueW, opaqueH are the device-pixel size the opaque region was
	// last set at; opaqueSet records that it went out at all. The
	// region only changes on resize/rescale, so the wire call runs once
	// per size change, never per frame.
	opaqueSet        bool
	opaqueW, opaqueH int
}

// windowHooks are the app-visible callbacks one window carries.
type windowHooks struct {
	background render.Color
	// opaque promises the surface is fully opaque: the frame pipeline
	// sets wl_surface.set_opaque_region so the compositor can skip
	// blending behind it. Computed once at config time by opaqueFor.
	opaque  bool
	onPress func(button uint32, serial uint32, over widget.Widget)
	onMove  func(x, y float64)
	onKey   func(r *widget.Router, keycode uint32, mods wlsession.Mods)
	// keyCapture sees each press before any routing; true consumes it.
	keyCapture func(Accel) bool
	onClosed   func()
}

// opaqueFor reports whether a window may promise an opaque surface:
// an explicit opt-in, or automatically whenever the background is
// fully opaque. A translucent background (alpha < 255) must never set
// the region: the compositor blends the surface over what is behind
// it, and promising opacity would make it skip that blend.
func opaqueFor(background render.Color, opaque bool) bool {
	return opaque || background.A() == 255
}

// focusRingPad is how far the keyboard focus ring extends beyond the
// focused widget's bounds.
const focusRingPad = 2

func newHostWindow(sess *wlsession.Session, host Host, initialScale int, root widget.Widget, hooks windowHooks, dnd *dragdrop.Controller, primary *primarySelection, animKind surfx.Kind, destroy func()) *hostWindow {
	if initialScale < 1 {
		initialScale = 1
	}
	w := &hostWindow{
		host: host, sess: sess, cfg: hooks,
		surf:    wireSurface{wl: host.HostSurface(), comp: sess.Compositor()},
		frac120: uint32(initialScale * 120),
		scale:   initialScale,
		router:  &widget.Router{Root: root},
		dnd:     dnd,
		tip:     &tooltipCtl{since: time.Now()},
		dirty:   true,
		destroy: destroy,
	}
	// Animated kinds (layer overlays, dialogs) wrap their tree in a
	// fader the coordinator drives; the enter starts lazily — at the
	// first usable pass, after the configure handshake — so the very
	// first frame paints at reveal 0, never the finished surface.
	if animKind != surfx.KindMenu {
		w.fader = widget.NewFader(root)
		w.router.Root = w.fader
		w.fx = surfx.NewCoordinator(animKind, w, func() bool { return widget.Current().Animations })
	}
	// Toplevel hosts carry the size limits, the resize grab, and the
	// decoration state; layer surfaces implement none of it.
	if sl, ok := host.(sizeLimiter); ok {
		w.limits = sl.SizeLimits
	}
	if rz, ok := host.(resizer); ok {
		w.startResize = func(edges uint32, serial uint32) {
			seat := sess.Seat()
			if seat == nil {
				return
			}
			if err := rz.Resize(seat, serial, edges); err != nil {
				debug.Log("input", "interactive resize: %v", err)
			}
		}
	}
	if sd, ok := host.(serverDecorated); ok {
		w.decorated = sd.ServerDecorated
	}
	if st, ok := host.(stateReporter); ok {
		w.state = st.State
	}
	w.lastW, w.lastH = w.layoutSize()
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
		dnd:         dnd,
		request:     func() { w.dirty = true },
		blocked:     func() bool { return w.blocked || w.exiting },
		frac:        func() uint32 { return w.frac120 },
		startResize: w.startResize,
		primary:     primary,
	}
	// The edge probe exists only where the resize grab does (toplevels);
	// layer surfaces stay pure widgets.
	if w.startResize != nil {
		input.resizeAt = w.resizeEdgeAt
	}
	w.input = input
	sess.SetSurfaceInput(host.HostSurface(), input)
	if dnd != nil {
		dnd.Bind(host.HostSurface(), input)
	}
	return w
}

// release tears down the window's loop state: input handlers are
// unregistered and the buffer pool's storage returns to the session
// arena (held buffers wait for the compositor's release), so closing
// windows leaves no fds, mappings, or slots behind.
func (w *hostWindow) release() {
	if w.sess != nil {
		w.sess.SetSurfaceInput(w.host.HostSurface(), nil)
	}
	if w.dnd != nil {
		w.dnd.Bind(w.host.HostSurface(), nil)
	}
	if w.pool != nil {
		w.pool.Close()
	}
}

// beginExit starts the animated close: Dismiss flips the logical state
// (input seals, the app's close semantics fire) and the exit tween
// keeps the surface mapped and repainting until it lands; the loop
// keeps ticking — event-driven, paced by the animation clock's wake
// deadlines — and only the tween's completion reaches destroy, the
// real wayland teardown. Reports whether the exit ran (or was already
// running); false means there is nothing to animate.
func (w *hostWindow) beginExit() bool {
	if w.fx == nil {
		return false
	}
	w.exiting = true
	w.fx.Dismiss()
	return true
}

// enterIfDue starts the enter tween on the first usable loop pass: the
// configure handshake has completed, so ApplyVisual(0) lands before
// the first draw and the window fades in from nothing — never a flash
// of the finished surface.
func (w *hostWindow) enterIfDue() {
	if w.fx != nil && !w.enterStarted {
		w.enterStarted = true
		w.fx.Enter()
		w.dirty = true
	}
}

// ApplyVisual implements surfx.Driver: one tween frame's reveal.
func (w *hostWindow) ApplyVisual(reveal float64) {
	if w.fader != nil {
		w.fader.SetOpacity(reveal)
	}
}

// MarkFrame implements surfx.Driver: request a repaint through the
// loop's kick flag. The app loop's anim.Tick already marks every
// window dirty when a callback runs there; the kick covers ticks that
// ran on another goroutine (a popup's nested loop).
func (w *hostWindow) MarkFrame() { w.kick.Store(true) }

// SealInput implements surfx.Driver: an empty input region, committed,
// so clicks during the exit pass through the dying window to whatever
// is beneath (the tooltip input-region trick). The commit only applies
// the region; the next frame re-attaches a buffer as usual.
func (w *hostWindow) SealInput() {
	if w.sess == nil {
		return
	}
	region, err := w.sess.Compositor().CreateRegion()
	if err != nil {
		return
	}
	_ = region.Add(0, 0, 0, 0)
	_ = w.host.HostSurface().SetInputRegion(region)
	_ = region.Destroy()
	_ = w.host.HostSurface().Commit()
}

// Destroy implements surfx.Driver: the real teardown, reached only
// when the exit tween has landed (or teardown interrupted it). The
// next loop pass sees host.Closed and reaps the window.
func (w *hostWindow) Destroy() {
	if w.destroy != nil {
		w.destroy()
	}
}

// create builds one buffer at the window's current logical size and
// device scale. The session arena owns the release-event wiring.
func (w *hostWindow) create() (*buffer.Buffer, error) {
	bw, bh := w.layoutSize()
	return buffer.NewFile(w.sess.Shm(),
		scale.DeviceSize(bw, w.frac120), scale.DeviceSize(bh, w.frac120), w.scale)
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

// layoutSize is the configured size clamped to the window's min/max
// limits: the one size every consumer works with - layout, buffers,
// edge hit tests, and the scale wire state. Compositors normally never
// configure outside the limits (set_min_size/set_max_size told them),
// so this is a client-side fallback, not the primary enforcement.
func (w *hostWindow) layoutSize() (int, int) {
	bw, bh := w.host.Size()
	if w.limits == nil {
		return bw, bh
	}
	minW, minH, maxW, maxH := w.limits()
	if minW != 0 && bw < minW {
		bw = minW
	}
	if minH != 0 && bh < minH {
		bh = minH
	}
	if maxW != 0 && bw > maxW {
		bw = maxW
	}
	if maxH != 0 && bh > maxH {
		bh = maxH
	}
	return bw, bh
}

// syncSize picks up a configure-driven size change: the pool resizes
// (retiring the free buffers, keeping those the compositor still holds
// until their release - Resize must run before the next Acquire) and a
// full repaint schedules, so the FIRST frame at the new size is
// already correct: Measure/Arrange run at the new size below, and the
// fresh buffers' full staleness forces a full repaint. Run polls this
// between events - a configure alone, with no widget damage pending,
// must still repaint - and draw re-checks so direct callers keep the
// guarantee. It reports whether the size changed.
func (w *hostWindow) syncSize() bool {
	bw, bh := w.layoutSize()
	if bw == w.lastW && bh == w.lastH {
		return false
	}
	w.lastW, w.lastH = bw, bh
	w.pool.Resize(w.allocator())
	w.dirty = true
	debug.Log("frame", "configure resize: %dx%d", bw, bh)
	return true
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
	bw, bh := w.layoutSize()
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
	// SetOpaqueRegion promises the full w x h rect (device pixels)
	// holds opaque content, so the compositor can skip blending behind
	// the surface. Double-buffered state like the scale: it applies at
	// the next commit and only needs re-sending when the size changes.
	SetOpaqueRegion(w, h int) error
}

// wireSurface is the production surfaceHandle over a wl_surface.
type wireSurface struct {
	wl   *wl.Surface
	comp *wl.Compositor
}

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

// SetOpaqueRegion implements surfaceHandle: the compositor's region
// object carries the full rect, the surface keeps a copy at
// set_opaque_region, and the region dies right after - it is a
// set-once-per-size message, not a long-lived proxy.
func (s wireSurface) SetOpaqueRegion(w, h int) error {
	region, err := s.comp.CreateRegion()
	if err != nil {
		return fmt.Errorf("app: create region: %w", err)
	}
	if err := region.Add(0, 0, int32(w), int32(h)); err != nil {
		_ = region.Destroy()
		return fmt.Errorf("app: region add: %w", err)
	}
	if err := s.wl.SetOpaqueRegion(region); err != nil {
		_ = region.Destroy()
		return fmt.Errorf("app: set_opaque_region: %w", err)
	}
	return region.Destroy()
}

// frameStaleAfter is how long a committed frame may sit unanswered
// before the loop treats the compositor as paused and lets the
// animation timer pace the draw itself. One and a half frame periods:
// safely past callback jitter at any refresh rate, tight enough to
// keep an occluded tween moving.
const frameStaleAfter = 3 * anim.FrameInterval / 2

// maxLayoutPasses bounds the re-layouts one draw runs while the tree
// settles (each level of newly parented widgets can take one).
const maxLayoutPasses = 4

// frameOwed reports whether a dirty window may commit a frame now:
// normally the previous frame's callback must have returned, but a
// running animation whose callback went unheard past frameStaleAfter
// draws on the animation clock's timer instead.
func (w *hostWindow) frameOwed(animating bool, now time.Time) bool {
	if !w.framePending {
		return true
	}
	return animating && now.Sub(w.frameArmedAt) >= frameStaleAfter
}

// syncOpaque publishes the surface's opaque region when it can have
// changed: the full device rect on the first draw and after every
// resize or rescale, and nothing in between - the region is surface
// state that tracks the buffer size, not per-frame traffic. Opaque
// windows only: a translucent background must stay blendable, so it
// never sets a region (opaqueFor). A failed set leaves the state
// unset, so the next frame retries.
func (w *hostWindow) syncOpaque(bw, bh int) {
	if !w.cfg.opaque || bw <= 0 || bh <= 0 {
		return
	}
	dw, dh := scale.DeviceSize(bw, w.frac120), scale.DeviceSize(bh, w.frac120)
	if w.opaqueSet && dw == w.opaqueW && dh == w.opaqueH {
		return
	}
	if err := w.surf.SetOpaqueRegion(dw, dh); err != nil {
		debug.Log("frame", "opaque region: %v", err)
		return
	}
	w.opaqueSet, w.opaqueW, w.opaqueH = true, dw, dh
	debug.Log("frame", "opaque region %dx%d", dw, dh)
}

// draw paints one frame: resize check, acquire, cached measure, arrange,
// damage collection, region-clipped paint, commit, and frame-callback
// arming. It reports whether the loop may continue; drawErr carries the
// failure.
func (w *hostWindow) draw() bool {
	// Resize before acquiring: Resize retires the free buffers at the
	// new size and keeps the ones the compositor still holds until
	// their release, so running it after Acquire would retire the very
	// buffer the frame is about to attach. A late configure therefore
	// also marks the frame dirty even without input, and the fresh
	// buffers are fully stale, so the frame repaints everything.
	w.syncSize()
	bw, bh := w.lastW, w.lastH
	// Publish the wire scale state on the first draw with a real size:
	// a surface created unconfigured has nothing to scale yet. Fractional
	// windows have already applied theirs through rescale.
	if w.sc != nil && !w.scaleApplied && bw > 0 && bh > 0 {
		if err := w.sc.Apply(w.frac120, bw, bh); err == nil {
			w.scaleApplied = true
		}
	}
	// Same surface-state shape as the scale: the opaque region tracks
	// the size and goes out once per size change, before the commit
	// that applies it.
	w.syncOpaque(bw, bh)

	// Measure (cached per widget: a static tree costs nothing) and
	// arrange; Arrange invalidates the old rect of anything that moved,
	// so the drain below sees moves from this frame too. All tree
	// coordinates are logical pixels.
	w.router.Root.Measure(widget.Constraints{Max: widget.Size{W: bw, H: bh}})
	w.router.Root.Arrange(render.Rect{X: 0, Y: 0, W: bw, H: bh})
	// Arranging a new subtree parents it, and a widget first measured
	// without ancestors resolved no scoped stylesheet: settle the layout
	// before painting rather than showing a frame of constructor styles.
	for range maxLayoutPasses {
		if !widget.LayoutPending(w.router.Root) {
			break
		}
		w.router.Root.Measure(widget.Constraints{Max: widget.Size{W: bw, H: bh}})
		w.router.Root.Arrange(render.Rect{X: 0, Y: 0, W: bw, H: bh})
	}

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
	// The focus ring draws right after its widget, in tree order, so
	// what paints later (a card over the list) covers it.
	if ring := w.focusRingRect(); !ring.Empty() {
		cv.MarkFocus(w.router.Focused(), func(cv *render.Canvas) {
			cv.BorderRect(ring, focusRingPad, widget.Current().Accent)
		})
	}
	cv.BeginOverlays()
	w.router.Root.Paint(cv)
	cv.FlushOverlays()
	cv.FinishFocus()
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
	// The frame callback is double-buffered state: requested before the
	// commit it rides on this frame. Requested after, it waited on a
	// next commit that the pending callback itself held back, so a
	// window with no animation running painted once and never again.
	if err := w.surf.Frame(&w.frameReady); err != nil {
		w.drawErr = fmt.Errorf("app: frame callback: %w", err)
		return false
	}
	if err := w.surf.Commit(); err != nil {
		w.drawErr = fmt.Errorf("app: commit: %w", err)
		return false
	}
	// The commit just went to the screen through b: b is current now,
	// and every other pooled buffer lags by what changed here.
	w.pool.Presented(b, changed)

	debug.Log("frame", "frame committed, waiting for callback")
	w.framePending = true
	w.frameArmedAt = time.Now()
	return true
}

// focusRingRect returns the rect the keyboard focus ring occupies, or
// an empty rect when nothing is focused or the focus came from the
// pointer (GTK's :focus-visible rule: a click focuses without a ring).
func (w *hostWindow) focusRingRect() render.Rect {
	f := w.router.Focused()
	if f == nil || !widget.FocusVisible(f) {
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
