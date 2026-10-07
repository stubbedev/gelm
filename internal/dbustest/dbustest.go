// Package dbustest runs private D-Bus daemons for tests: a session-
// style bus on a fresh unix socket, configured from a file the helper
// writes itself, so a test needs dbus-daemon on PATH and nothing in
// /etc (where a distro's session.conf may be absent - NixOS, minimal
// containers).
package dbustest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// config is a minimal session bus: anyone on the socket may connect,
// own names, and talk to anyone.
const config = `<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-BUS Bus Configuration 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">
<busconfig>
  <type>session</type>
  <listen>unix:path=%s</listen>
  <auth>EXTERNAL</auth>
  <policy context="default">
    <allow send_destination="*" eavesdrop="true"/>
    <allow eavesdrop="true"/>
    <allow own="*"/>
  </policy>
</busconfig>
`

// Start launches a private bus on a fresh socket in a test temp dir
// and returns its address and pid; the daemon dies with the test.
// Skips when dbus-daemon is not installed.
func Start(t testing.TB) (string, int) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "bus")
	if len(sock) > 88 {
		t.Skipf("socket path %q too long for AF_UNIX", sock)
	}
	return StartAt(t, sock)
}

// StartAt launches a private bus bound to sock (a fresh path).
// Readiness means a real connection, not a socket file: a file can
// outlive its daemon, and a daemon can outlive its sockets.
func StartAt(t testing.TB, sock string) (string, int) {
	t.Helper()
	daemon, err := exec.LookPath("dbus-daemon")
	if err != nil {
		t.Skip("dbus-daemon not installed; the test needs a real bus")
	}
	conf := filepath.Join(t.TempDir(), "bus.conf")
	if err := os.WriteFile(conf, []byte(fmt.Sprintf(config, sock)), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(daemon, "--config-file="+conf, "--fork", "--nopidfile", //nolint:gosec // the test daemon from PATH, a config this helper wrote
		"--print-address=1", "--print-pid=1")
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("dbus-daemon failed: %v; stderr: %s", err, errOut.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("dbus-daemon printed %q, want address and pid", out.String())
	}
	address := strings.TrimSpace(lines[0])
	pid, err := strconv.Atoi(strings.TrimSpace(lines[1]))
	if err != nil {
		t.Fatalf("dbus-daemon pid %q: %v", lines[1], err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGTERM) })
	deadline := time.Now().Add(2 * time.Second)
	for {
		if probe, err := dbus.Connect(address); err == nil {
			_ = probe.Close()
			return address, pid
		}
		if time.Now().After(deadline) {
			t.Fatal("private bus never accepted a connection")
		}
		time.Sleep(2 * time.Millisecond)
	}
}
