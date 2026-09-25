package wlsession

import "testing"

// Regression: the client never set a pointer cursor, leaving the
// compositor free to show a text caret across the whole window. The
// requested shape must resolve to a real xcursor name, never empty.
func TestEffectiveCursor(t *testing.T) {
	t.Run("empty means the default arrow", func(t *testing.T) {
		if got := effectiveCursor(""); got != defaultCursor {
			t.Errorf("effectiveCursor(\"\") = %q, want %q", got, defaultCursor)
		}
	})

	t.Run("a requested shape passes through", func(t *testing.T) {
		if got := effectiveCursor("xterm"); got != "xterm" {
			t.Errorf("effectiveCursor(xterm) = %q", got)
		}
	})
}
