package app

import (
	"slices"
	"sync"
	"time"

	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/sysfont"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// ToastPosition picks the window edge a toast stack hugs.
type ToastPosition uint8

const (
	// ToastBottom stacks toasts above the bottom edge; zero, the
	// default.
	ToastBottom ToastPosition = iota
	// ToastTop stacks toasts below the top edge.
	ToastTop
)

const (
	// toastMargin keeps the stack off the window edge by default, and
	// toastSpacing separates stacked cards.
	toastMargin  = 16
	toastSpacing = 8
	// toastMaxStack caps the stack: a burst of toasts drops its oldest
	// members instead of burying the window.
	toastMaxStack = 3
	// toastMaxWidth clamps a card's width so a long line cannot span
	// the window.
	toastMaxWidth = 420
)

// ToastConfig declares a toast's placement: which window hosts it, the
// edge the stack hugs, the stack cap, and the text face.
type ToastConfig struct {
	// Host picks the window; nil shows the toast on the focused
	// window, or the first one.
	Host Host
	// Position picks the edge; zero is the bottom.
	Position ToastPosition
	// Margin keeps the stack off the edge; zero uses the default.
	Margin int
	// MaxStack caps simultaneous toasts, oldest dropping first; zero
	// uses the default.
	MaxStack int
	// Face renders the text; nil falls back to the tooltip face, then
	// the default sans face.
	Face *render.Typeface
}

// ShowToast shows a transient feedback card over a window: bottom-
// centered by default (ToastConfig moves it), auto-dismissed after
// timeout, hover-pausable, optionally actionable through the returned
// toast (widget.Toast.SetAction). Toasts stack on the host window up
// to the cap, the oldest dropping first; when the last one goes, the
// window's tree is back to exactly what it was. A nil config uses the
// defaults. Without a window to host it, ShowToast returns nil.
func (a *Application) ShowToast(text string, timeout time.Duration, cfg *ToastConfig) *widget.Toast {
	if cfg == nil {
		cfg = &ToastConfig{}
	}
	hw := a.windowForToast(cfg.Host)
	if hw == nil {
		debug.Log("input", "toast dropped: no window to show it on")
		return nil
	}
	face := a.toastFace(cfg.Face)
	margin := cfg.Margin
	if margin <= 0 {
		margin = toastMargin
	}
	maxStack := cfg.MaxStack
	if maxStack <= 0 {
		maxStack = toastMaxStack
	}
	layer := a.toasts.layerFor(hw.host, func() *toastLayer {
		l := &toastLayer{hw: hw, root: hw.router.Root}
		// The first toast wraps the window's tree; the last removal
		// unwraps it.
		hw.router.Root = l
		hw.dirty = true
		return l
	})
	layer.pos = cfg.Position
	layer.margin = margin
	layer.max = maxStack
	t := widget.NewToast(face, text, timeout)
	t.OnDismissed = func() { layer.remove(t) }
	layer.add(t)
	return t
}

// toastFace resolves the toast text face: the config's pick, else the
// tooltip face, else the default sans face.
func (a *Application) toastFace(asked *render.Typeface) *render.Typeface {
	if asked != nil {
		return asked
	}
	if a.tooltipFace != nil {
		return a.tooltipFace
	}
	f, err := sysfont.Sans()
	if err != nil {
		debug.Log("input", "toast face: %v", err)
		return nil
	}
	return f
}

// windowForToast resolves the hosting window: the one asked for, else
// the focused window, else the first.
func (a *Application) windowForToast(h Host) *hostWindow {
	if h != nil {
		for _, w := range a.windows {
			if w.host == h {
				return w
			}
		}
		return nil
	}
	if len(a.windows) == 0 {
		return nil
	}
	return a.focused()
}

// toastRegistry tracks each host's toast layer.
type toastRegistry struct {
	mu   sync.Mutex
	open map[Host]*toastLayer
}

// layerFor returns the host's layer, building it with mk on first use.
func (r *toastRegistry) layerFor(h Host, mk func() *toastLayer) *toastLayer {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.open == nil {
		r.open = make(map[Host]*toastLayer)
	}
	if l := r.open[h]; l != nil {
		return l
	}
	l := mk()
	r.open[h] = l
	return l
}

// closeHost tears down a closing window's toast layer: its toasts
// stop holding timers, so the animation clock parks after the surface
// they painted on is gone.
func (r *toastRegistry) closeHost(h Host) {
	r.mu.Lock()
	l := r.open[h]
	delete(r.open, h)
	r.mu.Unlock()
	if l != nil {
		l.close()
	}
}

// toastLayer hosts one window's toast stack: the wrapped root keeps
// the whole window rect and the toasts lay out at the configured
// edge, newest toward the interior. It is a hand-rolled
// widget.Widget — app-side trees cannot embed widget.node — so its
// structural changes repaint through the window's dirty flag, while
// each toast invalidates its own pixels through the damage collector
// (which descends through Children).
type toastLayer struct {
	hw   *hostWindow
	root widget.Widget
	pos  ToastPosition
	// margin, max are the stack geometry; add refreshes them from the
	// latest toast's config.
	margin, max int
	slots       []*widget.Toast
}

// Children exposes the wrapped root and the live toasts in paint
// order, so damage collection and focus traversal descend.
func (l *toastLayer) Children() []widget.Widget {
	out := make([]widget.Widget, 0, len(l.slots)+1)
	out = append(out, l.root)
	for _, t := range l.slots {
		out = append(out, t)
	}
	return out
}

// Measure passes through to the wrapped root: the toasts float, the
// window's natural size is unchanged.
func (l *toastLayer) Measure(con widget.Constraints) widget.Size {
	return l.root.Measure(con)
}

// Arrange lays the root over the whole rect and stacks the toasts at
// the configured edge, centered along it: the newest card hugs the
// edge, older ones grow toward the interior.
func (l *toastLayer) Arrange(r render.Rect) {
	l.root.Arrange(r)
	if len(l.slots) == 0 {
		return
	}
	maxW := min(max(0, r.W-2*l.margin), toastMaxWidth)
	con := widget.Constraints{Max: widget.Size{W: maxW, H: max(0, r.H/2)}}
	if l.pos == ToastTop {
		y := r.Y + l.margin
		for _, t := range slices.Backward(l.slots) {
			rect := l.slotRect(t, con, r, y)
			t.Arrange(rect)
			y += rect.H + toastSpacing
		}
		return
	}
	y := r.Y + r.H - l.margin
	for _, t := range slices.Backward(l.slots) {
		rect := l.slotRect(t, con, r, y)
		rect.Y -= rect.H
		t.Arrange(rect)
		y -= rect.H + toastSpacing
	}
}

// slotRect measures one toast against the stack constraints and
// centers it horizontally at baseline y (the card's top edge for a
// top stack; the caller flips it for a bottom stack).
func (l *toastLayer) slotRect(t *widget.Toast, con widget.Constraints, r render.Rect, y int) render.Rect {
	nat := t.Measure(con)
	return render.Rect{
		X: r.X + max(0, (r.W-nat.W)/2),
		Y: y,
		W: min(nat.W, max(0, r.W-2*l.margin)),
		H: nat.H,
	}
}

// Paint draws the wrapped root, then the toasts on top.
func (l *toastLayer) Paint(cv *render.Canvas) {
	l.root.Paint(cv)
	for _, t := range l.slots {
		t.Paint(cv)
	}
}

// HitTest resolves toasts first (they float above the root), then the
// wrapped root.
func (l *toastLayer) HitTest(p widget.Point) widget.Widget {
	for _, t := range slices.Backward(l.slots) {
		if hit := t.HitTest(p); hit != nil {
			return hit
		}
	}
	return l.root.HitTest(p)
}

// add appends a toast, dropping — and thereby closing — the oldest
// past the cap. Structural changes repaint through the window flag.
func (l *toastLayer) add(t *widget.Toast) {
	for len(l.slots) >= l.max && len(l.slots) > 0 {
		oldest := l.slots[0]
		l.slots = l.slots[1:]
		oldest.Close()
		l.markDirty()
	}
	l.slots = append(l.slots, t)
	l.markDirty()
}

// remove detaches a toast whose exit fade landed (or that the stack
// dropped); the last removal unwraps the layer, handing the window
// its bare tree back.
func (l *toastLayer) remove(t *widget.Toast) {
	for i, s := range l.slots {
		if s == t {
			l.slots = append(l.slots[:i], l.slots[i+1:]...)
			break
		}
	}
	t.Close()
	l.markDirty()
	if len(l.slots) == 0 {
		l.close()
	}
}

// close tears the layer down — toasts stop, the wrapped root is
// restored — and detaches it from its window.
func (l *toastLayer) close() {
	for _, s := range l.slots {
		s.Close()
	}
	l.slots = nil
	if l.hw != nil && l.hw.router.Root == widget.Widget(l) {
		l.hw.router.Root = l.root
		l.markDirty()
	}
	l.hw = nil
}

// markDirty schedules a full repaint: structural toast changes are
// rare, and the window flag is the one invalidation an app-side
// wrapper owns.
func (l *toastLayer) markDirty() {
	if l.hw != nil {
		l.hw.dirty = true
	}
}
