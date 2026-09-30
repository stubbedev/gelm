package app

import (
	"errors"
	"testing"

	"github.com/stubbedev/gelm/internal/wlsession"
)

// TestLockSessionPreconditions pins the refusals that never reach the
// wire: a config without a surface builder, a compositor without the
// protocol, and a second lock while one is held.
func TestLockSessionPreconditions(t *testing.T) {
	surface := func(*Output) LockSurface { return LockSurface{} }

	a := &Application{sess: &wlsession.Session{}}
	if a.SessionLockAvailable() {
		t.Error("SessionLockAvailable = true without the manager global")
	}
	if _, err := a.LockSession(SessionLockConfig{}); err == nil {
		t.Error("a lock without a Surface builder must be refused")
	}
	if _, err := a.LockSession(SessionLockConfig{Surface: surface}); !errors.Is(err, ErrSessionLockUnavailable) {
		t.Errorf("LockSession without the protocol = %v, want ErrSessionLockUnavailable", err)
	}
	if a.SessionLock() != nil {
		t.Error("a refused lock must not be recorded as held")
	}

	held := &SessionLock{app: a}
	a.sessionLock = held
	if _, err := a.LockSession(SessionLockConfig{Surface: surface}); !errors.Is(err, ErrSessionLockActive) {
		t.Errorf("second LockSession = %v, want ErrSessionLockActive", err)
	}
	if a.SessionLock() != held {
		t.Error("the refused second lock replaced the held one")
	}
}

// TestHeldLockKeepsLoopAlive pins the loop-exit rule: with no window
// the loop ends, unless a session lock is held - the process that
// must unlock the session cannot exit because its last output went
// away. Quit still wins.
func TestHeldLockKeepsLoopAlive(t *testing.T) {
	a := &Application{}
	if !a.done() {
		t.Fatal("no windows and no lock: the loop must end")
	}
	a.sessionLock = &SessionLock{app: a}
	if a.done() {
		t.Fatal("a held lock with no surface mapped must keep the loop alive")
	}
	a.quit = true
	if !a.done() {
		t.Error("Quit must end the loop even while a lock is held")
	}
}

// TestLockHostCloser: the raw teardown of a lock host destroys its
// lock surface, like the layer and toplevel hosts' closers.
func TestLockHostCloser(t *testing.T) {
	if hostCloser(&lockHost{}) == nil {
		t.Error("lock hosts need a closer for the window teardown path")
	}
}
