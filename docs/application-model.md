# gelm application model

How gelm composes multiple windows in one process (stubbedev/gelm#3),
with the relm4 and gtk4-layer-shell concepts it borrows and the ones it
deliberately drops.

## Pieces

- `app.Application` — one per process. Owns the parked event loop, the
  key repeater, and the window list. `Quit` ends the loop; `Run`
  returns `ErrClosed` when quitting or when the last window closed.
- `app.Window` — a toplevel (`xdg_toplevel`) declared through
  `WindowConfig` and driven by the application. `SetCloseRequest`
  vetoes the compositor's close request (unsaved-changes prompts);
  client-side `Close` always wins.
- `app.LayerWindow` — a layer surface (`zwlr_layer_surface_v1`)
  declared through `LayerConfig`: layer, anchors, margins, exclusive
  zone, keyboard interactivity, namespace. No close request exists in
  the protocol; the surface dies when the compositor closes it or its
  output disappears.
- `app.Run(Config)` — the single-window convenience, implemented on the
  same loop and hostWindow machinery as Application.

## Compositor disconnects

Wayland has no reconnection: when the compositor restarts (crash,
reload, upgrade), the kernel kills every client's socket and every
proxy object on the connection dies with it. gelm turns that from
"whatever a dead socket read does" into a defined story.

**Detection.** Every dispatch surface — the loop's park
(`Session.Step`), `Session.Roundtrip`, and the frame path's
attach/damage/commit — maps wire failures onto one typed error,
`*wlsession.DisconnectError` (matchable as `app.ErrDisconnected` via
`errors.Is`), carrying a classified reason:

| wire shape | reason |
| --- | --- |
| EOF on the socket read (`ErrContextRunConnectionClosed`) | `DisconnectConnectionLost` — the compositor went away |
| dead-socket errno on read or write (EPIPE, ECONNRESET, ENOTCONN, EBADF, …) | `DisconnectConnectionLost` |
| fatal `wl_display.error` recorded, or a protocol-error marker in the dispatch failure | `DisconnectProtocol` |
| anything else (timeout, decode failure with no verdict) | not classified — returned as-is; not every wire hiccup is a disconnect |

The destroyed-proxy retry inside dispatch (`ErrContextRunProxyNil`, a
normal mid-queue abort) is bounded, so no error shape — a dead fd chief
among them — can turn a roundtrip or the park into a spin.

**Policy.** `Application.OnDisconnect` (or `Config.OnDisconnect` on the
single-window `Run`) fires exactly once, on the loop goroutine, before
teardown, with `DisconnectedEvent{Reason, Err}`. After it returns, Run
releases every window (buffer pools included), closes the session — the
shared arena's memfd, mapping, and fd, and the display — and shuts the
invoke queues, so the exiting process leaves nothing mapped and no
timer or queued fn reaches for a dead connection. Run then returns the
disconnect error.

The sanctioned behavior is a **clean exit**: `main` matches
`errors.Is(err, app.ErrDisconnected)` and exits with
`app.DisconnectExitCode` (75, EX_TEMPFAIL), which systemd
`Restart=on-failure` — or a wayle supervisor — treats as respawn-us on
the new session. The hook itself is optional observation (flush state
before the teardown, never block indefinitely); without it the same
clean exit runs.

**Reconnect (design sketch, deliberately not built).** A
reconnect-with-rebuild mode would reuse the same policy hook point and
go roughly like this:

1. In `OnDisconnect`, request reconnect instead of exit. The teardown
   above still runs — on a dead connection there is nothing to keep:
   every proxy (surfaces, seat, clipboard, outputs) is dead regardless.
2. `wlsession.Connect` a fresh session (the registry re-runs inside
   it), then rebuild every window from its declarative config:
   `WindowConfig`/`LayerConfig` are already values, so the application
   keeps a factory per window and re-runs `newWindow`/`NewLayer` on the
   new session. Widget trees, application state, `Invoke`/`Every`
   queues, accelerators, and keymaps survive — they were never
   wire-owned. The one-way state is compositor-side: clipboard contents,
   drag sessions, and pending configure serials are gone; focus is the
   new compositor's to give.
3. Output identity (`xdg_output` names) re-resolves on the new registry
   before layer surfaces pin themselves, so a panel lands on the same
   `DP-1` it came from.

The sketch is honest but unproven: it needs the window factory
plumbing (step 2), a buffer arena re-seeded per session, and tests for
every protocol re-binding. Clean exit plus a supervisor covers the
restart story with the machinery that already exists, so reconnect
stays a stretch goal until an embedder needs state-preserving restarts
wayle cannot get by respawning.

**Testing.** Unit: dead-socket classification, the bounded retry, the
exactly-once hook, the pool/queue/fd teardown, and the goroutine budget
(app and wlsession packages). Live: `internal/headlesstest`'s kill test
boots the headless session, runs the showcase, SIGKILLs sway mid-run,
and asserts one policy trace and exit code 75.

## Loop sharing

All windows live on one connection and one parked loop (see
docs/input-model.md). Each window carries its own dirty flag, buffer
pool, frame callback, and tooltip state machine; input routes by
surface, keyboard routes to the window holding the seat's keyboard
focus. A window that closes is reaped and its `OnClosed` hook runs.

## Multi-output

The session tracks `wl_output` globals in arrival order
(`Session.Outputs`) and exposes hotplug hooks:

- `Session.OnOutputAdded` — fires for outputs that appear after the
  hooks are installed; enumerate `Session.Outputs()` at startup for the
  initial set.
- `Session.OnOutputRemoved` — fires when an output's global goes away.
  A layer surface pinned to that output receives the layer-shell closed
  event, so its `OnClosed` is the spawn-cleanup path.

Per-output scale comes from the output's integer scale; `LayerConfig`
derives it when `Scale` is zero. With the fractional-scale protocols
(`wp_viewporter` plus `wp_fractional_scale_manager_v1`) the compositor's
`preferred_scale` — 1.25 and friends included — overrides the integer
scale live: buffers resize in place, no window recreation. Compositors
without the protocols keep the integer behavior.

## Single instance and remote activation

`app.ClaimInstance` (#79) is the GApplication single-instance guard:
the first process to claim an AppID becomes the primary and listens on
a unix socket under `XDG_RUNTIME_DIR`; every later process forwards
its invocation - argv, `--open` paths, working directory, activation
token - and exits with status 0 without ever connecting the session.

```go
ins, primary, err := app.ClaimInstance(app.InstanceConfig{
    AppID: "dev.stubbe.gelm.messages",
    OnCommandLine: func(args []string, cwd string) { ... },
    OnOpen:       func(paths []string, cwd string) { ... },
}, app.OSInvocation(os.Args[1:]))
if err != nil { return err }
if !primary { return nil } // forwarded to the primary; exit 0

defer ins.Close()
application := app.NewApplication(sess)
ins.Bind(application) // hooks run on the loop; buffered forwards flush
```

The hooks fire on the loop goroutine with event-callback guarantees;
invocations that arrive between the claim and `Bind` buffer and flush
through the first pump. An invocation carrying open paths fires
`OnOpen` alone (GApplication's open replacing command-line); a token
the launcher exported as `XDG_ACTIVATION_TOKEN` is spent after the
hooks to focus the application. A crashed primary leaves a stale
socket; the next claim dials it, gets no answer, and reclaims it.
`AllowMultipleInstances` opts out entirely (relm4's
`allow_multiple_instances`). The claim races safely between two
simultaneous launches: the socket bind is atomic, the loser forwards.

The worked example is `cmd/gelm-messages`: run it twice and the second
run toasts in the first's windows and exits. The compositor-in-the-loop
test is `TestSingleInstanceForwardsToPrimary`
(internal/headlesstest): two real processes, one wins, the loser's
`--open` lands in the winner's shell trace.

## File dialogs

`app.OpenFileDialog`, `OpenFilesDialog`, `OpenFolderDialog`, and
`SaveFileDialog` (#82) wrap `widget.FileChooser` - the pure-Go picker
- in the standard Dialog: a places row (Up, Home, Filesystem, and
Recent when recents are on), the directory listing, pattern filters,
a name row in save mode, and an ok button that refuses to close the
dialog until the choice is valid (the new `DialogConfig.
ValidateResponse` veto). Save asks before replacing an existing file;
choices land in `OnOpen`/`OnSave` on the loop goroutine, exactly once.

**The picker is portal-free by decision.** gelm ships every widget the
picker needs, a portal FileChooser adds a D-Bus dependency and a
second UI to keep honest, and portal-less sessions (lab machines,
early boot, tests) get the same dialog. The seam is
`FileChooser.SetSource` - the listing comes from an injected
`DirSource`, so a future xdg-desktop-portal backend plugs in under the
same dialog API without touching the picker's UI. Recents are the
XDG `recently-used.xbel` list (`internal/recentfiles`), shared with
the desktop's other citizens, bounded and written atomically; the
`text/uri-list` parsing the clipboard and drag-and-drop will need
lives next to it (`recentfiles.ParseURIList`).

## Window chrome

`widget.HeaderBar` is the CSD title bar - title/subtitle, start/end
packs, `WindowControls` (close/minimize/maximize buttons, shown when
asked and hooked by the app), `widget.MenuBar` the in-window primary
navigation opening the existing menu popovers, `widget.ActionBar` the
bottom bar (#91). A press on the bar's background starts the
compositor's `xdg_toplevel.move` grab (the `WindowMover` contract any
widget can implement), a double press maximizes, and
`app.AttachHeader(win, bar)` wires controls and double-press to the
window in one call - `Window.SetTitle`, `Minimize`, and
`ToggleMaximize` are the runtime setters the controls fire into.

**Decoration policy.** xdg-decoration stays as it is: a compositor
offering server-side decoration gets asked for it. A HeaderBar in the
tree is the app's explicit opt into client-side chrome - gelm windows
ship undecorated otherwise, the GTK4 default inverted. The two can
coexist (a compositor frame around an app-drawn header); the app is
always right about what it draws itself, the compositor about what it
draws around it. Minimize and maximize buttons are hints a compositor
is free to ignore - a shown-and-ignored button is honest degradation,
a hidden one is undiscoverable.

## Concept mapping

| relm4 / GTK | gelm | note |
| --- | --- | --- |
| `relm4::Application` | `app.Application` | no `init`/`shutdown` split; construct, hook, `Run` |
| `relm4::ApplicationWindow` | `app.Window` | declarative config instead of builder chains |
| gtk4-layer-shell namespace/anchor/margin/exclusive | `LayerConfig` fields | same wire concepts, no layer-shell-in-window trickery |
| `gtk::Window::close-request` | `Window.SetCloseRequest` | veto by returning false |
| `relm4::Component` | callbacks + `app.Invoke` | the Elm actor split is unneeded — see docs/threading.md for the mapping and the Component-shaped pattern on top of Invoke |
| `relm4::Worker` / `Command` | a plain goroutine + `app.Invoke` | background work computes, then crosses onto the loop through Invoke; periodic work is `app.Every` on the loop's timer wakes (docs/threading.md) |
| `relm4::Factory` | `widget.List[W]` + `ListModel[W]` | model-driven, virtualized rows with `OnSelect`/`OnActivate`; `Changed()` re-queries (docs/threading.md) |
| `gtk::Application::quit` | `Application.Quit` | authoritative, bypasses vetoes |
| GApplication single-instance / `command-line` / `open` | `app.ClaimInstance` + `InstanceConfig` hooks | socket-keyed guard; secondaries forward and exit 0, never touching the session (see above) |
| gtk4 CSD: `HeaderBar`/`WindowControls`/`ActionBar`/`PopoverMenuBar` | `widget.HeaderBar`/`MenuBar`/`ActionBar` + `app.AttachHeader`/`AttachMenuBar` | move grab + double-click maximize through the WindowMover contract; decoration policy above |
| relm4 `binding` module (`StringBinding`, `ConnectBindingExt`) | `widget.Binding[T]` + the widget `Bind*` connectors | loop-owned observable with equal-suppressed Set and deferred write-while-notifying; two-way wiring is echo-free by construction |
| relm4/macros `open_dialog` / `save_dialog` / `open_button`, GTK `FileDialog` | `app.OpenFileDialog` family over `widget.FileChooser` | pure-Go picker (see File dialogs above), recents in XDG recently-used.xbel, overwrite confirmation, validating ok button |
| GTK `UriLauncher`/`FileLauncher`, `g_app_info_launch_default_for_uri` | `app.OpenURL` / `app.OpenPath`, `OpenURLFromLink` for `OnLinkClick` | xdg-desktop-portal OpenURI with an activation token from the session (focus-correct launch), xdg-open fallback |
| per-window `scale-factor` | per-window `Scale` plus live `preferred_scale` | fractional scaling lands with #14 |

## What is deliberately absent

- No per-window goroutines: one loop, one goroutine, ordered requests
  (a protocol requirement, not a taste choice).
- No window manager: the application does not cascade or tile; that is
  the compositor's job on Wayland.
- No implicit single-window shortcut for layer surfaces: bars are
  windows like any other, one per output.
