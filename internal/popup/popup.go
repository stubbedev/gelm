// Package popup manages transient surfaces with the xdg_popup role:
// menus and tooltips. New positions a popup relative to its parent via
// an xdg_positioner, grabs it for dismissal, and Run drives a nested
// render/dispatch loop until it closes.
package popup

import (
	"errors"
	"fmt"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"
	"github.com/neurlang/wayland/xdg"

	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// ErrClosed reports that the popup was dismissed; Run returns it when
// the loop exits.
var ErrClosed = errors.New("popup: dismissed")

// Popup is one open transient surface.
type Popup struct {
	WLSurface  *wl.Surface
	XdgSurface *xdg.Surface
	XdgPopup   *xdg.Popup

	w          int32
	h          int32
	configured bool
	closed     bool
	onClosed   func()
}

// Config describes where the popup goes and how big it is.
type Config struct {
	// Parent is the parent's xdg_surface; a popup always nests under a
	// shell surface.
	Parent *xdg.Surface
	// X, Y is the anchor point in parent surface coordinates.
	X, Y int
	// Width, Height is the popup's size in surface pixels.
	Width, Height int
	// Serial is the pointer or keyboard serial of the event that opens
	// the popup, used for the grab.
	Serial uint32
	// NoGrab skips the seat grab: the popup tracks hover without
	// taking input, which tooltips need.
	NoGrab bool
}

// New positions and maps a popup, then grabs the seat so clicks outside
// dismiss it. The configure handshake completes inside New: callers can
// draw immediately.
func New(sess *wlsession.Session, cfg Config) (*Popup, error) {
	wmBase := sess.WmBase()
	if wmBase == nil {
		return nil, errors.New("popup: compositor has no xdg_wm_base")
	}
	positioner, err := wmBase.CreatePositioner()
	if err != nil {
		return nil, fmt.Errorf("popup: create positioner: %w", err)
	}
	defer func() { _ = positioner.Destroy() }()
	if err := positioner.SetSize(int32(cfg.Width), int32(cfg.Height)); err != nil {
		return nil, err
	}
	// A 1x1 anchor rect at the pointer; gravity pushes the popup down
	// and right of it, sliding/flipping if that would leave the output.
	if err := positioner.SetAnchorRect(int32(cfg.X), int32(cfg.Y), 1, 1); err != nil {
		return nil, err
	}
	if err := positioner.SetAnchor(xdg.PositionerAnchorTopLeft); err != nil {
		return nil, err
	}
	if err := positioner.SetGravity(xdg.PositionerGravityBottomRight); err != nil {
		return nil, err
	}
	if err := positioner.SetConstraintAdjustment(1 | 2 | 8); err != nil {
		return nil, err
	}

	surf, err := sess.Compositor().CreateSurface()
	if err != nil {
		return nil, fmt.Errorf("popup: create surface: %w", err)
	}
	debug.Log("input", "popup surface %d created", surf.Id())
	p := &Popup{WLSurface: surf, w: int32(cfg.Width), h: int32(cfg.Height)}

	xdgSurf, err := wmBase.GetSurface(surf)
	if err != nil {
		return nil, fmt.Errorf("popup: get xdg surface: %w", err)
	}
	pop, err := xdgSurf.GetPopup(cfg.Parent, positioner)
	if err != nil {
		return nil, fmt.Errorf("popup: get popup: %w", err)
	}
	p.XdgSurface, p.XdgPopup = xdgSurf, pop
	debug.Log("input", "popup xdg_surface %d xdg_popup %d", xdgSurf.Id(), pop.Id())
	xdgSurf.AddConfigureHandler(p)
	pop.AddConfigureHandler(p)
	pop.AddPopupDoneHandler(p)

	if err := surf.Commit(); err != nil {
		return nil, fmt.Errorf("popup: commit: %w", err)
	}
	for range 20 {
		if p.EnsureUsable() == nil {
			break
		}
		if err := sess.Roundtrip(); err != nil {
			return nil, fmt.Errorf("popup: configure roundtrip: %w", err)
		}
	}
	if err := p.EnsureUsable(); err != nil {
		return nil, fmt.Errorf("popup: %w", err)
	}
	if !cfg.NoGrab {
		if err := pop.Grab(sess.Seat(), cfg.Serial); err != nil {
			return nil, fmt.Errorf("popup: grab: %w", err)
		}
		if err := sess.Roundtrip(); err != nil {
			return nil, fmt.Errorf("popup: grab roundtrip: %w", err)
		}
	}
	return p, nil
}

// HandleSurfaceConfigure completes the configure round.
func (p *Popup) HandleSurfaceConfigure(ev xdg.SurfaceConfigureEvent) {
	p.configured = true
	if p.XdgSurface != nil {
		_ = p.XdgSurface.AckConfigure(ev.Serial)
	}
}

// HandlePopupConfigure records the placed geometry.
func (p *Popup) HandlePopupConfigure(ev xdg.PopupConfigureEvent) {
	if ev.Width != 0 {
		p.w = ev.Width
	}
	if ev.Height != 0 {
		p.h = ev.Height
	}
}

// HandlePopupDone implements dismissal: the user clicked away.
func (p *Popup) HandlePopupPopupDone(xdg.PopupPopupDoneEvent) {
	p.closed = true
	if p.onClosed != nil {
		p.onClosed()
	}
}

// EnsureUsable gates drawing until the first configure completed and the
// popup is not dismissed.
func (p *Popup) EnsureUsable() error {
	if p.closed {
		return ErrClosed
	}
	if !p.configured {
		return errors.New("popup: not configured yet")
	}
	return nil
}

// Closed reports whether the popup was dismissed.
func (p *Popup) Closed() bool { return p.closed }

// Size returns the placed popup size.
func (p *Popup) Size() (int, int) { return int(p.w), int(p.h) }

// HostSurface returns the underlying wl_surface.
func (p *Popup) HostSurface() *wl.Surface { return p.WLSurface }

// SetOnClosed runs f when the popup is dismissed.
func (p *Popup) SetOnClosed(f func()) { p.onClosed = f }

// Close dismisses the popup from the client side.
func (p *Popup) Close() {
	if p.XdgPopup != nil {
		_ = p.XdgPopup.Destroy()
	}
	p.closed = true
	if p.onClosed != nil {
		p.onClosed()
	}
}

// Run drives the popup's own render loop until it is dismissed: paint
// through paint each frame, dispatch input through the session. The
// popup surface registers its own pointer handler, so events land here
// only while the compositor routes focus (or the popup grab) to it -
// the host surface keeps receiving nothing and no hooks are swapped.
// A non-nil keys router receives seat keyboard events translated to
// KeyActions, which menus need for arrow and Enter navigation.
func Run(sess *wlsession.Session, p *Popup, scale int, root widget.Widget, bg render.Color, keys *widget.Router) error {
	surf := p.HostSurface()
	create := func() (*buffer.Buffer, error) {
		w, h := p.Size()
		return buffer.NewFile(sess.Shm(), w*scale, h, scale)
	}
	pool := buffer.New(create, 2)

	router := &widget.Router{Root: root}
	var pointer struct{ x, y float64 }
	dirty := true
	input := &popupInput{router: router, scale: scale, pointer: &pointer, markDirty: func() { dirty = true }}
	sess.SetSurfaceInput(p.WLSurface, input)
	defer sess.SetSurfaceInput(p.WLSurface, nil)

	if keys != nil {
		prevKey := sess.OnKey
		sess.OnKey = func(code uint32, mods wlsession.Mods) {
			if a, ok := widget.KeyActionForSym(sess.KeySym(code)); ok {
				keys.KeyAction(a, widget.Mods(mods))
			}
		}
		defer func() { sess.OnKey = prevKey }()
	}

	frameReady := false
	framePending := false
	for !p.Closed() {
		if frameReady {
			frameReady = false
			framePending = false
		}
		if dirty && !framePending {
			dirty = false
			b, err := pool.Acquire()
			if errors.Is(err, buffer.ErrBusy) {
				// The release event wakes the park below; stay dirty.
				dirty = true
			} else if err != nil {
				return fmt.Errorf("popup: acquire buffer: %w", err)
			} else {
				wlclient.BufferAddListener(b.WL, buffer.ReleaseHandler{B: b})

				w, h := p.Size()
				root.Measure(widget.Constraints{Max: widget.Size{W: w, H: h}})
				root.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})

				cv := render.New(b.Data, b.Stride, b.Width, b.Height)
				cv.Clear(cv.Rect(), bg)
				root.Paint(cv)

				if err := surf.Attach(b.WL, 0, 0); err != nil {
					return fmt.Errorf("popup: attach: %w", err)
				}
				if err := surf.DamageBuffer(0, 0, int32(b.Width), int32(b.Height)); err != nil {
					return fmt.Errorf("popup: damage: %w", err)
				}
				if err := surf.Commit(); err != nil {
					return fmt.Errorf("popup: commit: %w", err)
				}

				cb, err := surf.Frame()
				if err != nil {
					return fmt.Errorf("popup: frame callback: %w", err)
				}
				wlclient.CallbackAddListener(cb, frameDone{ready: &frameReady})
				framePending = true
			}
		}
		// Park: dismissal, pointer events, and buffer releases are all
		// events; nothing here polls.
		if err := sess.Step(); err != nil {
			return fmt.Errorf("popup: dispatch: %w", err)
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

// popupInput routes one popup surface's pointer events into its widget
// tree. It implements wlsession.SurfacePointerHandler.
type popupInput struct {
	router    *widget.Router
	scale     int
	pointer   *struct{ x, y float64 }
	markDirty func()
}

// HandlePointerEnter implements wlsession.SurfacePointerHandler.
func (in *popupInput) HandlePointerEnter(x, y float64) { in.move(x, y) }

// HandlePointerMotion implements wlsession.SurfacePointerHandler.
func (in *popupInput) HandlePointerMotion(x, y float64) { in.move(x, y) }

func (in *popupInput) move(x, y float64) {
	in.pointer.x, in.pointer.y = x, y
	in.router.Move(widget.Point{X: int(x) * in.scale, Y: int(y) * in.scale})
	in.markDirty()
}

// HandlePointerButton implements wlsession.SurfacePointerHandler.
func (in *popupInput) HandlePointerButton(button, state, serial uint32) {
	pt := widget.Point{X: int(in.pointer.x) * in.scale, Y: int(in.pointer.y) * in.scale}
	if state == 1 {
		in.router.Press(button, pt)
	} else {
		in.router.Release(button, pt)
	}
	in.markDirty()
}

// HandlePointerAxis implements wlsession.SurfacePointerHandler.
func (in *popupInput) HandlePointerAxis(dx, dy float64) {}

// HandlePointerLeave implements wlsession.SurfacePointerHandler.
func (in *popupInput) HandlePointerLeave() { in.router.Leave() }
