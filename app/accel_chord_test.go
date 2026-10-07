package app

import (
	"slices"
	"testing"
	"time"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/widget"
)

// pinAccelClock pins the chord clock for a test.
func pinAccelClock(t *testing.T) *time.Time {
	now := time.Unix(1750000000, 0)
	prev := accelNow
	accelNow = func() time.Time { return now }
	t.Cleanup(func() { accelNow = prev })
	return &now
}

// TestChords pins multi-key shortcuts: a chord fires after its last
// step, a bare modifier between steps does not break it, a stray key
// ends it and is tried alone, and a pause past the timeout drops it.
func TestChords(t *testing.T) {
	now := pinAccelClock(t)
	a := accelApp()
	var fired []string
	for _, n := range []string{"save", "top", "quit"} {
		a.AddAction(n, func() { fired = append(fired, n) })
	}
	for keys, action := range map[string]string{"ctrl+x ctrl+s": "save", "g g": "top", "q": "quit"} {
		if err := a.AddAccel(keys, action); err != nil {
			t.Fatal(err)
		}
	}
	press := func(sym xkb.Keysym, mods wlsession.Mods) bool { return a.accels.fire(nil, sym, mods) }

	if !press('x', wlsession.ModCtrl) || len(fired) != 0 {
		t.Fatal("the first step was not consumed and held")
	}
	press(xkb.KeyControlL, wlsession.ModCtrl)
	press('s', wlsession.ModCtrl)
	press('g', 0)
	press('q', 0) // ends the g chord, fires alone
	press('g', 0)
	*now = now.Add(2 * time.Second)
	press('g', 0) // the first g timed out: this one only arms
	press('g', 0)
	if want := []string{"save", "quit", "top"}; !slices.Equal(fired, want) {
		t.Errorf("fired %v, want %v", fired, want)
	}
	if press('z', 0) {
		t.Error("an unbound key was consumed")
	}
}

// TestChordConflicts pins the extended conflict rule: a shortcut that
// is a prefix or extension of a bound one, in the same scope, errors.
func TestChordConflicts(t *testing.T) {
	a := accelApp()
	a.AddAction("x", func() {})
	if err := a.AddAccel("ctrl+x ctrl+s", "x"); err != nil {
		t.Fatal(err)
	}
	for _, keys := range []string{"ctrl+x", "ctrl+x ctrl+s", "ctrl+x ctrl+s ctrl+a"} {
		if err := a.AddAccel(keys, "x"); err == nil {
			t.Errorf("%q bound despite the conflict", keys)
		}
	}
	w := widget.NewSpacer(1, 1)
	if err := a.AddWidgetAccel(w, "ctrl+x", func() {}); err != nil {
		t.Errorf("another scope conflicted: %v", err)
	}
	if err := a.AddAccel("ctrl+x ctrl+c", "x"); err != nil {
		t.Errorf("a sibling chord conflicted: %v", err)
	}
}

// TestAccelsIntrospection pins the read-only registry: every binding,
// in order, with its scope, as a copy.
func TestAccelsIntrospection(t *testing.T) {
	a := accelApp()
	a.AddAction("open", func() {})
	w := widget.NewSpacer(1, 1)
	_ = a.AddAccel("ctrl+o", "open")
	_ = a.AddWidgetAccel(w, "ctrl+Return", func() {})
	_ = a.AddAccel("g g", "open")
	infos := a.Accels()
	if len(infos) != 3 || infos[0].Action != "open" || infos[1].Widget != widget.Widget(w) || infos[2].Shortcut.String() != "G G" {
		t.Fatalf("Accels() = %+v", infos)
	}
	if infos[0].Shortcut.String() != "Ctrl+O" || infos[1].Shortcut.String() != "Ctrl+Return" {
		t.Errorf("labels %q %q", infos[0].Shortcut, infos[1].Shortcut)
	}
	infos[0].Shortcut[0].Sym = 'z'
	if a.Accels()[0].Shortcut.String() != "Ctrl+O" {
		t.Error("the registry is not a copy")
	}
}

// TestShortcutSections pins the overview's grouping: described actions
// under their sections in first-appearance order, the rest under
// General by action name, focus-scoped bindings left out.
func TestShortcutSections(t *testing.T) {
	a := accelApp()
	for _, n := range []string{"open", "save", "reload"} {
		a.AddAction(n, func() {})
	}
	a.DescribeAction("open", "Files", "Open a file")
	a.DescribeAction("save", "Files", "Save")
	_ = a.AddAccel("ctrl+o", "open")
	_ = a.AddAccel("F5", "reload")
	_ = a.AddAccel("ctrl+x ctrl+s", "save")
	_ = a.AddWidgetAccel(widget.NewSpacer(1, 1), "ctrl+Return", func() {})
	secs := a.accels.sections()
	if len(secs) != 2 || secs[0].Title != "Files" || secs[1].Title != "General" {
		t.Fatalf("sections = %+v", secs)
	}
	if got := secs[0].Items; len(got) != 2 || got[0].Title != "Open a file" || got[1].Keys != "Ctrl+X Ctrl+S" {
		t.Errorf("Files items = %+v", got)
	}
	if got := secs[1].Items; len(got) != 1 || got[0].Title != "reload" || got[0].Keys != "F5" {
		t.Errorf("General items = %+v", got)
	}
}
