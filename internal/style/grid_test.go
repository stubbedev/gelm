package style

import (
	"reflect"
	"testing"
)

// The breadth and size builders the grid tables read.
func pxB(n int) TrackBreadth      { return TrackBreadth{Kind: TrackLength, Len: Length{Px: n}} }
func pctB(p float64) TrackBreadth { return TrackBreadth{Kind: TrackLength, Len: Length{Pct: p}} }
func frB(f float64) TrackBreadth  { return TrackBreadth{Kind: TrackFr, Fr: f} }
func fixedT(n int) TrackSize      { return TrackSize{Min: pxB(n), Max: pxB(n)} }
func frT(f float64) TrackSize     { return TrackSize{Max: frB(f)} }
func run(names [][]string, sizes ...TrackSize) TrackRun {
	return TrackRun{Sizes: sizes, Names: names}
}

var (
	minC = TrackBreadth{Kind: TrackMinContent}
	maxC = TrackBreadth{Kind: TrackMaxContent}
)

// gridOf computes one declaration on a lone element.
func gridOf(t *testing.T, decl string) Values {
	t.Helper()
	return computeTree(parseOne(t, "e { "+decl+"; }"), el("e"))
}

// TestGridTemplateCompute pins grid-template-columns and -rows: every
// track breadth, minmax() and fit-content(), named lines, integer
// repeat() expanding with its boundary names merged, auto-fill and
// auto-fit around fixed tracks, subgrid, and none.
func TestGridTemplateCompute(t *testing.T) {
	cases := []struct {
		decl string
		want *TrackList
	}{
		{"grid-template-columns: none", nil},
		{"grid-template-columns: 100px 1fr auto", &TrackList{Head: run([][]string{nil, nil, nil, nil},
			fixedT(100), frT(1), autoTrack)}},
		{"grid-template-columns: 25% min-content max-content 0fr", &TrackList{Head: run([][]string{nil, nil, nil, nil, nil},
			TrackSize{Min: pctB(25), Max: pctB(25)}, TrackSize{Min: minC, Max: minC}, TrackSize{Min: maxC, Max: maxC}, frT(0))}},
		{"grid-template-columns: minmax(100px, 2fr) minmax(min-content, 50%)", &TrackList{Head: run([][]string{nil, nil, nil},
			TrackSize{Min: pxB(100), Max: frB(2)}, TrackSize{Min: minC, Max: pctB(50)})}},
		{"grid-template-columns: fit-content(120px) calc(10% + 5px)", &TrackList{Head: run([][]string{nil, nil, nil},
			TrackSize{Max: TrackBreadth{Kind: TrackFitContent, Len: Length{Px: 120}}},
			TrackSize{Min: TrackBreadth{Kind: TrackLength, Len: Length{Px: 5, Pct: 10}}, Max: TrackBreadth{Kind: TrackLength, Len: Length{Px: 5, Pct: 10}}})}},
		{"grid-template-rows: [top a] 10px [mid] 20px [bottom]", &TrackList{Head: run([][]string{{"top", "a"}, {"mid"}, {"bottom"}},
			fixedT(10), fixedT(20))}},
		{"grid-template-columns: [a] repeat(2, [b] 10px [c]) [d]", &TrackList{Head: run([][]string{{"a", "b"}, {"c", "b"}, {"c", "d"}},
			fixedT(10), fixedT(10))}},
		{"grid-template-columns: repeat(3, 1fr)", &TrackList{Head: run([][]string{nil, nil, nil, nil}, frT(1), frT(1), frT(1))}},
		{"grid-template-columns: [s] 50px repeat(auto-fill, [c] minmax(100px, 1fr)) [e] 20px", &TrackList{
			Head:   run([][]string{{"s"}, nil}, fixedT(50)),
			Repeat: RepeatFill,
			Auto:   run([][]string{{"c"}, nil}, TrackSize{Min: pxB(100), Max: frB(1)}),
			Tail:   run([][]string{{"e"}, nil}, fixedT(20)),
		}},
		{"grid-template-columns: repeat(auto-fit, 40px 60px)", &TrackList{
			Head: run([][]string{nil}), Repeat: RepeatFit,
			Auto: run([][]string{nil, nil, nil}, fixedT(40), fixedT(60)), Tail: run([][]string{nil}),
		}},
		{"grid-template-rows: subgrid", &TrackList{Subgrid: true}},
		{"grid-template-rows: subgrid [a] repeat(2, [b] [c]) repeat(auto-fill, [d])", &TrackList{Subgrid: true}},
	}
	for _, c := range cases {
		v := gridOf(t, c.decl)
		got := v.Layout.GridTemplateColumns
		if !v.Has(PropGridTemplateColumns) {
			got = v.Layout.GridTemplateRows
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n computed %#v\n want     %#v", c.decl, got, c.want)
		}
	}
}

// TestGridTrackListInterned pins the comparability of Layout: equal
// track lists, however spelled, compute to one pointer, so a restyle's
// == diff sees no change.
func TestGridTrackListInterned(t *testing.T) {
	a := gridOf(t, "grid-template-columns: 1fr 1fr 1fr")
	b := gridOf(t, "grid-template-columns: repeat(3, 1fr)")
	if a.Layout.GridTemplateColumns != b.Layout.GridTemplateColumns || a.Layout != b.Layout {
		t.Error("equal track lists computed to different pointers")
	}
	c := gridOf(t, "grid-template-columns: 1fr 2fr")
	if c.Layout.GridTemplateColumns == a.Layout.GridTemplateColumns {
		t.Error("different track lists share a pointer")
	}
	x, y := gridOf(t, `grid-template-areas: "a b"`), gridOf(t, `grid-template-areas: "a  b"`)
	if x.Layout.GridTemplateAreas != y.Layout.GridTemplateAreas {
		t.Error("equal area maps computed to different pointers")
	}
}

// TestGridTrackListLines pins the expanded line view of a track list
// with an auto repetition: the track count and sizes per repetition
// count, which tracks the repetition made, and the names on each line,
// merged where two runs meet.
func TestGridTrackListLines(t *testing.T) {
	v := gridOf(t, "grid-template-columns: [s] 50px [h] repeat(auto-fill, [c] 100px [d]) [e] 20px [z]")
	l := v.Layout.GridTemplateColumns
	if got := l.Count(3); got != 5 {
		t.Fatalf("Count(3) = %d, want 5", got)
	}
	for i, want := range []int{50, 100, 100, 100, 20} {
		if got := l.Size(i, 3).Max.Len.Px; got != want {
			t.Errorf("Size(%d) = %d, want %d", i, got, want)
		}
		if got, want := l.Repeated(i, 3), i >= 1 && i <= 3; got != want {
			t.Errorf("Repeated(%d) = %v, want %v", i, got, want)
		}
	}
	names := [][]string{{"s"}, {"h", "c"}, {"d", "c"}, {"d", "c"}, {"d", "e"}, {"z"}}
	for line, want := range names {
		for _, n := range []string{"s", "h", "c", "d", "e", "z"} {
			has := false
			for _, w := range want {
				has = has || w == n
			}
			if got := l.LineNamed(line, 3, n, ""); got != has {
				t.Errorf("line %d named %q = %v, want %v", line, n, got, has)
			}
		}
	}
	if !l.LineNamed(1, 0, "h", "") || !l.LineNamed(1, 0, "e", "") || l.LineNamed(1, 0, "c", "") {
		t.Error("with no repetitions, the head's and tail's boundary names should meet on line 1")
	}
}

// TestGridAreasCompute pins grid-template-areas: rows of cells, null
// cells of one or more periods, and each name's rectangle.
func TestGridAreasCompute(t *testing.T) {
	v := gridOf(t, `grid-template-areas: "head head" "nav main" ". main" "foot ..."`)
	want := &GridAreas{Rows: 4, Columns: 2, Areas: []GridArea{
		{Name: "head", Column: LineSpan{0, 2}, Row: LineSpan{0, 1}},
		{Name: "nav", Column: LineSpan{0, 1}, Row: LineSpan{1, 2}},
		{Name: "main", Column: LineSpan{1, 2}, Row: LineSpan{1, 3}},
		{Name: "foot", Column: LineSpan{0, 1}, Row: LineSpan{3, 4}},
	}}
	if got := v.Layout.GridTemplateAreas; !reflect.DeepEqual(got, want) {
		t.Errorf("areas %#v, want %#v", got, want)
	}
	for _, c := range []struct {
		columns   bool
		line      int
		name, suf string
		want      bool
	}{
		{false, 1, "main", "-start", true},
		{false, 3, "main", "-end", true},
		{false, 3, "main-e", "nd", true},
		{false, 3, "", "main-end", true},
		{false, 2, "main", "-end", false},
		{true, 1, "main", "-start", true},
		{true, 2, "head", "-end", true},
		{true, 0, "main", "-start", false},
		{false, 0, "head", "", false},
	} {
		if got := want.LineNamed(c.columns, c.line, c.name, c.suf); got != c.want {
			t.Errorf("LineNamed(%v, %d, %q+%q) = %v, want %v", c.columns, c.line, c.name, c.suf, got, c.want)
		}
	}
	if v := gridOf(t, "grid-template-areas: none"); !v.Has(PropGridTemplateAreas) || v.Layout.GridTemplateAreas != nil {
		t.Error("grid-template-areas: none should declare a nil map")
	}
}

// TestGridPlacementCompute pins the placement longhands and the
// grid-row, grid-column and grid-area shorthands, with their omitted
// lines copying a lone identifier.
func TestGridPlacementCompute(t *testing.T) {
	num := func(n int, name string) GridLine { return GridLine{Kind: GridLineNumber, N: n, Name: name} }
	span := func(n int, name string) GridLine { return GridLine{Kind: GridLineSpan, N: n, Name: name} }
	ident := func(name string) GridLine { return GridLine{Kind: GridLineIdent, Name: name} }
	type lines = [4]GridLine // column start, column end, row start, row end
	cases := []struct {
		decl string
		want lines
	}{
		{"grid-column-start: 2", lines{num(2, "")}},
		{"grid-column-end: -1", lines{1: num(-1, "")}},
		{"grid-row-start: span 3", lines{2: span(3, "")}},
		{"grid-row-end: 2 foo", lines{3: num(2, "foo")}},
		{"grid-row-end: foo -2", lines{3: num(-2, "foo")}},
		{"grid-column-start: span foo", lines{span(1, "foo")}},
		{"grid-column-start: foo span 2", lines{span(2, "foo")}},
		{"grid-column-start: 2 foo span", lines{span(2, "foo")}},
		{"grid-column-start: 2 span foo", lines{span(2, "foo")}},
		{"grid-column-start: Main", lines{ident("Main")}},
		{"grid-column-start: auto", lines{}},
		{"grid-column-start: 99999", lines{num(GridLineLimit, "")}},
		{"grid-column: 1 / 3", lines{num(1, ""), num(3, "")}},
		{"grid-column: 2", lines{num(2, "")}},
		{"grid-column: main", lines{ident("main"), ident("main")}},
		{"grid-column: 1 / span 2", lines{num(1, ""), span(2, "")}},
		{"grid-row: a / b", lines{2: ident("a"), 3: ident("b")}},
		{"grid-area: main", lines{ident("main"), ident("main"), ident("main"), ident("main")}},
		{"grid-area: 1 / 2", lines{num(2, ""), {}, num(1, ""), {}}},
		{"grid-area: a / b", lines{ident("b"), ident("b"), ident("a"), ident("a")}},
		{"grid-area: 1 / 2 / 3", lines{num(2, ""), {}, num(1, ""), num(3, "")}},
		{"grid-area: 1 / 2 / 3 / span 4", lines{num(2, ""), span(4, ""), num(1, ""), num(3, "")}},
	}
	for _, c := range cases {
		l := gridOf(t, c.decl).Layout
		if got := (lines{l.GridColumnStart, l.GridColumnEnd, l.GridRowStart, l.GridRowEnd}); got != c.want {
			t.Errorf("%s: computed %+v, want %+v", c.decl, got, c.want)
		}
	}
}

// TestGridContainerCompute pins grid-auto-flow, the implicit track
// sizes, and the grid-template and grid shorthands' expansions.
func TestGridContainerCompute(t *testing.T) {
	flow := func(decl string) GridAutoFlow { return gridOf(t, decl).Layout.GridAutoFlow }
	for decl, want := range map[string]GridAutoFlow{
		"grid-auto-flow: row": {}, "grid-auto-flow: column": {Column: true},
		"grid-auto-flow: dense": {Dense: true}, "grid-auto-flow: dense column": {Column: true, Dense: true},
		"grid-auto-flow: row dense": {Dense: true},
	} {
		if got := flow(decl); got != want {
			t.Errorf("%s: %+v, want %+v", decl, got, want)
		}
	}
	auto := gridOf(t, "grid-auto-rows: 10px minmax(20px, auto)").Layout.GridAutoRows
	if want := []TrackSize{fixedT(10), {Min: pxB(20)}}; auto == nil || !reflect.DeepEqual(auto.Sizes, want) {
		t.Errorf("grid-auto-rows %#v, want %#v", auto, want)
	}
	for k, want := range map[int]int{0: 10, 1: 20, 2: 10, -1: 20, -2: 10, -3: 20} {
		if got := auto.At(k).Min.Len.Px; got != want {
			t.Errorf("At(%d) = %d, want %d", k, got, want)
		}
	}
	if (*AutoTracks)(nil).At(3) != autoTrack {
		t.Error("a nil auto list should size implicit tracks auto")
	}

	tpl := gridOf(t, `grid-template: [top] "a a" 40px [mid] [head] "b c" [bottom] / [l] 1fr [m] 2fr [r]`).Layout
	wantRows := &TrackList{Head: run([][]string{{"top"}, {"mid", "head"}, {"bottom"}}, fixedT(40), autoTrack)}
	wantCols := &TrackList{Head: run([][]string{{"l"}, {"m"}, {"r"}}, frT(1), frT(2))}
	if !reflect.DeepEqual(tpl.GridTemplateRows, wantRows) || !reflect.DeepEqual(tpl.GridTemplateColumns, wantCols) {
		t.Errorf("grid-template areas form: rows %#v cols %#v", tpl.GridTemplateRows, tpl.GridTemplateColumns)
	}
	if tpl.GridTemplateAreas == nil || tpl.GridTemplateAreas.Rows != 2 || len(tpl.GridTemplateAreas.Areas) != 3 {
		t.Errorf("grid-template areas %#v", tpl.GridTemplateAreas)
	}
	rc := gridOf(t, "grid-template: 10px / 20px 30px").Layout
	if rc.GridTemplateRows.Count(0) != 1 || rc.GridTemplateColumns.Count(0) != 2 || rc.GridTemplateAreas != nil {
		t.Errorf("grid-template rows/columns: %+v", rc)
	}
	none := gridOf(t, "grid-template: none")
	if !none.Has(PropGridTemplateAreas) || none.Layout.GridTemplateRows != nil {
		t.Error("grid-template: none should declare all three longhands none")
	}

	g := gridOf(t, "grid: auto-flow dense 30px / 1fr 1fr").Layout
	if g.GridAutoFlow != (GridAutoFlow{Dense: true}) || g.GridAutoRows.Sizes[0] != fixedT(30) || g.GridTemplateColumns.Count(0) != 2 || g.GridTemplateRows != nil {
		t.Errorf("grid auto-flow rows form: %+v", g)
	}
	g = gridOf(t, "grid: 50px / auto-flow 20px").Layout
	if g.GridAutoFlow != (GridAutoFlow{Column: true}) || g.GridAutoColumns.Sizes[0] != fixedT(20) || g.GridTemplateRows.Count(0) != 1 {
		t.Errorf("grid auto-flow columns form: %+v", g)
	}
	g = gridOf(t, "grid: 50px / dense auto-flow").Layout
	if g.GridAutoFlow != (GridAutoFlow{Column: true, Dense: true}) || g.GridAutoColumns != nil {
		t.Errorf("grid auto-flow without sizes: %+v", g)
	}
	full := gridOf(t, `grid: "a" 10px / 1fr`)
	if !full.Has(PropGridAutoFlow) || !full.Has(PropGridAutoRows) || full.Layout.GridTemplateAreas == nil {
		t.Error("grid should declare every container longhand, the template form included")
	}
}

// TestGridAlignCompute pins justify-items, justify-self and the
// place-items and place-self shorthands.
func TestGridAlignCompute(t *testing.T) {
	a := func(k BoxAlignKind) BoxAlign { return BoxAlign{Kind: k} }
	cases := []struct {
		decl string
		get  func(l Layout) BoxAlign
		want BoxAlign
	}{
		{"justify-items: left", func(l Layout) BoxAlign { return l.JustifyItems }, a(BoxLeft)},
		{"justify-items: safe right", func(l Layout) BoxAlign { return l.JustifyItems }, BoxAlign{Kind: BoxRight, Safe: true}},
		{"justify-items: legacy", func(l Layout) BoxAlign { return l.JustifyItems }, a(BoxNormal)},
		{"justify-items: legacy center", func(l Layout) BoxAlign { return l.JustifyItems }, a(BoxCenter)},
		{"justify-items: right legacy", func(l Layout) BoxAlign { return l.JustifyItems }, a(BoxRight)},
		{"justify-items: last baseline", func(l Layout) BoxAlign { return l.JustifyItems }, a(BoxLastBaseline)},
		{"justify-self: auto", func(l Layout) BoxAlign { return l.JustifySelf }, a(BoxAuto)},
		{"justify-self: self-end", func(l Layout) BoxAlign { return l.JustifySelf }, a(BoxSelfEnd)},
		{"justify-self: stretch", func(l Layout) BoxAlign { return l.JustifySelf }, a(BoxStretch)},
		{"place-items: center", func(l Layout) BoxAlign { return l.AlignItems }, a(BoxCenter)},
		{"place-items: center", func(l Layout) BoxAlign { return l.JustifyItems }, a(BoxCenter)},
		{"place-items: end left", func(l Layout) BoxAlign { return l.JustifyItems }, a(BoxLeft)},
		{"place-items: first baseline safe end", func(l Layout) BoxAlign { return l.AlignItems }, a(BoxBaseline)},
		{"place-items: first baseline safe end", func(l Layout) BoxAlign { return l.JustifyItems }, BoxAlign{Kind: BoxEnd, Safe: true}},
		{"place-self: auto right", func(l Layout) BoxAlign { return l.JustifySelf }, a(BoxRight)},
		{"place-self: start", func(l Layout) BoxAlign { return l.JustifySelf }, a(BoxStart)},
		{"place-self: start", func(l Layout) BoxAlign { return l.AlignSelf }, a(BoxStart)},
	}
	for _, c := range cases {
		if got := c.get(gridOf(t, c.decl).Layout); got != c.want {
			t.Errorf("%s: computed %+v, want %+v", c.decl, got, c.want)
		}
	}
}

// TestGridPropsInvalid pins the grid values CSS rejects: each leaves
// its longhands undeclared and warns.
func TestGridPropsInvalid(t *testing.T) {
	for _, decl := range []string{
		"grid-template-columns: -10px", "grid-template-columns: -1fr", "grid-template-columns: red",
		"grid-template-columns: minmax(1fr, 100px)", "grid-template-columns: minmax(10px)",
		"grid-template-columns: fit-content(-1px)", "grid-template-columns: [a] [b] 10px",
		"grid-template-columns: [span] 10px", "grid-template-columns: [auto] 10px", "grid-template-columns: [1] 10px",
		"grid-template-columns: repeat(0, 10px)", "grid-template-columns: repeat(2)", "grid-template-columns: repeat(2, [a])",
		"grid-template-columns: repeat(auto-fill, 1fr)", "grid-template-columns: repeat(auto-fill, auto)",
		"grid-template-columns: repeat(auto-fill, 10px) repeat(auto-fit, 10px)",
		"grid-template-columns: 1fr repeat(auto-fill, 10px)", "grid-template-columns: repeat(2, repeat(2, 10px))",
		"grid-template-columns: none 10px", "grid-template-columns: subgrid 10px", "grid-template-rows: subgrid [a",
		"grid-template-rows: subgrid repeat(auto-fill, [a]) repeat(auto-fill, [b])",
		`grid-template-areas: "a b" "c"`, `grid-template-areas: "a b" "b a"`, `grid-template-areas: "a #"`,
		`grid-template-areas: "a . a"`, `grid-template-areas: ""`, "grid-template-areas: a",
		"grid-template: 10px", `grid-template: "a" [x] 10px`, `grid-template: "a" / repeat(2, 1fr)`,
		`grid-template: [a] [b] "a"`, `grid-template: "a" 10px 20px`, "grid-template: 1px / 2px / 3px",
		"grid-auto-rows: [a] 10px", "grid-auto-columns: repeat(2, 10px)", "grid-auto-rows: none",
		"grid-auto-flow: row column", "grid-auto-flow: dense dense", "grid-auto-flow: up",
		"grid: auto-flow / auto-flow", "grid: 10px / auto-flow 1fr 1fr / 2px",
		"grid-column-start: 0", "grid-column-start: span", "grid-column-start: span 0", "grid-column-start: span -2",
		"grid-column-start: 1.5", "grid-column-start: 2 span 3", "grid-column-start: span 2 foo bar",
		"grid-row: 1 / 2 / 3", "grid-area: 1 / 2 / 3 / 4 / 5",
		"grid-column-start: inherit 2", "grid-row-end: auto 2",
		"justify-items: legacy legacy", "justify-items: legacy start", "justify-items: space-between",
		"justify-self: space-around", "place-items: auto", "place-items: center center center",
		"place-self: left", "place-items: stretch space-evenly",
	} {
		warns := capture(t)
		v := computeTree(Parse("e { "+decl+"; }"), el("e"))
		if !v.Set.And(LayoutProps).Empty() {
			t.Errorf("%s: declared %+v, want the invalid value dropped", decl, v.Layout)
		}
		if len(*warns) == 0 {
			t.Errorf("%s: no warning", decl)
		}
	}
}

// TestGridSupports pins @supports for the grid values.
func TestGridSupports(t *testing.T) {
	s := parseOne(t, `@supports (grid-template-columns: repeat(auto-fill, minmax(100px, 1fr))) and (place-items: center) {
		e { order: 3; } }`)
	if v := computeTree(s, el("e")); v.Layout.Order != 3 {
		t.Error("@supports for grid values did not apply")
	}
}
