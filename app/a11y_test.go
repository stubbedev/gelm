//go:build atspi

// ServeAccessibility's consumer contract: the options type is nameable
// outside gelm (A11YOptions), and a broken address is the defined
// failure, not a panic.
package app

import (
	"strings"
	"testing"

	"github.com/stubbedev/gelm/internal/wlsession"
)

func TestServeAccessibilityTakesExportedOptions(t *testing.T) {
	app := NewApplication(&wlsession.Session{})
	err := app.ServeAccessibility(A11YOptions{Address: "unix:path=/nonexistent/gelm-a11y-test"})
	if err == nil {
		t.Fatal("bogus address: want the dial failure")
	}
	if !strings.Contains(err.Error(), "a11y") && !strings.Contains(err.Error(), "connect") && !strings.Contains(err.Error(), "dial") {
		t.Fatalf("error %v does not name the accessibility bus", err)
	}
	// Idempotence holds on failure too: a second call is attempted, not
	// wedged by a half-started bridge.
	if err := app.ServeAccessibility(A11YOptions{Address: "unix:path=/nonexistent/gelm-a11y-test"}); err == nil {
		t.Fatal("second ServeAccessibility on the same broken bus should also fail")
	}
}
