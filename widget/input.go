package widget

import (
	"io"
	"time"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/render"
)

// doubleClickWindow is the maximum gap between the clicks of a
// double-click.
const doubleClickWindow = 400 * time.Millisecond

// BTNLeft is the wayland button code of the primary pointer button.
const BTNLeft uint32 = 0x110

// BTNRight is the wayland button code of the secondary pointer button.
const BTNRight uint32 = 0x111

// BTNMiddle is the wayland button code of the middle pointer button,
// the X11 paste button.
const BTNMiddle uint32 = 0x112

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

// CursorNamer lets a widget request a pointer shape while hovered,
// using xcursor names ("xterm" for text, "left_ptr" arrow default).
type CursorNamer interface {
	// CursorName returns the xcursor shape for the hovered widget.
	CursorName() string
}

// TooltipTexter exposes a widget's hover tooltip text.
type TooltipTexter interface {
	// TooltipText returns the tooltip, empty when none is set.
	TooltipText() string
}

// IsInteractive reports whether w or any ancestor consumes presses:
// buttons, sliders, toggles, text inputs, and scroll areas. Hit tests
// return the deepest widget, often a plain label inside a control, so
// apps checking "was this chrome?" must walk up. Parents are recorded
// during Arrange, so call it on an arranged tree.
func IsInteractive(w Widget) bool {
	for w != nil {
		switch w.(type) {
		case *Button, *Slider, *Switch, *CheckButton, *Entry, *TextArea, *Scroll:
			return true
		}
		p, ok := w.(interface{ Parent() Widget })
		if !ok {
			return false
		}
		w = p.Parent()
	}
	return false
}

// HoverMover receives pointer motion while the widget is hovered, even
// without a press: menus highlight rows with it.
type HoverMover interface {
	HoverMove(p Point)
}

// ScrollHandler receives scroll deltas in 40px steps: dx from the
// horizontal axis (tilt wheels, trackpads), dy from the vertical.
type ScrollHandler interface {
	ScrollBy(dx, dy int)
}

// DragContent is the payload a drag carries: mime types best first
// and a provider that writes the bytes for one mime on demand. OnDone
// is optional; it fires once the drag concluded — dropped and handed
// off (true), or cancelled (false).
type DragContent struct {
	Mimes  []string
	Write  func(mime string, w io.Writer) error
	OnDone func(dropped bool)
}

// DragSource lets a widget start a drag-and-drop: a press on it plus
// motion past the app's drag threshold offers DragContent through the
// wl_data_device. Return nil to decline this particular press.
type DragSource interface {
	// DragContent returns the offered payload, or nil to not drag.
	DragContent() *DragContent
}

// DragEnterer decides whether a widget accepts a drag hovering it,
// by mime. mimes lists the offered types best first; return the mime
// to receive on drop, or "" to reject the drag.
type DragEnterer interface {
	DragEnter(mimes []string, p Point) string
}

// DragHoverer receives drag movement while a drag hovers the widget,
// for position-aware feedback (drop indicators, insertion gaps).
type DragHoverer interface {
	DragHover(p Point)
}

// DragLeaver receives a drag that left the widget without dropping.
type DragLeaver interface {
	DragLeave()
}

// Dropper receives a drop: the accepted mime, its bytes, and the
// drop point in root coordinates.
type Dropper interface {
	Drop(mime string, data []byte, p Point)
}

// DragOverSetter receives drag-hover tracking from the Router, the
// highlight bit while a drag sits over the widget — the drop-target
// counterpart of HoverSetter.
type DragOverSetter interface {
	SetDragOver(on bool)
}

// Axis routes vertical and horizontal scrolling to the hovered widget
// or the nearest ancestor that handles scrolling.
func (r *Router) Axis(dx, dy float64) {
	for target := r.hover; target != nil; target = parentOf(target) {
		if sc, ok := target.(ScrollHandler); ok {
			sc.ScrollBy(int(dx), int(dy))
			return
		}
	}
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

// TabTrapper lets a focused widget absorb a plain Tab press as
// indentation. When the focused widget implements it and TrapTab
// returns true, focus stays; ctrl+Tab and shift+Tab always move focus
// regardless of this interface.
type TabTrapper interface {
	// TrapTab inserts the widget's tab or indent and reports true.
	TrapTab(ctrl bool) bool
}

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
	KeyPriorPage
	KeyNextPage
	KeyDismiss
)

// KeyActionForSym maps a keysym to an editing or activation action.
// Popup hosts use it to route seat keyboard events into popup-local
// widget trees without importing the app package.
func KeyActionForSym(sym xkb.Keysym) (KeyAction, bool) {
	switch sym {
	case xkb.KeyBackSpace:
		return KeyBackspace, true
	case xkb.KeyDelete:
		return KeyDelete, true
	case xkb.KeyLeft:
		return KeyLeft, true
	case xkb.KeyRight:
		return KeyRight, true
	case xkb.KeyUp:
		return KeyUp, true
	case xkb.KeyDown:
		return KeyDown, true
	case xkb.KeyHome:
		return KeyHome, true
	case xkb.KeyEnd:
		return KeyEnd, true
	case xkb.KeyReturn, xkb.KeyKPEnter:
		return KeyEnter, true
	case xkb.KeyPrior:
		return KeyPriorPage, true
	case xkb.KeyNext:
		return KeyNextPage, true
	case xkb.KeyEscape:
		return KeyDismiss, true
	}
	return 0, false
}

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

	// drop-target state during a wl_data_device drag: the widget the
	// drag is over, the offered mimes, and the mime it accepted.
	dragTarget Widget
	dragMimes  []string
	dragMime   string
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
		handled := false
		if hit == r.lastClickWidget && time.Since(r.lastClick) < doubleClickWindow {
			// Only widgets with a double-click behavior consume the
			// second click; everything else treats each release as
			// its own click (two rapid clicks toggle twice).
			if dc, ok := hit.(DoubleClicker); ok {
				dc.DoubleClickAt(p)
				handled = true
			}
		}
		if !handled {
			if c, ok := hit.(Clicker); ok {
				c.ClickAt(p)
			}
		}
		r.lastClick = time.Now()
		r.lastClickWidget = hit
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

// Pressed returns the widget an implicit press is active on, or nil.
func (r *Router) Pressed() Widget { return r.pressed }

// CancelPress drops the active press without a click: a gesture that
// became a data-device drag must not fire the widget on release.
func (r *Router) CancelPress() {
	if pr, ok := r.pressed.(PressSetter); ok {
		pr.SetPressed(false)
	}
	r.pressed = nil
	r.dragging = false
}

// dragHandlerAt walks up from the widget at p to the nearest ancestor
// that decides drag acceptance. Hit tests return the deepest widget —
// a label inside a row — so drop targets are found on the parent
// chain, the same way Axis finds the scroll handler.
func (r *Router) dragHandlerAt(p Point) Widget {
	for w := r.Root.HitTest(p); w != nil; w = parentOf(w) {
		if _, ok := w.(DragEnterer); ok {
			return w
		}
	}
	return nil
}

// DragEnter routes a drag entering the tree: find the drop target at
// p, ask it for a decision by mime, and highlight it only when it
// accepted. Returns the accepted mime ("" rejects).
func (r *Router) DragEnter(mimes []string, p Point) string {
	r.dragMimes = mimes
	w := r.dragHandlerAt(p)
	mime := ""
	if d, ok := w.(DragEnterer); ok {
		mime = d.DragEnter(mimes, p)
	}
	r.applyDragTarget(w, mime != "")
	r.dragMime = mime
	return mime
}

// DragHover routes drag movement within the tree. The compositor only
// sends enter on surface changes, so crossing between targets inside
// one surface retargets here, with the mimes from the enter.
func (r *Router) DragHover(p Point) string {
	w := r.dragHandlerAt(p)
	if w != r.dragTarget {
		return r.DragEnter(r.dragMimes, p)
	}
	if h, ok := w.(DragHoverer); ok {
		h.DragHover(p)
	}
	return r.dragMime
}

// DragLeave clears the drop target after the drag left the surface or
// ended without a drop.
func (r *Router) DragLeave() {
	r.applyDragTarget(nil, false)
	r.dragMime = ""
}

// Drop delivers the drop to the current drop target and clears the
// hover state. data carries the payload bytes the app fetched from
// the offer — or through the same-process shortcut.
func (r *Router) Drop(mime string, data []byte, p Point) {
	w := r.dragTarget
	r.applyDragTarget(nil, false)
	r.dragMime = ""
	if d, ok := w.(Dropper); ok {
		d.Drop(mime, data, p)
	}
}

// DragMime returns the mime the current drop target accepted, empty
// while nothing is accepted; the payload is fetched for it on drop.
func (r *Router) DragMime() string { return r.dragMime }

// applyDragTarget swaps the drop target, clearing the old one's
// highlight and notifying it of the departure; the new one is
// highlighted only when it accepted the drag.
func (r *Router) applyDragTarget(w Widget, accepted bool) {
	if old := r.dragTarget; old != nil {
		if s, ok := old.(DragOverSetter); ok {
			s.SetDragOver(false)
		}
		if l, ok := old.(DragLeaver); ok {
			l.DragLeave()
		}
	}
	r.dragTarget = w
	if w == nil || !accepted {
		return
	}
	if s, ok := w.(DragOverSetter); ok {
		s.SetDragOver(true)
	}
}

// Axis routes vertical scrolling to the hovered widget or the nearest
// ancestor that handles it.

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

// TextInserter is an editable text widget that accepts pasted text:
// Insert places s at the caret, replacing any active selection, and
// reports the change through OnChanged. Entry and TextArea implement
// it; primary-selection pastes (middle click) target it.
type TextInserter interface {
	// Insert inserts s at the caret, replacing the selection.
	Insert(s string)
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
