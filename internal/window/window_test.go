package window

import (
	"errors"
	"testing"

	"github.com/neurlang/wayland/xdg"
)

func TestConfigureHandshake(t *testing.T) {
	t.Run("surface configure marks configured and records nothing alone", func(t *testing.T) {
		w := &Window{}
		w.HandleSurfaceConfigure(xdg.SurfaceConfigureEvent{Serial: 7})
		if !w.configured {
			t.Error("surface configure must set the configured flag")
		}
		if gotW, gotH := w.Size(); gotW != 0 || gotH != 0 {
			t.Errorf("size = %dx%d, want 0x0 (surface configure carries no size)", gotW, gotH)
		}
	})

	t.Run("toplevel configure records the size", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 320, Height: 200})
		gotW, gotH := w.Size()
		if gotW != 320 || gotH != 200 {
			t.Errorf("size = %dx%d, want 320x200", gotW, gotH)
		}
	})

	t.Run("zero axes keep the previous size", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 320, Height: 200})
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{})
		gotW, gotH := w.Size()
		if gotW != 320 || gotH != 200 {
			t.Errorf("zero configure clobbered size: %dx%d", gotW, gotH)
		}
	})

	t.Run("a second configure resizes", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 320, Height: 200})
		w.HandleToplevelConfigure(xdg.ToplevelConfigureEvent{Width: 640, Height: 400})
		gotW, gotH := w.Size()
		if gotW != 640 || gotH != 400 {
			t.Errorf("size = %dx%d, want 640x400", gotW, gotH)
		}
	})
}

func TestEnsureUsable(t *testing.T) {
	t.Run("fresh window is not usable", func(t *testing.T) {
		if err := (&Window{}).EnsureUsable(); !errors.Is(err, ErrNotConfigured) {
			t.Errorf("want ErrNotConfigured, got %v", err)
		}
	})

	t.Run("configured window is usable", func(t *testing.T) {
		w := &Window{}
		w.HandleSurfaceConfigure(xdg.SurfaceConfigureEvent{})
		if err := w.EnsureUsable(); err != nil {
			t.Errorf("configured window must be usable, got %v", err)
		}
	})

	t.Run("closed window is not usable even after configure", func(t *testing.T) {
		w := &Window{}
		w.HandleSurfaceConfigure(xdg.SurfaceConfigureEvent{})
		w.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if err := w.EnsureUsable(); !errors.Is(err, ErrClosed) {
			t.Errorf("want ErrClosed, got %v", err)
		}
		if !w.Closed() {
			t.Error("Closed must report the close event")
		}
	})
}

func TestPong(t *testing.T) {
	t.Run("pong on a zero-value window does not panic", func(t *testing.T) {
		w := &Window{}
		w.Pong(3)
	})
}

func TestDecorateNilManager(t *testing.T) {
	t.Run("a nil decoration manager is a silent no-op", func(t *testing.T) {
		if err := (&Window{}).Decorate(nil); err != nil {
			t.Errorf("Decorate(nil) = %v, want nil", err)
		}
	})
}
