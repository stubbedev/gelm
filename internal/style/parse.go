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
	"slices"
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

// Warn reports one stylesheet problem through the parse-warning sink
// outside a parse (a hot-reload stat failure, say), with the css:
// prefix warnf's messages carry.
func Warn(msg string) { parseWarn("css: " + msg) }

// cssWide is a CSS-wide keyword a declaration's whole value can be.
type cssWide uint8

const (
	wideNone cssWide = iota
	wideInherit
	wideInitial
	wideUnset
	wideRevert
	wideRevertLayer
)

// decl is one parsed declaration: which longhands it covers, its value
// tokens, and whether they reference var() (then the value is parsed
// per node at compute time, after substitution).
type decl struct {
	name   string // the property name, or the custom property's --name
	custom bool
	// meta marks the declarations that cascade outside the longhand
	// table: content, counter-reset, counter-increment.
	meta   bool
	covers PropSet
	parse  func([]token, *ctx, *Values) bool
	val    []token
	hasVar bool
	wide   cssWide
	// important marks a `!important` value: it outranks every normal
	// declaration, whatever the priority or specificity.
	important bool
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

	// mediaQs holds the distinct @media conditions the sheet's rules
	// carry, by first use; a rule's mq indexes it, -1 when the rule is
	// unconditional.
	mediaQs []mediaQuery

	// layerNames is the sheet's @layer order of first appearance;
	// rules carry the index of their layer, 255 when unlayered.
	layerNames []string

	// keyframes holds the sheet's @keyframes rules by (lowercased)
	// name; a later rule of the same name replaces an earlier one.
	keyframes map[string]*Keyframes

	// defines holds the sheet's @define-color values by custom-property
	// name (--name); a later define replaces an earlier one. They are
	// global, not inherited: the cascade consults them after the
	// node's inherited custom properties (Scratch.lookup).
	defines map[string][]token

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
	// HasRelational reports a :has() anywhere: a descendant or
	// following sibling's facts then feed an ancestor's match, so class
	// and state changes restyle up the tree too.
	HasRelational bool
	// Pseudo reports a ::before/::after rule: the widget layer then
	// computes pseudo-element styles.
	Pseudo bool
	// Counters reports counter-reset or counter-increment in play: the
	// widget layer then chains siblings' counter scopes.
	Counters bool
}

// Sensitivity returns the sheet's selector summary.
func (s *Sheet) Sensitivity() *Sensitivity { return &s.sens }

// MediaQueries reports how many distinct @media conditions the
// sheet's rules carry: zero when nothing in it is media-conditional.
func (s *Sheet) MediaQueries() int { return len(s.mediaQs) }

// Keyframes returns the sheet's @keyframes rule of that name, nil when
// it declares none.
func (s *Sheet) Keyframes(name string) *Keyframes {
	if s.keyframes == nil {
		return nil
	}
	return s.keyframes[name]
}

// addKeyframes files one @keyframes rule; a later rule of the same
// name replaces an earlier one.
func (s *Sheet) addKeyframes(kf *Keyframes) {
	if s.keyframes == nil {
		s.keyframes = make(map[string]*Keyframes)
	}
	s.keyframes[kf.Name] = kf
}

// rule is one selector of one stylesheet rule with its specificity,
// declarations, @media condition, and cascade layer.
type rule struct {
	sel   selector
	spec  spec
	block *Block
	mq    int32
	layer uint8
	// pseudo names the ::before/::after pseudo-element this rule
	// addresses; empty styles the element itself.
	pseudo string
}

// Parse compiles css into a Sheet. Malformed declarations and rules are
// warned about and skipped; the valid remainder still applies. An
// empty (or all-invalid) input yields an empty Sheet — never nil.
func Parse(css string) *Sheet {
	return parseSheet(css, "", nil, nil)
}

// ParseCollecting parses css like Parse and returns the sheet plus
// every warning the parse produced, without routing them to the sink:
// the caller reports them its own way (a file loader turns them into
// an error carrying the path).
func ParseCollecting(css string) (*Sheet, []string) {
	return ParseImporting(css, "", nil)
}

// ParseImporting parses css whose base is the importing file's path
// and whose @import targets load through resolve (nil: the
// filesystem, relative to the importing file). It is the file-backed
// loader's parse: imports resolve beside the file, and a cycle or a
// missing import reports precisely.
func ParseImporting(css, base string, resolve ImportResolver) (*Sheet, []string) {
	var warns []string
	sheet := parseSheet(css, base, resolve, func(msg string) { warns = append(warns, msg) })
	return sheet, warns
}

func parseSheet(css, base string, resolve ImportResolver, warn func(string)) *Sheet {
	p := parser{src: css, warn: warn, resolve: resolve, base: base, layer: 255}
	p.stylesheet(tokenize(css))
	return &p.sheet
}

// parser walks the token stream once.
type parser struct {
	src   string
	sheet Sheet
	order int
	// warn, when set, receives this parse's warnings instead of the
	// sink (ParseCollecting).
	warn func(msg string)
	// mq is the active @media condition (nil: everything) and layer
	// the active cascade layer rank (255: unlayered); dead is the
	// nesting depth of @supports branches that evaluated false.
	mq    mediaQuery
	layer uint8
	dead  int
	// resolve loads @import targets; base is the importing file's
	// path, importStack the active import chain for cycle detection.
	resolve     ImportResolver
	base        string
	importStack []string
}

// warnf reports one stylesheet problem and lets the parser carry on
// with the surrounding rules.
func (p *parser) warnf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	if p.warn != nil {
		p.warn(msg)
		return
	}
	parseWarn("css: " + msg)
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

// atRule consumes one at-rule: through its block, or to its
// semicolon. @keyframes, @define-color, @media, @supports, @layer and
// @import participate; the rest skip with a warning.
func (p *parser) atRule(ts []token, i int) int {
	name := ts[i].s
	switch name {
	case "keyframes", "-gtk-keyframes":
		return p.keyframes(ts, i)
	case "define-color":
		return p.defineColor(ts, i)
	case "media":
		return p.mediaRule(ts, i)
	case "supports":
		return p.supportsRule(ts, i)
	case "layer":
		return p.layerRule(ts, i)
	case "import":
		return p.importRule(ts, i)
	}
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

// defineColor files GTK's `@define-color name value;` as a sheet-level
// define under the custom-property name --name. References (`@name`,
// compiled to var(--name)) resolve through the ordinary var()
// substitution: a custom property on the node's chain first - so a
// subtree's --name overrides - then the defines, the highest-priority
// layer winning; defines may reference each other, and a cycle is
// invalid like any var() cycle. No second color resolver exists.
func (p *parser) defineColor(ts []token, i int) int {
	j := i + 1
	for j < len(ts) && ts[j].kind != tkSemi && ts[j].kind != tkLBrace {
		if closer(ts[j].kind) != 0 {
			j = blockEnd(ts, j) - 1
		}
		j++
	}
	if j < len(ts) && ts[j].kind == tkLBrace {
		p.warnf("@define-color skipped: want `name value;`")
		return blockEnd(ts, j)
	}
	body := trimWS(ts[i+1 : min(j, len(ts))])
	next := min(j+1, len(ts))
	if len(body) < 2 || body[0].kind != tkIdent {
		p.warnf("@define-color skipped: want `name value;`")
		return next
	}
	val := colorRefs(trimWS(body[1:]))
	if len(val) == 0 {
		p.warnf("@define-color skipped: %s has no value", body[0].s)
		return next
	}
	if p.sheet.defines == nil && p.dead == 0 {
		p.sheet.defines = make(map[string][]token)
	}
	if p.dead == 0 {
		p.sheet.defines["--"+body[0].s] = val
	}
	return next
}

// colorRefs rewrites GTK named-color references (`@name`) into
// `var(--name)`, the form the cascade resolves - what @define-color
// declares and what the theme layer provides for the Adwaita names.
func colorRefs(ts []token) []token {
	if !slices.ContainsFunc(ts, func(t token) bool { return t.kind == tkAt }) {
		return ts
	}
	out := make([]token, 0, len(ts)+2)
	for _, t := range ts {
		if t.kind != tkAt {
			out = append(out, t)
			continue
		}
		out = append(out,
			token{kind: tkFunc, s: "var", pos: t.pos, end: t.pos},
			token{kind: tkIdent, s: "--" + t.s, pos: t.pos, end: t.end},
			token{kind: tkRParen, pos: t.end, end: t.end})
	}
	return out
}

// keyframes parses one @keyframes rule into the sheet: the name, then
// stops of selector list (`from`, `to`, percentages) over declaration
// blocks. A stop keeps its phase even when it declares nothing the
// engine animates; declarations that do not interpolate warn and drop.
func (p *parser) keyframes(ts []token, i int) int {
	j := i + 1
	for ; j < len(ts); j++ {
		if ts[j].kind == tkWS {
			continue
		}
		if ts[j].kind == tkLBrace {
			break
		}
		if ts[j].kind == tkSemi {
			return j + 1 // `@keyframes name;`: dropped
		}
	}
	if j >= len(ts) {
		p.warnf("@keyframes skipped: want a block")
		return j
	}
	name := ""
	for k := i + 1; k < j; k++ {
		if ts[k].kind == tkWS {
			continue
		}
		name = strings.ToLower(ts[k].s)
		break
	}
	end := blockEnd(ts, j)
	if name == "" {
		p.warnf("@keyframes skipped: want a name")
		return min(end, len(ts))
	}
	kf := &Keyframes{Name: name}
	body := ts[j+1 : end-1]
	for x := 0; x < len(body); {
		if body[x].kind == tkWS || body[x].kind == tkSemi || body[x].kind == tkRBrace {
			x++
			continue
		}
		start := x
		for x < len(body) && body[x].kind != tkLBrace {
			if closer(body[x].kind) != 0 {
				x = blockEnd(body, x)
				continue
			}
			x++
		}
		if x >= len(body) {
			p.warnf("@keyframes stop skipped: want '{'")
			break
		}
		prelude := trimWS(body[start:x])
		stopEnd := blockEnd(body, x)
		frame := p.keyframe(p.declarations(body[x+1 : stopEnd-1]))
		for _, off := range keyframeOffsets(prelude) {
			f := frame
			f.Offset = off
			kf.Frames = append(kf.Frames, f)
		}
		x = stopEnd
	}
	sortFrames(kf)
	if p.dead == 0 {
		p.sheet.addKeyframes(kf)
	}
	return end
}

// keyframeOffsets maps a stop's selector list to offsets: `from` is 0,
// `to` is 1, `N%` is N/100.
func keyframeOffsets(ts []token) []float64 {
	var out []float64
	for _, part := range splitTop(ts, tkComma) {
		part = trimWS(part)
		if len(part) != 1 {
			continue
		}
		switch {
		case part[0].ident("from"):
			out = append(out, 0)
		case part[0].ident("to"):
			out = append(out, 1)
		case part[0].kind == tkPercent:
			if v := part[0].num; v >= 0 && v <= 100 {
				out = append(out, v/100)
			}
		}
	}
	return out
}

// keyframe extracts the animatable channels of one stop's
// declarations; the rest warn and drop. Each animatable name runs
// through its real property parser, so a stop writes exactly what a
// rule can: opacity, color, background-color, filter, transform, and
// -gtk-icon-transform.
func (p *parser) keyframe(decls []decl) Keyframe {
	var k Keyframe
	var op float64
	var color, bg render.Color
	var bright float64
	var xform, iconX Xform
	cx := ctx{rem: parseRem, em: parseRem}
	for _, d := range decls {
		var slot string
		var ok bool
		switch d.name {
		case "opacity":
			var v Values
			ok = parseOpacity(d.val, &cx, &v)
			if ok {
				op, slot = v.Opacity, "opacity"
			}
		case "color":
			cv, pok := parseColor(d.val, &cx)
			if pok {
				color, ok = cx.resolve(cv), true
			}
			slot = "color"
		case "background-color":
			cv, pok := parseColor(d.val, &cx)
			if pok {
				bg, ok = cx.resolve(cv), true
			}
			slot = "background-color"
		case "filter":
			var v Values
			ok = parseFilter(d.val, &cx, &v)
			if ok {
				bright, slot = v.Brightness, "filter"
			}
		case "transform":
			xform, ok = composeTransform(d.val, &cx)
			slot = "transform"
		case "-gtk-icon-transform":
			iconX, ok = composeTransform(d.val, &cx)
			slot = "-gtk-icon-transform"
		default:
			p.warnf("keyframe declaration skipped: %q does not interpolate", d.name)
			continue
		}
		if !ok {
			p.warnf("keyframe declaration skipped: %s: %s", slot, badText(rawText(p.src, d.val)))
			continue
		}
		switch slot {
		case "opacity":
			k.Opacity = &op
		case "color":
			k.Color = &color
		case "background-color":
			k.Background = &bg
		case "filter":
			k.Brightness = &bright
		case "transform":
			k.Transform = &xform
		case "-gtk-icon-transform":
			k.IconXform = &iconX
		}
	}
	return k
}

func (p *parser) warnAt(name string) {
	switch name {
	case "keyframes", "-gtk-keyframes":
		return
	}
	p.warnf("at-rule @%s ignored", name)
}

// mediaRule parses one @media: the condition becomes the context of
// every rule in the block, intersecting any enclosing condition.
func (p *parser) mediaRule(ts []token, i int) int {
	prelude, body, end, ok := p.atBlock(ts, i, "@media")
	if !ok {
		return end
	}
	q, okq := parseMediaQuery(prelude)
	if !okq {
		p.warnf("@media skipped: bad query %q", badText(rawText(p.src, prelude)))
		return end
	}
	save := p.mq
	p.mq = intersectMedia(p.mq, q)
	p.stylesheet(body)
	p.mq = save
	return end
}

// supportsRule parses one @supports: the condition evaluates against
// the engine's actual support at parse time, so a false block's rules
// never compile.
func (p *parser) supportsRule(ts []token, i int) int {
	prelude, body, end, ok := p.atBlock(ts, i, "@supports")
	if !ok {
		return end
	}
	save := p.dead
	if !evalSupports(prelude) {
		p.dead++
	}
	p.stylesheet(body)
	p.dead = save
	return end
}

// layerRule parses one @layer: `@layer a, b;` fixes the order of
// first appearance, `@layer name { ... }` styles in name's layer (the
// block's last name), `@layer { ... }` in a fresh anonymous layer.
// Unlayered rules outrank every layer.
func (p *parser) layerRule(ts []token, i int) int {
	j := i + 1
	for ; j < len(ts); j++ {
		switch ts[j].kind {
		case tkSemi:
			for _, name := range splitTop(trimWS(ts[i+1:j]), tkComma) {
				p.layerOf(rawName(name))
			}
			return j + 1
		case tkLBrace:
			goto block
		case tkFunc, tkLParen, tkLBrack:
			j = blockEnd(ts, j) - 1
		}
	}
	p.warnf("@layer skipped: want a block")
	return j
block:
	names := splitTop(trimWS(ts[i+1:j]), tkComma)
	end := blockEnd(ts, j)
	if end > len(ts) || ts[end-1].kind != tkRBrace {
		p.warnf("@layer skipped: unterminated block")
		return len(ts)
	}
	var last string
	have := false
	for _, name := range names {
		last, have = rawName(name), true
		p.layerOf(last)
	}
	save := p.layer
	if have {
		p.layer = p.layerOf(last)
	} else {
		p.layer = p.layerOf("")
	}
	p.stylesheet(ts[j+1 : end-1])
	p.layer = save
	return end
}

// rawName lowercases a layer name token span.
func rawName(ts []token) string {
	if len(ts) != 1 || ts[0].kind != tkIdent {
		return ""
	}
	return strings.ToLower(ts[0].s)
}

// layerOf returns name's rank, registering it (a "" name is a fresh
// anonymous layer) on first use.
func (p *parser) layerOf(name string) uint8 {
	if name != "" {
		if i := slices.Index(p.sheet.layerNames, name); i >= 0 {
			return uint8(i)
		}
	}
	if n := len(p.sheet.layerNames); n < 254 {
		p.sheet.layerNames = append(p.sheet.layerNames, name)
		return uint8(n)
	}
	return 253
}

// importRule loads one `@import "file" layer? media?;` through the
// resolver and inlines its rules in place, tracking the import chain
// so a cycle reports precisely instead of looping.
func (p *parser) importRule(ts []token, i int) int {
	j := i + 1
	for ; j < len(ts); j++ {
		switch ts[j].kind {
		case tkSemi:
			goto found
		case tkLBrace, tkRBrace:
			p.warnf("@import skipped: want `\"file\";`")
			return blockEnd(ts, j)
		case tkFunc, tkLParen, tkLBrack:
			j = blockEnd(ts, j) - 1
		}
	}
	p.warnf("@import skipped: want `;`")
	return j
found:
	prelude := trimWS(ts[i+1 : j])
	if len(prelude) == 0 {
		p.warnf("@import skipped: want a target")
		return j + 1
	}
	var ref string
	var rest []token
	t := prelude[0]
	switch {
	case t.kind == tkString:
		ref, rest = t.s, prelude[1:]
	case t.kind == tkFunc && t.s == "url":
		end := blockEnd(prelude, 0)
		inner := trimWS(funcArgs(prelude[:end]))
		ref = strings.Trim(rawText(p.src, inner), "\"'")
		rest = prelude[end:]
	default:
		p.warnf("@import skipped: want a string or url() target")
		return j + 1
	}
	if strings.Contains(rest2s(rest), "supports(") || strings.Contains(rest2s(rest), "layer(") {
		p.warnf("@import skipped: supports()/layer() qualifiers are not supported")
		return j + 1
	}
	q, okq := parseMediaQuery(trimWS(rest))
	if !okq {
		p.warnf("@import %s skipped: bad query", ref)
		return j + 1
	}
	resolve := p.resolve
	if resolve == nil {
		resolve = fileImport
	}
	path, content, err := resolve(ref, p.base)
	if err != nil {
		p.warnf("@import %s: %v", ref, err)
		return j + 1
	}
	if path != "" {
		if slices.Contains(p.importStack, path) {
			chain := strings.Join(append(slices.Clone(p.importStack), path), " -> ")
			p.warnf("@import cycle: %s", chain)
			return j + 1
		}
	}
	saveMQ, saveBase := p.mq, p.base
	p.mq = intersectMedia(p.mq, q)
	p.base = path
	p.importStack = append(p.importStack, path)
	p.stylesheet(tokenize(content))
	p.importStack = p.importStack[:len(p.importStack)-1]
	p.mq, p.base = saveMQ, saveBase
	return j + 1
}

func rest2s(ts []token) string {
	var b strings.Builder
	for _, t := range ts {
		b.WriteString(t.s)
	}
	return strings.ToLower(b.String())
}

// atBlock locates an at-rule's `{`, returning its trimmed prelude and
// body tokens and the index past the block.
func (p *parser) atBlock(ts []token, i int, what string) (prelude, body []token, end int, ok bool) {
	j := i + 1
	for ; j < len(ts); j++ {
		switch ts[j].kind {
		case tkSemi:
			p.warnf("%s skipped: want a block", what)
			return nil, nil, j + 1, false
		case tkLBrace:
			end := blockEnd(ts, j)
			if end > len(ts) || ts[end-1].kind != tkRBrace {
				p.warnf("%s skipped: unterminated block", what)
				return nil, nil, len(ts), false
			}
			return trimWS(ts[i+1 : j]), ts[j+1 : end-1], end, true
		case tkFunc, tkLParen, tkLBrack:
			j = blockEnd(ts, j) - 1
		}
	}
	p.warnf("%s skipped: want a block", what)
	return nil, nil, j, false
}

// qualifiedRule parses a selector list and its block starting at ts[i]
// and returns the index past it.
func (p *parser) qualifiedRule(ts []token, i int) int {
	start := i
	for i < len(ts) && ts[i].kind != tkLBrace {
		if ts[i].kind == tkSemi || ts[i].kind == tkRBrace {
			p.warnf("rule skipped: want '{' at %q", badText(rawText(p.src, ts[start:i+1])))
			return i + 1
		}
		if closer(ts[i].kind) != 0 {
			i = blockEnd(ts, i)
			continue
		}
		i++
	}
	if i >= len(ts) {
		p.warnf("rule skipped: want '{' at %q", badText(rawText(p.src, ts[start:])))
		return i
	}
	prelude := trimWS(ts[start:i])
	end := blockEnd(ts, i)
	if end > len(ts) || ts[end-1].kind != tkRBrace {
		p.warnf("rule skipped: unterminated block at %q", badText(rawText(p.src, prelude)))
		return len(ts)
	}
	body := ts[i+1 : end-1]
	p.ruleBody(body, splitTop(prelude, tkComma))
	return end
}

// ruleBody parses a rule's body — its declarations plus any nested
// rules and at-rules — under parents, the token lists of the selector
// each declaration and nested rule belongs to. A nested selector
// desugars its `&`s to the parent selector (an absent `&` is the
// descendant form), and a comma group crosses with the parent list.
func (p *parser) ruleBody(ts []token, parents [][]token) {
	for k := range parents {
		parents[k] = trimWS(parents[k])
	}
	var decls []decl
	start, seg := 0, 0
	i := 0
	for i < len(ts) {
		switch ts[i].kind {
		case tkLBrace:
			decls = append(decls, p.declarationsHead(ts[start:i])...)
			prelude := trimWS(ts[seg:i])
			end := blockEnd(ts, i)
			inner := ts[min(i+1, len(ts)):max(end-1, 0)]
			p.nestedRule(prelude, inner, parents)
			i, start, seg = end, end, end
		case tkFunc, tkLParen, tkLBrack:
			i = blockEnd(ts, i)
		case tkSemi:
			i++
			seg = i
		case tkAt:
			if atSegmentStart(ts, seg, i) {
				decls = append(decls, p.declarationsHead(ts[start:i])...)
				i = p.nestedAt(ts, i, parents)
				start, seg = i, i
			} else {
				i++
			}
		default:
			i++
		}
	}
	decls = append(decls, p.declarations(ts[start:])...)
	if len(decls) == 0 || p.dead > 0 {
		return
	}
	block := &Block{decls: decls}
	for _, par := range parents {
		sels, ok := parseSelectorList(p.src, par)
		if !ok {
			p.warnf("rule skipped: bad selector %q", badText(rawText(p.src, par)))
			continue
		}
		for _, sel := range sels {
			p.add(sel, block)
		}
	}
}

// atSegmentStart reports whether the token at i begins a fresh
// statement: nothing but whitespace since the last `;` or block. An
// `@` mid-declaration (GTK's `color: @fg`) is a value, not an at-rule.
func atSegmentStart(ts []token, seg, i int) bool {
	for _, t := range ts[seg:i] {
		if t.kind != tkWS {
			return false
		}
	}
	return true
}

// nestedRule parses one nested `& ... { }` block under parents.
func (p *parser) nestedRule(prelude, inner []token, parents [][]token) {
	if len(prelude) == 0 {
		p.warnf("rule skipped: want a selector before '{'")
		return
	}
	var combined [][]token
	for _, par := range parents {
		for _, raw := range splitTop(prelude, tkComma) {
			n := trimWS(raw)
			if len(n) > 0 {
				combined = append(combined, spliceParent(n, par))
			}
		}
	}
	if len(combined) > 0 {
		p.ruleBody(inner, combined)
	}
}

// spliceParent desugars n's `&`s to par; without one, n is a
// descendant of par.
func spliceParent(n, par []token) []token {
	amp := false
	for _, t := range n {
		if t.is('&') {
			amp = true
			break
		}
	}
	out := make([]token, 0, len(n)+len(par)+1)
	if !amp {
		out = append(out, par...)
		out = append(out, token{kind: tkWS})
		return append(out, n...)
	}
	for _, t := range n {
		if t.is('&') {
			out = append(out, par...)
			continue
		}
		out = append(out, t)
	}
	return out
}

// nestedAt applies a declaration-scoped at-rule inside a rule body —
// @media, @supports and @layer re-run the body's remaining content
// under their condition — and drops the rest with a warning.
func (p *parser) nestedAt(ts []token, i int, parents [][]token) int {
	prelude, body, end, ok := p.atBlock(ts, i, "at-rule @"+ts[i].s)
	if !ok {
		return end
	}
	switch ts[i].s {
	case "media":
		q, okq := parseMediaQuery(prelude)
		if !okq {
			p.warnf("@media skipped: bad query %q", badText(rawText(p.src, prelude)))
			return end
		}
		save := p.mq
		p.mq = intersectMedia(p.mq, q)
		p.ruleBody(body, parents)
		p.mq = save
	case "supports":
		save := p.dead
		if !evalSupports(prelude) {
			p.dead++
		}
		p.ruleBody(body, parents)
		p.dead = save
	case "layer":
		names := splitTop(prelude, tkComma)
		var last string
		have := false
		for _, name := range names {
			last, have = rawName(name), true
			p.layerOf(last)
		}
		save := p.layer
		if have {
			p.layer = p.layerOf(last)
		} else {
			p.layer = p.layerOf("")
		}
		p.ruleBody(body, parents)
		p.layer = save
	default:
		p.warnAt(ts[i].s)
	}
	return end
}

// declarationsHead parses the declarations ahead of a nested rule:
// a colon-less entry is the nested selector's start, not a malformed
// declaration, so it drops silently.
func (p *parser) declarationsHead(ts []token) []decl {
	var out []decl
	for _, raw := range splitTop(ts, tkSemi) {
		raw = trimWS(raw)
		if len(raw) == 0 || !hasTopColon(raw) || raw[0].is('&') {
			continue
		}
		if d, ok := p.declaration(raw); ok {
			out = append(out, d)
		}
	}
	return out
}

func hasTopColon(ts []token) bool {
	for i := 0; i < len(ts); i++ {
		if closer(ts[i].kind) != 0 {
			i = blockEnd(ts, i) - 1
			continue
		}
		if ts[i].kind == tkColon {
			return true
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
		p.warnf("declaration skipped: %s", badText(rawText(p.src, ts)))
		return decl{}, false
	}
	j := 1
	for j < len(ts) && ts[j].kind == tkWS {
		j++
	}
	if j >= len(ts) || ts[j].kind != tkColon {
		p.warnf("declaration skipped: %s", badText(rawText(p.src, ts)))
		return decl{}, false
	}
	name := ts[0].s
	val, imp := takeImportant(colorRefs(trimWS(ts[j+1:])))
	if val == nil {
		p.warnf("declaration skipped: %s", badText(rawText(p.src, ts)))
		return decl{}, false
	}
	p.order++
	d := decl{name: name, val: val, hasVar: containsVar(val), wide: wideOf(val), important: imp, order: p.order}
	if strings.HasPrefix(name, "--") {
		d.custom = true
		return d, true
	}
	lname := strings.ToLower(name)
	d.name = lname
	if lname == "content" || lname == "counter-reset" || lname == "counter-increment" {
		d.meta = true
		return d, true
	}
	if lname == "all" {
		if d.wide == wideNone {
			p.warnf("declaration skipped: all: %s", badText(rawText(p.src, val)))
			return decl{}, false
		}
		d.covers = allProps
		return d, true
	}
	def, ok := propTable[lname]
	if !ok {
		if !ignoredProps[lname] {
			p.warnf("declaration skipped: unknown property %q", name)
		}
		return decl{}, false
	}
	d.covers, d.parse = def.covers, def.parse
	if len(val) == 0 {
		p.warnf("declaration skipped: %s has no value", name)
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
		p.warnf("declaration skipped: %s: %s", name, badText(rawText(p.src, val)))
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

// takeImportant strips a trailing `!important` from a value; a nil
// return marks a malformed `!`.
func takeImportant(ts []token) ([]token, bool) {
	for i := len(ts) - 1; i >= 0; i-- {
		if !ts[i].is('!') {
			continue
		}
		j := i + 1
		for j < len(ts) && ts[j].kind == tkWS {
			j++
		}
		if j == len(ts)-1 && ts[j].ident("important") {
			return trimWS(ts[:i]), true
		}
		return nil, false
	}
	return ts, false
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
	case "unset":
		return wideUnset
	case "revert":
		return wideRevert
	case "revert-layer":
		return wideRevertLayer
	}
	return wideNone
}

// containsVar reports a var() or attr() call anywhere in the value:
// both resolve per node, so the declaration parses after substitution
// instead of at parse time.
func containsVar(ts []token) bool {
	for _, t := range ts {
		if t.kind == tkFunc && (t.s == "var" || t.s == "attr") {
			return true
		}
	}
	return false
}

// add files one compiled rule into the index and folds its selector
// into the sensitivity summary, carrying the parser's active @media
// condition and cascade layer.
func (p *parser) add(sel selector, block *Block) {
	if p.dead > 0 {
		return
	}
	s := &p.sheet
	idx := int32(len(s.rules))
	r := rule{sel: sel, spec: sel.spec(), block: block, mq: -1, layer: p.layer, pseudo: sel.parts[len(sel.parts)-1].pseudo}
	if p.mq != nil {
		r.mq = int32(len(s.mediaQs))
		for i, q := range s.mediaQs {
			if queryEqual(q, p.mq) {
				r.mq = int32(i)
				break
			}
		}
		if int(r.mq) == len(s.mediaQs) {
			s.mediaQs = append(s.mediaQs, p.mq)
		}
	}
	s.rules = append(s.rules, r)
	if r.pseudo != "" {
		s.sens.Pseudo = true
	}
	for i := range sel.parts {
		part := &sel.parts[i]
		if part.structs != 0 || len(part.nths) > 0 {
			s.sens.Siblings = true
		}
		s.noteCompound(part, i == len(sel.parts)-1)
	}
	for _, c := range sel.combs {
		if c == '+' || c == '~' {
			s.sens.Siblings = true
		}
	}
	for i := range block.decls {
		if d := &block.decls[i]; d.meta {
			if d.name == "counter-reset" || d.name == "counter-increment" || counterContent(d.val) {
				s.sens.Counters = true
			}
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

// noteCompound records the facts one compound tests: its own facts
// for a non-rightmost compound (the ancestor sensitivity), and any
// :has() anywhere (a descendant's facts feed this node's match).
func (s *Sheet) noteCompound(c *compound, rightmost bool) {
	if len(c.has) > 0 {
		s.sens.HasRelational = true
	}
	if rightmost {
		return
	}
	s.noteAncestor(c)
}

// counterContent reports a content value reading counters.
func counterContent(ts []token) bool {
	for _, t := range ts {
		if t.kind == tkFunc && (t.s == "counter" || t.s == "counters") {
			return true
		}
	}
	return false
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
		for j := range c.not[i].parts {
			s.noteAncestor(&c.not[i].parts[j])
		}
	}
	for i := range c.isLists {
		for j := range c.isLists[i].sels {
			for k := range c.isLists[i].sels[j].parts {
				s.noteAncestor(&c.isLists[i].sels[j].parts[k])
			}
		}
	}
	for i := range c.has {
		for k := range c.has[i].sel.parts {
			s.noteAncestor(&c.has[i].sel.parts[k])
		}
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
