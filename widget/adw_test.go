package widget

import (
	"image"
	"image/color"
	"testing"

	"github.com/stubbedev/gelm/anim"
	"github.com/stubbedev/gelm/render"
)

// The swipeable widgets settle on the router's gesture interfaces; a
// method the router never calls (a ReleaseAt, say) would compile and
// silently never fire, so the contract is pinned at compile time.
var (
	_ PressAter  = (*Carousel)(nil)
	_ DragMover  = (*Carousel)(nil)
	_ PressEnder = (*Carousel)(nil)
	_ PressAter  = (*BottomSheet)(nil)
	_ DragMover  = (*BottomSheet)(nil)
	_ PressEnder = (*BottomSheet)(nil)
	_ Clicker    = (*BottomSheet)(nil)
)

// TestToggleGroup pins the segmented control: one active toggle, the
// mark following it, clicks and Left/Right moving it (clamped), and
// OnChanged firing only when it moved.
func TestToggleGroup(t *testing.T) {
	face := adaptiveFace(t)
	th := DarkTheme()
	g := NewToggleGroup(NewLabel(face, 14, "a", th.Text), NewLabel(face, 14, "b", th.Text), NewLabel(face, 14, "c", th.Text))
	var changes []int
	g.OnChanged = func(i int) { changes = append(changes, i) }
	if g.Active() != 0 || !g.buttons[0].HasState(StateChecked) {
		t.Fatal("the first toggle is not active")
	}
	g.buttons[2].OnClick()
	g.KeyAction(KeyRight, 0) // clamped at the end: no change
	g.KeyAction(KeyLeft, 0)
	g.SetActive(1) // already active: no change
	if len(changes) != 2 || changes[0] != 2 || changes[1] != 1 {
		t.Errorf("changes = %v, want [2 1]", changes)
	}
	for i, b := range g.buttons {
		if b.HasState(StateChecked) != (i == 1) {
			t.Errorf("toggle %d checked=%v", i, b.HasState(StateChecked))
		}
	}
	g.Clear()
	if g.Active() != -1 || g.Len() != 0 {
		t.Errorf("cleared group: active=%d len=%d", g.Active(), g.Len())
	}
}

// TestButtonContent pins the halves: empty icon or label drops it.
func TestButtonContent(t *testing.T) {
	face := adaptiveFace(t)
	both := NewButtonContent(face, 14, "document-open", "Open")
	if n := len(both.row.Children()); n != 2 {
		t.Errorf("both halves: %d children", n)
	}
	label := NewButtonContent(face, 14, "", "Open")
	if n := len(label.row.Children()); n != 1 {
		t.Errorf("label only: %d children", n)
	}
	label.SetLabel("")
	if n := len(label.row.Children()); n != 0 {
		t.Errorf("emptied: %d children", n)
	}
}

// TestAvatar pins the initials and the stable per-name color.
func TestAvatar(t *testing.T) {
	face := adaptiveFace(t)
	cases := map[string]string{
		"Ada Lovelace":            "AL",
		"grace":                   "G",
		"Jean-Luc   de la Picard": "JP",
		"":                        "",
		"  ":                      "",
	}
	for name, want := range cases {
		if got := NewAvatar(face, 48, name).Initials(); got != want {
			t.Errorf("Initials(%q) = %q, want %q", name, got, want)
		}
	}
	a, b := NewAvatar(face, 48, "Ada Lovelace"), NewAvatar(face, 48, "Ada Lovelace")
	if a.Color() != b.Color() {
		t.Error("one name, two colors")
	}
}

// TestSplitButton pins the halves: the main fires OnClick, the arrow
// fires OnMenu with itself as the anchor.
func TestSplitButton(t *testing.T) {
	face := adaptiveFace(t)
	s := NewSplitButton(NewLabel(face, 14, "Save", DarkTheme().Text), 14)
	clicks := 0
	var anchor Boundser
	s.OnClick = func() { clicks++ }
	s.OnMenu = func(a Boundser) { anchor = a }
	s.main.OnClick()
	s.arrow.OnClick()
	if clicks != 1 || anchor != s.Arrow() {
		t.Errorf("clicks=%d anchor=%v", clicks, anchor)
	}
}

// TestBannerAndStatusPage pin the two announcement widgets' state.
func TestBannerAndStatusPage(t *testing.T) {
	face := adaptiveFace(t)
	b := NewBanner(face, 14, "Offline", "Retry")
	pressed := 0
	b.OnButton = func() { pressed++ }
	if b.Revealed() {
		t.Error("the banner starts revealed")
	}
	b.SetRevealed(true)
	b.button.OnClick()
	if !b.Revealed() || pressed != 1 {
		t.Errorf("revealed=%v pressed=%d", b.Revealed(), pressed)
	}
	if NewBanner(face, 14, "No action", "").button != nil {
		t.Error("an empty button label built a button")
	}

	p := NewStatusPage(face, "", "Nothing here", "Add something to get started.")
	before := len(p.column.Children())
	btn := NewButton(NewLabel(face, 14, "Add", DarkTheme().Text), 8, 4)
	p.SetChild(btn)
	kids := p.column.Children()
	if len(kids) != before+1 || kids[len(kids)-2] != Widget(btn) {
		t.Error("SetChild did not land above the trailing spacer")
	}
}

// TestWrapBoxJustify pins the justifications on one line of three
// 20px children in 100px with 5px spacing (leftover 30).
func TestWrapBoxJustify(t *testing.T) {
	xs := func(j Justify) []render.Rect {
		f := NewWrapBox(5, 5, j)
		for range 3 {
			f.Append(NewSpacer(20, 10))
		}
		f.Measure(Constraints{Max: Size{W: 100, H: 100}})
		f.Arrange(render.Rect{W: 100, H: 100})
		var out []render.Rect
		for _, c := range f.Children() {
			out = append(out, c.(Boundser).Bounds())
		}
		return out
	}
	if r := xs(JustifyStart); r[0].X != 0 || r[2].X != 50 {
		t.Errorf("start: %v", r)
	}
	if r := xs(JustifyCenter); r[0].X != 15 {
		t.Errorf("center: %v", r)
	}
	if r := xs(JustifyEnd); r[2].X+r[2].W != 100 {
		t.Errorf("end: %v", r)
	}
	if r := xs(JustifyFill); r[0].W != 30 || r[2].X+r[2].W != 100 {
		t.Errorf("fill: %v", r)
	}
	if r := xs(JustifySpread); r[0].X != 0 || r[2].X+r[2].W != 100 || r[1].X != 40 {
		t.Errorf("spread: %v", r)
	}
}

// TestBottomSheet pins the sheet's lifecycle on the instant clock:
// open shows it over a hit-tested backdrop, a short handle drag
// springs back, a long one dismisses (OnClosed fires), and a backdrop
// click and Escape close it too.
func TestBottomSheet(t *testing.T) {
	defer anim.SetInstant(true)()
	face := adaptiveFace(t)
	th := DarkTheme()
	content := NewLabel(face, 14, "content", th.Text)
	body := NewBox(Column, 0, 0)
	body.Append(NewSpacer(10, 120), false)
	b := NewBottomSheet(content, body)
	closed := 0
	b.OnClosed = func() { closed++ }
	layout := func() {
		b.Measure(Constraints{Max: Size{W: 300, H: 400}})
		b.Arrange(render.Rect{W: 300, H: 400})
	}
	layout()
	if hit := b.HitTest(Point{X: 150, Y: 380}); hit == Widget(b) {
		t.Fatal("a closed sheet captured the content")
	}

	b.SetOpen(true)
	rect := b.sheetRect()
	if rect.H != 144 || rect.Y != 256 {
		t.Fatalf("open sheet rect = %+v, want 144 tall at the bottom", rect)
	}
	if hit := b.HitTest(Point{X: 150, Y: 50}); hit != Widget(b) {
		t.Error("the backdrop is not the sheet's")
	}

	b.PressAt(Point{X: 150, Y: rect.Y + 5})
	b.DragMove(Point{X: 150, Y: rect.Y + 20})
	b.PressEnd()
	if !b.Open() || b.dragDY != 0 {
		t.Errorf("a short drag closed it or kept the offset (open=%v dy=%d)", b.Open(), b.dragDY)
	}
	b.PressAt(Point{X: 150, Y: rect.Y + 5})
	b.DragMove(Point{X: 150, Y: rect.Y + 100})
	b.PressEnd()
	if b.Open() || closed != 1 {
		t.Errorf("a long drag: open=%v closed=%d", b.Open(), closed)
	}

	b.SetOpen(true)
	b.ClickAt(Point{X: 150, Y: 20})
	if b.Open() {
		t.Error("a backdrop click left it open")
	}
	b.SetOpen(true)
	b.KeyAction(KeyDismiss, 0)
	if b.Open() || closed != 3 {
		t.Errorf("escape: open=%v closed=%d", b.Open(), closed)
	}
}

// TestGoldenAdw pins the painted look of the new Adwaita widgets.
func TestGoldenAdw(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	g := NewToggleGroup(NewLabel(face, 14, "Day", th.Text), NewLabel(face, 14, "Week", th.Text), NewLabel(face, 14, "Month", th.Text))
	g.SetActive(1)
	NewGolden(t, g, "togglegroup", goldenTheme(th))

	row := NewBox(Row, 8, 0)
	row.Append(NewAvatar(face, 48, "Ada Lovelace"), false)
	pic := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range pic.Pix {
		pic.Pix[i] = 0xff
	}
	pic.Set(0, 0, color.RGBA{0xff, 0, 0, 0xff})
	img := NewAvatar(face, 48, "x")
	img.SetImage(pic)
	row.Append(img, false)
	NewGolden(t, row, "avatars", goldenTheme(th))

	NewGolden(t, NewStatusPage(face, "", "No results", "Try a different search term or clear the filters."), "statuspage", goldenTheme(th), goldenFrame(320, 220))

	b := NewBanner(face, 14, "You are offline", "Retry")
	b.SetRevealed(true)
	NewGolden(t, b, "banner", goldenTheme(th), goldenFrame(320, 48))

	NewGolden(t, NewSplitButton(NewLabel(face, 14, "Save", th.Text), 14), "splitbutton", goldenTheme(th))

	sheet := NewBottomSheet(NewLabel(face, 14, "content", th.Text), NewLabel(face, 14, "Sheet body", th.Text))
	sheet.SetOpen(true)
	NewGolden(t, sheet, "bottomsheet", goldenTheme(th), goldenFrame(240, 200))
}
