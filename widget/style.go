// The CSS override layer (docs/css.md): classes and ids on the shared
// node, stylesheet loading with hot reload, and the per-widget
// computed style the render path reads where it reads Theme fields
// today.
//
// The data flow is one direction. LoadStylesheet parses once and
// installs the Sheet; a style generation stamp goes stale everywhere
// at once. Each widget's cascade is computed lazily on first read
// after an invalidation — never per paint — and the diff against the
// previous values drives the smallest correct invalidation: repaint
// the widget, relayout it when a layout-affecting property moved, and
// mark the subtree when an inherited one did. With no stylesheet
// loaded the cascade is empty, every consumer falls through to its
// theme-derived value, and the pixels are exactly the pre-CSS ones.
package widget

import (
	"os"
	"slices"
	"sync/atomic"
	"time"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// activeSheet holds the parsed stylesheet; nil means theme-only —
// the palette is the source of truth and nothing else participates.
var activeSheet atomic.Pointer[style.Sheet]

// styleGen bumps on every stylesheet (re)load. Every widget's computed
// style and measure cache compare their stamps against it, so a load
// restyles and relayouts the whole tree without walking it, and the
// damage collector repaints each widget once (the themeGen pattern).
var styleGen uint64 = 1

// LoadStylesheet parses css and installs it as the active stylesheet,
// replacing whatever was loaded before. An empty string removes the
// stylesheet and returns to theme-only. The next frame repaints with
// the new styles; call it from the event-loop goroutine, like SetTheme.
// Malformed rules are skipped with a warning on the library logger —
// the valid remainder still applies.
func LoadStylesheet(css string) {
	if css == "" {
		activeSheet.Store(nil)
	} else {
		activeSheet.Store(style.Parse(css))
	}
	styleGen++
}

// styleFile remembers the loaded stylesheet file for the stat poll.
type styleFile struct {
	path string
	size int64
	mod  time.Time
}

// pollPath is the file LoadStylesheetFile loaded, for the hot-reload
// poll; nil when no file was ever loaded.
var pollPath atomic.Pointer[styleFile]

// pollHook is the app-installed scheduler the poll rides: it runs fn
// once per second on the event-loop goroutine (app.Application.Every).
// Widget code cannot reach the loop timer itself, and the poll must
// not exist at all until a file is loaded.
var pollHook atomic.Pointer[func(fn func())]

// SetStylesheetPoller installs the scheduler LoadStylesheetFile's
// hot-reload poll ticks on: poll schedules fn once per second on the
// event-loop goroutine, for the life of the process. One hook per
// process, like SetRemovedHook; the app installs it once at startup.
// Without a hook (no running Application) a loaded file simply never
// hot-reloads.
func SetStylesheetPoller(poll func(fn func())) {
	if poll == nil {
		pollHook.Store(nil)
		return
	}
	pollHook.Store(&poll)
}

// LoadStylesheetFile loads a stylesheet from path and starts the
// hot-reload poll on it: once the file is loaded, a stat per second
// reloads on change, so a running showcase picks up edits for free.
// The poll is the only recurring cost a stylesheet adds, and it
// starts on the first file load.
func LoadStylesheetFile(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(path) //nolint:gosec // path is the app's stylesheet argument
	if err != nil {
		return err
	}
	LoadStylesheet(string(b))
	first := pollPath.Load() == nil
	pollPath.Store(&styleFile{path: path, size: fi.Size(), mod: fi.ModTime()})
	if first {
		if h := pollHook.Load(); h != nil {
			(*h)(pollStylesheet)
		}
	}
	return nil
}

// pollStylesheet stats the loaded file once and reloads it on change —
// the whole hot-reload cost is this one stat per second.
func pollStylesheet() {
	f := pollPath.Load()
	if f == nil {
		return
	}
	fi, err := os.Stat(f.path)
	if err != nil || (fi.Size() == f.size && fi.ModTime().Equal(f.mod)) {
		return
	}
	b, err := os.ReadFile(f.path)
	if err != nil {
		return
	}
	pollPath.Store(&styleFile{path: f.path, size: fi.Size(), mod: fi.ModTime()})
	LoadStylesheet(string(b))
}

// faceResolver resolves a stylesheet font-family/font-weight to a
// face — the app's font store (internal/sysfont) owns real families.
// Nil by default: family and weight declarations then keep the
// widget's constructor face, the same graceful no-op as an unknown
// element name.
var faceResolver atomic.Pointer[func(family string, weight int) (render.Font, bool)]

// SetFaceResolver installs the family/weight → face lookup the
// font-family and font-weight declarations shape with. One resolver
// per process; nil restores the default no-op. It is consulted at
// style-invalidation time, never per paint.
func SetFaceResolver(f func(family string, weight int) (render.Font, bool)) {
	if f == nil {
		faceResolver.Store(nil)
		return
	}
	faceResolver.Store(&f)
}

// resolveFace consults the installed resolver; ok is false when none
// is installed or it declines the family.
func resolveFace(family string, weight int) (render.Font, bool) {
	if f := faceResolver.Load(); f != nil {
		return (*f)(family, weight)
	}
	return nil, false
}

// effShadow resolves the box-shadow cascade for a floating surface:
// the stylesheet's when it declares one, else the theme's. Blur 0
// means off. The color defaults like the theme's unset one does.
func effShadow(w Widget, t *Theme) (render.Color, int) {
	n := nodeOf(w)
	if n == nil {
		return t.shadowPaint()
	}
	v := n.style(w)
	if v.Has(style.PropBoxShadow) {
		if v.ShadowBlur <= 0 {
			return 0, 0
		}
		col := v.ShadowColor
		if col == 0 {
			col = defaultShadowColor
		}
		return col, v.ShadowBlur
	}
	return t.shadowPaint()
}

// AddClass adds style classes; a widget matches `.class` selectors
// naming any of them. Adding a class already present is a no-op. The
// change restyles the widget (and, for inherited properties, its
// subtree) on the next read.
func (n *node) AddClass(names ...string) {
	changed := false
	for _, name := range names {
		if name == "" || n.hasClass(name) {
			continue
		}
		n.classes = append(n.classes, name)
		changed = true
	}
	if changed {
		n.invalidateStyle()
	}
}

// RemoveClass drops style classes; dropping one the widget does not
// carry is a no-op.
func (n *node) RemoveClass(names ...string) {
	changed := false
	for _, name := range names {
		for i, c := range n.classes {
			if c == name {
				n.classes = append(n.classes[:i], n.classes[i+1:]...)
				changed = true
				break
			}
		}
	}
	if changed {
		n.invalidateStyle()
	}
}

// HasClass reports whether the widget carries the style class.
func (n *node) HasClass(name string) bool { return n.hasClass(name) }

func (n *node) hasClass(name string) bool {
	return slices.Contains(n.classes, name)
}

// HasClass reports whether w carries the style class — the read side
// for tests and accessibility tooling.
func HasClass(w Widget, class string) bool {
	n := nodeOf(w)
	return n != nil && n.hasClass(class)
}

// SetID sets the widget's style id, which `#id` selectors match. It is
// distinct from the accessible name: a11y names stay semantic text,
// not style hooks. Empty clears.
func (n *node) SetID(id string) {
	checkLoop("SetID")
	if n.id == id {
		return
	}
	n.id = id
	n.invalidateStyle()
}

// ID returns the style id, empty when none is set.
func (n *node) ID() string { return n.id }

// SetElement overrides the element name the widget matches in
// stylesheets. The default is the widget's own type name lowercased;
// the app-level surfaces (dialog, popover, tooltip, toast) name
// themselves with this in their constructors.
func (n *node) SetElement(name string) {
	checkLoop("SetElement")
	if name == "" || n.element == name {
		return
	}
	n.element = name
	n.invalidateStyle()
}

// Element returns the element name the widget matches: the override
// set with SetElement, else the lowercased Go type name.
func (n *node) Element() string {
	if n.element != "" {
		return n.element
	}
	return n.elemName
}

// styleNoder is implemented by every node-embedding widget.
type styleNoder interface{ styleNode() *node }

func (n *node) styleNode() *node { return n }

// nodeOf returns the shared node behind w, or nil.
func nodeOf(w Widget) *node {
	if n, ok := w.(styleNoder); ok {
		return n.styleNode()
	}
	return nil
}

// styleTargeter adapts one widget (and, through StyleParent, its
// ancestor chain) to the matcher. Instances are process-level and
// reused — the n/w fields are rewritten per node — so walking the
// chain never allocates.
type styleTargeter struct {
	n   *node
	w   Widget
	anc *styleTargeter // the reused adapter for the chain's next link
}

func (s *styleTargeter) StyleTarget(t *style.Target) { s.n.fillTarget(t, s.w) }

func (s *styleTargeter) StyleParent() style.Node {
	p := s.n.parent
	if p == nil {
		return nil
	}
	pn := nodeOf(p)
	if pn == nil {
		return nil
	}
	if s.anc == nil {
		s.anc = &styleTargeter{}
	}
	s.anc.n, s.anc.w = pn, p
	return s.anc
}

// fillTarget fills the selector-relevant facts of one widget.
func (n *node) fillTarget(t *style.Target, w Widget) {
	t.Element = n.Element()
	if t.Element == "" {
		t.Element = typeElementName(w)
	}
	t.ID = n.id
	t.Classes = n.classes
	t.State = n.styleState(w)
}

// styleState is the widget's interactive state for matching: the
// focus bit the router maintains, the hover and press visuals each
// widget tracks in its own fields, and the effective disabled state —
// inherited, like IsEnabled reads it.
func (n *node) styleState(w Widget) style.State {
	var s style.State
	if n.focused {
		s |= style.Focus
	}
	for p := n; p != nil; p = parentNodeOf(p) {
		if p.disabled {
			s |= style.Disabled
			break
		}
	}
	s |= hoverActiveOf(w)
	return s
}

// parentNodeOf returns the node behind n's recorded parent.
func parentNodeOf(n *node) *node {
	if n.parent == nil {
		return nil
	}
	return nodeOf(n.parent)
}

// hoverActiveOf reads the hover and press visuals a widget tracks in
// its own fields, so the plain fields the router drives stay the one
// source of truth for :hover and :active.
func hoverActiveOf(w Widget) style.State {
	var s style.State
	switch t := w.(type) {
	case *Button:
		if t.Hovered {
			s |= style.Hover
		}
		if t.Pressed {
			s |= style.Active
		}
	case *Dropdown:
		if t.hovered {
			s |= style.Hover
		}
	case *Expander:
		if t.hovered {
			s |= style.Hover
		}
	case *Menu:
		if t.hovered >= 0 {
			s |= style.Hover
		}
	case *Paned:
		if t.hovered {
			s |= style.Hover
		}
		if t.pressed {
			s |= style.Active
		}
	case *RichLabel:
		if t.hoverValid {
			s |= style.Hover
		}
	case *Scroll:
		if t.hovered {
			s |= style.Hover
		}
		if t.dragV || t.dragH {
			s |= style.Active
		}
	case *Slider:
		if t.Pressed {
			s |= style.Active
		}
	case *Toast:
		if t.hovered {
			s |= style.Hover
		}
	}
	return s
}

// styleScratch is the matcher's shared workspace, and styleSelf its
// shared adapter: style work runs on the event-loop goroutine
// (docs/threading.md), so one buffer serves the process and matching
// allocates nothing.
var styleScratch style.Scratch

var styleSelf styleTargeter

// style returns the widget's computed style — the paint path's entry
// point, called exactly where Theme fields are read today. Steady
// state is two integer compares; after an invalidation the first read
// recomputes and diff-invalidates.
func (n *node) style(w Widget) *style.Values {
	if !n.styleDirty && n.csGen == styleGen {
		return &n.cs
	}
	return n.restyle(w)
}

// restyle recomputes the cascade and invalidates the smallest correct
// region: the widget when its own values changed, the subtree when an
// inherited one did, a relayout when a layout-affecting one did. The
// zero-style case changes nothing and costs one compare.
func (n *node) restyle(w Widget) *style.Values {
	old := n.cs
	var v style.Values
	if sheet := activeSheet.Load(); sheet != nil {
		if n.element == "" && n.elemName == "" {
			n.elemName = typeElementName(w)
		}
		styleSelf.n, styleSelf.w = n, w
		sheet.Match(&styleSelf, &styleScratch, &v)
	}
	if pw := n.parent; pw != nil {
		if pn := nodeOf(pw); pn != nil {
			// The chain above must be fresh before its values inherit:
			// a stale ancestor recomputes first, recursively.
			v.InheritFrom(pn.style(pw))
		}
	}
	n.cs = v
	n.csGen = styleGen
	n.styleSeen = styleGen
	n.styleDirty = false
	if v != old {
		if inv, ok := w.(interface{ Invalidate() }); ok {
			inv.Invalidate()
		}
		if h, ok := w.(styleRestyler); ok {
			h.styleRestyled(old, v)
		}
	}
	if layoutKey(v) != layoutKey(old) {
		if inv, ok := w.(interface{ InvalidateLayout() }); ok {
			inv.InvalidateLayout()
		}
	}
	if inheritedKey(v) != inheritedKey(old) {
		restyleSubtree(w)
	}
	return &n.cs
}

// styleRestyler is the optional per-widget reaction to a changed
// cascade: Label reshapes when its effective face or size moved.
type styleRestyler interface {
	styleRestyled(old, new style.Values)
}

// layoutKey projects the values that change what a widget wants to
// measure; a change requests a relayout through InvalidateLayout.
func layoutKey(v style.Values) style.Values {
	const keep = 1<<style.PropPadding |
		1<<style.PropFontSize | 1<<style.PropFontFamily | 1<<style.PropFontWeight |
		1<<style.PropMinWidth | 1<<style.PropMinHeight
	v.Set &= keep
	v.Color, v.Background, v.BorderColor, v.ShadowColor = 0, 0, 0, 0
	v.Radius, v.BorderWidth, v.ShadowBlur = 0, 0, 0
	return v
}

// inheritedKey projects the values descendants inherit; a change
// restyles the subtree.
func inheritedKey(v style.Values) style.Values {
	const keep = 1<<style.PropColor |
		1<<style.PropFontSize | 1<<style.PropFontFamily | 1<<style.PropFontWeight
	v.Set &= keep
	v.Background, v.BorderColor, v.ShadowColor = 0, 0, 0
	v.Padding, v.Radius, v.BorderWidth, v.ShadowBlur, v.MinWidth, v.MinHeight = 0, 0, 0, 0, 0, 0
	return v
}

// restyleSubtree marks every descendant's computed style stale and
// repaint-owed: an inherited property changed above them, so each
// recomputes (and diff-invalidates) on its next read.
func restyleSubtree(w Widget) {
	walkTree(w, 0, func(k Widget, _ int) {
		if k == w {
			return
		}
		n := nodeOf(k)
		if n == nil {
			return
		}
		n.styleDirty = true
		if inv, ok := k.(interface{ Invalidate() }); ok {
			inv.Invalidate()
		}
	})
}

// invalidateStyle is the shared body of the style-relevant mutators
// (state flips, class and id changes): mark the cascade stale and owe
// the repaint. The diff-driven subtree and relayout work happens at
// the next read, in restyle.
func (n *node) invalidateStyle() {
	checkLoop("invalidateStyle")
	n.styleDirty = true
	n.Invalidate()
}

// pickc resolves a color property: the programmatic widget color when
// set, else the stylesheet value when matched, else the
// theme-derived default — the cascade's three origins, in order.
func pickc(prog render.Color, v *style.Values, p style.Prop, def render.Color) render.Color {
	if prog != 0 {
		return prog
	}
	if v.Has(p) {
		switch p {
		case style.PropColor:
			return v.Color
		case style.PropBackgroundColor:
			return v.Background
		case style.PropBorderColor:
			return v.BorderColor
		case style.PropBoxShadow:
			return v.ShadowColor
		}
	}
	return def
}

// picki resolves an integer property: the stylesheet value when
// matched, else the constructor or theme default. Widget metrics have
// no programmatic layer — a constructor size is the default the
// stylesheet overrides, not app intent above it.
func picki(v *style.Values, p style.Prop, def int) int {
	if v.Has(p) {
		switch p {
		case style.PropPadding:
			return v.Padding
		case style.PropBorderRadius:
			return v.Radius
		case style.PropBorderWidth:
			return v.BorderWidth
		case style.PropMinWidth:
			return v.MinWidth
		case style.PropMinHeight:
			return v.MinHeight
		}
	}
	return def
}

// fontPx resolves the text size: the stylesheet's when matched, else
// the constructor default.
func fontPx(v *style.Values, def float64) float64 {
	if v.Has(style.PropFontSize) {
		return v.FontSize
	}
	return def
}

// shrinkRect pulls r in by n on every side.
func shrinkRect(r render.Rect, n int) render.Rect {
	return render.Rect{X: r.X + n, Y: r.Y + n, W: max(0, r.W-2*n), H: max(0, r.H-2*n)}
}

// typeElementName returns the widget type's element name: the
// lowercased Go type name. The list is pinned by TestCSSElementNames;
// a type missing from the switch matches no element selector — the
// same graceful no-op as an unknown name in a stylesheet.
func typeElementName(w Widget) string {
	switch w.(type) {
	case *Box:
		return "box"
	case *Button:
		return "button"
	case *Calendar:
		return "calendar"
	case *CheckButton:
		return "checkbutton"
	case *ColorChooser:
		return "colorchooser"
	case *Dropdown:
		return "dropdown"
	case *DropdownOf[any]:
		return "dropdown"
	case *Elevation:
		return "elevation"
	case *Entry:
		return "entry"
	case *Expander:
		return "expander"
	case *Fader:
		return "fader"
	case *Grid:
		return "grid"
	case *Icon:
		return "icon"
	case *Image:
		return "image"
	case *Label:
		return "label"
	case *List:
		return "list"
	case *listRow:
		return "listrow"
	case *Menu:
		return "menu"
	case *Notebook:
		return "notebook"
	case *Overlay:
		return "overlay"
	case *Paned:
		return "paned"
	case *ProgressBar:
		return "progressbar"
	case *RichLabel:
		return "richlabel"
	case *Scroll:
		return "scroll"
	case *Separator:
		return "separator"
	case *Slider:
		return "slider"
	case *Spinner:
		return "spinner"
	case *Stack:
		return "stack"
	case *Switch:
		return "switch"
	case *TextArea:
		return "textarea"
	case *Toast:
		return "toast"
	default:
		return ""
	}
}
