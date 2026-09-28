// Compositor drivers for the harness: the suite itself is
// compositor-agnostic; the pieces that differ - how to close a client
// window through the compositor's own control channel, where the boot
// recipe records the pid, what it named the config and log - live
// behind this interface. GELM_TEST_COMPOSITOR picks the driver
// ("sway" default, "hyprland"), so the same test run works against
// either the sway gate (just headless) or the Hyprland VM gate
// (tests/hyprland-vm.nix).
package headlesstest

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// Compositor is the compositor-specific slice of the harness. Every
// method names an artifact the boot recipe owns: the recipe writes the
// config the kill pattern points at, records the pid, and redirects
// the compositor's own output to the log, so the driver only carries
// the names and the control protocol.
type Compositor interface {
	// Name is the driver's GELM_TEST_COMPOSITOR value, also the
	// substring the failure diagnostics grep procfs for.
	Name() string
	// ConfigName is the compositor config file the recipe writes into
	// the session dir ("sway.cfg"). The path survives daemonizing, so
	// it is the reliable kill pattern.
	ConfigName() string
	// PIDName is the file the recipe records the compositor pid into
	// ("sway.pid").
	PIDName() string
	// LogName is the file the recipe redirects compositor output to
	// ("sway.log").
	LogName() string
	// CloseWindow asks the compositor, through its own control
	// channel, to close the client window carrying this app_id - the
	// real xdg_toplevel.close delivery the synthetic seat cannot
	// express. dir is the session's private runtime dir.
	CloseWindow(dir, appID string) error
}

// driverFor picks the compositor driver named by GELM_TEST_COMPOSITOR;
// sway stays the default so existing recipes keep working.
func driverFor(name string) (Compositor, error) {
	switch name {
	case "", "sway":
		return swayCompositor{}, nil
	case "hyprland":
		return hyprlandCompositor{}, nil
	default:
		return nil, fmt.Errorf("headlesstest: unknown GELM_TEST_COMPOSITOR %q (want \"sway\" or \"hyprland\")", name)
	}
}

// killByConfigPattern SIGKILLs every process whose cmdline still
// carries the session's config path: a daemonizing compositor leaves
// the recorded pid stale - the parent forks the real compositor and
// exits, and the socket holder reparents to init - so the config path
// is the only trace that finds whatever shape the fork took.
func killByConfigPattern(pattern string) {
	for _, pid := range procPIDsMatching(pattern) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

// procPIDsMatching lists every live pid whose NUL-separated cmdline
// contains pattern.
func procPIDsMatching(pattern string) []int {
	var pids []int
	forEachProcPID(func(pid int, cmdline string) {
		if strings.Contains(cmdline, pattern) {
			pids = append(pids, pid)
		}
	})
	return pids
}

// forEachProcPID walks /proc and calls fn with every numeric pid and
// its NUL-separated cmdline; unreadable entries are skipped.
func forEachProcPID(fn func(pid int, cmdline string)) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !isDigits(e.Name()) {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		cmd, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		fn(pid, string(cmd))
	}
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}
