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
	"fmt"
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

// liveCompositorProcs lists the surviving compositor-tree processes
// (sway, swaybg, any leftover holding the display), read from procfs
// cmdlines so the failure names its suspects.
func liveCompositorProcs() string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return fmt.Sprintf("procfs: %v", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() || !isDigits(e.Name()) {
			continue
		}
		cmd, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		line := strings.ReplaceAll(string(cmd), "\x00", " ")
		if strings.Contains(line, "sway") {
			names = append(names, e.Name()+"="+strings.TrimSpace(line))
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, "; ")
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// clientSocketPeers reports, for every unix socket the client holds
// open, which other processes share that same socket (its peer end).
// A read parked forever means no EOF arrived, which means some process
// still holds the far end: this names it.
func clientSocketPeers(clientPid int) string {
	descriptors, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", clientPid))
	if err != nil {
		return fmt.Sprintf("client fds: %v", err)
	}
	sockets := map[string]bool{}
	for _, d := range descriptors {
		target, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", clientPid, d.Name()))
		if err == nil && strings.HasPrefix(target, "socket:[") {
			sockets[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = true
		}
	}
	if len(sockets) == 0 {
		return "client holds no unix sockets"
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return fmt.Sprintf("procfs: %v", err)
	}
	var holders []string
	for _, e := range entries {
		pid := e.Name()
		if !e.IsDir() || !isDigits(pid) {
			continue
		}
		fds, err := os.ReadDir("/proc/" + pid + "/fd")
		if err != nil {
			continue
		}
		for _, fd := range fds {
			target, err := os.Readlink("/proc/" + pid + "/fd/" + fd.Name())
			if err != nil {
				continue
			}
			for inode := range sockets {
				if target == "socket:["+inode+"]" {
					holders = append(holders, "socket "+inode+" held by pid "+pid)
				}
			}
		}
	}
	if len(holders) == 0 {
		return "no process holds the client's sockets (peer gone, EOF owed)"
	}
	return strings.Join(holders, "; ")
}

// mainGoroutineStack extracts goroutine 1's frames from a runtime
// SIGQUIT dump: the park point of the client's loop.
func mainGoroutineStack(logPath string) string {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Sprintf("client log: %v", err)
	}
	text := string(data)
	start := strings.Index(text, "goroutine 1 ")
	if start < 0 {
		return "no goroutine 1 in dump; last 40 lines:\n" + tailFile(logPath, 40)
	}
	rest := text[start:]
	if end := strings.Index(rest, "\ngoroutine "); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

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
		// Evidence before failing: did the SIGKILL actually take sway
		// down (kill(2) with signal 0 only probes), where is the client
		// parked (SIGQUIT makes the Go runtime dump every goroutine's
		// stack into the client log), and who still holds the socket
		// ends (a blocked read means no EOF arrived, i.e. the peer fd
		// is still open in some process). The dump's tail is runtime
		// workers, so pull goroutine 1's frames out explicitly.
		swayAlive := syscall.Kill(pid, 0) == nil
		_ = c.cmd.Process.Signal(syscall.SIGQUIT)
		time.Sleep(2 * time.Second)
		t.Fatalf("showcase outlived the compositor: no disconnect policy ran; sway pid %d still alive: %v; procs: %s; client fd peers: %s; goroutine 1:\n\t%s",
			pid, swayAlive, liveCompositorProcs(), clientSocketPeers(c.cmd.Process.Pid),
			strings.ReplaceAll(mainGoroutineStack(c.LogPath), "\n", "\n\t"))
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
