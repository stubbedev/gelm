package component

import (
	"fmt"
	"iter"
	"slices"
)

// Index is a factory item's position, kept current as items are
// inserted, removed and moved: relm4's DynamicIndex.
type Index struct {
	i int
}

// Current returns the item's position, or -1 once it was removed.
func (x *Index) Current() int { return x.i }

type factoryItem[C Component[In, Out], In, Out any] struct {
	model C
	cx    *Context[In, Out]
	index *Index
}

// Factory is a collection of item components rendered into a container,
// relm4's FactoryVecDeque. Every item is a full component (Init, Update,
// outputs, children, Loader for async items) with a stable *Index. Edits
// apply to the container in place as they are made: items that stay
// keep their widgets, moved items move, and nothing is rebuilt. Like
// every widget mutation, a Factory belongs to the loop goroutine.
type Factory[C Component[In, Out], In, Out any] struct {
	lifetime
	loop    Loop
	view    FactoryView[C]
	items   []*factoryItem[C, In, Out]
	forward func(*Index, Out)
	dead    bool
}

// NewFactory returns a top-level factory rendering into view, shut down
// when the loop stops.
func NewFactory[C Component[In, Out], In, Out any](loop Loop, view FactoryView[C]) *Factory[C, In, Out] {
	f := &Factory[C, In, Out]{loop: loop, view: view}
	f.release = loop.OnStop(f.ownerShutdown)
	return f
}

// NewFactory returns a factory owned by this component, rendering into
// view: its items shut down with the component.
func (cx *Context[In, Out]) NewFactory[C Component[CIn, COut], CIn, COut any](view FactoryView[C]) *Factory[C, CIn, COut] {
	f := &Factory[C, CIn, COut]{loop: cx.loop, view: view}
	cx.adopt(f)
	return f
}

// Len returns the number of items.
func (f *Factory[C, In, Out]) Len() int { return len(f.items) }

// Get returns the item at i.
func (f *Factory[C, In, Out]) Get(i int) C { return f.at(i).model }

// All yields every item with its position, in order.
func (f *Factory[C, In, Out]) All() iter.Seq2[int, C] {
	return func(yield func(int, C) bool) {
		for i, it := range f.items {
			if !yield(i, it.model) {
				return
			}
		}
	}
}

// PushBack appends c and returns its index.
func (f *Factory[C, In, Out]) PushBack(c C) *Index { return f.Insert(len(f.items), c) }

// PushFront prepends c and returns its index.
func (f *Factory[C, In, Out]) PushFront(c C) *Index { return f.Insert(0, c) }

// Insert launches c as the item at position i (0 to Len) and returns
// its index.
func (f *Factory[C, In, Out]) Insert(i int, c C) *Index {
	if f.dead {
		panic("component: Insert on a shut down Factory")
	}
	if i < 0 || i > len(f.items) {
		panic(fmt.Sprintf("component: Factory.Insert at %d of %d items", i, len(f.items)))
	}
	idx := &Index{i: i}
	it := &factoryItem[C, In, Out]{model: c, index: idx}
	it.cx = startAt(f.loop, Component[In, Out](c), idx)
	it.cx.outputs.forward = func(o Out) {
		if f.forward != nil {
			f.forward(idx, o)
		}
	}
	f.items = slices.Insert(f.items, i, it)
	f.reindex(i + 1)
	f.view.Insert(i, c, it.cx.root)
	return idx
}

// Remove shuts down the item at i, removes its widget, and returns its
// model.
func (f *Factory[C, In, Out]) Remove(i int) C {
	it := f.at(i)
	f.view.Remove(i, it.model, it.cx.root)
	f.items = slices.Delete(f.items, i, i+1)
	it.index.i = -1
	f.reindex(i)
	it.cx.shutdown()
	return it.model
}

// Move moves the item at from to position to, in place.
func (f *Factory[C, In, Out]) Move(from, to int) {
	it := f.at(from)
	f.at(to)
	if from == to {
		return
	}
	f.items = slices.Insert(slices.Delete(f.items, from, from+1), to, it)
	f.reindex(min(from, to))
	f.view.Move(from, to, it.model, it.cx.root)
}

// Swap exchanges the items at i and j.
func (f *Factory[C, In, Out]) Swap(i, j int) {
	if i == j {
		f.at(i)
		return
	}
	lo, hi := min(i, j), max(i, j)
	f.Move(hi, lo)
	f.Move(lo+1, hi)
}

// Clear removes every item, last first.
func (f *Factory[C, In, Out]) Clear() {
	for len(f.items) > 0 {
		f.Remove(len(f.items) - 1)
	}
}

// Send delivers msg to the item at i.
func (f *Factory[C, In, Out]) Send(i int, msg In) { f.at(i).cx.Input(msg) }

// Broadcast delivers msg to every item.
func (f *Factory[C, In, Out]) Broadcast(msg In) {
	for _, it := range f.items {
		it.cx.Input(msg)
	}
}

// Forward routes every item output, with the emitting item's index, to
// fn on the loop goroutine, replacing any earlier route.
func (f *Factory[C, In, Out]) Forward(fn func(*Index, Out)) *Factory[C, In, Out] {
	f.forward = fn
	return f
}

// ForwardTo routes every item output, mapped by m, to s.
func (f *Factory[C, In, Out]) ForwardTo[T any](s Sender[T], m func(*Index, Out) T) *Factory[C, In, Out] {
	return f.Forward(func(x *Index, o Out) { s.Send(m(x, o)) })
}

// Detach releases the factory from its owner; it then lives until
// Shutdown or until the loop stops.
func (f *Factory[C, In, Out]) Detach() *Factory[C, In, Out] {
	f.detach(f.loop, f.Shutdown)
	return f
}

// Shutdown shuts every item down, last first, and leaves their widgets
// in place. Calling it again does nothing.
func (f *Factory[C, In, Out]) Shutdown() {
	if f.dead {
		return
	}
	f.dead = true
	for _, it := range slices.Backward(f.items) {
		it.cx.shutdown()
	}
	f.unown()
}

func (f *Factory[C, In, Out]) ownerShutdown() {
	if !f.detached {
		f.Shutdown()
	}
}

func (f *Factory[C, In, Out]) at(i int) *factoryItem[C, In, Out] {
	if i < 0 || i >= len(f.items) {
		panic(fmt.Sprintf("component: factory index %d out of range [0, %d)", i, len(f.items)))
	}
	return f.items[i]
}

func (f *Factory[C, In, Out]) reindex(from int) {
	for i := from; i < len(f.items); i++ {
		f.items[i].index.i = i
	}
}
