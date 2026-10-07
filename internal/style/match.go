package style

import (
	"slices"
)

// Target is one node's selector-relevant facts, filled by the widget
// layer for the node being styled and for each node a combinator walk
// crosses.
type Target struct {
	Element string
	ID      string
	Classes []string
	State   State
	// Inline is the node's own declaration block (a widget-scoped style,
	// GTK's per-widget provider), applied at InlinePriority. It is read
	// only for the node being styled.
	Inline         *Block
	InlinePriority int
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

// rank packs (priority, ids, classes, elements, layer, order) into one
// comparable value, most significant first.
func rank(prio int, sp spec, layer, order int) uint64 {
	clampTo := func(v, bits int) uint64 {
		limit := 1<<bits - 1
		return uint64(min(max(v, 0), limit))
	}
	return clampTo(prio, 10)<<54 | clampTo(sp.ids, 6)<<48 | clampTo(sp.cls, 10)<<38 |
		clampTo(sp.els, 6)<<32 | clampTo(layer, 8)<<24 | clampTo(order, 24)
}

// Scratch holds one matcher's reusable buffers. Style work runs on the
// event-loop goroutine, so one Scratch per process serves every
// compute; it is not safe for concurrent use.
type Scratch struct {
	self Target
	anc  Target

	best    [numProps]uint64
	win     [numProps]*decl
	customs []customWin
	byName  map[string]int

	parsed []parsedDecl
	cx     ctx
	// layers is the compute's cascade input, kept for the define
	// fallback of var() lookups.
	layers []Layer
}

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
	sc.customs = sc.customs[:0]
	if sc.byName == nil {
		sc.byName = map[string]int{}
	}
	clear(sc.byName)
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
	if b := sc.self.Inline; b != nil {
		// Inline declarations match the node itself with no specificity,
		// like a `*` rule in a widget-scoped provider; they rank as the
		// last layer at their priority.
		for i := range b.decls {
			sc.offer(&b.decls[i], rank(sc.self.InlinePriority, spec{}, 255, b.decls[i].order))
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
		if !sc.selectorMatches(&r.sel, n) {
			continue
		}
		for i := range r.block.decls {
			d := &r.block.decls[i]
			sc.offer(d, rank(prio, r.spec, layer, d.order))
		}
	}
}

// offer records d for every longhand it covers where its rank wins.
func (sc *Scratch) offer(d *decl, rk uint64) {
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
	for p := Prop(0); cov != 0; p++ {
		if cov&1 != 0 && rk >= sc.best[p] {
			sc.best[p] = rk
			sc.win[p] = d
		}
		cov >>= 1
	}
}

// selectorMatches evaluates sel against n: the rightmost compound
// against the node itself, then the combinators right to left, with
// backtracking so `.a > .b .c` tries every `.b` ancestor.
func (sc *Scratch) selectorMatches(sel *selector, n Node) bool {
	last := len(sel.parts) - 1
	if !sc.compoundMatches(&sel.parts[last], &sc.self, n) {
		return false
	}
	return sc.matchFrom(sel, last, n)
}

// matchFrom checks parts[:i] given that parts[i] matched n.
func (sc *Scratch) matchFrom(sel *selector, i int, n Node) bool {
	if i == 0 {
		return true
	}
	part := &sel.parts[i-1]
	switch sel.combs[i-1] {
	case '>':
		p := n.StyleParent()
		return p != nil && sc.nodeMatches(part, p) && sc.matchFrom(sel, i-1, p)
	case '+':
		p := n.StylePrev()
		return p != nil && sc.nodeMatches(part, p) && sc.matchFrom(sel, i-1, p)
	case '~':
		for p := n.StylePrev(); p != nil; p = p.StylePrev() {
			if sc.nodeMatches(part, p) && sc.matchFrom(sel, i-1, p) {
				return true
			}
		}
		return false
	default: // descendant
		for p := n.StyleParent(); p != nil; p = p.StyleParent() {
			if sc.nodeMatches(part, p) && sc.matchFrom(sel, i-1, p) {
				return true
			}
		}
		return false
	}
}

// nodeMatches fills the ancestor buffer for n and tests part.
func (sc *Scratch) nodeMatches(part *compound, n Node) bool {
	n.StyleTarget(&sc.anc)
	return sc.compoundMatches(part, &sc.anc, n)
}

// compoundMatches tests one compound against a filled target.
func (sc *Scratch) compoundMatches(c *compound, t *Target, n Node) bool {
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
	if c.structs != 0 || len(c.nths) > 0 {
		if c.structs&structRoot != 0 && n.StyleParent() != nil {
			return false
		}
		idx, cnt := n.StylePosition()
		if c.structs&structFirst != 0 && idx != 1 {
			return false
		}
		if c.structs&structLast != 0 && idx != cnt {
			return false
		}
		if c.structs&structOnly != 0 && cnt != 1 {
			return false
		}
		for _, x := range c.nths {
			if !x.matches(idx, cnt) {
				return false
			}
		}
	}
	for i := range c.not {
		if sc.compoundMatches(&c.not[i], t, n) {
			return false
		}
	}
	return true
}
