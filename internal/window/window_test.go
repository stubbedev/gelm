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

// TestLimitAndResizeStateOnWirelessWindow pins the wire-free paths: a
// window without a wire records limits, answers ServerDecorated, and
// treats resize grabs as no-ops instead of panicking.
func TestLimitAndResizeStateOnWirelessWindow(t *testing.T) {
	w := &Window{}
	if err := w.SetMinSize(100, 50); err != nil {
		t.Errorf("wire-free SetMinSize = %v, want nil", err)
	}
	if err := w.SetMaxSize(400, 300); err != nil {
		t.Errorf("wire-free SetMaxSize = %v, want nil", err)
	}
	minW, minH, maxW, maxH := w.SizeLimits()
	if minW != 100 || minH != 50 || maxW != 400 || maxH != 300 {
		t.Errorf("SizeLimits = %d,%d,%d,%d, want 100,50,400,300", minW, minH, maxW, maxH)
	}
	if err := w.Resize(nil, 1, 10); err != nil {
		t.Errorf("wire-free Resize = %v, want nil", err)
	}
	if w.ServerDecorated() {
		t.Error("undecorated window reports server decorations")
	}
}

// TestCloseRequestVeto pins the veto semantics: a nil veto accepts
// every close request, a false return keeps the window open, and a
// true return (or an explicit Close) closes it.
func TestCloseRequestVeto(t *testing.T) {
	t.Run("nil veto accepts the close", func(t *testing.T) {
		w := &Window{}
		w.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if !w.Closed() {
			t.Error("close request did not close the window")
		}
	})

	t.Run("a vetoing callback keeps the window open", func(t *testing.T) {
		w := &Window{}
		w.SetCloseRequest(func() bool { return false })
		w.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if w.Closed() {
			t.Error("vetoed close request closed the window")
		}
	})

	t.Run("an accepting callback closes", func(t *testing.T) {
		w := &Window{}
		count := 0
		w.SetCloseRequest(func() bool {
			count++
			return count >= 2
		})
		w.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if w.Closed() {
			t.Error("first close should have been vetoed")
		}
		w.HandleToplevelClose(xdg.ToplevelCloseEvent{})
		if !w.Closed() {
			t.Error("second close should have been accepted")
		}
	})

	t.Run("explicit Close ignores the veto", func(t *testing.T) {
		w := &Window{}
		w.SetCloseRequest(func() bool { return false })
		w.Close()
		if !w.Closed() {
			t.Error("client-side Close was vetoed")
		}
	})
}
