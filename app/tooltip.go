package app

import (
	"time"

	"github.com/neurlang/wayland/xdg"

	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/popup"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
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
// needs; real popups and tests both satisfy it.
type tooltipWindow interface {
	Closed() bool
	Close()
}

// tooltipCtl tracks the hover dwell and the one open tooltip.
type tooltipCtl struct {
	open  tooltipWindow
	hover widget.Widget
	since time.Time
}

// update advances the tooltip state to now: reset the dwell on hover
// change, close stale tooltips, open a due one through opener.
func (t *tooltipCtl) update(router *widget.Router, now time.Time, opener func(widget.Widget, string) tooltipWindow) {
	if t.open != nil && t.open.Closed() {
		t.open = nil
	}
	h := router.Hovered()
	var text string
	if h != nil {
		if tter, ok := h.(widget.TooltipTexter); ok {
			text = tter.TooltipText()
		}
	}
	changed := h != t.hover
	if changed {
		t.hover, t.since = h, now
	}
	if tooltipShouldClose(t.open != nil, changed, text != "") {
		t.open.Close()
		t.open = nil
	}
	if tooltipShouldOpen(t.open != nil, h, text, t.since, now) {
		if p := opener(h, text); p != nil {
			t.open = p
			t.since = now
		}
	}
}

// openTooltip maps a one-shot painted popup at the pointer.
func openTooltip(sess *wlsession.Session, host Host, cfg *Config, pointerX, pointerY int, text string) *popup.Popup {
	ts, ok := host.(tooltipSurfacer)
	if !ok || cfg.TooltipFace == nil {
		return nil
	}
	lbl := widget.NewLabel(cfg.TooltipFace, text, 12, widget.Current().Text)
	box := widget.NewBox(widget.Row, 0, 8)
	box.Append(lbl, false)
	size := box.Measure(widget.Constraints{Max: widget.Size{W: 400, H: 200}})
	tp, err := popup.New(sess, popup.Config{
		Parent: ts.TooltipSurface(),
		X:      pointerX + tooltipOffsetX,
		Y:      pointerY + tooltipOffsetY,
		Width:  size.W, Height: size.H,
		NoGrab: true,
	})
	if err != nil {
		return nil
	}
	w, h := tp.Size()
	box.Measure(widget.Constraints{Max: widget.Size{W: w, H: h}})
	box.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
	b, err := buffer.NewFile(sess.Shm(), w*cfg.Scale, h, cfg.Scale)
	if err != nil {
		tp.Close()
		return nil
	}
	cv := render.New(b.Data, b.Stride, b.Width, b.Height)
	cv.Clear(cv.Rect(), widget.Current().Surface)
	box.Paint(cv)
	surf := tp.HostSurface()
	if err := surf.Attach(b.WL, 0, 0); err != nil {
		tp.Close()
		return nil
	}
	if err := surf.DamageBuffer(0, 0, int32(b.Width), int32(b.Height)); err != nil {
		tp.Close()
		return nil
	}
	if err := surf.Commit(); err != nil {
		tp.Close()
		return nil
	}
	return tp
}
