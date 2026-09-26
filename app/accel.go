package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/widget"
)

// Accel identifies one accelerator: a keysym plus the modifiers that
// must be held. It is the key of the application's accelerator table.
// Letter keysyms are normalized to their lowercase form on both sides
// (parse and lookup), because shift changes the keysym a key produces
// and not just the modifier bits: that keeps shift+a text while
// letting "ctrl+P" and a ctrl+shift+p press name one binding.
type Accel struct {
	Sym  xkb.Keysym
	Mods wlsession.Mods
}

// ParseAccel parses an accelerator string: modifier names and one key
// name joined with "+" ("ctrl+p", "Ctrl+Shift+Return", "F5"), or in
// GTK's angle-bracket form ("<Control>p", "<Control><Shift>p").
// Modifiers (case-insensitive) are ctrl/control, shift, and alt/mod1;
// super/logo and friends are rejected because gelm tracks no bit for
// them. Key names are XKB keysym names ("p", "Return", "Escape",
// "F5", "space") plus the aliases esc, enter, del, bksp, ins, pgup,
// and pgdn. widget.Menu paints Accel labels from the same strings, so
// Accel.String output feeds a menu item unchanged.
func ParseAccel(s string) (Accel, error) {
	mods, key, err := parseAccelMods(s)
	if err != nil {
		return Accel{}, err
	}
	sym, err := accelSym(key)
	if err != nil {
		return Accel{}, err
	}
	return Accel{Sym: sym, Mods: mods}, nil
}

// String renders the canonical menu-label form: "Ctrl+Shift+P",
// "Ctrl+Return", "Esc".
func (a Accel) String() string {
	var parts []string
	if a.Mods&wlsession.ModCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if a.Mods&wlsession.ModShift != 0 {
		parts = append(parts, "Shift")
	}
	if a.Mods&wlsession.ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	name := xkb.KeysymGetName(a.Sym)
	if name == "" {
		name = "0x" + strconv.FormatUint(uint64(a.Sym), 16)
	}
	if len(name) == 1 {
		name = strings.ToUpper(name)
	}
	return strings.Join(append(parts, name), "+")
}

// parseAccelMods peels the modifier prefix off an accelerator string
// and returns the mods with the remaining key name.
func parseAccelMods(s string) (wlsession.Mods, string, error) {
	s = strings.TrimSpace(s)
	var mods wlsession.Mods
	if !strings.Contains(s, "<") {
		parts := strings.Split(s, "+")
		for _, part := range parts[:len(parts)-1] {
			bit, err := accelMod(part)
			if err != nil {
				return 0, "", err
			}
			mods |= bit
		}
		return mods, parts[len(parts)-1], nil
	}
	for {
		lt := strings.Index(s, "<")
		if lt < 0 {
			break
		}
		gt := strings.Index(s[lt:], ">")
		if gt < 0 {
			return 0, "", fmt.Errorf("app: accel %q: unclosed <modifier>", s)
		}
		bit, err := accelMod(s[lt+1 : lt+gt])
		if err != nil {
			return 0, "", err
		}
		mods |= bit
		s = s[lt+gt+1:]
	}
	return mods, strings.TrimSpace(s), nil
}

// accelMod resolves one modifier name to its bit.
func accelMod(name string) (wlsession.Mods, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ctrl", "control":
		return wlsession.ModCtrl, nil
	case "shift":
		return wlsession.ModShift, nil
	case "alt", "mod1":
		return wlsession.ModAlt, nil
	default:
		return 0, fmt.Errorf("app: accel: unsupported modifier %q (shift, ctrl, alt)", name)
	}
}

// accelKeyAliases maps friendly names onto XKB keysym names.
var accelKeyAliases = map[string]string{
	"esc":   "Escape",
	"enter": "Return",
	"del":   "Delete",
	"bksp":  "BackSpace",
	"ins":   "Insert",
	"pgup":  "Prior",
	"pgdn":  "Next",
}

// accelSym resolves a key name to its normalized keysym.
func accelSym(name string) (xkb.Keysym, error) {
	n := strings.ToLower(strings.TrimSpace(name))
	if alias, ok := accelKeyAliases[n]; ok {
		n = alias
	}
	sym := xkb.KeysymFromName(n, xkb.KeysymNameCaseInsensitive)
	if sym == xkb.KeyNoSymbol {
		return 0, fmt.Errorf("app: accel: unknown key %q", name)
	}
	return normalizeSym(sym), nil
}

// normalizeSym folds uppercase ASCII letter keysyms onto their
// lowercase letter, so a binding names the same key whether the user
// wrote "p", "P", or pressed it with shift held.
func normalizeSym(sym xkb.Keysym) xkb.Keysym {
	if sym >= 0x41 && sym <= 0x5a {
		return sym + 0x20
	}
	return sym
}

// accelBinding is one registered accelerator: either an app-wide
// binding naming an action from the action table, or a widget-scoped
// binding that runs while exactly that widget holds keyboard focus.
type accelBinding struct {
	widget widget.Widget // nil for an app-wide binding
	action string        // app-wide bindings name an action
	run    func()        // widget-scoped bindings run directly
}

// accelTable is the application's accelerator map: keysym+mods to
// bindings, plus the named actions app-wide bindings invoke.
type accelTable struct {
	actions map[string]func()
	bound   map[Accel][]accelBinding
}

func newAccelTable() *accelTable {
	return &accelTable{
		actions: map[string]func(){},
		bound:   map[Accel][]accelBinding{},
	}
}

// addAction registers a named action; re-registering a name replaces
// its function.
func (t *accelTable) addAction(name string, fn func()) {
	t.actions[name] = fn
}

// bindApp binds keys to a registered action. The conflict rule is an
// error at registration: the first binding on an accelerator keeps it
// and a second one fails naming the incumbent, so re-binding never
// silently steals a key.
func (t *accelTable) bindApp(keys, action string) error {
	key, err := ParseAccel(keys)
	if err != nil {
		return err
	}
	if _, ok := t.actions[action]; !ok {
		return fmt.Errorf("app: accel %q: no action %q registered", keys, action)
	}
	for _, b := range t.bound[key] {
		if b.widget == nil {
			return fmt.Errorf("app: accel %q already bound to action %q", keys, b.action)
		}
	}
	t.bound[key] = append(t.bound[key], accelBinding{action: action})
	return nil
}

// bindWidget binds keys to fn while w itself holds keyboard focus.
// The same keys may bind per-widget on many widgets; re-binding the
// same widget errors like the app-wide rule.
func (t *accelTable) bindWidget(w widget.Widget, keys string, fn func()) error {
	if w == nil {
		return fmt.Errorf("app: accel %q: nil widget", keys)
	}
	if fn == nil {
		return fmt.Errorf("app: accel %q: nil handler", keys)
	}
	key, err := ParseAccel(keys)
	if err != nil {
		return err
	}
	for _, b := range t.bound[key] {
		if b.widget == w {
			return fmt.Errorf("app: accel %q already bound on this widget", keys)
		}
	}
	t.bound[key] = append(t.bound[key], accelBinding{widget: w, run: fn})
	return nil
}

// unbind removes every binding on keys; error when none was bound.
func (t *accelTable) unbind(keys string) error {
	key, err := ParseAccel(keys)
	if err != nil {
		return err
	}
	if len(t.bound[key]) == 0 {
		return fmt.Errorf("app: accel %q is not bound", keys)
	}
	delete(t.bound, key)
	return nil
}

// fire runs the binding for sym+mods and reports whether one fired.
// Widget-scoped bindings for the focused widget shadow the app-wide
// binding on the same keys; a fired accelerator consumes the event,
// so text routing and widget actions never see it.
func (t *accelTable) fire(router *widget.Router, sym xkb.Keysym, mods wlsession.Mods) bool {
	if t == nil || len(t.bound) == 0 {
		return false
	}
	bs, ok := t.bound[Accel{Sym: normalizeSym(sym), Mods: mods}]
	if !ok {
		return false
	}
	if router != nil {
		if f := router.Focused(); f != nil {
			for _, b := range bs {
				if b.widget == f && b.run != nil {
					b.run()
					return true
				}
			}
		}
	}
	for _, b := range bs {
		if b.widget == nil {
			if fn, ok := t.actions[b.action]; ok {
				fn()
				return true
			}
		}
	}
	return false
}

// AddAction registers a named action that accelerators invoke.
// Registering a name again replaces its function; actions are
// independent of the accelerator table and may be shared by menu
// items and accels alike.
func (a *Application) AddAction(name string, fn func()) { a.accels.addAction(name, fn) }

// AddAccel binds an accelerator ("ctrl+q") to an action registered
// with AddAction. The conflict rule is an error at registration: the
// first binding on an accelerator keeps it, and a second AddAccel for
// the same keysym+mods fails naming the incumbent instead of stealing
// the key. Malformed keys or an unknown action also error.
func (a *Application) AddAccel(keys, action string) error { return a.accels.bindApp(keys, action) }

// AddWidgetAccel binds keys to fn in the focused widget's context:
// the accelerator fires only while w itself holds keyboard focus and
// then shadows an app-wide binding on the same keys (e.g.
// ctrl+Return inside one entry). Binding the same keys twice on the
// same widget errors, like AddAccel.
func (a *Application) AddWidgetAccel(w widget.Widget, keys string, fn func()) error {
	return a.accels.bindWidget(w, keys, fn)
}

// RemoveAccel unbinds keys whatever their scope. Unbinding keys that
// are not bound errors.
func (a *Application) RemoveAccel(keys string) error { return a.accels.unbind(keys) }
