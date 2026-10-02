package app

import (
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/xdg"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/popup"
	"github.com/stubbedev/gelm/internal/scale"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
	"github.com/stubbedev/gelm/wlr"
)

// Gravity is the side of its anchor a popover opens on.
type Gravity = popup.Gravity

// Popover gravities; the compositor flips to the opposite side when
// there is no room on the asked-for one.
const (
	GravityBottom = popup.GravityBottom
	GravityTop    = popup.GravityTop
	GravityRight  = popup.GravityRight
	GravityLeft   = popup.GravityLeft
)

// PopoverConfig declares a popover anchored to a widget.
type PopoverConfig struct {
	// Anchor is the widget to open beside; its Bounds must be current
	// (any arranged widget qualifies).
	Anchor widget.Boundser
	// Gravity picks the side; zero opens below the anchor.
	Gravity Gravity
	// Content is the popover's widget tree (a Menu is the common
	// case).
	Content widget.Widget
	// OnClosed runs exactly once when the popover goes away, whatever
	// the cause.
	OnClosed func()
	// Serial is the pointer serial of the opening event, for the grab.
	Serial uint32
	// Focus, when set, is the widget inside Content that takes keyboard
	// focus as the popover opens (a form's first Entry, as GTK focuses
	// a popover's first input): typing then reaches it without a click.
	Focus widget.Widget
	// Parent, when set, nests the popover under that open popover (an
	// xdg_popup child of its surface, GTK's nested PopoverMenu): Anchor
	// is a widget inside the parent, the host argument is ignored, and
	// the popover closes with its parent. A parent holds one child at a
	// time: opening another closes the one before.
	Parent *Popover
}

// Popover is a widget-anchored transient popup. It closes on
// click-away (the popup grab), Esc, or Dismiss; OnClosed fires exactly
// once.
type Popover struct {
	mu      sync.Mutex
	closed  bool
	closeFn func()
	// focus moves the open popover's keyboard focus (loop only).
	focus func(w widget.Widget)

	// host is the window the popover (or its root) opened over, pop its
	// surface, teardown its no-tween close; parent and children link a
	// nested chain (loop only).
	host     Host
	pop      *popup.Popup
	teardown func()
	parent   *Popover
	children []*Popover
}

// Parent is the popover this one is nested under, nil for a root.
func (p *Popover) Parent() *Popover { return p.parent }

// Child is the open popover nested under this one, nil without one.
func (p *Popover) Child() *Popover {
	if p == nil || len(p.children) == 0 {
		return nil
	}
	return p.children[len(p.children)-1]
}

// Root is the outermost popover of the chain.
func (p *Popover) Root() *Popover {
	for p != nil && p.parent != nil {
		p = p.parent
	}
	return p
}

// closeChildren tears every nested popover down on the spot, deepest
// first through each one's own close: a child surface must be gone
// before its parent's (xdg_popup's topmost rule), and an exit tween
// would outlive the parent's.
func (p *Popover) closeChildren() {
	for len(p.children) > 0 {
		child := p.children[len(p.children)-1]
		p.children = p.children[:len(p.children)-1]
		if child.teardown != nil {
			child.teardown()
		}
	}
}

// settle is a popover's close bookkeeping, whatever closed it: the
// first call flips it closed, closes what nests under it, and leaves
// its parent's list, reporting true; later calls report false.
func (p *Popover) settle() bool {
	if !p.markClosed() {
		return false
	}
	p.closeChildren()
	if p.parent != nil {
		p.parent.dropChild(p)
	}
	return true
}

// dropChild forgets a child that closed on its own.
func (p *Popover) dropChild(child *Popover) {
	p.children = slices.DeleteFunc(p.children, func(c *Popover) bool { return c == child })
}

// chainPressSerial is the newest press serial over the chain's
// surfaces: the grab serial a popover nested under p opens with.
func (p *Popover) chainPressSerial() uint32 {
	var serial uint32
	for at := p; at != nil; at = at.parent {
		if at.pop != nil {
			serial = max(serial, at.pop.LastPressSerial())
		}
	}
	return serial
}

// SetFocus moves keyboard focus to w inside the open popover - a form
// that appears in it takes typing at once (GTK's grab_focus). A w the
// router cannot focus is ignored (see widget.Router.SetFocus); nil
// clears focus. Loop goroutine only.
func (p *Popover) SetFocus(w widget.Widget) {
	if p != nil && p.focus != nil && !p.Closed() {
		p.focus(w)
	}
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

// popoverKeyRoot wraps popover content with Esc dismissal and the
// raw-key routes a popup owes: Alt-letter presses activate the
// content's mnemonics first (an open menu owns its Alt-letters), then
// application-wide accelerators fire from the very table the main
// routeKey path uses — so a combo fires exactly once whichever path
// sees it, and the two can never disagree.
type popoverKeyRoot struct {
	onDismiss func()
	content   widget.Widget
	// fireAccel fires the application's accelerators for one raw key,
	// reporting whether one fired.
	fireAccel func(sym xkb.Keysym, mods wlsession.Mods) bool
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

// Children exposes the content to the tree walks (damage, focus).
func (r *popoverKeyRoot) Children() []widget.Widget { return []widget.Widget{r.content} }

// HitTest is the content's own hit, so the widget under the pointer
// (a button, an entry) takes the press, and the root itself off the
// content.
func (r *popoverKeyRoot) HitTest(p widget.Point) widget.Widget {
	if hit := r.content.HitTest(p); hit != nil {
		return hit
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

// RawKey implements widget.RawKeyHandler: the popover's mnemonics and
// accelerators, in that order.
func (r *popoverKeyRoot) RawKey(code uint32, mods widget.Mods, sym xkb.Keysym) bool {
	if mods&widget.ModAlt != 0 && mods&widget.ModCtrl == 0 {
		if widget.ActivateMnemonic(r.content, sym) {
			return true
		}
	}
	return r.fireAccel != nil && r.fireAccel(sym, wlsession.Mods(mods))
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
	parent := cfg.Parent
	if parent != nil {
		if parent.Closed() || parent.pop == nil {
			return nil, errors.New("app: the parent popover is closed")
		}
		host = parent.host
		parent.closeChildren()
	} else if prev := a.popovers.take(host); prev != nil {
		prev.Dismiss()
	}

	anchor := cfg.Anchor.Bounds()
	bw, _ := host.Size()
	// The popover's content styles as the `popover` element (css.md):
	// whatever widget tree the app hands over IS the card.
	nameSurfaceElement(cfg.Content, elemPopover)
	size := cfg.Content.Measure(widget.Constraints{Max: widget.Size{W: bw, H: popoverMaxH}})
	// The positioner anchors to the anchor widget's rect on the gravity
	// side and the compositor flips or slides it against the output (a
	// bar is never tall enough to judge room by). The shadow gutter rides
	// on the surface: it grows the popup on every side and the
	// positioner offset keeps the CONTENT (not the gutter) aligned. The theme can
	// turn it off (ShadowGutter 0), collapsing to the exact pre-shadow
	// geometry.
	gutter := widget.Current().ShadowGutter()
	debug.Log("input", "popover anchor %+v size %dx%d gravity %d gutter %d", anchor, size.W, size.H, cfg.Gravity, gutter)

	p := &Popover{host: host, parent: parent}
	keyRoot := &popoverKeyRoot{
		onDismiss: p.Dismiss,
		content:   cfg.Content,
		fireAccel: func(sym xkb.Keysym, mods wlsession.Mods) bool {
			// App-wide bindings only: the popup holds the keyboard, so
			// there is no focused widget whose bindings could own the
			// combo — exactly the GTK rule that an open menu fires the
			// accelerator group and nothing else.
			return a.accels.fire(nil, sym, mods)
		},
	}

	pcfg := popup.Config{
		AnchorRect: anchor,
		Width:      size.W + 2*gutter, Height: size.H + 2*gutter,
		Gutter:  gutter,
		Gravity: cfg.Gravity,
		Serial:  cfg.Serial,
	}
	switch {
	case parent != nil:
		// A nested popup grabs with the newest press anywhere on the
		// chain: the click that opened it may have landed in the parent.
		pcfg.Parent = parent.pop.XdgSurface
		if pcfg.Serial == 0 {
			pcfg.Serial = max(parent.chainPressSerial(), a.LastPressSerial(host))
		}
	default:
		var ok bool
		if pcfg.Parent, pcfg.LayerParent, ok = popupParentOf(host); !ok {
			return nil, errors.New("app: host cannot carry popups")
		}
	}

	// A bar-style layer declines the keyboard; the popover's grab needs
	// it (Esc, menus, a form's Entry), so the layer takes keys on demand
	// while the popover is open, as the Rust bar does around a dropdown.
	// A nested popover rides on its root's.
	restoreKeys := func() {}
	if km, ok := host.(keyboardModer); ok && parent == nil && km.KeyboardMode() == KeyboardNone {
		if err := km.SetKeyboardMode(KeyboardOnDemand); err == nil {
			restoreKeys = func() { _ = km.SetKeyboardMode(KeyboardNone) }
		}
	}
	pop, err := popup.New(a.sess, pcfg)
	if err != nil {
		restoreKeys()
		return nil, err
	}
	// Dismiss, not Close: the programmatic path takes the same two-
	// phase route as outside-clicks and Esc — input seals, OnClosed
	// fires, and the surface runs its exit tween before the wire
	// teardown.
	p.closeFn = pop.Dismiss
	p.teardown = pop.Close
	p.pop = pop
	fireClosed := func() {
		// Whatever closed this popover (Esc, a click away, the
		// compositor's popup_done, Dismiss) closes what nests under it.
		if !p.settle() {
			return
		}
		restoreKeys()
		if cfg.OnClosed != nil {
			cfg.OnClosed()
		}
	}
	pop.SetOnClosed(fireClosed)

	if parent != nil {
		parent.children = append(parent.children, p)
	} else {
		a.popovers.openOrReplace(host, p)
	}
	// The application loop drives the popover like any of its windows:
	// one painter pass per loop pass, pointer input through a Router on
	// the loop goroutine, keys through the same Router once a widget
	// holds focus. Content the app mutates from Invoke repaints on the
	// next pass, and nothing paints from a second goroutine.
	router := &widget.Router{Root: keyRoot}
	op := &openPopover{
		host:       host,
		pop:        pop,
		painter:    pop.NewPainter(a.sess, a.fracFor(host), keyRoot, widget.Current().Surface),
		router:     router,
		keyRoot:    keyRoot,
		popover:    p,
		fireClosed: fireClosed,
	}
	if cfg.Focus != nil {
		router.SetFocus(cfg.Focus)
	}
	p.focus = func(w widget.Widget) {
		router.SetFocus(w)
		op.pop.MarkFrame()
	}
	op.detach = pop.AttachInput(a.sess, router)
	a.openPopovers = append(a.openPopovers, op)
	return p, nil
}

// fracFor returns host's current 120-based device scale, so popups and
// other one-shot surfaces match the window they open over; 120 (1x)
// before the first rescale or for unknown hosts.
func (a *Application) fracFor(host Host) uint32 {
	for _, w := range a.windows {
		if w.host == host {
			return w.frac120
		}
	}
	return scale.Denom
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

// popSurface is the popup half the loop drives (a *popup.Popup).
type popSurface interface {
	Destroyed() bool
	Dismissed() bool
	MarkFrame()
	// RequestedSize, Gutter and Resize fit the surface to its content.
	RequestedSize() (int, int)
	Gutter() int
	Resize(w, h int) bool
}

// popPainter is the frame pipeline half (a *popup.Painter).
type popPainter interface {
	Pass() (bool, error)
	Close()
}

// openPopover is one popover the application loop drives.
type openPopover struct {
	host       Host
	pop        popSurface
	painter    popPainter
	router     *widget.Router
	keyRoot    *popoverKeyRoot
	popover    *Popover
	detach     func()
	fireClosed func()
}

// drivePopovers runs one loop pass over the open popovers: a destroyed
// one (its exit tween landed) releases its painter and input and fires
// its close; a live one repaints when anything changed — all marks it
// dirty (loop work ran that may have touched its content) — and paints
// at most one frame.
func (a *Application) drivePopovers(all bool) error {
	// Newest first: a nested popover destroyed in the same pass as its
	// parent must leave the wire before it (xdg_popup's topmost rule).
	kept := make([]*openPopover, 0, len(a.openPopovers))
	for _, op := range slices.Backward(a.openPopovers) {
		if op.pop.Destroyed() {
			op.painter.Close()
			op.detach()
			if a.popovers.open[op.host] == op.popover {
				a.popovers.take(op.host)
			}
			op.fireClosed()
			continue
		}
		if _, damaged := widget.CollectDamage(op.keyRoot); damaged || all {
			op.fitContent()
			op.pop.MarkFrame()
		}
		if _, err := op.painter.Pass(); err != nil {
			return fmt.Errorf("app: popover paint: %w", err)
		}
		kept = append(kept, op)
	}
	slices.Reverse(kept)
	a.openPopovers = kept
	return nil
}

// fitContent grows (or shrinks) an open popover to its content's
// natural size when that changed - a tray menu re-publishing more rows
// - measured as at open, against the host's width and a 600px cap.
func (op *openPopover) fitContent() {
	if op.pop.Dismissed() {
		return
	}
	bw, _ := op.host.Size()
	size := op.keyRoot.content.Measure(widget.Constraints{Max: widget.Size{W: bw, H: popoverMaxH}})
	g := op.pop.Gutter()
	w, h := size.W+2*g, size.H+2*g
	if rw, rh := op.pop.RequestedSize(); rw != w || rh != h {
		op.pop.Resize(w, h)
	}
}

// keyPopover is the popover holding the keyboard: the newest one still
// taking input. An open popover grabs the seat, so keys never reach
// the windows beneath it.
func (a *Application) keyPopover() *openPopover {
	for _, op := range slices.Backward(a.openPopovers) {
		if !op.pop.Dismissed() {
			return op
		}
	}
	return nil
}

// deliverPopoverKey routes one press inside a popover. Esc dismisses
// whatever has focus (GTK's popover rule). With a focused widget — an
// Entry the user clicked — the press takes the windows' full key path
// (editing, clipboard, focus traversal, text); without one, the
// actions and raw keys go to the popover root, which forwards them to
// the content (a menu's arrows, mnemonics, accelerators).
func (a *Application) deliverPopoverKey(tr keyTranslator, op *openPopover, keycode uint32, mods wlsession.Mods) {
	defer op.pop.MarkFrame()
	sym := tr.KeySym(keycode)
	act, isAction := widget.KeyActionForSym(sym)
	if isAction && act == widget.KeyDismiss {
		op.keyRoot.onDismiss()
		return
	}
	if op.router.Focused() != nil {
		routeKey(tr, op.router, keycode, mods, a.clip, a.accels, nil)
		return
	}
	if isAction {
		op.keyRoot.KeyAction(act, widget.Mods(mods))
		return
	}
	_ = op.keyRoot.RawKey(keycode, widget.Mods(mods), sym)
}

// keyboardModer is a host whose keyboard interactivity a popover can
// raise while open (*LayerWindow).
type keyboardModer interface {
	KeyboardMode() KeyboardMode
	SetKeyboardMode(KeyboardMode) error
}

// popoverMaxH caps a popover's height when its content is measured.
const popoverMaxH = 600

// popupParentOf is the surface a popup on host hangs from: a layer's
// layer surface, a toplevel's (Window or internal host) xdg surface.
func popupParentOf(host Host) (xdgParent *xdg.Surface, layerParent *wlr.ZwlrLayerSurfaceV1, ok bool) {
	switch h := host.(type) {
	case LayerSurfacer:
		return nil, h.LayerPopupSurface(), true
	case *Window:
		if h.win != nil && h.win.XdgSurface != nil {
			return h.win.XdgSurface, nil, true
		}
	case tooltipSurfacer:
		return h.TooltipSurface(), nil, true
	}
	return nil, nil, false
}
