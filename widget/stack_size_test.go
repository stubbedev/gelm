package widget

import (
	"testing"
	"time"
)

func sizedStack() *Stack {
	return NewStack().Add("short", newStub(40, 50)).Add("tall", newStub(60, 150))
}

func measureH(s *Stack) int { return s.Measure(Constraints{Max: Size{W: 500, H: 500}}).H }

// TestStackSizesToItsVisiblePage pins GTK's homogeneous off: the stack
// measures its visible page, the default its largest, and PageExtent
// the largest either way.
func TestStackSizesToItsVisiblePage(t *testing.T) {
	s := sizedStack()
	if got := measureH(s); got != 150 {
		t.Errorf("homogeneous = %d, want the tallest page's 150", got)
	}
	s.SetHomogeneous(false)
	if got := measureH(s); got != 50 {
		t.Errorf("sized to the visible page = %d, want 50", got)
	}
	s.Show("tall")
	if got := measureH(s); got != 150 {
		t.Errorf("after switching = %d, want 150 (a switch relayouts)", got)
	}
	if got := s.PageExtent(Constraints{Max: Size{W: 500, H: 500}}); got != (Size{W: 60, H: 150}) {
		t.Errorf("PageExtent = %+v, want the largest page", got)
	}
	s.SetHomogeneous(true)
	s.Show("short")
	if got := measureH(s); got != 150 {
		t.Errorf("homogeneous again = %d, want 150", got)
	}
}

// TestStackInterpolatesItsSizeThroughASwitch pins interpolate-size: the
// size tweens between the pages on the switch's easing and lands on
// the new page; without it, it jumps at once.
func TestStackInterpolatesItsSizeThroughASwitch(t *testing.T) {
	c := pinAnimClock(t)
	s := sizedStack()
	s.SetHomogeneous(false)
	s.SetInterpolateSize(true)
	s.SetTransition(StackCrossfade, 200*time.Millisecond)
	measureH(s)
	s.Show("tall")
	c.step()
	c.step()
	if got := measureH(s); got <= 50 || got >= 150 {
		t.Errorf("mid-switch = %d, want between 50 and 150", got)
	}
	c.drive()
	if got := measureH(s); got != 150 {
		t.Errorf("after the switch = %d, want 150", got)
	}

	jump := sizedStack()
	jump.SetHomogeneous(false)
	jump.SetTransition(StackCrossfade, 200*time.Millisecond)
	measureH(jump)
	jump.Show("tall")
	c.step()
	if got := measureH(jump); got != 150 {
		t.Errorf("without interpolate-size = %d mid-switch, want the jump to 150", got)
	}
}
