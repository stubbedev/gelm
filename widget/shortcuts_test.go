package widget

import "testing"

// TestShortcutLabel pins the keycaps: one per key, chord steps apart.
func TestShortcutLabel(t *testing.T) {
	l := NewShortcutLabel(goldenFace(t), "Ctrl+X Ctrl+S")
	steps := l.Children()
	if len(steps) != 2 || len(steps[0].(*Box).Children()) != 2 {
		t.Fatalf("steps = %d", len(steps))
	}
	if k := steps[1].(*Box).Children()[1].(*Label); k.Text() != "S" || !k.hasClass("keycap") {
		t.Errorf("second step's key = %q", k.Text())
	}
}

// TestGoldenShortcutsView pins the overview.
func TestGoldenShortcutsView(t *testing.T) {
	v := NewShortcutsView(goldenFace(t), []ShortcutSection{
		{Title: "General", Items: []ShortcutItem{{Title: "Open", Keys: "Ctrl+O"}, {Title: "Command palette", Keys: "Ctrl+Shift+P"}}},
		{Title: "Editing", Items: []ShortcutItem{{Title: "Save", Keys: "Ctrl+X Ctrl+S"}, {Title: "Go to top", Keys: "G G"}}},
	})
	NewGolden(t, v, "shortcuts", goldenTheme(DarkTheme()), goldenFrame(360, 260))
}
