package headlesstest

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// LoopbackDisplay binds a real unix socket as a wayland display the
// test process owns: the far end for wire-level and dead-peer tests,
// no compositor involved. The socket lives in a fresh runtime dir
// created at the temp root instead of inside t.TempDir, because the
// kernel caps a socket path at 107 bytes and CI's nested temp dirs
// push a t.TempDir path past that - bind then fails with EINVAL
// before the test body runs. The process env is pointed at the
// display, so wl.Connect("") reaches the listener. The caller closes
// the listener; the dir goes away at cleanup.
func LoopbackDisplay(t *testing.T, display string) (*net.UnixListener, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "gelm-wl-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, display)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", dir)
	t.Setenv("WAYLAND_DISPLAY", display)
	return ln, sock
}
