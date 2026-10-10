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
	cx.releaseHeld()
}

func (cx *Context[In, Out]) releaseHeld() {
	held := cx.held
	cx.held = nil
	if len(held) > 0 {
		cx.update(held)
	}
}

// Await is an asynchronous update step, relm4's async update: work runs
// on its own goroutine, and the function it returns runs on the loop
// to apply the result to the model. Until then the component is Busy
// and later input waits, the rest of the current batch included, so
// no update ever sees the model halfway. The view refreshes when the
// step starts and when it lands. work's context is cancelled at
// shutdown, and a step landing after that never applies.
func (cx *Context[In, Out]) Await(work func(ctx context.Context) (apply func())) {
	if cx.busy {
		panic("component: Await while a step is in flight")
	}
	cx.busy = true
	go func() {
		apply := work(cx.ctx)
		cx.loop.Invoke(func() {
			if cx.dead {
				return
			}
			cx.busy = false
			if apply != nil {
				apply()
			}
			if len(cx.held) == 0 {
				cx.refresh()
				return
			}
			cx.releaseHeld()
		})
	}()
}

// Busy reports whether an Await step is in flight; a view can watch it
// to show progress.
func (cx *Context[In, Out]) Busy() bool { return cx.busy }
