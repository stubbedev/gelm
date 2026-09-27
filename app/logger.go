// Logging convention for the application-facing package. gelm the
// library is silent by default; the demo binaries and any embedder
// that wants the library's protocol warnings install a logger once —
// here or on wlsession directly — and everything routes through it
// (internal/logutil holds the levels contract).

package app

import (
	"log/slog"

	"github.com/stubbedev/gelm/internal/wlsession"
)

// SetLogger installs the library-wide logger: gelm's internals (session
// protocol warnings, event-loop diagnostics, theme contrast warnings,
// the appearance monitor) emit through it. Nothing logs per-frame or
// per-keypress, and nothing logs at Info. A nil logger selects the
// default: discard all output, so an application that never calls this
// sees no library output on stdout or stderr. Call it before Connect;
// safe from any goroutine.
func SetLogger(l *slog.Logger) { wlsession.SetLogger(l) }
