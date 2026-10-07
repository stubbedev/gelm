//go:build atspi

// The D-Bus object surface: the AT-SPI interface handlers exported per
// accessible object. Each interface is a small struct with the
// interface's methods (godbus's ExportAll path — the map form of
// Export does not dispatch in this godbus version, which the
// appearance tests already note). Every handler reads the published
// snapshot under the bridge mutex — no widget is ever touched from the
// D-Bus handler goroutine — and the actions that must reach widgets
// (DoAction, SetCurrentValue) hop through Scene.Invoke onto the loop.
package atspi

import (
	"github.com/godbus/dbus/v5"

	"github.com/stubbedev/gelm/widget"
)

// objRef is the (so) object reference: a bus name and an object path.
type objRef struct {
	Name string
	Path dbus.ObjectPath
}

// atspiRect is the (iiii) extents tuple.
type atspiRect struct {
	X, Y, W, H int32
}

// Interface names.
const (
	ifaceAccessible     = "org.a11y.atspi.Accessible"
	ifaceApplication    = "org.a11y.atspi.Application"
	ifaceComponent      = "org.a11y.atspi.Component"
	ifaceText           = "org.a11y.atspi.Text"
	ifaceAction         = "org.a11y.atspi.Action"
	ifaceValue          = "org.a11y.atspi.Value"
	ifaceProperties     = "org.freedesktop.DBus.Properties"
	ifaceIntrospectable = "org.freedesktop.DBus.Introspectable"
)

// nullPath is the AT-SPI null object reference's path.
const nullPath dbus.ObjectPath = "/org/a11y/atspi/null"

// desktopRef is the registry's desktop object — the AT-SPI tree root
// above applications; the application root's parent.
var desktopRef = objRef{Name: "org.a11y.atspi.Registry", Path: "/org/a11y/atspi/accessible/0"}

// exportNode wires one object's interfaces on the bus. id 0 is the
// application root; text/action/value select the subinterfaces; app
// adds the Application interface (root only). Called under the bridge
// mutex (exportNew) or from Serve.
func (b *Bridge) exportNode(id int32, text, action, value, app bool) {
	path := RootPath
	if id != 0 {
		path = widgetPath(id)
	}
	mustExport(b, path, ifaceAccessible, &accessibleIface{b: b, id: id})
	mustExport(b, path, ifaceComponent, &componentIface{b: b, id: id})
	if text {
		mustExport(b, path, ifaceText, &textIface{b: b, id: id})
	}
	if action {
		mustExport(b, path, ifaceAction, &actionIface{b: b, id: id})
	}
	if value {
		mustExport(b, path, ifaceValue, &valueIface{b: b, id: id})
	}
	if app {
		mustExport(b, path, ifaceApplication, &applicationIface{b: b})
	}
	mustExport(b, path, ifaceProperties, &propertiesIface{b: b, id: id})
	mustExport(b, path, ifaceIntrospectable, &introspectIface{text: text, action: action, value: value, app: app})
}

// mustExport is ExportAll with the error fatal — a table that fails to
// export means the bridge is broken, not degraded.
func mustExport(b *Bridge, path dbus.ObjectPath, iface string, obj any) {
	if err := b.conn.ExportAll(obj, path, iface); err != nil {
		panic("atspi: export " + iface + " on " + string(path) + ": " + err.Error())
	}
}

// accessibleIface is org.a11y.atspi.Accessible.
type accessibleIface struct {
	b  *Bridge
	id int32
}

func (o *accessibleIface) GetChildAtIndex(index int32) objRef { return o.b.childAt(o.id, index) }
func (o *accessibleIface) GetChildren() []objRef              { return o.b.childrenOf(o.id) }
func (o *accessibleIface) GetIndexInParent() int32            { return o.b.indexInParent(o.id) }
func (o *accessibleIface) GetRelationSet() []any              { return []any{} }
func (o *accessibleIface) GetRole() uint32                    { return o.b.roleOf(o.id) }
func (o *accessibleIface) GetRoleName() string                { return o.b.roleNameOf(o.id) }
func (o *accessibleIface) GetLocalizedRoleName() string       { return o.b.roleNameOf(o.id) }
func (o *accessibleIface) GetState() []uint32                 { return o.b.stateOf(o.id) }
func (o *accessibleIface) GetAttributes() map[string]string   { return map[string]string{} }
func (o *accessibleIface) GetApplication() objRef             { return objRef{Name: o.b.name, Path: RootPath} }

func (o *accessibleIface) GetInterfaces() []string {
	return o.b.interfacesOf(o.id)
}

// componentIface is org.a11y.atspi.Component — extents and hit
// testing. Coordinates are window-relative: the semantic model carries
// arranged rects and the compositor owns the window's screen position,
// so a screen coord_type returns the same window-local values
// (documented limitation: review cursors and magnifiers read positions
// relative to the gelm window, not the desktop).
type componentIface struct {
	b  *Bridge
	id int32
}

func (o *componentIface) Contains(x, y int32, coordType uint32) bool {
	n := o.b.node(o.id)
	return n != nil && n.st.Bounds.Contains(int(x), int(y))
}

func (o *componentIface) GetAccessibleAtPoint(x, y int32, coordType uint32) objRef {
	return o.b.accessibleAtPoint(o.id, int(x), int(y))
}

func (o *componentIface) GetExtents(coordType uint32) atspiRect {
	if n := o.b.node(o.id); n != nil {
		r := n.st.Bounds
		return atspiRect{X: int32(r.X), Y: int32(r.Y), W: int32(r.W), H: int32(r.H)}
	}
	return atspiRect{}
}

func (o *componentIface) GetPosition(coordType uint32) (int32, int32) {
	if n := o.b.node(o.id); n != nil {
		return int32(n.st.Bounds.X), int32(n.st.Bounds.Y)
	}
	return 0, 0
}

func (o *componentIface) GetSize() (int32, int32) {
	if n := o.b.node(o.id); n != nil {
		return int32(n.st.Bounds.W), int32(n.st.Bounds.H)
	}
	return 0, 0
}

func (o *componentIface) GetLayer() uint32    { return 3 } // WIDGET
func (o *componentIface) GetMDIZOrder() int16 { return -1 }

// GrabFocus is honestly false: the semantic model is read-only and
// gelm exposes no programmatic focus setter — focus belongs to the
// Router's input path.
func (o *componentIface) GrabFocus() bool                                    { return false }
func (o *componentIface) GetAlpha() float64                                  { return 1.0 }
func (o *componentIface) SetExtents(x, y, w, h int32, coordType uint32) bool { return false }
func (o *componentIface) SetPosition(x, y int32, coordType uint32) bool      { return false }
func (o *componentIface) SetSize(w, h int32) bool                            { return false }
func (o *componentIface) ScrollTo(scrollType uint32) bool                    { return false }
func (o *componentIface) ScrollToPoint(coordType uint32, x, y int32) bool    { return false }

// textIface is org.a11y.atspi.Text for text-bearing roles. The rune
// offsets are the semantic model's. The caret and selection setters
// are honestly false — gelm exposes no programmatic caret or selection
// API (the semantic model is deliberately read-only), and a read-only
// Text is a well-formed AT-SPI citizen.
type textIface struct {
	b  *Bridge
	id int32
}

func (o *textIface) snapshot() (string, widget.A11yState) {
	n := o.b.node(o.id)
	if n == nil {
		return "", widget.A11yState{}
	}
	return n.st.Text, n.st
}

func (o *textIface) runes() []rune {
	t, _ := o.snapshot()
	return []rune(t)
}

func (o *textIface) bound(offset, kind int32) (string, int32, int32) {
	return textBoundary(o.runes(), offset, kind)
}

func (o *textIface) GetCaretOffset() int32 {
	_, st := o.snapshot()
	return int32(st.Caret)
}

func (o *textIface) GetCharacterCount() int32 {
	return int32(len(o.runes()))
}

func (o *textIface) GetText(start, end int32) string {
	r := o.runes()
	start = max32(0, min32(start, int32(len(r))))
	end = max32(start, min32(end, int32(len(r))))
	return string(r[start:end])
}

func (o *textIface) GetCharacterAtOffset(offset int32) int32 {
	r := o.runes()
	if offset < 0 || offset >= int32(len(r)) {
		return -1
	}
	return int32(r[offset])
}

func (o *textIface) GetNSelections() int32 {
	_, st := o.snapshot()
	if st.HasSelection {
		return 1
	}
	return 0
}

func (o *textIface) GetSelection(num int32) (int32, int32) {
	_, st := o.snapshot()
	if num != 0 || !st.HasSelection {
		return -1, -1
	}
	return int32(st.SelStart), int32(st.SelEnd)
}

func (o *textIface) GetStringAtOffset(offset, granularity uint32) (string, int32, int32) {
	off := int32(offset)
	r := o.runes()
	switch granularity {
	case 0: // CHAR
		if off < 0 || off >= int32(len(r)) {
			return "", off, off
		}
		return string(r[off]), off, off + 1
	case 1: // WORD
		return wordAt(r, off)
	case 2: // SENTENCE
		return sentenceAt(r, off)
	case 3: // LINE
		return lineAt(r, off)
	default: // PARAGRAPH: gelm text is a single paragraph
		return string(r), 0, int32(len(r))
	}
}

func (o *textIface) GetTextBeforeOffset(offset, kind uint32) (string, int32, int32) {
	s, start, end := o.bound(int32(offset), int32(kind))
	if start >= int32(offset) {
		return "", int32(offset), int32(offset)
	}
	return s, start, min32(end, int32(offset))
}

func (o *textIface) GetTextAtOffset(offset, kind uint32) (string, int32, int32) {
	return o.bound(int32(offset), int32(kind))
}

func (o *textIface) GetTextAfterOffset(offset, kind uint32) (string, int32, int32) {
	s, start, end := o.bound(int32(offset), int32(kind))
	if end <= int32(offset) {
		return "", int32(offset), int32(offset)
	}
	return s, max32(start, int32(offset)), end
}

func (o *textIface) GetAttributeValue(offset int32, name string) string { return "" }

func (o *textIface) GetAttributes(offset int32) (map[string]string, int32, int32) {
	return map[string]string{}, offset, int32(len(o.runes()))
}

func (o *textIface) GetAttributeRun(offset int32, includeDefaults bool) (map[string]string, int32, int32) {
	return map[string]string{}, 0, int32(len(o.runes()))
}

func (o *textIface) GetDefaultAttributes() map[string]string   { return map[string]string{} }
func (o *textIface) GetDefaultAttributeSet() map[string]string { return map[string]string{} }

// Per-glyph geometry is not in the semantic model (it is shaping-time
// state), so the extent and point probes report nothing rather than
// guessing.
func (o *textIface) GetCharacterExtents(offset int32, coordType uint32) (int32, int32, int32, int32) {
	return 0, 0, 0, 0
}

func (o *textIface) GetOffsetAtPoint(x, y int32, coordType uint32) int32 { return -1 }

func (o *textIface) GetRangeExtents(start, end int32, coordType uint32) (int32, int32, int32, int32) {
	return 0, 0, 0, 0
}

func (o *textIface) GetBoundedRanges(x, y, w, h int32, coordType, xClip, yClip uint32) []any {
	return []any{}
}

func (o *textIface) SetCaretOffset(offset int32) bool                           { return false }
func (o *textIface) AddSelection(start, end int32) bool                         { return false }
func (o *textIface) RemoveSelection(num int32) bool                             { return false }
func (o *textIface) SetSelection(num, start, end int32) bool                    { return false }
func (o *textIface) ScrollSubstringTo(start, end int32, scrollType uint32) bool { return false }
func (o *textIface) ScrollSubstringToPoint(start, end int32, coordType uint32, x, y int32) bool {
	return false
}

// actionIface is org.a11y.atspi.Action for activatable controls: one
// action, the click, performed through the widget's own Clicker path
// on the loop goroutine — the same entry point a pointer release
// takes.
type actionIface struct {
	b  *Bridge
	id int32
}

func (o *actionIface) GetNActions() int32                  { return 1 }
func (o *actionIface) GetDescription(index int32) string   { return "Activate the control" }
func (o *actionIface) GetName(index int32) string          { return "click" }
func (o *actionIface) GetLocalizedName(index int32) string { return "click" }
func (o *actionIface) GetKeyBinding(index int32) string    { return "" }

// actionTuple is one (localized name, description, keybinding).
type actionTuple struct {
	Name, Description, KeyBinding string
}

func (o *actionIface) GetActions() []actionTuple {
	return []actionTuple{{Name: "click", Description: "Activate the control"}}
}

func (o *actionIface) DoAction(index int32) bool {
	if index != 0 {
		return false
	}
	n := o.b.node(o.id)
	if n == nil || !n.st.Enabled {
		return false
	}
	w := n.w
	center := widget.Point{
		X: n.st.Bounds.X + n.st.Bounds.W/2,
		Y: n.st.Bounds.Y + n.st.Bounds.H/2,
	}
	o.b.scene.Invoke(func() {
		// The router's full press-release: a widget may act on the
		// press (wayle's bar toggle opens its dropdown there), the
		// release belongs to the Clicker when there is one.
		if p, ok := w.(widget.PressSetter); ok {
			p.SetPressed(true)
			p.SetPressed(false)
		}
		if c, ok := w.(widget.Clicker); ok {
			c.ClickAt(center)
		}
	})
	return true
}

// valueIface is org.a11y.atspi.Value for the ranged roles; the
// current-value setter applies through the widget where the widget has
// one (sliders do; progress bars are display-only).
type valueIface struct {
	b  *Bridge
	id int32
}

func (o *valueIface) GetMinimumValue() float64 {
	if n := o.b.node(o.id); n != nil {
		return n.st.Min
	}
	return 0
}

func (o *valueIface) GetMaximumValue() float64 {
	if n := o.b.node(o.id); n != nil {
		return n.st.Max
	}
	return 0
}

func (o *valueIface) GetMinimumIncrement() float64 {
	if n := o.b.node(o.id); n != nil {
		return n.st.Step
	}
	return 0
}

func (o *valueIface) GetCurrentValue() float64 {
	if n := o.b.node(o.id); n != nil {
		return n.st.Value
	}
	return 0
}

func (o *valueIface) GetText() string { return "" }

func (o *valueIface) SetCurrentValue(v float64) bool {
	n := o.b.node(o.id)
	if n == nil || n.st.Role != widget.RoleSlider {
		return false
	}
	w := n.w
	o.b.scene.Invoke(func() {
		if s, ok := w.(*widget.Slider); ok {
			s.SetValue(v)
		}
	})
	return true
}

// applicationIface is org.a11y.atspi.Application on the root: the
// toolkit identity the registry and ATs read during the handshake.
type applicationIface struct {
	b *Bridge
}

func (o *applicationIface) GetToolkitName() string           { return "gelm" }
func (o *applicationIface) GetVersion() string               { return toolkitVersion }
func (o *applicationIface) GetAtspiVersion() string          { return "2.1" }
func (o *applicationIface) GetLocale(lctype uint32) string   { return "" }
func (o *applicationIface) GetApplicationBusAddress() string { return o.b.addr }

// propertiesIface is org.freedesktop.DBus.Properties.
type propertiesIface struct {
	b  *Bridge
	id int32
}

func (o *propertiesIface) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	v, ok := o.b.property(o.id, iface, prop)
	if !ok {
		return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.InvalidArgs",
			[]any{"unknown property " + iface + "." + prop})
	}
	return v, nil
}

func (o *propertiesIface) GetAll(iface string) map[string]dbus.Variant {
	return o.b.properties(o.id, iface)
}

func (o *propertiesIface) Set(iface, prop string, val dbus.Variant) *dbus.Error {
	if iface == ifaceApplication && prop == "Id" {
		if v, ok := val.Value().(int32); ok {
			o.b.mu.Lock()
			o.b.appID = v
			o.b.mu.Unlock()
			return nil
		}
	}
	return dbus.NewError("org.freedesktop.DBus.Error.PropertyReadOnly",
		[]any{iface + "." + prop + " is read-only"})
}

// introspectIface is org.freedesktop.DBus.Introspectable: the node's
// interface set, so real AT tooling (which introspects before calling)
// marshals correctly.
type introspectIface struct {
	text, action, value, app bool
}

func (o *introspectIface) Introspect() (string, *dbus.Error) {
	return introspectionXML(o.text, o.action, o.value, o.app), nil
}

// toolkitVersion reports the module's version from the build info,
// "devel" for untagged checkouts.
var toolkitVersion = readToolkitVersion()
