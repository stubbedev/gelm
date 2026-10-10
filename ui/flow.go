package ui

import (
	"fmt"
	"iter"
	"strconv"

	"github.com/stubbedev/gelm/widget"
)

type packed struct {
	Node
	expand  bool
	align   widget.Align
	aligned bool
}

// Expand marks a Column or Row child to take the spare space on the
// box's axis.
func Expand(n Node) Node { return packed{Node: n, expand: true} }

// Aligned places a Column or Row child at a on the cross axis.
func Aligned(n Node, a widget.Align) Node { return packed{Node: n, align: a, aligned: true} }

// Column stacks children top to bottom in a widget.Box.
func Column(children ...Node) *BoxBuilder { return pack(widget.Column, children) }

// Row lays children out start to end in a widget.Box.
func Row(children ...Node) *BoxBuilder { return pack(widget.Row, children) }

func pack(axis widget.Axis, children []Node) *BoxBuilder {
	b := Box(axis, 0, 0)
	b.do(func(s *Scope, box *widget.Box) {
		for _, c := range children {
			p, ok := c.(packed)
			if !ok {
				box.Append(build(s, c), false)
				continue
			}
			if p.aligned {
				box.AppendAligned(build(s, p.Node), p.expand, p.align)
				continue
			}
			box.Append(build(s, p.Node), p.expand)
		}
	})
	return b
}

// If shows then while cond holds and otherwise when it does not,
// re-checked after every update: a widget.Stack with two pages, the
// counterpart of an if in relm4's view!. A nil otherwise shows nothing.
func If(cond func() bool, then, otherwise Node) *StackBuilder {
	if otherwise == nil {
		otherwise = Spacer(0, 0)
	}
	return Stack().Add("then", then).Add("else", otherwise).Watch(func(st *widget.Stack) {
		if cond() {
			st.Show("then")
		} else {
			st.Show("else")
		}
	})
}

// Case is one branch of a Match.
type Case[K comparable] struct {
	key  K
	node Node
}

// When is the Match branch shown while the key equals k.
func When[K comparable](k K, n Node) Case[K] { return Case[K]{key: k, node: n} }

// Match shows the case whose key equals key(), re-checked after every
// update: the counterpart of a match in relm4's view!. A key with no
// case panics, naming the key.
func Match[K comparable](key func() K, cases ...Case[K]) *StackBuilder {
	st := Stack()
	pages := make(map[K]string, len(cases))
	for i, c := range cases {
		name := strconv.Itoa(i)
		pages[c.key] = name
		st.Add(name, c.node)
	}
	return st.Watch(func(w *widget.Stack) {
		k := key()
		name, ok := pages[k]
		if !ok {
			panic(fmt.Sprintf("ui: Match has no case for %v", k))
		}
		w.Show(name)
	})
}

// Each maps every item of seq to a node, for static lists of children:
// Column(Each(slices.Values(items), row)...). Collections that change
// belong in a component.Factory.
func Each[T any](seq iter.Seq[T], fn func(T) Node) []Node {
	var nodes []Node
	for v := range seq {
		nodes = append(nodes, fn(v))
	}
	return nodes
}

type existing[W widget.Widget] struct{ w W }

func (e existing[W]) Build(*Scope) widget.Widget { return e.w }
func (e existing[W]) Widget(*Scope) W            { return e.w }

// Use places an already built widget in a description: a child
// component's root, a widget made elsewhere.
func Use[W widget.Widget](w W) Of[W] { return existing[W]{w: w} }
