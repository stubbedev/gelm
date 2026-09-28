// Package style is gelm's CSS engine: a hand-rolled parser for the
// selector and property subset docs/css.md scopes, the rule index,
// and the matcher/cascader that computes one widget's style. It knows
// nothing about widgets; the widget package fills the Target, walks
// the ancestor chain, and layers the cascade result over the typed
// Theme.
//
// The error policy is warn-and-skip-rule: a malformed declaration
// skips that declaration, a malformed selector skips that rule, and
// everything around either still applies. Warnings go to the injected
// library logger at Warn, silent by default — the same contract as
// SetTheme's contrast warnings.
package style

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/stubbedev/gelm/internal/logutil"
	"github.com/stubbedev/gelm/render"
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

// Sheet is a parsed stylesheet: the rules in source order plus the
// match index. It is immutable after Parse and safe for concurrent
// readers.
type Sheet struct {
	rules []rule
	// buckets key rules by their rightmost simple selector — the only
	// buckets a widget can plausibly match. A rule is filed under every
	// class or element its rightmost compound names.
	byElement map[string][]int32
	byClass   map[string][]int32
	byID      map[string][]int32
	universal []int32
}

// Parse compiles css into a Sheet. Malformed declarations and rules are
// warned about and skipped; the valid remainder still applies. An
// empty (or all-invalid) input yields an empty Sheet — never nil.
func Parse(css string) *Sheet {
	p := parser{src: css}
	p.stylesheet()
	return &p.sheet
}

// rule is one selector of one stylesheet rule with its cached
// specificity rank (source order breaks ties) and parsed declarations.
// A comma group compiles to one rule per selector, all sharing the
// declarations.
type rule struct {
	sel   selector
	rank  uint64
	decls []decl
}

// selector is a compound sequence with combinators, left to right.
type selector struct {
	parts []simple
	// combs holds len(parts)-1 entries: ' ' descendant, '>' child.
	combs []byte
}

// simple is one compound-selector term.
type simple struct {
	element string
	id      string
	classes []string
	state   State
	univ    bool
}

// decl is one property declaration with its parsed value.
type decl struct {
	prop  Prop
	color render.Color
	num   int
	flt   float64
	str   string
}

// apply writes the declaration into v when rank still wins prop —
// the whole cascade is this one comparison: higher specificity wins,
// source order breaks ties, and within one rule a later declaration
// of the same property wins over an earlier (equal-rank) one.
func (d *decl) apply(v *Values, rank uint64, best *[numProps]uint64) {
	if rank < best[d.prop] {
		return
	}
	best[d.prop] = rank
	v.Set |= 1 << d.prop
	switch d.prop {
	case PropColor:
		v.Color = d.color
	case PropBackgroundColor:
		v.Background = d.color
	case PropPadding:
		v.Padding = d.num
	case PropFontFamily:
		v.FontFamily = d.str
	case PropFontSize:
		v.FontSize = d.flt
	case PropFontWeight:
		v.FontWeight = d.num
	case PropBorderRadius:
		v.Radius = d.num
	case PropBorderWidth:
		v.BorderWidth = d.num
	case PropBorderColor:
		v.BorderColor = d.color
	case PropBoxShadow:
		v.ShadowColor = d.color
		v.ShadowBlur = d.num
	case PropMinWidth:
		v.MinWidth = d.num
	case PropMinHeight:
		v.MinHeight = d.num
	}
}

// parser walks the input once, byte-oriented like the markup parser.
type parser struct {
	src   string
	pos   int
	sheet Sheet
}

// stylesheet consumes rule after rule to the end of the input.
func (p *parser) stylesheet() {
	for {
		p.skipInsignificant()
		if p.pos >= len(p.src) {
			return
		}
		if p.src[p.pos] == '}' {
			// A stray closer: drop it, keep parsing.
			p.pos++
			continue
		}
		p.rule()
	}
}

// rule parses one rule: a selector group and its declaration block.
// Any selector-level failure (including out-of-subset syntax and
// `!important`) warns and skips to past the block, leaving the rest of
// the stylesheet intact.
func (p *parser) rule() {
	start := p.pos
	sels, ok := p.selectorGroup()
	if !ok {
		p.warnSkip("rule skipped: bad selector", start)
		return
	}
	p.skipInsignificant()
	if !p.eat('{') {
		p.warnSkip("rule skipped: want '{'", start)
		return
	}
	decls, ok := p.declarations()
	if !ok {
		p.warnSkip("rule skipped", start)
		return
	}
	for _, sel := range sels {
		p.sheet.add(sel, decls)
	}
}

// add files one compiled rule into the index. Unknown element names
// are fine — they simply never match.
func (s *Sheet) add(sel selector, decls []decl) {
	idx := int32(len(s.rules))
	s.rules = append(s.rules, rule{sel: sel, rank: rank(sel, int(idx)), decls: decls})
	right := &sel.parts[len(sel.parts)-1]
	switch {
	case right.univ && right.element == "" && right.id == "" && len(right.classes) == 0:
		s.universal = append(s.universal, idx)
	case right.id != "":
		if s.byID == nil {
			s.byID = map[string][]int32{}
		}
		s.byID[right.id] = append(s.byID[right.id], idx)
	case len(right.classes) > 0:
		if s.byClass == nil {
			s.byClass = map[string][]int32{}
		}
		for _, c := range right.classes {
			s.byClass[c] = append(s.byClass[c], idx)
		}
	default:
		if s.byElement == nil {
			s.byElement = map[string][]int32{}
		}
		s.byElement[right.element] = append(s.byElement[right.element], idx)
	}
}

// rank packs (ids, classes+state, elements, source order) into one
// comparable value: lexicographic on the first three, source order
// breaking ties. A universal `*` counts nothing, per CSS.
func rank(sel selector, order int) uint64 {
	var ids, cls, els int
	for i := range sel.parts {
		pt := &sel.parts[i]
		if pt.id != "" {
			ids++
		}
		cls += len(pt.classes) + bits(pt.state)
		if pt.element != "" {
			els++
		}
	}
	return uint64(ids)<<48 | uint64(cls)<<32 | uint64(els)<<16 | uint64(order)&0xffff
}

// bits counts the set state bits — each state pseudo-class adds one
// class-level specificity step.
func bits(s State) int {
	n := 0
	for ; s != 0; s &= s - 1 {
		n++
	}
	return n
}

// selectorGroup parses comma-separated selectors.
func (p *parser) selectorGroup() ([]selector, bool) {
	var out []selector
	for {
		sel, ok := p.selector()
		if !ok {
			return nil, false
		}
		out = append(out, sel)
		p.skipInsignificant()
		if !p.eat(',') {
			return out, true
		}
	}
}

// selector parses compound terms joined by descendant (whitespace) or
// child (`>`) combinators.
func (p *parser) selector() (selector, bool) {
	var sel selector
	for {
		p.skipInsignificant()
		if p.pos < len(p.src) && p.src[p.pos] == '>' {
			if len(sel.parts) == 0 {
				return selector{}, false
			}
			sel.combs = append(sel.combs, '>')
			p.pos++
			p.skipInsignificant()
		} else if len(sel.parts) > 0 {
			sel.combs = append(sel.combs, ' ')
		}
		part, ok := p.compound()
		if !ok {
			return selector{}, false
		}
		sel.parts = append(sel.parts, part)
		// A term may run straight into ',' or '{'; peek past
		// whitespace handled by the caller loops.
		p.skipInsignificant()
		if p.pos >= len(p.src) {
			return selector{}, false
		}
		switch p.src[p.pos] {
		case ',', '{':
			return sel, true
		case '>':
			// loop: combinator handled at the top
		default:
			// more compound terms follow (descendant combinator)
		}
	}
}

// compound parses one term: an optional element name or `*`, then any
// number of `.class`, `#id`, and `:state` pieces. At least one piece
// is required.
func (p *parser) compound() (simple, bool) {
	var sm simple
	seen := false
	for p.pos < len(p.src) {
		switch c := p.src[p.pos]; {
		case c == '*':
			if seen {
				return simple{}, false
			}
			sm.univ = true
			seen = true
			p.pos++
		case isIdentStart(c):
			if seen {
				return simple{}, false
			}
			name, ok := p.ident()
			if !ok || name == "" {
				return simple{}, false
			}
			sm.element = strings.ToLower(name)
			seen = true
		case c == '.':
			p.pos++
			name, ok := p.ident()
			if !ok || name == "" {
				return simple{}, false
			}
			sm.classes = append(sm.classes, name)
			seen = true
		case c == '#':
			p.pos++
			name, ok := p.ident()
			if !ok || name == "" {
				return simple{}, false
			}
			if sm.id != "" {
				return simple{}, false
			}
			sm.id = name
			seen = true
		case c == ':':
			p.pos++
			if p.pos < len(p.src) && p.src[p.pos] == ':' {
				return simple{}, false // pseudo-elements are out of the subset
			}
			name, ok := p.ident()
			if !ok {
				return simple{}, false
			}
			st, ok := stateByName(strings.ToLower(name))
			if !ok {
				return simple{}, false
			}
			sm.state |= st
			seen = true
		default:
			if !seen {
				return simple{}, false
			}
			return sm, true
		}
	}
	if !seen {
		return simple{}, false
	}
	return sm, true
}

// declarations parses the block's declarations until '}' or EOF. A
// malformed declaration warns and skips to the next ';' — the rule
// survives. `!important` and an unterminated block are rule-level
// errors.
func (p *parser) declarations() ([]decl, bool) {
	var out []decl
	for {
		p.skipInsignificant()
		if p.pos >= len(p.src) {
			return nil, false // unterminated block
		}
		if p.src[p.pos] == '}' {
			p.pos++
			return out, true
		}
		start := p.pos
		d, ok := p.declaration()
		if !ok {
			// A malformed declaration takes only itself down; an
			// `!important` in the skipped span takes the whole rule.
			if !p.skipDeclaration(start) {
				return nil, false
			}
			continue
		}
		out = append(out, d)
	}
}

// skipDeclaration skips the remainder of a malformed declaration, to
// the next ';' or the block's closing brace (left for the caller), and
// reports whether the rule may continue: a `!important` anywhere in
// the declaration is a rule-level parse error.
func (p *parser) skipDeclaration(start int) bool {
	bang := strings.IndexByte(p.src[start:p.pos], '!')
	for p.pos < len(p.src) {
		switch c := p.src[p.pos]; c {
		case ';':
			p.pos++
			if bang < 0 {
				warnf("declaration skipped: %s", badText(p.src[start:p.pos]))
				return true
			}
			return false
		case '}':
			if bang < 0 {
				warnf("declaration skipped: %s", badText(p.src[start:p.pos]))
			}
			return false // '}' (or the !important) ends the rule
		case '!':
			bang = 0
		}
		p.pos++
	}
	return false // unterminated block
}

// declaration parses `prop: value` up to and including its ';' (or up
// to the closing brace, consumed by the caller).
func (p *parser) declaration() (decl, bool) {
	prop, ok := p.ident()
	if !ok {
		return decl{}, false
	}
	p.skipInsignificant()
	if !p.eat(':') {
		return decl{}, false
	}
	d, ok := p.value(strings.ToLower(prop))
	if !ok {
		return decl{}, false
	}
	p.skipInsignificant()
	if p.pos < len(p.src) && p.src[p.pos] == ';' {
		p.pos++
	}
	return d, true
}

// value parses the right-hand side of one declaration according to the
// property. Unknown properties are a declaration error — warned, and
// the declaration is skipped.
func (p *parser) value(prop string) (decl, bool) {
	switch prop {
	case "color":
		return p.colorDecl(PropColor)
	case "background-color":
		return p.colorDecl(PropBackgroundColor)
	case "border-color":
		return p.colorDecl(PropBorderColor)
	case "padding":
		return p.lengthDecl(PropPadding, 1<<16)
	case "border-radius":
		return p.lengthDecl(PropBorderRadius, 1<<16)
	case "border-width":
		return p.lengthDecl(PropBorderWidth, 1<<8)
	case "min-width":
		return p.lengthDecl(PropMinWidth, 1<<16)
	case "min-height":
		return p.lengthDecl(PropMinHeight, 1<<16)
	case "font-size":
		return p.floatDecl(PropFontSize, 1, 1<<10)
	case "font-weight":
		return p.weightDecl()
	case "font-family":
		return p.familyDecl()
	case "box-shadow":
		return p.shadowDecl()
	default:
		return decl{}, false
	}
}

func (p *parser) colorDecl(prop Prop) (decl, bool) {
	p.skipInsignificant()
	col, ok := p.hexColor()
	if !ok {
		return decl{}, false
	}
	return decl{prop: prop, color: col}, true
}

func (p *parser) lengthDecl(prop Prop, max int) (decl, bool) {
	p.skipInsignificant()
	n, ok := p.length()
	if !ok || n < 0 || n > max {
		return decl{}, false
	}
	return decl{prop: prop, num: n}, true
}

// floatDecl parses a fractional length, for the one property (font-size)
// that takes sub-pixel sizes.
func (p *parser) floatDecl(prop Prop, min, max float64) (decl, bool) {
	p.skipInsignificant()
	start := p.pos
	n, ok := p.length()
	if !ok {
		return decl{}, false
	}
	v, err := strconv.ParseFloat(strings.TrimSuffix(p.src[start:p.pos], "px"), 64)
	if err != nil {
		return decl{}, false
	}
	if v < min || v > max {
		return decl{}, false
	}
	return decl{prop: prop, flt: v, num: n}, true
}

// weightDecl parses `normal`, `bold`, or a numeric weight.
func (p *parser) weightDecl() (decl, bool) {
	p.skipInsignificant()
	if name, ok := p.tryIdent(); ok {
		switch strings.ToLower(name) {
		case "normal":
			return decl{prop: PropFontWeight, num: 400}, true
		case "bold":
			return decl{prop: PropFontWeight, num: 700}, true
		default:
			return decl{}, false
		}
	}
	n, ok := p.intTokens()
	if !ok || n < 1 || n > 1000 {
		return decl{}, false
	}
	return decl{prop: PropFontWeight, num: n}, true
}

// familyDecl parses a comma-separated family list and keeps the head —
// the font chain head the consumer shapes with.
func (p *parser) familyDecl() (decl, bool) {
	p.skipInsignificant()
	family, ok := p.familyName()
	if !ok {
		return decl{}, false
	}
	// Skip the remaining fallback list: the subset keeps the head only.
	for {
		p.skipInsignificant()
		if !p.eat(',') {
			break
		}
		p.skipInsignificant()
		if _, ok := p.familyName(); !ok {
			return decl{}, false
		}
	}
	return decl{prop: PropFontFamily, str: family}, true
}

// familyName parses one family: a quoted string or an identifier run
// (spaces allowed inside an unquoted name, per CSS's font-family
// grammar).
func (p *parser) familyName() (string, bool) {
	if p.pos < len(p.src) && (p.src[p.pos] == '"' || p.src[p.pos] == '\'') {
		return p.quoted()
	}
	start := p.pos
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if isIdentStart(c) || c >= '0' && c <= '9' || c == '-' || c == '_' || c == ' ' {
			p.pos++
			continue
		}
		break
	}
	name := strings.TrimRight(p.src[start:p.pos], " ")
	if name == "" {
		return "", false
	}
	return name, true
}

// shadowDecl parses `none` or `COLOR BLUR` — the one-shadow subset.
func (p *parser) shadowDecl() (decl, bool) {
	p.skipInsignificant()
	if name, ok := p.tryIdent(); ok {
		if strings.ToLower(name) == "none" {
			return decl{prop: PropBoxShadow}, true
		}
		return decl{}, false
	}
	col, ok := p.hexColor()
	if !ok {
		return decl{}, false
	}
	p.skipInsignificant()
	blur, ok := p.length()
	if !ok || blur > 1<<8 {
		return decl{}, false
	}
	return decl{prop: PropBoxShadow, color: col, num: blur}, true
}

// length parses `[+-]?digits[.digits]?[px]`, rounding to the nearest
// integer pixel. A unit other than px fails.
func (p *parser) length() (int, bool) {
	start := p.pos
	neg := p.eat('-')
	digits := 0
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		p.pos++
		digits++
	}
	frac := 0
	if p.pos < len(p.src) && p.src[p.pos] == '.' {
		p.pos++
		for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
			p.pos++
			frac++
		}
	}
	if digits == 0 && frac == 0 {
		return 0, false
	}
	numEnd := p.pos
	if p.pos+1 < len(p.src) && p.src[p.pos] == 'p' && p.src[p.pos+1] == 'x' {
		p.pos += 2
	} else if p.pos < len(p.src) && (isIdentStart(p.src[p.pos]) || p.src[p.pos] == '%') {
		return 0, false // a unit the subset does not take
	}
	v, err := strconv.ParseFloat(p.src[start:numEnd], 64)
	if err != nil {
		return 0, false
	}
	if neg {
		v = -v
	}
	return int(v + 0.5), true
}

// intTokens parses a bare decimal integer.
func (p *parser) intTokens() (int, bool) {
	start := p.pos
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start {
		return 0, false
	}
	n, err := strconv.Atoi(p.src[start:p.pos])
	if err != nil {
		return 0, false
	}
	return n, true
}

// hexColor parses #rgb, #rrggbb, or #rrggbbaa — the forms the markup
// parser accepts — into a premultiplied color.
func (p *parser) hexColor() (render.Color, bool) {
	if p.pos >= len(p.src) || p.src[p.pos] != '#' {
		return 0, false
	}
	start := p.pos
	p.pos++
	n := 0
	for p.pos < len(p.src) && isHex(p.src[p.pos]) {
		p.pos++
		n++
	}
	if n != 3 && n != 6 && n != 8 {
		p.pos = start
		return 0, false
	}
	hex := p.src[start+1 : p.pos]
	nib := func(c byte) uint8 {
		switch {
		case c >= '0' && c <= '9':
			return c - '0'
		case c >= 'a' && c <= 'f':
			return c - 'a' + 10
		default:
			return c - 'A' + 10
		}
	}
	if n == 3 {
		r, g, b := nib(hex[0]), nib(hex[1]), nib(hex[2])
		return render.RGBA(r*0x11, g*0x11, b*0x11, 0xff), true
	}
	var c [4]uint8
	c[3] = 0xff
	for j := range n / 2 {
		c[j] = nib(hex[j*2])<<4 | nib(hex[j*2+1])
	}
	return render.RGBA(c[0], c[1], c[2], c[3]), true
}

// quoted parses a single- or double-quoted string.
func (p *parser) quoted() (string, bool) {
	q := p.src[p.pos]
	p.pos++
	start := p.pos
	for p.pos < len(p.src) && p.src[p.pos] != q {
		p.pos++
	}
	if p.pos >= len(p.src) {
		return "", false
	}
	s := p.src[start:p.pos]
	p.pos++
	return s, true
}

// ident parses an identifier: letters, digits, '-', '_'.
func (p *parser) ident() (string, bool) {
	start := p.pos
	for p.pos < len(p.src) && isIdentByte(p.src[p.pos]) {
		p.pos++
	}
	if p.pos == start {
		return "", false
	}
	return p.src[start:p.pos], true
}

// tryIdent parses an identifier only if one starts here.
func (p *parser) tryIdent() (string, bool) {
	if p.pos >= len(p.src) || !isIdentStart(p.src[p.pos]) {
		return "", false
	}
	return p.ident()
}

// eat consumes c if it is next.
func (p *parser) eat(c byte) bool {
	if p.pos < len(p.src) && p.src[p.pos] == c {
		p.pos++
		return true
	}
	return false
}

// skipInsignificant advances past whitespace and /* comments */.
func (p *parser) skipInsignificant() {
	for p.pos < len(p.src) {
		switch c := p.src[p.pos]; {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			p.pos++
		case c == '/' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '*':
			end := strings.Index(p.src[p.pos+2:], "*/")
			if end < 0 {
				p.pos = len(p.src)
				return
			}
			p.pos += 2 + end + 2
		default:
			return
		}
	}
}

// warnSkip reports a rule-level failure and skips past the offending
// block (the next '}' at depth zero) or to EOF, so parsing resumes
// with the following rule.
func (p *parser) warnSkip(msg string, start int) {
	warnf("%s at %q", msg, badText(p.src[start:]))
	depth := 0
	for p.pos < len(p.src) {
		switch p.src[p.pos] {
		case '{':
			depth++
		case '}':
			if depth--; depth <= 0 {
				p.pos++
				return
			}
		}
		p.pos++
	}
}

// badText truncates a source excerpt for a warning message.
func badText(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}

func isIdentStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-' || c == '_'
}

func isIdentByte(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9'
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}
