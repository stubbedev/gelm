package widget

import (
	"time"

	"github.com/stubbedev/gelm/anim"
	"github.com/stubbedev/gelm/render"
)

// sheetHandle is the drag strip at the sheet's top edge.
const sheetHandle = 24

// BottomSheet is the modal sheet over content (adw BottomSheet): the
// content fills the widget; SetOpen slides a rounded sheet up from the
// bottom over a dimmed backdrop. Dragging the handle down past a third
// of the sheet dismisses it (a shorter drag springs back), and so do a
// click on the backdrop and Escape. The pointer path works today; the
// touch ticket adds swipe velocity.
type BottomSheet struct {
	composite
	sheet Widget

	open     bool
	progress float64
	cancel   anim.Cancel
	drag     dragGesture
	dragDY   int

	// OnClosed fires when a close finished sliding out, whatever
	// caused it.
	OnClosed func()
}

// NewBottomSheet returns a closed sheet holding sheet over content.
func NewBottomSheet(content, sheet Widget) *BottomSheet {
	b := &BottomSheet{sheet: sheet}
	b.SetElement("bottomsheet")
	b.initComposite(b, content)
	b.surfaceRadius = 12
	return b
}

// Open reports whether the sheet is open (or opening).
func (b *BottomSheet) Open() bool { return b.open }

// SetOpen slides the sheet in or out.
func (b *BottomSheet) SetOpen(on bool) {
	if b.open == on {
		return
	}
	b.open = on
	b.dragDY = 0
	if b.cancel != nil {
		b.cancel()
	}
	from, to := b.progress, 0.0
	if on {
		to = 1
	}
	b.cancel = anim.Start(220*time.Millisecond, func(t float64) {
		b.progress = from + (to-from)*easeOut(t)
		b.Invalidate()
		if t >= 1 && !on && b.OnClosed != nil {
			b.OnClosed()
		}
	})
}

// showing reports whether any of the sheet is on screen.
func (b *BottomSheet) showing() bool { return b.progress > 0 }

// sheetRect places the sheet for the current progress and drag: its
// natural height (handle included), capped at nine tenths of the
// widget, rising from the bottom edge.
func (b *BottomSheet) sheetRect() render.Rect {
	r := b.bounds
	nat := b.sheet.Measure(Constraints{Max: Size{W: r.W, H: r.H}})
	h := min(nat.H+sheetHandle, r.H*9/10)
	y := r.Y + r.H - int(float64(h)*b.progress) + b.dragDY
	return render.Rect{X: r.X, Y: y, W: r.W, H: h}
}

// Paint draws the content, then - while showing - the dimmed backdrop,
// the sheet's surface, its handle, and its child. The sheet is placed
// here, per frame, so a drag moves it without a relayout.
func (b *BottomSheet) Paint(cv *render.Canvas) {
	PaintChild(cv, b.root)
	if !b.showing() {
		return
	}
	th := Current()
	cv.FillRect(b.bounds, render.RGBA(0, 0, 0, uint8(96*b.progress)))
	rect := b.sheetRect()
	b.paintSurface(cv, rect, th.Surface)
	grip := render.Rect{X: rect.X + rect.W/2 - 18, Y: rect.Y + 10, W: 36, H: 4}
	cv.RoundedRect(grip, 2, th.Border)
	b.sheet.Arrange(render.Rect{X: rect.X, Y: rect.Y + sheetHandle, W: rect.W, H: rect.H - sheetHandle})
	setParents(b, b.sheet)
	PaintChild(cv, b.sheet)
}

// HitTest resolves into the sheet while it shows (the backdrop and the
// handle are the sheet widget itself), else into the content.
func (b *BottomSheet) HitTest(p Point) Widget {
	if !b.showing() {
		return b.composite.HitTest(p)
	}
	if !b.bounds.Contains(p.X, p.Y) {
		return nil
	}
	if rect := b.sheetRect(); rect.Contains(p.X, p.Y) && p.Y >= rect.Y+sheetHandle {
		if hit := b.sheet.HitTest(p); hit != nil {
			return hit
		}
	}
	return b
}

// PressAt begins a drag on the handle strip.
func (b *BottomSheet) PressAt(p Point) {
	if rect := b.sheetRect(); b.open && rect.Contains(p.X, p.Y) && p.Y < rect.Y+sheetHandle {
		b.drag.begin(p)
	}
}

// DragMove follows a downward drag.
func (b *BottomSheet) DragMove(p Point) {
	if _, dy, ok := b.drag.travel(p); ok {
		b.dragDY = max(dy, 0)
		b.Invalidate()
	}
}

// PressEnd dismisses past a third of the sheet's height, else springs
// back.
func (b *BottomSheet) PressEnd() {
	if !b.drag.end() {
		return
	}
	if b.dragDY > b.sheetRect().H/3 {
		b.SetOpen(false)
		return
	}
	b.dragDY = 0
	b.Invalidate()
}

// ClickAt closes on a click on the backdrop.
func (b *BottomSheet) ClickAt(p Point) {
	if b.open && !b.sheetRect().Contains(p.X, p.Y) {
		b.SetOpen(false)
	}
}

// KeyAction closes on Escape.
func (b *BottomSheet) KeyAction(a KeyAction, _ Mods) {
	if a == KeyDismiss && b.open {
		b.SetOpen(false)
	}
}

// styleChildren is the content and, while showing, the sheet.
func (b *BottomSheet) styleChildren() []Widget {
	if b.showing() {
		return []Widget{b.root, b.sheet}
	}
	return []Widget{b.root}
}

// Content returns the widget the sheet slides over.
func (b *BottomSheet) Content() Widget { return b.root }
