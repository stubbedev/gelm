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
| per-window `scale-factor` | per-window `Scale` plus live `preferred_scale` | fractional scaling lands with #14 |

## What is deliberately absent

- No per-window goroutines: one loop, one goroutine, ordered requests
  (a protocol requirement, not a taste choice).
- No window manager: the application does not cascade or tile; that is
  the compositor's job on Wayland.
- No implicit single-window shortcut for layer surfaces: bars are
  windows like any other, one per output.
