package wlsession

import (
	"testing"

	"github.com/neurlang/wayland/wl"
)

// TestOutputHotplug pins the registry-level output bookkeeping: added
// outputs land in Outputs() in arrival order and a removed global
// drops the matching output and fires the removal hook.
func TestOutputHotplug(t *testing.T) {
	s := &Session{
		globals:         make(map[string]bool),
		ifaceNames:      make(map[uint32]string),
		surfaceHandlers: make(map[*wl.Surface]SurfacePointerHandler),
	}

	var added, removed []*Output
	s.OnOutputAdded = func(o *Output) { added = append(added, o) }
	s.OnOutputRemoved = func(o *Output) { removed = append(removed, o) }

	s.trackOutput(&Output{Scale: 1, name: 40})
	s.trackOutput(&Output{Scale: 1, name: 41})
	// Mirror the registry bookkeeping HandleRegistryGlobal performs.
	s.ifaceNames[40] = "wl_output"
	s.ifaceNames[41] = "wl_output"
	if len(s.Outputs()) != 2 || len(added) != 2 {
		t.Fatalf("outputs = %d, added = %d, want 2/2", len(s.Outputs()), len(added))
	}
	if s.Outputs()[0].name != 40 || s.Outputs()[1].name != 41 {
		t.Errorf("output registry names = %d,%d, want 40,41", s.Outputs()[0].name, s.Outputs()[1].name)
	}

	s.HandleRegistryGlobalRemove(wl.RegistryGlobalRemoveEvent{Name: 40})
	if len(s.Outputs()) != 1 || len(removed) != 1 {
		t.Fatalf("after removal: outputs = %d, removed = %d, want 1/1", len(s.Outputs()), len(removed))
	}
	if removed[0].name != 40 {
		t.Errorf("removed output name = %d, want 40", removed[0].name)
	}
	if _, ok := s.ifaceNames[40]; ok {
		t.Error("removed output's registry name still tracked")
	}

	// Removing an unknown name is a no-op.
	s.HandleRegistryGlobalRemove(wl.RegistryGlobalRemoveEvent{Name: 99})
	if len(s.Outputs()) != 1 {
		t.Errorf("unknown removal changed the output set: %d", len(s.Outputs()))
	}
}
