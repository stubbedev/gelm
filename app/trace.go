package app

import "github.com/stubbedev/lys-ui/internal/debug"

// Trace writes one line to lys-ui's debug trace stream, the one the
// toolkit traces its wire, routing and frame decisions into: compiled
// in only with the lysuidebug build tag, and printed only for the
// categories GOELM_DEBUG admits (comma-separated names, "*" for all).
// Applications trace their own events beside the toolkit's in one
// timeline.
func Trace(category, format string, args ...any) { debug.Log(category, format, args...) }
