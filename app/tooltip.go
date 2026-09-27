package app

import (
	"time"

	"github.com/neurlang/wayland/xdg"

	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/popup"
	"github.com/stubbedev/gelm/internal/surfx"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/widget"
)

// tooltipDelay is how long the pointer must rest on a widget before
// its tooltip opens.
const tooltipDelay = 500 * time.Millisecond

// tooltipOffset places the tooltip below-right of the pointer.
const (
	tooltipOffsetX = 12
	tooltipOffsetY = 22
)

// tooltipShouldOpen reports whether a resting hover earns a tooltip:
// none open yet, a hovered widget, hover text, and dwell past the
// delay.
func tooltipShouldOpen(open bool, hover widget.Widget, text string, since, now time.Time) bool {
	return !open && hover != nil && text != "" && now.Sub(since) >= tooltipDelay
}

// tooltipShouldClose reports whether an open tooltip must go: the
// hover changed or the new hover carries no text.
func tooltipShouldClose(open, hoverChanged, hasText bool) bool {
	return open && (hoverChanged || !hasText)
}

// tooltipSurfacer supplies the xdg_surface tooltips anchor to. Layer
// surfaces have none, so hosts without it get no tooltips.
type tooltipSurfacer interface {
	TooltipSurface() *xdg.Surface
}

// tooltipWindow is the slice of a popup the tooltip state machine
// needs; real popups and tests both satisfy it. Dismissed — not a
// closed/destroyed flag — is what drops the reference: a tooltip
// running its exit tween is logically gone, so the next dwell may open
// a new one while the old surface is still fading out.
type tooltipWindow interface {
	Dismissed() bool
	Dismiss()
}

// tooltipCtl tracks the hover dwell and the one open tooltip. The
// open tooltip's painter is driven by the application loop (one Pass
// per loop pass): tooltips must never run their own dispatcher — one
// event-loop goroutine owns the connection.
type tooltipCtl struct {
	open    tooltipWindow
	painter *popup.Painter
	hover   widget.Widget
	since   time.Time
}

// next returns when a pending tooltip could open: a hovered widget
// with text, not yet open, one delay away. False when nothing is
// pending, so the loop need not wake for tooltips.
func (t *tooltipCtl) next() (time.Time, bool) {
	if t.open != nil || t.hover == nil {
		return time.Time{}, false
	}
	if text := hoverTooltipText(t.hover); text != "" {
		return t.since.Add(tooltipDelay), true
	}
	return time.Time{}, false
}

// hoverTooltipText returns the hovered widget's tooltip text, empty
// when it has none.
func hoverTooltipText(h widget.Widget) string {
	if tter, ok := h.(widget.TooltipTexter); ok {
		return tter.TooltipText()
	}
	return ""
}

// update advances the tooltip state to now: reset the dwell on hover
// change, dismiss stale tooltips, open a due one through opener, and
// paint the open tooltip's tween frames.
func (t *tooltipCtl) update(router *widget.Router, now time.Time, opener func(widget.Widget, string) (tooltipWindow, *popup.Painter)) {
	if t.open != nil && t.open.Dismissed() {
		t.open, t.painter = nil, nil
	}
	h := router.Hovered()
	text := hoverTooltipText(h)
	changed := h != t.hover
	if changed {
		t.hover, t.since = h, now
	}
	if tooltipShouldClose(t.open != nil, changed, text != "") {
		// Traced as "tooltip closed": the headless harness waits on it.
		debug.Log("input", "tooltip closed")
		t.open.Dismiss()
		t.open, t.painter = nil, nil
	}
	if tooltipShouldOpen(t.open != nil, h, text, t.since, now) {
		if p, pc := opener(h, text); p != nil {
			t.open, t.painter = p, pc
		}
		// Advance the dwell clock even when the open failed, so a
		// tooltip that cannot open (host without a face, compositor
		// rejection) retries after a full delay instead of every loop.
		t.since = now
	}
	if t.painter != nil {
		if _, err := t.painter.Pass(); err != nil {
			debug.Log("frame", "tooltip paint: %v", err)
			t.painter = nil
		}
	}
}

// cursorFor resolves the pointer shape for the hovered widget: the
// widget's own request (text fields ask for the caret), else the arrow.
func cursorFor(hover widget.Widget) string {
	if cn, ok := hover.(widget.CursorNamer); ok {
		return cn.CursorName()
	}
	return ""
}

// openTooltip maps a tooltip popup at the pointer. The popup surface
// scales with the host window: frac120 is the window's current
// 120-based device scale. Frames are painted by the application loop
// through the returned Painter — the enter fade, the rest state, and
// the exit fade all repaint through the same tween machinery menus
// use, without a second wayland dispatcher. Tooltips are ungrabbed
// (NoGrab), so the fades never hold input hostage.
func openTooltip(sess *wlsession.Session, host Host, cfg *Config, frac120 uint32, pointerX, pointerY int, text string) (*popup.Popup, *popup.Painter) {
	ts, ok := host.(tooltipSurfacer)
	if !ok || cfg.TooltipFace == nil {
		debug.Log("input", "tooltip unavailable: host %T or nil face", host)
		return nil, nil
	}
	lbl := widget.NewLabel(cfg.TooltipFace, 12, text, widget.Current().Text)
	box := widget.NewBox(widget.Row, 0, 8)
	box.Append(lbl, false)
	size := box.Measure(widget.Constraints{Max: widget.Size{W: 400, H: 200}})
	// The shadow gutter rides on the surface (see OpenPopover): the
	// tooltip's plate is inset by it and the pointer offset stays on
	// the plate.
	gutter := widget.Current().ShadowGutter()
	tp, err := popup.New(sess, popup.Config{
		Parent: ts.TooltipSurface(),
		X:      pointerX + tooltipOffsetX - gutter,
		Y:      pointerY + tooltipOffsetY - gutter,
		Width:  size.W + 2*gutter, Height: size.H + 2*gutter,
		Gutter: gutter,
		NoGrab: true,
		Kind:   surfx.KindTooltip,
	})
	if err != nil {
		debug.Log("input", "tooltip popup: %v", err)
		return nil, nil
	}
	// Tooltips are pure display: an empty input region keeps the
	// compositor from ever routing the pointer to the popup, which
	// would steal clicks from the window beneath. Sealed once at open;
	// the state machine re-seals on dismissal, and later commits keep
	// the region (double-buffered state persists until changed).
	tp.SealInput()
	painter := tp.NewPainter(sess, frac120, box, widget.Current().Surface)
	debug.Log("input", "tooltip open %q at (%d,%d)", text, pointerX, pointerY)
	return tp, painter
}
