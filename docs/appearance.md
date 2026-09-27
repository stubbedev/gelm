# System dark/light preference (appearance)

How gelm follows the desktop's dark/light preference — and why it
still never switches a theme on its own (#53). The preference lives in
xdg-desktop-portal's `org.freedesktop.portal.Settings` `color-scheme`
key, the one place Hyprland, GNOME, KDE and friends agree to publish
it. gelm reads it over the session bus in pure Go via
`github.com/godbus/dbus/v5` (no cgo, repo rule; the a11y decision in
[docs/a11y.md](a11y.md) kept godbus out for AT-SPI, but #53 sanctions
it here).

## The split: gelm offers, wayle wires

Theming in gelm is explicit — `widget.SetTheme`, a palette value
([docs/architecture.md](architecture.md), "Theming"). The toolkit has
no notion of "system mode" and no code path that flips the palette by
itself. `internal/appearance` is the observation half only: it reports
the preference and its changes, and the application decides what a
change means. That keeps gelm honest for apps with their own fixed
brand (they simply never call `SetTheme` in the callback) and makes
wayle's wiring three lines:

```go
mon := appearance.New()
defer mon.Close()

mon.OnChange(func(a appearance.Appearance) {
	if a == appearance.Unknown {
		return // no portal / no preference: keep whatever is set
	}
	// Cross from the dbus goroutine onto the loop goroutine —
	// docs/threading.md; a widget mutation from the callback itself
	// would trip the off-loop hook in debug builds.
	application.Invoke(func() {
		if a == appearance.Dark {
			widget.SetTheme(widget.DarkTheme())
		} else {
			widget.SetTheme(widget.LightTheme())
		}
	})
})
```

`Appearance()` is valid the moment `New()` returns — the startup read
is synchronous — so an app can theme its first frame from the current
preference without waiting for an event:

```go
if mon.Appearance() == appearance.Light {
	widget.SetTheme(widget.LightTheme())
}
```

## Threading contract

`OnChange` callbacks run on the **monitor's own goroutine**, serialized
in signal-arrival order, never concurrent, and never on a Wayland loop
goroutine. This is the honest contract: the monitor is a background
worker in the [docs/threading.md](threading.md) sense (the
Worker/Command rows of the relm4 mapping — a plain goroutine whose
results `Invoke` back), and bridging is the app's one explicit `Invoke`
line. The alternative — threading an `Application` into
`internal/appearance` so callbacks land pre-bridged — would invert the
dependency (a wire-adjacent package reaching into the loop layer) and
hide the crossing that the threading contract exists to make visible.

Practical notes pinned by tests
(`internal/appearance/appearance_test.go`):

- The startup read never fires `OnChange`; it is the baseline, not an
  event. A listener registered after `New` sees only changes.
- Callbacks are ordered and serialized; a burst of signals delivers
  one at a time, in signal order.
- Callbacks never run on the registering goroutine
  (`TestCallbacksRunOnMonitorGoroutine`).
- Keep handlers fast: a slow one delays later signals (the monitor
  buffers 32 signals before godbus starts shedding). Hand anything
  loop-shaped through `Invoke` as above.

## Failure model

Deliberately boring, and fail-silent where failure is expected:

- **No session bus at all** (no portal desktop): the monitor is inert —
  `Appearance()` is `Unknown` forever, no events, and no goroutines
  exist to leak.
- **No portal / key not readable**: `Unknown`, no events. The
  name-ownership probe is a fast bus call — gelm does not wait on dbus
  activation at startup, which could sit 25s on a system with a stale
  `.service` file. A portal that appears later is picked up on app
  restart, not live.
- **Bus or portal restarts mid-session**: a monitor that dialed its own
  connection (New) reconnects with exponential backoff (250ms doubling
  to 8s, capped), re-reads the key, and fires `OnChange` if the
  preference changed while disconnected. The reconnect re-read is an
  event; the startup read is not.
- **Injected connection dies** (`NewOn(conn)` — the app owns a shared
  bus connection): the monitor cannot re-dial, so it fails silent and
  keeps the last known value until the owner replaces it. Failures
  never fabricate `Unknown` events — an `Unknown` only ever reflects a
  real read or signal (no preference `0`, an unparseable value) or the
  startup fallback.
- Duplicate announcements (the portal re-sending the same scheme) are
  deduped; listeners hear actual changes.

## Connection hygiene

One connection per monitor, shared by the read and the signal
subscription, created with `dbus.ConnectSessionBus` (a private,
authed connection — not the process-wide `dbus.SessionBus` singleton,
whose goroutines would outlive `Close`). `Close()` is idempotent,
stops the loop goroutine, deregisters the match rule, and closes the
connection — a goroutine-leak test pins the drain
(`TestCloseStopsGoroutines`). Subscribe-then-read ordering means no
signal can slip through between construction and the loop's first
pass; a change emitted while the startup read is in flight is queued
and reconciled by value.

Every dbus round trip is bounded client-side (3s probe / 5s read; 8s
backoff ceiling), so a stalled portal costs an `Unknown`, never a hung
startup.

## Tests

`internal/appearance/appearance_test.go` runs against a real private
bus — a per-test `dbus-daemon` on a fresh unix socket (never the
developer's session bus) — with a scripted mock portal that claims the
well-known name, exports a fallback-filled `ReadOne`, and emits raw
`SettingChanged` signals. Covered: the startup read (prefer-dark →
Dark, prefer-light → Light), every Unknown fallback (absent key, `0`
no-preference, out-of-range, wrong type), ordered delivery of a signal
burst with dedup and foreign-setting filtering, no-portal and no-bus
inertia (no events, no goroutines), reconnect across a real bus
restart (daemon killed, new daemon on the same socket, change
delivered), shared-connection fail-silent semantics, `Close` draining
goroutines, and the monitor-goroutine callback identity. Skips cleanly
when no `dbus-daemon` binary exists.
