package widget

import (
	"reflect"
	"sync"
)

// Binding is a loop-owned observable model value, gelm's counterpart of
// relm4's binding module: one T that any number of widgets observe.
// Set applies the value and notifies the subscribers synchronously,
// which is what a widget connector needs - the new value is on screen
// by the time Set returns. Two rules keep two-way wiring echo-free by
// construction: an equal value is a no-op (nothing notifies), and a Set
// from inside a notification is deferred until that notification
// finishes, so a subscriber writing back cannot recurse.
//
// Like every widget mutation, Get/Set/Subscribe belong on the loop
// goroutine (docs/threading.md); they are sampled by the off-loop hook.
type Binding[T any] struct {
	mu        sync.Mutex
	val       T
	subs      []bindingSub[T]
	nextSubID int
	notifying bool
	pending   *T
}

type bindingSub[T any] struct {
	id int
	fn func(T)
}

// NewBinding creates a Binding holding initial.
func NewBinding[T any](initial T) *Binding[T] {
	return &Binding[T]{val: initial}
}

// Get returns the current value.
func (b *Binding[T]) Get() T {
	checkLoop("Binding.Get")
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.val
}

// Set applies v and notifies the subscribers - unless v equals the
// current value, which is a no-op, or a notification is running, in
// which case v replaces the deferred write and is applied (and
// notified) when the running notification completes.
func (b *Binding[T]) Set(v T) {
	checkLoop("Binding.Set")
	b.mu.Lock()
	if b.notifying {
		b.pending = &v
		b.mu.Unlock()
		return
	}
	if reflect.DeepEqual(b.val, v) {
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()
	b.notify(v)
}

// Subscribe registers fn for every change. The returned cancel removes
// it (idempotent, loop-side like the rest of the Binding API).
func (b *Binding[T]) Subscribe(fn func(T)) (cancel func()) {
	checkLoop("Binding.Subscribe")
	if fn == nil {
		return func() {}
	}
	b.mu.Lock()
	b.nextSubID++
	id := b.nextSubID
	b.subs = append(b.subs, bindingSub[T]{id: id, fn: fn})
	b.mu.Unlock()
	return sync.OnceFunc(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		for i, s := range b.subs {
			if s.id == id {
				b.subs = append(b.subs[:i], b.subs[i+1:]...)
				return
			}
		}
	})
}

// notify stores v, marks the notification, and runs the subscribers
// outside the lock with a snapshot; writes they make land in pending
// and flush afterwards, one deferred notification at most.
func (b *Binding[T]) notify(v T) {
	b.mu.Lock()
	b.val = v
	b.notifying = true
	subs := make([]bindingSub[T], len(b.subs))
	copy(subs, b.subs)
	b.mu.Unlock()
	for _, s := range subs {
		s.fn(v)
	}
	b.mu.Lock()
	b.notifying = false
	pending := b.pending
	b.pending = nil
	b.mu.Unlock()
	if pending != nil {
		b.Set(*pending)
	}
}

// bindWidget is the connector core every widget binding shares: apply
// the current value, subscribe the widget's setter to later changes,
// and install hook as the widget's change callback so user edits write
// back through Set. Unbind cancels the subscription and clears the
// callback; the connector owns the widget's callback field for its
// lifetime.
func bindWidget[T any](b *Binding[T], set func(T), hook func(func(T))) (unbind func()) {
	set(b.Get())
	cancel := b.Subscribe(set)
	hook(func(v T) { b.Set(v) })
	return sync.OnceFunc(func() {
		cancel()
		hook(nil)
	})
}

// BindText wires the binding two-way to the entry's text: changes
// write through, edits write back. Entry.OnChanged fires for every
// source, so the write-back of the binding's own value is the equal
// no-op that ends the loop.
func (e *Entry) BindText(b *Binding[string]) (unbind func()) {
	return bindWidget(b, e.SetText, func(fn func(string)) { e.OnChanged = fn })
}

// BindText wires the binding two-way to the text area's contents.
func (t *TextArea) BindText(b *Binding[string]) (unbind func()) {
	return bindWidget(b, t.SetText, func(fn func(string)) {
		if fn == nil {
			t.OnChanged = nil
			return
		}
		t.OnChanged = func() { fn(t.Text()) }
	})
}

// BindText wires the binding one-way to the label's text: a label
// never edits, so nothing writes back.
func (l *Label) BindText(b *Binding[string]) (unbind func()) {
	l.SetText(b.Get())
	return b.Subscribe(l.SetText)
}

// BindOn wires the binding two-way to the switch's state.
func (s *Switch) BindOn(b *Binding[bool]) (unbind func()) {
	return bindWidget(b, s.SetOn, func(fn func(bool)) { s.OnChanged = fn })
}

// BindChecked wires the binding two-way to the checkbox's state.
func (c *CheckButton) BindChecked(b *Binding[bool]) (unbind func()) {
	return bindWidget(b, c.SetChecked, func(fn func(bool)) { c.OnChanged = fn })
}

// BindValue wires the binding two-way to the slider's value. SetValue
// clamps and snaps, and fires OnChanged only when the shown value
// moved, so a binding value off-grid settles on the snapped one
// without looping.
func (s *Slider) BindValue(b *Binding[float64]) (unbind func()) {
	return bindWidget(b, s.SetValue, func(fn func(float64)) { s.OnChanged = fn })
}

// BindValue wires the binding two-way to the spin button's value.
// SetValue is silent - only user edits fire OnValueChanged - so the
// programmatic apply never echoes.
func (s *SpinButton) BindValue(b *Binding[float64]) (unbind func()) {
	return bindWidget(b, s.SetValue, func(fn func(float64)) { s.OnValueChanged = fn })
}

// BindSelected wires the binding two-way to the dropdown's selection
// index.
func (d *Dropdown) BindSelected(b *Binding[int]) (unbind func()) {
	return bindWidget(b, d.SetSelected, func(fn func(int)) { d.OnSelect = fn })
}
