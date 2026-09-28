// The Hyprland driver for the VM gate (tests/hyprland-vm.nix):
// compositor-side actions go through Hyprland's own control socket -
// a plain request/reply channel at
// $XDG_RUNTIME_DIR/hypr/<instance>/.socket.sock - so the same kill
// by compositor command the sway tests use works here too.
package headlesstest

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// hyprCommandTimeout bounds one control-socket roundtrip; like sway
// IPC, a healthy compositor answers immediately.
const hyprCommandTimeout = 5 * time.Second

// hyprlandCompositor names the artifacts the VM recipe creates and
// speaks Hyprland's control-socket protocol.
type hyprlandCompositor struct{}

func (hyprlandCompositor) Name() string       { return "hyprland" }
func (hyprlandCompositor) ConfigName() string { return "hypr.conf" }
func (hyprlandCompositor) PIDName() string    { return "hyprland.pid" }
func (hyprlandCompositor) LogName() string    { return "hyprland.log" }

// CloseWindow dispatches closewindow for the window with the given
// app_id (Hyprland matches it against the Wayland app_id, exactly the
// class the recipes' windowrules pin).
func (hyprlandCompositor) CloseWindow(dir, appID string) error {
	sock, err := hyprControlSocket(dir)
	if err != nil {
		return err
	}
	_, err = hyprControlCommand(sock, "dispatch closewindow class:"+appID)
	return err
}

// hyprInstanceDir resolves the live Hyprland instance inside the
// session dir. HYPRLAND_INSTANCE_SIGNATURE wins when it names a live
// instance in THIS dir (the recipes export it); anything else - a
// leaked signature from the developer's own session, an unset var -
// falls back to discovery: a single instance dir with a control socket
// is the live one, and a private test session never hosts two.
func hyprInstanceDir(dir string) (string, error) {
	if sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"); sig != "" && validInstanceName(sig) {
		instance := filepath.Join(dir, "hypr", sig)
		// sig is a single path element: validInstanceName rejects
		// separators and dot segments.
		if _, err := os.Stat(filepath.Join(instance, ".socket.sock")); err == nil { //nolint:gosec // validated above, see validInstanceName
			return instance, nil
		}
	}
	matches, err := filepath.Glob(filepath.Join(dir, "hypr", "*"))
	if err != nil {
		return "", err
	}
	var live []string
	for _, m := range matches {
		if fi, err := os.Stat(filepath.Join(m, ".socket.sock")); err == nil && !fi.IsDir() {
			live = append(live, m)
		}
	}
	switch len(live) {
	case 1:
		return live[0], nil
	case 0:
		return "", errors.New("headlesstest: no live Hyprland instance in " + dir + "/hypr; set HYPRLAND_INSTANCE_SIGNATURE or boot the VM recipe's compositor")
	default:
		return "", fmt.Errorf("headlesstest: %d live Hyprland instances in %s/hypr; set HYPRLAND_INSTANCE_SIGNATURE", len(live), dir)
	}
}

// validInstanceName accepts only a single path element: instance
// signatures are <hash>_<epoch>_<rand>, and anything carrying a
// separator or a dot segment is either garbage or an attempt to walk
// out of the session dir via the environment.
func validInstanceName(sig string) bool {
	return sig != "." && sig != ".." && !strings.ContainsAny(sig, `\/`)
}

// hyprControlSocket locates the compositor's control socket for the
// session.
func hyprControlSocket(dir string) (string, error) {
	instance, err := hyprInstanceDir(dir)
	if err != nil {
		return "", err
	}
	sock := filepath.Join(instance, ".socket.sock")
	if _, err := os.Stat(sock); err != nil {
		return "", fmt.Errorf("headlesstest: Hyprland control socket: %w", err)
	}
	return sock, nil
}

// hyprControlCommand sends one request (hyprctl's language, without
// the hyprctl) and returns the reply; Hyprland answers "ok" or an
// error description.
func hyprControlCommand(sock, request string) (string, error) {
	conn, err := net.DialTimeout("unix", sock, hyprCommandTimeout)
	if err != nil {
		return "", fmt.Errorf("headlesstest: Hyprland control %s: %w", sock, err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(hyprCommandTimeout))

	if _, err := conn.Write([]byte(request)); err != nil {
		return "", fmt.Errorf("headlesstest: Hyprland control write: %w", err)
	}
	reply := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		reply = append(reply, buf[:n]...)
		if err != nil || n == 0 {
			break
		}
		if len(reply) > 1<<20 {
			return "", errors.New("headlesstest: Hyprland control reply exceeds 1MiB")
		}
	}
	return strings.TrimRight(string(reply), "\n"), nil
}
