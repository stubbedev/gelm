// Package style is gelm's CSS engine: a tokenizer and parser for the
// GTK-flavored subset docs/css.md scopes, the rule index, and the
// matcher/cascader that computes one widget's style — custom properties
// with var() substitution, calc(), color-mix(), and per-longhand
// cascade across prioritized stylesheets and per-widget inline
// declarations. It knows nothing about widgets; the widget package
// fills the Target, walks the tree, and layers the result over the
// typed Theme.
//
// The error policy is CSS's: a malformed declaration drops that
// declaration, a malformed selector drops that rule, an unknown
// at-rule drops its block, and everything around them still applies.
// A declaration whose var() references make it invalid is invalid at
// computed-value time and behaves as `unset`. Warnings go to the
// injected library logger at Warn, silent by default — the same
// contract as SetTheme's contrast warnings.
package style

import (
	"fmt"
	"strings"

	"github.com/stubbedev/gelm/internal/logutil"
)

// parseWarn is the parse-warning sink: warnings route to the injected
// library logger at Warn level. Tests swap it to capture.
var parseWarn = func(msg string) { logutil.L().Warn(msg) }

// SetParseWarn installs f as the parse-warning sink; nil restores the
// library logger. One sink per process, like the widget package's
// theme warning sink.
func SetParseWarn(f func(msg string)) {
	if f == nil {
		f = func(msg string) { logutil.L().Warn(msg) }
	}
	parseWarn = f
}

// warnf reports one stylesheet problem at Warn and lets the parser
// carry on with the surrounding rules.
func warnf(format string, args ...any) {
	parseWarn("css: " + fmt.Sprintf(format, args...))
}

// cssWide is a CSS-wide keyword a declaration's whole value can be.
type cssWide uint8

const (
	wideNone cssWide = iota
	wideInherit
	wideInitial
	wideUnset
)

// decl is one parsed declaration: which longhands it covers, its value
// tokens, and whether they reference var() (then the value is parsed
// per node at compute time, after substitution).
type decl struct {
	name   string // the property name, or the custom property's --name
	custom bool
	covers PropSet
	parse  func([]token, *ctx, *Values) bool
	val    []token
	hasVar bool
	wide   cssWide
	// order is the declaration's source position within its sheet:
	// later wins among equal priority and specificity.
	order int
	// pre is the parsed value of a var-free declaration that read no
	// node-dependent input (em, currentColor, the inherited weight): it
	// is the same for every node, so compute copies it instead of
	// re-parsing.
	pre *Values
	// preRem is the rem base pre was computed with, zero when it read
	// none: a compute under another base re-parses.
	preRem float64
}

// Block is a parsed declaration list: one rule's body, or a widget's
// inline style.
type Block struct {
	decls []decl
}

// Len returns the number of valid declarations.
func (b *Block) Len() int {
	if b == nil {
		return 0
	}
	return len(b.decls)
}

// ParseDeclarations parses a bare declaration list — `color: red;
// --x: 1px` — the body a widget's inline style carries. Malformed
// declarations are warned about and dropped.
func ParseDeclarations(css string) *Block {
	p := parser{src: css}
	ts := tokenize(css)
	return &Block{decls: p.declarations(ts)}
}

// Sheet is a parsed stylesheet: the rules in source order plus the
// match index. It is immutable after Parse and safe for concurrent
// readers.
type Sheet struct {
	rules []rule
	// buckets key rules by their rightmost compound — the only buckets
	// a widget can plausibly match. A rule is filed under the id when
	// the rightmost compound names one, else its first class, else its
	// element, else the universal bucket.
	byElement map[string][]int32
	byClass   map[string][]int32
	byID      map[string][]int32
	universal []int32

	sens Sensitivity
}

// Sensitivity summarizes what a sheet's selectors read beyond a node's
// own facts, so a state or class flip restyles no more than it must: a
// state bit or class named left of a combinator makes descendants (or
// following siblings) depend on it.
type Sensitivity struct {
	// AncestorStates are the state bits some selector tests on a
	// non-rightmost compound.
	AncestorStates State
	// AncestorClasses are the classes some selector tests on a
	// non-rightmost compound.
	AncestorClasses map[string]bool
	// Siblings reports sibling combinators or structural pseudo-classes
	// anywhere: a sibling's add, removal, visibility, class, or state
	// can then restyle its neighbors.
	Siblings bool
}

// Sensitivity returns the sheet's selector summary.
func (s *Sheet) Sensitivity() *Sensitivity { return &s.sens }

// rule is one selector of one stylesheet rule with its specificity and
// declarations. A comma group compiles to one rule per selector, all
// sharing the block.
type rule struct {
	sel   selector
	spec  spec
	block *Block
}

// Parse compiles css into a Sheet. Malformed declarations and rules are
// warned about and skipped; the valid remainder still applies. An
// empty (or all-invalid) input yields an empty Sheet — never nil.
func Parse(css string) *Sheet {
	p := parser{src: css}
	p.stylesheet(tokenize(css))
	return &p.sheet
}

// parser walks the token stream once.
type parser struct {
	src   string
	sheet Sheet
	order int
}

// stylesheet consumes rule after rule to the end of the input.
func (p *parser) stylesheet(ts []token) {
	for i := 0; i < len(ts); {
		t := ts[i]
		switch t.kind {
		case tkWS, tkSemi:
			i++
		case tkRBrace:
			i++ // a stray closer: drop it, keep parsing
		case tkAt:
			i = p.atRule(ts, i)
		default:
			i = p.qualifiedRule(ts, i)
		}
	}
}

// atRule skips one at-rule: through its block, or to its semicolon.
// None of them participates in the subset; @keyframes and @media are
// common in GTK stylesheets and skip silently, the rest warn.
func (p *parser) atRule(ts []token, i int) int {
	name := ts[i].s
	j := i + 1
	for ; j < len(ts); j++ {
		switch ts[j].kind {
		case tkSemi:
			j++
			p.warnAt(name)
			return j
		case tkLBrace:
			end := blockEnd(ts, j)
			p.warnAt(name)
			return end
		case tkFunc, tkLParen, tkLBrack:
			j = blockEnd(ts, j) - 1
		}
	}
	p.warnAt(name)
	return j
}

func (p *parser) warnAt(name string) {
	switch name {
	case "keyframes", "media", "-gtk-keyframes":
		return
	}
	warnf("at-rule @%s ignored", name)
}

// qualifiedRule parses a selector list and its block starting at ts[i]
// and returns the index past it.
func (p *parser) qualifiedRule(ts []token, i int) int {
	start := i
	for i < len(ts) && ts[i].kind != tkLBrace {
		if ts[i].kind == tkSemi || ts[i].kind == tkRBrace {
			warnf("rule skipped: want '{' at %q", badText(rawText(p.src, ts[start:i+1])))
			return i + 1
		}
		if closer(ts[i].kind) != 0 {
			i = blockEnd(ts, i)
			continue
		}
		i++
	}
	if i >= len(ts) {
		warnf("rule skipped: want '{' at %q", badText(rawText(p.src, ts[start:])))
		return i
	}
	prelude := trimWS(ts[start:i])
	end := blockEnd(ts, i)
	if end > len(ts) || ts[end-1].kind != tkRBrace {
		warnf("rule skipped: unterminated block at %q", badText(rawText(p.src, prelude)))
		return len(ts)
	}
	body := ts[i+1 : end-1]
	sels, ok := parseSelectorList(p.src, prelude)
	if !ok {
		warnf("rule skipped: bad selector %q", badText(rawText(p.src, prelude)))
		return end
	}
	for _, t := range body {
		if t.kind == tkLBrace {
			// Nested rules are SCSS, not CSS: the whole rule goes.
			warnf("rule skipped: nested block in %q", badText(rawText(p.src, prelude)))
			return end
		}
	}
	if hasImportant(body) {
		warnf("rule skipped: !important in %q", badText(rawText(p.src, prelude)))
		return end
	}
	block := &Block{decls: p.declarations(body)}
	for _, sel := range sels {
		p.sheet.add(sel, block)
	}
	return end
}

// hasImportant reports a `!important` anywhere in a block: gelm has no
// important layer, so the rule is a parse error (docs/css.md).
func hasImportant(ts []token) bool {
	for i := 0; i+1 < len(ts); i++ {
		if ts[i].is('!') {
			j := i + 1
			for j < len(ts) && ts[j].kind == tkWS {
				j++
			}
			if j < len(ts) && ts[j].ident("important") {
				return true
			}
		}
	}
	return false
}

// declarations parses a declaration list, dropping malformed entries
// with a warning.
func (p *parser) declarations(ts []token) []decl {
	var out []decl
	for _, raw := range splitTop(ts, tkSemi) {
		raw = trimWS(raw)
		if len(raw) == 0 {
			continue
		}
		d, ok := p.declaration(raw)
		if ok {
			out = append(out, d)
		}
	}
	return out
}

// declaration parses `name: value`.
func (p *parser) declaration(ts []token) (decl, bool) {
	if ts[0].kind != tkIdent {
		warnf("declaration skipped: %s", badText(rawText(p.src, ts)))
		return decl{}, false
	}
	j := 1
	for j < len(ts) && ts[j].kind == tkWS {
		j++
	}
	if j >= len(ts) || ts[j].kind != tkColon {
		warnf("declaration skipped: %s", badText(rawText(p.src, ts)))
		return decl{}, false
	}
	name := ts[0].s
	val := trimWS(ts[j+1:])
	p.order++
	d := decl{name: name, val: val, hasVar: containsVar(val), wide: wideOf(val), order: p.order}
	if strings.HasPrefix(name, "--") {
		d.custom = true
		return d, true
	}
	lname := strings.ToLower(name)
	d.name = lname
	if lname == "all" {
		if d.wide == wideNone {
			warnf("declaration skipped: all: %s", badText(rawText(p.src, val)))
			return decl{}, false
		}
		d.covers = allProps
		return d, true
	}
	def, ok := propTable[lname]
	if !ok {
		if !ignoredProps[lname] {
			warnf("declaration skipped: unknown property %q", name)
		}
		return decl{}, false
	}
	d.covers, d.parse = def.covers, def.parse
	if len(val) == 0 {
		warnf("declaration skipped: %s has no value", name)
		return decl{}, false
	}
	if d.wide != wideNone || d.hasVar {
		return d, true
	}
	// A var-free value is validated now: CSS drops an invalid
	// declaration at parse time, so it never wins the cascade.
	scratch := initialValues()
	cx := ctx{rem: parseRem, em: parseRem, weight: 400}
	if !d.parse(val, &cx, &scratch) {
		warnf("declaration skipped: %s: %s", name, badText(rawText(p.src, val)))
		return decl{}, false
	}
	if !cx.dep {
		d.pre = &scratch
		if cx.usedRem {
			d.preRem = parseRem
		}
	}
	return d, true
}

// parseRem is the rem base declarations are validated (and, when
// node-independent, pre-computed) under: CSS's 16px default.
const parseRem = 16

// wideOf reports a CSS-wide keyword value.
func wideOf(val []token) cssWide {
	if len(val) != 1 || val[0].kind != tkIdent {
		return wideNone
	}
	switch strings.ToLower(val[0].s) {
	case "inherit":
		return wideInherit
	case "initial":
		return wideInitial
	case "unset", "revert", "revert-layer":
		return wideUnset
	}
	return wideNone
}

// containsVar reports a var() call anywhere in the value.
func containsVar(ts []token) bool {
	for _, t := range ts {
		if t.kind == tkFunc && t.s == "var" {
			return true
		}
	}
	return false
}

// add files one compiled rule into the index and folds its selector
// into the sensitivity summary.
func (s *Sheet) add(sel selector, block *Block) {
	idx := int32(len(s.rules))
	s.rules = append(s.rules, rule{sel: sel, spec: sel.spec(), block: block})
	for i := range sel.parts {
		part := &sel.parts[i]
		if part.structs != 0 || len(part.nths) > 0 {
			s.sens.Siblings = true
		}
		if i == len(sel.parts)-1 {
			continue
		}
		s.noteAncestor(part)
	}
	for _, c := range sel.combs {
		if c == '+' || c == '~' {
			s.sens.Siblings = true
		}
	}
	right := &sel.parts[len(sel.parts)-1]
	switch {
	case right.id != "":
		if s.byID == nil {
			s.byID = map[string][]int32{}
		}
		s.byID[right.id] = append(s.byID[right.id], idx)
	case len(right.classes) > 0:
		if s.byClass == nil {
			s.byClass = map[string][]int32{}
		}
		s.byClass[right.classes[0]] = append(s.byClass[right.classes[0]], idx)
	case right.element != "":
		if s.byElement == nil {
			s.byElement = map[string][]int32{}
		}
		s.byElement[right.element] = append(s.byElement[right.element], idx)
	default:
		s.universal = append(s.universal, idx)
	}
}

// noteAncestor records the facts a non-rightmost compound tests.
func (s *Sheet) noteAncestor(c *compound) {
	s.sens.AncestorStates |= c.state
	for _, cl := range c.classes {
		if s.sens.AncestorClasses == nil {
			s.sens.AncestorClasses = map[string]bool{}
		}
		s.sens.AncestorClasses[cl] = true
	}
	for i := range c.not {
		s.noteAncestor(&c.not[i])
	}
}

// badText truncates a source excerpt for a warning message.
func badText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}
