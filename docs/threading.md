# gelm threading model

Who may touch a widget, when, and how everything else gets in
(stubbedev/gelm#27). gelm's answer to relm4's Component/Worker/Command
machinery is deliberately smaller: **callbacks + `Invoke` is the
model** — the second half of this document maps relm4's concepts onto
it and says where the mapping earns its keep and where it does not.

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
| `Component` (model + `update(msg)` + `view`) | none — retained widgets mutated directly by callbacks; the "message" is an `Invoke` closure | relm4 needs the actor split because Rust widgets are `!Send` and data must cross threads as typed messages before the main thread reconciles them. Go closures over ordinary structs are already a safe cross-thread message: the closure body *is* `update`, running on the loop. Given up: no enforced view/state separation and no automatic widget diffing — apps that want a `state` + `sync()` shape roll it in plain code (see below). |
| `ComponentSender::send(msg)` from any thread | `app.Invoke(fn)` from any goroutine | Same contract, untyped. relm4 panics if the component is gone; gelm drops the closure after `Run` returns. |
| `Worker` (background actor, input/output channels) | a plain goroutine; results `Invoke` back | Go already has the thing Worker abstracts. A Worker's typed `Input` becomes a channel *into* the goroutine; its `Output` becomes an `Invoke`. |
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
If wayle ever outgrows this, a typed `Component[Msg]` helper is a
straightforward addition on top of `Invoke` — deliberately not
built yet, so the need has to prove itself first. `Worker`-style
patterns are likewise app-level: a goroutine with an input channel
whose loop `Invoke`s results (cmd/gelm-invoke is the 20-line version).

## Acceptance

- `cmd/gelm-invoke` updates a label twice a second from a goroutine;
  with nothing else pending the loop parks between ticks.
- The wake budget is pinned by tests (app/invoke_test.go): a 100-deep
  invoke storm arms exactly one wake; one `Every` deadline is exactly
  one wake per tick over a simulated four seconds and zero after
  cancel — on a fake clock, deterministically.
- `TestInvokeRunsOnLoopGoroutine` (app) and
  `TestOffLoopMutationTripsHook` (widget) prove the identity half:
  invoke-routed mutations are silent on the loop goroutine, direct
  ones from anywhere else trip the hook.
- `go test -race ./...` is clean, including the four-producer invoke
  storm driving un-atomic loop-owned state.
