// Compositor disconnect policy coverage: the typed error the loop
// returns, the exactly-once hook, the children-first teardown (window
// pool, session fd, invoke queues), and the goroutine budget after the
// exit path. The disconnect is the real thing — a unix socket whose
// peer closed — no compositor.
package app

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/widget"
)

// deadPeerSession connects a real Session to a unix listener and then
// closes the listener: the client's socket is now a dead fd, the same
// state a SIGKILLed compositor leaves. The caller must Close it.
func deadPeerSession(t *testing.T) (*wlsession.Session, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("WAYLAND_DISPLAY", "wayland-dead")
	sock := filepath.Join(dir, "wayland-dead")
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := wl.Connect("")
	if err != nil {
		ln.Close()
		t.Fatal(err)
	}
	ln.Close()
	t.Cleanup(func() { _ = d.Context().Close() })
	return &wlsession.Session{Display: d}, sock
}

// disconnectApp builds an Application over the dead session carrying
// one harness window at a fixed size, so the loop pass is otherwise
// idle: nothing draws, nothing wakes, the only event is the dead
// socket.
func disconnectApp(t *testing.T, sess *wlsession.Session) (*Application, *hostWindow, *paintHarness) {
	t.Helper()
	anim.Reset()
	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 320, 200)
	h.wnd.sess = sess
	a := NewApplication(sess)
	a.windows = append(a.windows, h.wnd)
	t.Cleanup(func() { widget.SetInvoker(nil) })
	return a, h.wnd, h
}

// socketFDOpen reports whether any open fd still points at the display
// socket.
func socketFDOpen(sock string) bool {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if target, err := os.Readlink("/proc/self/fd/" + e.Name()); err == nil && target == sock {
			return true
		}
	}
	return false
}

// TestApplicationDisconnectPolicy is the acceptance in one pass: the
// loop over a dead socket runs OnDisconnect exactly once with the
// classified reason, closes the window pool and the session fd, shuts
// the invoke queues, leaves no goroutine behind, and returns an error
// callers match with ErrDisconnected.
func TestApplicationDisconnectPolicy(t *testing.T) {
	sess, sock := deadPeerSession(t)
	a, _, h := disconnectApp(t, sess)

	var events []DisconnectedEvent
	a.OnDisconnect(func(ev DisconnectedEvent) { events = append(events, ev) })
	if socketFDOpen(sock) {
		t.Fatal("display socket fd missing before Run; the test is wired wrong")
	}
	baseline := runtime.NumGoroutine()

	err := a.Run()
	de, ok := errors.AsType[*wlsession.DisconnectError](err)
	if !ok {
		t.Fatalf("Run over a dead socket = %v, want a *wlsession.DisconnectError", err)
	}
	if de.Reason != wlsession.DisconnectConnectionLost {
		t.Errorf("reason = %s, want connection lost", de.Reason)
	}
	if !errors.Is(err, ErrDisconnected) {
		t.Errorf("Run error %v does not match ErrDisconnected", err)
	}

	if len(events) != 1 {
		t.Fatalf("OnDisconnect fired %d times, want exactly once", len(events))
	}
	if events[0].Reason != wlsession.DisconnectConnectionLost {
		t.Errorf("event reason = %s, want connection lost", events[0].Reason)
	}

	// Children first: the window's pool stopped handing out buffers,
	// and the display socket fd is closed.
	if _, err := h.wnd.pool.Acquire(); !errors.Is(err, buffer.ErrClosed) {
		t.Errorf("pool after disconnect = %v, want buffer.ErrClosed", err)
	}
	if socketFDOpen(sock) {
		t.Error("display socket fd still open after the disconnect teardown")
	}

	// The queues died with the loop: a late Invoke drops instead of
	// accumulating (mirrors invoke_test.go's post-shutdown contract).
	a.Invoke(func() { t.Error("invoke ran after a disconnect exit") })
	if got := a.queues.pending(); got != 0 {
		t.Errorf("pending invokes after disconnect = %d, want 0", got)
	}

	// The once-guard: a second dead-socket error (a nested dispatch
	// loop surfacing the same death) must not re-run the policy.
	a.handleDisconnect(&wlsession.DisconnectError{Reason: wlsession.DisconnectConnectionLost})
	if len(events) != 1 {
		t.Errorf("OnDisconnect fired again on a repeat disconnect: %d events", len(events))
	}

	// No goroutine left spinning: the count settles back to the
	// pre-run baseline.
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > baseline+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > baseline+2 {
		t.Errorf("goroutines after the exit path = %d, want the pre-run baseline %d", n, baseline)
	}
}

// TestDisconnectDefaultPolicyCleansUp: without a hook the same teardown
// runs and the same typed error comes back — the hook is optional
// observation, the clean exit is the default policy.
func TestDisconnectDefaultPolicyCleansUp(t *testing.T) {
	sess, sock := deadPeerSession(t)
	a, _, h := disconnectApp(t, sess)

	err := a.Run()
	if !errors.Is(err, ErrDisconnected) {
		t.Fatalf("Run = %v, want ErrDisconnected", err)
	}
	if _, err := h.wnd.pool.Acquire(); !errors.Is(err, buffer.ErrClosed) {
		t.Errorf("pool after disconnect = %v, want buffer.ErrClosed", err)
	}
	if socketFDOpen(sock) {
		t.Error("display socket fd still open after the default teardown")
	}
	if !a.disconnectOnce.Load() {
		t.Error("the once-guard never armed without a hook")
	}
}

// failingSurface is a fakeSurface whose commit hits a dead socket: the
// frame path must classify like the dispatch path.
type failingSurface struct {
	fakeSurface
}

func (f *failingSurface) Commit() error {
	return fmt.Errorf("write unix ->@dead: %w", syscall.EPIPE)
}

// TestDisconnectThroughFramePath: a wire failure mid-frame (attach,
// damage, commit on a dead socket) surfaces through drawErr and runs
// the same policy — the audit covers every dispatch surface, not just
// the park.
func TestDisconnectThroughFramePath(t *testing.T) {
	sess, _ := deadPeerSession(t)
	a, _, h := disconnectApp(t, sess)
	h.wnd.surf = &failingSurface{fakeSurface{}}
	h.wnd.dirty = true

	var fired int
	a.OnDisconnect(func(DisconnectedEvent) { fired++ })

	err := a.Run()
	if !errors.Is(err, ErrDisconnected) {
		t.Fatalf("Run = %v, want ErrDisconnected (drawErr = %v)", err, h.wnd.drawErr)
	}
	de, ok := errors.AsType[*wlsession.DisconnectError](err)
	if !ok || de.Reason != wlsession.DisconnectConnectionLost {
		t.Errorf("frame-path disconnect = %v, want connection-lost reason", err)
	}
	if fired != 1 {
		t.Errorf("OnDisconnect fired %d times, want once", fired)
	}
}

// TestDisconnectedEventCarriesUnderlyingError: the event's Err is the
// classified wire error, so an app can log what actually failed.
func TestDisconnectedEventCarriesUnderlyingError(t *testing.T) {
	sess, _ := deadPeerSession(t)
	a, _, _ := disconnectApp(t, sess)

	var msg string
	a.OnDisconnect(func(ev DisconnectedEvent) { msg = ev.Err.Error() })
	_ = a.Run()
	if !strings.Contains(msg, "compositor disconnected") {
		t.Errorf("event error %q does not name the disconnect", msg)
	}
}
