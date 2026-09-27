// Package widget is gelm's retained-mode widget layer: a tree of widgets
// measured against constraints, arranged into pixel rects, and painted onto
// a render.Canvas. Input handling arrives in M4; HitTest is already here so
// event dispatch can hook into the same geometry.
//
// # API conventions
//
// Constructors are positional, in one fixed order: the font first,
// then the pixel size (sizePx, logical, when the widget paints text at
// a configurable size), then the widget's content and initial state,
// then colors. A nil face panics in the constructor with a message
// naming the argument (see requireFace) — it never fails later, deep
// in shaping.
//
// Event hooks are exported function fields set after construction
// (Button.OnClick, Entry.OnChanged, Dropdown.OnSelect, ...), one per
// event, fired synchronously from whatever produced it. The Router
// methods and the handler interfaces they dispatch to (Clicker,
// KeyActionHandler, HoverSetter, ...) are input plumbing, not app
// hooks, and are not part of this convention.
//
// State comes in getter/setter pairs with no Get prefix: Text and
// SetText, On and SetOn, Alignment and SetAlignment. Every Set* whose
// state is app-meaningful has a matching getter; the router-facing
// Set* methods (SetHovered, SetPressed, SetDragOver) are interface
// plumbing, not state API. Bounds returns the last arranged rect, and
// the zero rect before the first Arrange — a defined value, never a
// panic.
//
// Every coordinate, size, and pixel parameter is logical pixels; the
// device scale is applied exactly once, at the buffer boundary in the
// app and render packages.
package widget

import (
	"slices"
	"sync/atomic"

	"github.com/stubbedev/gelm/render"
)

// Size is a width and height in pixels.
type Size struct {
	W, H int
}

// Point is a position in pixels.
type Point struct {
	X, Y int
}

// Constraints bounds the size a widget may claim during layout. Max is the
// hard ceiling; Min what the parent needs at least.
type Constraints struct {
	Min, Max Size
}

// Widget is one node in the retained widget tree. The passes run in order:
// Measure computes how much space a widget wants, Arrange assigns its final
// rect, Paint draws it.
type Widget interface {
	// Measure returns the size this widget wants under con. The result
	// must lie within con.
	Measure(con Constraints) Size
	// Arrange assigns the widget's final pixel rect and recurses into
	// children.
	Arrange(r render.Rect)
	// Paint draws the widget, honoring any clip already set on the
	// canvas.
	Paint(cv *render.Canvas)
	// HitTest returns the deepest widget whose area contains p, or nil
	// when p misses the tree below this widget.
	HitTest(p Point) Widget
}

// node carries the arranged bounds, parent link, and tooltip text
// shared by every implementation. Embed it; call HitLeaf from leaf
// HitTests and ArrangeRoot from implementations that position children
// themselves.
//
// It also carries the two caches the frame loop leans on: the
// invalidation flag the damage collector turns into repaint rects, and
// the memoized Measure result, dropped by InvalidateLayout when a
// mutation changes what a widget wants.
type node struct {
	bounds  render.Rect
	parent  Widget
	tooltip string
	// debugName labels the widget for the inspector's dump and
	// overlay; plain data the toolkit itself never reads.
	debugName string

	// paint damage: invalid marks the bounds as needing a repaint;
	// extras are additional rects (in root coordinates) a widget owes a
	// repaint for; subInvalid mirrors that somewhere below this widget
	// in the tree is dirty, so containers that do not expose children
	// still join the damage union.
	invalid    bool
	subInvalid bool
	extras     []render.Rect

	// measure cache: measuredIn/out memoize the last Measure call once
	// measureSeen; measureDirty forces a recompute after InvalidateLayout.
	measureSeen  bool
	measureDirty bool
	measureIn    Constraints
	measureOut   Size
	// measureCount tallies Measure entries; the zero-recursion pin test
	// reads it.
	measureCount int

	// themeSeen stamps the theme generation this widget last joined a
	// frame with; SetTheme bumps themeGen so every widget repaints once.
	themeSeen uint64

	// disabled is the widget's own half of the enable state: false (the
	// zero value) means enabled, so plain widgets accept input without
	// opting in. The other half is inherited: widget.IsEnabled walks the
	// ancestors, so a disabled container disables its subtree by query
	// — see SetEnabled.
	disabled bool
}

// SetTooltip sets hover text shown after a dwell; empty clears it.
func (n *node) SetTooltip(s string) {
	checkLoop("SetTooltip")
	n.tooltip = s
}

// TooltipText returns the hover text, empty when none is set.
func (n *node) TooltipText() string { return n.tooltip }

// SetEnabled turns the widget's own interactivity on or off; every
// node-embedding widget inherits it. The zero state is enabled.
//
// Propagation model (pinned): a container's flag does NOT rewrite its
// children's. Disable a Box and the whole subtree reads as disabled
// through widget.IsEnabled, the per-query ancestor walk — the same
// shape as IsInteractive. Per-query beats a recursive flag because it
// keeps every widget's own decision intact: re-enabling the container
// never resurrects a child the app disabled on purpose, and a child
// disabled after its container keeps its state when the container
// comes back. Each widget's own flag only ever changes through its own
// SetEnabled.
//
// The flip invalidates self and — for containers that expose Children,
// which override this with invalidateTree — every descendant, so the
// muted paint lands on the next frame without a relayout: enabled
// state changes what Paint draws, never what Measure wants.
func (n *node) SetEnabled(enabled bool) {
	checkLoop("SetEnabled")
	if n.disabled == !enabled {
		return
	}
	n.disabled = !enabled
	n.Invalidate()
}

// Enabled reports the widget's own flag, NOT the effective state: a
// widget inside a disabled container still reads true here. Input
// paths and Paint must consult widget.IsEnabled, which folds the
// ancestors in.
func (n *node) Enabled() bool { return !n.disabled }

// invalidateTree marks self and every descendant for repaint.
// Containers call it after a state flip (enable/disable) that the
// query walks project onto the whole subtree at once.
func invalidateTree(self Widget) {
	walkTree(self, 0, func(w Widget, _ int) {
		if v, ok := w.(interface{ Invalidate() }); ok {
			v.Invalidate()
		}
	})
}

// Invalidate schedules a repaint of the widget's arranged bounds. Call
// it after any state change that alters what Paint draws. Layout is
// untouched; mutations that change the wanted size need InvalidateLayout.
func (n *node) Invalidate() {
	checkLoop("Invalidate")
	n.invalid = true
	n.markSub()
}

// InvalidateRect schedules a repaint of r, an arbitrary rect in root
// coordinates - for widgets that owe pixels outside their arranged
// bounds, such as a scrollbar strip beside a viewport. Unlike
// Invalidate it does not mark the arranged bounds themselves.
func (n *node) InvalidateRect(r render.Rect) {
	checkLoop("InvalidateRect")
	if r.Empty() || slices.Contains(n.extras, r) {
		return
	}
	n.extras = append(n.extras, r)
	n.markSub()
}

// markSub flags ancestors that some descendant needs a repaint, so
// the damage collector notices through containers that hide their
// children (a virtualized List, say). Parent links come from the last
// Arrange. The walk stops at the first ancestor that exposes Children:
// the collector descends through those anyway, so flagging them would
// only widen the damage without hiding anything.
func (n *node) markSub() {
	p := n.parent
	for p != nil {
		if _, ok := p.(childser); ok {
			return
		}
		s, ok := p.(interface{ markSubInvalid() bool })
		if !ok {
			return
		}
		if s.markSubInvalid() {
			return
		}
		p = parentOf(p)
	}
}

// markSubInvalid records a descendant invalidation and reports whether
// it was already recorded (so the ancestor walk can stop early).
func (n *node) markSubInvalid() bool {
	was := n.subInvalid
	n.subInvalid = true
	return was
}

// InvalidateLayout drops the cached Measure result here and in every
// ancestor, so the next frame remeasures the affected branch only, and
// schedules a repaint of the bounds (a size change usually alters the
// painting too).
func (n *node) InvalidateLayout() {
	n.measureDirty = true
	n.Invalidate()
	n.markSubLayout()
}

// markSubLayout propagates the measure drop upward: every container
// cache between here and the root depends on this widget's natural
// size, whatever its child exposure.
func (n *node) markSubLayout() {
	p := n.parent
	for p != nil {
		s, ok := p.(interface{ markMeasureDirty() bool })
		if !ok {
			return
		}
		if s.markMeasureDirty() {
			return
		}
		p = parentOf(p)
	}
}

// markMeasureDirty drops this widget's cached measure because a
// descendant needs remeasuring; reports whether it was already dropped.
func (n *node) markMeasureDirty() bool {
	was := n.measureDirty
	n.measureDirty = true
	return was
}

// measureHit returns the cached natural size when it is still valid
// for con. Widget Measures open with it to skip recomputation on the
// static-tree fast path.
func (n *node) measureHit(con Constraints) (Size, bool) {
	n.measureCount++
	if n.measureSeen && !n.measureDirty && n.measureIn == con {
		return n.measureOut, true
	}
	return Size{}, false
}

// measureCalls reports how many times Measure entered this widget; the
// zero-measure-recursion pin test reads it.
func (n *node) measureCalls() int { return n.measureCount }

// measureStore records the computed size as the cache entry for con
// and returns it, closing the measureHit pair.
func (n *node) measureStore(con Constraints, s Size) Size {
	n.measureSeen = true
	n.measureDirty = false
	n.measureIn = con
	n.measureOut = s
	return s
}

// takeDamage drains one widget's pending repaint: it reports the
// arranged bounds when the bounds themselves are owed (flag set, a
// descendant hidden from the collector is dirty, or the theme changed)
// plus any extra rects, and clears the flags. Extras alone - scrollbar
// strips beside a viewport, say - do not drag the whole bounds in. The
// damage collector calls it walking down the tree.
func (n *node) takeDamage() (bounds render.Rect, extra []render.Rect, dirty bool) {
	boundsDirty := n.invalid || n.subInvalid || n.themeSeen != themeGen
	n.invalid = false
	n.subInvalid = false
	n.themeSeen = themeGen
	extra, n.extras = n.extras, nil
	if !boundsDirty && len(extra) == 0 {
		return render.Rect{}, nil, false
	}
	if !boundsDirty {
		return render.Rect{}, extra, true
	}
	return n.bounds, extra, true
}

// Arrange records the widget's rect. A rect that moved or resized from
// the previous frame also invalidates the old pixels, so partial
// damage does not leave the widget's old paint behind.
func (n *node) Arrange(r render.Rect) {
	if !n.bounds.Empty() && r != n.bounds {
		n.InvalidateRect(n.bounds)
		n.Invalidate()
	}
	n.bounds = r
}

// Bounds returns the last arranged rect. Before the first Arrange —
// and that includes a widget built but never put in a tree — it is the
// zero rect, a defined value, never a panic and never stale geometry.
func (n *node) Bounds() render.Rect {
	return n.bounds
}

// Parent returns the container that arranged this widget, or nil for the
// tree root.
func (n *node) Parent() Widget {
	return n.parent
}

// setParent records the arranging container; containers call it on their
// children during Arrange.
func (n *node) setParent(p Widget) {
	n.parent = p
}

// parentOf returns w's parent, or nil.
func parentOf(w Widget) Widget {
	if p, ok := w.(interface{ Parent() Widget }); ok {
		return p.Parent()
	}
	return nil
}

// setParents records parent as the arranging container of every child.
func setParents(parent Widget, kids ...Widget) {
	for _, k := range kids {
		if k == nil {
			continue
		}
		if s, ok := k.(interface{ setParent(Widget) }); ok {
			s.setParent(parent)
		}
	}
}

// clearParents drops the recorded parent of every ws — the bookkeeping
// a removal owes: a detached widget must stop claiming its old
// ancestor, or the IsEnabled/markSub walks (and a re-Append elsewhere)
// would read a tree it no longer belongs to.
func clearParents(ws ...Widget) {
	for _, w := range ws {
		if w == nil {
			continue
		}
		if s, ok := w.(interface{ setParent(Widget) }); ok {
			s.setParent(nil)
		}
	}
}

// removedHook is the callback the tree-mutation API (Box.Remove
// family, Stack.Remove, Scroll.SetChild, Grid.Remove, Notebook.CloseTab)
// fires with each detached widget. The Application installs one that
// drops the widget from every window router's hover, press, focus, and
// drop-target state, so input never reaches a ghost — and through
// router.Hovered() neither does a tooltip dwell (app's tooltipCtl).
var removedHook atomic.Pointer[func(Widget)]

// SetRemovedHook installs the hook fired when a widget is detached from
// its container; nil disables. One hook per process, like SetInvoker —
// the last install wins. It runs on the goroutine that mutated the tree
// (the loop goroutine, per docs/threading.md), while the widget is
// still linked to its container, so a router can find the focus
// traversal neighbor of a removed focus.
func SetRemovedHook(fn func(Widget)) {
	if fn == nil {
		removedHook.Store(nil)
		return
	}
	removedHook.Store(&fn)
}

// notifyRemoved fires the removal hook for every non-nil ws.
func notifyRemoved(ws ...Widget) {
	fn := removedHook.Load()
	if fn == nil {
		return
	}
	for _, w := range ws {
		if w != nil {
			(*fn)(w)
		}
	}
}

// HitLeaf returns the widget when p falls inside its bounds, else nil. It
// is the leaf implementation of HitTest.
func (n *node) HitLeaf(self Widget, p Point) Widget {
	if n.bounds.Contains(p.X, p.Y) {
		return self
	}
	return nil
}

// clampSize pins s to con.
func clampSize(s Size, con Constraints) Size {
	if s.W < con.Min.W {
		s.W = con.Min.W
	}
	if s.H < con.Min.H {
		s.H = con.Min.H
	}
	if con.Max.W < s.W {
		s.W = con.Max.W
	}
	if con.Max.H < s.H {
		s.H = con.Max.H
	}
	return s
}
