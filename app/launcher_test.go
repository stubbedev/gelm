package app

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// launchRecorder records transport calls race-safely.
type launchRecorder struct {
	mu        sync.Mutex
	calls     []launchCall
	portalErr error
}

type launchCall struct {
	uri    string
	parent string
	token  string
}

func (r *launchRecorder) portal() func(uri, parent, token string) error {
	return func(uri, parent, token string) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.calls = append(r.calls, launchCall{uri, parent, token})
		return r.portalErr
	}
}

func (r *launchRecorder) external() func(uri string) error {
	return func(uri string) error {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.calls = append(r.calls, launchCall{uri: uri, token: "external"})
		return nil
	}
}

func (r *launchRecorder) snapshot() []launchCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]launchCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// waitFor polls until cond turns true or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// TestOpenURLPortalFirst pins the transport order: without activation
// support the open runs immediately through the portal; a failing
// portal falls back to the external opener exactly once.
func TestOpenURLPortalFirst(t *testing.T) {
	a := testApp(nil) // no session: no activation protocol
	rec := &launchRecorder{}
	a.launchState.openPortal = rec.portal()
	a.launchState.openExternal = rec.external()

	a.OpenURL("https://example.com")
	waitFor(t, "the portal call", func() bool { return len(rec.snapshot()) == 1 })
	if got := rec.snapshot()[0]; got.uri != "https://example.com" || got.token != "" {
		t.Errorf("portal call = %+v", got)
	}

	// A failing portal falls back.
	rec.calls, rec.portalErr = nil, errors.New("portal down")
	a.OpenURL("https://fallback.example")
	waitFor(t, "the fallback", func() bool {
		for _, c := range rec.snapshot() {
			if c.token == "external" {
				return true
			}
		}
		return false
	})
	if got := rec.snapshot(); len(got) != 2 || got[1].uri != "https://fallback.example" {
		t.Errorf("fallback calls = %+v", got)
	}
}

// TestOpenPathEncodesFileURI pins the FileLauncher shape: a path
// launches as its file:// URI.
func TestOpenPathEncodesFileURI(t *testing.T) {
	a := testApp(nil)
	rec := &launchRecorder{}
	a.launchState.openPortal = rec.portal()
	a.OpenPath("/tmp/report.pdf")
	waitFor(t, "the portal call", func() bool { return len(rec.snapshot()) == 1 })
	if got := rec.snapshot()[0].uri; got != "file:///tmp/report.pdf" {
		t.Errorf("opened %q, want the file URI", got)
	}
}

// TestOpenURLActivationRidesTheToken pins the focus plumbing: a queued
// open waits for its activation token and carries it into the portal
// call.
func TestOpenURLActivationRidesTheToken(t *testing.T) {
	a := testApp(nil)
	rec := &launchRecorder{}
	a.launchState.openPortal = rec.portal()
	a.launch("https://tok.example")
	waitFor(t, "the immediate open", func() bool { return len(rec.snapshot()) == 1 })

	// Queue one and hand the launcher the token the way the session's
	// callback would.
	a.launchState.pending = append(a.launchState.pending, "https://queued.example")
	a.launchOldestWith("tok-1")
	waitFor(t, "the queued open", func() bool { return len(rec.snapshot()) == 2 })
	if got := rec.snapshot()[1]; got.token != "tok-1" || got.uri != "https://queued.example" {
		t.Errorf("queued open = %+v, want the token riding along", got)
	}
}

// TestLauncherBurstDoesNotQueueForever pins the burst rule: beyond the
// bound, the oldest pending opens without waiting for a token.
func TestLauncherBurstDoesNotQueueForever(t *testing.T) {
	a := testApp(nil)
	rec := &launchRecorder{}
	a.launchState.openPortal = rec.portal()
	for range 6 {
		a.launch("https://burst.example")
	}
	waitFor(t, "the oldest burst open", func() bool { return len(rec.snapshot()) >= 1 })
	if got := rec.snapshot()[0]; got.uri != "https://burst.example" {
		t.Errorf("burst opened %+v first", got)
	}
}
