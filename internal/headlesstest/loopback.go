package headlesstest

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

// RuntimeDir creates a private XDG_RUNTIME_DIR at the temp root and
// points the process env at it. It sits outside t.TempDir because the
// kernel caps a socket path at 107 bytes and CI's nested temp dirs push
// a t.TempDir path past that, so bind fails with EINVAL. The dir goes
// away at cleanup.
func RuntimeDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "lys-ui-rt-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)
	return dir
}

// LoopbackDisplay binds a real unix socket as a wayland display the
// test process owns: the far end for wire-level and dead-peer tests,
// no compositor involved. The socket lives in a fresh RuntimeDir and
// the process env is pointed at the display, so wl.Connect("")
// reaches the listener. The caller closes the listener.
func LoopbackDisplay(t *testing.T, display string) (*net.UnixListener, string) {
	t.Helper()
	sock := filepath.Join(RuntimeDir(t), display)
	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("WAYLAND_DISPLAY", display)
	return ln, sock
}
