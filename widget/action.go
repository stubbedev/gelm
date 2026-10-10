package widget

import (
	"fmt"
	"slices"
	"sync"
)

// Activatable is what menu items, buttons and accelerators bind to: an
// action, or a stateful action's target. Activate does nothing while
// Enabled is false. Subscribe hears every enabled or state change.
type Activatable interface {
	Name() string
	Activate()
	Enabled() bool
	Subscribe(fn func()) (cancel func())
}

// Checkable is an Activatable with a checked state: a toggle or a
// radio target, rendered as a check or radio menu item.
type Checkable interface {
	Activatable
	Checked() bool
}

type actionCore struct {
	name     string
	disabled bool
	nextSub  int
	subs     []actionSub
}

type actionSub struct {
	id int
	fn func()
}

// Name returns the action's name.
func (c *actionCore) Name() string { return c.name }

// Enabled reports whether the action may be activated.
func (c *actionCore) Enabled() bool { return !c.disabled }

// SetEnabled enables or disables the action and every proxy bound to
// it.
func (c *actionCore) SetEnabled(on bool) {
	checkLoop("Action.SetEnabled")
	if c.disabled == !on {
		return
	}
	c.disabled = !on
	c.notify()
}

// Subscribe registers fn for every enabled or state change; the
// returned cancel removes it.
func (c *actionCore) Subscribe(fn func()) (cancel func()) {
	checkLoop("Action.Subscribe")
	c.nextSub++
	id := c.nextSub
	c.subs = append(c.subs, actionSub{id: id, fn: fn})
	return sync.OnceFunc(func() {
		c.subs = slices.DeleteFunc(c.subs, func(s actionSub) bool { return s.id == id })
	})
}

func (c *actionCore) notify() {
	for _, s := range slices.Clone(c.subs) {
		s.fn()
	}
}

// Action is a named, stateless action, relm4's RelmAction without
// state: menu items, buttons and accelerators activate it, and
// disabling it disables them all. Like every widget, it belongs to the
// loop goroutine.
type Action struct {
	actionCore
	activate func()
}

// NewAction returns an enabled action running fn on activation.
func NewAction(name string, fn func()) *Action {
	if fn == nil {
		panic(fmt.Sprintf("widget: action %q has no function", name))
	}
	return &Action{name: name, activate: fn}
}

// Activate runs the action unless it is disabled.
func (a *Action) Activate() {
	if !a.disabled {
		a.activate()
	}
}

// StateAction is a named action holding a state, relm4's stateful
// RelmAction: activating one of its targets sets the state, and every
// proxy shows the state it holds. A bool state is a toggle (Toggle); any
// comparable state can drive a radio group (Target).
type StateAction[S comparable] struct {
	actionCore
	state    S
	onChange func(S)
}

// NewStateAction returns an enabled action at initial; onChange, when
// not nil, hears every change of state.
func NewStateAction[S comparable](name string, initial S, onChange func(S)) *StateAction[S] {
	return &StateAction[S]{name: name, state: initial, onChange: onChange}
}

// State returns the current state.
func (a *StateAction[S]) State() S { return a.state }

// SetState changes the state, notifying onChange and every proxy; an
// equal state does nothing. It applies even while the action is
// disabled, since disabling gates the user, not the program.
func (a *StateAction[S]) SetState(s S) {
	checkLoop("StateAction.SetState")
	if s == a.state {
		return
	}
	a.state = s
	if a.onChange != nil {
		a.onChange(s)
	}
	a.notify()
}

// Target is the Checkable that sets the state to v: one radio item of
// the group the action's states form.
func (a *StateAction[S]) Target(v S) Checkable { return stateTarget[S]{action: a, value: v} }

type stateTarget[S comparable] struct {
	action *StateAction[S]
	value  S
}

func (t stateTarget[S]) Name() string  { return fmt.Sprintf("%s(%v)", t.action.name, t.value) }
func (t stateTarget[S]) Enabled() bool { return t.action.Enabled() }
func (t stateTarget[S]) Checked() bool { return t.action.state == t.value }
func (t stateTarget[S]) Subscribe(fn func()) (cancel func()) {
	return t.action.Subscribe(fn)
}

func (t stateTarget[S]) Activate() {
	if t.action.Enabled() {
		t.action.SetState(t.value)
	}
}

// ParamAction is a named action activated with a value, relm4's
// stateless action with a target type: one "open-file" action serving
// every recent-file row. Proxies bind a Target, a value fixed at
// binding time. The parameter is comparable because targets are
// compared and keyed by the accelerator registry.
type ParamAction[P comparable] struct {
	actionCore
	activate func(P)
}

// NewParamAction returns an enabled action running fn with the bound
// value on activation.
func NewParamAction[P comparable](name string, fn func(P)) *ParamAction[P] {
	if fn == nil {
		panic(fmt.Sprintf("widget: action %q has no function", name))
	}
	return &ParamAction[P]{name: name, activate: fn}
}

// ActivateWith runs the action with p unless it is disabled.
func (a *ParamAction[P]) ActivateWith(p P) {
	if !a.disabled {
		a.activate(p)
	}
}

// Target is the Activatable that runs the action with p.
func (a *ParamAction[P]) Target(p P) Activatable { return paramTarget[P]{action: a, value: p} }

type paramTarget[P comparable] struct {
	action *ParamAction[P]
	value  P
}

func (t paramTarget[P]) Name() string  { return fmt.Sprintf("%s(%v)", t.action.name, t.value) }
func (t paramTarget[P]) Enabled() bool { return t.action.Enabled() }
func (t paramTarget[P]) Activate()     { t.action.ActivateWith(t.value) }
func (t paramTarget[P]) Subscribe(fn func()) (cancel func()) {
	return t.action.Subscribe(fn)
}

// Toggle is the Checkable that flips a bool action: a check item, a
// toggle button.
func Toggle(a *StateAction[bool]) Checkable { return toggleTarget{action: a} }

type toggleTarget struct{ action *StateAction[bool] }

func (t toggleTarget) Name() string  { return t.action.name }
func (t toggleTarget) Enabled() bool { return t.action.Enabled() }
func (t toggleTarget) Checked() bool { return t.action.state }
func (t toggleTarget) Subscribe(fn func()) (cancel func()) {
	return t.action.Subscribe(fn)
}

func (t toggleTarget) Activate() {
	if t.action.Enabled() {
		t.action.SetState(!t.action.state)
	}
}

// BindAction makes the button a proxy of a: a click activates it, and
// the button is enabled exactly while the action is. The returned
// unbind detaches it.
func (b *Button) BindAction(a Activatable) (unbind func()) {
	b.OnClick = a.Activate
	return followEnabled(a, b, func() { b.OnClick = nil })
}

// BindAction makes the toggle a proxy of c: it shows c's checked state,
// a click activates c, and it is enabled exactly while c is.
func (t *ToggleButton) BindAction(c Checkable) (unbind func()) {
	t.OnClick = t.clicked
	return bindCheckable(c, t, t.SetActive, func(fn func(bool)) { t.OnToggled = fn })
}

// BindAction makes the check button a proxy of c, like
// ToggleButton.BindAction.
func (c *CheckButton) BindAction(target Checkable) (unbind func()) {
	return bindCheckable(target, c, c.SetChecked, func(fn func(bool)) { c.OnChanged = fn })
}

// BindAction makes the switch a proxy of c, like
// ToggleButton.BindAction.
func (s *Switch) BindAction(c Checkable) (unbind func()) {
	return bindCheckable(c, s, s.SetOn, func(fn func(bool)) { s.OnChanged = fn })
}

func followEnabled(a Activatable, w interface{ SetEnabled(bool) }, detach func()) (unbind func()) {
	w.SetEnabled(a.Enabled())
	cancel := a.Subscribe(func() { w.SetEnabled(a.Enabled()) })
	return sync.OnceFunc(func() {
		cancel()
		detach()
	})
}

func bindCheckable(c Checkable, w interface{ SetEnabled(bool) }, set func(bool), hook func(func(bool))) (unbind func()) {
	set(c.Checked())
	hook(func(on bool) {
		if on != c.Checked() {
			c.Activate()
		}
		set(c.Checked())
	})
	cancel := c.Subscribe(func() { set(c.Checked()) })
	return followEnabled(c, w, func() {
		cancel()
		hook(nil)
	})
}
