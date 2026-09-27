package widget

import (
	"testing"
)

// TextArea grapheme cluster editing: same cluster unit as Entry (#57),
// per line, with the cluster-refined word segmentation (#56).
func TestTextAreaGraphemeEditing(t *testing.T) {
	t.Run("backspace removes a ZWJ family emoji whole", func(t *testing.T) {
		ta := newTextArea(t, "a"+familyEmoji+"b")
		ta.SetCursor(0, 7)
		ta.Backspace() // b
		if got := ta.Text(); got != "a"+familyEmoji {
			t.Fatalf("text = %q, want a+%q", got, familyEmoji)
		}
		ta.Backspace()
		if got := ta.Text(); got != "a" {
			t.Fatalf("text = %q, want a (family removed whole)", got)
		}
		if _, col := ta.CursorPos(); col != 1 {
			t.Fatalf("col = %d, want 1", col)
		}
	})

	t.Run("delete removes a family emoji whole", func(t *testing.T) {
		ta := newTextArea(t, "a"+familyEmoji+"b")
		ta.SetCursor(0, 1)
		ta.Delete() // the whole family
		if got := ta.Text(); got != "ab" {
			t.Fatalf("text = %q, want ab (family removed whole)", got)
		}
		if _, col := ta.CursorPos(); col != 1 {
			t.Fatalf("col = %d, want 1", col)
		}
	})

	t.Run("backspace removes a flag emoji whole", func(t *testing.T) {
		ta := newTextArea(t, "x"+flagEmoji)
		ta.SetCursor(0, 3)
		ta.Backspace()
		if got := ta.Text(); got != "x" {
			t.Fatalf("text = %q, want x (flag removed whole)", got)
		}
	})

	t.Run("e plus combining acute edits as one unit", func(t *testing.T) {
		ta := newTextArea(t, "caf"+accentedE)
		ta.SetCursor(0, 5)
		ta.Backspace()
		if got := ta.Text(); got != "caf" {
			t.Fatalf("text = %q, want caf (accented e removed whole)", got)
		}
	})

	t.Run("caret steps once per cluster", func(t *testing.T) {
		ta := newTextArea(t, "a"+familyEmoji+"b")
		ta.SetCursor(0, 0)
		for _, want := range []int{1, 6, 7} {
			ta.KeyAction(KeyRight, 0)
			if _, col := ta.CursorPos(); col != want {
				t.Fatalf("right col = %d, want %d", col, want)
			}
		}
		for _, want := range []int{6, 1, 0} {
			ta.KeyAction(KeyLeft, 0)
			if _, col := ta.CursorPos(); col != want {
				t.Fatalf("left col = %d, want %d", col, want)
			}
		}
	})

	t.Run("shift+arrow selection spans cluster boundaries exactly", func(t *testing.T) {
		ta := newTextArea(t, "a"+familyEmoji+"b")
		ta.SetCursor(0, 1)
		ta.KeyAction(KeyRight, ModShift)
		start, end, active := ta.Selection()
		if !active || start != (pos{0, 1}) || end != (pos{0, 6}) {
			t.Fatalf("selection = %v..%v active=%v, want 0:1..0:6 true", start, end, active)
		}
		if got, _ := ta.SelectedText(); got != familyEmoji {
			t.Fatalf("selected = %q, want the family whole", got)
		}
	})

	t.Run("a cluster delete is one undo entry", func(t *testing.T) {
		ta := newTextArea(t, "a"+familyEmoji+"b")
		ta.SetCursor(0, 6)
		ta.Backspace() // the family, whole
		if got := ta.Text(); got != "ab" {
			t.Fatalf("text = %q, want ab", got)
		}
		if !ta.Undo() {
			t.Fatal("undo must restore the family")
		}
		if got := ta.Text(); got != "a"+familyEmoji+"b" {
			t.Fatalf("after undo text = %q, want a+%q+b", got, familyEmoji)
		}
		if ta.Undo() {
			t.Fatal("a cluster delete is exactly one undo entry")
		}
	})

	t.Run("word motion composes: the accented cluster is one word unit", func(t *testing.T) {
		ta := newTextArea(t, "caf"+accentedE+" blues")
		ta.SetCursor(0, 0)
		ta.KeyAction(KeyRight, ModCtrl)
		if _, col := ta.CursorPos(); col != 5 {
			t.Fatalf("ctrl+right col = %d, want 5 (past the accented cluster)", col)
		}
		ta.KeyAction(KeyBackspace, ModCtrl)
		if got := ta.Text(); got != " blues" {
			t.Fatalf("text = %q, want ' blues'", got)
		}
	})

	t.Run("double-click selects the accented cluster's word", func(t *testing.T) {
		ta := newTextArea(t, "caf"+accentedE+" blues")
		sh := ta.face.Shape("caf"+accentedE+" blues", 14)
		ta.DoubleClickAt(Point{X: 8 + int(sh.CaretX(2)), Y: 6 + 7})
		if got, _ := ta.SelectedText(); got != "caf"+accentedE {
			t.Fatalf("selected = %q, want caf+accented cluster", got)
		}
	})
}
