package style

import (
	"slices"

	"github.com/stubbedev/gelm/render"
)

// State is a widget's interactive state the selector subset matches
// with: the pseudo-classes :hover, :focus, :active, :disabled.
type State uint8

// The state bits, in specificity-neutral order (each counts as one
// class-level step in a selector's rank).
const (
	Hover State = 1 << iota
	Focus
	Active
	Disabled
)

// stateByName maps a pseudo-class name to its bit; anything the subset
// does not name fails the selector (the rule is skipped, warned).
func stateByName(name string) (State, bool) {
	switch name {
	case "hover":
		return Hover, true
	case "focus":
		return Focus, true
	case "active":
		return Active, true
	case "disabled":
		return Disabled, true
	default:
		return 0, false
	}
}

// Prop names one styleable property of the docs/css.md table.
type Prop uint8

// The properties, as bit indexes into Values.Set.
const (
	PropColor Prop = iota
	PropBackgroundColor
	PropPadding
	PropFontFamily
	PropFontSize
	PropFontWeight
	PropBorderRadius
	PropBorderWidth
	PropBorderColor
	PropBoxShadow
	PropMinWidth
	PropMinHeight
	numProps
)

// Target is one node's selector-relevant facts, filled by the widget
// layer for the node being styled and for each ancestor a combinator
// walk crosses.
type Target struct {
	Element string
	ID      string
	Classes []string
	State   State
}

// Node is one node of the widget tree as the matcher sees it.
type Node interface {
	// StyleTarget fills t with the node's selector facts.
	StyleTarget(t *Target)
	// StyleParent returns the parent node, or nil at the root.
	StyleParent() Node
}

// Values is the cascade result for one node: the winning declaration
// per property, with Set recording which properties the stylesheet
// provided (directly on the node or through inheritance). Consumers
// layer programmatic widget values above it and the theme under it.
type Values struct {
	Set uint32

	Color       render.Color
	Background  render.Color
	BorderColor render.Color
	ShadowColor render.Color
	Padding     int
	Radius      int
	BorderWidth int
	ShadowBlur  int
	MinWidth    int
	MinHeight   int
	FontFamily  string
	FontSize    float64
	FontWeight  int
}

// Has reports whether the property was set for this node.
func (v *Values) Has(p Prop) bool { return v.Set&(1<<p) != 0 }

// InheritFrom fills v's unset inherited properties — color and the
// font group, the CSS inheritance set — from parent's, marking them
// set. Properties outside the set never inherit. The root passes a
// zero parent: the theme-derived defaults stay consumer-side, so a
// widget whose cascade never matched paints exactly as before.
func (v *Values) InheritFrom(parent *Values) {
	inherit := func(p Prop, set func()) {
		if !v.Has(p) && parent.Has(p) {
			set()
			v.Set |= 1 << p
		}
	}
	inherit(PropColor, func() { v.Color = parent.Color })
	inherit(PropFontFamily, func() { v.FontFamily = parent.FontFamily })
	inherit(PropFontSize, func() { v.FontSize = parent.FontSize })
	inherit(PropFontWeight, func() { v.FontWeight = parent.FontWeight })
}

// Scratch holds one matcher's reusable buffers. Style work runs on the
// event-loop goroutine, so one Scratch per process serves every
// compute; it is not safe for concurrent use.
type Scratch struct {
	self Target
	anc  Target
	best [numProps]uint64
}

// Match computes v: the cascade of every rule of s that matches n.
// Matching consults only the index buckets the node's own element,
// classes, and id hit, so the cost is proportional to the rules that
// can plausibly apply, never to the stylesheet size. Match allocates
// nothing.
func (s *Sheet) Match(n Node, sc *Scratch, v *Values) {
	*v = Values{}
	if len(s.rules) == 0 {
		return
	}
	n.StyleTarget(&sc.self)
	clear(sc.best[:])

	sc.match(s.universal, s, n, v)
	if rules, ok := s.byElement[sc.self.Element]; ok {
		sc.match(rules, s, n, v)
	}
	if sc.self.ID != "" {
		if rules, ok := s.byID[sc.self.ID]; ok {
			sc.match(rules, s, n, v)
		}
	}
	for _, c := range sc.self.Classes {
		if rules, ok := s.byClass[c]; ok {
			sc.match(rules, s, n, v)
		}
	}
}

// match walks one bucket, fully matching each candidate rule and
// applying its declarations — the best-per-property rank comparison
// makes visiting order irrelevant.
func (sc *Scratch) match(rules []int32, s *Sheet, n Node, v *Values) {
	for _, idx := range rules {
		r := &s.rules[idx]
		if !sc.selectorMatches(&r.sel, n) {
			continue
		}
		for i := range r.decls {
			r.decls[i].apply(v, r.rank, &sc.best)
		}
	}
}

// selectorMatches evaluates sel against n: the rightmost compound
// against the node itself, then the combinators right to left across
// the ancestors — `>` one parent step, descendant any number.
func (sc *Scratch) selectorMatches(sel *selector, n Node) bool {
	if !sc.matchesPart(&sel.parts[len(sel.parts)-1], &sc.self) {
		return false
	}
	cur := n
	for i := len(sel.parts) - 2; i >= 0; i-- {
		parent := cur.StyleParent()
		if parent == nil {
			return false
		}
		parent.StyleTarget(&sc.anc)
		if sel.combs[i] == '>' {
			if !sc.matchesPart(&sel.parts[i], &sc.anc) {
				return false
			}
			cur = parent
			continue
		}
		for !sc.matchesPart(&sel.parts[i], &sc.anc) {
			cur = parent
			parent = parent.StyleParent()
			if parent == nil {
				return false
			}
			parent.StyleTarget(&sc.anc)
		}
		cur = parent
	}
	return true
}

// matchesPart tests one compound term against a filled target.
func (sc *Scratch) matchesPart(part *simple, t *Target) bool {
	if part.univ && part.element == "" && part.id == "" && len(part.classes) == 0 && part.state == 0 {
		return true
	}
	if part.element != "" && part.element != t.Element {
		return false
	}
	if part.id != "" && part.id != t.ID {
		return false
	}
	for _, c := range part.classes {
		if !slices.Contains(t.Classes, c) {
			return false
		}
	}
	return t.State&part.state == part.state
}
