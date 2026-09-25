package widget

import (
	"time"

	"github.com/stubbedev/gelm/render"
)

// doubleClickWindow is the maximum gap between the clicks of a
// double-click.
const doubleClickWindow = 400 * time.Millisecond

// BTNLeft is the wayland button code of the primary pointer button.
const BTNLeft uint32 = 0x110

// BTNRight is the wayland button code of the secondary pointer button.
const BTNRight uint32 = 0x111

// HoverSetter receives hover tracking from the Router.
type HoverSetter interface {
	SetHovered(on bool)
}

// PressSetter receives press tracking from the Router.
type PressSetter interface {
	SetPressed(on bool)
}

// Clicker is invoked when a press and release land on the same widget.
// p is the release point in root coordinates.
type Clicker interface {
	ClickAt(p Point)
}

// DragMover receives pointer motion while the widget is pressed.
type DragMover interface {
	DragMove(p Point)
}

// TooltipTexter exposes a widget's hover tooltip text.
type TooltipTexter interface {
	// TooltipText returns the tooltip, empty when none is set.
	TooltipText() string
}

// HoverMover receives pointer motion while the widget is hovered, even
// without a press: menus highlight rows with it.
type HoverMover interface {
	HoverMove(p Point)
}

// ScrollHandler receives axis scrolling; the Router walks the parent chain
// from the hovered widget until one handles it.
type ScrollHandler interface {
	ScrollBy(dy int)
}

// Mods is a bitmask of held keyboard modifiers.
type Mods uint8

// Modifier bits.
const (
	ModShift Mods = 1 << iota
	ModCapsLock
	ModCtrl
	ModAlt
)

// KeyAction is an editing or activation action derived from a keyboard
// event.
type KeyAction uint8

const (
	KeyBackspace KeyAction = iota
	KeyDelete
	KeyLeft
	KeyRight
	KeyUp
	KeyDown
	KeyHome
	KeyEnd
	KeyEnter
)

// KeyActionHandler receives editing and activation actions with the
// modifiers held at the time.
type KeyActionHandler interface {
	KeyAction(a KeyAction, mods Mods)
}

// RuneHandler receives typed characters.
type RuneHandler interface {
	InsertRune(r rune)
}

// Router turns pointer and keyboard input into widget state changes: hover
// tracking, press/release with click detection, drags, scroll bubbling,
// and focus for keyboard text.
//
// The Router is not safe for concurrent use; call it from the same
// goroutine that dispatches the wayland connection.
type Router struct {
	// Root is the widget tree inputs route through.
	Root Widget

	hover, pressed, focus Widget
	dragging              bool
	lastClick             time.Time
	lastClickWidget       Widget
}

// Move updates hover state and feeds drags and in-widget hover
// tracking. p is in root coordinates.
func (r *Router) Move(p Point) {
	hit := r.Root.HitTest(p)
	if r.hover != hit {
		if h, ok := r.hover.(HoverSetter); ok {
			h.SetHovered(false)
		}
		r.hover = hit
		if h, ok := hit.(HoverSetter); ok {
			h.SetHovered(true)
		}
	}
	if h, ok := r.hover.(HoverMover); ok {
		h.HoverMove(p)
	}
	if r.dragging {
		if d, ok := r.pressed.(DragMover); ok {
			d.DragMove(p)
		}
	}
}

// Press records a pointer button press. p is in root coordinates.
func (r *Router) Press(button uint32, p Point) {
	if button != BTNLeft {
		return
	}
	hit := r.Root.HitTest(p)
	r.focus = hit
	r.pressed = hit
	r.dragging = hit != nil
	if pr, ok := hit.(PressSetter); ok {
		pr.SetPressed(true)
	}
}

// Release finishes a press: a release over the pressed widget clicks it.
func (r *Router) Release(button uint32, p Point) {
	if button != BTNLeft || r.pressed == nil {
		return
	}
	if pr, ok := r.pressed.(PressSetter); ok {
		pr.SetPressed(false)
	}
	hit := r.Root.HitTest(p)
	if hit == r.pressed {
		switch {
		case hit == r.lastClickWidget && time.Since(r.lastClick) < doubleClickWindow:
			if c, ok := hit.(DoubleClicker); ok {
				c.DoubleClickAt(p)
			}
			r.lastClick = time.Time{}
			r.lastClickWidget = nil
		default:
			if c, ok := hit.(Clicker); ok {
				c.ClickAt(p)
			}
			r.lastClick = time.Now()
			r.lastClickWidget = hit
		}
	}
	r.pressed = nil
	r.dragging = false
}

// Leave clears hover state when the pointer leaves the surface.
func (r *Router) Leave() {
	if h, ok := r.hover.(HoverSetter); ok {
		h.SetHovered(false)
	}
	r.hover = nil
}

// Axis routes vertical scrolling to the hovered widget or the nearest
// ancestor that handles scrolling. dy is positive to scroll down.
func (r *Router) Axis(dy float64) {
	for target := r.hover; target != nil; target = parentOf(target) {
		if sc, ok := target.(ScrollHandler); ok {
			sc.ScrollBy(int(dy))
			return
		}
	}
}

// KeyAction delivers an editing or activation action to the focused
// widget.
func (r *Router) KeyAction(a KeyAction, mods Mods) {
	if h, ok := r.focus.(KeyActionHandler); ok {
		h.KeyAction(a, mods)
	}
}

// SelectedTexter exposes the widget's active selection.
type SelectedTexter interface {
	// SelectedText returns the selected text and whether a non-empty
	// selection exists.
	SelectedText() (string, bool)
}

// Type delivers a typed character to the focused widget.
func (r *Router) Type(ch rune) {
	if h, ok := r.focus.(RuneHandler); ok {
		h.InsertRune(ch)
	}
}

// DoubleClicker is invoked on the second click of a double-click; p is
// the release point in root coordinates.
type DoubleClicker interface {
	DoubleClickAt(p Point)
}

// SelectAller clears or sets a full selection on the focused widget.
type SelectAller interface {
	SelectAll()
}

// SelectAll asks the focused widget to select its entire content.
func (r *Router) SelectAll() {
	if s, ok := r.focus.(SelectAller); ok {
		s.SelectAll()
	}
}

// Hovered returns the widget currently under the pointer.
func (r *Router) Hovered() Widget { return r.hover }

// Focused returns the widget receiving keyboard input.
func (r *Router) Focused() Widget { return r.focus }

// focusWalker visits the tree in paint order for focus traversal.
func focusWalker(w Widget, fn func(Widget)) {
	if w == nil {
		return
	}
	fn(w)
	type childser interface {
		Children() []Widget
	}
	if c, ok := w.(childser); ok {
		for _, k := range c.Children() {
			focusWalker(k, fn)
		}
	}
}

// FocusNext moves focus to the next focusable widget in paint order,
// wrapping around; with no focus it takes the first.
func (r *Router) FocusNext() { r.focusStep(1) }

// FocusPrev moves focus to the previous focusable widget.
func (r *Router) FocusPrev() { r.focusStep(-1) }

// focusStep walks the tree collecting focusable widgets and lands on
// the neighbor of the current focus.
func (r *Router) focusStep(dir int) {
	var order []Widget
	focusWalker(r.Root, func(w Widget) {
		if _, ok := w.(KeyActionHandler); ok {
			order = append(order, w)
		}
	})
	if len(order) == 0 {
		return
	}
	idx := 0
	if r.focus != nil {
		for i, w := range order {
			if w == r.focus {
				idx = (i + dir + len(order)) % len(order)
				break
			}
		}
	} else if dir < 0 {
		idx = len(order) - 1
	}
	r.focus = order[idx]
}

// Boundser exposes a widget's arranged rect; the app draws the focus
// ring around whatever widget.Boundser the router focuses.
type Boundser interface {
	Bounds() render.Rect
}
