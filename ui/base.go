package ui

import (
	"github.com/stubbedev/gelm/component"
	"github.com/stubbedev/gelm/widget"
)

type base[W nodeProps, B any] struct {
	self      B
	construct func(*Scope) W
	ops       []func(*Scope, W)
}

func (b *base[W, B]) init(self B, construct func(*Scope) W) {
	b.self = self
	b.construct = construct
}

func (b *base[W, B]) do(op func(*Scope, W)) { b.ops = append(b.ops, op) }

func (b *base[W, B]) watch(fn func(W)) {
	b.do(func(s *Scope, w W) { s.watch(func() { fn(w) }) })
}

// Build builds the widget in s; it implements Node.
func (b *base[W, B]) Build(s *Scope) widget.Widget { return b.Widget(s) }

// Widget builds the widget in s once: later calls in the same scope
// return the same widget.
func (b *base[W, B]) Widget(s *Scope) W {
	if built, ok := s.built[b]; ok {
		w, ok := built.(W)
		if !ok {
			panic("ui: a builder's memoized widget changed type")
		}
		return w
	}
	w := b.construct(s)
	s.built[b] = w
	for _, op := range b.ops {
		op(s, w)
	}
	return w
}

// Ref stores the built widget in *p, relm4's #[name].
func (b *base[W, B]) Ref(p *W) B {
	b.do(func(_ *Scope, w W) { *p = w })
	return b.self
}

// With runs fn on the built widget: the escape hatch for anything the
// builder does not cover.
func (b *base[W, B]) With(fn func(W)) B {
	b.do(func(_ *Scope, w W) { fn(w) })
	return b.self
}

// Watch runs fn on the widget now and after every update, relm4's
// #[watch].
func (b *base[W, B]) Watch(fn func(W)) B {
	b.watch(fn)
	return b.self
}

// Track runs fn on the widget now, and after an update only when one of
// deps changed: relm4's #[track].
func (b *base[W, B]) Track(fn func(W), deps ...component.Change) B {
	b.do(func(s *Scope, w W) { s.track(func() { fn(w) }, deps) })
	return b.self
}
