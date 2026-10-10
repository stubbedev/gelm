package component

import (
	"context"

	"github.com/stubbedev/gelm/widget"
)

// Context returns a context cancelled when the component shuts down,
// for work that must not outlive it.
func (cx *Context[In, Out]) Context() context.Context { return cx.ctx }

// Oneshot runs fn on its own goroutine and delivers its result to
// Update: relm4's oneshot_command. fn's context is cancelled when the
// component shuts down, and a result arriving after that is dropped.
func (cx *Context[In, Out]) Oneshot(fn func(ctx context.Context) In) {
	go func() {
		msg := fn(cx.ctx)
		if cx.ctx.Err() == nil {
			cx.Input(msg)
		}
	}()
}

// Spawn runs fn on its own goroutine; every emit delivers a message to
// Update: relm4's spawn_command. fn's context is cancelled when the
// component shuts down, and emits after that are dropped.
func (cx *Context[In, Out]) Spawn(fn func(ctx context.Context, emit func(In))) {
	go fn(cx.ctx, func(msg In) {
		if cx.ctx.Err() == nil {
			cx.Input(msg)
		}
	})
}

// Loader is a component whose initialization needs work off the loop:
// relm4's AsyncComponent. Loading returns the widget shown meanwhile.
// Load then runs on its own goroutine and may fill in the model. Init
// runs on the loop once Load returns, and its view replaces the loading
// one. Input sent before that is held and delivered after Init. Load's
// context is cancelled when the component shuts down, and Init never
// runs for a component that shut down while loading.
type Loader[In, Out any] interface {
	Component[In, Out]
	Loading() widget.Widget
	Load(ctx context.Context)
}

func (cx *Context[In, Out]) load(l Loader[In, Out]) widget.Widget {
	loading := l.Loading()
	if loading == nil {
		panic("component: Loading returned no widget")
	}
	slot := widget.NewStack()
	slot.Add("loading", loading)
	cx.loading = true
	go func() {
		l.Load(cx.ctx)
		cx.loop.Invoke(func() { cx.loaded(slot) })
	}()
	return slot
}

func (cx *Context[In, Out]) loaded(slot *widget.Stack) {
	if cx.dead {
		return
	}
	slot.Add("ready", cx.init())
	slot.Show("ready")
	slot.Remove("loading")
	cx.loading = false
	held := cx.held
	cx.held = nil
	if len(held) > 0 {
		cx.update(held)
	}
}
