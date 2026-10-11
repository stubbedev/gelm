package style

import (
	"math"
	"slices"
	"strings"
)

// ContainerType is the container-type keyword.
type ContainerType uint8

// container-type keywords.
const (
	ContainerNormal ContainerType = iota
	ContainerInlineSize
	ContainerSize
)

// Container is one query container a @container condition and the
// container-query units resolve against: an ancestor widget with
// container-type set, as the widget layer reports it. Size is false
// for an inline-size container, whose block axis stays unqueryable;
// Names is its container-name list.
type Container struct {
	W, H  float64
	Size  bool
	Names string
}

// containerNode is the optional Node interface that lists the node's
// query containers: the widget layer walks the node's style chain and
// appends every ancestor whose computed style sets container-type,
// nearest first, with its arranged bounds.
type containerNode interface {
	StyleContainers(buf []Container) []Container
}

// containerCond is one <container-condition>: an optional name filter
// and a size query over the container features.
type containerCond struct {
	name string
	q    mqExpr
	// block reports a block-axis feature in q: only a size container
	// is eligible to answer it.
	block bool
}

// containerQuery is one @container rule's condition, nested rules
// included: every level must hold, and a level holds when one of its
// comma-separated conditions does — each against its own query
// container.
type containerQuery [][]containerCond

// blockFeats are the features an inline-size container cannot
// evaluate: its block axis and the two-axis features.
var blockFeats = map[mediaFeat]bool{featHeight: true, featAspect: true, featOrientation: true}

// matches evaluates the query against conts, the node's query
// containers nearest first.
func (cq containerQuery) matches(conts []Container) bool {
	for _, level := range cq {
		if !slices.ContainsFunc(level, func(c containerCond) bool { return c.matches(conts) }) {
			return false
		}
	}
	return true
}

// matches evaluates one condition against its query container
// (css-contain-3 §6.1): the nearest container whose name list holds
// the condition's name and whose type can answer every queried
// feature. Without an eligible container the condition is unknown,
// which reads false.
func (c *containerCond) matches(conts []Container) bool {
	for i := range conts {
		k := &conts[i]
		if c.name != "" && !containerNamed(k.Names, c.name) || c.block && !k.Size {
			continue
		}
		return c.q.eval(nil, k) == triTrue
	}
	return false
}

// matchContainer is mediaCond.match with the container's size as the
// viewport (resolution never appears in a container query).
func (c mediaCond) matchContainer(k Container) bool {
	m := Media{Width: k.W, Height: k.H}
	return c.match(&m)
}

// containerNamed reports whether the space-separated name list holds
// name (container names are case-sensitive custom idents).
func containerNamed(list, name string) bool {
	return slices.Contains(strings.Fields(list), name)
}

// condsEqual compares two @container condition lists.
func condsEqual(a, b []containerCond) bool {
	return slices.EqualFunc(a, b, func(x, y containerCond) bool {
		return x.name == y.name && x.block == y.block && exprEqual(&x.q, &y.q)
	})
}

// parseContainerQuery parses a @container prelude: a comma list of
// conditions, each an optional name (a leading custom ident that is
// none of not/and/or/none) and a size query in the media condition
// grammar with the container feature names (inline-size/block-size)
// mapped onto the media ones.
func parseContainerQuery(ts []token) ([]containerCond, bool) {
	var out []containerCond
	for _, raw := range splitTop(ts, tkComma) {
		c, ok := parseContainerCond(trimWS(raw))
		if !ok {
			return nil, false
		}
		out = append(out, c)
	}
	return out, len(out) > 0
}

// parseContainerCond parses one <container-condition>.
func parseContainerCond(ts []token) (containerCond, bool) {
	if len(ts) == 0 {
		return containerCond{}, false
	}
	var c containerCond
	if ts[0].kind == tkIdent {
		switch strings.ToLower(ts[0].s) {
		case "not", "and", "or", "none":
		default:
			c.name = ts[0].s
			ts = trimWS(ts[1:])
		}
	}
	if len(ts) == 0 {
		return containerCond{}, false
	}
	q, ok := parseCondition(mapContainerNames(ts), true)
	if !ok {
		return containerCond{}, false
	}
	c.q, c.block = q, q.usesBlock()
	return c, true
}

// mapContainerNames rewrites the container feature names of a
// condition's tokens onto the media feature names (keeping the
// min-/max- prefixes); unknown names pass through and the query
// parser evaluates them as false.
func mapContainerNames(ts []token) []token {
	out := make([]token, 0, len(ts))
	for _, t := range ts {
		if t.kind != tkIdent {
			out = append(out, t)
			continue
		}
		nt := t
		nt.s = containerFeatName(strings.ToLower(t.s))
		out = append(out, nt)
	}
	return out
}

// containerFeatName maps one feature name, with its optional
// min-/max- prefix, onto the media grammar's name for it; any other
// identifier passes through unchanged (a value keyword, or an
// unknown feature the query parser then evaluates as false).
func containerFeatName(name string) string {
	prefix := ""
	switch {
	case strings.HasPrefix(name, "min-"):
		prefix, name = "min-", name[4:]
	case strings.HasPrefix(name, "max-"):
		prefix, name = "max-", name[4:]
	}
	switch name {
	case "width", "height", "aspect-ratio", "orientation":
		return prefix + name
	case "inline-size":
		return prefix + "width"
	case "block-size":
		return prefix + "height"
	}
	return name
}

// cqUnit resolves one container-query unit (css-contain-3 §8): cqw
// and cqi against the nearest query container, cqh and cqb against the
// nearest size container (the only kind with a queryable block axis),
// cqmin and cqmax the smaller and larger of the two. An axis with no
// eligible container takes the viewport's size, the small viewport
// the spec names.
func cqUnit(unit string, conts []Container, vpW, vpH float64) (float64, bool) {
	w, h := vpW, vpH
	if len(conts) > 0 {
		w = conts[0].W
	}
	if i := slices.IndexFunc(conts, func(c Container) bool { return c.Size }); i >= 0 {
		h = conts[i].H
	}
	switch unit {
	case "cqw", "cqi":
		return w / 100, true
	case "cqh", "cqb":
		return h / 100, true
	case "cqmin":
		return math.Min(w, h) / 100, true
	case "cqmax":
		return math.Max(w, h) / 100, true
	}
	return 0, false
}

// parseContainerType parses container-type: normal (no query
// container), inline-size (the inline axis queries), or size (both).
func parseContainerType(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) != 1 || len(comps[0]) != 1 || comps[0][0].kind != tkIdent {
		return false
	}
	switch strings.ToLower(comps[0][0].s) {
	case "normal":
		v.ContainerType = ContainerNormal
	case "inline-size":
		v.ContainerType = ContainerInlineSize
	case "size":
		v.ContainerType = ContainerSize
	default:
		return false
	}
	return true
}

// parseContainerName parses container-name: none, or a
// space-separated custom-ident list a @container name filter
// matches against.
func parseContainerName(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) == 0 {
		return false
	}
	if len(comps) == 1 && len(comps[0]) == 1 && comps[0][0].ident("none") {
		v.ContainerNames = ""
		return true
	}
	var names []string
	for _, c := range comps {
		if len(c) != 1 || c[0].kind != tkIdent || c[0].ident("none") {
			return false
		}
		names = append(names, c[0].s)
	}
	v.ContainerNames = strings.Join(names, " ")
	return true
}

// parseContainer parses the container shorthand: a container-name,
// then optionally `/` and a container-type (normal when omitted).
func parseContainer(ts []token, cx *ctx, v *Values) bool {
	name, typ := ts, []token(nil)
	if i := slices.IndexFunc(ts, func(t token) bool { return t.is('/') }); i >= 0 {
		name, typ = trimWS(ts[:i]), trimWS(ts[i+1:])
		if len(typ) == 0 {
			return false
		}
	}
	if !parseContainerName(name, cx, v) {
		return false
	}
	if typ == nil {
		v.ContainerType = ContainerNormal
		return true
	}
	return parseContainerType(typ, cx, v)
}
