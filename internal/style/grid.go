package style

// The CSS grid properties (CSS Grid Layout Module Level 2, docs/css.md):
// the track lists of grid-template-columns and grid-template-rows with
// repeat(), minmax(), fit-content() and named lines,
// grid-template-areas, the implicit track sizes and the auto-placement
// flow, the placement longhands with their grid-row, grid-column and
// grid-area shorthands, grid-template and grid, and justify-items and
// justify-self with place-items and place-self. They compute here;
// widget/cssgrid.go lays the grid out. A track list or an area map is
// variable-length, so it computes to an interned pointer: equal values
// share one, which keeps Layout comparable by value with ==.

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"weak"
)

// GridLineLimit bounds grid line numbers and track counts, as browsers
// clamp them: a placement past it clamps to it and a repeat() expanding
// past it stops there, so a stylesheet cannot ask for unbounded grids.
const GridLineLimit = 10000

// TrackKind is the form of one bound of a track sizing function.
type TrackKind uint8

// Track breadth kinds. Auto is the zero value; FitContent is a max bound
// only, its limit in Len, and Fr likewise, its factor in Fr.
const (
	TrackAuto TrackKind = iota
	TrackLength
	TrackFr
	TrackMinContent
	TrackMaxContent
	TrackFitContent
)

// TrackBreadth is one bound of a track sizing function: a
// <length-percentage> or fit-content()'s limit in Len, a flex factor in
// Fr, or a keyword.
type TrackBreadth struct {
	Kind TrackKind
	Len  Length
	Fr   float64
}

// TrackSize is one track's sizing function as minmax(Min, Max). A single
// breadth b computes to minmax(b, b), except a flex factor, which is
// minmax(auto, <flex>), and fit-content(L), minmax(auto, fit-content(L)).
type TrackSize struct{ Min, Max TrackBreadth }

// autoTrack is the auto track size: minmax(auto, auto).
var autoTrack = TrackSize{}

// fixed reports a <fixed-size>: a definite length bound on either side.
// An auto repetition, and the tracks beside one, take only these.
func (t TrackSize) fixed() bool { return t.Min.Kind == TrackLength || t.Max.Kind == TrackLength }

// TrackRun is a run of tracks with the names of the lines around them:
// Names[i] names the line before Sizes[i], the last entry the line after
// the run.
type TrackRun struct {
	Sizes []TrackSize
	Names [][]string
}

// RepeatKind is the keyword of an auto repetition.
type RepeatKind uint8

// Auto repetition keywords; RepeatNone is a track list without one.
const (
	RepeatNone RepeatKind = iota
	RepeatFill
	RepeatFit
)

// TrackList is a computed grid-template-columns or grid-template-rows
// value other than none: the tracks before an auto-fill or auto-fit
// repetition (all of them without one), the repeated run, and the tracks
// after it. Integer repetitions expand at parse time. Subgrid marks
// `subgrid`, which carries no tracks (docs/css.md's grid deferrals).
type TrackList struct {
	Subgrid bool
	Head    TrackRun
	Repeat  RepeatKind
	Auto    TrackRun
	Tail    TrackRun
}

// Count is the number of tracks with the auto repetition repeated reps
// times.
func (t *TrackList) Count(reps int) int {
	return len(t.Head.Sizes) + reps*len(t.Auto.Sizes) + len(t.Tail.Sizes)
}

// Size is the sizing function of track i with reps repetitions.
func (t *TrackList) Size(i, reps int) TrackSize {
	if i < len(t.Head.Sizes) {
		return t.Head.Sizes[i]
	}
	i -= len(t.Head.Sizes)
	if n := reps * len(t.Auto.Sizes); i < n {
		return t.Auto.Sizes[i%len(t.Auto.Sizes)]
	}
	return t.Tail.Sizes[i-reps*len(t.Auto.Sizes)]
}

// Repeated reports whether track i, with reps repetitions, is one the
// auto repetition generated: what auto-fit collapses when it is empty.
func (t *TrackList) Repeated(i, reps int) bool {
	i -= len(t.Head.Sizes)
	return t.Repeat != RepeatNone && i >= 0 && i < reps*len(t.Auto.Sizes)
}

// LineNamed reports whether line i (zero-based, before track i) of the
// list with reps repetitions carries the name name+suffix; the suffix
// spares a caller building an area's "<ident>-start" to look it up. A
// line where two runs meet carries both runs' names.
func (t *TrackList) LineNamed(i, reps int, name, suffix string) bool {
	h := len(t.Head.Sizes)
	if i < h {
		return runNamed(t.Head, i, name, suffix)
	}
	if t.Repeat == RepeatNone || reps == 0 {
		return runNamed(t.Head, h, name, suffix) || runNamed(t.Tail, i-h, name, suffix)
	}
	n := len(t.Auto.Sizes)
	k := i - h
	switch {
	case k == 0:
		return runNamed(t.Head, h, name, suffix) || runNamed(t.Auto, 0, name, suffix)
	case k < reps*n && k%n == 0:
		return runNamed(t.Auto, n, name, suffix) || runNamed(t.Auto, 0, name, suffix)
	case k < reps*n:
		return runNamed(t.Auto, k%n, name, suffix)
	case k == reps*n:
		return runNamed(t.Auto, n, name, suffix) || runNamed(t.Tail, 0, name, suffix)
	}
	return runNamed(t.Tail, k-reps*n, name, suffix)
}

// runNamed reports whether line i of the run carries name+suffix.
func runNamed(r TrackRun, i int, name, suffix string) bool {
	if i < 0 || i >= len(r.Names) {
		return false
	}
	for _, n := range r.Names[i] {
		if joinedEq(n, "", name, suffix) {
			return true
		}
	}
	return false
}

// joinedEq reports whether a+b equals c+d without building either.
func joinedEq(a, b, c, d string) bool {
	if len(a)+len(b) != len(c)+len(d) {
		return false
	}
	if len(a) > len(c) {
		a, b, c, d = c, d, a, b
	}
	k := len(c) - len(a)
	return c[:len(a)] == a && b[:k] == c[len(a):] && b[k:] == d
}

// AutoTracks is a computed grid-auto-columns or grid-auto-rows value: the
// sizes the implicit tracks cycle through. A nil list is auto.
type AutoTracks struct {
	Sizes []TrackSize
}

// At is the size of implicit track k: k counts from zero after the
// explicit grid, and from -1 backwards before it, so the first track
// after takes the first size and the last track before the last size.
func (a *AutoTracks) At(k int) TrackSize {
	if a == nil {
		return autoTrack
	}
	n := len(a.Sizes)
	return a.Sizes[((k%n)+n)%n]
}

// LineSpan is a run of grid lines, zero-based, End exclusive of the
// tracks: a grid area's extent along one axis.
type LineSpan struct{ Start, End int }

// GridArea is one named area of grid-template-areas.
type GridArea struct {
	Name        string
	Column, Row LineSpan
}

// GridAreas is a computed grid-template-areas value other than none: the
// template's size and its named areas, in order of first appearance.
type GridAreas struct {
	Rows, Columns int
	Areas         []GridArea
}

// LineNamed reports whether line i of the template's columns (or rows)
// carries the name name+suffix: each area names its edges
// <area>-start and <area>-end, the implicit names CSS gives them.
func (g *GridAreas) LineNamed(columns bool, i int, name, suffix string) bool {
	if g == nil {
		return false
	}
	for _, a := range g.Areas {
		span := a.Row
		if columns {
			span = a.Column
		}
		if span.Start == i && joinedEq(a.Name, "-start", name, suffix) || span.End == i && joinedEq(a.Name, "-end", name, suffix) {
			return true
		}
	}
	return false
}

// GridAutoFlow is grid-auto-flow: the axis auto-placement fills (rows
// unless Column) and whether it packs densely.
type GridAutoFlow struct {
	Column, Dense bool
}

// GridLineKind is the form of one placement line.
type GridLineKind uint8

// Placement line kinds. Ident is a lone <custom-ident>: a named area's
// edge (the line called <ident>-start or <ident>-end), else the first
// line of that name. Number is the Nth line, negative from the end,
// counting only lines called Name when it is set. Span spans N tracks,
// or to the Nth line called Name.
const (
	GridLineAuto GridLineKind = iota
	GridLineIdent
	GridLineNumber
	GridLineSpan
)

// GridLine is one computed grid-row-start, grid-row-end,
// grid-column-start or grid-column-end value.
type GridLine struct {
	Kind GridLineKind
	N    int
	Name string
}

// gridContainerProps are the grid container longhands the grid
// shorthand resets.
var gridContainerProps = setOf(PropGridTemplateColumns, PropGridTemplateRows, PropGridTemplateAreas,
	PropGridAutoColumns, PropGridAutoRows, PropGridAutoFlow)

// gridPropDefs are the grid properties' definitions, merged into
// propTable.
func gridPropDefs() map[string]propDef {
	return map[string]propDef{
		"grid-template-columns": {setOf(PropGridTemplateColumns), templateInto(func(l *Layout) **TrackList { return &l.GridTemplateColumns })},
		"grid-template-rows":    {setOf(PropGridTemplateRows), templateInto(func(l *Layout) **TrackList { return &l.GridTemplateRows })},
		"grid-template-areas":   {setOf(PropGridTemplateAreas), parseTemplateAreas},
		"grid-template":         {setOf(PropGridTemplateColumns, PropGridTemplateRows, PropGridTemplateAreas), parseGridTemplate},
		"grid-auto-columns":     {setOf(PropGridAutoColumns), autoTracksInto(func(l *Layout) **AutoTracks { return &l.GridAutoColumns })},
		"grid-auto-rows":        {setOf(PropGridAutoRows), autoTracksInto(func(l *Layout) **AutoTracks { return &l.GridAutoRows })},
		"grid-auto-flow":        {setOf(PropGridAutoFlow), parseGridAutoFlow},
		"grid":                  {gridContainerProps, parseGrid},
		"grid-column-start":     {setOf(PropGridColumnStart), gridLineInto(func(l *Layout) *GridLine { return &l.GridColumnStart })},
		"grid-column-end":       {setOf(PropGridColumnEnd), gridLineInto(func(l *Layout) *GridLine { return &l.GridColumnEnd })},
		"grid-row-start":        {setOf(PropGridRowStart), gridLineInto(func(l *Layout) *GridLine { return &l.GridRowStart })},
		"grid-row-end":          {setOf(PropGridRowEnd), gridLineInto(func(l *Layout) *GridLine { return &l.GridRowEnd })},
		"grid-column":           {setOf(PropGridColumnStart, PropGridColumnEnd), gridAxisInto(func(l *Layout) (*GridLine, *GridLine) { return &l.GridColumnStart, &l.GridColumnEnd })},
		"grid-row":              {setOf(PropGridRowStart, PropGridRowEnd), gridAxisInto(func(l *Layout) (*GridLine, *GridLine) { return &l.GridRowStart, &l.GridRowEnd })},
		"grid-area":             {setOf(PropGridRowStart, PropGridColumnStart, PropGridRowEnd, PropGridColumnEnd), parseGridArea},
		"justify-items":         {setOf(PropJustifyItems), parseJustifyItems},
		"justify-self":          {setOf(PropJustifySelf), alignInto(func(v *Values) *BoxAlign { return &v.Layout.JustifySelf }, justifySelfKinds)},
		"place-items":           {setOf(PropAlignItems, PropJustifyItems), parsePlaceItems},
		"place-self":            {setOf(PropAlignSelf, PropJustifySelf), parsePlaceSelf},
	}
}

// internTable shares computed values by content: equal values intern to
// one pointer. Entries are weak, so a value no style holds any more
// drops out instead of accumulating; parsing runs on any goroutine, so
// the table locks.
type internTable[T any] struct {
	mu sync.Mutex
	m  map[string]weak.Pointer[T]
}

var (
	trackLists = &internTable[TrackList]{}
	autoLists  = &internTable[AutoTracks]{}
	areaMaps   = &internTable[GridAreas]{}
)

// intern returns the shared pointer for v's content, v itself when it
// is the first of its value. The key is the Go-syntax rendering, which
// spells every field of these pointer-free types.
func (t *internTable[T]) intern(v *T) *T {
	key := fmt.Sprintf("%#v", *v)
	t.mu.Lock()
	defer t.mu.Unlock()
	if p, ok := t.m[key]; ok {
		if old := p.Value(); old != nil {
			return old
		}
	}
	if t.m == nil {
		t.m = map[string]weak.Pointer[T]{}
	}
	t.m[key] = weak.Make(v)
	runtime.AddCleanup(v, t.drop, key)
	return v
}

// drop forgets key once its value is collected, unless a newer value
// took the key since.
func (t *internTable[T]) drop(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if p, ok := t.m[key]; ok && p.Value() == nil {
		delete(t.m, key)
	}
}

// splitSlash splits a value at its top-level `/` delimiters.
func splitSlash(ts []token) [][]token {
	var out [][]token
	start := 0
	for i := 0; i < len(ts); {
		if closer(ts[i].kind) != 0 {
			i = blockEnd(ts, i)
			continue
		}
		if ts[i].is('/') {
			out = append(out, ts[start:i])
			start = i + 1
		}
		i++
	}
	return append(out, ts[start:])
}

// isFunc reports whether component c is a call of the function name.
func isFunc(c []token, name string) bool {
	return c[0].kind == tkFunc && strings.EqualFold(c[0].s, name)
}

// customIdentOf reads component c as a grid <custom-ident>: an
// identifier other than span, auto, and the CSS-wide keywords. Names
// keep their case, as CSS compares them.
func customIdentOf(c []token) (string, bool) {
	if len(c) != 1 || c[0].kind != tkIdent {
		return "", false
	}
	s := c[0].s
	switch strings.ToLower(s) {
	case "span", "auto", "default":
		return "", false
	}
	return s, wideOf(c) == wideNone
}

// lineNamesOf parses one bracketed <line-names> group.
func lineNamesOf(c []token) ([]string, bool) {
	if c[0].kind != tkLBrack || c[len(c)-1].kind != tkRBrack {
		return nil, false
	}
	var names []string
	for _, n := range components(c[1 : len(c)-1]) {
		s, ok := customIdentOf(n)
		if !ok {
			return nil, false
		}
		names = append(names, s)
	}
	return names, true
}

// breadthOf parses one track breadth; min restricts it to the
// inflexible ones a minimum takes.
func breadthOf(c []token, cx *ctx, min bool) (TrackBreadth, bool) {
	if len(c) == 0 {
		return TrackBreadth{}, false
	}
	t := c[0]
	if len(c) == 1 && t.kind == tkIdent {
		switch strings.ToLower(t.s) {
		case "auto":
			return TrackBreadth{}, true
		case "min-content":
			return TrackBreadth{Kind: TrackMinContent}, true
		case "max-content":
			return TrackBreadth{Kind: TrackMaxContent}, true
		}
		return TrackBreadth{}, false
	}
	if len(c) == 1 && t.kind == tkDim && t.s == "fr" {
		return TrackBreadth{Kind: TrackFr, Fr: t.num}, !min && t.num >= 0
	}
	l, ok := lengthPctOf(c, cx)
	return TrackBreadth{Kind: TrackLength, Len: l}, ok && !l.Negative()
}

// trackSizeOf parses one <track-size>: a breadth, minmax(), or
// fit-content().
func trackSizeOf(c []token, cx *ctx) (TrackSize, bool) {
	switch {
	case isFunc(c, "minmax"):
		args := splitTop(funcArgs(c), tkComma)
		if len(args) != 2 {
			return TrackSize{}, false
		}
		lo, okLo := breadthOf(trimWS(args[0]), cx, true)
		hi, okHi := breadthOf(trimWS(args[1]), cx, false)
		return TrackSize{Min: lo, Max: hi}, okLo && okHi
	case isFunc(c, "fit-content"):
		args := components(funcArgs(c))
		if len(args) != 1 {
			return TrackSize{}, false
		}
		l, ok := lengthPctOf(args[0], cx)
		return TrackSize{Max: TrackBreadth{Kind: TrackFitContent, Len: l}}, ok && !l.Negative()
	}
	b, ok := breadthOf(c, cx, false)
	if b.Kind == TrackFr {
		return TrackSize{Max: b}, ok
	}
	return TrackSize{Min: b, Max: b}, ok
}

// runBuilder accumulates a TrackRun: line names wait for the next track
// (or the run's end), and two name groups in a row are invalid.
type runBuilder struct {
	run     TrackRun
	pending []string
	named   bool
}

// names adds a <line-names> group, reporting false for a second group in
// a row.
func (b *runBuilder) names(ns []string) bool {
	if b.named {
		return false
	}
	b.pending, b.named = append(b.pending, ns...), true
	return true
}

// track adds a track after the pending names.
func (b *runBuilder) track(s TrackSize) {
	b.run.Names = append(b.run.Names, b.pending)
	b.run.Sizes = append(b.run.Sizes, s)
	b.pending, b.named = nil, false
}

// end closes the run with the pending names on its last line.
func (b *runBuilder) end() TrackRun {
	b.run.Names = append(b.run.Names, b.pending)
	return b.run
}

// repeatOf parses repeat(): the count (0 for an auto repetition, whose
// kind it reports) and the repeated run, whose sizes an auto repetition
// requires fixed.
func repeatOf(c []token, cx *ctx) (kind RepeatKind, n int, run TrackRun, ok bool) {
	args := splitTop(funcArgs(c), tkComma)
	if len(args) != 2 {
		return 0, 0, TrackRun{}, false
	}
	head := components(args[0])
	switch {
	case len(head) == 1 && head[0][0].ident("auto-fill"):
		kind = RepeatFill
	case len(head) == 1 && head[0][0].ident("auto-fit"):
		kind = RepeatFit
	default:
		if n, ok = integerOf(head, cx); !ok || n < 1 {
			return 0, 0, TrackRun{}, false
		}
	}
	var b runBuilder
	for _, p := range components(args[1]) {
		if p[0].kind == tkLBrack {
			ns, ok := lineNamesOf(p)
			if !ok || !b.names(ns) {
				return 0, 0, TrackRun{}, false
			}
			continue
		}
		s, ok := trackSizeOf(p, cx)
		if !ok || kind != RepeatNone && !s.fixed() {
			return 0, 0, TrackRun{}, false
		}
		b.track(s)
	}
	run = b.end()
	return kind, n, run, len(run.Sizes) > 0
}

// trackListOf parses a <track-list> or <auto-track-list>; repeats false
// takes only an <explicit-track-list> (grid-template's columns after its
// area strings). Integer repetitions expand in place, their boundary
// names merging; the expansion stops at GridLineLimit tracks.
func trackListOf(comps [][]token, cx *ctx, repeats bool) (TrackList, bool) {
	var t TrackList
	var b runBuilder
	count := 0
	for _, c := range comps {
		switch {
		case c[0].kind == tkLBrack:
			ns, ok := lineNamesOf(c)
			if !ok || !b.names(ns) {
				return TrackList{}, false
			}
		case repeats && isFunc(c, "repeat"):
			kind, n, run, ok := repeatOf(c, cx)
			if !ok {
				return TrackList{}, false
			}
			if kind != RepeatNone {
				if t.Repeat != RepeatNone {
					return TrackList{}, false
				}
				t.Head, t.Repeat, t.Auto = b.end(), kind, run
				b = runBuilder{}
				continue
			}
			for range n {
				for j, s := range run.Sizes {
					if count >= GridLineLimit {
						break
					}
					b.pending = append(b.pending, run.Names[j]...)
					b.track(s)
					count++
				}
				b.pending = append(b.pending, run.Names[len(run.Sizes)]...)
			}
			b.named = false
		default:
			s, ok := trackSizeOf(c, cx)
			if !ok {
				return TrackList{}, false
			}
			if count < GridLineLimit {
				b.track(s)
				count++
			}
		}
	}
	if t.Repeat == RepeatNone {
		t.Head = b.end()
	} else {
		t.Tail = b.end()
		fixed := func(r TrackRun) bool {
			for _, s := range r.Sizes {
				if !s.fixed() {
					return false
				}
			}
			return true
		}
		if !fixed(t.Head) || !fixed(t.Tail) {
			return TrackList{}, false
		}
	}
	return t, t.Count(1) > 0
}

// templateOf parses one grid-template-columns or grid-template-rows
// value: none (nil), subgrid with its optional line-name lists, or a
// track list.
func templateOf(ts []token, cx *ctx) (*TrackList, bool) {
	comps := components(ts)
	if len(comps) == 0 {
		return nil, false
	}
	if s, ok := identOf(comps); ok && s == "none" {
		return nil, true
	}
	if comps[0][0].ident("subgrid") {
		return trackLists.intern(&TrackList{Subgrid: true}), subgridNamesOK(comps[1:], cx)
	}
	t, ok := trackListOf(comps, cx, true)
	if !ok {
		return nil, false
	}
	return trackLists.intern(&t), true
}

// subgridNamesOK validates subgrid's <line-name-list>: line-name groups
// and repeat() of them, by an integer or auto-fill (once).
func subgridNamesOK(comps [][]token, cx *ctx) bool {
	autoFill := false
	for _, c := range comps {
		if !isFunc(c, "repeat") {
			if _, ok := lineNamesOf(c); !ok {
				return false
			}
			continue
		}
		args := splitTop(funcArgs(c), tkComma)
		if len(args) != 2 {
			return false
		}
		head := components(args[0])
		if len(head) == 1 && head[0][0].ident("auto-fill") {
			if autoFill {
				return false
			}
			autoFill = true
		} else if n, ok := integerOf(head, cx); !ok || n < 1 {
			return false
		}
		groups := components(args[1])
		if len(groups) == 0 {
			return false
		}
		for _, g := range groups {
			if _, ok := lineNamesOf(g); !ok {
				return false
			}
		}
	}
	return true
}

// templateInto parses grid-template-columns or grid-template-rows.
func templateInto(get func(l *Layout) **TrackList) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		t, ok := templateOf(ts, cx)
		if ok {
			*get(&v.Layout) = t
		}
		return ok
	}
}

// autoTracksOf parses grid-auto-columns or grid-auto-rows: one or more
// track sizes.
func autoTracksOf(comps [][]token, cx *ctx) (*AutoTracks, bool) {
	if len(comps) == 0 {
		return nil, false
	}
	a := AutoTracks{Sizes: make([]TrackSize, 0, len(comps))}
	for _, c := range comps {
		s, ok := trackSizeOf(c, cx)
		if !ok {
			return nil, false
		}
		a.Sizes = append(a.Sizes, s)
	}
	return autoLists.intern(&a), true
}

// autoTracksInto parses grid-auto-columns or grid-auto-rows.
func autoTracksInto(get func(l *Layout) **AutoTracks) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		a, ok := autoTracksOf(components(ts), cx)
		if ok {
			*get(&v.Layout) = a
		}
		return ok
	}
}

// areaCellsOf splits one grid-template-areas string into its cell
// tokens: a run of name code points is a named cell, a run of periods
// a null cell (""); any other code point makes the string invalid.
func areaCellsOf(s string) ([]string, bool) {
	var cells []string
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			i++
		case c == '.':
			for i < len(s) && s[i] == '.' {
				i++
			}
			cells = append(cells, "")
		case isNameByte(c):
			j := i
			for j < len(s) && isNameByte(s[j]) {
				j++
			}
			cells, i = append(cells, s[i:j]), j
		default:
			return nil, false
		}
	}
	return cells, len(cells) > 0
}

// areasOf builds the area map of grid-template-areas' strings: every row
// as wide as the first, every name covering one filled rectangle.
func areasOf(strs []string) (*GridAreas, bool) {
	g := GridAreas{Rows: len(strs)}
	var rows [][]string
	for _, s := range strs {
		cells, ok := areaCellsOf(s)
		if !ok || len(rows) > 0 && len(cells) != g.Columns {
			return nil, false
		}
		g.Columns = len(cells)
		rows = append(rows, cells)
	}
	for r, row := range rows {
		for c, name := range row {
			if name == "" {
				continue
			}
			k := -1
			for i := range g.Areas {
				if g.Areas[i].Name == name {
					k = i
				}
			}
			if k < 0 {
				g.Areas = append(g.Areas, GridArea{Name: name, Column: LineSpan{c, c + 1}, Row: LineSpan{r, r + 1}})
				continue
			}
			a := &g.Areas[k]
			a.Column = LineSpan{min(a.Column.Start, c), max(a.Column.End, c+1)}
			a.Row = LineSpan{min(a.Row.Start, r), max(a.Row.End, r+1)}
		}
	}
	for _, a := range g.Areas {
		for r := a.Row.Start; r < a.Row.End; r++ {
			for c := a.Column.Start; c < a.Column.End; c++ {
				if rows[r][c] != a.Name {
					return nil, false
				}
			}
		}
	}
	return areaMaps.intern(&g), true
}

// parseTemplateAreas parses grid-template-areas: none or one string per
// row.
func parseTemplateAreas(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if s, ok := identOf(comps); ok && s == "none" {
		v.Layout.GridTemplateAreas = nil
		return true
	}
	if len(comps) == 0 {
		return false
	}
	strs := make([]string, 0, len(comps))
	for _, c := range comps {
		if len(c) != 1 || c[0].kind != tkString {
			return false
		}
		strs = append(strs, c[0].s)
	}
	g, ok := areasOf(strs)
	if ok {
		v.Layout.GridTemplateAreas = g
	}
	return ok
}

// gridTemplateOf parses the grid-template shorthand: none, `<rows> /
// <columns>`, or the area strings, each row's optional line names
// around it and its optional size after, then an optional `/` and an
// explicit column list.
func gridTemplateOf(ts []token, cx *ctx) (rows, cols *TrackList, areas *GridAreas, ok bool) {
	comps := components(ts)
	if s, isIdent := identOf(comps); isIdent && s == "none" {
		return nil, nil, nil, true
	}
	parts := splitSlash(ts)
	hasString := false
	for _, c := range components(parts[0]) {
		hasString = hasString || c[0].kind == tkString
	}
	if !hasString {
		if len(parts) != 2 {
			return nil, nil, nil, false
		}
		rows, okR := templateOf(parts[0], cx)
		cols, okC := templateOf(parts[1], cx)
		return rows, cols, nil, okR && okC
	}
	if len(parts) > 2 {
		return nil, nil, nil, false
	}
	// Each row is its string's track, auto until a size follows the
	// string directly; a row's trailing names and the next row's leading
	// names are two groups in a row, so only the ends take one group.
	var b runBuilder
	var strs []string
	groups, afterString := 0, false
	for _, c := range components(parts[0]) {
		switch {
		case c[0].kind == tkLBrack:
			ns, okN := lineNamesOf(c)
			groups++
			if !okN || groups > 2 || len(strs) == 0 && groups > 1 {
				return nil, nil, nil, false
			}
			b.pending = append(b.pending, ns...)
			afterString = false
		case c[0].kind == tkString && len(c) == 1:
			b.track(autoTrack)
			strs = append(strs, c[0].s)
			groups, afterString = 0, true
		default:
			s, okS := trackSizeOf(c, cx)
			if !okS || !afterString {
				return nil, nil, nil, false
			}
			b.run.Sizes[len(b.run.Sizes)-1] = s
			afterString = false
		}
	}
	if groups > 1 {
		return nil, nil, nil, false
	}
	areas, ok = areasOf(strs)
	if !ok {
		return nil, nil, nil, false
	}
	rowList := TrackList{Head: b.end()}
	rows = trackLists.intern(&rowList)
	if len(parts) == 2 {
		colList, okC := trackListOf(components(parts[1]), cx, false)
		if !okC {
			return nil, nil, nil, false
		}
		cols = trackLists.intern(&colList)
	}
	return rows, cols, areas, true
}

// parseGridTemplate parses the grid-template shorthand.
func parseGridTemplate(ts []token, cx *ctx, v *Values) bool {
	rows, cols, areas, ok := gridTemplateOf(ts, cx)
	if ok {
		v.Layout.GridTemplateRows, v.Layout.GridTemplateColumns, v.Layout.GridTemplateAreas = rows, cols, areas
	}
	return ok
}

// autoFlowOf reads a leading `auto-flow && dense?` group, reporting how
// many components it took (0 without auto-flow).
func autoFlowOf(comps [][]token) (dense bool, n int) {
	flow := false
	for _, c := range comps[:min(2, len(comps))] {
		switch {
		case c[0].ident("auto-flow") && !flow:
			flow = true
		case c[0].ident("dense") && !dense:
			dense = true
		default:
			if flow {
				return dense, n
			}
			return false, 0
		}
		n++
	}
	if !flow {
		return false, 0
	}
	return dense, n
}

// parseGrid parses the grid shorthand, which resets every grid container
// longhand first: a grid-template value, or one axis's template with the
// other axis's `auto-flow` (dense optional) and implicit track sizes.
func parseGrid(ts []token, cx *ctx, v *Values) bool {
	l := &v.Layout
	l.GridTemplateRows, l.GridTemplateColumns, l.GridTemplateAreas = nil, nil, nil
	l.GridAutoRows, l.GridAutoColumns, l.GridAutoFlow = nil, nil, GridAutoFlow{}
	parts := splitSlash(ts)
	if len(parts) == 2 {
		left, right := components(parts[0]), components(parts[1])
		if dense, n := autoFlowOf(right); n > 0 {
			rows, ok := templateOf(parts[0], cx)
			if !ok {
				return false
			}
			l.GridTemplateRows, l.GridAutoFlow = rows, GridAutoFlow{Column: true, Dense: dense}
			if n < len(right) {
				l.GridAutoColumns, ok = autoTracksOf(right[n:], cx)
			}
			return ok
		}
		if dense, n := autoFlowOf(left); n > 0 {
			cols, ok := templateOf(parts[1], cx)
			if !ok {
				return false
			}
			l.GridTemplateColumns, l.GridAutoFlow = cols, GridAutoFlow{Dense: dense}
			if n < len(left) {
				l.GridAutoRows, ok = autoTracksOf(left[n:], cx)
			}
			return ok
		}
	}
	return parseGridTemplate(ts, cx, v)
}

// parseGridAutoFlow parses grid-auto-flow: row or column, dense, or
// both in either order.
func parseGridAutoFlow(ts []token, _ *ctx, v *Values) bool {
	comps := components(ts)
	if len(comps) == 0 || len(comps) > 2 {
		return false
	}
	var f GridAutoFlow
	axis, dense := false, false
	for _, c := range comps {
		switch {
		case len(c) == 1 && c[0].ident("row") && !axis:
			axis = true
		case len(c) == 1 && c[0].ident("column") && !axis:
			axis, f.Column = true, true
		case len(c) == 1 && c[0].ident("dense") && !dense:
			dense, f.Dense = true, true
		default:
			return false
		}
	}
	v.Layout.GridAutoFlow = f
	return true
}

// gridLineOf parses one placement line: auto, a lone <custom-ident>,
// `<integer> && <custom-ident>?` (the integer not zero), or `span &&
// [<integer> || <custom-ident>]` (the integer positive), each part in
// any order as browsers read them. Integers clamp to GridLineLimit.
func gridLineOf(ts []token, cx *ctx) (GridLine, bool) {
	comps := components(ts)
	if len(comps) == 0 || len(comps) > 3 {
		return GridLine{}, false
	}
	if s, ok := identOf(comps); ok && s == "auto" {
		return GridLine{}, true
	}
	var l GridLine
	span, haveN := false, false
	for _, c := range comps {
		switch {
		case len(c) == 1 && c[0].ident("span") && !span:
			span = true
		case l.Name == "" && c[0].kind == tkIdent:
			name, ok := customIdentOf(c)
			if !ok {
				return GridLine{}, false
			}
			l.Name = name
		case !haveN:
			n, ok := integerOf([][]token{c}, cx)
			if !ok {
				return GridLine{}, false
			}
			l.N, haveN = max(-GridLineLimit, min(n, GridLineLimit)), true
		default:
			return GridLine{}, false
		}
	}
	switch {
	case span:
		if len(comps) == 1 || haveN && l.N < 1 {
			return GridLine{}, false
		}
		if !haveN {
			l.N = 1
		}
		l.Kind = GridLineSpan
	case haveN:
		l.Kind = GridLineNumber
		if l.N == 0 {
			return GridLine{}, false
		}
	default:
		l.Kind = GridLineIdent
	}
	return l, true
}

// gridLineInto parses one placement longhand.
func gridLineInto(get func(l *Layout) *GridLine) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		l, ok := gridLineOf(ts, cx)
		if ok {
			*get(&v.Layout) = l
		}
		return ok
	}
}

// omittedLine is the value an omitted placement line takes: the given
// line's name when it is a lone identifier, auto otherwise.
func omittedLine(l GridLine) GridLine {
	if l.Kind == GridLineIdent {
		return l
	}
	return GridLine{}
}

// gridLinesOf parses the slash-separated lines of a placement shorthand,
// at most max of them.
func gridLinesOf(ts []token, cx *ctx, max int) ([]GridLine, bool) {
	parts := splitSlash(ts)
	if len(parts) > max {
		return nil, false
	}
	out := make([]GridLine, 0, len(parts))
	for _, p := range parts {
		l, ok := gridLineOf(p, cx)
		if !ok {
			return nil, false
		}
		out = append(out, l)
	}
	return out, true
}

// gridAxisInto parses grid-column or grid-row: a start line and an
// optional end line after a slash.
func gridAxisInto(get func(l *Layout) (*GridLine, *GridLine)) func([]token, *ctx, *Values) bool {
	return func(ts []token, cx *ctx, v *Values) bool {
		lines, ok := gridLinesOf(ts, cx, 2)
		if !ok {
			return false
		}
		start, end := get(&v.Layout)
		*start, *end = lines[0], omittedLine(lines[0])
		if len(lines) == 2 {
			*end = lines[1]
		}
		return true
	}
}

// parseGridArea parses grid-area: row-start, column-start, row-end and
// column-end, slash-separated; an omitted column-start or row-end copies
// a lone identifier row-start, and an omitted column-end a lone
// identifier column-start.
func parseGridArea(ts []token, cx *ctx, v *Values) bool {
	lines, ok := gridLinesOf(ts, cx, 4)
	if !ok {
		return false
	}
	l := &v.Layout
	l.GridRowStart = lines[0]
	l.GridColumnStart, l.GridRowEnd = omittedLine(lines[0]), omittedLine(lines[0])
	if len(lines) > 1 {
		l.GridColumnStart = lines[1]
	}
	if len(lines) > 2 {
		l.GridRowEnd = lines[2]
	}
	l.GridColumnEnd = omittedLine(l.GridColumnStart)
	if len(lines) > 3 {
		l.GridColumnEnd = lines[3]
	}
	return true
}

// The keyword sets of justify-items and justify-self: align-items' and
// align-self's plus left and right.
var (
	justifyItemsKinds = alignItemsKinds | kindsOf(BoxLeft, BoxRight)
	justifySelfKinds  = justifyItemsKinds | kindsOf(BoxAuto)
)

// justifyItemsOf parses justify-items: the alignment keywords, or legacy
// alone (normal) or with left, right or center (that keyword).
func justifyItemsOf(comps [][]token) (BoxAlign, bool) {
	legacy := -1
	for i, c := range comps {
		if c[0].ident("legacy") {
			legacy = i
		}
	}
	switch {
	case legacy < 0:
		return alignOf(comps, justifyItemsKinds)
	case len(comps) == 1:
		return BoxAlign{Kind: BoxNormal}, true
	case len(comps) == 2:
		a, ok := alignOf(comps[1-legacy:2-legacy], kindsOf(BoxLeft, BoxRight, BoxCenter))
		return a, ok
	}
	return BoxAlign{}, false
}

// parseJustifyItems parses justify-items.
func parseJustifyItems(ts []token, _ *ctx, v *Values) bool {
	a, ok := justifyItemsOf(components(ts))
	if ok {
		v.Layout.JustifyItems = a
	}
	return ok
}

// placeOf parses a place-* shorthand: the align half, then the justify
// half, which copies the align half when omitted. Every split point is
// tried, since either half may be one or two keywords.
func placeOf(comps [][]token, align func([][]token) (BoxAlign, bool), justify func([][]token) (BoxAlign, bool)) (a, j BoxAlign, ok bool) {
	if len(comps) == 0 || len(comps) > 4 {
		return BoxAlign{}, BoxAlign{}, false
	}
	for split := 1; split <= len(comps); split++ {
		if a, ok = align(comps[:split]); !ok {
			continue
		}
		rest := comps[split:]
		if len(rest) == 0 {
			rest = comps[:split]
		}
		if j, ok = justify(rest); ok {
			return a, j, true
		}
	}
	return BoxAlign{}, BoxAlign{}, false
}

// parsePlaceItems parses place-items: align-items, then justify-items.
func parsePlaceItems(ts []token, _ *ctx, v *Values) bool {
	a, j, ok := placeOf(components(ts), func(c [][]token) (BoxAlign, bool) { return alignOf(c, alignItemsKinds) }, justifyItemsOf)
	if ok {
		v.Layout.AlignItems, v.Layout.JustifyItems = a, j
	}
	return ok
}

// parsePlaceSelf parses place-self: align-self, then justify-self.
func parsePlaceSelf(ts []token, _ *ctx, v *Values) bool {
	a, j, ok := placeOf(components(ts),
		func(c [][]token) (BoxAlign, bool) { return alignOf(c, alignSelfKinds) },
		func(c [][]token) (BoxAlign, bool) { return alignOf(c, justifySelfKinds) })
	if ok {
		v.Layout.AlignSelf, v.Layout.JustifySelf = a, j
	}
	return ok
}
