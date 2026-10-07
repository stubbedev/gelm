package widget

import "testing"

// TestEditableLabel pins the edit cycle: at rest keys are ignored and
// the field is read-only; a click starts editing with the text
// selected; Enter commits (OnChanged once), Escape reverts, losing
// focus commits.
func TestEditableLabel(t *testing.T) {
	face := chromeFace(t)
	l := NewEditableLabel(face, 14, DarkTheme().Text, "Title")
	var changes []string
	l.OnChanged = func(s string) { changes = append(changes, s) }
	l.InsertRune('x')
	l.KeyAction(KeyBackspace, 0)
	if l.Editing() || l.Text() != "Title" || !l.ReadOnly() || !l.hasClass("flat") {
		t.Fatalf("resting label edited or styled wrong: editing=%v text=%q", l.Editing(), l.Text())
	}

	l.ClickAt(Point{})
	if !l.Editing() || l.ReadOnly() {
		t.Fatal("a click did not start editing")
	}
	if s, ok := l.SelectedText(); !ok || s != "Title" {
		t.Errorf("editing selected %q, want the whole text", s)
	}
	l.InsertRune('N')
	l.KeyAction(KeyEnter, 0)
	if l.Editing() || l.Text() != "N" || len(changes) != 1 || changes[0] != "N" {
		t.Errorf("enter: editing=%v text=%q changes=%v", l.Editing(), l.Text(), changes)
	}

	l.KeyAction(KeyEnter, 0) // Enter at rest starts editing
	l.InsertRune('Z')
	l.KeyAction(KeyDismiss, 0)
	if l.Editing() || l.Text() != "N" || len(changes) != 1 {
		t.Errorf("escape: editing=%v text=%q changes=%v", l.Editing(), l.Text(), changes)
	}

	l.StartEditing()
	l.InsertRune('Q')
	l.onFocus(false)
	if l.Editing() || l.Text() != "Q" || len(changes) != 2 {
		t.Errorf("blur: editing=%v text=%q changes=%v", l.Editing(), l.Text(), changes)
	}
}

// TestGoldenEditableLabel pins the resting look (plain text) beside
// the editing one (the entry field).
func TestGoldenEditableLabel(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	rest := NewEditableLabel(face, 14, th.Text, "Resting")
	edit := NewEditableLabel(face, 14, th.Text, "Editing")
	edit.StartEditing()
	col := NewBox(Column, 8, 0)
	col.Append(rest, false)
	col.Append(edit, false)
	NewGolden(t, col, "editablelabel", goldenTheme(th), goldenFrame(160, 80))
}
