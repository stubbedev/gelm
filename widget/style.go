// The CSS layer (docs/css.md): classes, ids, states, and inline styles
// on the shared node; prioritized stylesheets with hot reload; and the
// per-widget computed style the render path reads where it reads Theme
// fields.
//
// The data flow is one direction. AddStylesheet and LoadStylesheet
// parse once and install the layer; a style generation stamp goes stale
// everywhere at once. Each widget's cascade is computed lazily on first
// read after an invalidation — never per paint — and the diff against
// the previous values drives the smallest correct invalidation: repaint
// the widget, relayout it when a layout-affecting property moved, and
// mark the subtree when an inherited one (a custom property included)
// did. A state, class, or id flip restyles the widget alone unless a
// loaded selector reads that fact left of a combinator; then the
// subtree (and, for sibling selectors, the neighbors) restyle too. With
// no stylesheet loaded the cascade is empty, every consumer falls
// through to its theme-derived value, and the pixels are exactly the
// pre-CSS ones.
package widget

import (
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// Style priorities, GTK's provider priorities: a higher priority wins
// over any specificity below it; equal priorities fall to specificity,
// then to load order.
const (
	// StylePriorityFallback is for defaults below everything.
	StylePriorityFallback = style.PriorityFallback
	// StylePriorityTheme is for a theme's stylesheet.
	StylePriorityTheme = style.PriorityTheme
	// StylePrioritySettings is for settings-derived rules.
	StylePrioritySettings = style.PrioritySettings
	// StylePriorityApplication is the application's own stylesheet;
	// LoadStylesheet installs at it.
	StylePriorityApplication = style.PriorityApplication
	// StylePriorityUser is for user overrides, and the default priority
	// of a widget's inline style.
	StylePriorityUser = style.PriorityUser
)

// Stylesheet is one parsed stylesheet at a priority, reloadable in
// place. Installed with AddStylesheet it applies to every widget;
// created with NewStylesheet and attached to widgets
// (AttachStylesheet) it applies to those subtrees only — one surface's
// styling that must not reach another's. The zero value is not usable.
type Stylesheet struct {
	sheet    *style.Sheet
	priority int
	live     bool
	// attached counts the widgets the sheet is attached to.
	attached int
}

// sheets is every installed stylesheet in install order; layersNow is
// the snapshot the cascade reads, rebuilt on every change. Both are
// loop-goroutine state, like the widget tree.
var (
	sheets    []*Stylesheet
	layersNow []style.Layer
	sensNow   sensitivity
)

// sensitivity is the union of the loaded sheets' selector summaries.
type sensitivity struct {
	states   style.State
	classes  map[string]bool
	siblings bool
}

// styleGen bumps on every stylesheet (re)load and environment change.
// Every widget's computed style and measure cache compare their stamps
// against it, so a load restyles and relayouts the whole tree without
// walking it, and the damage collector repaints each widget once (the
// themeGen pattern).
var styleGen uint64 = 1

// NewStylesheet parses css into a stylesheet at priority without
// installing it: attach it to the widgets whose subtrees it styles.
func NewStylesheet(css string, priority int) *Stylesheet {
	return &Stylesheet{sheet: style.Parse(css), priority: priority}
}

// SetParseWarn installs f as the CSS engine's parse-warning sink; nil
// restores the library logger. One sink per process. Hosts that want
// stylesheet problems visible — a settings app showing theme errors —
// route the messages to their log or interface.
func SetParseWarn(f func(msg string)) {
	style.SetParseWarn(f)
}

// AddStylesheet parses css and installs it at priority, above or below
// the other stylesheets by priority and, at equal priority, after every
// earlier one. The next frame repaints with it; call it from the
// event-loop goroutine, like SetTheme. Malformed rules are skipped with
// a warning on the library logger — the valid remainder still applies.
func AddStylesheet(css string, priority int) *Stylesheet {
	s := &Stylesheet{sheet: style.Parse(css), priority: priority, live: true}
	sheets = append(sheets, s)
	relayer()
	return s
}

// Load replaces the stylesheet's rules with css, keeping its priority
// and position. Loading into a removed (never attached) stylesheet
// reinstalls it; an attached one keeps styling its subtrees.
func (s *Stylesheet) Load(css string) {
	s.sheet = style.Parse(css)
	if !s.live && s.attached == 0 {
		s.live = true
		sheets = append(sheets, s)
	}
	relayer()
}

// Remove uninstalls the stylesheet; removing twice is a no-op.
func (s *Stylesheet) Remove() {
	if !s.live {
		return
	}
	s.live = false
	sheets = slices.DeleteFunc(sheets, func(o *Stylesheet) bool { return o == s })
	relayer()
}

// Priority returns the priority the stylesheet was installed at.
func (s *Stylesheet) Priority() int { return s.priority }

// relayer rebuilds the cascade snapshot and the sensitivity union and
// stamps every widget stale.
func relayer() {
	layersNow = layersNow[:0]
	sensNow = sensitivity{}
	for _, s := range sheets {
		layersNow = append(layersNow, style.Layer{Sheet: s.sheet, Priority: s.priority})
	}
	for _, s := range append(slices.Clone(sheets), scopedSheets...) {
		sn := s.sheet.Sensitivity()
		sensNow.states |= sn.AncestorStates
		sensNow.siblings = sensNow.siblings || sn.Siblings
		for c := range sn.AncestorClasses {
			if sensNow.classes == nil {
				sensNow.classes = map[string]bool{}
			}
			sensNow.classes[c] = true
		}
	}
	styleGen++
}

// scopedSheets are the stylesheets attached to at least one widget.
var scopedSheets []*Stylesheet

// AttachStylesheet applies s to this widget and its descendants, on top
// of the installed stylesheets (a tie at equal priority and specificity
// goes to the attached one). Attaching the same stylesheet twice is a
// no-op; one stylesheet can style many subtrees, and a Load restyles
// them all.
func (n *node) AttachStylesheet(s *Stylesheet) {
	checkLoop("AttachStylesheet")
	if slices.Contains(n.sheets, s) {
		return
	}
	n.sheets = append(n.sheets, s)
	if s.attached++; s.attached == 1 {
		scopedSheets = append(scopedSheets, s)
	}
	relayer()
}

// DetachStylesheet stops s from styling this subtree; detaching one
// that is not attached is a no-op.
func (n *node) DetachStylesheet(s *Stylesheet) {
	checkLoop("DetachStylesheet")
	i := slices.Index(n.sheets, s)
	if i < 0 {
		return
	}
	n.sheets = slices.Delete(n.sheets, i, i+1)
	if s.attached--; s.attached == 0 {
		scopedSheets = slices.DeleteFunc(scopedSheets, func(o *Stylesheet) bool { return o == s })
	}
	relayer()
}

// layerBuf is layersFor's reusable buffer (loop-goroutine only).
var layerBuf []style.Layer

// layersFor returns the cascade layers for n: the installed sheets,
// then the sheets attached to n and its ancestors, outermost first.
func layersFor(n *node) []style.Layer {
	if len(scopedSheets) == 0 {
		return layersNow
	}
	layerBuf = append(layerBuf[:0], layersNow...)
	mark := len(layerBuf)
	for p := n; p != nil; p = styleParentNodeOf(p) {
		for _, s := range slices.Backward(p.sheets) {
			layerBuf = append(layerBuf, style.Layer{Sheet: s.sheet, Priority: s.priority})
		}
	}
	slices.Reverse(layerBuf[mark:])
	return layerBuf
}

// appSheet is the stylesheet LoadStylesheet manages.
var appSheet *Stylesheet

// LoadStylesheet parses css and installs it as the application
// stylesheet (StylePriorityApplication), replacing whatever
// LoadStylesheet loaded before. An empty string removes it. Other
// stylesheets added with AddStylesheet are untouched.
func LoadStylesheet(css string) {
	if css == "" {
		if appSheet != nil {
			appSheet.Remove()
		}
		return
	}
	if appSheet == nil {
		appSheet = AddStylesheet(css, StylePriorityApplication)
		return
	}
	appSheet.Load(css)
}

// styleEnv is the unit environment: what 1rem is, and the font size a
// widget computes when nothing in its ancestry sets one.
var styleEnv = style.Env{Rem: 16, FontPx: 16}

// SetRootFontSize sets the root font size stylesheet units resolve
// against: 1rem, and the default font size (CSS's `medium`) of widgets
// nothing sets one for. The default is 16 logical pixels, the CSS
// default; GTK derives its own from the desktop font setting. The whole
// tree restyles.
func SetRootFontSize(px float64) {
	if px <= 0 || px == styleEnv.Rem {
		return
	}
	styleEnv = style.Env{Rem: px, FontPx: px}
	styleGen++
}

// RootFontSize returns the current root font size.
func RootFontSize() float64 { return styleEnv.Rem }

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

// LoadStylesheetFile loads a stylesheet from path as the application
// stylesheet and starts the hot-reload poll on it: once the file is
// loaded, a stat per second reloads on change, so a running showcase
// picks up edits for free. The poll is the only recurring cost a
// stylesheet adds, and it starts on the first file load.
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
// the stylesheet's first outer layer when it declares a shadow, else
// the theme's. Blur 0 means off. A zero color defaults like the theme's
// unset one does.
func effShadow(w Widget, t *Theme) (render.Color, int) {
	n := nodeOf(w)
	if n == nil {
		return t.shadowPaint()
	}
	v := n.style(w)
	if v.Has(style.PropBoxShadow) {
		for _, sh := range v.Shadow.List() {
			if sh.Inset || sh.Blur <= 0 {
				continue
			}
			col := sh.Color
			if col == 0 {
				col = defaultShadowColor
			}
			return col, sh.Blur
		}
		return 0, 0
	}
	return t.shadowPaint()
}

// StateFlags are the widget states a stylesheet matches that no router
// drives: an application sets them to reflect its own model.
type StateFlags uint8

// The application-driven states.
const (
	// StateChecked matches :checked — a toggled-on control, or a menu
	// button whose popup is open.
	StateChecked StateFlags = 1 << iota
	// StateSelected matches :selected.
	StateSelected
	// StateIndeterminate matches :indeterminate.
	StateIndeterminate
	// StateBackdrop matches :backdrop — the widget's window is inactive.
	StateBackdrop
)

// styleBits maps flags onto the matcher's state bits.
func (f StateFlags) styleBits() style.State {
	var s style.State
	if f&StateChecked != 0 {
		s |= style.Checked
	}
	if f&StateSelected != 0 {
		s |= style.Selected
	}
	if f&StateIndeterminate != 0 {
		s |= style.Indeterminate
	}
	if f&StateBackdrop != 0 {
		s |= style.Backdrop
	}
	return s
}

// SetState turns application-driven state flags on or off. The widget
// restyles when the set changed.
func (n *node) SetState(flags StateFlags, on bool) {
	next := n.flags &^ flags
	if on {
		next |= flags
	}
	if next == n.flags {
		return
	}
	changed := n.flags ^ next
	n.flags = next
	n.invalidateState(changed.styleBits())
}

// HasState reports whether every flag in flags is on.
func (n *node) HasState(flags StateFlags) bool { return n.flags&flags == flags }

// SetInlineStyle sets the widget's inline declarations — `color: red;
// --accent: #f80` — the widget-scoped provider GTK attaches per widget.
// They apply to this widget alone (custom properties then inherit into
// its subtree) at StylePriorityUser, with no specificity. Empty clears.
// Malformed declarations are skipped with a warning.
func (n *node) SetInlineStyle(css string) {
	n.SetInlineStylePriority(css, StylePriorityUser)
}

// SetInlineStylePriority is SetInlineStyle at an explicit priority.
func (n *node) SetInlineStylePriority(css string, priority int) {
	checkLoop("SetInlineStyle")
	if css == n.inlineSrc && priority == n.inlinePrio && (n.inline != nil) == (css != "") {
		return
	}
	n.inlineSrc, n.inlinePrio = css, priority
	if css == "" {
		n.inline = nil
	} else {
		n.inline = style.ParseDeclarations(css)
	}
	n.invalidateStyle()
}

// InlineStyle returns the inline declarations as set.
func (n *node) InlineStyle() string { return n.inlineSrc }

// AddClass adds style classes; a widget matches `.class` selectors
// naming any of them. Adding a class already present is a no-op. The
// change restyles the widget (and, when a selector tests the class on
// an ancestor or sibling, the widgets that depend on it) on the next
// read.
func (n *node) AddClass(names ...string) {
	var changed []string
	for _, name := range names {
		if name == "" || n.hasClass(name) {
			continue
		}
		n.classes = append(n.classes, name)
		changed = append(changed, name)
	}
	if len(changed) > 0 {
		n.invalidateClasses(changed)
	}
}

// RemoveClass drops style classes; dropping one the widget does not
// carry is a no-op.
func (n *node) RemoveClass(names ...string) {
	var changed []string
	for _, name := range names {
		for i, c := range n.classes {
			if c == name {
				n.classes = append(n.classes[:i], n.classes[i+1:]...)
				changed = append(changed, name)
				break
			}
		}
	}
	if len(changed) > 0 {
		n.invalidateClasses(changed)
	}
}

// SetClasses replaces the class list wholesale — the GTK
// set_css_classes idiom for widgets whose modifiers are recomputed at
// once. Order is irrelevant to matching.
func (n *node) SetClasses(names ...string) {
	var next []string
	for _, name := range names {
		if name != "" && !slices.Contains(next, name) {
			next = append(next, name)
		}
	}
	var changed []string
	for _, c := range n.classes {
		if !slices.Contains(next, c) {
			changed = append(changed, c)
		}
	}
	for _, c := range next {
		if !slices.Contains(n.classes, c) {
			changed = append(changed, c)
		}
	}
	if len(changed) == 0 {
		return
	}
	n.classes = next
	n.invalidateClasses(changed)
}

// Classes returns a copy of the widget's classes.
func (n *node) Classes() []string { return slices.Clone(n.classes) }

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
	n.invalidateScope(true, true)
}

// ID returns the style id, empty when none is set.
func (n *node) ID() string { return n.id }

// StyleBackground returns the widget's computed background color, the
// diagnostic view a paint test reads.
func StyleBackground(w Widget) render.Color {
	if n := nodeOf(w); n != nil {
		return pickc(0, n.style(w), style.PropBackgroundColor, 0)
	}
	return 0
}

// CascadeColor resolves the widget's computed color: the CSS `color`,
// inherited down the tree, which the canvas-drawn primitives take as
// their ink (the progress ring's stroke). Zero paints nothing — the
// no-stylesheet behavior, same as everywhere else.
func CascadeColor(w Widget) render.Color {
	n := nodeOf(w)
	if n == nil {
		return 0
	}
	return pickc(0, n.style(w), style.PropColor, 0)
}

// CascadeBorder resolves the widget's computed border widths: the
// stroke a canvas-drawn primitive takes from the stylesheet (the
// progress ring reads the top width as its stroke).
func CascadeBorder(w Widget) render.Insets {
	n := nodeOf(w)
	if n == nil {
		return render.Insets{}
	}
	return borderOf(n.style(w))
}

// PaintBoxLayers paints the widget's computed box layers — background,
// image, borders, shadows, and the outline — at its bounds. A custom
// widget's Paint calls it for its own chrome before drawing content,
// so the container styling stays in the cascade (the hovered row's
// background, the :hover rule, is this). The opacity and brightness
// in force wrap the layers.
func PaintBoxLayers(cv *render.Canvas, w Widget) {
	n := nodeOf(w)
	if n == nil {
		return
	}
	v := n.style(w)
	fx := pushEffects(cv, v)
	defer fx.pop(cv)
	radii := radiusOr(v, 0)
	bg := pickc(0, v, style.PropBackgroundColor, 0)
	if bg != 0 || hasBoxLayers(v) {
		paintBoxBehind(cv, v, n.bounds, radii, borderOf(v), bg)
	}
	paintOutline(cv, v, n.bounds, radii)
}

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
	n.invalidateScope(true, true)
}

// Element returns the element name the widget matches: the override
// set with SetElement, else the lowercased Go type name.
func (n *node) Element() string {
	if n.element != "" {
		return n.element
	}
	if n.elemName == "" && n.self != nil {
		n.elemName = typeElementName(n.self)
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

// styleTargeter adapts one widget (and, through StyleParent and
// StylePrev, its neighborhood) to the matcher. Instances are reused —
// each link's adapters hang off the previous one and are rewritten per
// match — so walking the tree allocates only on first use of a depth.
type styleTargeter struct {
	n    *node
	w    Widget
	anc  *styleTargeter // the reused adapter for the parent link
	prev *styleTargeter // the reused adapter for the previous sibling
}

func (s *styleTargeter) StyleTarget(t *style.Target) { s.n.fillTarget(t, s.w) }

func (s *styleTargeter) StyleParent() style.Node {
	p := s.n.parent
	if p == nil || s.n.styleRoot {
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

func (s *styleTargeter) StylePrev() style.Node {
	sib, idx := visibleSiblings(s.n, s.w)
	if idx <= 0 {
		return nil
	}
	pw := sib[idx-1]
	pn := nodeOf(pw)
	if pn == nil {
		return nil
	}
	if s.prev == nil {
		s.prev = &styleTargeter{}
	}
	s.prev.n, s.prev.w = pn, pw
	return s.prev
}

func (s *styleTargeter) StylePosition() (int, int) {
	sib, idx := visibleSiblings(s.n, s.w)
	if idx < 0 {
		return 1, 1
	}
	return idx + 1, len(sib)
}

// siblingBuf is visibleSiblings' reusable buffer (loop-goroutine only).
var siblingBuf []Widget

// visibleSiblings returns the visible children of n's parent and n's
// index among them; -1 when n has no parent or is not among them.
func visibleSiblings(n *node, w Widget) ([]Widget, int) {
	if n.parent == nil {
		return nil, -1
	}
	siblingBuf = siblingBuf[:0]
	switch p := n.parent.(type) {
	case childBuf:
		siblingBuf = p.appendChildren(siblingBuf)
	case childser:
		siblingBuf = append(siblingBuf, p.Children()...)
	default:
		return nil, -1
	}
	out := siblingBuf[:0]
	idx := -1
	for _, k := range siblingBuf {
		if k == nil {
			continue
		}
		if kn := nodeOf(k); kn != nil && kn.hidden {
			continue
		}
		if k == w {
			idx = len(out)
		}
		out = append(out, k)
	}
	return out, idx
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
	t.Inline = n.inline
	t.InlinePriority = n.inlinePrio
}

// styleState is the widget's interactive state for matching: the
// focus bits the router maintains, the hover chain (a hovered widget's
// ancestors are hovered too, GTK's rule), the hover and press visuals
// each widget tracks in its own fields, the application flags, and the
// effective disabled state — inherited, like IsEnabled reads it.
func (n *node) styleState(w Widget) style.State {
	s := n.flags.styleBits()
	if n.focused {
		s |= style.Focus
		if n.focusVisible {
			s |= style.FocusVisible
		}
	}
	if n.focusWithin > 0 {
		s |= style.FocusWithin
	}
	if n.hoverChain {
		s |= style.Hover
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

// styleParentNodeOf is n's parent for styling: none past a style root.
func styleParentNodeOf(n *node) *node {
	if n.styleRoot {
		return nil
	}
	return parentNodeOf(n)
}

// SetStyleRoot makes w the root of its tree for styling: :root matches
// it, and nothing above it is inherited or contributes scoped sheets,
// while it keeps its layout parent - the app wraps every window's tree
// in plumbing (a fader, the inspector overlay) that invalidations must
// climb through but stylesheets must not see.
func SetStyleRoot(w Widget) {
	if n := nodeOf(w); n != nil && !n.styleRoot {
		n.styleRoot = true
		n.styleDirty = true
	}
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
// source of truth for :hover and :active, plus the intrinsic :checked
// of toggles.
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
	case *menuRow:
		if t.hovered {
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
		if t.hovered {
			s |= style.Hover
		}
		if t.Pressed {
			s |= style.Active
		}
	case *Toast:
		if t.hovered {
			s |= style.Hover
		}
	case *Switch:
		if t.On() {
			s |= style.Checked
		}
	case *CheckButton:
		if t.Checked() {
			s |= style.Checked
		}
		if t.Inconsistent() {
			s |= style.Indeterminate
		}
	}
	return s
}

// styleScratch is the matcher's shared workspace, and styleSelf its
// shared adapter: style work runs on the event-loop goroutine
// (docs/threading.md), so one buffer serves the process.
var styleScratch style.Scratch

var styleSelf styleTargeter

// style returns the widget's computed style — the paint path's entry
// point, called exactly where Theme fields are read. Steady state is
// two integer compares; after an invalidation the first read
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
	var parent *style.Values
	if pw := n.parent; pw != nil && !n.styleRoot {
		if pn := nodeOf(pw); pn != nil {
			// The chain above must be fresh before it inherits: a stale
			// ancestor recomputes first, recursively. It runs before the
			// layers are gathered, which share a buffer with its compute.
			parent = pn.style(pw)
		}
	}
	inherits := parent != nil && (parent.Set != 0 || parent.Vars != nil)
	if layers := layersFor(n); len(layers) > 0 || n.inline != nil || inherits {
		if n.element == "" && n.elemName == "" {
			n.elemName = typeElementName(w)
		}
		styleSelf.n, styleSelf.w = n, w
		style.Compute(layers, &styleSelf, parent, &old, styleEnv, &styleScratch, &v)
	}
	n.cs = v
	n.csGen = styleGen
	n.styleSeen = styleGen
	n.styleDirty = false
	n.ink = inkOf(&v)
	n.transitionValues(old, v)
	n.syncAnimation(v)
	// Values carries the animation group, a slice, and so is not
	// comparable; the field-wise diff decides whether the widget's
	// paint is stale and gives the hooks their old/new pair.
	if styleChanged(&old, &v) {
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

// styleChanged reports whether a recomputed cascade differs from the
// cached one in anything a widget paints or lays out. Values carries
// slices (the animation group and its longhand slots), so the diff is
// field-wise; the animation entries themselves stay comparable.
func styleChanged(a, b *style.Values) bool {
	return a.Set != b.Set || a.Own != b.Own || a.Vars != b.Vars ||
		a.Color != b.Color || a.Background != b.Background || a.Image != b.Image ||
		a.Opacity != b.Opacity || a.Brightness != b.Brightness ||
		a.Padding != b.Padding || a.Margin != b.Margin ||
		a.BorderWidth != b.BorderWidth || a.BorderStyle != b.BorderStyle ||
		a.BorderColor != b.BorderColor || a.Radius != b.Radius || a.Shadow != b.Shadow ||
		a.OutlineWidth != b.OutlineWidth || a.OutlineStyle != b.OutlineStyle ||
		a.OutlineColor != b.OutlineColor || a.OutlineOffset != b.OutlineOffset ||
		a.MinWidth != b.MinWidth || a.MinHeight != b.MinHeight ||
		a.BorderSpacingH != b.BorderSpacingH || a.BorderSpacingV != b.BorderSpacingV ||
		a.FontFamily != b.FontFamily || a.FontSize != b.FontSize ||
		a.FontWeight != b.FontWeight || a.Italic != b.Italic ||
		a.LetterSpacing != b.LetterSpacing || a.TextTransform != b.TextTransform ||
		a.LineHeight != b.LineHeight || a.Features != b.Features ||
		a.Transform.M != b.Transform.M || a.OriginFrac != b.OriginFrac ||
		a.OriginPx != b.OriginPx || a.IconSize != b.IconSize ||
		a.IconSource != b.IconSource || a.IconXform.M != b.IconXform.M ||
		a.PaletteTint != b.PaletteTint || a.CaretColor != b.CaretColor ||
		a.Decoration != b.Decoration || a.Transition != b.Transition ||
		!animsEqual(a.Animation, b.Animation)
}

// animsEqual compares the computed animation groups entrywise.
func animsEqual(a, b []style.Animation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// layoutKey projects the values that change what a widget wants to
// measure or where its children go; a change requests a relayout
// through InvalidateLayout.
func layoutKey(v style.Values) layoutFacts {
	eff := v.EffBorder()
	return layoutFacts{
		padding: v.Padding, margin: v.Margin, border: eff,
		setPad: v.HasAny(style.PropPaddingTop, style.PropPaddingRight, style.PropPaddingBottom, style.PropPaddingLeft),
		minW:   v.MinWidth, minH: v.MinHeight, spacingH: v.BorderSpacingH, spacingV: v.BorderSpacingV,
		setSpacing: v.Has(style.PropBorderSpacing),
		fontSize:   v.FontSize, fontFamily: v.FontFamily, fontWeight: v.FontWeight, italic: v.Italic,
		iconSize: v.IconSize, transform: v.TextTransform, letter: v.LetterSpacing,
		set: v.Set & (1<<style.PropFontSize | 1<<style.PropFontFamily | 1<<style.PropFontWeight |
			1<<style.PropIconSize | 1<<style.PropMinWidth | 1<<style.PropMinHeight | 1<<style.PropTextTransform),
	}
}

// layoutFacts is layoutKey's comparable projection.
type layoutFacts struct {
	padding, margin, border style.Sides
	setPad                  bool
	minW, minH              int
	spacingH, spacingV      int
	setSpacing              bool
	fontSize                float64
	fontFamily              string
	fontWeight              int
	italic                  bool
	iconSize                int
	transform               style.TextTransform
	letter                  float64
	set                     style.PropSet
}

// inheritedKey projects the values descendants inherit — the custom
// property environment included; a change restyles the subtree.
func inheritedKey(v style.Values) inheritedFacts {
	const keep = 1<<style.PropColor | 1<<style.PropFontSize | 1<<style.PropFontFamily |
		1<<style.PropFontWeight | 1<<style.PropFontStyle | 1<<style.PropLetterSpacing |
		1<<style.PropTextTransform | 1<<style.PropIconSize
	return inheritedFacts{
		set: v.Set & keep, color: v.Color, fontSize: v.FontSize, fontFamily: v.FontFamily,
		fontWeight: v.FontWeight, italic: v.Italic, letter: v.LetterSpacing,
		transform: v.TextTransform, iconSize: v.IconSize, vars: v.Vars,
	}
}

// inheritedFacts is inheritedKey's comparable projection.
type inheritedFacts struct {
	set        style.PropSet
	color      render.Color
	fontSize   float64
	fontFamily string
	fontWeight int
	italic     bool
	letter     float64
	transform  style.TextTransform
	iconSize   int
	vars       *style.Vars
}

// inkOf is how far a widget paints outside its border box: outer box
// shadows and the outline. The damage collector grows the widget's
// repaint rect by it.
func inkOf(v *style.Values) render.Insets {
	var ink render.Insets
	for _, sh := range v.Shadow.List() {
		e := render.BoxShadow{X: sh.X, Y: sh.Y, Blur: sh.Blur, Spread: sh.Spread, Color: sh.Color, Inset: sh.Inset}.Extent()
		ink.Top, ink.Right = max(ink.Top, e.Top), max(ink.Right, e.Right)
		ink.Bottom, ink.Left = max(ink.Bottom, e.Bottom), max(ink.Left, e.Left)
	}
	if o := v.EffOutline(); o > 0 {
		reach := max(0, o+v.OutlineOffset)
		ink.Top, ink.Right = max(ink.Top, reach), max(ink.Right, reach)
		ink.Bottom, ink.Left = max(ink.Bottom, reach), max(ink.Left, reach)
	}
	return ink
}

// restyleSubtree marks every descendant's computed style stale:
// something above them changed what they match or inherit, so each
// recomputes on its next read — the damage walk reaches them
// pre-order — and repaints only if its values actually moved (the
// restyle diff invalidates).
func restyleSubtree(w Widget) {
	for _, k := range styleKids(w) {
		if n := nodeOf(k); n != nil {
			n.styleDirty = true
		}
		restyleSubtree(k)
	}
}

// styleKids are the widgets that inherit from w: its Children, or what
// a widget that keeps its children out of the Children walks (a
// Button's content, a List's rows, a Menu's icons: focus, hit
// testing and a11y treat the parent as the leaf) lists through
// styleChildren.
func styleKids(w Widget) []Widget {
	switch c := w.(type) {
	case interface{ styleChildren() []Widget }:
		return c.styleChildren()
	case childser:
		return c.Children()
	}
	return nil
}

// restyleChildren marks w's children (and their subtrees) stale: a
// child was added, removed, reordered, or hidden, and a loaded sibling
// selector may now match differently. Without sibling selectors it is
// free.
func restyleChildren(w Widget) {
	if !sensNow.siblings {
		return
	}
	restyleSubtree(w)
}

// invalidateStyle is the shared body of the self-only style mutators:
// mark the cascade stale and owe the repaint. The diff-driven subtree
// and relayout work happens at the next read, in restyle.
func (n *node) invalidateStyle() {
	checkLoop("invalidateStyle")
	n.styleDirty = true
	n.Invalidate()
}

// invalidateState restyles after the state bits in changed flipped:
// the widget, plus its subtree when a selector reads one of them on an
// ancestor, plus its siblings when sibling selectors are loaded. The
// widget repaints: its own paint may read the state (a theme hover
// shade).
func (n *node) invalidateState(changed style.State) {
	n.invalidateScope(sensNow.states&changed != 0, sensNow.siblings)
}

// restyleState is invalidateState for the propagated states — the
// hover chain and focus-within — which no widget paints from itself:
// the cascade is marked stale, and only a moved value repaints.
func (n *node) restyleState(changed style.State) {
	n.styleDirty = true
	n.scope(sensNow.states&changed != 0, sensNow.siblings)
}

// invalidateClasses is invalidateState for a class-list change.
func (n *node) invalidateClasses(changed []string) {
	anc := false
	for _, c := range changed {
		if sensNow.classes[c] {
			anc = true
			break
		}
	}
	n.invalidateScope(anc, sensNow.siblings)
}

// invalidateScope marks n stale, and its subtree when sub, and its
// parent's other children when sib. A widget that has never been
// arranged has no subtree links yet: its first restyle happens anyway.
func (n *node) invalidateScope(sub, sib bool) {
	n.invalidateStyle()
	n.scope(sub, sib)
}

// scope marks the dependents of a flipped fact: the subtree when sub,
// the parent's children (and theirs) when sib.
func (n *node) scope(sub, sib bool) {
	if !sub && !sib {
		return
	}
	if sib && n.parent != nil {
		restyleSubtree(n.parent)
		return
	}
	if n.self != nil {
		restyleSubtree(n.self)
		return
	}
	// The root (or a widget no container arranged yet): no link to walk
	// from, so the whole generation goes stale.
	styleGen++
}

// setHoverChain moves the hover chain from old's ancestry to new's: a
// hovered widget and all its ancestors match :hover. Nodes in both
// chains keep their bit and restyle nothing.
func setHoverChain(old, new Widget) {
	if old == new {
		return
	}
	in := map[*node]bool{}
	for w := new; w != nil; w = parentOf(w) {
		if n := nodeOf(w); n != nil {
			in[n] = true
		}
	}
	for w := old; w != nil; w = parentOf(w) {
		if n := nodeOf(w); n != nil && n.hoverChain && !in[n] {
			n.hoverChain = false
			n.restyleState(style.Hover)
			if n.onHoverWithin != nil {
				n.onHoverWithin(false)
			}
		}
	}
	for w := new; w != nil; w = parentOf(w) {
		if n := nodeOf(w); n != nil && !n.hoverChain {
			n.hoverChain = true
			n.restyleState(style.Hover)
			if n.onHoverWithin != nil {
				n.onHoverWithin(true)
			}
		}
	}
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
		case style.PropBorderTopColor:
			return v.BorderColor[0]
		case style.PropOutlineColor:
			return v.OutlineColor
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
		case style.PropMinWidth:
			return v.MinWidth
		case style.PropMinHeight:
			return v.MinHeight
		case style.PropBorderTopLeftRadius:
			return v.Radius.TopLeft
		case style.PropIconSize:
			return v.IconSize
		}
	}
	return def
}

// pickf returns the stylesheet's float value for p, def when unset —
// the same precedence picki gives ints.
func pickf(v *style.Values, p style.Prop, def float64) float64 {
	if v.Has(p) {
		switch p {
		case style.PropLineHeight:
			return v.LineHeight
		}
	}
	return def
}

// styledFace applies the cascade's face-level settings to f: weight
// (font-weight the resolver did not already serve) on a variable
// face's wght axis, font-variation-settings over it (CSS lets the
// explicit axis win), and the tabular twin when font-feature-settings
// asks for tnum. Every derived face is memoized, so the shaping cache
// keeps hitting.
func styledFace(f render.Font, v *style.Values, weight int) render.Font {
	vars := cascadeVariations(v, weight)
	tnum := v.Has(style.PropFontFeatures) && strings.Contains(v.Features, "tnum")
	letter := 0.0
	if v.Has(style.PropLetterSpacing) {
		letter = v.LetterSpacing
	}
	switch t := f.(type) {
	case *render.Typeface:
		t = t.WithVariations(vars...).Spaced(letter)
		if tnum {
			t = t.Tabular()
		}
		return t
	case *render.Chain:
		t = t.WithVariations(vars...).Spaced(letter)
		if tnum {
			t = t.Tabular()
		}
		return t
	}
	return f
}

// faceKey is what a cascade-styled face depends on.
type faceKey struct {
	base                 render.Font
	weight               int
	letter               float64
	features, variations string
}

// faceCache memoizes a widget's cascade-styled faces (styledFace) per
// base face and the cascade values that shape them, so the measuring
// and painting paths reuse one derived face without re-deriving it.
type faceCache struct{ faces map[faceKey]render.Font }

// get is base styled by v (weight: a font-weight the caller did not
// already serve, 0 for none).
func (c *faceCache) get(base render.Font, v *style.Values, weight int) render.Font {
	k := faceKey{base: base, weight: weight}
	if v.Has(style.PropLetterSpacing) {
		k.letter = v.LetterSpacing
	}
	if v.Has(style.PropFontFeatures) {
		k.features = v.Features
	}
	if v.Has(style.PropFontVariations) {
		k.variations = v.Variations
	}
	if f, ok := c.faces[k]; ok {
		return f
	}
	if c.faces == nil || len(c.faces) > 16 {
		c.faces = map[faceKey]render.Font{}
	}
	f := styledFace(base, v, weight)
	c.faces[k] = f
	return f
}

// cssDecoration is the cascade's text decoration, zero when none.
func cssDecoration(v *style.Values) render.Decoration {
	if v.Has(style.PropTextDecoration) {
		return v.Decoration
	}
	return render.Decoration{}
}

// cascadeVariations is the axis settings the cascade implies: wght
// from weight (0 for none), then font-variation-settings, a later tag
// replacing an earlier one.
func cascadeVariations(v *style.Values, weight int) []render.Variation {
	var out []render.Variation
	set := func(tag string, val float32) {
		for i := range out {
			if out[i].Tag == tag {
				out[i].Value = val
				return
			}
		}
		out = append(out, render.Variation{Tag: tag, Value: val})
	}
	if weight > 0 {
		set("wght", float32(weight))
	}
	if v.Has(style.PropFontVariations) {
		for kv := range strings.SplitSeq(v.Variations, ";") {
			tag, num, ok := strings.Cut(kv, "=")
			if f, err := strconv.ParseFloat(num, 32); ok && err == nil {
				set(tag, float32(f))
			}
		}
	}
	return out
}

// radiusOr returns the stylesheet's corner radii when any corner is
// set, else def on every corner.
func radiusOr(v *style.Values, def int) render.Corners {
	if !v.HasAny(style.PropBorderTopLeftRadius, style.PropBorderTopRightRadius,
		style.PropBorderBottomRightRadius, style.PropBorderBottomLeftRadius) {
		return render.UniformCorners(def)
	}
	r := v.Radius
	c := render.UniformCorners(def)
	if v.Has(style.PropBorderTopLeftRadius) {
		c.TopLeft = r.TopLeft
	}
	if v.Has(style.PropBorderTopRightRadius) {
		c.TopRight = r.TopRight
	}
	if v.Has(style.PropBorderBottomRightRadius) {
		c.BottomRight = r.BottomRight
	}
	if v.Has(style.PropBorderBottomLeftRadius) {
		c.BottomLeft = r.BottomLeft
	}
	return c
}

// paddingOr returns the stylesheet's padding per side where set, def
// elsewhere.
func paddingOr(v *style.Values, def render.Insets) render.Insets {
	if v.Has(style.PropPaddingTop) {
		def.Top = v.Padding.Top
	}
	if v.Has(style.PropPaddingRight) {
		def.Right = v.Padding.Right
	}
	if v.Has(style.PropPaddingBottom) {
		def.Bottom = v.Padding.Bottom
	}
	if v.Has(style.PropPaddingLeft) {
		def.Left = v.Padding.Left
	}
	return def
}

// borderOf returns the used border widths.
func borderOf(v *style.Values) render.Insets {
	b := v.EffBorder()
	return render.Insets{Top: b.Top, Right: b.Right, Bottom: b.Bottom, Left: b.Left}
}

// ringOr is the border widths the cascade declares, else a uniform
// px ring: the themed default ring a widget draws until a stylesheet
// takes the border over (check indicators, frames).
func ringOr(v *style.Values, px int) render.Insets {
	if v.HasAny(style.PropBorderTopWidth, style.PropBorderRightWidth, style.PropBorderBottomWidth, style.PropBorderLeftWidth) {
		return borderOf(v)
	}
	return render.UniformInsets(px)
}

// marginOf returns the margins.
func marginOf(v *style.Values) render.Insets {
	m := v.Margin
	return render.Insets{Top: m.Top, Right: m.Right, Bottom: m.Bottom, Left: m.Left}
}

// fontPx resolves the text size: the stylesheet's when matched, else
// the constructor default.
func fontPx(v *style.Values, def float64) float64 {
	if v.Has(style.PropFontSize) {
		return v.FontSize
	}
	return def
}

// typeElementName returns the widget type's element name: GTK's CSS
// node name where gelm has the GTK widget (an Icon is GtkImage's
// image, a Scroll GtkScrolledWindow's scrolledwindow, a TextArea
// GtkTextView's textview, a Slider GtkScale's scale, a List GtkListView's
// listview, a RichLabel a label), so a GTK stylesheet applies as
// written; else the lowercased Go type name. The list is pinned by
// TestCSSElementNames; a type missing from the switch matches no
// element selector — the same graceful no-op as an unknown name in a
// stylesheet.
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
		return "image"
	case *Image:
		return "image"
	case *Label:
		return "label"
	case *LevelBar:
		return "levelbar"
	case *List:
		return "listview"
	case *listRow:
		return "row"
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
		return "label"
	case *Scroll:
		return "scrolledwindow"
	case *Separator:
		return "separator"
	case *Slider:
		return "scale"
	case *Spinner:
		return "spinner"
	case *Stack:
		return "stack"
	case *Switch:
		return "switch"
	case *TextArea:
		return "textview"
	case *Toast:
		return "toast"
	default:
		return ""
	}
}
