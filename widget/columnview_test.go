package widget

import (
	"strconv"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/gelm/render"
)

// viewFace builds the test font for the table widgets.
func viewFace(t *testing.T) render.Font {
	t.Helper()
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// tableRow is the demo/test row shape.
type tableRow struct {
	Name  string
	Count int
}

func tableCols(face render.Font) []TableColumn[tableRow] {
	th := DarkTheme()
	return []TableColumn[tableRow]{
		{
			Title: "Name", Width: 90, Expand: true,
			Cell: func(r tableRow) Widget { return NewLabel(face, 14, r.Name, th.Text) },
			Sort: func(a, b tableRow) int { return strings.Compare(a.Name, b.Name) },
		},
		{
			Title: "Count", Width: 60,
			Cell: func(r tableRow) Widget { return NewLabel(face, 14, strconv.Itoa(r.Count), th.TextMuted) },
			Sort: func(a, b tableRow) int { return a.Count - b.Count },
		},
	}
}

// TestColumnViewSortAndFilter pins the model layer: the permutation is
// stable (equal rows keep source order), sorting is descending on the
// second cycle, -1 returns to source order, and the filter narrows
// without touching the source indices' data.
func TestColumnViewSortAndFilter(t *testing.T) {
	face := viewFace(t)
	data := SliceModel[tableRow]{{"bravo", 2}, {"alpha", 1}, {"alpine", 1}, {"charlie", 3}}
	v := NewColumnView(face, 14, data, tableCols(face))

	if v.Rows() != 4 {
		t.Fatalf("rows = %d", v.Rows())
	}
	v.SortBy(1, false) // count ascending, stable within equal counts
	if got := v.Row(0).Name; got != "alpha" {
		t.Errorf("first ascending row = %q, want alpha (source order among equals)", got)
	}
	if got := v.Row(1).Name; got != "alpine" {
		t.Errorf("second ascending row = %q, want alpine (stability)", got)
	}
	v.SortBy(1, true) // descending
	if got := v.Row(0).Name; got != "charlie" {
		t.Errorf("first descending row = %q", got)
	}
	v.SetFilter(func(r tableRow) bool { return r.Count == 1 })
	if v.Rows() != 2 {
		t.Fatalf("filtered rows = %d, want 2", v.Rows())
	}
	if got := v.Row(0).Name; got != "alpine" && got != "alpha" {
		t.Errorf("filtered row = %q", got)
	}
	v.SetFilter(nil)
	v.SortBy(-1, false)
	if got := v.Row(0).Name; got != "bravo" {
		t.Errorf("source order not restored: %q", got)
	}
}

// TestColumnViewCycleSort pins the header walk: a sortable column
// cycles ascending, descending, off; a sortless column clears.
func TestColumnViewCycleSort(t *testing.T) {
	face := viewFace(t)
	v := NewColumnView(face, 14, SliceModel[tableRow]{{"b", 2}, {"a", 1}}, tableCols(face))

	v.cycleSort(0)
	if col, up := v.SortedBy(); col != 0 || !up {
		t.Errorf("first cycle = %d %v, want 0 ascending", col, up)
	}
	v.cycleSort(0)
	if col, up := v.SortedBy(); col != 0 || up {
		t.Errorf("second cycle = %d %v, want 0 descending", col, up)
	}
	v.cycleSort(0)
	if col, _ := v.SortedBy(); col != -1 {
		t.Errorf("third cycle = %d, want off", col)
	}
	v.SortBy(0, false)
	v.cycleSort(5) // a column that does not exist clears
	if col, _ := v.SortedBy(); col != -1 {
		t.Errorf("unknown column cycle = %d, want off", col)
	}
}

// TestColumnViewVirtualizationBounds pins the virtualization claim at
// the model seam: a hundred-thousand-row table builds only the rows
// the List asks for - the adapter never prebuilds.
func TestColumnViewVirtualizationBounds(t *testing.T) {
	face := viewFace(t)
	big := make(SliceModel[tableRow], 100_000)
	for i := range big {
		big[i] = tableRow{Name: "row" + strconv.Itoa(i), Count: i}
	}
	v := NewColumnView(face, 14, big, tableCols(face))
	if v.Rows() != 100_000 {
		t.Fatalf("rows = %d", v.Rows())
	}
	built := 0
	for i := range 25 { // a viewport's worth
		if v.rows.Row(i) != nil {
			built++
		}
	}
	if built != 25 || len(v.rows.built) > 25 {
		t.Errorf("adapter built %d widgets for 25 asked (cache %d)", built, len(v.rows.built))
	}
}

// TestRowPermutation pins the shared sort/filter layer directly:
// stability, filtering, and the identity order.
func TestRowPermutation(t *testing.T) {
	var p rowPermutation
	p.rebuild(4, nil, nil)
	if len(p.order) != 4 || p.order[2] != 2 {
		t.Fatalf("identity order = %v", p.order)
	}
	p.rebuild(4, func(i, j int) int { return j - i }, nil) // reverse by index
	if p.order[0] != 3 || p.order[3] != 0 {
		t.Fatalf("reversed order = %v", p.order)
	}
	p.rebuild(6, nil, func(i int) bool { return i%2 == 0 })
	if len(p.order) != 3 || p.order[2] != 4 {
		t.Fatalf("filtered order = %v", p.order)
	}
}

// TestFlatTree pins the tree flattening: pre-order visible rows with
// depths, the all-expanded default, collapse hiding descendants,
// re-expand restoring them, and leaves refusing to toggle.
func TestFlatTree(t *testing.T) {
	roots := []TreeRow[string]{
		{Value: "a", Children: []TreeRow[string]{
			{Value: "a1"},
			{Value: "a2", Children: []TreeRow[string]{{Value: "a2x"}}},
		}},
		{Value: "b"},
	}
	f := NewFlatTree(roots)
	if f.Len() != 5 {
		t.Fatalf("expanded rows = %d, want 5", f.Len())
	}
	if f.Row(1).Value != "a1" || f.Row(1).Depth != 1 || f.Row(2).Value != "a2" || f.Row(3).Depth != 2 {
		t.Fatalf("pre-order flattening wrong: %+v %+v", f.Row(1), f.Row(2))
	}

	f.Toggle(2) // collapse a2
	if f.Len() != 4 {
		t.Fatalf("after collapse rows = %d, want 4", f.Len())
	}
	if f.Row(3).Value != "b" {
		t.Errorf("collapse did not hide descendants: %+v", f.Row(3))
	}
	f.Toggle(2)
	if f.Len() != 5 || f.Row(3).Value != "a2x" {
		t.Errorf("re-expand lost the subtree: %d rows, row3 = %+v", f.Len(), f.Row(3))
	}
	f.Toggle(3) // a leaf
	if f.Len() != 5 {
		t.Error("a leaf toggle changed the tree")
	}
}

// TestFlatTreeNotify pins the refresh hook: the expander's toggle runs
// the installed refresh, and the rows re-flattened underneath it.
func TestFlatTreeNotify(t *testing.T) {
	f := NewFlatTree([]TreeRow[int]{{Value: 1, Children: []TreeRow[int]{{Value: 2}}}})
	fired := 0
	f.SetRefresh(func() {
		fired++
		if f.Len() != 1 {
			t.Errorf("refresh saw %d rows", f.Len())
		}
	})
	f.Toggle(0)
	f.Notify()
	if fired != 1 {
		t.Errorf("refresh fired %d times", fired)
	}
}

// TestGoldenColumnView pins the table's painted look: header with the
// sort arrow, aligned columns, and the tree column's indent guides.
func TestGoldenColumnView(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	data := SliceModel[tableRow]{{"bravo", 2}, {"alpha", 1}}
	v := NewColumnView(face, 14, data, tableCols(face))
	v.SortBy(0, false)
	NewGolden(t, v, "columnview-sorted", goldenTheme(th), goldenFrame(220, 120))

	tree := NewFlatTree([]TreeRow[string]{
		{Value: "home", Children: []TreeRow[string]{{Value: "docs"}, {Value: "pics"}}},
		{Value: "etc"},
	})
	tv := NewColumnView(face, 14, tree, []TableColumn[FlatRow[string]]{{
		Title:  "Name",
		Expand: true,
		Cell: TreeCell(tree, 14, func(s string) Widget {
			return NewLabel(face, 14, s, th.Text)
		}),
	}})
	NewGolden(t, tv, "columnview-tree", goldenTheme(th), goldenFrame(220, 150))
}

// TestColumnViewHeaderAlignsWithRows pins the column contract: every
// header cell and the matching cell of a row share an x and a width,
// fixed and expanding columns alike - the sized cell both use.
func TestColumnViewHeaderAlignsWithRows(t *testing.T) {
	face := viewFace(t)
	v := NewColumnView(face, 14, SliceModel[tableRow]{{"alpha", 1}, {"bravo", 2}}, tableCols(face))
	v.Measure(Constraints{Max: Size{W: 300, H: 200}})
	v.Arrange(render.Rect{W: 300, H: 200})
	// List arranges its visible rows as it paints them.
	v.Paint(render.New(make([]byte, render.Stride(300)*200), render.Stride(300), 300, 200))
	row, ok := v.rows.Row(0).(*Box)
	if !ok {
		t.Fatalf("row 0 is %T", v.rows.Row(0))
	}
	heads, cells := v.header.Children(), row.Children()
	if len(heads) != len(cells) {
		t.Fatalf("%d header cells, %d row cells", len(heads), len(cells))
	}
	for i := range heads {
		h, c := heads[i].(Boundser).Bounds(), cells[i].(Boundser).Bounds()
		if h.X != c.X || h.W != c.W {
			t.Errorf("column %d: header %+v, row %+v", i, h, c)
		}
	}
}
