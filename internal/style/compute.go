package style

import (
	"slices"
	"strconv"
	"strings"

	"github.com/stubbedev/gelm/render"
)

// Env is the unit environment a compute resolves against.
type Env struct {
	// Rem is the root font size in logical pixels: what 1rem is.
	Rem float64
	// FontPx is the font size a node computes when nothing in its
	// ancestry sets one (CSS's `medium`); zero means Rem.
	FontPx float64
	// Font is the node's face metrics the ch, ex, cap, ic and lh units
	// resolve against, as ratios of the font size (render.Font.Metrics).
	// The zero value falls back to the CSS default ratios.
	Font FontMetrics
	// RootLineHeight is what 1rlh is; zero falls back to the natural
	// line height of Font at Rem.
	RootLineHeight float64
	// Media is what @media conditions evaluate against (the viewport
	// and appearance state); the zero value is a 0×0 viewport at 96dpi.
	// Its Width and Height also resolve the vw/vh unit family and its
	// Dark flag picks light-dark()'s half.
	Media Media
}

// maxVarDepth bounds var() expansion: a chain deeper than this is
// treated as a cycle.
const maxVarDepth = 32

// Compute runs the cascade for n: match every layer (and n's inline
// block), resolve the node's custom properties over parent's, substitute
// var() and parse the winning declarations, then inherit the unset
// inherited longhands from parent. parent is nil at the root. old, when
// non-nil, is the node's previous result: an unchanged custom-property
// environment keeps its old pointer so equality stays cheap.
func Compute(layers []Layer, n Node, parent, old *Values, env Env, sc *Scratch, v *Values) {
	sc.pseudo = ""
	sc.computeAll(layers, n, parent, old, env, v)
}

// ComputePseudo is Compute for one pseudo-element (::before, ::after)
// of n: only the rules addressing that pseudo-element cascade, and the
// result inherits from parent, the element's own computed style. A
// content declaration's quote keywords move the element's inherited
// quote depth, so descendants' open-quote continues the count.
func ComputePseudo(layers []Layer, n Node, parent *Values, env Env, sc *Scratch, v *Values, name string) {
	sc.pseudo = name
	sc.computeAll(layers, n, parent, nil, env, v)
	if parent != nil && v.HasContent {
		parent.QuoteDepth = v.QuoteDepth
	}
}

func (sc *Scratch) computeAll(layers []Layer, n Node, parent, old *Values, env Env, v *Values) {
	*v = Values{}
	if env.Rem <= 0 {
		env.Rem = 16
	}
	if env.FontPx <= 0 {
		env.FontPx = env.Rem
	}
	sc.media = env.Media
	if sc.media.DPI <= 0 {
		sc.media.DPI = 96
	}
	sc.collect(layers, n)
	sc.layers = layers

	var pv *Values
	if parent != nil {
		pv = parent
	} else {
		pv = &Values{}
	}
	v.Vars = sc.resolveVars(pv.Vars)
	if old != nil && old.Vars != v.Vars && v.Vars.equalOwn(old.Vars) {
		v.Vars = old.Vars
	}

	// The context lives in the scratch: the parsers are function values,
	// so a stack ctx would escape and allocate per compute.
	sc.cx = ctx{
		rem: env.Rem, em: env.FontPx, weight: 400, self: &sc.self,
		vpW: env.Media.Width, vpH: env.Media.Height,
		font: env.Font, rlh: env.RootLineHeight, dark: env.Media.Dark,
		cq: sc.container, cqHave: sc.haveContainer,
	}
	if sc.cx.rlh == 0 {
		sc.cx.rlh = fontRatio(env.Font.LineHeight, fallbackLH) * env.Rem
	}
	if an, ok := n.(attrNode); ok {
		sc.cx.attrs = an
	}
	cx := &sc.cx
	if pv.Has(PropFontSize) {
		cx.em = pv.FontSize
	}
	if pv.Has(PropFontWeight) {
		cx.weight = pv.FontWeight
	}
	if pv.Has(PropColor) {
		cx.color = pv.Color
	}
	if pv.Has(PropLineHeight) {
		cx.lh = pv.LineHeight
	}
	sc.parsed = sc.parsed[:0]

	// font-size first: em lengths everywhere else resolve against it.
	// color next: currentColor everywhere else resolves against it.
	// line-height third: the lh unit reads it.
	var blocked PropSet
	blocked = blocked.Or(sc.computeProp(PropFontSize, v, pv, cx))
	if v.Has(PropFontSize) {
		cx.em = v.FontSize
	}
	blocked = blocked.Or(sc.computeProp(PropColor, v, pv, cx))
	if v.Has(PropColor) {
		cx.color = v.Color
	}
	blocked = blocked.Or(sc.computeProp(PropLineHeight, v, pv, cx))
	if v.Has(PropLineHeight) {
		cx.lh = v.LineHeight
	}
	if cx.lh == 0 {
		cx.lh = fontRatio(cx.font.LineHeight, fallbackLH) * cx.em
	}
	for p := range numProps {
		if p == PropFontSize || p == PropColor || p == PropLineHeight {
			continue
		}
		blocked = blocked.Or(sc.computeProp(p, v, pv, cx))
	}

	// Inheritance: unset inherited longhands take the parent's value,
	// unless an explicit `initial` blocked it. The quote depth
	// inherits too: descendants' open-quote continues the count.
	v.Own = v.Set
	for p := range numProps {
		if inheritedProps.Has(p) && !v.Has(p) && !blocked.Has(p) && pv.Has(p) {
			copyProp(v, pv, p)
			v.Set = v.Set.With(p)
		}
	}
	v.QuoteDepth = pv.QuoteDepth
	sc.applyMeta(v, pv, cx)
	sc.prevCounters = nil
	resolveKeyframes(v, layers)
	mergeAnimSlots(v)
	sc.computeStarting(v, cx)
}

// computeStarting parses the winning @starting-style declarations
// into v.Starting, nil when no matched rule carries any: the entry
// transition's start values. Only the properties the blocks declare
// are read; the rest of the struct is meaningless.
func (sc *Scratch) computeStarting(v *Values, cx *ctx) {
	var any bool
	for p := range numProps {
		if sc.winStart[p] != nil {
			any = true
			break
		}
	}
	if !any {
		return
	}
	st := initialValues()
	for p := range numProps {
		d := sc.winStart[p]
		if d == nil {
			continue
		}
		if src, ok := sc.parse(d, v.Vars, cx); ok {
			copyProp(&st, src, p)
			st.Set = st.Set.With(p)
		}
	}
	v.Starting = &st
}

// computeProp computes one longhand from its winning declaration. It
// returns p's bit when an explicit `initial` must keep the inherited
// value out.
func (sc *Scratch) computeProp(p Prop, v, parent *Values, cx *ctx) PropSet {
	d := sc.win[p]
	if d == nil {
		return PropSet{}
	}
	wide := d.wide
	if wide == wideRevertLayer {
		if lo := sc.revertBelow(p, int(sc.bestLayer[p])); lo != nil && lo.wide != wideRevertLayer {
			if src, ok := sc.parse(lo, v.Vars, cx); ok {
				copyProp(v, src, p)
				v.Set = v.Set.With(p)
				return PropSet{}
			}
		}
		wide = wideUnset
	}
	if wide == wideRevert {
		wide = wideUnset
	}
	if wide == wideNone {
		src, ok := sc.parse(d, v.Vars, cx)
		if ok {
			copyProp(v, src, p)
			v.Set = v.Set.With(p)
			return PropSet{}
		}
		// Invalid at computed-value time: the declaration acts as unset.
		wide = wideUnset
	}
	if wide == wideUnset {
		if inheritedProps.Has(p) {
			wide = wideInherit
		} else {
			wide = wideInitial
		}
	}
	switch wide {
	case wideInherit:
		if parent.Has(p) {
			copyProp(v, parent, p)
			v.Set = v.Set.With(p)
		} else if !inheritedProps.Has(p) {
			sc.setInitial(p, v, cx)
		}
	case wideInitial:
		// An inherited property's initial value is the theme's (the
		// consumer's default), so it stays unset; the rest take the CSS
		// initial value, which is what strips theme styling under
		// `all: unset`.
		if !inheritedProps.Has(p) {
			sc.setInitial(p, v, cx)
			return PropSet{}
		}
		return setOf(p)
	}
	return PropSet{}
}

// setInitial writes p's initial value. Border and outline colors start
// as currentColor.
func (sc *Scratch) setInitial(p Prop, v *Values, cx *ctx) {
	init := initialValues()
	init.BorderColor = [4]render.Color{cx.color, cx.color, cx.color, cx.color}
	init.OutlineColor = cx.color
	copyProp(v, &init, p)
	v.Set = v.Set.With(p)
}

// revertBelow walks the revert-layer chain of p down from cascade
// layer from: the best declaration in the next layer below, and when
// that one is itself a revert-layer, the layer below it, until a
// concrete declaration or the bottom (nil).
func (sc *Scratch) revertBelow(p Prop, from int) *decl {
	for {
		var best *decl
		var bestRk uint64
		bestLayer := -1
		for i := range sc.chain {
			e := &sc.chain[i]
			if e.p != p || int(e.layer) >= from {
				continue
			}
			if bestLayer < 0 || e.rk > bestRk {
				best, bestRk, bestLayer = e.d, e.rk, int(e.layer)
			}
		}
		if best == nil || best.wide != wideRevertLayer {
			return best
		}
		from = bestLayer
	}
}

// parse substitutes and parses d once per compute.
func (sc *Scratch) parse(d *decl, vars *Vars, cx *ctx) (*Values, bool) {
	if d.pre != nil && (d.preRem == 0 || d.preRem == cx.rem) {
		return d.pre, true
	}
	for i := range sc.parsed {
		if sc.parsed[i].d == d {
			return &sc.parsed[i].v, sc.parsed[i].ok
		}
	}
	ts := d.val
	ok := true
	if d.hasVar {
		ts, ok = substitute(d.val, func(name string) ([]token, bool) { return sc.lookup(vars, name) }, 0)
		ts = trimWS(ts)
		ok = ok && len(ts) > 0
	}
	sc.parsed = append(sc.parsed, parsedDecl{d: d})
	e := &sc.parsed[len(sc.parsed)-1]
	e.v = initialValues()
	if ok {
		e.ok = d.parse(ts, cx, &e.v)
	}
	return &e.v, e.ok
}

// applyMeta resolves the winning content/counter-reset/
// counter-increment declarations — the meta declarations, which
// cascade outside the longhand table. The counter scope derives from
// the previous sibling's (the widget layer supplies it through
// sc.prevCounters when counters are in play) over the parent's, per
// CSS's document-order counting; content resolves against the node's
// own target (attr()) and counters.
func (sc *Scratch) applyMeta(v, pv *Values, cx *ctx) {
	v.Counters = sc.prevCounters
	if v.Counters == nil {
		v.Counters = pv.Counters
	}
	if len(sc.metas) == 0 {
		return
	}
	var reset, incr, content *decl
	for i := range sc.metas {
		switch sc.metas[i].d.name {
		case "counter-reset":
			reset = sc.metas[i].d
		case "counter-increment":
			incr = sc.metas[i].d
		case "content":
			content = sc.metas[i].d
		}
	}
	if reset != nil || incr != nil {
		v.Counters = &CounterScope{parent: v.Counters}
		if reset != nil {
			if ts, ok := sc.metaTokens(reset, v.Vars); ok {
				v.Counters.apply(ts, true)
			}
		}
		if incr != nil {
			if ts, ok := sc.metaTokens(incr, v.Vars); ok {
				v.Counters.apply(ts, false)
			}
		}
	}
	if content == nil {
		return
	}
	ts, ok := sc.metaTokens(content, v.Vars)
	if !ok {
		return
	}
	cx.counters = v.Counters
	cx.quoteDepth = pv.QuoteDepth
	if c, ok := parseContentTokens(ts, cx); ok {
		v.Content, v.ContentURL, v.HasContent = c.text, c.url, true
		v.QuoteDepth = c.depth
	}
}

// metaTokens substitutes a meta declaration's var() references.
func (sc *Scratch) metaTokens(d *decl, vars *Vars) ([]token, bool) {
	if !d.hasVar {
		return d.val, true
	}
	ts, ok := substitute(d.val, func(name string) ([]token, bool) { return sc.lookup(vars, name) }, 0)
	ts = trimWS(ts)
	return ts, ok && len(ts) > 0
}

// CounterEntry is one counter's value at one scope level.
type CounterEntry struct {
	Name  string
	Value int
}

// CounterScope is a node's counter environment: its own entries over
// the chain above (the parent's, or the previous sibling's — CSS
// counts in document order).
type CounterScope struct {
	parent *CounterScope
	own    []CounterEntry
}

// get returns the counter's innermost value.
func (cs *CounterScope) get(name string) int {
	for ; cs != nil; cs = cs.parent {
		for i := range cs.own {
			if cs.own[i].Name == name {
				return cs.own[i].Value
			}
		}
	}
	return 0
}

// chain returns the counter's values from the outermost scope in.
func (cs *CounterScope) chain(name string) []int {
	var out []int
	for ; cs != nil; cs = cs.parent {
		for i := range cs.own {
			if cs.own[i].Name == name {
				out = append(out, cs.own[i].Value)
			}
		}
	}
	slices.Reverse(out)
	return out
}

// apply folds one counter-reset/counter-increment declaration into
// the scope.
func (cs *CounterScope) apply(ts []token, reset bool) {
	for i := 0; i < len(ts); i++ {
		if ts[i].kind != tkIdent {
			continue
		}
		name := ts[i].s
		n := 0
		if !reset {
			n = 1
		}
		j := i + 1
		for j < len(ts) && (ts[j].kind == tkWS || ts[j].kind == tkComma) {
			j++
		}
		if j < len(ts) && ts[j].kind == tkNumber {
			n = int(ts[j].num)
			i = j
		} else {
			i = j - 1
		}
		if !reset {
			n += cs.get(name)
		}
		replaced := false
		for k := range cs.own {
			if cs.own[k].Name == name {
				cs.own[k].Value = n
				replaced = true
			}
		}
		if !replaced {
			cs.own = append(cs.own, CounterEntry{Name: name, Value: n})
		}
	}
}

// parseContentTokens resolves a content value: strings, attr(),
// counter() and counters(), url() (an image in the generated
// content), and the quote keywords open-quote/close-quote/
// no-open-quote/no-close-quote over the inherited quote depth.
// Anything else is invalid at computed-value time and drops the
// declaration.
type parsedContent struct {
	text  string
	url   string
	depth int
}

// defaultQuotes are the quote pairs the quote keywords emit without
// a quotes property: the typographic double and single pairs.
var defaultQuotes = [][2]string{{"\u201c", "\u201d"}, {"\u2018", "\u2019"}}

func parseContentTokens(ts []token, cx *ctx) (parsedContent, bool) {
	var c parsedContent
	c.depth = cx.quoteDepth
	if len(ts) == 1 && ts[0].kind == tkIdent {
		switch strings.ToLower(ts[0].s) {
		case "none", "normal":
			return c, true
		}
		return c, false
	}
	var b strings.Builder
	quote := func(closing bool) string {
		pair := defaultQuotes[min(c.depth, len(defaultQuotes)-1)]
		if closing {
			return pair[1]
		}
		return pair[0]
	}
	for i := 0; i < len(ts); i++ {
		t := ts[i]
		switch {
		case t.kind == tkString:
			b.WriteString(t.s)
		case t.kind == tkIdent:
			switch strings.ToLower(t.s) {
			case "open-quote":
				b.WriteString(quote(false))
				c.depth++
			case "close-quote":
				c.depth = max(0, c.depth-1)
				b.WriteString(quote(true))
			case "no-open-quote":
				c.depth++
			case "no-close-quote":
				c.depth = max(0, c.depth-1)
			default:
				return c, false
			}
		case t.kind == tkFunc && t.s == "url":
			end := blockEnd(ts, i)
			inner := trimWS(funcArgs(ts[i:end]))
			if len(inner) == 0 {
				return c, false
			}
			if len(inner) == 1 && inner[0].kind == tkString {
				c.url = inner[0].s
			} else if len(inner) == 1 && inner[0].kind == tkIdent {
				c.url = inner[0].s
			} else {
				c.url = tokensText(inner)
			}
			i = end - 1
		case t.kind == tkFunc && t.s == "attr" && cx.self != nil:
			end := blockEnd(ts, i)
			if a := trimWS(funcArgs(ts[i:end])); len(a) > 0 && a[0].kind == tkIdent {
				if v, ok := cx.self.attrValue(a[0].s); ok {
					b.WriteString(v)
				}
			}
			i = end - 1
		case t.kind == tkFunc && (t.s == "counter" || t.s == "counters"):
			end := blockEnd(ts, i)
			a := splitTop(trimWS(funcArgs(ts[i:end])), tkComma)
			if len(a) > 0 && trimWS(a[0]) != nil && trimWS(a[0])[0].kind == tkIdent {
				name := trimWS(a[0])[0].s
				if t.s == "counter" || len(a) < 2 {
					b.WriteString(cx.counterValue(name, ""))
				} else if sep := trimWS(a[1]); len(sep) == 1 && sep[0].kind == tkString {
					b.WriteString(cx.counterValue(name, sep[0].s))
				}
			}
			i = end - 1
		case t.kind == tkWS || t.kind == tkComma:
		default:
			return parsedContent{}, false
		}
	}
	c.text = b.String()
	return c, true
}

// counterValue renders counter(name) or counters(name, sep).
func (cx *ctx) counterValue(name, sep string) string {
	if cx.counters == nil {
		return "0"
	}
	if sep == "" {
		return strconv.Itoa(cx.counters.get(name))
	}
	vals := cx.counters.chain(name)
	parts := make([]string, len(vals))
	for i, v := range vals {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, sep)
}

// lookup resolves a var() reference: the node's inherited custom
// properties first, then the @define-color defines, then the @property
// registrations' initial values, of the compute's layers.
func (sc *Scratch) lookup(vs *Vars, name string) ([]token, bool) {
	if ts, ok := vs.lookup(name); ok {
		return ts, true
	}
	if ts, ok := sc.define(name, 0); ok {
		return ts, true
	}
	return sc.registered(name)
}

// registered resolves a @property registration: the highest-priority
// layer's (the later on a tie), whose initial value answers for a
// name nothing declares. A universal syntax without an initial value
// has none, like an unregistered property.
func (sc *Scratch) registered(name string) ([]token, bool) {
	var reg *PropertyReg
	best := -1
	for i, l := range sc.layers {
		if l.Sheet == nil || l.Sheet.properties == nil {
			continue
		}
		if r, ok := l.Sheet.properties[name]; ok && (best < 0 || l.Priority >= sc.layers[best].Priority) {
			reg, best = r, i
		}
	}
	if reg == nil {
		return nil, false
	}
	return reg.Initial, reg.Initial != nil
}

// define resolves a @define-color: the highest-priority layer's value
// (the later layer on a tie), its own references substituted against
// the defines - defines name colors, never a node's properties.
func (sc *Scratch) define(name string, depth int) ([]token, bool) {
	var ts []token
	best := -1
	for i, l := range sc.layers {
		if l.Sheet == nil {
			continue
		}
		if d, ok := l.Sheet.defines[name]; ok && (best < 0 || l.Priority >= sc.layers[best].Priority) {
			ts, best = d, i
		}
	}
	if best < 0 || depth > maxVarDepth {
		return nil, false
	}
	if !containsVar(ts) {
		return ts, true
	}
	out, ok := substitute(ts, func(n string) ([]token, bool) { return sc.define(n, depth+1) }, depth)
	return trimWS(out), ok
}

// resolveVars computes the node's custom-property environment: the
// winning declarations with their own var() references substituted
// (against each other and the inherited chain), cycles made invalid.
func (sc *Scratch) resolveVars(parent *Vars) *Vars {
	if len(sc.customs) == 0 && !sc.hasNonInheritedProps() {
		return parent
	}
	sc.seedRegistrations()
	own := make(map[string][]token, len(sc.customs))
	const (
		pending = iota
		resolving
		done
	)
	state := make(map[string]uint8, len(sc.customs))
	var resolve func(name string, depth int) ([]token, bool)
	resolve = func(name string, depth int) ([]token, bool) {
		i, mine := sc.byName[name]
		if !mine {
			return sc.lookup(parent, name)
		}
		switch state[name] {
		case done:
			ts := own[name]
			return ts, ts != nil
		case resolving:
			return nil, false // a cycle: every member is invalid
		}
		if depth > maxVarDepth {
			return nil, false
		}
		state[name] = resolving
		d := sc.customs[i].d
		var ts []token
		ok := true
		switch d.wide {
		case wideInherit, wideUnset, wideRevert, wideRevertLayer:
			ts, ok = sc.lookup(parent, name)
		case wideInitial:
			ok = false
		default:
			ts = d.val
			if d.hasVar {
				ts, ok = substitute(d.val, func(n string) ([]token, bool) { return resolve(n, depth+1) }, depth)
				ts = trimWS(ts)
			}
		}
		state[name] = done
		if !ok {
			own[name] = nil
			return nil, false
		}
		if ts == nil {
			ts = []token{} // an empty value is valid and distinct from invalid
		}
		own[name] = ts
		return ts, true
	}
	for _, w := range sc.customs {
		resolve(w.d.name, 0)
	}
	return &Vars{parent: parent, own: own}
}

// hasNonInheritedProps reports whether any loaded layer registers an
// inherits: false @property: those resolve on nodes that declare
// nothing for them, so the environment must be built even without a
// custom declaration.
func (sc *Scratch) hasNonInheritedProps() bool {
	for _, l := range sc.layers {
		if l.Sheet == nil {
			continue
		}
		for _, reg := range l.Sheet.properties {
			if !reg.Inherits {
				return true
			}
		}
	}
	return false
}

// seedRegistrations injects every inherits: false registration the
// node did not declare as a synthetic own declaration holding the
// registration's initial value, which stops the inherited chain:
// the CSS semantics of a non-inherited registered property.
func (sc *Scratch) seedRegistrations() {
	for _, l := range sc.layers {
		if l.Sheet == nil || l.Sheet.properties == nil {
			continue
		}
		for name, reg := range l.Sheet.properties {
			if reg.Inherits {
				continue
			}
			if _, mine := sc.byName[name]; mine {
				continue
			}
			d := &decl{name: name, custom: true, val: reg.Initial}
			sc.byName[name] = len(sc.customs)
			sc.customs = append(sc.customs, customWin{d: d})
		}
	}
}

// substitute replaces every var(--name[, fallback]) in ts with the named
// value (or the substituted fallback). ok is false when a reference
// cannot resolve: the declaration is then invalid at computed-value
// time.
func substitute(ts []token, lookup func(string) ([]token, bool), depth int) ([]token, bool) {
	if depth > maxVarDepth {
		return nil, false
	}
	var out []token
	for i := 0; i < len(ts); i++ {
		t := ts[i]
		if t.kind != tkFunc || t.s != "var" {
			out = append(out, t)
			continue
		}
		end := blockEnd(ts, i)
		args := trimWS(funcArgs(ts[i:end]))
		if len(args) == 0 || args[0].kind != tkIdent || !strings.HasPrefix(args[0].s, "--") {
			return nil, false
		}
		name := args[0].s
		rest := trimWS(args[1:])
		val, found := lookup(name)
		switch {
		case found:
			out = append(out, val...)
		case len(rest) > 0 && rest[0].kind == tkComma:
			fb, ok := substitute(rest[1:], lookup, depth+1)
			if !ok {
				return nil, false
			}
			out = append(out, trimWS(fb)...)
		default:
			return nil, false
		}
		i = end - 1
	}
	return out, true
}

// copyProp copies longhand p's computed value from src to dst.
func copyProp(dst, src *Values, p Prop) {
	switch p {
	case PropColor:
		dst.Color = src.Color
	case PropBackgroundColor:
		dst.Background = src.Background
	case PropBackgroundImage:
		dst.Image = src.Image
		dst.BgImageURL = src.BgImageURL
	case PropOpacity:
		dst.Opacity = src.Opacity
	case PropContainerType:
		dst.ContainerType = src.ContainerType
	case PropContainerName:
		dst.ContainerNames = src.ContainerNames
	case PropFilter:
		dst.Brightness = src.Brightness
		dst.Filter = src.Filter
	case PropPaddingTop:
		dst.Padding.Top = src.Padding.Top
	case PropPaddingRight:
		dst.Padding.Right = src.Padding.Right
	case PropPaddingBottom:
		dst.Padding.Bottom = src.Padding.Bottom
	case PropPaddingLeft:
		dst.Padding.Left = src.Padding.Left
	case PropMarginTop:
		dst.Margin.Top = src.Margin.Top
	case PropMarginRight:
		dst.Margin.Right = src.Margin.Right
	case PropMarginBottom:
		dst.Margin.Bottom = src.Margin.Bottom
	case PropMarginLeft:
		dst.Margin.Left = src.Margin.Left
	case PropBorderTopWidth:
		dst.BorderWidth.Top = src.BorderWidth.Top
	case PropBorderRightWidth:
		dst.BorderWidth.Right = src.BorderWidth.Right
	case PropBorderBottomWidth:
		dst.BorderWidth.Bottom = src.BorderWidth.Bottom
	case PropBorderLeftWidth:
		dst.BorderWidth.Left = src.BorderWidth.Left
	case PropBorderTopStyle:
		dst.BorderStyle[0] = src.BorderStyle[0]
	case PropBorderRightStyle:
		dst.BorderStyle[1] = src.BorderStyle[1]
	case PropBorderBottomStyle:
		dst.BorderStyle[2] = src.BorderStyle[2]
	case PropBorderLeftStyle:
		dst.BorderStyle[3] = src.BorderStyle[3]
	case PropBorderTopColor:
		dst.BorderColor[0] = src.BorderColor[0]
	case PropBorderRightColor:
		dst.BorderColor[1] = src.BorderColor[1]
	case PropBorderBottomColor:
		dst.BorderColor[2] = src.BorderColor[2]
	case PropBorderLeftColor:
		dst.BorderColor[3] = src.BorderColor[3]
	case PropBorderTopLeftRadius:
		dst.Radius.TopLeft = src.Radius.TopLeft
	case PropBorderTopRightRadius:
		dst.Radius.TopRight = src.Radius.TopRight
	case PropBorderBottomRightRadius:
		dst.Radius.BottomRight = src.Radius.BottomRight
	case PropBorderBottomLeftRadius:
		dst.Radius.BottomLeft = src.Radius.BottomLeft
	case PropBoxShadow:
		dst.Shadow = src.Shadow
	case PropOutlineWidth:
		dst.OutlineWidth = src.OutlineWidth
	case PropOutlineStyle:
		dst.OutlineStyle = src.OutlineStyle
	case PropOutlineColor:
		dst.OutlineColor = src.OutlineColor
	case PropOutlineOffset:
		dst.OutlineOffset = src.OutlineOffset
	case PropMinWidth:
		dst.MinWidth = src.MinWidth
	case PropMinHeight:
		dst.MinHeight = src.MinHeight
	case PropBorderSpacing:
		dst.BorderSpacingH, dst.BorderSpacingV = src.BorderSpacingH, src.BorderSpacingV
	case PropFontFamily:
		dst.FontFamily = src.FontFamily
	case PropFontSize:
		dst.FontSize = src.FontSize
	case PropFontWeight:
		dst.FontWeight = src.FontWeight
	case PropFontStyle:
		dst.Italic = src.Italic
	case PropLetterSpacing:
		dst.LetterSpacing = src.LetterSpacing
	case PropLineHeight:
		dst.LineHeight = src.LineHeight
	case PropFontFeatures:
		dst.Features = src.Features
	case PropFontVariations:
		dst.Variations = src.Variations
	case PropTransform:
		dst.Transform = src.Transform
	case PropTransformOrigin:
		dst.OriginFrac, dst.OriginPx = src.OriginFrac, src.OriginPx
	case PropTextTransform:
		dst.TextTransform = src.TextTransform
	case PropIconSize:
		dst.IconSize = src.IconSize
	case PropTransitionProperty:
		dst.Transition.All, dst.Transition.Props = src.Transition.All, src.Transition.Props
	case PropTransitionDuration:
		dst.Transition.Duration = src.Transition.Duration
	case PropTransitionTiming:
		dst.Transition.Timing = src.Transition.Timing
	case PropTransitionDelay:
		dst.Transition.Delay = src.Transition.Delay
	case PropAnimation:
		dst.Animation = src.Animation
	case PropAnimationPlayState:
		for i := range dst.Animation {
			dst.Animation[i].Running = src.Animation[i].Running
		}
	case PropAnimDuration:
		dst.AnimDuration = src.AnimDuration
	case PropAnimDelay:
		dst.AnimDelay = src.AnimDelay
	case PropAnimDirection:
		dst.AnimDirection = src.AnimDirection
	case PropAnimFill:
		dst.AnimFill = src.AnimFill
	case PropIconTransform:
		dst.IconXform = src.IconXform
	case PropIconSource:
		dst.IconSource = src.IconSource
	case PropIconPalette:
		dst.PaletteTint = src.PaletteTint
	case PropCaretColor:
		dst.CaretColor = src.CaretColor
	case PropTextDecoration:
		dst.Decoration = src.Decoration
	case PropCursor:
		dst.Cursor = src.Cursor
	case PropPointerEvents:
		dst.PointerEvents = src.PointerEvents
	case PropUserSelect:
		dst.UserSelect = src.UserSelect
	case PropCaretShape:
		dst.CaretShape = src.CaretShape
	case PropAccentColor:
		dst.AccentColor, dst.AccentAuto = src.AccentColor, src.AccentAuto
	case PropAppearance:
		dst.Appearance = src.Appearance
	case PropTouchAction:
		dst.TouchAction = src.TouchAction
	case PropTextAlign:
		dst.TextAlign = src.TextAlign
	case PropTextIndent:
		dst.TextIndent = src.TextIndent
	case PropWhiteSpace:
		dst.WhiteSpace = src.WhiteSpace
	case PropWordBreak:
		dst.WordBreak = src.WordBreak
	case PropLineBreak:
		dst.LineBreak = src.LineBreak
	case PropHyphens:
		dst.Hyphens = src.Hyphens
	case PropWordSpacing:
		dst.WordSpacing = src.WordSpacing
	case PropVerticalAlign:
		dst.VerticalAlign, dst.VerticalAlignPx, dst.VerticalAlignPct = src.VerticalAlign, src.VerticalAlignPx, src.VerticalAlignPct
	case PropLineClamp:
		dst.LineClamp = src.LineClamp
	case PropTextOverflow:
		dst.Overflow = src.Overflow
	case PropFontVariantCaps:
		dst.VariantCaps = src.VariantCaps
	case PropFontVariantNumeric:
		dst.VariantNumeric = src.VariantNumeric
	case PropFontVariantLigatures:
		dst.VariantLig = src.VariantLig
	case PropFontVariantEastAsian:
		dst.VariantEA = src.VariantEA
	case PropFontVariantPosition:
		dst.VariantPos = src.VariantPos
	case PropFontVariantAlternates:
		dst.VariantAlt = src.VariantAlt
	case PropFontSynthesis:
		dst.Synthesis = src.Synthesis
	case PropFontOpticalSizing:
		dst.OpticalSizing = src.OpticalSizing
	case PropDirection:
		dst.Direction = src.Direction
	case PropUnicodeBidi:
		dst.UnicodeBidi = src.UnicodeBidi
	case PropWritingMode:
		dst.WritingMode = src.WritingMode
	case PropBackdropFilter:
		dst.Backdrop = src.Backdrop
	case PropMixBlendMode:
		dst.Blend = src.Blend
	case PropIsolation:
		dst.Isolate = src.Isolate
	case PropClipPath:
		dst.Clip = src.Clip
	case PropMaskImage:
		dst.Mask.Kind, dst.Mask.Image, dst.Mask.URL = src.Mask.Kind, src.Mask.Image, src.Mask.URL
	case PropMaskPosition:
		dst.Mask.PosX, dst.Mask.PosY = src.Mask.PosX, src.Mask.PosY
	case PropMaskSize:
		dst.Mask.SizeKind, dst.Mask.SizeW, dst.Mask.SizeH = src.Mask.SizeKind, src.Mask.SizeW, src.Mask.SizeH
	case PropMaskRepeat:
		dst.Mask.Repeat = src.Mask.Repeat
	case PropMaskMode:
		dst.Mask.Luminance = src.Mask.Luminance
	case PropPerspective:
		dst.Perspective = src.Perspective
	case PropTransformStyle:
		dst.Preserve3D = src.Preserve3D
	case PropBackfaceVisibility:
		dst.BackfaceHidden = src.BackfaceHidden
	case PropScrollBehavior:
		dst.ScrollSmooth = src.ScrollSmooth
	case PropScrollMarginTop:
		dst.ScrollMargin.Top = src.ScrollMargin.Top
	case PropScrollMarginRight:
		dst.ScrollMargin.Right = src.ScrollMargin.Right
	case PropScrollMarginBottom:
		dst.ScrollMargin.Bottom = src.ScrollMargin.Bottom
	case PropScrollMarginLeft:
		dst.ScrollMargin.Left = src.ScrollMargin.Left
	case PropScrollPaddingTop:
		dst.ScrollPad.Top = src.ScrollPad.Top
	case PropScrollPaddingRight:
		dst.ScrollPad.Right = src.ScrollPad.Right
	case PropScrollPaddingBottom:
		dst.ScrollPad.Bottom = src.ScrollPad.Bottom
	case PropScrollPaddingLeft:
		dst.ScrollPad.Left = src.ScrollPad.Left
	case PropScrollSnapType:
		dst.Snap = src.Snap
	case PropScrollSnapAlign:
		dst.SnapAlign = src.SnapAlign
	case PropScrollSnapStop:
		dst.SnapStopAlways = src.SnapStopAlways
	case PropOverscrollX:
		dst.Overscroll[0] = src.Overscroll[0]
	case PropOverscrollY:
		dst.Overscroll[1] = src.Overscroll[1]
	case PropScrollbarColor:
		dst.BarThumb, dst.BarTrack, dst.BarColorAuto = src.BarThumb, src.BarTrack, src.BarColorAuto
	case PropScrollbarWidth:
		dst.BarWidth = src.BarWidth
	case PropScrollbarGutter:
		dst.BarGutter = src.BarGutter
	case PropTransitionBehavior:
		dst.Transition.Discrete = src.Transition.Discrete
	case PropAnimComposition:
		dst.AnimComposition = src.AnimComposition
	}
}

// tokensText serializes tokens back to CSS source text.
func tokensText(ts []token) string {
	var b strings.Builder
	for _, t := range ts {
		switch t.kind {
		case tkIdent:
			b.WriteString(t.s)
		case tkFunc:
			b.WriteString(t.s)
			b.WriteByte('(')
		case tkAt:
			b.WriteByte('@')
			b.WriteString(t.s)
		case tkHash:
			b.WriteByte('#')
			b.WriteString(t.s)
		case tkString:
			b.WriteString(strconv.Quote(t.s))
		case tkNumber:
			b.WriteString(strconv.FormatFloat(t.num, 'f', -1, 64))
		case tkPercent:
			b.WriteString(strconv.FormatFloat(t.num, 'f', -1, 64))
			b.WriteByte('%')
		case tkDim:
			b.WriteString(strconv.FormatFloat(t.num, 'f', -1, 64))
			b.WriteString(t.s)
		case tkWS:
			b.WriteByte(' ')
		default:
			b.WriteString(t.s)
		}
	}
	return b.String()
}
