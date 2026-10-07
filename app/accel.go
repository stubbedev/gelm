package app

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

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
	if a.Mods&wlsession.ModSuper != 0 {
		parts = append(parts, "Super")
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
	case "super", "mod4", "logo":
		return wlsession.ModSuper, nil
	default:
		return 0, fmt.Errorf("app: accel: unsupported modifier %q (shift, ctrl, alt, super)", name)
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

// Shortcut is a key sequence: one Accel, or a chord of several pressed
// in turn ("ctrl+x ctrl+s", "g g").
type Shortcut []Accel

// ParseShortcut parses a shortcut: accelerators (ParseAccel's forms)
// separated by spaces, each step pressed after the last.
func ParseShortcut(s string) (Shortcut, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, fmt.Errorf("app: accel %q: empty", s)
	}
	out := make(Shortcut, len(fields))
	for i, f := range fields {
		a, err := ParseAccel(f)
		if err != nil {
			return nil, err
		}
		out[i] = a
	}
	return out, nil
}

// String renders the canonical label: the steps' Accel labels joined
// by spaces ("Ctrl+X Ctrl+S").
func (s Shortcut) String() string {
	parts := make([]string, len(s))
	for i, a := range s {
		parts[i] = a.String()
	}
	return strings.Join(parts, " ")
}

// hasPrefix reports whether p is a (non-strict) prefix of s.
func (s Shortcut) hasPrefix(p Shortcut) bool {
	return len(p) <= len(s) && slices.Equal(s[:len(p)], p)
}

// AccelInfo is one registered binding, as Application.Accels reports
// it: the shortcut, and either the action an app-wide binding invokes
// or the widget a focus-scoped one belongs to.
type AccelInfo struct {
	Shortcut Shortcut
	Action   string        // app-wide bindings
	Widget   widget.Widget // focus-scoped bindings
}

// accelBinding is one registered shortcut: either an app-wide binding
// naming an action from the action table, or a widget-scoped binding
// that runs while exactly that widget holds keyboard focus.
type accelBinding struct {
	keys   Shortcut
	widget widget.Widget // nil for an app-wide binding
	action string        // app-wide bindings name an action
	run    func()        // widget-scoped bindings run directly
}

// chordTimeout is how long a chord waits for its next step.
const chordTimeout = 1500 * time.Millisecond

// accelNow is the chord clock; tests pin it.
var accelNow = time.Now

// accelTable is the application's accelerator map: shortcuts to
// bindings in registration order, the named actions app-wide bindings
// invoke, and the chord in progress.
type accelTable struct {
	actions map[string]func()
	// described holds the shortcuts overview's section and title per
	// action (DescribeAction).
	described map[string]actionDescription
	bound     []accelBinding
	// pending is the chord typed so far, since pendingAt.
	pending   Shortcut
	pendingAt time.Time
}

func newAccelTable() *accelTable {
	return &accelTable{actions: map[string]func(){}, described: map[string]actionDescription{}}
}

// addAction registers a named action; re-registering a name replaces
// its function.
func (t *accelTable) addAction(name string, fn func()) {
	t.actions[name] = fn
}

// actionDescription is how the shortcuts overview presents an action.
type actionDescription struct{ section, title string }

// sections groups the app-wide bindings for the shortcuts overview:
// sections in order of first appearance (undescribed actions under
// "General"), one item per binding titled by its description, else
// its action name.
func (t *accelTable) sections() []widget.ShortcutSection {
	var out []widget.ShortcutSection
	index := map[string]int{}
	for _, b := range t.bound {
		if b.widget != nil {
			continue // focus-scoped bindings are contextual
		}
		d := t.described[b.action]
		if d.section == "" {
			d.section = widget.Tr("General")
		}
		if d.title == "" {
			d.title = b.action
		}
		i, ok := index[d.section]
		if !ok {
			i = len(out)
			index[d.section] = i
			out = append(out, widget.ShortcutSection{Title: d.section})
		}
		out[i].Items = append(out[i].Items, widget.ShortcutItem{Title: d.title, Keys: b.keys.String()})
	}
	return out
}

// conflict reports the binding keys would be ambiguous with in the
// same scope: the same shortcut, or one a prefix of the other (a chord
// could never complete, or a single key would always wait).
func (t *accelTable) conflict(keys Shortcut, w widget.Widget) (accelBinding, bool) {
	for _, b := range t.bound {
		if b.widget == w && (b.keys.hasPrefix(keys) || keys.hasPrefix(b.keys)) {
			return b, true
		}
	}
	return accelBinding{}, false
}

// bindApp binds keys to a registered action. The conflict rule is an
// error at registration: the first binding keeps its keys, and a
// second one on the same shortcut - or on a prefix or extension of it
// - fails naming the incumbent, so re-binding never silently steals a
// key.
func (t *accelTable) bindApp(keys, action string) error {
	sc, err := ParseShortcut(keys)
	if err != nil {
		return err
	}
	if _, ok := t.actions[action]; !ok {
		return fmt.Errorf("app: accel %q: no action %q registered", keys, action)
	}
	if b, ok := t.conflict(sc, nil); ok {
		return fmt.Errorf("app: accel %q conflicts with %q, bound to action %q", keys, b.keys, b.action)
	}
	t.bound = append(t.bound, accelBinding{keys: sc, action: action})
	return nil
}

// bindWidget binds keys to fn while w itself holds keyboard focus.
// The same keys may bind per-widget on many widgets; a conflicting
// binding on the same widget errors like the app-wide rule.
func (t *accelTable) bindWidget(w widget.Widget, keys string, fn func()) error {
	if w == nil {
		return fmt.Errorf("app: accel %q: nil widget", keys)
	}
	if fn == nil {
		return fmt.Errorf("app: accel %q: nil handler", keys)
	}
	sc, err := ParseShortcut(keys)
	if err != nil {
		return err
	}
	if b, ok := t.conflict(sc, w); ok {
		return fmt.Errorf("app: accel %q conflicts with %q on this widget", keys, b.keys)
	}
	t.bound = append(t.bound, accelBinding{keys: sc, widget: w, run: fn})
	return nil
}

// unbind removes every binding on keys; error when none was bound.
func (t *accelTable) unbind(keys string) error {
	sc, err := ParseShortcut(keys)
	if err != nil {
		return err
	}
	n := len(t.bound)
	t.bound = slices.DeleteFunc(t.bound, func(b accelBinding) bool { return slices.Equal(b.keys, sc) })
	if len(t.bound) == n {
		return fmt.Errorf("app: accel %q is not bound", keys)
	}
	return nil
}

// infos lists the bindings in registration order.
func (t *accelTable) infos() []AccelInfo {
	out := make([]AccelInfo, len(t.bound))
	for i, b := range t.bound {
		out[i] = AccelInfo{Shortcut: slices.Clone(b.keys), Action: b.action, Widget: b.widget}
	}
	return out
}

// isModifierSym reports whether sym is a bare modifier key: pressing
// ctrl between ctrl+x and ctrl+s must not break the chord.
func isModifierSym(sym xkb.Keysym) bool {
	switch sym {
	case xkb.KeyShiftL, xkb.KeyShiftR, xkb.KeyControlL, xkb.KeyControlR,
		xkb.KeyAltL, xkb.KeyAltR, xkb.KeyMetaL, xkb.KeyMetaR,
		xkb.KeySuperL, xkb.KeySuperR, xkb.KeyCapsLock, xkb.KeyISOLevel3Shift:
		return true
	}
	return false
}

// fire runs the binding for sym+mods and reports whether the key was
// consumed. The key extends the chord in progress (dropped once it
// waited past chordTimeout): a complete shortcut fires, a prefix of
// one waits for the next step, and a key that matches neither ends
// the chord and is tried on its own. Bindings in scope are the
// focused widget's, which shadow the app-wide ones, then the app-wide
// ones; a consumed key never reaches text routing or widget actions.
func (t *accelTable) fire(router *widget.Router, sym xkb.Keysym, mods wlsession.Mods) bool {
	if t == nil || len(t.bound) == 0 || isModifierSym(sym) {
		return false
	}
	if len(t.pending) > 0 && accelNow().Sub(t.pendingAt) > chordTimeout {
		t.pending = nil
	}
	var focus widget.Widget
	if router != nil {
		focus = router.Focused()
	}
	key := Accel{Sym: normalizeSym(sym), Mods: mods}
	chord := append(slices.Clone(t.pending), key)
	t.pending = nil
	if t.step(chord, focus) {
		return true
	}
	if len(chord) > 1 {
		return t.step(Shortcut{key}, focus)
	}
	return false
}

// step resolves a chord against the bindings in scope: fires a
// complete match (the focused widget's first), arms a prefix, and
// reports whether either happened.
func (t *accelTable) step(chord Shortcut, focus widget.Widget) bool {
	prefix := false
	for _, scope := range [2]widget.Widget{focus, nil} {
		if scope == nil && focus != nil && prefix {
			break // the focused widget's chord in progress shadows
		}
		for _, b := range t.bound {
			if b.widget != scope {
				continue
			}
			switch {
			case slices.Equal(b.keys, chord):
				return t.run(b)
			case b.keys.hasPrefix(chord):
				prefix = true
			}
		}
		if focus == nil {
			break
		}
	}
	if prefix {
		t.pending, t.pendingAt = chord, accelNow()
	}
	return prefix
}

// run invokes a binding; false when its action is gone.
func (t *accelTable) run(b accelBinding) bool {
	if b.run != nil {
		b.run()
		return true
	}
	if fn, ok := t.actions[b.action]; ok {
		fn()
		return true
	}
	return false
}

// AddAction registers a named action that accelerators invoke.
// Registering a name again replaces its function; actions are
// independent of the accelerator table and may be shared by menu
// items and accels alike.
func (a *Application) AddAction(name string, fn func()) { a.accels.addAction(name, fn) }

// AddAccel binds a shortcut - an accelerator ("ctrl+q") or a chord of
// them pressed in turn ("ctrl+x ctrl+s", "g g") - to an action
// registered with AddAction. The conflict rule is an error at
// registration: the first binding keeps its keys, and a second one on
// the same shortcut, or on a prefix or extension of it, fails naming
// the incumbent instead of stealing the key. Malformed keys or an
// unknown action also error.
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

// DescribeAction sets how the shortcuts overview presents an action:
// the section it is listed under and its human title (the action name
// when empty).
func (a *Application) DescribeAction(name, section, title string) {
	a.accels.described[name] = actionDescription{section: section, title: title}
}

// ShortcutsDialog opens the shortcuts overview (GTK ShortcutsWindow):
// every app-wide binding of the registry, grouped by the sections
// DescribeAction gave their actions, keys drawn as keycaps. Esc
// closes it.
func (a *Application) ShortcutsDialog(parent *Window) (*Dialog, error) {
	face := a.resolveFace(nil)
	if face == nil {
		return nil, ErrNoDialogFace
	}
	return a.NewDialog(parent, DialogConfig{
		Title:          widget.Tr("Keyboard Shortcuts"),
		Width:          420,
		Height:         480,
		Content:        widget.NewShortcutsView(face, a.accels.sections()),
		Buttons:        []DialogButton{{Label: widget.Tr("Close"), Response: "close", Role: ButtonRoleCancel}},
		CancelResponse: "close",
	})
}

// Accels lists every binding in registration order, read-only (a
// copy): the registry a shortcuts window or a settings page renders.
func (a *Application) Accels() []AccelInfo { return a.accels.infos() }
