package component

import (
	"cmp"
	"fmt"
	"iter"
	"slices"
)

// KeyedFactory is a Factory whose items are addressed by an
// application key instead of a position: relm4's FactoryHashMap. Items
// keep their insertion order (or the order SortFunc set), render
// through the same FactoryViews, and forward outputs with their key.
type KeyedFactory[K comparable, C Component[In, Out], In, Out any] struct {
	items   *Factory[C, In, Out]
	index   map[K]*Index
	keys    map[*Index]K
	forward func(K, Out)
}

// NewKeyedFactory returns a top-level keyed factory rendering into
// view, shut down when the loop stops.
func NewKeyedFactory[K comparable, C Component[In, Out], In, Out any](loop Loop, view FactoryView[C]) *KeyedFactory[K, C, In, Out] {
	return keyed[K](NewFactory(loop, view))
}

// NewKeyedFactory returns a keyed factory owned by this component.
func (cx *Context[In, Out]) NewKeyedFactory[K comparable, C Component[CIn, COut], CIn, COut any](view FactoryView[C]) *KeyedFactory[K, C, CIn, COut] {
	return keyed[K](cx.NewFactory(view))
}

func keyed[K comparable, C Component[In, Out], In, Out any](f *Factory[C, In, Out]) *KeyedFactory[K, C, In, Out] {
	k := &KeyedFactory[K, C, In, Out]{items: f, index: map[K]*Index{}, keys: map[*Index]K{}}
	f.Forward(func(x *Index, o Out) {
		if k.forward != nil {
			k.forward(k.keys[x], o)
		}
	})
	return k
}

// Len returns the number of items.
func (k *KeyedFactory[K, C, In, Out]) Len() int { return k.items.Len() }

// Has reports whether an item is stored under key.
func (k *KeyedFactory[K, C, In, Out]) Has(key K) bool {
	_, ok := k.index[key]
	return ok
}

// Get returns the item under key.
func (k *KeyedFactory[K, C, In, Out]) Get(key K) C { return k.items.Get(k.position(key)) }

// Index returns the current position of the item under key.
func (k *KeyedFactory[K, C, In, Out]) Index(key K) *Index { return k.index[k.mustKey(key)] }

// All yields every key and item in order.
func (k *KeyedFactory[K, C, In, Out]) All() iter.Seq2[K, C] {
	return func(yield func(K, C) bool) {
		for i, c := range k.items.All() {
			if !yield(k.keys[k.items.items[i].index], c) {
				return
			}
		}
	}
}

// Insert stores c under key at the end. An existing item under key is
// replaced in place: it shuts down and c takes its position.
func (k *KeyedFactory[K, C, In, Out]) Insert(key K, c C) {
	at := k.items.Len()
	if old, ok := k.index[key]; ok {
		at = old.Current()
		k.Remove(key)
	}
	x := k.items.Insert(at, c)
	k.index[key] = x
	k.keys[x] = key
}

// Remove shuts down the item under key, removes its widget, and
// returns its model.
func (k *KeyedFactory[K, C, In, Out]) Remove(key K) C {
	x := k.index[k.mustKey(key)]
	delete(k.index, key)
	delete(k.keys, x)
	return k.items.Remove(x.Current())
}

// Clear removes every item.
func (k *KeyedFactory[K, C, In, Out]) Clear() {
	k.items.Clear()
	clear(k.index)
	clear(k.keys)
}

// SortFunc reorders the items in place by their keys.
func (k *KeyedFactory[K, C, In, Out]) SortFunc(compare func(a, b K) int) {
	order := slices.SortedFunc(func(yield func(K) bool) {
		for key := range k.All() {
			if !yield(key) {
				return
			}
		}
	}, compare)
	for to, key := range order {
		k.items.Move(k.index[key].Current(), to)
	}
}

// Sort reorders the items in place by ascending key.
func Sort[K cmp.Ordered, C Component[In, Out], In, Out any](k *KeyedFactory[K, C, In, Out]) {
	k.SortFunc(cmp.Compare[K])
}

// Send delivers msg to the item under key.
func (k *KeyedFactory[K, C, In, Out]) Send(key K, msg In) { k.items.Send(k.position(key), msg) }

// Broadcast delivers msg to every item.
func (k *KeyedFactory[K, C, In, Out]) Broadcast(msg In) { k.items.Broadcast(msg) }

// Forward routes every item output, with its key, to fn on the loop.
func (k *KeyedFactory[K, C, In, Out]) Forward(fn func(K, Out)) *KeyedFactory[K, C, In, Out] {
	k.forward = fn
	return k
}

// ForwardTo routes every item output, mapped by m, to s.
func (k *KeyedFactory[K, C, In, Out]) ForwardTo[T any](s Sender[T], m func(K, Out) T) *KeyedFactory[K, C, In, Out] {
	return k.Forward(func(key K, o Out) { s.Send(m(key, o)) })
}

// Detach releases the factory from its owner.
func (k *KeyedFactory[K, C, In, Out]) Detach() *KeyedFactory[K, C, In, Out] {
	k.items.Detach()
	return k
}

// Shutdown shuts every item down.
func (k *KeyedFactory[K, C, In, Out]) Shutdown() { k.items.Shutdown() }

func (k *KeyedFactory[K, C, In, Out]) position(key K) int { return k.index[k.mustKey(key)].Current() }

func (k *KeyedFactory[K, C, In, Out]) mustKey(key K) K {
	if _, ok := k.index[key]; !ok {
		panic(fmt.Sprintf("component: no factory item under key %v", key))
	}
	return key
}
