package style

import (
	"strconv"
	"strings"
)

// State is a widget's interactive state the selector subset matches
// with: one bit per state pseudo-class.
type State uint16

// The state bits, in specificity-neutral order (each counts as one
// class-level step in a selector's rank).
const (
	Hover State = 1 << iota
	Focus
	Active
	Disabled
	// FocusVisible is focus that arrived by keyboard traversal.
	FocusVisible
	// FocusWithin is set on the focused widget and all its ancestors.
	FocusWithin
	// Checked is a toggled-on control, or a menu button whose popup is
	// open (GTK's :checked).
	Checked
	// Selected is a selected row or item.
	Selected
	// Indeterminate is a check or progress in its mixed state.
	Indeterminate
	// Backdrop is a widget whose window is not the active one.
	Backdrop
	// ReadOnly is a text field the user cannot edit (:read-only).
	ReadOnly
	// PlaceholderShown is an empty field showing its placeholder
	// (:placeholder-shown).
	PlaceholderShown
)

// stateByName maps a pseudo-class name to its bit.
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
	case "focus-visible":
		return FocusVisible, true
	case "focus-within":
		return FocusWithin, true
	case "checked":
		return Checked, true
	case "selected":
		return Selected, true
	case "indeterminate":
		return Indeterminate, true
	case "backdrop":
		return Backdrop, true
	case "read-only":
		return ReadOnly, true
	case "placeholder-shown":
		return PlaceholderShown, true
	default:
		return 0, false
	}
}

// structural pseudo-classes: position among visible siblings, plus the
// negated-state forms :enabled and :read-write.
type structural uint16

const (
	structFirst structural = 1 << iota
	structLast
	structOnly
	structRoot
	structEmpty
	structFirstOf
	structLastOf
	structOnlyOf
	// structEnabled is :enabled, the negation of the Disabled state.
	structEnabled
	// structReadWrite is :read-write, the negation of ReadOnly.
	structReadWrite
)

// nth is one :nth-child(an+b) (and -last, -of-type) test.
type nth struct {
	a, b int
	last bool
	// ofType counts only siblings of the subject's element name.
	ofType bool
}

// matches reports whether the 1-based position satisfies an+b for some
// n >= 0.
func (x nth) matches(index, count int) bool {
	pos := index
	if x.last {
		pos = count - index + 1
	}
	if x.a == 0 {
		return pos == x.b
	}
	d := pos - x.b
	return d%x.a == 0 && d/x.a >= 0
}

// attrTest is one attribute selector: presence, or an operator against
// the attribute's value, with the optional case-insensitivity flag.
type attrTest struct {
	name string
	// op is 0 for a bare presence test, else one of = ~ | ^ $ *.
	op  byte
	val string
	ci  bool
}

// isList is one :is()/:where() argument list.
type isList struct {
	sels []selector
	// zero marks :where(), whose specificity is always 0.
	zero bool
}

// hasArg is one :has() relative selector argument: the combinator
// that anchors it to the :has element (' ' descendant, '>' child,
// '+' next sibling, '~' following sibling) and the selector list to
// its right.
type hasArg struct {
	comb byte
	sel  selector
}

// compound is one compound selector: an optional element name or `*`,
// then classes, id, attribute tests, state and structural
// pseudo-classes, :not() negations, :is()/:where() lists, and :has()
// relational arguments.
type compound struct {
	element string
	id      string
	classes []string
	attrs   []attrTest
	state   State
	structs structural
	nths    []nth
	// not holds the :not() alternatives, each a full complex selector:
	// the compound matches only when none of them does.
	not []selector
	// isLists holds :is()/:where() argument lists, all of which must
	// match.
	isLists []isList
	// has holds :has() relative arguments, one of which must match.
	has []hasArg
	// pseudo names a ::before/::after pseudo-element on this compound,
	// the subject of the rule's declarations.
	pseudo string
	// partElem names a desugared part pseudo-element (::placeholder,
	// ::selection, ::marker): the selector grows a descendant compound
	// of that element name, so the existing part machinery styles it.
	partElem string
}

// spec is a selector's specificity: ids, classes (with pseudo-classes,
// attribute tests, and functional pseudo-class arguments), and
// elements.
type spec struct{ ids, cls, els int }

func (c *compound) spec() spec {
	s := spec{cls: len(c.classes) + len(c.attrs) + bits(uint32(c.state)) + bits(uint32(c.structs)) + len(c.nths)}
	if c.id != "" {
		s.ids++
	}
	if c.element != "" {
		s.els++
	}
	// :not() and :is() count their most specific argument;
	// :where() counts nothing (Selectors Level 4).
	var best spec
	for i := range c.not {
		if ns := c.not[i].spec(); ns.greater(best) {
			best = ns
		}
	}
	for i := range c.isLists {
		if c.isLists[i].zero {
			continue
		}
		for j := range c.isLists[i].sels {
			if ns := c.isLists[i].sels[j].spec(); ns.greater(best) {
				best = ns
			}
		}
	}
	s.ids += best.ids
	s.cls += best.cls
	s.els += best.els
	var hbest spec
	for i := range c.has {
		if ns := c.has[i].sel.spec(); ns.greater(hbest) {
			hbest = ns
		}
	}
	s.ids += hbest.ids
	s.cls += hbest.cls
	s.els += hbest.els
	return s
}

func (s spec) greater(o spec) bool {
	if s.ids != o.ids {
		return s.ids > o.ids
	}
	if s.cls != o.cls {
		return s.cls > o.cls
	}
	return s.els > o.els
}

// bits counts the set bits.
func bits(s uint32) int {
	n := 0
	for ; s != 0; s &= s - 1 {
		n++
	}
	return n
}

// selector is a compound sequence with combinators, left to right.
type selector struct {
	parts []compound
	// combs holds len(parts)-1 entries: ' ' descendant, '>' child, '+'
	// adjacent sibling, '~' general sibling.
	combs []byte
}

func (s *selector) spec() spec {
	var out spec
	for i := range s.parts {
		p := s.parts[i].spec()
		out.ids += p.ids
		out.cls += p.cls
		out.els += p.els
	}
	return out
}

// selParser parses selectors from a rule prelude.
type selParser struct {
	src   string
	ts    []token
	pos   int
	depth int
}

func (p *selParser) peek() (token, bool) {
	if p.pos < len(p.ts) {
		return p.ts[p.pos], true
	}
	return token{}, false
}

func (p *selParser) skipWS() {
	for p.pos < len(p.ts) && p.ts[p.pos].kind == tkWS {
		p.pos++
	}
}

// parseSelectorList parses a comma-separated selector list; any invalid
// member invalidates the whole list, per CSS.
func parseSelectorList(src string, ts []token) ([]selector, bool) {
	return parseSelectorListAt(src, ts, 0)
}

// maxPseudoDepth bounds functional pseudo-class nesting (:is inside
// :has inside :is, ...): deeper arguments fail to parse, keeping
// pathological stylesheets from recursing.
const maxPseudoDepth = 8

func parseSelectorListAt(src string, ts []token, depth int) ([]selector, bool) {
	var out []selector
	for _, part := range splitTop(ts, tkComma) {
		part = trimWS(part)
		if len(part) == 0 {
			return nil, false
		}
		p := selParser{src: src, ts: part, depth: depth}
		sel, ok := p.selector()
		if !ok {
			return nil, false
		}
		out = append(out, sel)
	}
	return out, len(out) > 0
}

// selector parses compounds joined by combinators.
func (p *selParser) selector() (selector, bool) {
	var sel selector
	for {
		c, ok := p.compound()
		if !ok {
			return selector{}, false
		}
		if c.partElem != "" {
			// A desugared part pseudo-element (::placeholder, ...)
			// ends the selector: the part is the subject, a descendant
			// compound of that element name. A compound that was only the
			// pseudo-element contributes no part of its own.
			if compoundHasSimple(&c) {
				sel.parts = append(sel.parts, c)
				sel.combs = append(sel.combs, ' ')
			}
			sel.parts = append(sel.parts, compound{element: c.partElem})
			if p.pos < len(p.ts) {
				return selector{}, false
			}
			return sel, true
		}
		sel.parts = append(sel.parts, c)
		if c.pseudo != "" && p.pos < len(p.ts) {
			return selector{}, false
		}
		sawWS := p.pos < len(p.ts) && p.ts[p.pos].kind == tkWS
		p.skipWS()
		t, more := p.peek()
		if !more {
			return sel, true
		}
		comb := byte(' ')
		switch {
		case t.is('>'), t.is('+'), t.is('~'):
			comb = t.s[0]
			p.pos++
			p.skipWS()
		case !sawWS:
			return selector{}, false
		}
		sel.combs = append(sel.combs, comb)
	}
}

// compoundHasSimple reports whether the compound carries a simple
// selector beyond a part pseudo-element.
func compoundHasSimple(c *compound) bool {
	return c.element != "" || c.id != "" || len(c.classes) > 0 || len(c.attrs) > 0 ||
		c.state != 0 || c.structs != 0 || len(c.nths) > 0 || len(c.not) > 0 ||
		len(c.isLists) > 0 || len(c.has) > 0
}

// compound parses one compound selector. At least one simple selector is
// required.
func (p *selParser) compound() (compound, bool) {
	var c compound
	seen := false
	for p.pos < len(p.ts) {
		t := p.ts[p.pos]
		switch {
		case t.is('*'):
			if seen {
				return compound{}, false
			}
			p.pos++
		case t.kind == tkIdent:
			if seen {
				return compound{}, false
			}
			c.element = strings.ToLower(t.s)
			p.pos++
		case t.is('.'):
			p.pos++
			n, ok := p.peek()
			if !ok || n.kind != tkIdent {
				return compound{}, false
			}
			c.classes = append(c.classes, n.s)
			p.pos++
		case t.kind == tkHash:
			if c.id != "" {
				return compound{}, false
			}
			c.id = t.s
			p.pos++
		case t.kind == tkLBrack:
			a, ok := p.attr()
			if !ok {
				return compound{}, false
			}
			c.attrs = append(c.attrs, a)
		case t.kind == tkColon:
			p.pos++
			if !p.pseudo(&c) {
				return compound{}, false
			}
			if c.pseudo != "" || c.partElem != "" {
				return c, true
			}
		default:
			return c, seen
		}
		seen = true
	}
	return c, seen
}

// attr parses one attribute selector after a '['.
func (p *selParser) attr() (attrTest, bool) {
	p.pos++
	t, ok := p.peek()
	if !ok || t.kind != tkIdent {
		return attrTest{}, false
	}
	a := attrTest{name: t.s}
	p.pos++
	p.skipWS()
	t, ok = p.peek()
	if !ok {
		return attrTest{}, false
	}
	if t.kind == tkRBrack {
		p.pos++
		return a, true
	}
	if t.kind != tkDelim || len(t.s) != 1 {
		return attrTest{}, false
	}
	switch t.s[0] {
	case '=':
		a.op = '='
	case '~', '|', '^', '$', '*':
		p.pos++
		n, more := p.peek()
		if !more || !n.is('=') {
			return attrTest{}, false
		}
		a.op = t.s[0]
	default:
		return attrTest{}, false
	}
	p.pos++
	p.skipWS()
	t, ok = p.peek()
	if !ok || (t.kind != tkString && t.kind != tkIdent) {
		return attrTest{}, false
	}
	a.val = t.s
	p.pos++
	p.skipWS()
	if t, ok := p.peek(); ok && t.kind == tkIdent {
		switch strings.ToLower(t.s) {
		case "i":
			a.ci = true
			p.pos++
			p.skipWS()
		case "s":
			p.pos++
			p.skipWS()
		default:
			return attrTest{}, false
		}
	}
	if t, ok := p.peek(); !ok || t.kind != tkRBrack {
		return attrTest{}, false
	}
	p.pos++
	return a, true
}

// pseudo parses the pseudo-class after a ':' (or, after a second ':',
// a pseudo-element).
func (p *selParser) pseudo(c *compound) bool {
	t, ok := p.peek()
	if !ok {
		return false
	}
	if t.kind == tkColon {
		p.pos++
		return p.pseudoElem(c)
	}
	switch t.kind {
	case tkIdent:
		p.pos++
		name := strings.ToLower(t.s)
		if st, ok := stateByName(name); ok {
			c.state |= st
			return true
		}
		switch name {
		case "first-child":
			c.structs |= structFirst
		case "last-child":
			c.structs |= structLast
		case "only-child":
			c.structs |= structOnly
		case "root":
			c.structs |= structRoot
		case "empty":
			c.structs |= structEmpty
		case "first-of-type":
			c.structs |= structFirstOf
		case "last-of-type":
			c.structs |= structLastOf
		case "only-of-type":
			c.structs |= structOnlyOf
		case "enabled":
			c.structs |= structEnabled
		case "read-write":
			c.structs |= structReadWrite
		default:
			return false
		}
		return true
	case tkFunc:
		end := blockEnd(p.ts, p.pos)
		args := trimWS(funcArgs(p.ts[p.pos:end]))
		p.pos = end
		switch t.s {
		case "not":
			sels, ok := p.subList(args)
			if !ok {
				return false
			}
			c.not = append(c.not, sels...)
			return true
		case "is", "where", "-webkit-any", "-moz-any":
			sels, ok := p.subList(args)
			if !ok {
				return false
			}
			c.isLists = append(c.isLists, isList{sels: sels, zero: t.s == "where"})
			return true
		case "has":
			args, ok := p.hasArgs(args)
			if !ok {
				return false
			}
			c.has = append(c.has, args...)
			return true
		case "nth-child", "nth-last-child", "nth-of-type", "nth-last-of-type":
			x, ok := parseNth(rawText(p.src, nthArg(args)))
			if !ok {
				return false
			}
			x.last = t.s == "nth-last-child" || t.s == "nth-last-of-type"
			x.ofType = t.s == "nth-of-type" || t.s == "nth-last-of-type"
			c.nths = append(c.nths, x)
			return true
		}
	}
	return false
}

// pseudoElem parses a pseudo-element after '::'.
func (p *selParser) pseudoElem(c *compound) bool {
	t, ok := p.peek()
	if !ok || t.kind != tkIdent {
		return false
	}
	p.pos++
	switch strings.ToLower(t.s) {
	case "before":
		c.pseudo = "before"
	case "after":
		c.pseudo = "after"
	case "placeholder", "selection", "marker":
		c.partElem = strings.ToLower(t.s)
	default:
		return false
	}
	return true
}

// subList parses a functional pseudo-class's selector-list argument at
// one deeper nesting level.
func (p *selParser) subList(args []token) ([]selector, bool) {
	if p.depth >= maxPseudoDepth {
		return nil, false
	}
	return parseSelectorListAt(p.src, args, p.depth+1)
}

// hasArgs parses a :has() argument list: each member may lead with a
// relative combinator (default descendant).
func (p *selParser) hasArgs(args []token) ([]hasArg, bool) {
	if p.depth >= maxPseudoDepth {
		return nil, false
	}
	var out []hasArg
	for _, part := range splitTop(args, tkComma) {
		part = trimWS(part)
		comb := byte(' ')
		if len(part) > 0 && (part[0].is('>') || part[0].is('+') || part[0].is('~')) {
			comb = part[0].s[0]
			part = trimWS(part[1:])
		}
		if len(part) == 0 {
			return nil, false
		}
		sels, ok := parseSelectorListAt(p.src, part, p.depth+1)
		if !ok || len(sels) != 1 {
			return nil, false
		}
		out = append(out, hasArg{comb: comb, sel: sels[0]})
	}
	return out, len(out) > 0
}

// nthArg strips the `of S` clause (Selects Level 4's
// :nth-child(an+b of S)) this engine does not support yet, so the
// an+b microsyntax reads clean text.
func nthArg(ts []token) []token {
	for i := len(ts) - 1; i >= 0; i-- {
		if ts[i].ident("of") && i+1 < len(ts) {
			return ts[:i]
		}
	}
	return ts
}

// parseNth parses the an+b microsyntax: odd, even, an, b, an+b, an-b.
func parseNth(s string) (nth, bool) {
	s = strings.ToLower(strings.ReplaceAll(s, " ", ""))
	switch s {
	case "odd":
		return nth{a: 2, b: 1}, true
	case "even":
		return nth{a: 2, b: 0}, true
	}
	i := strings.IndexByte(s, 'n')
	if i < 0 {
		b, err := strconv.Atoi(s)
		return nth{b: b}, err == nil
	}
	var a int
	switch head := s[:i]; head {
	case "", "+":
		a = 1
	case "-":
		a = -1
	default:
		v, err := strconv.Atoi(head)
		if err != nil {
			return nth{}, false
		}
		a = v
	}
	var b int
	if tail := s[i+1:]; tail != "" {
		v, err := strconv.Atoi(tail)
		if err != nil {
			return nth{}, false
		}
		b = v
	}
	return nth{a: a, b: b}, true
}
