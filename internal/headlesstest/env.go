package headlesstest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Env is the private headless compositor session the tests run
// against: the runtime dir the justfile's test-env recipe booted sway
// into, and the WAYLAND_DISPLAY socket name inside it. The harness
// never starts the compositor itself - booting it is the recipe's job
// (the proven path: sway on wlroots' headless backend with the pixman
// renderer inside a private XDG_RUNTIME_DIR, socket polled until it
// appears) - so the test binary carries no sway knowledge and the env
// outlives it, shared by every test in the run.
type Env struct {
	// Dir is the session's private XDG_RUNTIME_DIR (mode 0700). Sway
	// logs to sway.log inside it; client logs and the built showcase
	// binary land here too.
	Dir string
	// Display is the WAYLAND_DISPLAY socket name (e.g. "wayland-1").
	Display string
}

// Attach connects the harness to the compositor the caller booted:
// the process environment carries WAYLAND_DISPLAY and XDG_RUNTIME_DIR
// (exported by the just headless recipe, or by hand after
// just test-env). An error names what is missing, so a mis-wired run
// fails with the fix in the message instead of a connection timeout
// deep inside the first test.
func Attach() (*Env, error) {
	display, dir := os.Getenv("WAYLAND_DISPLAY"), os.Getenv("XDG_RUNTIME_DIR")
	if display == "" || dir == "" {
		return nil, errors.New("headlesstest: WAYLAND_DISPLAY or XDG_RUNTIME_DIR is unset; boot the test compositor first (just test-env) or run under just headless")
	}
	if _, err := os.Stat(filepath.Join(dir, display)); err != nil { //nolint:gosec // the dir is the recipe's private runtime dir, the display its socket name
		return nil, fmt.Errorf("headlesstest: compositor socket %s/%s: %w", dir, display, err)
	}
	return &Env{Dir: dir, Display: display}, nil
}

// strippedEnv lists the session variables that must never leak from
// the test process into a harness child: a stray XDG_RUNTIME_DIR would
// point the child at the developer's live desktop.
var strippedEnv = []string{
	"XDG_RUNTIME_DIR",
	"WAYLAND_DISPLAY",
	"WAYLAND_SOCKET",
	"SWAYSOCK",
	"I3SOCK",
	"WLR_BACKENDS",
	"WLR_LIBINPUT_NO_DEVICES",
	"WLR_RENDERER",
	"GOELM_DEBUG",
}

// privateEnv returns os.Environ() with every strippedEnv entry removed
// and the given assignments appended last, so the child sees exactly
// one copy of each.
func privateEnv(assignments ...string) []string {
	out := make([]string, 0, len(os.Environ())+len(assignments))
	for _, kv := range os.Environ() {
		strip := false
		for _, key := range strippedEnv {
			if strings.HasPrefix(kv, key+"=") {
				strip = true
				break
			}
		}
		if !strip {
			out = append(out, kv)
		}
	}
	return append(out, assignments...)
}
