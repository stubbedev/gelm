// Global shortcuts over the freedesktop GlobalShortcuts portal: the
// xdg-desktop-portal transport that works on every portal desktop,
// chosen when the compositor offers no hyprland_global_shortcuts_v1
// manager. The portal's bind flow is interactive - the desktop shows
// its shortcut-binding dialog and the user confirms - so a portal
// registration returns only after that confirmation (denial comes
// back as ErrGlobalShortcutDenied). Triggers arrive as Activated and
// Deactivated signals and dispatch onto the loop goroutine like every
// other event.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/stubbedev/gelm/internal/debug"
)

// ErrGlobalShortcutDenied reports the user refusing the portal's bind
// dialog, or the portal cancelling the request.
var ErrGlobalShortcutDenied = errors.New("app: global shortcut denied by the user")

const (
	portalShortcutsName  = "org.freedesktop.portal.Desktop"
	portalShortcutsPath  = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	portalShortcutsIface = "org.freedesktop.portal.GlobalShortcuts"

	portalCreateSession = portalShortcutsIface + ".CreateSession"
	portalBindShortcuts = portalShortcutsIface + ".BindShortcuts"

	portalResponse = "org.freedesktop.portal.Request.Response"

	portalActivated   = portalShortcutsIface + ".Activated"
	portalDeactivated = portalShortcutsIface + ".Deactivated"

	// portalRequestTimeout bounds the method call's round trip and the
	// wait for the user's bind-dialog verdict; the portal times the
	// dialog itself, but a wedged portal must not wedge the caller
	// forever.
	portalRequestTimeout = 30 * time.Second
)

// portalResponseData is one Request.Response verdict.
type portalResponseData struct {
	code    uint32
	results map[string]dbus.Variant
}

// portalShortcuts is the application's one portal shortcuts session,
// lazily created on the first portal registration.
type portalShortcuts struct {
	app  *Application
	mu   sync.Mutex
	conn *dbus.Conn
	sig  chan *dbus.Signal

	session dbus.ObjectPath
	nextReq int
	// pending binds wait on their request handle's Response, keyed by
	// the handle token the caller chose.
	pending map[string]chan portalResponseData
	// live routes Activated and Deactivated by shortcut id.
	live map[string]*GlobalShortcut
}

// portalShortcutsUp reports whether the desktop portal owns its name.
func portalShortcutsUp() bool {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()
	var owned bool
	if err := conn.Object("org.freedesktop.DBus", "/").CallWithContext(
		conn.Context(), "org.freedesktop.DBus.NameHasOwner", 0,
		portalShortcutsName).Store(&owned); err != nil {
		return false
	}
	return owned
}

// connect dials the bus once and subscribes to the portal's signals;
// idempotent.
func (p *portalShortcuts) connect() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		return nil
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return err
	}
	p.conn = conn
	p.pending = map[string]chan portalResponseData{}
	p.live = map[string]*GlobalShortcut{}
	p.sig = make(chan *dbus.Signal, 16)
	conn.Signal(p.sig)
	go p.drain()
	return nil
}

// drain dispatches signals until the connection closes.
func (p *portalShortcuts) drain() {
	for sig := range p.sig {
		p.dispatch(sig)
	}
}

// dispatch routes one portal signal: a Request.Response resolves the
// pending call its handle token names, Activated and Deactivated fire
// the live shortcut on the loop through Invoke. This is the whole
// portal state machine, signal-shaped so tests feed it directly.
func (p *portalShortcuts) dispatch(sig *dbus.Signal) {
	switch sig.Name {
	case portalResponse:
		token := pathTail(sig.Path)
		p.mu.Lock()
		ch := p.pending[token]
		delete(p.pending, token)
		p.mu.Unlock()
		if ch == nil {
			return
		}
		resp := portalResponseData{code: 1}
		if len(sig.Body) >= 1 {
			resp.code, _ = sig.Body[0].(uint32)
		}
		if len(sig.Body) >= 2 {
			resp.results, _ = sig.Body[1].(map[string]dbus.Variant)
		}
		ch <- resp
	case portalActivated, portalDeactivated:
		if len(sig.Body) < 2 {
			return
		}
		id, _ := sig.Body[1].(string)
		p.mu.Lock()
		s := p.live[id]
		p.mu.Unlock()
		if s == nil {
			return
		}
		pressed := sig.Name == portalActivated
		var sec uint64
		if len(sig.Body) >= 3 {
			if us, ok := sig.Body[2].(uint64); ok {
				sec = us / 1_000_000
			}
		}
		p.app.Invoke(func() { s.on(pressed, sec) })
	}
}

// pathTail is the segment after the last slash: the request handle's
// token, the only per-call part of .../request/SENDER/TOKEN.
func pathTail(path dbus.ObjectPath) string {
	if i := strings.LastIndexByte(string(path), '/'); i >= 0 {
		return string(path)[i+1:]
	}
	return ""
}

// call issues one portal method whose verdict arrives as a Response
// signal on the returned request handle, and waits for it. opts may
// already carry entries; the handle token is added here.
func (p *portalShortcuts) call(method string, opts map[string]dbus.Variant, args ...any) (portalResponseData, error) {
	p.mu.Lock()
	conn := p.conn
	p.nextReq++
	token := fmt.Sprintf("gelm%d", p.nextReq)
	ch := make(chan portalResponseData, 1)
	p.pending[token] = ch
	p.mu.Unlock()
	if conn == nil {
		return portalResponseData{}, ErrGlobalShortcutsUnavailable
	}
	opts["handle_token"] = dbus.MakeVariant(token)
	ctx, cancel := context.WithTimeout(conn.Context(), portalRequestTimeout)
	defer cancel()
	var handle dbus.ObjectPath
	err := conn.Object(portalShortcutsName, portalShortcutsPath).CallWithContext(
		ctx, method, 0, append(append([]any{}, args...), opts)...).Store(&handle)
	if err != nil {
		p.mu.Lock()
		delete(p.pending, token)
		p.mu.Unlock()
		return portalResponseData{}, err
	}
	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(portalRequestTimeout):
		p.mu.Lock()
		delete(p.pending, token)
		p.mu.Unlock()
		return portalResponseData{}, errors.New("app: portal shortcut request timed out")
	}
}

// createSession opens the portal session; it has no dialog.
func (p *portalShortcuts) createSession() error {
	if err := p.connect(); err != nil {
		return err
	}
	p.mu.Lock()
	p.nextReq++
	token := fmt.Sprintf("gelm-s%d", p.nextReq)
	p.mu.Unlock()
	opts := map[string]dbus.Variant{
		"handle_token":         dbus.MakeVariant(token),
		"session_handle_token": dbus.MakeVariant(token),
	}
	resp, err := p.call(portalCreateSession, opts)
	if err != nil {
		return err
	}
	if resp.code != 0 {
		return ErrGlobalShortcutDenied
	}
	var session dbus.ObjectPath
	if v, ok := resp.results["session_handle"]; ok {
		switch h := v.Value().(type) {
		case dbus.ObjectPath:
			session = h
		case string:
			session = dbus.ObjectPath(h)
		}
	}
	if session == "" {
		return errors.New("app: portal shortcut session handle missing")
	}
	p.mu.Lock()
	p.session = session
	p.mu.Unlock()
	return nil
}

// bindOne runs the interactive bind for one shortcut: the desktop
// shows its binding dialog, and the verdict comes back - denial as
// ErrGlobalShortcutDenied.
func (p *portalShortcuts) bindOne(id, description, trigger string) error {
	p.mu.Lock()
	session := p.session
	p.mu.Unlock()
	if session == "" {
		return ErrGlobalShortcutsUnavailable
	}
	shortcut := map[string]dbus.Variant{"id": dbus.MakeVariant(id)}
	if description != "" {
		shortcut["description"] = dbus.MakeVariant(description)
	}
	if trigger != "" {
		shortcut["preferred_trigger"] = dbus.MakeVariant(trigger)
	}
	resp, err := p.call(portalBindShortcuts, map[string]dbus.Variant{},
		session, []map[string]dbus.Variant{shortcut})
	if err != nil {
		return err
	}
	if resp.code != 0 {
		return ErrGlobalShortcutDenied
	}
	return nil
}

// registerPortal is the RegisterGlobalShortcut portal path: one
// session, then the interactive bind, then live.
func (p *portalShortcuts) registerPortal(id, description, trigger string, s *GlobalShortcut) error {
	p.mu.Lock()
	hasSession := p.session != ""
	p.mu.Unlock()
	if !hasSession {
		if err := p.createSession(); err != nil {
			return err
		}
	}
	if err := p.bindOne(id, description, trigger); err != nil {
		debug.Log("shell", "portal shortcut %s: %v", id, err)
		return err
	}
	p.mu.Lock()
	p.live[id] = s
	p.mu.Unlock()
	return nil
}

// shutdown closes the portal connection with the loop.
func (p *portalShortcuts) shutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		_ = p.conn.Close()
	}
	p.conn = nil
	p.live = map[string]*GlobalShortcut{}
}
