// Package component is gelm's component framework, the counterpart of
// relm4's Component: a model with typed input and output messages, an
// Init that builds its view, an Update that runs on the loop goroutine
// for every input, and controllers that own a component's lifetime and
// forward its outputs.
//
//	type Counter struct{ n int }
//
//	func (c *Counter) Init(cx *component.Context[Msg, Out]) widget.Widget { ... }
//	func (c *Counter) Update(cx *component.Context[Msg, Out], msg Msg) { ... }
//
//	ctrl := component.Launch(application, &Counter{}).
//		Forward(func(o Out) { ... })
//
// A component launched from another component's Context is that
// component's child: it shuts down with its parent unless detached.
package component

import (
	"slices"

	"github.com/stubbedev/gelm/internal/mailbox"
	"github.com/stubbedev/gelm/widget"
)

// Loop is the event loop components run on. *app.Application is the
// production loop; componenttest.Loop drives components in tests.
type Loop interface {
	Invoke(fn func())
	OnStop(fn func()) (cancel func())
}

// Component is a model with typed input and output messages. Init runs
// once at launch and returns the root widget. Update runs on the loop
// goroutine for every input, in send order and never reentrantly.
type Component[In, Out any] interface {
	Init(cx *Context[In, Out]) widget.Widget
	Update(cx *Context[In, Out], msg In)
}

// ViewUpdater is implemented by components that refresh their view
// after each batch of updates, relm4's update_view.
type ViewUpdater[In, Out any] interface {
	UpdateView(cx *Context[In, Out])
}

// Shutdowner is implemented by components that release resources when
// they shut down. Outputs sent from Shutdown are still delivered.
type Shutdowner[In, Out any] interface {
	Shutdown(cx *Context[In, Out])
}

// Sender sends messages of one type to a component, from any
// goroutine.
type Sender[M any] struct {
	send func(M)
}

// Send delivers msg.
func (s Sender[M]) Send(msg M) {
	if s.send == nil {
		panic("component: Send on a zero Sender")
	}
	s.send(msg)
}

type owned interface {
	ownerShutdown()
	setRelease(release func())
}

// Context is a running component's handle to its runtime: sending
// itself input, emitting output, registering view refreshes and
// cleanups, and launching children.
type Context[In, Out any] struct {
	loop     Loop
	model    Component[In, Out]
	root     widget.Widget
	inputs   mailbox.Box[In]
	outputs  mailbox.Box[Out]
	forward  func(Out)
	watches  []func()
	hooks    []func()
	children []owned
	release  func()
	detached bool
	dead     bool
}

// Loop returns the loop the component runs on.
func (cx *Context[In, Out]) Loop() Loop { return cx.loop }

// Input sends msg to the component's own Update. Safe from any
// goroutine; from inside Update it lands on the next loop pass.
func (cx *Context[In, Out]) Input(msg In) {
	if cx.inputs.Put(msg) {
		cx.loop.Invoke(cx.drainInputs)
	}
}

// Output emits msg to whatever the controller forwards to. Safe from
// any goroutine; delivery happens on the loop.
func (cx *Context[In, Out]) Output(msg Out) {
	if cx.outputs.Put(msg) {
		cx.loop.Invoke(cx.drainOutputs)
	}
}

// Sender returns a Sender for the component's input, for goroutines and
// other components.
func (cx *Context[In, Out]) Sender() Sender[In] { return Sender[In]{send: cx.Input} }

// Watch runs fn now and again after every batch of updates, relm4's
// #[watch]. It is how a view keeps a widget in step with the model.
func (cx *Context[In, Out]) Watch(fn func()) {
	cx.watches = append(cx.watches, fn)
	fn()
}

// OnShutdown registers fn to run when the component shuts down, in
// reverse registration order.
func (cx *Context[In, Out]) OnShutdown(fn func()) {
	cx.hooks = append(cx.hooks, fn)
}

// Launch starts c as a child of this component: it shuts down when this
// component does, unless its controller is detached.
func (cx *Context[In, Out]) Launch[CIn, COut any](c Component[CIn, COut]) *Controller[CIn, COut] {
	child := start(cx.loop, c)
	cx.adopt(child)
	return &Controller[CIn, COut]{cx: child}
}

func (cx *Context[In, Out]) adopt(child owned) {
	cx.children = append(cx.children, child)
	child.setRelease(func() {
		cx.children = slices.DeleteFunc(cx.children, func(o owned) bool { return o == child })
	})
}

func (cx *Context[In, Out]) setRelease(fn func()) { cx.release = fn }

// Launch starts c on loop as a top-level component, shut down when the
// loop stops.
func Launch[In, Out any](loop Loop, c Component[In, Out]) *Controller[In, Out] {
	cx := start(loop, c)
	cx.release = loop.OnStop(cx.ownerShutdown)
	return &Controller[In, Out]{cx: cx}
}

func start[In, Out any](loop Loop, c Component[In, Out]) *Context[In, Out] {
	cx := &Context[In, Out]{loop: loop, model: c}
	cx.root = c.Init(cx)
	if cx.root == nil {
		panic("component: Init returned no root widget")
	}
	return cx
}

func (cx *Context[In, Out]) drainInputs() {
	msgs := cx.inputs.Take()
	if cx.dead || len(msgs) == 0 {
		return
	}
	for _, msg := range msgs {
		cx.model.Update(cx, msg)
		if cx.dead {
			return
		}
	}
	if v, ok := cx.model.(ViewUpdater[In, Out]); ok {
		v.UpdateView(cx)
	}
	for _, w := range cx.watches {
		w()
	}
}

func (cx *Context[In, Out]) drainOutputs() {
	for _, msg := range cx.outputs.Take() {
		if cx.forward != nil {
			cx.forward(msg)
		}
	}
}

func (cx *Context[In, Out]) ownerShutdown() {
	if !cx.detached {
		cx.shutdown()
	}
}

func (cx *Context[In, Out]) shutdown() {
	if cx.dead {
		return
	}
	cx.dead = true
	cx.inputs.Stop()
	children := cx.children
	cx.children = nil
	for _, child := range slices.Backward(children) {
		child.ownerShutdown()
	}
	if s, ok := cx.model.(Shutdowner[In, Out]); ok {
		s.Shutdown(cx)
	}
	for _, fn := range slices.Backward(cx.hooks) {
		fn()
	}
	cx.drainOutputs()
	cx.outputs.Stop()
	if cx.release != nil {
		cx.release()
	}
}

// Controller owns a launched component: its root widget, its input,
// where its outputs go, and its lifetime.
type Controller[In, Out any] struct {
	cx *Context[In, Out]
}

// Widget returns the component's root widget.
func (c *Controller[In, Out]) Widget() widget.Widget { return c.cx.root }

// Send delivers msg to the component's Update. Safe from any goroutine.
func (c *Controller[In, Out]) Send(msg In) { c.cx.Input(msg) }

// Sender returns a Sender for the component's input.
func (c *Controller[In, Out]) Sender() Sender[In] { return c.cx.Sender() }

// Forward routes every output to fn on the loop goroutine, replacing
// any earlier route. Outputs with no route are dropped.
func (c *Controller[In, Out]) Forward(fn func(Out)) *Controller[In, Out] {
	c.cx.forward = fn
	return c
}

// ForwardTo routes every output, mapped by f, to s: relm4's
// forward(sender, map).
func (c *Controller[In, Out]) ForwardTo[T any](s Sender[T], f func(Out) T) *Controller[In, Out] {
	return c.Forward(func(o Out) { s.Send(f(o)) })
}

// Detach releases the component from its owner, so the owner's shutdown
// no longer reaches it. It then runs until Shutdown or until the loop
// stops.
func (c *Controller[In, Out]) Detach() *Controller[In, Out] {
	cx := c.cx
	if cx.dead || cx.detached {
		return c
	}
	cx.detached = true
	if cx.release != nil {
		cx.release()
	}
	cx.release = cx.loop.OnStop(cx.shutdown)
	return c
}

// Shutdown stops the component on the loop goroutine: its children shut
// down first, then the component's own Shutdown and OnShutdown hooks
// run, and later input is dropped. Calling it again does nothing.
func (c *Controller[In, Out]) Shutdown() { c.cx.shutdown() }

// Alive reports whether the component has not shut down.
func (c *Controller[In, Out]) Alive() bool { return !c.cx.dead }
