package widget

import (
	"bytes"
	"testing"

	"github.com/stubbedev/gelm/render"
)

func TestSetFocus(t *testing.T) {
	a := newFocusTarget()
	b := newFocusTarget()
	root := NewBox(Column, 4, 0)
	root.Append(a, false)
	root.Append(NewScroll(b), false)

	t.Run("a focusable widget in the tree takes focus", func(t *testing.T) {
		r := &Router{Root: root}
		r.SetFocus(b)
		if r.Focused() != Widget(b) {
			t.Fatalf("focus = %v, want b", r.Focused())
		}
		// Programmatic focus is real focus: traversal continues from it.
		r.FocusNext()
		if r.Focused() != Widget(a) {
			t.Errorf("focus after next = %v, want wrap to a", r.Focused())
		}
	})

	t.Run("nil clears focus", func(t *testing.T) {
		r := &Router{Root: root}
		r.SetFocus(a)
		r.SetFocus(nil)
		if r.Focused() != nil {
			t.Errorf("focus = %v, want none", r.Focused())
		}
	})

	t.Run("what cannot take keys is ignored", func(t *testing.T) {
		r := &Router{Root: root}
		r.SetFocus(a)
		r.SetFocus(root) // a Box has no KeyAction
		if r.Focused() != Widget(a) {
			t.Errorf("focus = %v, want a kept", r.Focused())
		}
	})

	t.Run("a widget outside the tree is ignored", func(t *testing.T) {
		r := &Router{Root: root}
		r.SetFocus(a)
		r.SetFocus(newFocusTarget())
		if r.Focused() != Widget(a) {
			t.Errorf("focus = %v, want a kept", r.Focused())
		}
	})

	t.Run("a disabled widget is ignored", func(t *testing.T) {
		r := &Router{Root: root}
		r.SetFocus(a)
		b.SetEnabled(false)
		defer b.SetEnabled(true)
		r.SetFocus(b)
		if r.Focused() != Widget(a) {
			t.Errorf("focus = %v, want a kept", r.Focused())
		}
	})
}

// The focus hook hears every way focus moves: a press elsewhere,
// traversal, SetFocus, and the widget leaving the tree; a focus that
// stays put says nothing.
func TestFocusChangedHook(t *testing.T) {
	a := newFocusTarget()
	b := newFocusTarget()
	root := NewBox(Column, 4, 0)
	root.Append(a, false)
	root.Append(b, false)
	var got []bool
	a.SetOnFocusChanged(func(focused bool) { got = append(got, focused) })
	r := &Router{Root: root}

	r.SetFocus(a)
	r.SetFocus(a)
	r.FocusNext()
	r.FocusNext()
	r.SetFocus(nil)
	if want := []bool{true, false, true, false}; !equalBools(got, want) {
		t.Fatalf("hook heard %v, want %v", got, want)
	}

	got = nil
	r.SetFocus(a)
	root.Remove(a)
	r.Forget(a)
	if want := []bool{true, false}; !equalBools(got, want) {
		t.Errorf("removal: hook heard %v, want %v", got, want)
	}

	got = nil
	a.SetOnFocusChanged(nil)
	root.Append(a, false)
	r.SetFocus(a)
	if got != nil {
		t.Errorf("an unregistered hook heard %v", got)
	}
}

func equalBools(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A text field paints its caret only while it holds keyboard focus.
func TestCaretOnlyWhileFocused(t *testing.T) {
	face := testFace(t)
	e := NewEntry(face, 14, render.RGB(0xff, 0xff, 0xff))
	ta := NewTextArea(face, 14, render.RGB(0xff, 0xff, 0xff))
	other := newFocusTarget()
	root := NewBox(Column, 0, 0)
	root.Append(e, false).Append(ta, false).Append(other, false)
	root.Measure(Constraints{Max: Size{W: 120, H: 120}})
	root.Arrange(render.Rect{W: 120, H: 120})
	r := &Router{Root: root}
	shot := func(w Widget) []byte {
		data := make([]byte, render.Stride(120)*120)
		w.Paint(render.New(data, render.Stride(120), 120, 120))
		return data
	}
	for name, w := range map[string]Widget{"entry": e, "textarea": ta} {
		r.SetFocus(other)
		blurred := shot(w)
		r.SetFocus(w)
		focused := shot(w)
		r.SetFocus(other)
		if bytes.Equal(blurred, focused) {
			t.Errorf("%s: focus painted no caret", name)
		}
		if !bytes.Equal(blurred, shot(w)) {
			t.Errorf("%s: the caret stayed after focus left", name)
		}
	}
}
