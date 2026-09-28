// Package appearance follows the desktop's dark/light preference —
// xdg-desktop-portal's org.freedesktop.portal.Settings color-scheme key,
// the one place Hyprland, GNOME, KDE and friends agree to publish it —
// over the session bus. Pure Go end to end via github.com/godbus/dbus/v5
// (no cgo, repo rule); #53 sanctions the dependency.
//
// gelm never switches themes on its own: theming is explicit
// (widget.SetTheme, docs/architecture.md "Theming"). This package only
// reports the system preference and its changes; the application wires
// the two together:
//
//	mon := appearance.New()
//	defer mon.Close()
//	mon.OnChange(func(a appearance.Appearance) {
//		if a == appearance.Unknown {
//			return
//		}
//		application.Invoke(func() {
//			if a == appearance.Dark {
//				widget.SetTheme(widget.DarkTheme())
//			} else {
//				widget.SetTheme(widget.LightTheme())
//			}
//		})
//	})
//
// See docs/appearance.md for the full walkthrough.
//
// # Threading
//
// OnChange callbacks run on the monitor's own goroutine — serialized in
// signal-arrival order, never concurrent, and never on a Wayland loop
// goroutine. That is the honest contract: the monitor is a background
// worker in the docs/threading.md sense, and the app bridges into the
// loop with Application.Invoke (the example above; the Worker/Command
// rows of the relm4 mapping). Keep handlers fast — a slow one delays
// later signals — and hand anything loop-shaped through Invoke.
// Appearance and OnChange are safe from any goroutine; the startup read
// does NOT fire OnChange (read Appearance once instead), so a listener
// registered after New never sees stale history.
//
// # Connection hygiene
//
// One connection per monitor, shared by the read and the signal
// subscription. New dials the session bus; if there is no bus at all
// (no portal desktop) the monitor is inert: Unknown, no goroutines, no
// events. If a live connection later dies (bus or portal restart) a
// dialing monitor reconnects with exponential backoff (250ms doubling
// to 8s), re-reads the key, and fires OnChange if the preference
// changed while disconnected — the reconnect re-read is an event, the
// startup read is not. A monitor built on an injected connection
// (NewOn) cannot re-dial: it fails silent, keeping the last known
// value until the owner replaces it. Failures never fabricate
// Unknown events; Unknown only comes from a real read or signal that
// says so (no preference, unknown value) or from a failed startup.
//
// Close stops everything — the goroutine, the subscription, and the
// connection when the monitor owns it — and is idempotent. A callback
// already running finishes first; Close does not wait for it. Without
// Close the monitor's goroutine outlives Run, so apps defer it.
package appearance

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/stubbedev/gelm/internal/logutil"
)

// Appearance is the system dark/light preference.
type Appearance uint8

const (
	// Unknown means no portal, no readable preference, or a value
	// outside the spec (0 "no preference" included). Apps keep
	// whatever theme they set; Unknown is a reason to do nothing.
	Unknown Appearance = iota
	// Dark means the system prefers a dark style ("prefer-dark", 1).
	Dark
	// Light means the system prefers a light style ("prefer-light", 2).
	Light
)

// String implements fmt.Stringer, for logs.
func (a Appearance) String() string {
	switch a {
	case Dark:
		return "dark"
	case Light:
		return "light"
	default:
		return "unknown"
	}
}

// The portal: a well-known name, one object, one method, one signal
// (https://flatpak.github.io/xdg-desktop-portal/). The color-scheme
// value is a variant around uint32: 0 no preference, 1 prefer-dark,
// 2 prefer-light.
const (
	portalName  = "org.freedesktop.portal.Settings"
	portalPath  = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	portalIface = "org.freedesktop.portal.Settings"

	readOne    = portalIface + ".ReadOne"
	changedSig = portalIface + ".SettingChanged"

	schemeNamespace = "org.freedesktop.appearance"
	schemeKey       = "color-scheme"

	interfaceNamespace = "org.gnome.desktop.interface"
	iconThemeKey       = "icon-theme"

	schemeDark  uint32 = 1
	schemeLight uint32 = 2

	// signalBuffer rides out two failure modes: a slow OnChange handler
	// must not wedge the bus's reader (godbus falls back to spawning a
	// delivery goroutine per signal once the buffer is full), and a
	// burst of signals queued behind one handler's Invoke must not drop
	// events. 32 is far beyond any human flipping dark/light by hand.
	signalBuffer = 32

	// probeTimeout bounds the is-the-portal-here check against the bus
	// daemon (which answers fast or is broken); readTimeout bounds the
	// portal's ReadOne. godbus's default call timeout is 25s — far too
	// long for a startup probe against a missing portal. Both apply to
	// the startup read and to each reconnect refresh, so New and the
	// reconnect loop never hang on a stalled peer.
	probeTimeout = 3 * time.Second
	readTimeout  = 5 * time.Second

	defaultRetry = 250 * time.Millisecond
	maxRetry     = 8 * time.Second
)

// dialSession is New's connection source: a private (non-shared) session
// bus connection from DBUS_SESSION_BUS_ADDRESS, authed and helloed. The
// monitor owns it and closes it, so Close really does drain godbus's
// reader/writer goroutines — the shared dbus.SessionBus singleton would
// keep them for the process lifetime.
func dialSession() (*dbus.Conn, error) {
	return dbus.ConnectSessionBus()
}

// Monitor tracks the system dark/light preference. Create one with New
// (session bus) or NewOn (an existing connection), call OnChange, and
// Close it when the application exits. All methods are safe from any
// goroutine; see the package comment for the threading and failure
// contracts.
type Monitor struct {
	// dial produces the (owned) connections for the initial read and
	// every reconnect. nil means the monitor is inert: no bus was
	// reachable at startup, so there is nothing to follow and nothing
	// to retry — Unknown, no events, no goroutines.
	dial func() (*dbus.Conn, error)
	// retry is the first reconnect backoff; it doubles up to maxRetry.
	retry time.Duration

	done chan struct{}

	mu            sync.Mutex
	closed        bool
	current       Appearance
	listeners     []func(Appearance)
	iconTheme     string
	iconListeners []func(string)
}

// New starts a monitor on the session bus. It performs the startup read
// synchronously (bounded by probeTimeout + readTimeout), so Appearance
// is valid as soon as New returns. Without a session bus the monitor is
// inert — Unknown, no events, no goroutines (see the package comment).
func New() *Monitor {
	return newMonitor(dialSession, defaultRetry)
}

// NewOn starts a monitor on a caller-provided connection — for apps
// that already hold one shared session bus connection. The monitor
// does NOT own conn: Close leaves it open, and when it dies the
// monitor fails silent (keeps the last known value; no re-dial, since
// only the owner knows how the connection was made). Otherwise the
// behavior matches New.
func NewOn(conn *dbus.Conn) *Monitor {
	return startOn(&Monitor{done: make(chan struct{}), retry: defaultRetry}, conn)
}

// newMonitor dials, performs the startup read, and starts the loop.
// The retry seam keeps the reconnect tests fast without reachable
// globals; New passes defaultRetry.
func newMonitor(dial func() (*dbus.Conn, error), retry time.Duration) *Monitor {
	m := &Monitor{dial: dial, retry: retry, done: make(chan struct{})}
	if dial == nil {
		return m
	}
	conn, err := dial()
	if err != nil {
		// No session bus: no portal desktop. Fail silent — Unknown
		// forever, no goroutines, nothing to leak (documented) — and
		// say so at Debug for applications that opted into gelm's
		// logger.
		logutil.L().Debug("appearance: no session bus; preference monitoring disabled", slog.Any("err", err))
		return m
	}
	return startOn(m, conn)
}

// startOn subscribes first, then reads: the match rule must be live
// before New returns, so a change emitted right after construction is
// queued, not lost. The startup read is the baseline, never an event:
// listeners register after New and must not see history. The reconnect
// refresh passes true instead, so a preference flipped while
// disconnected is still delivered.
func startOn(m *Monitor, conn *dbus.Conn) *Monitor {
	owned := m.dial != nil
	ch := m.subscribe(conn)
	if ch == nil {
		// The connection was already dying: fail silent like no bus.
		if owned {
			_ = conn.Close()
		}
		return m
	}
	baseline := readScheme(conn)
	m.set(baseline, false)
	m.setIconTheme(readIconTheme(conn), false)
	go m.run(conn, ch, owned)
	return m
}

// Appearance returns the current system preference: Dark, Light, or
// Unknown (no portal, no preference, unparseable value, or the monitor
// never connected). Safe from any goroutine.
func (m *Monitor) Appearance() Appearance {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

// OnChange registers fn to run with the new value whenever the system
// preference changes: on a color-scheme SettingChanged from the portal,
// or on a reconnect refresh that found a different value. Callbacks run
// on the monitor's goroutine, in signal order, one at a time — never on
// a Wayland loop goroutine. Bridge into the loop with
// Application.Invoke (package comment, docs/appearance.md). Safe from
// any goroutine; registering fn twice calls it once per change each.
func (m *Monitor) OnChange(fn func(Appearance)) {
	if fn == nil {
		return
	}
	m.mu.Lock()
	m.listeners = append(m.listeners, fn)
	m.mu.Unlock()
}

// Close stops the monitor: the loop goroutine exits, the subscription
// is removed, and a monitor-owned connection is closed (NewOn callers
// keep theirs). Idempotent, safe from any goroutine. A callback already
// running finishes first; Close does not wait for it. Appearance stays
// readable with the last known value.
func (m *Monitor) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.closed = true
	close(m.done)
}

// run is the monitor goroutine: listen on one connection until it or
// the monitor dies, then — only for connections this monitor dialed —
// reconnect, resubscribe, and refresh.
func (m *Monitor) run(conn *dbus.Conn, ch chan *dbus.Signal, owned bool) {
	for {
		if m.listen(conn, ch, owned) {
			return
		}
		// The connection died under us (bus or portal restart). An
		// injected connection cannot be replaced: fail silent, keep the
		// last known value (documented in NewOn).
		if !owned {
			logutil.L().Debug("appearance: session bus connection lost; monitoring stops, keeping last known preference")
			return
		}
		logutil.L().Debug("appearance: session bus connection lost; reconnecting")
		conn = m.redial()
		if conn == nil {
			return
		}
		ch = m.subscribe(conn)
		if ch == nil {
			return
		}
		// A reconnect is a fresh read of a possibly-changed world: the
		// refresh is an event (unlike the startup read), but only when
		// the value actually moved (set dedups).
		m.set(readScheme(conn), true)
		m.setIconTheme(readIconTheme(conn), true)
	}
}

// subscribe installs the bus match rule and the local signal channel —
// in that order, before any read, so nothing between construction and
// the loop start can be missed. nil means the connection refused the
// rule (already dying); the caller treats it as a dead connection.
func (m *Monitor) subscribe(conn *dbus.Conn) chan *dbus.Signal {
	// The match rule is what makes the daemon forward SettingChanged to
	// this connection at all; conn.Signal only registers the local
	// channel that receives what the rule lets through.
	err := conn.AddMatchSignal(
		dbus.WithMatchInterface(portalIface),
		dbus.WithMatchMember("SettingChanged"),
		dbus.WithMatchObjectPath(portalPath),
	)
	if err != nil {
		return nil
	}
	ch := make(chan *dbus.Signal, signalBuffer)
	conn.Signal(ch)
	return ch
}

// listen consumes the signal channel until the connection dies (returns
// false) or Close fires (returns true). The deferred cleanup runs
// whichever way it ends: deregister, and close the connection if this
// monitor owns it.
func (m *Monitor) listen(conn *dbus.Conn, ch chan *dbus.Signal, owned bool) (stopped bool) {
	defer func() {
		conn.RemoveSignal(ch)
		if owned {
			_ = conn.Close()
		}
	}()
	for {
		select {
		case sig, ok := <-ch:
			if !ok {
				// godbus closes registered signal channels when the
				// connection dies — our disconnect signal.
				return false
			}
			if a, matches := schemeChanged(sig); matches {
				m.set(a, true)
			}
			if name, matches := iconThemeChanged(sig); matches {
				m.setIconTheme(name, true)
			}
		case <-m.done:
			return true
		}
	}
}

// redial reconnects with exponential backoff (retry doubling to
// maxRetry) until a dial succeeds or the monitor closes.
func (m *Monitor) redial() *dbus.Conn {
	delay := m.retry
	for {
		if m.isClosed() {
			return nil
		}
		if conn, err := m.dial(); err == nil {
			return conn
		}
		select {
		case <-m.done:
			return nil
		case <-time.After(delay):
		}
		if delay < maxRetry {
			delay *= 2
		}
	}
}

// set updates the current value and, when deliver is set and something
// actually changed, runs the listeners — on the monitor goroutine, in
// registration order, without holding the lock (a listener may call
// Appearance or OnChange back). Duplicate values are dropped: the
// portal can re-announce a scheme nobody changed.
func (m *Monitor) set(a Appearance, deliver bool) {
	m.mu.Lock()
	changed := a != m.current
	m.current = a
	var fns []func(Appearance)
	if deliver && changed {
		fns = slices.Clone(m.listeners)
	}
	m.mu.Unlock()
	for _, fn := range fns {
		fn(a)
	}
}

// IconTheme returns the current icon-theme name (the portal's
// org.gnome.desktop.interface icon-theme setting): empty means no
// portal, no setting, or a value that is not a string. Safe from any
// goroutine.
func (m *Monitor) IconTheme() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.iconTheme
}

// OnIconThemeChange registers fn to run with the new icon-theme name
// whenever the desktop setting changes (#64); the returned function
// unregisters it. The same threading contract as OnChange: callbacks
// run serialized on the monitor's goroutine, never on a Wayland loop
// goroutine — bridge with Application.Invoke. The startup read is a
// baseline and never fires; an explicitly emptied setting is reported
// as "" (the consumer owns the fallback policy).
func (m *Monitor) OnIconThemeChange(fn func(string)) (off func()) {
	if fn == nil {
		return func() {}
	}
	m.mu.Lock()
	m.iconListeners = append(m.iconListeners, fn)
	i := len(m.iconListeners) - 1
	m.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			// Function values are not comparable, so the slot is the
			// identity: unregistering tombstones it and delivery skips
			// the hole.
			if i < len(m.iconListeners) {
				m.iconListeners[i] = nil
			}
		})
	}
}

// setIconTheme is set() for the icon-theme name, with the same
// deliver-on-change-only contract.
func (m *Monitor) setIconTheme(name string, deliver bool) {
	m.mu.Lock()
	changed := name != m.iconTheme
	m.iconTheme = name
	var fns []func(string)
	if deliver && changed {
		fns = slices.Clone(m.iconListeners)
	}
	m.mu.Unlock()
	for _, fn := range fns {
		if fn != nil {
			fn(name)
		}
	}
}

// isClosed reports whether Close has fired.
func (m *Monitor) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// readScheme reads the color-scheme key once: a bounded name-ownership
// probe (no portal, and deliberately no on-demand activation — a
// startup that may sit 25s on dbus activation is worse than Unknown),
// then a bounded ReadOne. Any failure reads Unknown; the value itself
// may also map to Unknown (no preference, unparseable).
func readScheme(conn *dbus.Conn) Appearance {
	body, err := call(conn.BusObject(), probeTimeout,
		"org.freedesktop.DBus.NameHasOwner", portalName)
	if err != nil || len(body) != 1 {
		return Unknown
	}
	owned, _ := body[0].(bool)
	if !owned {
		return Unknown
	}
	body, err = call(conn.Object(portalName, portalPath), readTimeout,
		readOne, schemeNamespace, schemeKey)
	if err != nil || len(body) != 1 {
		return Unknown
	}
	return schemeValue(body[0])
}

// schemeChanged inspects one signal and reports whether it is the
// portal's color-scheme SettingChanged (namespace, key, value); the
// mapped value may still be Unknown. Signals for other settings —
// gtk-theme, font settings, other namespaces — are filtered here so
// listeners never see them.
func schemeChanged(sig *dbus.Signal) (Appearance, bool) {
	if sig == nil || sig.Name != changedSig || sig.Path != portalPath {
		return Unknown, false
	}
	if len(sig.Body) != 3 {
		return Unknown, false
	}
	namespace, _ := sig.Body[0].(string)
	key, _ := sig.Body[1].(string)
	if namespace != schemeNamespace || key != schemeKey {
		return Unknown, false
	}
	return schemeValue(sig.Body[2]), true
}

// schemeValue maps a portal value — a variant around uint32 — to an
// Appearance. Anything the spec does not define — 0 "no preference",
// out-of-range numbers, wrong types — maps to Unknown rather than
// guessing a polarity.
func schemeValue(v any) Appearance {
	if variant, ok := v.(dbus.Variant); ok {
		v = variant.Value()
	}
	switch u := v.(type) {
	case uint32:
		switch u {
		case schemeDark:
			return Dark
		case schemeLight:
			return Light
		}
	}
	return Unknown
}

// readIconTheme reads the desktop's icon-theme name once (the
// org.gnome.desktop.interface setting every portal publishes): the
// same bounded ownership probe as readScheme, then a bounded ReadOne.
// Any failure — no portal, no key, a non-string value — reads empty.
func readIconTheme(conn *dbus.Conn) string {
	body, err := call(conn.BusObject(), probeTimeout,
		"org.freedesktop.DBus.NameHasOwner", portalName)
	if err != nil || len(body) != 1 {
		return ""
	}
	owned, _ := body[0].(bool)
	if !owned {
		return ""
	}
	body, err = call(conn.Object(portalName, portalPath), readTimeout,
		readOne, interfaceNamespace, iconThemeKey)
	if err != nil || len(body) != 1 {
		return ""
	}
	return stringValue(body[0])
}

// iconThemeChanged inspects one signal and reports whether it is the
// icon-theme SettingChanged, with the new name (empty for unreadable
// values). Same filtering discipline as schemeChanged: every other
// setting — color-scheme included — is not ours.
func iconThemeChanged(sig *dbus.Signal) (string, bool) {
	if sig == nil || sig.Name != changedSig || sig.Path != portalPath {
		return "", false
	}
	if len(sig.Body) != 3 {
		return "", false
	}
	namespace, _ := sig.Body[0].(string)
	key, _ := sig.Body[1].(string)
	if namespace != interfaceNamespace || key != iconThemeKey {
		return "", false
	}
	return stringValue(sig.Body[2]), true
}

// stringValue maps a portal value to its string — a variant around
// string, or anything else mapped to empty rather than guessing.
func stringValue(v any) string {
	if variant, ok := v.(dbus.Variant); ok {
		v = variant.Value()
	}
	s, _ := v.(string)
	return s
}

// call issues one method call with a client-side deadline: godbus's
// default reply timeout is 25s, and neither a startup read nor a
// reconnect refresh may hang that long on a stalled peer.
func call(obj dbus.BusObject, timeout time.Duration, method string, args ...any) ([]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c := obj.CallWithContext(ctx, method, 0, args...)
	return c.Body, c.Err
}
