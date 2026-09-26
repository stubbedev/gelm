package app

import (
	"errors"
	"sync"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/internal/popup"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
	"github.com/stubbedev/gelm/wlr"
)

// PopoverConfig declares a popover anchored to a widget.
type PopoverConfig struct {
	// Anchor is the widget to open beside; its Bounds must be current
	// (any arranged widget qualifies).
	Anchor widget.Boundser
	// Gravity picks the side; zero opens below the anchor.
	Gravity popup.Gravity
	// Content is the popover's widget tree (a Menu is the common
	// case).
	Content widget.Widget
	// OnClosed runs exactly once when the popover goes away, whatever
	// the cause.
	OnClosed func()
	// Serial is the pointer serial of the opening event, for the grab.
	Serial uint32
}

// Popover is a widget-anchored transient popup. It closes on
// click-away (the popup grab), Esc, or Dismiss; OnClosed fires exactly
// once.
type Popover struct {
	mu      sync.Mutex
	closed  bool
	closeFn func()
}

// Dismiss closes the popover programmatically; OnClosed still fires
// exactly once.
func (p *Popover) Dismiss() {
	if p != nil && p.closeFn != nil {
		p.closeFn()
	}
}

// Closed reports whether the popover has gone away.
func (p *Popover) Closed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// markClosed flips the closed flag and reports whether this call is
// the first, so OnClosed hooks fire exactly once.
func (p *Popover) markClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	p.closed = true
	return true
}

// popoverRegistry tracks the one open popover per host; opening a new
// one on the same host re-anchors by closing the previous.
type popoverRegistry struct {
	open map[Host]*Popover
}

// openOrReplace registers p for host and returns the popover it
// replaced, if any; the caller closes it.
func (r *popoverRegistry) openOrReplace(h Host, p *Popover) *Popover {
	if r.open == nil {
		r.open = make(map[Host]*Popover)
	}
	prev := r.open[h]
	r.open[h] = p
	return prev
}

// take removes and returns the popover registered for host.
func (r *popoverRegistry) take(h Host) *Popover {
	p := r.open[h]
	delete(r.open, h)
	return p
}

// anchorOrigin picks the popup's top-left for the requested gravity,
// flipping to the opposite side when the content would leave the host
// bounds, and clamping along the perpendicular axis. This is the
// testable half of the positioner setup.
func anchorOrigin(host, anchor render.Rect, size widget.Size, g popup.Gravity) (int, int) {
	x, y := anchor.X, anchor.Y
	switch g {
	case popup.GravityTop:
		y = anchor.Y - size.H
		if y < host.Y {
			y = anchor.Y + anchor.H
		}
	case popup.GravityRight:
		x = anchor.X + anchor.W
		if x+size.W > host.X+host.W {
			x = anchor.X - size.W
		}
	case popup.GravityLeft:
		x = anchor.X - size.W
		if x < host.X {
			x = anchor.X + anchor.W
		}
	default:
		y = anchor.Y + anchor.H
		if y+size.H > host.Y+host.H {
			y = anchor.Y - size.H
		}
	}
	if g == popup.GravityTop || g == popup.GravityBottom {
		x = min(max(x, host.X), max(host.X, host.X+host.W-size.W))
	} else {
		y = min(max(y, host.Y), max(host.Y, host.Y+host.H-size.H))
	}
	return x, y
}

// popoverKeyRoot wraps popover content with Esc dismissal.
type popoverKeyRoot struct {
	onDismiss func()
	content   widget.Widget
	bounds    render.Rect
}

func (r *popoverKeyRoot) Measure(con widget.Constraints) widget.Size {
	return r.content.Measure(con)
}

func (r *popoverKeyRoot) Arrange(rect render.Rect) {
	r.bounds = rect
	r.content.Arrange(rect)
}

func (r *popoverKeyRoot) Paint(cv *render.Canvas) { r.content.Paint(cv) }

func (r *popoverKeyRoot) HitTest(p widget.Point) widget.Widget {
	if r.content.HitTest(p) != nil {
		return r.content
	}
	return r
}

func (r *popoverKeyRoot) KeyAction(a widget.KeyAction, mods widget.Mods) {
	if a == widget.KeyDismiss {
		r.onDismiss()
		return
	}
	if h, ok := r.content.(widget.KeyActionHandler); ok {
		h.KeyAction(a, mods)
	}
}

// LayerSurfacer is implemented by layer-shell hosts so popovers can
// parent to the layer surface, which has no xdg_surface of its own.
type LayerSurfacer interface {
	// LayerPopupSurface returns the layer surface popups attach to.
	LayerPopupSurface() *wlr.ZwlrLayerSurfaceV1
}

// OpenPopover opens content beside anchor on the given host, on the
// side gravity asks for, flipping inside the host when the anchor sits
// near an edge. Opening a second popover on the same host re-anchors:
// the previous popover closes first. The popover's own key router
// dismisses on Esc and forwards everything else to the content.
func (a *Application) OpenPopover(host Host, cfg PopoverConfig) (*Popover, error) {
	if cfg.Anchor == nil || cfg.Content == nil {
		return nil, errors.New("app: popover needs an anchor and content")
	}
	if prev := a.popovers.take(host); prev != nil {
		prev.Dismiss()
	}

	anchor := cfg.Anchor.Bounds()
	bw, bh := host.Size()
	size := cfg.Content.Measure(widget.Constraints{Max: widget.Size{W: bw, H: 600}})
	x, y := anchorOrigin(render.Rect{W: bw, H: bh}, anchor, size, cfg.Gravity)

	p := &Popover{}
	keyRoot := &popoverKeyRoot{onDismiss: p.Dismiss, content: cfg.Content}
	keys := &widget.Router{Root: keyRoot}

	pcfg := popup.Config{
		X: x, Y: y,
		Width: size.W, Height: size.H,
		Gravity: cfg.Gravity,
		Serial:  cfg.Serial,
	}
	layer, onLayer := host.(LayerSurfacer)
	if onLayer {
		pcfg.LayerParent = layer.LayerPopupSurface()
	} else if ts, ok := host.(tooltipSurfacer); ok {
		pcfg.Parent = ts.TooltipSurface()
	} else {
		return nil, errors.New("app: host cannot carry popups")
	}

	pop, err := popup.New(a.sess, pcfg)
	if err != nil {
		return nil, err
	}
	p.closeFn = pop.Close
	fireClosed := func() {
		if p.markClosed() && cfg.OnClosed != nil {
			cfg.OnClosed()
		}
	}
	pop.SetOnClosed(fireClosed)

	a.popovers.openOrReplace(host, p)
	go func() {
		_ = popup.Run(a.sess, pop, 1, keyRoot, widget.Current().Surface, keys)
		a.popovers.take(host)
		fireClosed()
	}()
	return p, nil
}

// EnsureUsable implements Host for the toplevel handle.
func (w *Window) EnsureUsable() error { return w.win.EnsureUsable() }

// Size implements Host for the toplevel handle.
func (w *Window) Size() (int, int) { return w.win.Size() }

// HostSurface implements Host for the toplevel handle.
func (w *Window) HostSurface() *wl.Surface { return w.win.HostSurface() }

// EnsureUsable implements Host for the layer handle.
func (l *LayerWindow) EnsureUsable() error { return l.ls.EnsureUsable() }

// Size implements Host for the layer handle.
func (l *LayerWindow) Size() (int, int) { return l.ls.Size() }

// HostSurface implements Host for the layer handle.
func (l *LayerWindow) HostSurface() *wl.Surface { return l.ls.HostSurface() }

// LayerPopupSurface implements LayerSurfacer for the layer handle.
func (l *LayerWindow) LayerPopupSurface() *wlr.ZwlrLayerSurfaceV1 {
	return l.ls.LayerPopupSurface()
}
