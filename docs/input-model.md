# gelm input model

This document defines gelm's input semantics, modeled on GTK's event
controllers and seat grabs (see stubbedev/gelm#1). It is the contract
behind `wlsession` -> `app` -> `widget.Router`; change the code and this
document together.

## Event flow

    compositor
      |  wl_pointer / wl_keyboard events
      v
    wlsession.Session            ONE dispatch path: Session.Roundtrip /
      |  routed per surface      Session.Run are the only readers of the
      v                          connection; nothing else may call them.
    wlsession.SurfacePointerHandler   (one per surface)
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
