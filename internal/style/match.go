package style

import (
	"slices"
	"strings"
)

// Attr is one app-set data attribute a widget carries: attribute
// selectors test it by name.
type Attr struct {
	Name  string
	Value string
}

// Target is one node's selector-relevant facts, filled by the widget
// layer for the node being styled and for each node a combinator walk
// crosses.
type Target struct {
	Element string
	ID      string
	Classes []string
	State   State
	// Role is the node's semantic role name (a11y), the [role=...]
	// selector's value.
	Role string
	// Attrs are the app-set data attributes (SetAttr), [name] and
	// [name=value] selectors' subject.
	Attrs []Attr
	// Inline is the node's own declaration block (a widget-scoped style,
	// GTK's per-widget provider), applied at InlinePriority. It is read
	// only for the node being styled.
	Inline         *Block
	InlinePriority int
}

// attrValue reads one attribute, the built-in names (id, class, role)
// included.
func (t *Target) attrValue(name string) (string, bool) {
	switch name {
	case "id":
		return t.ID, t.ID != ""
	case "class":
		return strings.Join(t.Classes, " "), len(t.Classes) > 0
	case "role":
		return t.Role, t.Role != ""
	}
	for i := range t.Attrs {
		if t.Attrs[i].Name == name {
			return t.Attrs[i].Value, true
		}
	}
	return "", false
}

// nodeIDer is the optional identity a Node can expose when one
// widget is wrapped in several adapters: identity then compares by
// it, not by adapter pointer.
type nodeIDer interface{ StyleID() any }

// sameNode reports whether two Node values wrap the same node.
func sameNode(a, b Node) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a == b {
		return true
	}
	ai, ok := a.(nodeIDer)
	if !ok {
		return false
	}
	bi, ok := b.(nodeIDer)
	return ok && ai.StyleID() == bi.StyleID()
}

// Node is one node of the widget tree as the matcher sees it.
type Node interface {
	// StyleTarget fills t with the node's selector facts.
	StyleTarget(t *Target)
	// StyleParent returns the parent node, or nil at the root.
	StyleParent() Node
	// StylePrev returns the previous visible sibling, or nil.
	StylePrev() Node
	// StylePosition returns the node's 1-based index among its visible
	// siblings and their count; a root reports (1, 1).
	StylePosition() (index, count int)
	// StyleTypedPosition returns the 1-based index among the visible
	// siblings of element name and their count (:nth-of-type and
	// friends).
	StyleTypedPosition(element string) (index, count int)
	// StyleChildren returns the visible children, or nil for a leaf;
	// :empty and :has() read it.
	StyleChildren() []Node
}

// Layer is one stylesheet in a cascade with its priority: GTK's
// provider priority, compared before specificity. Among equal
// priorities, a later layer wins ties.
type Layer struct {
	Sheet    *Sheet
	Priority int
}

// Priorities, GTK's STYLE_PROVIDER_PRIORITY_* values.
const (
	PriorityFallback    = 1
	PriorityTheme       = 200
	PrioritySettings    = 400
	PriorityApplication = 600
	PriorityUser        = 800
)

// rank packs (important, priority, ids, classes, elements, cascade
// layer, sheet, order) into one comparable value, most significant
// first: a `!important` declaration outranks every normal one, then
// the provider priority decides, then specificity, then the @layer
// order (255: unlayered, which outranks every layer), then the
// sheet's install position, then source order.
func rank(prio int, sp spec, layer, sheet, order int, imp bool) uint64 {
	clampTo := func(v, bits int) uint64 {
		limit := 1<<bits - 1
		return uint64(min(max(v, 0), limit))
	}
	r := clampTo(prio, 10)<<53 | clampTo(sp.ids, 6)<<47 | clampTo(sp.cls, 9)<<38 |
		clampTo(sp.els, 6)<<32 | clampTo(layer, 8)<<24 | clampTo(sheet, 6)<<18 | clampTo(order, 18)
	if imp {
		r |= 1 << 63
	}
	return r
}

// maxHasBudget bounds the :has() work of one compute: node visits a
// relational argument may spend before it stops matching
// (conservatively false). :has is the one selector that walks down
// into subtrees, so without a budget a sheet could make matching
// quadratic in tree size; with it, one compute stays O(rules +
// budget).
const maxHasBudget = 4096

// Scratch holds one matcher's reusable buffers. Style work runs on the
// event-loop goroutine, so one Scratch per process serves every
// compute; it is not safe for concurrent use.
type Scratch struct {
	self Target
	// bufs are the nested-matching targets: one per functional
	// pseudo-class nesting level (:is inside :has inside ...), so a
	// nested evaluation never clobbers the target it runs within.
	bufs [maxPseudoDepth + 2]Target
	// hasBudget is the remaining :has() node visits of this compute.
	hasBudget int
	// pseudo, when not empty, restricts the compute to the rules of
	// that pseudo-element (::before, ::after) instead of the element's
	// own rules.
	pseudo string
	// prevCounters is the previous sibling's counter scope the widget
	// layer supplies when counters are in play (Compute resets it).
	prevCounters *CounterScope
	// media is the environment the sheets' @media conditions evaluate
	// against, copied from the compute's Env.
	media Media

	best      [numProps]uint64
	win       [numProps]*decl
	bestLayer [numProps]uint8
	// lowRank/winLow/lowLayer track the best declaration strictly
	// below the winner's cascade layer — the value revert-layer rolls
	// back to.
	lowRank  [numProps]uint64
	winLow   [numProps]*decl
	lowLayer [numProps]uint8

	customs []customWin
	byName  map[string]int

	// metas are the winning content/counter-reset/counter-increment
	// declarations (metaByName their index), which apply outside the
	// longhand cascade.
	metas      []customWin
	metaByName map[string]int

	parsed []parsedDecl
	cx     ctx
	// layers is the compute's cascade input, kept for the define
	// fallback of var() lookups.
	layers []Layer
}

// SetPrevCounters supplies the previous sibling's counter scope, the
// widget layer's half of CSS's document-order counting; Compute
// consumes and clears it.
func (sc *Scratch) SetPrevCounters(cs *CounterScope) { sc.prevCounters = cs }

// customWin is the winning declaration of one custom property.
type customWin struct {
	d    *decl
	rank uint64
}

// parsedDecl caches one declaration's parse within a compute, so a
// shorthand winning several longhands parses once.
type parsedDecl struct {
	d  *decl
	v  Values
	ok bool
}

// collect runs the match phase: every rule of every layer that matches
// n offers its declarations; the per-longhand and per-custom-property
// winners land in sc.
func (sc *Scratch) collect(layers []Layer, n Node) {
	clear(sc.best[:])
	clear(sc.win[:])
	clear(sc.bestLayer[:])
	clear(sc.lowRank[:])
	clear(sc.winLow[:])
	clear(sc.lowLayer[:])
	sc.customs = sc.customs[:0]
	sc.metas = sc.metas[:0]
	sc.hasBudget = maxHasBudget
	if sc.byName == nil {
		sc.byName = map[string]int{}
	}
	clear(sc.byName)
	if sc.metaByName == nil {
		sc.metaByName = map[string]int{}
	}
	clear(sc.metaByName)
	n.StyleTarget(&sc.self)
	for li, l := range layers {
		s := l.Sheet
		if s == nil || len(s.rules) == 0 {
			continue
		}
		sc.bucket(s.universal, s, l.Priority, li, n)
		if rules, ok := s.byElement[sc.self.Element]; ok {
			sc.bucket(rules, s, l.Priority, li, n)
		}
		if sc.self.ID != "" {
			if rules, ok := s.byID[sc.self.ID]; ok {
				sc.bucket(rules, s, l.Priority, li, n)
			}
		}
		for _, c := range sc.self.Classes {
			if rules, ok := s.byClass[c]; ok {
				sc.bucket(rules, s, l.Priority, li, n)
			}
		}
	}
	if sc.pseudo == "" {
		if b := sc.self.Inline; b != nil {
			// Inline declarations match the node itself with no specificity,
			// like a `*` rule in a widget-scoped provider; they rank as the
			// last layer at their priority.
			for i := range b.decls {
				sc.offer(&b.decls[i], rank(sc.self.InlinePriority, spec{}, 255, 63, b.decls[i].order, b.decls[i].important), 255)
			}
		}
	}
}

// bucket walks one bucket, fully matching each candidate rule and
// offering its declarations — the best-per-property rank comparison
// makes visiting order irrelevant, and a rule filed once is visited at
// most once per compute.
func (sc *Scratch) bucket(rules []int32, s *Sheet, prio, layer int, n Node) {
	for _, idx := range rules {
		r := &s.rules[idx]
		if r.mq >= 0 && !mediaMatches(s.mediaQs[r.mq], &sc.media) {
			continue
		}
		if r.pseudo != sc.pseudo {
			continue
		}
		if !sc.selectorMatches(&r.sel, n) {
			continue
		}
		for i := range r.block.decls {
			d := &r.block.decls[i]
			sc.offer(d, rank(prio, r.spec, int(r.layer), layer, d.order, d.important), r.layer)
		}
	}
}

// offer records d for every longhand it covers where its rank wins,
// tracking also the best declaration below the winner's cascade layer
// (the revert-layer fallback).
func (sc *Scratch) offer(d *decl, rk uint64, layer uint8) {
	if d.meta {
		if i, ok := sc.metaByName[d.name]; ok {
			if rk >= sc.metas[i].rank {
				sc.metas[i] = customWin{d: d, rank: rk}
			}
			return
		}
		sc.metaByName[d.name] = len(sc.metas)
		sc.metas = append(sc.metas, customWin{d: d, rank: rk})
		return
	}
	if d.custom {
		if i, ok := sc.byName[d.name]; ok {
			if rk >= sc.customs[i].rank {
				sc.customs[i] = customWin{d: d, rank: rk}
			}
			return
		}
		sc.byName[d.name] = len(sc.customs)
		sc.customs = append(sc.customs, customWin{d: d, rank: rk})
		return
	}
	cov := d.covers
	offers := func(p Prop) {
		if rk >= sc.best[p] {
			if sc.win[p] != nil {
				if layer > sc.bestLayer[p] {
					sc.lowRank[p], sc.winLow[p], sc.lowLayer[p] = sc.best[p], sc.win[p], sc.bestLayer[p]
				} else if layer < sc.bestLayer[p] && sc.lowLayer[p] >= layer {
					sc.lowRank[p], sc.winLow[p], sc.lowLayer[p] = 0, nil, 0
				}
			}
			sc.best[p], sc.win[p], sc.bestLayer[p] = rk, d, layer
		} else if layer < sc.bestLayer[p] && rk >= sc.lowRank[p] {
			sc.lowRank[p], sc.winLow[p], sc.lowLayer[p] = rk, d, layer
		}
	}
	for p := Prop(0); p < propWordBits && (cov.lo>>p) != 0; p++ {
		if cov.lo&(1<<p) != 0 {
			offers(p)
		}
	}
	for i := Prop(0); i < numProps-propWordBits && (cov.hi>>i) != 0; i++ {
		if cov.hi&(1<<i) != 0 {
			offers(i + propWordBits)
		}
	}
}

// selectorMatches evaluates sel against n: the rightmost compound
// against the node itself, then the combinators right to left, with
// backtracking so `.a > .b .c` tries every `.b` ancestor.
func (sc *Scratch) selectorMatches(sel *selector, n Node) bool {
	if !sc.compoundMatches(&sel.parts[len(sel.parts)-1], &sc.self, 0, n) {
		return false
	}
	_, ok := sc.matchFrom(sel, len(sel.parts)-1, 0, n)
	return ok
}

// matchFrom checks parts[:i] given that parts[i] matched n, returning
// the node the leftmost compound matched.
func (sc *Scratch) matchFrom(sel *selector, i, d int, n Node) (Node, bool) {
	if i == 0 {
		return n, true
	}
	part := &sel.parts[i-1]
	switch sel.combs[i-1] {
	case '>':
		p := n.StyleParent()
		if p == nil || !sc.nodeMatches(part, d, p) {
			return nil, false
		}
		return sc.matchFrom(sel, i-1, d, p)
	case '+':
		p := n.StylePrev()
		if p == nil || !sc.nodeMatches(part, d, p) {
			return nil, false
		}
		return sc.matchFrom(sel, i-1, d, p)
	case '~':
		for p := n.StylePrev(); p != nil; p = p.StylePrev() {
			if sc.nodeMatches(part, d, p) {
				if root, ok := sc.matchFrom(sel, i-1, d, p); ok {
					return root, true
				}
			}
		}
		return nil, false
	default: // descendant
		for p := n.StyleParent(); p != nil; p = p.StyleParent() {
			if sc.nodeMatches(part, d, p) {
				if root, ok := sc.matchFrom(sel, i-1, d, p); ok {
					return root, true
				}
			}
		}
		return nil, false
	}
}

// nodeMatches fills the ancestor buffer for n and tests part.
func (sc *Scratch) nodeMatches(part *compound, d int, n Node) bool {
	n.StyleTarget(&sc.bufs[d])
	return sc.compoundMatches(part, &sc.bufs[d], d, n)
}

// matchesSub evaluates a nested selector list member (:not, :is,
// :where) against a target already filled for n.
func (sc *Scratch) matchesSub(sel *selector, t *Target, d int, n Node) bool {
	if d+1 >= len(sc.bufs) {
		return false
	}
	if !sc.compoundMatches(&sel.parts[len(sel.parts)-1], t, d+1, n) {
		return false
	}
	_, ok := sc.matchFrom(sel, len(sel.parts)-1, d+1, n)
	return ok
}

// compoundMatches tests one compound against a filled target.
func (sc *Scratch) compoundMatches(c *compound, t *Target, d int, n Node) bool {
	if c.element != "" && c.element != t.Element {
		return false
	}
	if c.id != "" && c.id != t.ID {
		return false
	}
	for _, cl := range c.classes {
		if !slices.Contains(t.Classes, cl) {
			return false
		}
	}
	if t.State&c.state != c.state {
		return false
	}
	if len(c.attrs) > 0 && !attrsMatch(c.attrs, t) {
		return false
	}
	if c.structs != 0 || len(c.nths) > 0 {
		if !sc.structsMatch(c, t, n) {
			return false
		}
	}
	for i := range c.not {
		if sc.matchesSub(&c.not[i], t, d, n) {
			return false
		}
	}
	for i := range c.isLists {
		ok := false
		for j := range c.isLists[i].sels {
			if sc.matchesSub(&c.isLists[i].sels[j], t, d, n) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for i := range c.has {
		if sc.hasMatches(&c.has[i], d, n) {
			return true
		}
	}
	return len(c.has) == 0
}

// attrsMatch tests the compound's attribute selectors.
func attrsMatch(attrs []attrTest, t *Target) bool {
	for _, a := range attrs {
		v, ok := t.attrValue(a.name)
		if !ok {
			return false
		}
		if a.op == 0 {
			continue
		}
		want, got := a.val, v
		if a.ci {
			want, got = strings.ToLower(want), strings.ToLower(got)
		}
		switch a.op {
		case '=':
			if got != want {
				return false
			}
		case '~':
			if !slices.Contains(strings.Fields(got), want) || want == "" {
				return false
			}
		case '|':
			if got != want && !strings.HasPrefix(got, want+"-") {
				return false
			}
		case '^':
			if want == "" || !strings.HasPrefix(got, want) {
				return false
			}
		case '$':
			if want == "" || !strings.HasSuffix(got, want) {
				return false
			}
		case '*':
			if want == "" || !strings.Contains(got, want) {
				return false
			}
		}
	}
	return true
}

// structsMatch tests the structural pseudo-classes and an+b tests.
func (sc *Scratch) structsMatch(c *compound, t *Target, n Node) bool {
	st := c.structs
	if st&structRoot != 0 && n.StyleParent() != nil {
		return false
	}
	if st&structEnabled != 0 && t.State&Disabled != 0 {
		return false
	}
	if st&structReadWrite != 0 && t.State&ReadOnly != 0 {
		return false
	}
	if st&structEmpty != 0 {
		if kids := n.StyleChildren(); len(kids) != 0 {
			return false
		}
	}
	if st&(structFirst|structLast|structOnly) != 0 || hasPlainNth(c.nths) {
		idx, cnt := n.StylePosition()
		if !positionOk(st, idx, cnt) || !nthsMatch(c.nths, idx, cnt, false) {
			return false
		}
	}
	if st&(structFirstOf|structLastOf|structOnlyOf) != 0 || hasTypeNth(c.nths) {
		idx, cnt := n.StyleTypedPosition(t.Element)
		if !positionOk(st>>5, idx, cnt) || !nthsMatch(c.nths, idx, cnt, true) {
			return false
		}
	}
	return true
}

func hasPlainNth(nths []nth) bool {
	for _, x := range nths {
		if !x.ofType {
			return true
		}
	}
	return false
}

func hasTypeNth(nths []nth) bool {
	for _, x := range nths {
		if x.ofType {
			return true
		}
	}
	return false
}

// positionOk tests the positional bits of st (already shifted to the
// of-type group when needed) against a 1-based index and count.
func positionOk(st structural, idx, cnt int) bool {
	if st&structFirst != 0 && idx != 1 {
		return false
	}
	if st&structLast != 0 && idx != cnt {
		return false
	}
	if st&structOnly != 0 && cnt != 1 {
		return false
	}
	return true
}

func nthsMatch(nths []nth, idx, cnt int, ofType bool) bool {
	for _, x := range nths {
		if x.ofType != ofType {
			continue
		}
		if !x.matches(idx, cnt) {
			return false
		}
	}
	return true
}

// hasMatches evaluates one :has() argument against its element n.
func (sc *Scratch) hasMatches(ha *hasArg, d int, n Node) bool {
	if d+1 >= len(sc.bufs) {
		return false
	}
	sel := &ha.sel
	visit := func(m Node) bool {
		if sc.hasBudget <= 0 {
			return false
		}
		sc.hasBudget--
		m.StyleTarget(&sc.bufs[d+1])
		if !sc.compoundMatches(&sel.parts[len(sel.parts)-1], &sc.bufs[d+1], d+1, m) {
			return false
		}
		root, ok := sc.matchFrom(sel, len(sel.parts)-1, d+1, m)
		return ok && sc.relates(ha.comb, root, n)
	}
	var walk func(kids []Node) bool
	walk = func(kids []Node) bool {
		for _, k := range kids {
			if visit(k) {
				return true
			}
			if sc.hasBudget <= 0 {
				return false
			}
			if walk(k.StyleChildren()) {
				return true
			}
		}
		return false
	}
	switch ha.comb {
	case '+', '~':
		for sib := nextSibling(n); sib != nil; sib = nextSibling(sib) {
			if visit(sib) || (sc.hasBudget > 0 && walk(sib.StyleChildren())) {
				return true
			}
		}
		return false
	default:
		return walk(n.StyleChildren())
	}
}

// relates reports whether anchor stands in comb's relation to scope,
// the :has element.
func (sc *Scratch) relates(comb byte, anchor, scope Node) bool {
	switch comb {
	case '>':
		return sameNode(anchor.StyleParent(), scope)
	case '+':
		return sameNode(anchor.StylePrev(), scope)
	case '~':
		for p := anchor.StylePrev(); p != nil; p = p.StylePrev() {
			if sameNode(p, scope) {
				return true
			}
		}
		return false
	default:
		for p := anchor.StyleParent(); p != nil; p = p.StyleParent() {
			if sameNode(p, scope) {
				return true
			}
		}
		return false
	}
}

// nextSibling returns the following visible sibling, or nil.
func nextSibling(n Node) Node {
	p := n.StyleParent()
	if p == nil {
		return nil
	}
	kids := p.StyleChildren()
	for i, k := range kids {
		if sameNode(k, n) && i+1 < len(kids) {
			return kids[i+1]
		}
	}
	return nil
}
