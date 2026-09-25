package wlsession

import "testing"

func TestMissingGlobals(t *testing.T) {
	t.Run("nothing present lists every required global", func(t *testing.T) {
		got := missingGlobals(map[string]bool{})
		if len(got) != len(requiredGlobals) {
			t.Errorf("want %d missing globals, got %v", len(requiredGlobals), got)
		}
	})

	t.Run("everything present lists nothing", func(t *testing.T) {
		have := make(map[string]bool)
		for _, g := range requiredGlobals {
			have[g] = true
		}
		if got := missingGlobals(have); len(got) != 0 {
			t.Errorf("want no missing globals, got %v", got)
		}
	})

	t.Run("only the absent ones are listed", func(t *testing.T) {
		have := map[string]bool{"wl_compositor": true, "wl_shm": true}
		got := missingGlobals(have)
		if len(got) != 2 || got[0] != "wl_output" || got[1] != "zwlr_layer_shell_v1" {
			t.Errorf("want [wl_output zwlr_layer_shell_v1], got %v", got)
		}
	})
}

func TestBindVersion(t *testing.T) {
	if got := bindVersion(6, 4); got != 4 {
		t.Errorf("bindVersion(6, 4) = %d, want 4", got)
	}
	if got := bindVersion(2, 4); got != 2 {
		t.Errorf("bindVersion(2, 4) = %d, want 2 (must not bind above advertised)", got)
	}
}
