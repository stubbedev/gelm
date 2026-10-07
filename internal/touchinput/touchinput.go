// Package touchinput is the session-to-widget bridge for the
// non-pointer devices every input surface embeds (app windows,
// popups): it implements wlsession.SurfaceTouchHandler,
// SurfaceGestureHandler and SurfaceTabletHandler over a
// widget.TouchTracker, the router's gestures, and its stylus samples.
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
	penDown  bool    // a tablet tool's tip is down
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

// stylusTools maps the session's tablet tools.
var stylusTools = map[wlsession.TabletTool]widget.StylusTool{
	wlsession.ToolPen: widget.StylusPen, wlsession.ToolEraser: widget.StylusEraser,
	wlsession.ToolBrush: widget.StylusBrush, wlsession.ToolPencil: widget.StylusPencil,
	wlsession.ToolAirbrush: widget.StylusAirbrush, wlsession.ToolFinger: widget.StylusFinger,
	wlsession.ToolMouse: widget.StylusMouse, wlsession.ToolLens: widget.StylusLens,
}

// HandleTabletFrame implements wlsession.SurfaceTabletHandler: the
// tool drives the pointer - in range it hovers, its tip presses and
// releases, leaving range leaves - and then the sample goes to the
// widget under it (Router.Stylus).
func (in *Input) HandleTabletFrame(s wlsession.TabletSample) {
	if in.blocked() {
		return
	}
	p := widget.Point{X: int(s.X), Y: int(s.Y)}
	ptr := in.Tracker.Pointer
	if ptr == nil {
		ptr = widget.RouterPointer{R: in.Tracker.Router}
	}
	if s.Down {
		in.Serial = s.Serial
	}
	sample := widget.Stylus{
		Tool: stylusTools[s.Tool], At: p, Pressure: s.Pressure, Distance: s.Distance,
		TiltX: s.TiltX, TiltY: s.TiltY, Rotation: s.Rotation, Down: in.penDown && !s.Up,
		Button: s.Button, Pressed: s.Pressed,
	}
	switch {
	case s.ProximityOut:
		if in.penDown {
			ptr.TouchCancel()
			in.penDown = false
		}
		in.Tracker.Router.Leave()
	case s.Down:
		ptr.TouchPress(p)
		in.penDown = true
		sample.Down = true
		in.Tracker.Router.Stylus(sample)
	case s.Up:
		// The stroke's end goes to the widget holding the stroke,
		// before the release lets go of it.
		in.Tracker.Router.Stylus(sample)
		ptr.TouchRelease(p)
		ptr.TouchMove(p) // the pen still hovers in range
		in.penDown = false
	default:
		ptr.TouchMove(p)
		in.Tracker.Router.Stylus(sample)
	}
	in.changed()
}
