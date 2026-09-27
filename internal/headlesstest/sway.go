// sway IPC access for the compositor-in-the-loop tests: some scenarios
// need the COMPOSITOR to act on a client - a close request delivered
// while the window is fullscreen, say - and the synthetic seat cannot
// express that. The recipe boots sway with its IPC socket in the
// session's private runtime dir; this file speaks the small i3/sway
// wire protocol (magic, length, type, payload) so tests run swaymsg
// commands against the live compositor.
package headlesstest

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"time"
)

// ipcMagic fronts every i3/sway IPC message and reply.
const ipcMagic = "i3-ipc"

// ipcRunCommand is the i3/sway IPC message type that executes a sway
// command string.
const ipcRunCommand = 0

// ipcTimeout bounds one IPC roundtrip; the compositor answers in
// microseconds, so anything past this is a broken session.
const ipcTimeout = 5 * time.Second

// SwayCommand runs one sway command (swaymsg's language) against the
// session's compositor and fails when the command itself failed (the
// reply carries one JSON object per command, each with a success flag
// and an error string).
func (e *Env) SwayCommand(command string) error {
	sock, err := e.ipcSocket()
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", sock, ipcTimeout)
	if err != nil {
		return fmt.Errorf("headlesstest: sway IPC %s: %w", sock, err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(ipcTimeout))

	head := make([]byte, 0, 14+len(command))
	head = append(head, ipcMagic...)
	var sizes [8]byte
	binary.LittleEndian.PutUint32(sizes[0:4], uint32(len(command)))
	binary.LittleEndian.PutUint32(sizes[4:8], ipcRunCommand)
	head = append(head, sizes[:]...)
	head = append(head, command...)
	if _, err := conn.Write(head); err != nil {
		return fmt.Errorf("headlesstest: sway IPC write: %w", err)
	}

	reply := make([]byte, 14)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return fmt.Errorf("headlesstest: sway IPC reply header: %w", err)
	}
	if string(reply[0:6]) != ipcMagic {
		return fmt.Errorf("headlesstest: sway IPC reply magic %q", reply[0:6])
	}
	payload := make([]byte, binary.LittleEndian.Uint32(reply[6:10]))
	if _, err := io.ReadFull(conn, payload); err != nil {
		return fmt.Errorf("headlesstest: sway IPC reply body: %w", err)
	}
	var results []struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(payload, &results); err != nil {
		return fmt.Errorf("headlesstest: sway IPC reply %s: %w", payload, err)
	}
	for _, r := range results {
		if !r.Success {
			return fmt.Errorf("headlesstest: sway rejected %q: %s", command, r.Error)
		}
	}
	return nil
}

// ipcSocket finds the sway IPC socket in the session's private runtime
// dir; sway names it sway-ipc.<uid>.<pid>.sock.
func (e *Env) ipcSocket() (string, error) {
	matches, err := filepath.Glob(filepath.Join(e.Dir, "sway-ipc.*.sock"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", errors.New("headlesstest: no sway IPC socket in " + e.Dir + "; the test-env recipe's compositor must keep its default IPC enabled")
	}
	return matches[0], nil
}
