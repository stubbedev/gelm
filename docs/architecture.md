# gelm architecture

How gelm is put together, and the rules that keep it correct. The
layering first, then the invariants each layer must hold — most of them
were paid for by a real bug, and each cites where the code and its test
pins live. Companion documents: [input-model.md](input-model.md) (the
input contract), [application-model.md](application-model.md) (many
windows, one process), [threading.md](threading.md) (the goroutine
rules and `Invoke`/`Every`), [a11y.md](a11y.md), and [icons.md](icons.md).

## Layers

    compositor
      |  one wayland connection
      v
    internal/wlsession          Session: the only reader of the
      |  routes per surface     connection (Step / Roundtrip /
      |                         WakeAfter), registry bindings, focus
      v                         and grab state, keymap, outputs
    internal/window             one Host per surface:
    internal/layersurface         xdg_toplevel | zwlr_layer_surface_v1
      |
      v
    app                         Application / Run: the parked loop,
      |  per-window state       hostWindow (pool, damage, pacing),
      |                         router wiring, dialogs, popovers,
      v                         tooltips, IME, drag and drop
    widget.Router               hover, grab, click, drag, focus,
      |                         scroll, dnd bubbling, key actions
      v
    widget.*                    the retained tree: Measure / Arrange /
      |                         Paint / HitTest, damage flags
      v
    render                      Canvas (premultiplied ARGB), shaped
                                text, icons; the only device-scale
                                boundary in the stack

Each layer talks only to its neighbors: widgets never touch the wire,
the session never knows widgets exist, and everything above the session
speaks logical pixels. Demos in cmd/ compose these pieces; they contain
no loop plumbing of their own.

## The rules

### 1. One connection, one loop, one goroutine

`Session.Roundtrip` / `Session.Step` are the only readers of the
connection, and one goroutine drives the loop (internal/wlsession/
session.go, app/application.go). Wayland requests from one client must
stay ordered; per-window goroutines would need a marshalling layer to
preserve that, so gelm simply does not have them (a deliberate
non-goal, see docs/application-model.md). Timer work — key repeat,
animation deadlines, tooltip dwell, animated cursors — runs on small
goroutines that only arm `Session.WakeAfter`, which kicks the parked
read with a `wl_display.sync` and returns.

### 2. The loop parks

`app.Run` and `Application.Run` block in `Session.Step` (one blocking
dispatch) instead of polling. Wakeups come from exactly three sources:
compositor events, frame callbacks, and timer deadlines armed through
`WakeAfter` — key repeat, animation frames, tooltip dwell, `Every`
poller deadlines, and `Invoke` kicks (app/invoke.go,
docs/threading.md; one armed wake covers an entire invoke storm).
`nextWake` (app/app.go) computes the earliest pending
deadline; with no dirty state and nothing scheduled the loop schedules
no kick and holds no CPU — idle is 0%, verified against headless sway.
`loopKicker.schedule` coalesces timer kicks: one outstanding kick
covers its span (re-arming covered deadlines once queued a sync per
pass and turned an animation into a pass storm).

### 3. Redraw only damage, paced by frame callbacks

Widgets record invalidation instead of apps blindly repainting:
`node.Invalidate` / `InvalidateRect` / `InvalidateLayout` walk the
parent chain into `widget.CollectDamage` (widget/damage.go,
widget/widget.go). The frame loop clips painting to the damage union,
sends per-rect `damage_buffer`, and skips buffer acquisition and
commit entirely when nothing is dirty. `Measure` is memoized per
widget against the constraints it was asked with (widget/widget.go),
so a static tree costs one cache hit at the root; edits and resizes
drop exactly the affected branch.

Partial repaint carries an obligation: it is only correct if the
acquired buffer already matches the screen outside the damaged region.
Each pooled buffer therefore tracks `Stale` — the damage painted into
other buffers since its content was last shown — and repaints Stale ∪
frame damage; fresh buffers start fully stale (internal/buffer/pool.go,
app/window.go). `Presented` records only what actually changed on
screen, so one buffer's init paint cannot poison the pool.

### 4. Resize before Acquire; a resize commit bypasses pacing

The draw path checks size and scale before acquiring a buffer:
`Pool.Resize` destroys buffers, and doing that after Acquire destroyed
the buffer that was about to be attached — the compositor killed the
connection. Configure relayout runs `syncSize` between events
(app/window.go) and marks the frame dirty.

The resize commit bypasses frame pacing (`shouldDraw`, app/app.go):
compositors withhold a surface's frame callback until it commits at
the configured size, so a paced repaint deadlocks the resize. That is
a liveness requirement, not politeness. Min/max limits are clamped
client-side in the same pass, so a compositor configuring outside the
limits still lays out inside them.

### 5. Occlusion degrades to parking, never spinning

A fully occluded surface stops receiving frame callbacks. The loop
must not spin waiting for one: dirty input still forces the next
redraw, and a running animation takes over pacing itself — the
animation clock's timer pushes a frame only once the callback has gone
unheard for a frame and a half (`frameOwed`, app/window.go). Animation
is timer-paced for the same reason (internal/anim): `Tick` advances
eased, composable timelines (Easing / Sequence / Parallel / Cancel) on
wakes, `Next` reports the next frame deadline, and nothing running
means no deadline and a full park. Only ticks that ran callbacks mark
windows dirty.

### 6. Logical coordinates end to end; the device scale exists only at the paint boundary

The widget tree — input routing, hit tests, caret rects, popup
anchoring — lives in logical surface pixels. The device scale appears
where the hardware needs it and nowhere else (internal/scale,
render/canvas.go):

- scales are rational and 120-based (`scale.Denom` = 120; 1.25 is
  150/120), matching `wp_fractional_scale_v1`'s wire unit;
- `render.Canvas` carries the rational scale; primitives take logical
  rects and land on device pixels rounded outward, so a logical rect
  covers every device pixel it spans;
- text is shaped once at its logical size and rasterized at the device
  scale — crisp at any factor, no glyph cache to invalidate;
- `preferred_scale` rescales the window in place (same router, focus,
  and pool; buffers rebuilt, whole window queued for repaint — the
  fresh buffers' full staleness is what guarantees the full repaint);
  compositors without the fractional protocols keep the integer
  `set_buffer_scale` path.

### 7. Premultiplied alpha, and anti-aliasing must respect it

`render.Color` is premultiplied ARGB end to end; blending is
source-over, and straight alpha exists only at the PNG boundary.
Anti-aliased primitives (signed-distance RoundedRect and Line)
multiply the *coverage into all premultiplied channels*, not just
alpha — scaling alpha alone leaves full-strength source color on edge
pixels and halos around every rounded control (render/canvas.go, pinned
by TestRoundedRectEdgeBlending and TestLine).

### 8. A widget's Measure and Paint must agree

Layout and painting derive from the same sources, or the widget
measures boxes its own painter refuses to fill into. The canonical bug:
Label.Measure rounded a line height to 15 while DrawAligned's guard
demanded the exact 15.13 float — the panel's server list measured fine
and painted nothing. Both now share `ShapedText.LineHeight`
(render/text.go), pinned by TestLabelNaturalHeightPaints. When adding a
widget, compute shared geometry once and measure the thing you paint.

### 9. Buffers outlive the frames that read them; objects die with their done events

The shell never draws into a buffer the compositor holds: `Pool.Resize`
drops busy buffers from rotation but keeps the wl_buffer object and its
slot alive until the release event lands, retiring the oldest frame
beyond pool capacity instead of stacking unbounded pending buffers
(internal/buffer/pool.go). `ErrBusy` survives: no pending buffer is
ever handed out.

Storage is one arena per session: a single anonymous file backs one
`wl_shm_pool`, buffers are sub-allocations (offset + byte range), and
growth goes through truncate + `pool.resize` + remap — a session holds
exactly one fd and one mapping no matter how many windows resize how
often (internal/buffer/arena.go). `Session.Close` tears the arena down.

Done events are destructor events: frame callbacks, sync callbacks, and
loop kicks unregister their proxies after done, or a per-frame loop
leaks an object id per frame (app/app.go `frameDone`).

### 10. Input routes by surface, under an implicit grab

The session owns a focus/grab state machine and routes every pointer
event to the handler registered for its surface; surfaces never swap
global hooks (internal/wlsession, docs/input-model.md). A button press
opens a client-side implicit grab on the focus surface — drags keep
feeding the widget they started on — and a press on a resize edge is
intercepted before the router sees it, so the frame and the widget
model never fight over the same press. On the keyboard side,
`widget.KeyActionForSym` is the single keysym-to-action map; app-level
routing and popup hosts both delegate to it (widget/input.go), so an
editing key means the same thing everywhere.

### 11. A layer surface's auto axis needs both edges of that axis

zwlr_layer_surface_v1 leaves an axis to the compositor only when both
edges of that axis are anchored; requesting a zero width or height
otherwise is a protocol error. The layer surface validates the anchor
rules and the application derives the fallback size from the pinned
output's current mode (internal/layersurface, app/application.go
`LayerConfig`, cmd/gelm-bar for the shape).

### 12. Optional globals are feature-detected and never gate Connect

Every protocol beyond the required core — xdg-decoration,
fractional-scale, text-input, the data device (version-capped) — binds
optionally, reports its presence or negotiated version, and the
callers gate on it. A compositor missing any of them gets the
pre-feature behavior byte for byte; none of them fail `Connect`
(internal/wlsession). Feature work adds capabilities without adding
requirements.

## Non-goals

- **No per-window goroutines** — one loop, one goroutine, ordered
  requests (a protocol requirement, rule 1). Long work needs a
  goroutine plus app-side marshalling back onto the loop.
- **No window manager** — no cascading, tiling, or focus stealing;
  that is the compositor's job on Wayland.
- **No in-process AT-SPI** — semantic roles and a keyboard-first
  guarantee ship instead; the decision and the integration path are
  recorded in docs/a11y.md.
- **No live theme-change signal** — following the desktop setting
  would pull xsettings in as a dependency; theme switches are explicit
  (docs/icons.md).
- **No actor model or component framework** — widgets are retained
  objects with plain Go callbacks (docs/application-model.md).
