// The compositor-restart kill test (#42): the headless suite's honest
// disconnect. The showcase client runs against the live sway session;
// this test SIGKILLs the compositor mid-run and asserts the client's
// defined story — the disconnect policy fires exactly once (one wire
// trace), the loop unwinds, and the process exits by itself with the
// distinct DisconnectExitCode a supervisor respawns on. No panic, no
// zombie.
//
// This file sorts after integration_test.go on purpose: killing sway
// ends the shared session, so the kill must be the suite's last act.
// The env recipe's trap-based teardown still runs afterwards and is
// happy to tear down an already-dead compositor.
package headlesstest

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stubbedev/gelm/app"
)

// killTimeout bounds how long the client may take to notice the dead
// socket and unwind after the compositor is SIGKILLed. The read fails
// in milliseconds; the budget is for a loaded CI machine, not for the
// detection itself.
const killTimeout = 30 * time.Second

// TestHeadlessCompositorKillExitsCleanly drives the clean-exit policy
// end to end: real compositor, real client, real death.
func TestHeadlessCompositorKillExitsCleanly(t *testing.T) {
	requireEnv(t)
	c, w, _ := startShowcase(t)

	// The client must be alive right up to the kill; an early exit
	// would make every later assertion vacuous.
	select {
	case <-c.Exited():
		t.Fatal("showcase exited before the compositor was killed")
	case <-time.After(250 * time.Millisecond):
	}

	pidFile := testEnv.Dir + "/sway.pid"
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("compositor pid file %s: %v", pidFile, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("bad compositor pid %q: %v", data, err)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatalf("SIGKILL compositor (pid %d): %v", pid, err)
	}

	// The client must exit on its own — the zombie scenario this issue
	// kills is a client that parks forever on a dead fd.
	select {
	case <-c.Exited():
	case <-time.After(killTimeout):
		t.Fatal("showcase outlived the compositor: no disconnect policy ran")
	}
	waitErr := c.Wait()
	var ee *exec.ExitError
	if !errors.As(waitErr, &ee) {
		t.Fatalf("client wait = %v, want an exit status", waitErr)
	}
	if code := ee.ExitCode(); code != app.DisconnectExitCode {
		tail := w.Tail(25)
		lines := make([]string, len(tail))
		for i, tr := range tail {
			lines[i] = tr.String()
		}
		t.Fatalf("client exit code = %d, want %d (signal death or crash, not the clean-exit policy); log tail:\n\t%s",
			code, app.DisconnectExitCode, strings.Join(lines, "\n\t"))
	}

	// The policy ran exactly once, and nothing panicked on the way out.
	if _, err := w.Wait("wire", "app: compositor disconnected", traceTimeout); err != nil {
		t.Errorf("disconnect trace missing: %v", err)
	}
	raw, err := os.ReadFile(c.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(raw), "app: compositor disconnected"); got != 1 {
		t.Errorf("disconnect policy traced %d times, want exactly once", got)
	}
	if strings.Contains(string(raw), "panic") {
		tail := w.Tail(25)
		lines := make([]string, len(tail))
		for i, tr := range tail {
			lines[i] = tr.String()
		}
		t.Errorf("client log contains a panic:\n\t%s", strings.Join(lines, "\n\t"))
	}
}
