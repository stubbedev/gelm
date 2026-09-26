# gelm input model

This document defines gelm's input semantics, modeled on GTK's event
controllers and seat grabs (see stubbedev/gelm#1). It is the contract
behind `wlsession` -> `app` -> `widget.Router`; change the code and this
document together.

## Event flow

    compositor
      |  wl_pointer / wl_keyboard / wl_data_device events
      v
    wlsession.Session            ONE dispatch path: Session.Roundtrip /
      |  routed per surface      Session.Run are the only readers of the
      v                          connection; nothing else may call them.
    wlsession.SurfacePointerHandler   (one per surface)
    wlsession.SurfaceDropHandler      (one per surface, via internal/dragdrop)
      v
    app.surfaceInput / popup.popupInput
      v
    widget.Router                hover, grab, click, drag, focus, scroll
      v
    widget callbacks (OnClick, OnChanged, InsertRune, ...)

There is a single event source: the session owns the connection and
every pointer event is routed to the handler registered for the surface
it belongs to (`Session.SetSurfaceInput`). Surfaces never swap global
hooks; nested loops (popup.Run) stay safe because routing, not hook
replacement, decides who receives an event.

## The parked loop

The app loop parks in `Session.Step` (one blocking dispatch) instead of
polling. Wakeups come from exactly three sources:

- compositor events: pointer, keyboard, frame callbacks, configure,
  buffer releases;
- timer deadlines — key repeat, animation ends, tooltip dwell — each
  armed through `Session.WakeAfter`, which kicks the parked read with a
  `wl_display.sync` from the timer goroutine;
- nothing else. With no dirty state and no pending timers the loop
  schedules no kick and holds no CPU.

Redraws happen only when dirty, paced by the frame callback. A frame
that never completes (fully occluded surface) leaves the loop waiting
for events rather than spinning: dirty input still forces the next
redraw, animation redraws wait for the next timer tick, and configure
size changes mark the frame dirty themselves.

If the connection ever needs a timeout-based wake without a timer
(single-shot waits), prefer `WakeAfter`; raw `Roundtrip` remains for
handshakes that must complete before proceeding (popup configure).

## Pointer focus and routing

- The compositor's `wl_pointer.enter` names the surface under the
  pointer; the session records it as *pointer focus*. Enter, motion,
  button, and axis events route to the focus surface's handler with
  coordinates in that surface's logical space.
- `wl_pointer.leave` clears focus and notifies that surface. Popups and
  tooltips that must not take input defend themselves with an empty
  input region (tooltips), so the compositor never routes to them.

## Implicit grab

- A button **press** opens a client-side implicit grab on the focus
  surface. While grabbed, motion, further buttons, and axis route to
  the grabbing surface even if the compositor would move focus — drags
  keep feeding the widget the drag started on.
- A **release** of the grabbing button ends the grab. Press/release
  pairs of different buttons may nest conceptually; gelm tracks a
  single grab because its widgets act on the primary button.
- This mirrors GTK: `gdk_seat_grab` semantics at the toolkit level,
  with the compositor's own popup grab (xdg_popup.grab) handling
  click-away dismissal above the client.

## Click, double-click, drag

- A click is a press and release on the same widget (`Clicker`).
- A second click within 400ms on a `DoubleClicker` is a double-click;
  widgets without double-click behavior treat each release as its own
  click.
- Dragging is motion while pressed (`DragMover`); the grab guarantees
  delivery. There is no slop threshold for widget drags (sliders must
  track from the first pixel), but the demo's chrome drag-move uses
  `xdg_toplevel.move`, whose threshold is the compositor's.
- A scroll-drag distinction therefore does not arise inside a widget:
  axis events route to the grab during a drag (scroll-follows-drag),
  and to the hover chain otherwise.

## Drag and drop (wl_data_device)

Data-device drag and drop runs beside the pointer path on the same
session, and beside the clipboard, which keeps the same device's
*selection*: offers are tracked per object, so a drag's offer never
touches the selection and vice versa.

- **Source.** A widget declares content with `widget.DragSource`:
  mimes best first plus a provider that writes the bytes for one mime
  on demand. A press on the widget plus motion past the app's
  threshold (8 logical px) starts `data_device.start_drag` with the
  press serial (the implicit grab) and a drag icon surface — a
  standalone snapshot of the source widget; failures degrade to the
  compositor's fallback icon. A drag that started cancels the click:
  the router press is dropped, so the release never fires `Clicker`.
- **Destination.** The session routes `data_device.enter/motion/
  leave/drop` to the handler registered for the named surface
  (`Session.SetSurfaceDrop`); enter names the surface, motion and
  drop broadcast to every registered handler, and only the surface
  holding the drag acts. The router bubbles from the deepest widget to
  the nearest `DragEnterer`, the same way scroll finds its handler;
  the target decides by mime — return the mime to accept, "" to
  reject — and `DragOverSetter` carries the highlight, lit only for
  accepted drags. Crossing between targets inside one surface
  retargets on hover (the compositor only sends enter on surface
  changes).
- **Serials.** `start_drag` carries the press serial;
  `data_offer.accept` carries the enter serial on every enter and
  whenever the accepted mime changes. The keyboard enter serial stays
  reserved for `set_selection`.
- **Payload.** On drop the app fetches the bytes for the accepted
  mime. A drag that originated in the same process short-circuits the
  wire: the provider hands over the bytes directly and the source
  concludes locally — no `receive`/`finish` round-trip, and no
  deadlock, since both ends share one connection. Cross-process drops
  read through the offer pipe (request, flush, read to EOF) and, on
  data-device version 3+, acknowledge with `finish`; below version 3
  there is no `dnd_finished`, so the source concludes on
  `dnd_drop_performed`.
- **Version.** The session binds `wl_data_device_manager` capped at
  version 3 and reports the negotiated version
  (`Session.DataDeviceVersion`); `set_actions`/`finish` and the dnd
  action events exist only from version 3, and the controller gates
  them accordingly (both sides declare the copy action there).

## Click-to-focus

Pressing any widget moves keyboard focus to it (`Router.Press` sets
focus from the hit test), matching GTK click-to-focus. Tab and
shift+Tab traverse focus (`FocusNext` / `FocusPrev`); shift+click
extends a text selection (`Entry` / `TextArea` handle shift+left /
shift+right and shift+click anchor).

## Hit testing and z-order

- `HitTest` returns the deepest widget whose area contains the point.
- Overlay paints all children; its hit test must return the **last
  (topmost)** child containing the point, never the first. Pinned by
  the "topmost child wins the hit test" case in TestOverlay
  (widget/widgets_test.go).
- Containers return themselves when the point is inside their padding,
  so presses on chrome within a control still belong to the control.

## Keyboard

- Keymap-driven text goes to the focused widget (`RuneHandler`);
  editing keys map to `KeyAction`s; ctrl+c/x/v/a act on the focused
  widget's selection when a clipboard is configured; Escape is an app
  keybinding (`Config.OnKey`).
- Key repeat is synthesized on the compositor's reported rate/delay.
- Tab trap: a plain Tab is indentation inside a widget that implements
  `widget.TabTrapper` (multi-line text areas); ctrl+Tab and shift+Tab
  always move focus, so a focused TextArea can contain tab characters
  while Tab-based traversal keeps working everywhere else.
- shift+click extends a text selection (`Entry` / `TextArea` handle
  shift+left / shift+right and shift+click anchor).
- Keyboard input is seat-wide (one focused window), unlike pointer
  input which routes per surface.

## Text wrapping

`TextArea` soft-wraps by default. All painting, vertical motion, click
mapping, and Home/End resolve through a visual-row cache
(`line, startCol, endCol` per painted row) built from shaped advance
widths at the offered width; Left/Right and every edit operation stay
in logical coordinates, which remain canonical for `Text()`. A row
always holds at least one rune, so an unbreakable token wider than the
viewport wraps rune-by-rune instead of vanishing, and a zero wrap
width degenerates to one rune per row.

## Input methods (zwp_text_input_v3)

When the compositor advertises `zwp_text_input_manager_v3` the session
binds one text input per seat; the global is optional, so compositors
without it keep the pre-IME keyboard path untouched — every text-input
call is a no-op.

`app.Application` drives the input method from the focused widget once
per loop iteration: focusing an editable widget (Entry, TextArea) sends
enable with the widget's surrounding text (caret and anchor as byte
offsets, composing text excluded) and caret rectangle (surface
coordinates, for the candidate window); focusing anything else sends
disable. Pushes are deduped against the last state; local changes (all
routing, not just keys) carry `set_text_change_cause: other` to ask the
input method to drop its composing state.

done events assemble the double-buffered batch and the controller
applies it to the focused widget in protocol order: IMEDelete removes
the requested bytes around the caret, IMECommit inserts the commit
string through the widget's normal Insert path, and IMEPreedit shows
the next composing text. A stale done serial still applies — committed
text is never dropped — but skips the state re-push.

While composing, the composing text is display-only: `Text()` and
`OnChanged` stay untouched until a commit lands, and surrounding text
keeps excluding it. It renders at the caret with an accent underline;
backspace trims its last rune; any other edit or reposition drops the
local display (the cause=other resync tells the input method).
Composing over a selection removes the selection, as the protocol's
done ordering prescribes. TextArea builds its visual-row cache from
the composed display, so soft wrap stays consistent while composing.

## Seat capability churn

Capabilities are state, not a one-shot: when the compositor reports a
capability lost, the session drops the matching object and any routing
state (focus, grab) that depended on it; a later gain creates a fresh
object. This keeps input alive across virtual-device cycles, tablet
mode switches, and unplug-replug (gelm#46).

## Debug traces

`internal/debug` (build tag `gelmdebug`, categories via GOELM_DEBUG)
traces the path at its seams: `wire`-level pointer events with surface
ids in `wlsession`, routing decisions and hit bounds in `app`, frame
pacing under `frame`. Without the tag every call compiles out.
