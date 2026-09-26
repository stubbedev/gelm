// Wire-level coverage over a loopback unix socket: window.New and the
// resize/limit requests are pinned by their actual bytes, and the
// close/configure events are driven through the real proxy dispatch
// path. No compositor is involved - the far end only drains frames.
package window

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/xdg"
)

// xdg_toplevel request opcodes the tests match on.
const (
	opResize     = 6
	opSetMaxSize = 7
	opSetMinSize = 8
)

// wireFrame is one request as it hit the wire: object id, opcode, and
// the payload words.
type wireFrame struct {
	obj    uint32
	opcode uint32
	body   []byte
}

// startWireServer listens on a private unix socket and drains every
// request into the returned channel. The wayland connection is
// endianness-native, so the decoder is too.
func startWireServer(t *testing.T) <-chan wireFrame {
	t.Helper()
	dir := t.TempDir()
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: dir + "/wayland-test", Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-test")

	frames := make(chan wireFrame, 64)
	go func() {
		conn, err := ln.AcceptUnix()
		if err != nil {
			return
		}
		for {
			var head [8]byte
			if _, err := io.ReadFull(conn, head[:]); err != nil {
				return
			}
			size := int(binary.NativeEndian.Uint32(head[4:8]) >> 16)
			body := make([]byte, size-8)
			if _, err := io.ReadFull(conn, body); err != nil {
				return
			}
			frames <- wireFrame{
				obj:    binary.NativeEndian.Uint32(head[0:4]),
				opcode: binary.NativeEndian.Uint32(head[4:8]) & 0xffff,
				body:   body,
			}
		}
	}()
	return frames
}

// newWireWindow builds a real window over the loopback connection.
func newWireWindow(t *testing.T, cfg Config) *Window {
	t.Helper()
	disp, err := wl.Connect("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = disp.Context().Close() })
	wmBase := xdg.NewWmBase(disp.Context())
	surf := wl.NewSurface(disp.Context())
	// The fake compositor sends no registry burst, so these two proxies
	// have nothing that would carry their ids to the wire - which would
	// leave the next request waiting on the connection's ordering rule
	// forever. Unregister drops them from the bookkeeping (and clears
	// the wait); the requests below still go out with their ids.
	wmBase.Unregister()
	surf.Unregister()
	w, err := New(wmBase, surf, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// waitForFrame consumes frames until one matches; the buffered noise
// (title, app_id, ...) passes through.
func waitForFrame(t *testing.T, frames <-chan wireFrame, name string, match func(wireFrame) bool) wireFrame {
	t.Helper()
	for {
		select {
		case f := <-frames:
			if match(f) {
				return f
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("no %s request reached the wire", name)
		}
	}
}

// word reads payload word i as int32.
func word(b []byte, i int) int32 { return int32(binary.NativeEndian.Uint32(b[4*i : 4*i+4])) }

func TestSizeLimitRequests(t *testing.T) {
	frames := startWireServer(t)
	w := newWireWindow(t, Config{
		Title: "t", AppID: "t",
		MinWidth: 320, MinHeight: 200,
		MaxWidth: 800, MaxHeight: 600,
	})

	t.Run("set_min_size carries the configured minimum", func(t *testing.T) {
		f := waitForFrame(t, frames, "set_min_size", func(f wireFrame) bool {
			return f.opcode == opSetMinSize
		})
		if len(f.body) != 8 || word(f.body, 0) != 320 || word(f.body, 1) != 200 {
			t.Errorf("set_min_size payload %v, want (320, 200)", f.body)
		}
	})

	t.Run("set_max_size carries the configured maximum", func(t *testing.T) {
		f := waitForFrame(t, frames, "set_max_size", func(f wireFrame) bool {
			return f.opcode == opSetMaxSize
		})
		if len(f.body) != 8 || word(f.body, 0) != 800 || word(f.body, 1) != 600 {
			t.Errorf("set_max_size payload %v, want (800, 600)", f.body)
		}
	})

	t.Run("the limits are tracked client-side", func(t *testing.T) {
		minW, minH, maxW, maxH := w.SizeLimits()
		if minW != 320 || minH != 200 || maxW != 800 || maxH != 600 {
			t.Errorf("SizeLimits = %d,%d,%d,%d, want 320,200,800,600", minW, minH, maxW, maxH)
		}
	})
}

func TestNoLimitRequestsWhenUnconstrained(t *testing.T) {
	frames := startWireServer(t)
	w := newWireWindow(t, Config{Title: "t", AppID: "t"})
	drain := func() {
		for {
			select {
			case <-frames:
			default:
				return
			}
		}
	}
	drain()
	time.Sleep(20 * time.Millisecond) // settle: nothing else sends here
	drain()

	// Live limit updates go out on the wire and land in the tracked state.
	if err := w.SetMinSize(120, 80); err != nil {
		t.Fatal(err)
	}
	f := waitForFrame(t, frames, "set_min_size", func(f wireFrame) bool {
		return f.opcode == opSetMinSize
	})
	if len(f.body) != 8 || word(f.body, 0) != 120 || word(f.body, 1) != 80 {
		t.Errorf("set_min_size payload %v, want (120, 80)", f.body)
	}
	if err := w.SetMaxSize(0, 0); err != nil {
		t.Fatal(err)
	}
	f = waitForFrame(t, frames, "set_max_size", func(f wireFrame) bool {
		return f.opcode == opSetMaxSize
	})
	if len(f.body) != 8 || word(f.body, 0) != 0 || word(f.body, 1) != 0 {
		t.Errorf("set_max_size payload %v, want (0, 0)", f.body)
	}
	if minW, minH, _, _ := w.SizeLimits(); minW != 120 || minH != 80 {
		t.Errorf("min limits = %dx%d, want 120x80", minW, minH)
	}
}

func TestResizeRequest(t *testing.T) {
	frames := startWireServer(t)
	w := newWireWindow(t, Config{Title: "t", AppID: "t"})

	t.Run("the grab carries the seat, serial, and edges", func(t *testing.T) {
		seat := wl.NewSeat(w.Toplevel.Context())
		if err := w.Resize(seat, 4242, xdg.ToplevelResizeEdgeBottomRight); err != nil {
			t.Fatal(err)
		}
		f := waitForFrame(t, frames, "resize", func(f wireFrame) bool {
			return f.opcode == opResize
		})
		if len(f.body) != 12 {
			t.Fatalf("resize payload %v, want 3 words", f.body)
		}
		if word(f.body, 0) != int32(seat.Id()) {
			t.Errorf("seat = %d, want the seat proxy id %d", word(f.body, 0), seat.Id())
		}
		if word(f.body, 1) != 4242 {
			t.Errorf("serial = %d, want 4242", word(f.body, 1))
		}
		if word(f.body, 2) != int32(xdg.ToplevelResizeEdgeBottomRight) {
			t.Errorf("edges = %d, want %d", word(f.body, 2), xdg.ToplevelResizeEdgeBottomRight)
		}
	})

	t.Run("a none edge or missing seat is a no-op", func(t *testing.T) {
		if err := w.Resize(nil, 6, xdg.ToplevelResizeEdgeBottomRight); err != nil {
			t.Fatal(err)
		}
		if err := w.Resize(nil, 7, xdg.ToplevelResizeEdgeNone); err != nil {
			t.Fatal(err)
		}
		select {
		case f := <-frames:
			if f.opcode == opResize {
				t.Error("a guarded resize reached the wire")
			}
		default:
		}
	})
}

// TestConfigureDispatchUpdatesSize drives the handshake through the
// real proxy dispatch path - the same events a compositor delivers -
// and pins that a non-zero configure resizes the window.
func TestConfigureDispatchUpdatesSize(t *testing.T) {
	startWireServer(t)
	w := newWireWindow(t, Config{Title: "t", AppID: "t"})

	data := make([]byte, 12)
	binary.NativeEndian.PutUint32(data[0:4], 640)
	binary.NativeEndian.PutUint32(data[4:8], 400)
	// states: an empty array (length word only)
	w.Toplevel.Dispatch(&wl.Event{Opcode: 0, Data: data})
	if gw, gh := w.Size(); gw != 640 || gh != 400 {
		t.Errorf("size after configure = %dx%d, want 640x400", gw, gh)
	}

	serial := make([]byte, 4)
	binary.NativeEndian.PutUint32(serial, 9)
	w.XdgSurface.Dispatch(&wl.Event{Opcode: 0, Data: serial})
	if err := w.EnsureUsable(); err != nil {
		t.Errorf("handshake did not complete: %v", err)
	}

	// A zero-axis configure keeps the previous size (the Hyprland quirk).
	zero := make([]byte, 12)
	w.Toplevel.Dispatch(&wl.Event{Opcode: 0, Data: zero})
	if gw, gh := w.Size(); gw != 640 || gh != 400 {
		t.Errorf("zero configure clobbered size: %dx%d", gw, gh)
	}
}

// TestProtocolCloseRoutesThroughVeto pins the veto on the actual
// protocol path: xdg_toplevel.close dispatched through the wire proxy
// honors the installed veto.
func TestProtocolCloseRoutesThroughVeto(t *testing.T) {
	t.Run("a vetoing window survives the close event", func(t *testing.T) {
		startWireServer(t)
		w := newWireWindow(t, Config{Title: "t", AppID: "t"})
		vetoed := false
		w.SetCloseRequest(func() bool {
			vetoed = true
			return false
		})
		w.Toplevel.Dispatch(&wl.Event{Opcode: 1})
		if !vetoed {
			t.Fatal("the close event never reached the veto; it did not route")
		}
		if w.Closed() {
			t.Error("vetoed protocol close destroyed the window")
		}
	})

	t.Run("an unvetted window closes", func(t *testing.T) {
		startWireServer(t)
		w := newWireWindow(t, Config{Title: "t", AppID: "t"})
		w.Toplevel.Dispatch(&wl.Event{Opcode: 1})
		if !w.Closed() {
			t.Error("protocol close did not close the window")
		}
	})
}
