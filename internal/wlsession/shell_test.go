package wlsession

import "testing"

// shellProtocols are the wayle-facing protocols added for issue #33.
// Each is feature-detected: binding one must never become a Connect
// requirement, so a compositor without any of them still runs every
// existing gelm behavior.
var shellProtocols = []string{
	"zwlr_foreign_toplevel_manager_v1",
	"xdg_activation_v1",
	"zwp_idle_inhibit_manager_v1",
	"zwp_keyboard_shortcuts_inhibit_manager_v1",
	"zxdg_output_manager_v1",
}

// TestShellProtocolsAreOptional pins that none of the shell
// protocols leaks into the required globals, and that each missing
// global is silently skipped rather than fatal.
func TestShellProtocolsAreOptional(t *testing.T) {
	have := make(map[string]bool)
	for _, g := range requiredGlobals {
		if have[g] {
			t.Errorf("duplicate required global %q", g)
		}
		have[g] = true
	}
	for _, p := range shellProtocols {
		if have[p] {
			t.Errorf("%q is required; it must stay feature-detected", p)
		}
		// With only the required globals bound, a compositor missing
		// this shell protocol still connects.
		if missing := missingGlobals(have); len(missing) != 0 {
			t.Errorf("without %s: missing = %v, want nothing", p, missing)
		}
	}
}
