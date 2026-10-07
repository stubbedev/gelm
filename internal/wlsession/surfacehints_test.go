package wlsession

import (
	"errors"
	"testing"
)

// Without the protocols every surface hint reports unavailable rather
// than failing on the wire.
func TestSurfaceHintsUnavailable(t *testing.T) {
	s := newRoutingSession()
	if err := s.SetContentType(nil, ContentPhoto); !errors.Is(err, ErrProtocolUnavailable) {
		t.Errorf("content type: %v", err)
	}
	if err := s.SetSurfaceAlpha(nil, 0.5); !errors.Is(err, ErrProtocolUnavailable) {
		t.Errorf("alpha: %v", err)
	}
	if err := s.Bell(nil); !errors.Is(err, ErrProtocolUnavailable) {
		t.Errorf("bell: %v", err)
	}
	s.releaseSurfaceHints(nil) // nothing bound: a no-op
}
