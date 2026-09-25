package layersurface

import (
	"errors"
	"testing"

	"github.com/stubbedev/gelm/wlr"
)

func TestValidateAutoAxisNeedsBothEdgesAnchored(t *testing.T) {
	t.Run("auto width with a single horizontal anchor is rejected", func(t *testing.T) {
		c := Config{Anchor: AnchorTop}
		if err := c.validate(); err == nil {
			t.Error("width 0 with only top anchored must be rejected")
		}
	})

	t.Run("auto height with a single vertical anchor is rejected", func(t *testing.T) {
		c := Config{Anchor: AnchorLeft | AnchorRight}
		if err := c.validate(); err == nil {
			t.Error("height 0 with only left and right anchored must be rejected")
		}
	})

	t.Run("auto width spanning left and right passes", func(t *testing.T) {
		c := Config{Anchor: AnchorTop | AnchorLeft | AnchorRight, Height: 32}
		if err := c.validate(); err != nil {
			t.Errorf("spanning bar config must pass: %v", err)
		}
	})

	t.Run("auto both axes centered passes", func(t *testing.T) {
		c := Config{Anchor: AnchorTop | AnchorBottom | AnchorLeft | AnchorRight}
		if err := c.validate(); err != nil {
			t.Errorf("fully anchored auto-size config must pass: %v", err)
		}
	})

	t.Run("fixed size with a single anchor passes", func(t *testing.T) {
		c := Config{Anchor: AnchorTop, Width: 100, Height: 32}
		if err := c.validate(); err != nil {
			t.Errorf("fixed-size config must pass: %v", err)
		}
	})
}

func TestConfigureStateMachine(t *testing.T) {
	t.Run("configure records size and marks configured", func(t *testing.T) {
		s := &Surface{}
		s.applyConfigure(800, 32)
		if !s.configured {
			t.Error("configure must set the configured flag")
		}
		w, h := s.Size()
		if w != 800 || h != 32 {
			t.Errorf("size = %dx%d, want 800x32", w, h)
		}
	})

	t.Run("zero axes keep the previous size", func(t *testing.T) {
		s := &Surface{}
		s.applyConfigure(800, 32)
		s.applyConfigure(0, 0)
		w, h := s.Size()
		if w != 800 || h != 32 {
			t.Errorf("zero configure clobbered size: %dx%d", w, h)
		}
	})

	t.Run("a second configure resizes", func(t *testing.T) {
		s := &Surface{}
		s.applyConfigure(800, 32)
		s.applyConfigure(1024, 32)
		w, _ := s.Size()
		if w != 1024 {
			t.Errorf("width = %d, want 1024 after resize configure", w)
		}
	})
}

func TestEnsureUsableGate(t *testing.T) {
	t.Run("fresh surface is not usable", func(t *testing.T) {
		if err := (&Surface{}).EnsureUsable(); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("want ErrNotConfigured, got %v", err)
		}
	})

	t.Run("configured surface is usable", func(t *testing.T) {
		s := &Surface{}
		s.applyConfigure(800, 32)
		if err := s.EnsureUsable(); err != nil {
			t.Errorf("configured surface must be usable, got %v", err)
		}
	})

	t.Run("closed surface is not usable, even after configure", func(t *testing.T) {
		s := &Surface{}
		s.applyConfigure(800, 32)
		s.HandleZwlrLayerSurfaceV1Closed(wlr.ZwlrLayerSurfaceV1ClosedEvent{})
		if err := s.EnsureUsable(); !errors.Is(err, ErrClosed) {
			t.Errorf("want ErrClosed, got %v", err)
		}
	})
}

func TestProtocolEnumWireValues(t *testing.T) {
	t.Run("layers", func(t *testing.T) {
		want := map[Layer]uint32{
			LayerBackground: wlr.ZwlrLayerShellV1LayerBackground,
			LayerBottom:     wlr.ZwlrLayerShellV1LayerBottom,
			LayerTop:        wlr.ZwlrLayerShellV1LayerTop,
			LayerOverlay:    wlr.ZwlrLayerShellV1LayerOverlay,
		}
		for l, wire := range want {
			if uint32(l) != wire {
				t.Errorf("%d != wire value %d", l, wire)
			}
		}
	})

	t.Run("anchors", func(t *testing.T) {
		if uint32(AnchorTop) != wlr.ZwlrLayerSurfaceV1AnchorTop ||
			uint32(AnchorBottom) != wlr.ZwlrLayerSurfaceV1AnchorBottom ||
			uint32(AnchorLeft) != wlr.ZwlrLayerSurfaceV1AnchorLeft ||
			uint32(AnchorRight) != wlr.ZwlrLayerSurfaceV1AnchorRight {
			t.Error("anchor bits diverge from the protocol constants")
		}
	})

	t.Run("keyboard modes", func(t *testing.T) {
		want := map[KeyboardMode]uint32{
			KeyboardNone:      wlr.ZwlrLayerSurfaceV1KeyboardInteractivityNone,
			KeyboardExclusive: wlr.ZwlrLayerSurfaceV1KeyboardInteractivityExclusive,
			KeyboardOnDemand:  wlr.ZwlrLayerSurfaceV1KeyboardInteractivityOnDemand,
		}
		for m, wire := range want {
			if uint32(m) != wire {
				t.Errorf("%d != wire value %d", m, wire)
			}
		}
	})
}
