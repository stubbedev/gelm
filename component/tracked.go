package component

// Tracked is a model value that knows whether it changed since the
// component's last view refresh, relm4's tracker. Create it with
// Context.Tracked; Set and Update mark it changed, and the component
// clears the mark after every refresh.
type Tracked[T any] struct {
	value   T
	changed bool
	owned   bool
}

// Get returns the value.
func (t *Tracked[T]) Get() T { return t.value }

// Set replaces the value and marks it changed.
func (t *Tracked[T]) Set(v T) {
	t.mark()
	t.value = v
}

// Update applies fn to the value in place and marks it changed.
func (t *Tracked[T]) Update(fn func(*T)) {
	t.mark()
	fn(&t.value)
}

func (t *Tracked[T]) mark() {
	if !t.owned {
		panic("component: Tracked value not created by Context.Tracked")
	}
	t.changed = true
}

// Changed reports whether the value was set since the last refresh.
func (t *Tracked[T]) Changed() bool { return t.changed }

func (t *Tracked[T]) clearChanged() { t.changed = false }

// Change is anything that reports a change since the last refresh,
// such as a *Tracked value.
type Change interface {
	Changed() bool
}

// Tracked returns a tracked value owned by this component, starting
// unchanged at initial.
func (cx *Context[In, Out]) Tracked[T any](initial T) *Tracked[T] {
	t := &Tracked[T]{value: initial, owned: true}
	cx.clears = append(cx.clears, t.clearChanged)
	return t
}

// Track runs fn now, and after a batch of updates only when one of deps
// changed: relm4's #[track]. Without deps it never runs again.
func (cx *Context[In, Out]) Track(fn func(), deps ...Change) {
	cx.watches = append(cx.watches, func() {
		for _, d := range deps {
			if d.Changed() {
				fn()
				return
			}
		}
	})
	fn()
}
