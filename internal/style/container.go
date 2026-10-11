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

// Container is the query container a @container condition and the
// container-query units resolve against: the nearest ancestor widget
// with container-type set, as the widget layer reports it. Size is
// false for an inline-size container, whose block axis stays
// unqueryable; Names is its container-name list.
type Container struct {
	W, H  float64
	Size  bool
	Names string
}

// containerNode is the optional Node interface that resolves the
// nearest query container: the widget layer walks the node's style
// chain and reports the closest ancestor whose computed style sets
// container-type, with its arranged bounds.
type containerNode interface {
	StyleContainer() (Container, bool)
}

// containerQuery is one @container condition: an optional name filter
// (matched against the query container's container-name list) and a
// size query in the @media condition grammar over the container
// feature names.
type containerQuery struct {
	name string
	q    mediaQuery
}

// blockFeats are the features an inline-size container cannot
// evaluate: its block axis and the two-axis features read as unknown.
var blockFeats = map[mediaFeat]bool{featHeight: true, featAspect: true, featOrientation: true}

// matches evaluates the query against c; no container means no match.
func (cq containerQuery) matches(c Container, have bool) bool {
	if !have {
		return false
	}
	if cq.name != "" && !containerNamed(c.Names, cq.name) {
		return false
	}
	for _, branch := range cq.q {
		ok := true
		for _, part := range branch {
			if !part.matchContainer(c) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return len(cq.q) == 0
}

// matchContainer is mediaPart.match over a container: the block-axis
// features evaluate unknown (false, negated under not) unless the
// container is a size container.
func (p mediaPart) matchContainer(c Container) bool {
	for _, cond := range p.conds {
		if blockFeats[cond.feat] && !c.Size {
			return p.neg
		}
		if !cond.matchContainer(c) {
			return p.neg
		}
	}
	return !p.neg
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

// parseContainerQuery parses a @container prelude: an optional name
// (a leading custom ident that is none of not/and/or/none), then a
// size query in the @media condition grammar with the container
// feature names (inline-size/block-size) mapped onto the media ones.
func parseContainerQuery(ts []token) (containerQuery, bool) {
	ts = trimWS(ts)
	if len(ts) == 0 {
		return containerQuery{}, false
	}
	var q containerQuery
	if ts[0].kind == tkIdent {
		switch strings.ToLower(ts[0].s) {
		case "not", "and", "or", "none":
		default:
			q.name = ts[0].s
			ts = trimWS(ts[1:])
		}
	}
	if len(ts) == 0 {
		return containerQuery{}, false
	}
	mapped := mapContainerNames(ts)
	rest, ok := parseMediaQuery(mapped)
	if !ok {
		return containerQuery{}, false
	}
	q.q = rest
	return q, true
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

// cqUnit resolves one container-query unit against c: cqw and cqi
// (the inline axis), cqh and cqb (the block axis), cqmin and cqmax.
// Without a query container every container unit resolves to zero —
// the same unset-viewport reading the vw family takes at 0×0.
func cqUnit(unit string, c Container, have bool) (float64, bool) {
	if !have {
		return 0, true
	}
	switch unit {
	case "cqw", "cqi":
		return c.W / 100, true
	case "cqh", "cqb":
		return c.H / 100, true
	case "cqmin":
		return math.Min(c.W, c.H) / 100, true
	case "cqmax":
		return math.Max(c.W, c.H) / 100, true
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
