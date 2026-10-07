// Package touchinput is the session-to-widget touch bridge every
// input surface embeds (app windows, popups): it implements
// wlsession.SurfaceTouchHandler and SurfaceGestureHandler over a
// widget.TouchTracker and the router's gestures.
package touchinput

import (
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/widget"
)

// Input routes one surface's touch contacts and touchpad gestures.
type Input struct {
	// Tracker turns contacts into pointer emulation, gestures, and
	// scrolling; its Router must be set, its Pointer optionally.
	Tracker widget.TouchTracker
	// Pointer reports where the pointer is, for touchpad gestures.
	Pointer func() widget.Point
	// Blocked, when set and true, drops input (a modal elsewhere).
	Blocked func() bool
	// Changed runs after every routed event (request a frame).
	Changed func()
	// Serial records the last touch-down serial (a frame grab wants it).
	Serial uint32

	rotation float64 // the touchpad pinch's accumulated rotation
}

// blocked reports whether input is dropped.
func (in *Input) blocked() bool { return in.Blocked != nil && in.Blocked() }

// changed requests a frame.
func (in *Input) changed() {
	if in.Changed != nil {
		in.Changed()
	}
}

// HandleTouchDown implements wlsession.SurfaceTouchHandler.
func (in *Input) HandleTouchDown(id int32, x, y float64, serial uint32) {
	if in.blocked() {
		return
	}
	in.Serial = serial
	in.Tracker.Down(id, widget.Point{X: int(x), Y: int(y)})
	in.changed()
}

// HandleTouchMotion implements wlsession.SurfaceTouchHandler.
func (in *Input) HandleTouchMotion(id int32, x, y float64) {
	if in.blocked() {
		return
	}
	in.Tracker.Motion(id, widget.Point{X: int(x), Y: int(y)})
	in.changed()
}

// HandleTouchUp implements wlsession.SurfaceTouchHandler.
func (in *Input) HandleTouchUp(id int32) {
	if in.blocked() {
		return
	}
	in.Tracker.Up(id)
	in.changed()
}

// HandleTouchCancel implements wlsession.SurfaceTouchHandler.
func (in *Input) HandleTouchCancel() {
	in.Tracker.Cancel()
	in.changed()
}

// gestureKinds maps the session's touchpad gestures.
var gestureKinds = map[wlsession.GestureKind]widget.GestureKind{
	wlsession.GestureSwipe: widget.GestureSwipe,
	wlsession.GesturePinch: widget.GesturePinch,
	wlsession.GestureHold:  widget.GestureHold,
}

// HandlePointerGesture implements wlsession.SurfaceGestureHandler: the
// touchpad gesture at the pointer, its per-event rotation accumulated
// into the widget gesture's since-start rotation.
func (in *Input) HandlePointerGesture(g wlsession.PointerGesture) {
	if in.blocked() {
		return
	}
	if g.Phase == wlsession.GestureBegin {
		in.rotation = 0
	}
	in.rotation += g.Rotation
	var at widget.Point
	if in.Pointer != nil {
		at = in.Pointer()
	}
	in.Tracker.Router.Gesture(widget.Gesture{
		Kind: gestureKinds[g.Kind], Phase: widget.GesturePhase(g.Phase), At: at, Fingers: g.Fingers,
		DX: g.DX, DY: g.DY, Scale: g.Scale, Rotation: in.rotation,
	})
	in.changed()
}
