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
	default:
		return 0, false
	}
}

// structural pseudo-classes: position among visible siblings.
type structural uint8

const (
	structFirst structural = 1 << iota
	structLast
	structOnly
	structRoot
	structEmpty
)

// nth is one :nth-child(an+b) or :nth-last-child(an+b) test.
type nth struct {
	a, b int
	last bool
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

// compound is one compound selector: an optional element name or `*`,
// then classes, id, state and structural pseudo-classes, and :not()
// negations.
type compound struct {
	element string
	id      string
	classes []string
	state   State
	structs structural
	nths    []nth
	// not holds the :not() alternatives: the compound matches only when
	// none of them does.
	not []compound
}

// spec is a selector's specificity: ids, classes (with pseudo-classes
// and attribute-level tests), and elements.
type spec struct{ ids, cls, els int }

func (c *compound) spec() spec {
	s := spec{cls: len(c.classes) + bits(uint32(c.state)) + bits(uint32(c.structs)) + len(c.nths)}
	if c.id != "" {
		s.ids++
	}
	if c.element != "" {
		s.els++
	}
	// :not() counts its most specific argument (Selectors Level 4).
	var best spec
	for i := range c.not {
		if ns := c.not[i].spec(); ns.greater(best) {
			best = ns
		}
	}
	s.ids += best.ids
	s.cls += best.cls
	s.els += best.els
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
	src string
	ts  []token
	pos int
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
	var out []selector
	for _, part := range splitTop(ts, tkComma) {
		part = trimWS(part)
		if len(part) == 0 {
			return nil, false
		}
		p := selParser{src: src, ts: part}
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
		sel.parts = append(sel.parts, c)
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
		case t.kind == tkColon:
			p.pos++
			if !p.pseudo(&c) {
				return compound{}, false
			}
		default:
			return c, seen
		}
		seen = true
	}
	return c, seen
}

// pseudo parses the pseudo-class after a ':'.
func (p *selParser) pseudo(c *compound) bool {
	t, ok := p.peek()
	if !ok {
		return false
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
			for _, alt := range splitTop(args, tkComma) {
				alt = trimWS(alt)
				sub := selParser{src: p.src, ts: alt}
				nc, ok := sub.compound()
				if !ok || sub.pos != len(alt) {
					return false
				}
				c.not = append(c.not, nc)
			}
			return len(c.not) > 0
		case "nth-child", "nth-last-child":
			x, ok := parseNth(rawText(p.src, args))
			if !ok {
				return false
			}
			x.last = t.s == "nth-last-child"
			c.nths = append(c.nths, x)
			return true
		}
	}
	return false
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
