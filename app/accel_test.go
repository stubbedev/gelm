package app

import (
	"testing"

	"github.com/unxed/xkb-go"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// accelApp builds an application with just the accelerator table so
// tests exercise binding and routing without a session.
func accelApp() *Application {
	return &Application{accels: newAccelTable()}
}

// accelFixture is a focused entry plus a fake translator: press feeds
// one synthetic key press through the real routeKey path.
type accelFixture struct {
	app    *Application
	router *widget.Router
	entry  *widget.Entry
	tr     *fakeTranslator
}

func newAccelFixture(t *testing.T) *accelFixture {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	e := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	e.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 30})
	r := &widget.Router{Root: e}
	r.Press(widget.BTNLeft, widget.Point{X: 5, Y: 5})
	r.Release(widget.BTNLeft, widget.Point{X: 5, Y: 5})
	return &accelFixture{
		app:    accelApp(),
		router: r,
		entry:  e,
		tr:     &fakeTranslator{syms: map[uint32]xkb.Keysym{}, text: map[uint32]string{}},
	}
}

// press routes one keycode with mods and reports whether the app-level
// OnKey hook saw it.
func (f *accelFixture) press(code uint32, mods wlsession.Mods) (onKeySeen bool) {
	seen := false
	routeKey(f.tr, f.router, code, mods, nil, f.app.accels, func(*widget.Router, uint32, wlsession.Mods) {
		seen = true
	})
	return seen
}

func TestParseAccel(t *testing.T) {
	valid := []struct {
		in   string
		want Accel
	}{
		{"p", Accel{Sym: xkb.Keysym('p')}},
		{"ctrl+p", Accel{Sym: xkb.Keysym('p'), Mods: wlsession.ModCtrl}},
		{"Ctrl+P", Accel{Sym: xkb.Keysym('p'), Mods: wlsession.ModCtrl}},
		{"ctrl+shift+a", Accel{Sym: xkb.Keysym('a'), Mods: wlsession.ModCtrl | wlsession.ModShift}},
		{"Return", Accel{Sym: xkb.KeyReturn}},
		{"ctrl+enter", Accel{Sym: xkb.KeyReturn, Mods: wlsession.ModCtrl}},
		{"esc", Accel{Sym: xkb.KeyEscape}},
		{"del", Accel{Sym: xkb.KeyDelete}},
		{"F5", Accel{Sym: xkb.KeyF5}},
		{"space", Accel{Sym: xkb.Keysym(' ')}},
		{"1", Accel{Sym: xkb.Keysym('1')}},
		{"alt+Delete", Accel{Sym: xkb.KeyDelete, Mods: wlsession.ModAlt}},
		{"<Control>q", Accel{Sym: xkb.Keysym('q'), Mods: wlsession.ModCtrl}},
		{"<Control><Shift>p", Accel{Sym: xkb.Keysym('p'), Mods: wlsession.ModCtrl | wlsession.ModShift}},
	}
	for _, tc := range valid {
		got, err := ParseAccel(tc.in)
		if err != nil {
			t.Errorf("ParseAccel(%q) = %v, want %v", tc.in, err, tc.want)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseAccel(%q) = %v, want %v", tc.in, got, tc.want)
		}
		// The label form must parse back to the same binding: that is
		// what keeps widget.Menu Accel labels and bindings one format.
		if again, err := ParseAccel(got.String()); err != nil || again != got {
			t.Errorf("ParseAccel(%q.String() = %q) = (%v, %v), want round trip", tc.in, got, again, err)
		}
	}
	if got := (Accel{Sym: xkb.Keysym('p'), Mods: wlsession.ModCtrl}).String(); got != "Ctrl+P" {
		t.Errorf("label = %q, want Ctrl+P", got)
	}
	if got := (Accel{Sym: xkb.KeyReturn, Mods: wlsession.ModCtrl | wlsession.ModShift}).String(); got != "Ctrl+Shift+Return" {
		t.Errorf("label = %q, want Ctrl+Shift+Return", got)
	}
	for _, bad := range []string{"", "ctrl", "ctrl+", "ctrl+bogus", "super+q", "logo+q", "ctrl+shift", "<Control}q", "space+q"} {
		if got, err := ParseAccel(bad); err == nil {
			t.Errorf("ParseAccel(%q) = %v, want error", bad, got)
		}
	}
}

func TestAccelFiresBeforeTextRouting(t *testing.T) {
	f := newAccelFixture(t)
	f.tr.text[25], f.tr.syms[25] = "p", xkb.Keysym('p')
	fired := 0
	f.app.AddAction("print", func() { fired++ })
	if err := f.app.AddAccel("ctrl+p", "print"); err != nil {
		t.Fatal(err)
	}
	if onKey := f.press(25, wlsession.ModCtrl); !onKey {
		t.Error("OnKey did not observe the accel-consumed press")
	}
	if fired != 1 {
		t.Errorf("accel fired %d times, want 1", fired)
	}
	if got := f.entry.Text(); got != "" {
		t.Errorf("ctrl+P inserted %q into the entry, want nothing", got)
	}
	// The same key without ctrl is text routing: it must type, and the
	// accel must stay quiet.
	if onKey := f.press(25, 0); !onKey {
		t.Error("OnKey did not observe the plain press")
	}
	if fired != 1 {
		t.Errorf("plain p fired the accel (%d times), want no change", fired)
	}
	if got := f.entry.Text(); got != "p" {
		t.Errorf("text = %q, want p", got)
	}
}

func TestShiftIsTextCtrlShiftIsAccel(t *testing.T) {
	f := newAccelFixture(t)
	f.tr.syms[30] = xkb.Keysym('A') // shift re-keys the letter
	f.tr.text[30] = "A"
	fired := 0
	f.app.AddAction("mark", func() { fired++ })
	if err := f.app.AddAccel("ctrl+shift+a", "mark"); err != nil {
		t.Fatal(err)
	}
	// shift+a stays text: exact mods match, so the binding does not
	// claim it.
	f.press(30, wlsession.ModShift)
	if got := f.entry.Text(); got != "A" {
		t.Errorf("shift+a text = %q, want A", got)
	}
	if fired != 0 {
		t.Errorf("shift+a fired the accel %d times", fired)
	}
	// ctrl+shift+a is an accel candidate: the built-ins stand down with
	// shift held, the accel fires, and select-all does not.
	f.entry.SetText("abc")
	f.press(30, wlsession.ModCtrl|wlsession.ModShift)
	if fired != 1 {
		t.Errorf("ctrl+shift+a fired %d times, want 1", fired)
	}
	if _, _, active := f.entry.Selection(); active {
		t.Error("ctrl+shift+a select-alled despite the accel binding")
	}
	// Without the binding the same press stays inert (the built-in
	// select-all claims only plain ctrl+a).
	if err := f.app.RemoveAccel("ctrl+shift+a"); err != nil {
		t.Fatal(err)
	}
	f.press(30, wlsession.ModCtrl|wlsession.ModShift)
	if fired != 1 {
		t.Errorf("unbound ctrl+shift+a fired %d extra times", fired-1)
	}
}

func TestBuiltinsPrecedeAccels(t *testing.T) {
	f := newAccelFixture(t)
	f.tr.syms[30] = xkb.Keysym('a')
	fired := 0
	f.app.AddAction("mark", func() { fired++ })
	if err := f.app.AddAccel("ctrl+a", "mark"); err != nil {
		t.Fatal(err)
	}
	f.entry.SetText("abc")
	f.press(30, wlsession.ModCtrl)
	if start, end, active := f.entry.Selection(); !active || start != 0 || end != 3 {
		t.Errorf("selection = %d..%d active=%v, want built-in select-all 0..3", start, end, active)
	}
	if fired != 0 {
		t.Errorf("accel fired %d times despite the built-in", fired)
	}
}

func TestWidgetAccelFiresOnlyWhenFocused(t *testing.T) {
	f := newAccelFixture(t)
	other := newAccelFixture(t)
	f.tr.syms[28], other.tr.syms[28] = xkb.KeyReturn, xkb.KeyReturn
	widgetFired, appFired := 0, 0
	f.app.AddAction("app-act", func() { appFired++ })
	if err := f.app.AddWidgetAccel(f.entry, "ctrl+Return", func() { widgetFired++ }); err != nil {
		t.Fatal(err)
	}
	if err := f.app.AddAccel("ctrl+Return", "app-act"); err != nil {
		t.Fatal(err)
	}

	// In the entry: the per-widget binding shadows the app-wide one.
	f.press(28, wlsession.ModCtrl)
	if widgetFired != 1 || appFired != 0 {
		t.Errorf("focused: widget=%d app=%d, want widget=1 app=0", widgetFired, appFired)
	}
	// Focus elsewhere: only the app-wide binding fires. The press goes
	// through the same table but a router whose focus sits on the other
	// entry.
	routeKey(other.tr, other.router, 28, wlsession.ModCtrl, nil, f.app.accels,
		func(*widget.Router, uint32, wlsession.Mods) {})
	if widgetFired != 1 || appFired != 1 {
		t.Errorf("elsewhere: widget=%d app=%d, want widget=1 app=1", widgetFired, appFired)
	}
}

func TestAccelConflictIsRegistrationError(t *testing.T) {
	a := accelApp()
	quit, other := 0, 0
	a.AddAction("quit", func() { quit++ })
	a.AddAction("other", func() { other++ })
	if err := a.AddAccel("ctrl+q", "quit"); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	// A second binding on the same accelerator errors and the first
	// one keeps the key.
	if err := a.AddAccel("ctrl+q", "other"); err == nil {
		t.Error("conflicting AddAccel succeeded, want error")
	}
	if err := a.AddAccel("Ctrl+Q", "quit"); err == nil {
		t.Error("re-registration under the label form succeeded, want error")
	}
	if err := a.AddAccel("ctrl+w", "missing"); err == nil {
		t.Error("binding an unregistered action succeeded, want error")
	}
	if err := a.AddAccel("bogus", "quit"); err == nil {
		t.Error("binding malformed keys succeeded, want error")
	}
	r := newAccelFixture(t)
	r.tr.syms[24] = xkb.Keysym('q')
	// fire through the fixture's own table via routeKey: build a press
	// against the app under test.
	routeKey(r.tr, r.router, 24, wlsession.ModCtrl, nil, a.accels, func(*widget.Router, uint32, wlsession.Mods) {})
	if quit != 1 || other != 0 {
		t.Errorf("quit=%d other=%d, want the first binding to keep the key", quit, other)
	}
	// Widget-scoped: same keys on a different widget is fine; the same
	// widget again errors.
	if err := a.AddWidgetAccel(r.entry, "ctrl+q", func() {}); err != nil {
		t.Errorf("widget binding on distinct scope: %v", err)
	}
	if err := a.AddWidgetAccel(r.entry, "ctrl+q", func() {}); err == nil {
		t.Error("duplicate widget binding succeeded, want error")
	}
	if err := a.AddWidgetAccel(nil, "ctrl+q", func() {}); err == nil {
		t.Error("nil-widget binding succeeded, want error")
	}
	if err := a.AddWidgetAccel(r.entry, "ctrl+q", nil); err == nil {
		t.Error("nil-handler binding succeeded, want error")
	}
}

func TestRemoveAccel(t *testing.T) {
	f := newAccelFixture(t)
	f.tr.syms[24] = xkb.Keysym('q')
	fired := 0
	f.app.AddAction("quit", func() { fired++ })
	if err := f.app.AddAccel("ctrl+q", "quit"); err != nil {
		t.Fatal(err)
	}
	f.press(24, wlsession.ModCtrl)
	if fired != 1 {
		t.Fatalf("fired %d times before removal, want 1", fired)
	}
	if err := f.app.RemoveAccel("ctrl+q"); err != nil {
		t.Fatalf("RemoveAccel: %v", err)
	}
	f.press(24, wlsession.ModCtrl)
	if fired != 1 {
		t.Errorf("fired %d times after removal, want no change", fired)
	}
	if err := f.app.RemoveAccel("ctrl+q"); err == nil {
		t.Error("removing an unbound accelerator succeeded, want error")
	}
}
