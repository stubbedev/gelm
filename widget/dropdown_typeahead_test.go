package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/gelm/internal/animclock"
)

// typeClock steps past the type-ahead idle timeout without disturbing
// the rest of the schedule.
func typeClock(c *animClock, t *testing.T) {
	t.Helper()
	c.set(c.now().Add(dropdownTypeTimeout + time.Millisecond))
	animclock.Tick(c.now())
}

// TestDropdownTypeAheadOpen pins the open-list behavior (#62): a
// printable prefix jumps the highlight to the first matching row
// without selecting it, repeated keys cycle among equal prefixes,
// Enter picks the highlight, and the idle timeout resets the chain.
func TestDropdownTypeAheadOpen(t *testing.T) {
	c := pinAnimClock(t)
	dd := newDropdown(t, "apple", "apricot", "banana", "cherry", "Apple juice")
	var picked []int
	dd.OnSelect = func(i int) { picked = append(picked, i) }

	dd.Open()
	c.drive() // land the reveal
	if got := dd.menu.hovered; got != 0 {
		t.Fatalf("open highlight = %d, want the selected row 0", got)
	}

	// A prefix jumps to its first match: "ap" lands on apple (the
	// first ap- row), never touching the selection.
	dd.InsertRune('a')
	dd.InsertRune('p')
	if got := dd.menu.hovered; got != 0 {
		t.Fatalf("highlight after \"ap\" = %d, want 0 (apple)", got)
	}
	if dd.Selected() != 0 || len(picked) != 0 {
		t.Error("type-ahead changed the selection while open")
	}

	// Repeated 'a' cycles the equal-prefix rows: apple, apricot, Apple
	// juice, wrap to apple — each extra key resets onto the single
	// letter and advances past the current row.
	dd.InsertRune('a')
	if got := dd.menu.hovered; got != 1 {
		t.Fatalf("second a highlighted %d, want 1 (apricot)", got)
	}
	dd.InsertRune('a')
	if got := dd.menu.hovered; got != 4 {
		t.Errorf("third a highlighted %d, want 4 (Apple juice)", got)
	}
	dd.InsertRune('a')
	if got := dd.menu.hovered; got != 0 {
		t.Errorf("fourth a highlighted %d, want the wrap to 0", got)
	}
	dd.InsertRune('a')
	if got := dd.menu.hovered; got != 1 {
		t.Errorf("fifth a highlighted %d, want 1", got)
	}

	// Enter picks the highlighted row.
	dd.KeyAction(KeyEnter, 0)
	if dd.Selected() != 1 {
		t.Fatalf("selected = %d after Enter, want 1", dd.Selected())
	}
	c.drive()

	// Case-insensitive: "B" jumps to banana.
	dd.Open()
	c.drive()
	dd.InsertRune('B')
	if got := dd.menu.hovered; got != 2 {
		t.Errorf("highlight after \"B\" = %d, want 2 (banana)", got)
	}

	// The idle timeout clears the chain: a later key starts fresh.
	typeClock(c, t)
	if dd.typed != "" {
		t.Errorf("chain %q survived the idle timeout", dd.typed)
	}
	dd.InsertRune('a')
	if got := dd.menu.hovered; got != 0 {
		t.Errorf("post-timeout a highlighted %d, want a fresh 0", got)
	}
}

// TestDropdownTypeAheadClosed pins the closed-face decision: printable
// keys first-letter-cycle the selection (firing OnSelect), while Space
// still only opens.
func TestDropdownTypeAheadClosed(t *testing.T) {
	c := pinAnimClock(t)
	dd := newDropdown(t, "apple", "apricot", "banana")
	var picked []int
	dd.OnSelect = func(i int) { picked = append(picked, i) }

	// First-letter cycling moves the selection on the closed face.
	dd.InsertRune('b')
	if dd.Selected() != 2 {
		t.Fatalf("closed b selected %d, want 2", dd.Selected())
	}
	if len(picked) != 1 {
		t.Errorf("closed cycling fired OnSelect %d times, want 1", len(picked))
	}

	// A different letter starts a fresh chain only after the idle
	// timeout; before it, the letters extend the running prefix.
	dd.InsertRune('a')
	if dd.Selected() != 2 {
		t.Errorf("ba extended the prefix onto banana's row: selected %d, want 2", dd.Selected())
	}
	typeClock(c, t)
	dd.InsertRune('a')
	if dd.Selected() != 0 {
		t.Errorf("fresh a selected %d, want 0", dd.Selected())
	}
	dd.InsertRune('a')
	if dd.Selected() != 1 {
		t.Errorf("repeated closed a selected %d, want cycling to 1", dd.Selected())
	}

	// Space never joins the chain: closed, it opens.
	typeClock(c, t)
	dd.InsertRune(' ')
	if !dd.Opened() {
		t.Fatal("Space stopped opening the closed dropdown")
	}
	if dd.typed != "" {
		t.Errorf("Space joined the type-ahead chain: %q", dd.typed)
	}
	c.drive()
}

// TestDropdownTypeAheadUnmatchedKeysPassThrough pins that a key with
// no matching rows interferes with nothing: the chain, the highlight,
// and the selection stay put, and the navigation keys keep working.
func TestDropdownTypeAheadUnmatchedKeysPassThrough(t *testing.T) {
	c := pinAnimClock(t)
	dd := newDropdown(t, "apple", "banana")
	dd.Open()
	c.drive()

	dd.InsertRune('q')
	if dd.typed != "" || dd.Selected() != 0 {
		t.Error("an unmatched key left state behind")
	}
	if dd.menu.hovered != 0 {
		t.Errorf("unmatched key moved the highlight to %d", dd.menu.hovered)
	}
	dd.KeyAction(KeyDown, 0)
	if dd.menu.hovered != 1 {
		t.Fatalf("arrows stopped navigating after an unmatched key: highlight %d", dd.menu.hovered)
	}
	dd.KeyAction(KeyDismiss, 0)
	c.drive()
	if dd.Opened() {
		t.Error("Esc stopped closing the list")
	}

	// Space while closed still opens (the router's text path lives).
	dd.InsertRune(' ')
	if !dd.Opened() {
		t.Error("Space-opens broke after unmatched type-ahead")
	}
	c.drive()
}
