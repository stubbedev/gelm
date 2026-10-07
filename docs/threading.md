# gelm threading model

Who may touch a widget, when, and how everything else gets in
(stubbedev/gelm#27). gelm's answer to relm4's Component/Worker/Command
machinery is deliberately smaller: **callbacks + `Invoke` is the
model** — the second half of this document maps relm4's concepts onto
it and says where the mapping earns its keep and where it does not.
For apps that want the Elm discipline anyway, an optional typed layer
(`app/message.go`) now sits above `Invoke` without replacing it.

## The contract

1. **One loop goroutine owns the UI.** `Application.Run` (and the
   single-window `app.Run`) run on one goroutine. Every widget
   mutation, every event callback (`OnClick`, `OnKey`, `OnSelect`,
   `OnActivate`, dnd hooks, IME delivery, `Invoke` closures, `Every`
   ticks) runs on that goroutine. No locking exists inside gelm's
   widgets; the single-thread rule *is* the lock.
2. **Any other goroutine reaches the tree only through
   `Application.Invoke(fn)`** (app/invoke.go). `Invoke` queues the
   closure and wakes the parked loop; the loop runs `fn` at the top of
   its next pass with exactly the guarantees of an event callback.
   Reads of loop-owned state from other goroutines are off-limits too
   — hand the value into the closure instead.
3. **The compositor connection is loop-only.** Wayland requests from
   one client must stay ordered (docs/architecture.md rule 1); nothing
   outside the loop goroutine issues protocol requests. Background
   work computes, then `Invoke`s the result.
4. **Timer callbacks are loop callbacks.** `Application.Every(d, fn)`
   runs `fn` on the loop goroutine at a fixed cadence; there is no
   timer thread whose callback could race the tree.

Construction is the pragmatic exception: building a tree before `Run`
starts, on the goroutine that will call `Run`, is ordinary — nothing
else can be touching it. After `Run` returns the guard disarms and
teardown may touch widgets from any goroutine (the loop is gone).

## How `Invoke` wakes the park

The loop parks in `Session.Step`; wake sources are compositor events,
frame callbacks, and the timer deadlines `nextWake` computes
(app/app.go). `Invoke` joins the third group without a goroutine of
its own: enqueueing arms a single pending wake and issues one
`Session.WakeAfter(0)` (the same kick a new window uses). The armed
flag lives under the queue mutex, so a storm of Invokes arms **one**
wake no matter how many closures queue — the loop drains the whole
queue per pass, then parks again. Re-arming per call would queue a
sync per closure and turn a 2 Hz poller into a pass storm (the same
failure mode `loopKicker.schedule` exists to prevent for timers).

`Every` deadlines join the existing `nextWake` set (repeat, animation
frame, tooltip) as one more timestamp: a pending poller wakes the park
exactly at its deadline and costs nothing in between. Ticks anchor at
fire time — a slow frame delays the next tick instead of bursting to
catch up — and a canceled timer is reaped on the next pass. Timers
stop when `Run` returns (its deferred cleanup also drops queued
Invokes, so a worker outliving the windows cannot pile closures onto a
dead application).

The single-window `app.Run(Config)` convenience creates its
`Application` internally and hands nothing back, so goroutine bridging
needs the `Application` API (`NewApplication` + `NewWindow`/`NewLayer`
+ `Run`) — see `cmd/gelm-invoke` for the full shape.

## Detection: the off-loop hook

The contract is enforced, not just written down. `Application.Run`
marks its goroutine (`widget.MarkLoop`) and the sampled mutation entry
points — `node.Invalidate`, `node.InvalidateRect`, `node.SetTooltip`,
and through `Invalidate` every `SetText`-style mutation — check it
(`widget/checkLoop`, widget/thread.go). A mutation from any other
goroutine fires the hook installed by `widget.SetOffLoopHook`. The
guard costs one atomic load per call while disarmed and is a debugging
aid, not a lock: it samples the common choke points rather than
vouching for every field. Tests arm it deliberately; a gelmdebug build
could install a hook permanently.

## The relm4 mapping

relm4 wraps GTK in the Elm architecture: components are actors with
typed message channels, `update` mutates a model, `view` reconciles
widgets from the model, Workers are background actors, Commands are
spawned futures reporting back as messages, and Factories are
data-driven list generators that diff against their widgets. The
mapping:

| relm4 | gelm | Why it is enough (or what is given up) |
| --- | --- | --- |
| `Component` (model + `update(msg)` + `view`) | callbacks mutating widgets directly, or the optional `app.Component[Msg]` when the typed shape is wanted | relm4 needs the actor split because Rust widgets are `!Send` and data must cross threads as typed messages before the main thread reconciles them. Go closures over ordinary structs are already a safe cross-thread message: the closure body *is* `update`, running on the loop. `Component[Msg]` (#78) is that closure with a typed `Send` in front — the model is what `update` closes over, the view is whatever it mutates. Given up: no enforced view/state separation and no automatic widget diffing. |
| `ComponentSender::send(msg)` from any thread | `c.Send(msg)` / `s.Send(msg)` from any goroutine, or `app.Invoke(fn)` untyped | Same contract. The typed layer routes through `Invoke` underneath, so the wake coalescing and post-`Run` dropping come free. |
| `ComponentStream` (extra output streams) | `app.Stream[Msg]` — `Subscribe(fn)` delivers on the loop | One shape for every fan-out: a component's outputs are a `Stream` its update Sends to. |
| `MessageBroker<M>` (static pub/sub) | a `Stream[Msg]` held in a package variable | `Send` publishes, `Subscribe` registers; any nesting depth reaches the package variable without senders threaded through. |
| relm4 `SharedState<T>` / `Reducer` | `app.SharedState[T]`: `Get` snapshots, `Update` notifies subscribers with the new value on the loop | The notification stream is the same `Stream` shape; one state shared across windows cannot fall out of sync because there is no per-window copy. |
| `Worker` (background actor, input/output channels) | a plain goroutine; results `Invoke` back (or `Send` to a `Component`) | Go already has the thing Worker abstracts. A Worker's typed `Input` becomes a channel *into* the goroutine; its `Output` becomes an `Invoke` or a `Send`. |
| `Command` / `oneshot_command` / `spawn_command` | `go func() { r := work(); app.Invoke(func(){ ...r... }) }()` from a callback | Two lines instead of a trait; lifetime management is explicit (cancel via your own channel/`context`) where relm4 ties commands to component shutdown. |
| `Factory` / `FactoryVecDeque` (data-driven lists, per-item diffing) | `widget.List[W]` + `ListModel[W]` (widget/list.go) | The model-driven list is already the Factory shape: `Len`/`Row(i)` are queried by the loop on `Changed()`, only viewport rows are instantiated (virtualized — the factory's raison d'être), `OnSelect`/`OnActivate` deliver row indices as events. Given up: no per-row component identity; mutating a visible row means `Changed()` re-queries, which rebuilds row widgets rather than patching in place. |
| glib `timeout_add` / periodic updates | `app.Every(d, fn)` | Loop-driven like everything else; canceled on `Run` exit or the returned handle. |
| main-thread assertion (`MainContext.is_main_thread`) | `widget.MarkLoop` guard + `SetOffLoopHook` | Sampling hook instead of a global assertion, armed by `Run`. |

### What a Component shape looks like when an app wants it

Nothing stops an app from keeping the Elm discipline inside the
callback model — the loop is the runtime a Component would sit on:

```go
type counter struct {
	n     int
	label *widget.Label
}

func (c *counter) update(msg string) { // runs on the loop goroutine
	switch msg {
	case "tick":
		c.n++
		c.label.SetText(fmt.Sprintf("%d", c.n))
	}
}

// anywhere: c.update crosses threads as a closure
application.Invoke(func() { c.update("tick") })
```

That is Component with the framework removed: the enum is a string (or
your own type), the channel is the invoke queue, `view` is `SetText`.
For apps that want the typed discipline at more than one site, the
optional helpers below wrap exactly this shape; for one-off crossings
the plain closure remains the shorter read.

### The typed layer: `app/message.go`

The typed helpers are layered entirely above `Invoke` — no actor
framework, no second runtime. One queue shape powers them all: a
mailbox whose first append routes a single drain closure through
`Invoke`, the same armed-wake rule the invoke queue applies to closures
(app/invoke.go), so a hundred `Send`s cost one loop wake
and deliver as one ordered batch per pass.

- **`app.Component[Msg]`** — `NewComponent(a, update)` builds a typed
  update: `Send(msg)` from any goroutine runs `update(msg)` on the
  loop, in `Send` order, never reentrantly (a self-`Send` lands on the
  next pass). `Shutdown` stops it; `Detach` shields a shared component
  from a scoped owner's `Shutdown` — relm4's detach, with shutdown
  explicit instead of drop-driven.
- **`app.Stream[Msg]`** — typed publish/subscribe: `Send` from any
  goroutine, `Subscribe(fn)` on the loop, per-subscriber cancel. It is
  the one shape behind component outputs (the update Sends to a
  `Stream` it holds) and message brokers (a `Stream` in a package
  variable connects app level to any nested depth without threading
  senders).
- **`app.SharedState[T]`** — a loop-owned observable value: `Get`
  snapshots from any goroutine, `Update` mutates and notifies
  subscribers with the new value on the loop. Cross-window by
  construction: both windows subscribe to the one state.

Lifecycle is shared with the loop: everything above stops when `Run`
returns (later `Send`s drop instead of accumulating against a dead
loop), and `Close`/`Shutdown` stop a messenger early. Delivery is
deferred: a `Send` from inside a subscriber or update runs on the next
pass, which is what makes fan-out safe without locks. The worked
example is `cmd/gelm-messages`: two windows share one `SharedState`
counter, one broker `Stream` toasts both windows, and the click path
is a `Component`.

The shipped example is `internal/appearance` (#53): it watches the
desktop's dark/light preference on its own dbus goroutine and delivers
`OnChange` callbacks there — deliberately NOT pre-bridged onto the
loop. The wiring example in [appearance.md](appearance.md) is the
Worker→`Invoke` pattern above, applied: a background watcher whose
result crosses onto the loop in exactly one visible place. That doc
also records why the callback does not fire on the loop goroutine
(hiding the crossing would invert the layering) and why it does not
fire on the caller's either (the monitor outlives the registration
site).

## Acceptance

- `cmd/gelm-invoke` updates a label twice a second from a goroutine;
  with nothing else pending the loop parks between ticks.
- The wake budget is pinned by tests (app/invoke_test.go): a 100-deep
  invoke storm arms exactly one wake; one `Every` deadline is exactly
  one wake per tick over a simulated four seconds and zero after
  cancel — on a fake clock, deterministically. The typed layer rides
  the same budget (app/message_test.go): 100 `Send`s route exactly one
  invoke, and the counter example above is the first test.
- Messenger lifecycle is pinned too (app/message_test.go): `Send`
  order delivers in order, fan-out reaches every subscriber, a cancel
  takes effect at the next message, and everything the loop owns stops
  at `Run`'s exit — a producer outliving the loop cannot accumulate
  messages against it.
- `cmd/gelm-messages` shows the layer end to end: a `SharedState`
  counter shared by two windows, a broker `Stream` toasting both, a
  `Component` on the click path.
- `TestInvokeRunsOnLoopGoroutine` (app) and
  `TestOffLoopMutationTripsHook` (widget) prove the identity half:
  invoke-routed mutations are silent on the loop goroutine, direct
  ones from anywhere else trip the hook.
- `go test -race ./...` is clean, including the four-producer invoke
  storm driving un-atomic loop-owned state.
