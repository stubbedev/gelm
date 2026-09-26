package wlsession

import (
	"testing"

	"github.com/neurlang/wayland/wl"
)

// axisRecordingHandler captures scroll deltas.
type axisRecordingHandler struct {
	recordingHandler
	dx, dy []float64
}

func (h *axisRecordingHandler) HandlePointerAxis(dx, dy float64) {
	h.dx = append(h.dx, dx)
	h.dy = append(h.dy, dy)
}

// TestWheelValue120Conversion pins the discrete-wheel pipeline:
// axis_value120 accumulates per axis and the pointer frame flushes
// it as whole-notch deltas on the right axis, vertical notches
// negated (positive value120 is away from the user; positive dy is
// down), and the app's 10-per-step scale sees one step per notch.
func TestWheelValue120Conversion(t *testing.T) {
	s := newRoutingSession()
	surf := &wl.Surface{}
	h := &axisRecordingHandler{}
	s.SetSurfaceInput(surf, h)
	s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf})

	// Two notches down (positive value120 = up on the wire, so the
	// app must see negative dy).
	s.HandlePointerAxisValue120(wl.PointerAxisValue120Event{Axis: 0, Value120: 240})
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if len(h.dy) != 1 || h.dy[0] != -20 || h.dx[0] != 0 {
		t.Errorf("flushed dx=%v dy=%v, want one event with dy=-20", h.dx, h.dy)
	}

	// A horizontal wheel event lands on dx.
	s.HandlePointerAxisValue120(wl.PointerAxisValue120Event{Axis: 1, Value120: 120})
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if len(h.dx) != 2 || h.dx[1] != 10 {
		t.Errorf("flushed dx = %v, want [0 10] (one notch on the second flush)", h.dx)
	}

	// An empty frame does not emit a spurious scroll.
	before := len(h.dx) + len(h.dy)
	s.HandlePointerFrame(wl.PointerFrameEvent{})
	if len(h.dx)+len(h.dy) != before {
		t.Errorf("empty frame emitted a scroll")
	}
}

// TestSmoothAxisPassthrough pins that smooth (trackpad) axis values
// bypass the value120 pipeline and deliver their raw value directly.
func TestSmoothAxisPassthrough(t *testing.T) {
	s := newRoutingSession()
	surf := &wl.Surface{}
	h := &axisRecordingHandler{}
	s.SetSurfaceInput(surf, h)
	s.HandlePointerEnter(wl.PointerEnterEvent{Surface: surf})

	s.HandlePointerAxis(wl.PointerAxisEvent{Axis: 0, Value: 12.5})
	if len(h.dy) != 1 || h.dy[0] != 12.5 {
		t.Errorf("smooth dy = %v, want [12.5]", h.dy)
	}
}
