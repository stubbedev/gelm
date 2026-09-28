// Prototype for the CSS design (docs/css.md, ticket #75): a minimal
// match+compute engine over a synthetic widget gallery, measuring what
// the real engine (ticket #76) must meet. Throwaway by design - the
// numbers live in the design doc and in bench output, the code does
// not ship.
//
// Run: go test ./bench/css-proto -bench . -benchmem
package main

import (
	"slices"
	"strings"
	"testing"
)

// node is a synthetic widget: the selector-relevant facts only.
type node struct {
	name    string
	id      string
	classes []string
	hover   bool
	focus   bool
	active  bool
	disbled bool
	parent  *node
	child   []*node
	style   computed
}

func (n *node) add(name string, classes ...string) *node {
	c := &node{name: name, classes: classes, parent: n}
	n.child = append(n.child, c)
	return c
}

type computed struct {
	color      uint32
	background uint32
	padding    int
	radius     int
	borderW    int
	borderCol  uint32
	fontSize   float64
	fontWeight int
	shadowBlur int
	minWidth   int
}

// simple is one compound-selector term.
type simple struct {
	element string
	classes []string
	id      string
	states  uint8 // 1 hover, 2 focus, 4 active, 8 disabled
	univ    bool
}

// selector is a compound sequence with combinators, left to right.
type selector struct {
	parts []simple
	combs []byte // parts-1 entries: ' ' descendant, '>' child
}

// decl is one property declaration with its parsed value.
type decl struct {
	prop  string
	color uint32
	num   int
}

// rule is one stylesheet rule with cached specificity.
type rule struct {
	sel      selector
	ids      int
	classes  int
	elements int
	decls    []decl
}

// stylesheet buckets rules by their rightmost simple term, the only
// buckets a widget can plausibly match.
type stylesheet struct {
	byElement map[string][]*rule
	byClass   map[string][]*rule
	byID      map[string][]*rule
	universal []*rule
}

func newStylesheet(rules []*rule) *stylesheet {
	s := &stylesheet{
		byElement: map[string][]*rule{},
		byClass:   map[string][]*rule{},
		byID:      map[string][]*rule{},
	}
	for _, r := range rules {
		right := r.sel.parts[len(r.sel.parts)-1]
		switch {
		case right.univ:
			s.universal = append(s.universal, r)
		case right.id != "":
			s.byID[right.id] = append(s.byID[right.id], r)
		case len(right.classes) > 0:
			for _, c := range right.classes {
				s.byClass[c] = append(s.byClass[c], r)
			}
		default:
			s.byElement[right.element] = append(s.byElement[right.element], r)
		}
	}
	return s
}

const (
	stHover = 1 << iota
	stFocus
	stActive
	stDisabled
)

func matchSimple(s *simple, n *node) bool {
	if s.univ {
		return true
	}
	if s.element != "" && s.element != n.name {
		return false
	}
	if s.id != "" && s.id != n.id {
		return false
	}
	for _, c := range s.classes {
		if !slices.Contains(n.classes, c) {
			return false
		}
	}
	if s.states&stHover != 0 && !n.hover {
		return false
	}
	if s.states&stFocus != 0 && !n.focus {
		return false
	}
	if s.states&stActive != 0 && !n.active {
		return false
	}
	if s.states&stDisabled != 0 && !n.disbled {
		return false
	}
	return true
}

func matchSelector(sel *selector, n *node) bool {
	if !matchSimple(&sel.parts[len(sel.parts)-1], n) {
		return false
	}
	cur := n
	for i := len(sel.parts) - 2; i >= 0; i-- {
		if sel.combs[i] == '>' {
			cur = cur.parent
			if cur == nil || !matchSimple(&sel.parts[i], cur) {
				return false
			}
			continue
		}
		anc := cur.parent
		for anc != nil && !matchSimple(&sel.parts[i], anc) {
			anc = anc.parent
		}
		if anc == nil {
			return false
		}
		cur = anc
	}
	return true
}

func (r *rule) rank() int { return r.ids<<16 | r.classes<<8 | r.elements }

func (s *stylesheet) computeStyle(n *node) computed {
	var c computed
	best := map[string]int{}
	for _, r := range s.candidatesFor(n) {
		if !matchSelector(&r.sel, n) {
			continue
		}
		rank := r.rank()
		for _, d := range r.decls {
			if best[d.prop] > rank {
				continue
			}
			best[d.prop] = rank
			applyProp(&c, d)
		}
	}
	return c
}

func applyProp(c *computed, d decl) {
	switch d.prop {
	case "color":
		c.color = d.color
	case "background-color":
		c.background = d.color
	case "padding":
		c.padding = d.num
	case "border-radius":
		c.radius = d.num
	case "border-width":
		c.borderW = d.num
	case "border-color":
		c.borderCol = d.color
	case "font-size":
		c.fontSize = float64(d.num)
	case "font-weight":
		c.fontWeight = d.num
	case "box-shadow-blur":
		c.shadowBlur = d.num
	case "min-width":
		c.minWidth = d.num
	}
}

func (s *stylesheet) candidatesFor(n *node) []*rule {
	out := s.universal
	if r := s.byElement[n.name]; r != nil {
		out = append(out, r...)
	}
	if n.id != "" {
		if r := s.byID[n.id]; r != nil {
			out = append(out, r...)
		}
	}
	for _, c := range n.classes {
		if r := s.byClass[c]; r != nil {
			out = append(out, r...)
		}
	}
	return out
}

func (s *stylesheet) restyle(n *node) {
	n.style = s.computeStyle(n)
	for _, c := range n.child {
		s.restyle(c)
	}
}

// gallery builds the standard showcase shape: a window box, two
// columns (controls, editor), a bottom row - ~200 nodes, 3 deep.
func gallery() *node {
	root := &node{name: "box", classes: []string{"window"}}
	left := root.add("box", "column")
	right := root.add("box", "column")
	bottom := root.add("box", "row")
	left.add("button", "primary")
	left.add("button", "destructive")
	left.add("switch")
	left.add("checkbox")
	left.add("slider")
	left.add("progressbar")
	for range 30 {
		right.add("entry", "field")
		right.add("textarea", "field")
		row := right.add("listrow")
		row.add("label")
		row.add("icon")
	}
	for range 20 {
		left.add("label")
	}
	for range 20 {
		bottom.add("button", "tool")
	}
	for range 20 {
		bottom.add("label", "status")
	}
	return root
}

func rules() []*rule {
	mk := func(spec string, ids, classes, elements int, decls ...decl) *rule {
		return &rule{sel: parseSel(spec), ids: ids, classes: classes, elements: elements, decls: decls}
	}
	col := func(p string, v uint32) decl { return decl{prop: p, color: v} }
	num := func(p string, v int) decl { return decl{prop: p, num: v} }
	return []*rule{
		mk("button", 0, 0, 1, col("background-color", 0x111111), num("border-radius", 8), col("color", 0xffffff), num("padding", 8)),
		mk("button:hover", 0, 1, 1, col("background-color", 0x222222)),
		mk("button:active", 0, 1, 1, col("background-color", 0x090909)),
		mk("button.destructive", 0, 1, 1, col("background-color", 0xaa0000)),
		mk("button.primary", 0, 1, 1, col("background-color", 0x0055bb), col("border-color", 0x0077ee)),
		mk("button:focus", 0, 1, 1, num("border-width", 2), col("border-color", 0x55aaff)),
		mk("entry, textarea", 0, 0, 1, col("background-color", 0x0c0c0c), num("padding", 6), num("border-radius", 6), num("min-width", 120)),
		mk("entry:focus", 0, 1, 1, num("border-width", 2), col("border-color", 0x55aaff)),
		mk("label", 0, 0, 1, col("color", 0xcccccc)),
		mk("label.status", 0, 1, 1, col("color", 0x888888)),
		mk("listrow", 0, 0, 1, num("padding", 4)),
		mk("listrow:hover", 0, 1, 1, col("background-color", 0x1a1a1a)),
		mk("listrow:hover listrow", 0, 1, 2, col("color", 0xffffff)),
		mk("box.window", 0, 1, 1, col("background-color", 0x161616), num("padding", 12)),
		mk("box.column", 0, 1, 1, num("padding", 8)),
		mk("box.column > button", 0, 1, 2, num("padding", 10)),
		mk("box.row > .tool", 0, 1, 1, col("background-color", 0x1e1e1e)),
		mk("slider", 0, 0, 1, col("background-color", 0x333333)),
		mk("progressbar", 0, 0, 1, num("border-radius", 4), col("background-color", 0x004488)),
		mk("switch", 0, 0, 1, num("border-radius", 12)),
		mk("checkbox", 0, 0, 1, num("border-radius", 4)),
		mk("icon", 0, 0, 1, col("color", 0xdddddd)),
		mk("textarea", 0, 0, 1, num("font-size", 13)),
		mk("menuitem", 0, 0, 1, num("padding", 6), col("color", 0xdddddd)),
		mk("menuitem:hover", 0, 1, 1, col("background-color", 0x224466)),
		mk("#sidebar", 1, 0, 0, col("background-color", 0x101014)),
		mk("*", 0, 0, 0, num("font-weight", 400)),
	}
}

// parseSel parses the prototype's spec notation: "a.b#c:hover > d e".
func parseSel(spec string) selector {
	var sel selector
	parts := splitKeep(spec)
	for i, part := range parts {
		if i > 0 {
			if part[0] == '>' {
				sel.combs = append(sel.combs, '>')
				part = part[1:]
			} else {
				sel.combs = append(sel.combs, ' ')
			}
		}
		sel.parts = append(sel.parts, parseSimple(part))
	}
	return sel
}

func splitKeep(spec string) []string {
	var out []string
	start := 0
	for i := 0; i < len(spec); i++ {
		if spec[i] == ' ' || spec[i] == '>' {
			if i > start {
				out = append(out, spec[start:i])
			}
			if spec[i] == '>' {
				out = append(out, ">")
			}
			start = i + 1
		}
	}
	if start < len(spec) {
		out = append(out, spec[start:])
	}
	return out
}

func parseSimple(s string) simple {
	const stop = ":.#"
	endToken := func(i int) int {
		j := i
		for j < len(s) && strings.IndexByte(stop, s[j]) < 0 {
			j++
		}
		return j
	}
	var sm simple
	for i := 0; i < len(s); {
		switch s[i] {
		case '.':
			j := endToken(i + 1)
			sm.classes = append(sm.classes, s[i+1:j])
			i = j
		case '#':
			j := endToken(i + 1)
			sm.id = s[i+1 : j]
			i = j
		case ':':
			j := endToken(i + 1)
			switch s[i+1 : j] {
			case "hover":
				sm.states |= stHover
			case "focus":
				sm.states |= stFocus
			case "active":
				sm.states |= stActive
			case "disabled":
				sm.states |= stDisabled
			}
			i = j
		default:
			j := endToken(i)
			if s[j-1] == '*' && j-1 == i {
				sm.univ = true
			} else {
				sm.element = s[i:j]
			}
			i = j
		}
	}
	return sm
}

var sinkComputed computed

func BenchmarkFullRestyle(b *testing.B) {
	ss := newStylesheet(rules())
	root := gallery()
	root.hover = true
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		ss.restyle(root)
	}
	sinkComputed = root.style
}

func BenchmarkSingleRestyle(b *testing.B) {
	ss := newStylesheet(rules())
	root := gallery()
	var leaf *node
	var walk func(n *node)
	walk = func(n *node) {
		if n.name == "listrow" && leaf == nil {
			leaf = n
		}
		for _, ch := range n.child {
			walk(ch)
		}
	}
	walk(root)
	ss.restyle(root)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		leaf.hover = !leaf.hover
		leaf.style = ss.computeStyle(leaf)
		leaf.hover = !leaf.hover
	}
	sinkComputed = leaf.style
}

func BenchmarkSteadyFrameReadsStyles(b *testing.B) {
	ss := newStylesheet(rules())
	root := gallery()
	ss.restyle(root)
	var sink uint32
	var walk func(n *node)
	walk = func(n *node) {
		sink += n.style.color + n.style.background
		for _, ch := range n.child {
			walk(ch)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		walk(root)
	}
	if sink == 0xffffffff {
		panic("impossible")
	}
}
