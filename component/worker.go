package component

import (
	"sync"

	"github.com/stubbedev/gelm/internal/mailbox"
)

// Worker is a typed background actor, relm4's Worker. Update runs on
// the worker's own goroutine for every input, in send order; emit sends
// an output, delivered on the loop.
type Worker[In, Out any] interface {
	Update(msg In, emit func(Out))
}

// WorkerShutdowner is implemented by workers that release resources
// when they stop; Shutdown runs on the worker's goroutine after its
// last Update.
type WorkerShutdowner interface {
	Shutdown()
}

// WorkerController owns a launched worker: its input, where its outputs
// go, and its lifetime.
type WorkerController[In, Out any] struct {
	lifetime
	loop    Loop
	worker  Worker[In, Out]
	inputs  mailbox.Box[In]
	outputs outlet[Out]
	wake    chan struct{}
	done    chan struct{}
	stop    sync.Once
}

// LaunchWorker starts w on its own goroutine as a top-level worker,
// stopped when the loop stops.
func LaunchWorker[In, Out any](loop Loop, w Worker[In, Out]) *WorkerController[In, Out] {
	c := startWorker(loop, w)
	c.release = loop.OnStop(c.ownerShutdown)
	return c
}

// LaunchWorker starts w as a worker owned by this component: it stops
// when the component shuts down, unless detached.
func (cx *Context[In, Out]) LaunchWorker[WIn, WOut any](w Worker[WIn, WOut]) *WorkerController[WIn, WOut] {
	c := startWorker(cx.loop, w)
	cx.adopt(c)
	return c
}

func startWorker[In, Out any](loop Loop, w Worker[In, Out]) *WorkerController[In, Out] {
	c := &WorkerController[In, Out]{
		loop:    loop,
		worker:  w,
		outputs: outlet[Out]{loop: loop},
		wake:    make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	go c.run()
	return c
}

func (c *WorkerController[In, Out]) run() {
	for {
		select {
		case <-c.done:
			c.finish()
			return
		case <-c.wake:
		}
		for _, msg := range c.inputs.Take() {
			select {
			case <-c.done:
				c.finish()
				return
			default:
			}
			c.worker.Update(msg, c.outputs.send)
		}
	}
}

func (c *WorkerController[In, Out]) finish() {
	if s, ok := c.worker.(WorkerShutdowner); ok {
		s.Shutdown()
	}
}

// Send delivers msg to the worker's Update. Safe from any goroutine.
func (c *WorkerController[In, Out]) Send(msg In) {
	if c.inputs.Put(msg) {
		select {
		case c.wake <- struct{}{}:
		default:
		}
	}
}

// Sender returns a Sender for the worker's input.
func (c *WorkerController[In, Out]) Sender() Sender[In] { return Sender[In]{send: c.Send} }

// Forward routes every output to fn on the loop goroutine, replacing
// any earlier route. Outputs with no route are dropped.
func (c *WorkerController[In, Out]) Forward(fn func(Out)) *WorkerController[In, Out] {
	c.outputs.forward = fn
	return c
}

// ForwardTo routes every output, mapped by f, to s.
func (c *WorkerController[In, Out]) ForwardTo[T any](s Sender[T], f func(Out) T) *WorkerController[In, Out] {
	return c.Forward(func(o Out) { s.Send(f(o)) })
}

// Detach releases the worker from its owner; it then runs until
// Shutdown or until the loop stops.
func (c *WorkerController[In, Out]) Detach() *WorkerController[In, Out] {
	c.detach(c.loop, c.Shutdown)
	return c
}

// Shutdown stops the worker: queued input is dropped, the Update in
// progress finishes, and its WorkerShutdowner hook runs. Outputs not
// yet delivered when Shutdown runs on the loop are delivered then; later
// ones are dropped. Calling it again does nothing.
func (c *WorkerController[In, Out]) Shutdown() {
	c.stop.Do(func() {
		c.inputs.Stop()
		close(c.done)
		c.outputs.stop()
		c.unown()
	})
}

func (c *WorkerController[In, Out]) ownerShutdown() {
	if !c.detached {
		c.Shutdown()
	}
}
