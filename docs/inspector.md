# The debug inspector

gelm's counterpart of the GTK inspector: a widget-tree overlay over
the live UI, a text dump of the tree for bug reports, and a doctor
block that snapshots the session's environment. Everything here is
read-only observation; the inspector never mutates widget state (a
test pins the overlay's paint as side-effect free).

## Prod visibility: always compiled in, inert until asked for

The inspector is deliberately NOT behind the `gelmdebug` build tag.
Its whole purpose is diagnosing layout bugs in the field, where
tagged builds do not exist. The cost when it is off is one
passthrough call per frame operation (measure, arrange, paint, hit
test) and one bool check per key press — nothing traces, draws, or
allocates. Real trace instrumentation stays behind the tag
(`internal/debug`, `GOELM_DEBUG`), so prod binaries carry no trace
output.

The tree dump and doctor block are plain function calls
(`widget.DumpTree`, `inspect.Doctor`) and always available.

## Turning it on

Two equivalent paths:

- `GELM_INSPECT=1` (or `true`/`yes`/`on`) in the environment arms the
  inspector for any gelm app at startup, overlay on.
- In code: `Application.SetInspect(true)` / `ToggleInspect()`, or
  `app.Config{Inspect: true}` for the single-window `app.Run`
  convenience.

## The overlay

While armed, every window's tree is painted with:

- each widget's bounds, outlined, with an alternating blue/orange
  tint per nesting depth;
- a label chip per widget: its debug name (`SetDebugName`), else its
  type (`Button`, `Label`), with `[F]`/`[H]`/`[P]` markers for the
  focus/hover/press targets;
- the hovered subtree (the router's hover target plus its ancestors)
  washed stronger;
- the router's hover target outlined in magenta — distinct from any
  widget-level hover styling;
- the focus target outlined in green, matching the regular focus
  ring the window already draws.

The overlay is a passthrough widget wrapped around the tree: layout,
hit testing, and damage are unchanged whether it is on or off.
Toggling queues a full-window repaint, since annotations own no
widget pixels.

## Keybindings

While the inspector is armed:

- `ctrl+shift+i` toggles the overlay;
- `ctrl+shift+d` dumps the tree to stdout.

The GTK muscle memory is deliberate. The chords fire before widget
routing and accelerators, but only while armed — an app that never
arms the inspector keeps both combos to itself.

## The tree dump

`inspect.Dump(root, router)` (or `widget.DumpTree` directly) renders
one indented line per widget in paint order:

    gelm tree dump: 5 widgets
    *widget.Box bounds=0,0+400x300
      *widget.Label bounds=4,4+392x16 role=label tooltip="the page title"
      *widget.Button name="toolbar:save" bounds=4,28+392x36 role=button tooltip="saves the thing" focusable hover pressed
      *widget.Entry bounds=4,72+392x27 role=entry focusable
      *widget.Slider name="zoom" bounds=4,107+392x18 role=slider focusable focused

The format is stable — `widget/testdata/dump.golden` pins it byte for
byte — so a dump pasted into an issue means the same thing to
everyone. Fields: type, `name=` (debug name, when set), `bounds=`
(X,Y+WxH in surface pixels), `role=` (when not none), `tooltip=`
(when set), then the flags `focusable` (Tab-reachable), `focused`,
`hover`, and `pressed` (this widget IS the router's target). Flags
are absent without a router.

Debug names: `w.SetDebugName("toolbar:save")` on any widget. Names
are plain data — text and state updates never clear them — and show
up in the dump and the overlay labels.

## The doctor block

`inspect.Doctor(sess, inspect.DoctorOptions{...})` renders the
diagnostics block a bug report carries verbatim. Apps print it at
startup when `GELM_DOCTOR=1` is set (the app package does this in
`NewApplication`):

    gelm doctor
    go: go1.27.1 (linux/amd64)
    globals: wl_compositor v4, wl_output v2, wl_seat v7, wl_shm v1
    outputs: DP-1 mode 2560x1440 scale 2 logical 0,0+1280x720
    cursor theme: Bibata-Modern-Ice 24px
    fonts: sans="DejaVu Sans" mono="JetBrains Mono" fallback=[Noto Sans CJK JP, Noto Color Emoji]
    buffer: ARGB8888 premultiplied (wl_shm format 0x0)
    fractional scale: yes (wp_viewporter + wp_fractional_scale_v1)

- `globals` — every interface the registry advertised, with the
  version it was advertised at (`Session.Globals`).
- `outputs` — xdg-output name, current mode, integer scale, and the
  logical geometry the fractional scale applies to.
- `cursor theme` — the xcursor theme that loaded, or why it could
  not.
- `fonts` — the faces `sysfont.Sans`/`Monospace` resolved (override
  with `DoctorOptions.Sans`/`Mono`) plus the installed standard
  fallback families.
- `buffer` — the one shm pixel format gelm submits
  (`buffer.FormatName`).
- `fractional scale` — whether surfaces scale fractionally or fall
  back to integer `set_buffer_scale`.

The session snapshot itself is `Session.Inspect()` (type
`wlsession.InspectInfo`), so tools can consume the values without
parsing the block.

## Tests

- `widget/debug_test.go` — golden dump (exact bytes against
  `testdata/dump.golden`), router-free dumps carry no input flags,
  live snapshot agrees with the a11y snapshot, `SetDebugName`
  survives `SetText`/entry edits, flag correctness.
- `internal/inspect/inspect_test.go` — overlay passthrough
  (measure/arrange/hit test/children), overlay paint does not mutate
  widget state and issues no invalidation, annotations carry names
  and states, label-face failure is safe, `GELM_INSPECT` parsing,
  doctor lines against a fake-session snapshot, banner format.
- `internal/wlsession/inspect_test.go` — globals+versions recording
  and removal, snapshot fields on a fake session, cursor theme
  reporting.
- `app/inspect_test.go` — chord mapping, window toggle behavior
  (overlay flips, dirty set, full-repaint rect queued), overlay
  wrapping transparency.
