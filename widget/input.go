package widget

// BTNLeft is the wayland button code of the primary pointer button.
const BTNLeft uint32 = 0x110

// HoverSetter receives hover tracking from the Router.
type HoverSetter interface {
	SetHovered(on bool)
}

// PressSetter receives press tracking from the Router.
type PressSetter interface {
	SetPressed(on bool)
}

// Clicker is invoked when a press and release land on the same widget.
type Clicker interface {
	Click()
}

// DragMover receives pointer motion while the widget is pressed.
type DragMover interface {
	DragMove(p Point)
}

// ScrollHandler receives axis scrolling; the Router walks the parent chain
// from the hovered widget until one handles it.
type ScrollHandler interface {
	ScrollBy(dy int)
}

// KeyAction is an editing or activation action derived from a keyboard
// event.
type KeyAction uint8

const (
	KeyBackspace KeyAction = iota
	KeyDelete
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyEnter
)

// KeyActionHandler receives editing and activation actions.
type KeyActionHandler interface {
	KeyAction(a KeyAction)
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
}

// Move updates hover state and feeds drags. p is in root coordinates.
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
		if c, ok := r.pressed.(Clicker); ok {
			c.Click()
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
func (r *Router) KeyAction(a KeyAction) {
	if h, ok := r.focus.(KeyActionHandler); ok {
		h.KeyAction(a)
	}
}

// Type delivers a typed character to the focused widget.
func (r *Router) Type(ch rune) {
	if h, ok := r.focus.(RuneHandler); ok {
		h.InsertRune(ch)
	}
}

// Hovered returns the widget currently under the pointer.
func (r *Router) Hovered() Widget { return r.hover }

// Focused returns the widget receiving keyboard input.
func (r *Router) Focused() Widget { return r.focus }
