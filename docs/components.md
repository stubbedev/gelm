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
	face *render.Typeface
	n    int
}

func (c *Counter) Init(cx *component.Context[Msg, int]) widget.Widget {
	label := widget.NewLabel(c.face, 15, "", widget.Current().Text)
	cx.Watch(func() { label.SetText(strconv.Itoa(c.n)) })

	inc := widget.NewButton(widget.NewLabel(c.face, 15, "+", widget.Current().Text), 8, 6)
	inc.OnClick = func() { cx.Input(Increment) }

	box := widget.NewBox(widget.Row, 6, 6)
	box.Append(inc, false)
	box.Append(label, true)
	return box
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

The model is the struct; dependencies such as fonts or the
`*app.Application` are fields on it. `Init` runs once at launch and
returns the root widget. `Update` runs for every input, in send order
and never reentrantly: an input sent from inside `Update` lands on the
next loop pass.

Optional interfaces:

- `UpdateView(cx)` runs after each batch of updates (relm4's
  `update_view`).
- `Shutdown(cx)` runs when the component stops. Outputs sent from it
  are still delivered.

`cx.Watch(fn)` runs `fn` now and again after every batch of updates.
It is the primitive behind relm4's `#[watch]`.

## Launching and controllers

```go
ctrl, win, err := component.Window(application, app.WindowConfig{
	Title: "counter", AppID: "dev.example.counter",
}, &Counter{face: face})
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
	child := cx.Launch(&Counter{face: p.face}).
		ForwardTo(cx.Sender(), func(n int) ParentMsg { return CounterMoved{n} })
	box := widget.NewBox(widget.Column, 6, 6)
	box.Append(child.Widget(), false)
	return box
}
```

`Context.Launch` and `Controller.ForwardTo` are Go 1.27 generic
methods, and the child's message types are inferred from the
component value.

Lifetime follows ownership. A parent's shutdown stops its children
first (the last launched stops first), then its own `Shutdown`, then its
`OnShutdown` hooks. Top-level components stop when the loop stops.
`Detach` moves a child's ownership to the loop.

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
ctrl := component.Launch(&loop, &Counter{face: face})
ctrl.Send(Increment)
loop.Settle()
```
