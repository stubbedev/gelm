# Components

`package component` is gelm's counterpart of relm4's component
framework. A component is a model with typed input and output messages.
Its `Init` builds the view, its `Update` runs on the loop goroutine for
every input, and a controller owns its lifetime and forwards its
outputs.

## A component

```go
type Msg int

const (
	Increment Msg = iota
	Decrement
)

type Counter struct {
	env ui.Env
	n   int
}

func (c *Counter) Init(cx *component.Context[Msg, int]) widget.Widget {
	return ui.Mount(cx, c.env, ui.Row(
		ui.Button(ui.Label("-"), 8, 6).OnClick(func() { cx.Input(Decrement) }),
		ui.Expand(ui.Label("").WatchText(func() string { return strconv.Itoa(c.n) })),
		ui.Button(ui.Label("+"), 8, 6).OnClick(func() { cx.Input(Increment) }),
	).Spacing(6))
}

func (c *Counter) Update(cx *component.Context[Msg, int], msg Msg) {
	switch msg {
	case Increment:
		c.n++
	case Decrement:
		c.n--
	}
	cx.Output(c.n)
}
```

The model is the struct. Dependencies such as the `ui.Env` (the
typeface and text size) or the `*app.Application` are fields on it. `Init` runs once at launch and
returns the root widget. `Update` runs for every input, in send order
and never reentrantly: an input sent from inside `Update` lands on the
next loop pass.

Optional interfaces:

- `UpdateView(cx)` runs after each batch of updates (relm4's
  `update_view`).
- `Shutdown(cx)` runs when the component stops. Outputs sent from it
  are still delivered.

## Views: typed builders

`package ui` is the counterpart of relm4's `view!`: a typed builder for
every widget, generated from package widget's API (`go generate ./ui`;
a test fails when the generated file is stale).

| relm4 `view!` | `ui` |
| --- | --- |
| `gtk::Label { set_label: "x" }` | `ui.Label("x")`, then any setter without its `Set`: `.Wrap(true)` |
| `connect_clicked => Msg::Inc` | `.OnClick(func() { cx.Input(Inc) })`: every exported hook field |
| `#[watch] set_label: &...` | `.WatchText(func() string { ... })` for every setter, or `.Watch(func(*widget.Label))` |
| `#[track(cond)]` | `.Track(func(*widget.Label), deps...)` |
| `#[name = "x"]` | `.Ref(&field)` |
| child widgets | constructor and adder arguments take nodes: `ui.Button(ui.Label("+"), 8, 6)`, `.PackStart(n)`, `.AppendTab("t", n)` |
| `gtk::Box` children | `ui.Column(...)`, `ui.Row(...)` with `ui.Expand(n)` and `ui.Aligned(n, a)` |
| `if` / `match` in a view | `ui.If(cond, then, else)`, `ui.Match(key, ui.When(k, n)...)` (Stack-backed, re-checked after every update) |
| `#[iterate]` over static data | `ui.Each(seq, fn)` |
| `#[local_ref]` | `ui.Use(w)` for a widget built elsewhere, such as a child's root |
| `#[relm4::widget_template]` | an ordinary function returning a `ui.Node` |

Every builder also has the props all widgets share: `Tooltip`, `AddClass`,
`ID`, `Enabled`, `Visible` (each with a `Watch` form), `InlineStyle`,
`State`, `OnFocusChanged`, and `With(func(W))` as the escape hatch.

The `ui.Env` supplies what constructors need but the description leaves
out: the typeface, the text size and the text color (the theme's text
color when zero). `Font(face, size)` and `Ink(c)` override it per
widget.

- `ui.Mount(cx, env, node)` builds inside a component. Watches and
  tracks refresh after its updates, and bindings (`BindText`, ...)
  unbind when it shuts down.
- `ui.Build(env, node)` builds on its own and returns a `*ui.View`
  whose `Refresh` and `Close` play those roles.

Builders are descriptions. One builder placed in the tree and passed to
another, such as a `ui.Stack` given to `ui.ViewSwitcher`, builds once,
so references resolve whatever the tree order.

## Watching and tracking

- `cx.Watch(fn)` runs `fn` now and again after every batch of updates:
  relm4's `#[watch]`.
- `cx.Tracked(v)` creates a `*component.Tracked[T]` owned by the
  component: relm4's tracker. `Set` and `Update` mark it changed, and
  `Changed()` reports the mark. The component clears every tracked
  value it created after each refresh, so there is no manual
  `reset()`.
- `cx.Track(fn, deps...)` runs `fn` now, and after a batch only when
  one of `deps` changed: relm4's `#[track]`.

```go
func (c *Counter) Init(cx *component.Context[Msg, int]) widget.Widget {
	c.count = cx.Tracked(0)
	return ui.Mount(cx, c.env, ui.Label("").
		Track(func(l *widget.Label) { l.SetText(strconv.Itoa(c.count.Get())) }, c.count))
}
```

A refresh runs `UpdateView`, then the watches and tracks in
registration order, then clears the marks. A mark set by an update is
visible to every later update in the same batch.

## Launching and controllers

```go
ctrl, win, err := component.Window(application, app.WindowConfig{
	Title: "counter", AppID: "dev.example.counter",
}, &Counter{env: ui.Env{Face: face, Size: 15}})
```

`component.Window` and `component.Layer` are relm4's `RelmApp::run`:
the component becomes the window's root and shuts down when the window
closes. `component.Launch(loop, c)` starts a component without a
window, for embedding its widget yourself.

A `*Controller[In, Out]` gives you:

| method | meaning |
| --- | --- |
| `Widget()` | the root widget |
| `Send(msg)`, `Sender()` | input from any goroutine |
| `Forward(fn)` | route outputs to `fn`, on the loop |
| `ForwardTo(sender, f)` | route outputs, mapped by `f`, into another component |
| `Detach()` | survive the owner's shutdown, until the loop stops |
| `Shutdown()`, `Alive()` | stop it, and ask whether it runs |

## Children

A component launched from another component's context is its child:

```go
func (p *Parent) Init(cx *component.Context[ParentMsg, struct{}]) widget.Widget {
	child := cx.Launch(&Counter{env: p.env}).
		ForwardTo(cx.Sender(), func(n int) ParentMsg { return CounterMoved{n} })
	return ui.Mount(cx, p.env, ui.Column(ui.Label("counter"), ui.Use(child.Widget())))
}
```

`Context.Launch` and `Controller.ForwardTo` are Go 1.27 generic
methods, and the child's message types are inferred from the
component value.

Lifetime follows ownership. A parent's shutdown stops its children
first (the last launched stops first), then its own `Shutdown`, then its
`OnShutdown` hooks. Top-level components stop when the loop stops.
`Detach` moves a child's ownership to the loop.

## Commands and async components

Background work is bound to the component's lifetime:

- `cx.Oneshot(func(ctx) In)` runs on its own goroutine and delivers its
  result to `Update` (relm4's `oneshot_command`).
- `cx.Spawn(func(ctx, emit))` runs on its own goroutine, and every
  `emit` delivers a message (`spawn_command`).
- `cx.Context()` is cancelled when the component shuts down. It is the
  context both forms receive, and results after shutdown are dropped.

```go
cx.Oneshot(func(ctx context.Context) Msg {
	body, err := fetch(ctx, url)
	return Fetched{body, err}
})
```

A component that implements `Loader` initializes asynchronously
(relm4's `AsyncComponent`):

1. `Loading()` returns the widget shown first.
2. `Load(ctx)` runs on its own goroutine and may fill in the model.
3. `Init` runs on the loop once `Load` returns, and its view replaces
   the loading one.

Input sent while loading is held and delivered after `Init`. A
component shut down mid-load cancels `Load`'s context and never runs
`Init`.

## Workers

A `Worker[In, Out]` is a typed background actor (relm4's `Worker`).
Its `Update(msg, emit)` runs on the worker's own goroutine, in send
order, and every `emit` reaches the controller's forward route on the
loop.

```go
type Indexer struct{ db *Index }

func (x *Indexer) Update(path string, emit func(Indexed)) {
	emit(Indexed{path, x.db.Add(path)})
}

func (a *App) Init(cx *component.Context[Msg, struct{}]) widget.Widget {
	a.indexer = cx.LaunchWorker(&Indexer{db: a.db}).
		ForwardTo(cx.Sender(), func(r Indexed) Msg { return IndexDone{r} })
	...
}
```

`component.LaunchWorker(loop, w)` starts a top-level worker that stops
with the loop, and `cx.LaunchWorker(w)` starts one owned by the
component. The `WorkerController` has the same `Send`, `Sender`,
`Forward`, `ForwardTo`, `Detach` and `Shutdown` as a component's.

On shutdown, queued input is dropped and the update in progress
finishes. An optional `Shutdown()` then runs on the worker's goroutine.

## Messaging

- `cx.Input(msg)` and `Sender.Send(msg)` work from any goroutine. A
  burst of sends costs one loop wake and runs as one batch.
- `cx.Output(msg)` is delivered on the loop to the controller's
  forward route. Outputs with no route are dropped.
- `app.Stream[Msg]` is relm4's `MessageBroker`, and
  `app.SharedState[T]` its `SharedState`; both work alongside
  components ([threading.md](threading.md)).

## Testing

`component/componenttest.Loop` runs components without a compositor.
Launch on it, send input, and drive it with `Pass` (one loop pass) or
`Settle` (until idle). `Stop` ends it the way `Run` returning does.

```go
var loop componenttest.Loop
ctrl := component.Launch(&loop, &Counter{env: env})
ctrl.Send(Increment)
loop.Settle()
```
