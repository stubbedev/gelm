package style

import (
	"fmt"
	"testing"

	"github.com/stubbedev/gelm/render"
)

// capture swaps the warning sink for one test and returns the messages.
func capture(tb testing.TB) *[]string {
	tb.Helper()
	var warns []string
	prev := parseWarn
	parseWarn = func(msg string) { warns = append(warns, msg) }
	tb.Cleanup(func() { parseWarn = prev })
	return &warns
}

// node is a synthetic widget tree for matcher tests.
type node struct {
	target Target
	parent *node
}

func (n *node) StyleTarget(t *Target) { *t = n.target }
func (n *node) StyleParent() Node {
	if n.parent == nil {
		return nil
	}
	return n.parent
}

func (n *node) add(element string, classes ...string) *node {
	c := &node{target: Target{Element: element, Classes: classes}, parent: n}
	return c
}

// parseOne parses css and fails the test when any warning fired.
func parseOne(tb testing.TB, css string) *Sheet {
	tb.Helper()
	warns := capture(tb)
	s := Parse(css)
	if len(*warns) != 0 {
		tb.Fatalf("Parse(%q) warned: %v", css, *warns)
	}
	return s
}

// compute matches s against n and returns the values.
func compute(s *Sheet, n *node) Values {
	var v Values
	s.Match(n, &Scratch{}, &v)
	return v
}

func TestParseSelectorsAndValues(t *testing.T) {
	s := parseOne(t, `
		/* a comment */
		button {
			color: #f00;               /* short hex */
			background-color: #AABBCC; /* upper hex */
			padding: 12px;
			border-radius: 8;
			border-width: 2px;
			border-color: #11223344;
			min-width: 120px;
			min-height: 40;
		}
		label { font-size: 15.6px; font-weight: bold; font-family: "Cantarell", sans-serif; }
		box > label:hover, #sidebar .active:focus { box-shadow: #00000088 24px; }
		* { font-weight: normal; }
		toast { box-shadow: none; }
	`)
	if len(s.rules) != 6 {
		t.Fatalf("got %d rules, want 6 (the comma group is two)", len(s.rules))
	}
	n := &node{target: Target{Element: "button"}}
	v := compute(s, n)
	if !v.Has(PropColor) || v.Color != render.RGBA(0xff, 0, 0, 0xff) {
		t.Errorf("color = %v, want #f00", v.Color)
	}
	if !v.Has(PropBackgroundColor) || v.Background != render.RGB(0xaa, 0xbb, 0xcc) {
		t.Errorf("background = %v, want #AABBCC", v.Background)
	}
	if v.Padding != 12 || v.Radius != 8 || v.BorderWidth != 2 {
		t.Errorf("metrics = %d/%d/%d, want 12/8/2", v.Padding, v.Radius, v.BorderWidth)
	}
	if v.BorderColor != render.RGBA(0x11, 0x22, 0x33, 0x44) {
		t.Errorf("border color = %v, want #11223344", v.BorderColor)
	}
	if v.MinWidth != 120 || v.MinHeight != 40 {
		t.Errorf("min = %d/%d, want 120/40", v.MinWidth, v.MinHeight)
	}

	l := &node{target: Target{Element: "label"}}
	v = compute(s, l)
	if v.FontSize != 15.6 || v.FontWeight != 700 {
		t.Errorf("font = %v/%d, want 15.6/700", v.FontSize, v.FontWeight)
	}
	if v.FontFamily != "Cantarell" {
		t.Errorf("font family = %q, want the chain head", v.FontFamily)
	}

	// Combinators and states.
	box := &node{target: Target{Element: "box"}}
	lbl := box.add("label")
	lbl.target.State = Hover
	v = compute(s, lbl)
	if !v.Has(PropBoxShadow) || v.ShadowBlur != 24 {
		t.Errorf("box > label:hover shadow = %v blur %d, want set/24", v.ShadowColor, v.ShadowBlur)
	}

	side := &node{target: Target{Element: "box", ID: "sidebar"}}
	act := side.add("panel", "active")
	act.target.State = Focus
	v = compute(s, act)
	if !v.Has(PropBoxShadow) {
		t.Error("#sidebar .active:focus did not match")
	}
	if v2 := compute(s, side.add("panel", "active")); v2.Has(PropBoxShadow) {
		t.Error(":focus matched without the state")
	}

	univ := &node{target: Target{Element: "slider"}}
	if v := compute(s, univ); v.FontWeight != 400 {
		t.Errorf("* rule weight = %d, want 400", v.FontWeight)
	}
	if v := compute(s, &node{target: Target{Element: "toast"}}); v.Has(PropBoxShadow) {
		if v.ShadowBlur != 0 || v.ShadowColor != 0 {
			t.Errorf("box-shadow: none set values %v/%d", v.ShadowColor, v.ShadowBlur)
		}
	}
}

func TestCascadeSpecificityAndOrder(t *testing.T) {
	s := parseOne(t, `
		button { color: #010101; }
		.x { color: #020202; }
		button.x { color: #030303; }
		#i { color: #040404; }
		button.x.y { color: #050505; }
	`)
	n := &node{target: Target{Element: "button", Classes: []string{"x"}}}
	if got := compute(s, n).Color; got != render.RGB(3, 3, 3) {
		t.Errorf("button.x lost to lower specificity: %v", got)
	}
	n.target.ID = "i"
	if got := compute(s, n).Color; got != render.RGB(4, 4, 4) {
		t.Errorf("id lost to class+element: %v", got)
	}
	n.target.Classes = []string{"x", "y"}
	// An id outranks any number of classes, per CSS specificity.
	if got := compute(s, n).Color; got != render.RGB(4, 4, 4) {
		t.Errorf("two classes should lose to the id: %v", got)
	}

	// Source order breaks ties: the later declaration wins.
	s = parseOne(t, "button { color: #101010; } button { color: #202020; }")
	if got := compute(s, &node{target: Target{Element: "button"}}).Color; got != render.RGB(0x20, 0x20, 0x20) {
		t.Errorf("source order tie-break: %v, want the later rule", got)
	}

	// Within one rule, the later declaration of a property wins.
	s = parseOne(t, "button { color: #101010; color: #303030; }")
	if got := compute(s, &node{target: Target{Element: "button"}}).Color; got != render.RGB(0x30, 0x30, 0x30) {
		t.Errorf("in-rule tie-break: %v", got)
	}
}

func TestStateSpecificity(t *testing.T) {
	s := parseOne(t, `
		button { background-color: #000001; }
		button:hover { background-color: #000002; }
		button:focus { background-color: #000003; }
		button:hover:focus { background-color: #000004; }
	`)
	mk := func(state State) *node {
		return &node{target: Target{Element: "button", State: state}}
	}
	if got := compute(s, mk(0)).Background; got != render.RGB(0, 0, 1) {
		t.Errorf("idle bg = %v", got)
	}
	if got := compute(s, mk(Hover)).Background; got != render.RGB(0, 0, 2) {
		t.Errorf("hover bg = %v", got)
	}
	if got := compute(s, mk(Hover|Focus)).Background; got != render.RGB(0, 0, 4) {
		t.Errorf("hover:focus bg = %v, want the two-state rule", got)
	}
}

func TestBucketsIndexByRightmostSelector(t *testing.T) {
	s := parseOne(t, "box listrow { color: #111111; } .card { color: #222222; } #x { color: #333333; }")
	// A rule whose rightmost term is `listrow` must not be a candidate
	// for a bare listrow — wait, it must: the bucket is by rightmost.
	// A `box` widget alone must not match it.
	if v := compute(s, &node{target: Target{Element: "box"}}); v.Has(PropColor) {
		t.Error("ancestor-only term matched a bare box")
	}
	box := &node{target: Target{Element: "box"}}
	row := box.add("listrow")
	if got := compute(s, row).Color; got != render.RGB(0x11, 0x11, 0x11) {
		t.Errorf("descendant rule missed: %v", got)
	}
	if got := compute(s, &node{target: Target{Element: "panel", Classes: []string{"card"}}}).Color; got != render.RGB(0x22, 0x22, 0x22) {
		t.Errorf("class rule missed: %v", got)
	}
	if got := compute(s, &node{target: Target{Element: "label", ID: "x"}}).Color; got != render.RGB(0x33, 0x33, 0x33) {
		t.Errorf("id rule missed: %v", got)
	}
}

func TestInheritanceSet(t *testing.T) {
	parent := Values{}
	parent.Set = 1<<PropColor | 1<<PropBackgroundColor | 1<<PropFontSize | 1<<PropFontWeight
	parent.Color = render.RGB(1, 2, 3)
	parent.Background = render.RGB(9, 9, 9)
	parent.FontSize = 16
	parent.FontWeight = 700

	var v Values
	v.Padding = 4 // set but non-inherited
	v.Set = 1 << PropPadding
	v.InheritFrom(&parent)

	if !v.Has(PropColor) || v.Color != parent.Color {
		t.Errorf("color did not inherit: %v", v.Color)
	}
	if !v.Has(PropFontSize) || v.FontSize != 16 || !v.Has(PropFontWeight) {
		t.Errorf("font-* did not inherit: %v/%v", v.FontSize, v.FontWeight)
	}
	if v.Has(PropBackgroundColor) || v.Background != 0 {
		t.Error("background-color must not inherit")
	}
	if v.Padding != 4 {
		t.Error("padding was clobbered by inheritance")
	}

	// A local declaration beats the inherited value.
	v = Values{Set: 1 << PropColor, Color: render.RGB(7, 7, 7)}
	v.InheritFrom(&parent)
	if v.Color != render.RGB(7, 7, 7) {
		t.Errorf("inheritance overrode a direct declaration: %v", v.Color)
	}
}

func TestWarnAndSkipPolicy(t *testing.T) {
	warns := capture(t)
	s := Parse(`
		button { color: #ff0000; bogus-prop: 12px; background-color: #00ff00; }
		button:visited { color: #123123; }
		label::after { color: #456456; }
		button { color: #0000ff !important; }
		entry { padding: 4px; color: #abcdef; }
		@media screen { button { color: #222222; } }
		button[flavored] { color: #333333; }
		button { background-color: #445566; ` + // unterminated block at EOF
		``)
	if len(*warns) < 5 {
		t.Errorf("got %d warnings, want at least 5 (unknown property, bad pseudo, ::, !important, at-rule, attribute, unterminated)", len(*warns))
	}

	// The valid declarations around the bad one still apply.
	v := compute(s, &node{target: Target{Element: "button"}})
	if v.Color != render.RGB(0xff, 0, 0) {
		t.Errorf("color = %v, want the declaration before the unknown property", v.Color)
	}
	if v.Background != render.RGB(0, 0xff, 0) {
		t.Errorf("background = %v, want the declaration after it", v.Background)
	}
	if v := compute(s, &node{target: Target{Element: "entry"}}).Color; v != render.RGB(0xab, 0xcd, 0xef) {
		t.Errorf("entry color = %v, want the rule after the skipped ones", v)
	}

	// Unknown elements and ids never match, and never warn.
	s = parseOne(t, "nosuchwidget { color: #010101; } #nosuchid { color: #020202; }")
	if v := compute(s, &node{target: Target{Element: "button"}}); v.Has(PropColor) {
		t.Error("unknown element name matched")
	}
}

func TestParseNoMatchesWithoutStylesheet(t *testing.T) {
	s := Parse("")
	if s == nil || len(s.rules) != 0 {
		t.Fatal("empty stylesheet must parse to an empty sheet")
	}
	var v Values
	s.Match(&node{target: Target{Element: "button"}}, &Scratch{}, &v)
	if v.Set != 0 {
		t.Error("empty sheet set properties")
	}
}

func TestValuesNoAllocationPerMatch(t *testing.T) {
	s := parseOne(t, `
		button { color: #111111; background-color: #222222; padding: 8; }
		button:hover { background-color: #333333; }
		box > button { border-radius: 4; }
		* { font-weight: 400; }
	`)
	root := &node{target: Target{Element: "box"}}
	b := root.add("button")
	sc := &Scratch{}
	var v Values
	alloc := testing.AllocsPerRun(100, func() {
		s.Match(b, sc, &v)
	})
	if alloc != 0 {
		t.Errorf("Match allocated %v ops, want 0", alloc)
	}
}

func ExampleParse() {
	s := Parse(`button.destructive { background-color: #a00; }`)
	var v Values
	s.Match(&node{target: Target{
		Element: "button", Classes: []string{"destructive"},
	}}, &Scratch{}, &v)
	fmt.Println(v.Has(PropBackgroundColor), v.Background == render.RGBA(0xaa, 0, 0, 0xff))
	// Output: true true
}
