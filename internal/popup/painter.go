// The per-frame paint pipeline for a popup surface, factored out of
// Run so two kinds of host loops can drive it without a second
// wayland dispatcher: Run (a nested loop on the caller's goroutine,
// for grabbed menus) and the application loop (for tooltips, which
// must never dispatch — see docs/threading.md: one event-loop
// goroutine owns the connection).
package popup

import (
	"errors"
	"sync/atomic"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"

	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/scale"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// Painter owns one popup surface's buffer pool and frame pacing.
// Pass drives at most one frame per call; the owning loop calls it
// once per pass and parks however it likes between passes.
type Painter struct {
	p *Popup

	pool    *buffer.Pool
	surf    *wl.Surface
	frac120 uint32
	bg      render.Color
	gutter  int           // the shadow margin the content is inset by
	visual  widget.Widget // the slide wrapper around the fader-wrapped content

	frameReady   bool
	framePending bool
	dirty        atomic.Bool
}

// NewPainter wraps root in the tween view (seeded from the
// coordinator, so the first frame is the hidden or offset one, never a
// flash of the finished surface) and returns the frame driver.
func (p *Popup) NewPainter(sess *wlsession.Session, frac120 uint32, root widget.Widget, bg render.Color) *Painter {
	if frac120 == 0 {
		frac120 = scale.Denom
	}
	create := func() (*buffer.Buffer, error) {
		w, h := p.Size()
		return buffer.NewFile(sess.Shm(),
			scale.DeviceSize(w, frac120), scale.DeviceSize(h, frac120),
			scale.IntegerScale(frac120))
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.anim = newAnimView(root, p.fx.Reveal(), p.gravity)
	pc := &Painter{
		p:       p,
		pool:    buffer.New(create, 2),
		surf:    p.WLSurface,
		frac120: frac120,
		bg:      bg,
		gutter:  p.gutter,
		visual:  p.anim.slide,
	}
	pc.dirty.Store(true)
	p.painter = pc
	return pc
}

// frameDone flips ready when the compositor reports the frame as taken
// and unregisters the callback: done is a destructor event, so the
// object is dead on both sides and its id must rejoin the client's
// pool. A frame loop that skips this leaks a proxy per frame.
type frameDone struct {
	ready *bool
}

// HandleCallbackDone implements wl.CallbackDoneHandler.
func (f frameDone) HandleCallbackDone(ev wl.CallbackDoneEvent) {
	ev.C.Unregister()
	*f.ready = true
}

// Pass paints at most one frame: when a repaint is owed and the
// compositor has taken the previous one, measure, arrange, paint under
// the current reveal, commit, and arm the next frame callback. The
// whole wire sequence holds p.mu, serialized against the dismissal
// state machine's SealInput and Destroy. It returns false when the
// popup is destroyed and the painter is spent.
func (pc *Painter) Pass() (bool, error) {
	if pc.p.Destroyed() {
		return false, nil
	}
	if pc.p.dirty.CompareAndSwap(true, false) {
		pc.dirty.Store(true)
	}
	if pc.frameReady {
		pc.frameReady = false
		pc.framePending = false
	}
	if !pc.dirty.Load() || pc.framePending {
		return true, nil
	}
	pc.dirty.Store(false)
	b, err := pc.pool.Acquire()
	if errors.Is(err, buffer.ErrBusy) {
		// The release event (dispatched by the owning loop) wakes it.
		pc.dirty.Store(true)
		return true, nil
	}
	if err != nil {
		return false, err
	}
	p := pc.p
	p.mu.Lock()
	if p.Destroyed() {
		p.mu.Unlock()
		buffer.Cancel(b)
		return false, nil
	}
	w, h := p.Size()
	_ = p.sc.Apply(pc.frac120, w, h)
	cw, ch := max(0, w-2*pc.gutter), max(0, h-2*pc.gutter)
	content := render.Rect{X: pc.gutter, Y: pc.gutter, W: cw, H: ch}
	pc.visual.Measure(widget.Constraints{Max: widget.Size{W: cw, H: ch}})
	pc.visual.Arrange(content)

	cv := render.NewScaled(b.Data, b.Stride, b.Width, b.Height, int(pc.frac120), scale.Denom)
	reveal := p.fx.Reveal()
	// The surface raw-clears to transparent either way: with a shadow
	// gutter the falloff must blend over whatever the popup floats
	// above, and a recycled buffer must not leak its last frame
	// through (ClearDevice ignores PushAlpha by design).
	cv.ClearDevice(cv.Rect(), render.Color(0))
	if reveal < 1 {
		// Tween frame: the whole surface — shadow, plate, content —
		// fades under one PushAlpha, so the exit tween carries the
		// elevation out with the popup.
		prev := cv.PushAlpha(reveal)
		pc.paintPlate(cv, content)
		pc.visual.Paint(cv)
		cv.PopAlpha(prev)
	} else {
		pc.paintPlate(cv, content)
		pc.visual.Paint(cv)
	}

	if err := pc.surf.Attach(b.WL, 0, 0); err != nil {
		p.mu.Unlock()
		return false, err
	}
	if err := pc.surf.DamageBuffer(0, 0, int32(b.Width), int32(b.Height)); err != nil {
		p.mu.Unlock()
		return false, err
	}
	// The frame request is double-buffered state: it must precede the
	// commit it rides on. Requested after, it would wait for a commit
	// that never comes - this pass only commits again once the callback
	// it is waiting for has fired.
	cb, err := pc.surf.Frame()
	if err != nil {
		p.mu.Unlock()
		return false, err
	}
	wlclient.CallbackAddListener(cb, frameDone{ready: &pc.frameReady})
	if err := pc.surf.Commit(); err != nil {
		p.mu.Unlock()
		return false, err
	}
	p.mu.Unlock()
	pc.framePending = true
	debug.Log("frame", "popup %d frame %dx%d", pc.surf.Id(), b.Width, b.Height)
	return true, nil
}

// Close releases the painter's pool storage back to the session arena.
func (pc *Painter) Close() { pc.pool.Close() }

// paintPlate fills the content rect with the popup background and,
// when the theme reserves a shadow gutter, paints the elevation around
// it first — the shared elevation look every popup carries, content
// kind aside. The plate rounds at the theme radius, matching the
// shadow silhouette and the menu plates painted on top of it.
func (pc *Painter) paintPlate(cv *render.Canvas, content render.Rect) {
	t := widget.Current()
	if pc.gutter > 0 {
		t.DrawShadow(cv, content, t.Radius)
		cv.RoundedRect(content, t.Radius, pc.bg)
		return
	}
	cv.FillRect(content, pc.bg)
}
