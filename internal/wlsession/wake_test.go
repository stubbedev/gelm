package wlsession

import (
	"testing"
	"time"
)

// A WakeAfter timer that fires after Close must stand down: the
// closed connection's proxy map is gone, and before the guard the
// timer's sync panicked the process (seen when an app quit with a
// wake pending). The session here has no display at all, so any touch
// of the connection would crash the test binary.
func TestWakeAfterCloseStandsDown(t *testing.T) {
	s := &Session{}
	s.Close()
	s.WakeAfter(0)
	time.Sleep(50 * time.Millisecond)
	if !s.closed.Load() {
		t.Fatal("Close did not mark the session closed")
	}
}
