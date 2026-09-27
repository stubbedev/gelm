package widget

import (
	"slices"
	"testing"

	"github.com/stubbedev/gelm/render"
)

// newDropdown builds a face-equipped dropdown arranged at (10, 20), so
// the open list hangs below at a known offset.
func newDropdown(t *testing.T, items ...string) *Dropdown {
	t.Helper()
	dd := NewDropdown(items, 0)
	dd.SetFace(entryFace(t), 14)
	sz := dd.Measure(Constraints{Max: Size{W: 400, H: 100}})
	dd.Arrange(render.Rect{X: 10, Y: 20, W: sz.W, H: sz.H})
	return dd
}

// rowPoint maps item i of the open list to a clickable point.
func rowPoint(dd *Dropdown, i int) Point {
	b := dd.menu.Bounds()
	return Point{X: b.X + 10, Y: b.Y + 4 + i*dd.menu.itemH + dd.menu.itemH/2}
}

func facePoint(dd *Dropdown) Point {
	b := dd.Bounds()
	return Point{X: b.X + b.W/2, Y: b.Y + b.H/2}
}

func TestDropdownClick(t *testing.T) {
	dd := newDropdown(t, "Light", "Dark", "System")

	t.Run("clicking the face opens the list below it", func(t *testing.T) {
		dd.ClickAt(facePoint(dd))
		if !dd.Opened() {
			t.Fatal("face click did not open the list")
		}
		if b := dd.menu.Bounds(); b.Y != dd.Bounds().Y+dd.Bounds().H {
			t.Errorf("list top = %d, want directly below the face at %d", b.Y, dd.Bounds().Y+dd.Bounds().H)
		}
	})

	t.Run("clicking a row selects it exactly once and closes", func(t *testing.T) {
		fired := 0
		dd.OnSelect = func(int) { fired++ }
		// Row clicks reach the list through its own ClickAt, exactly
		// what the Router does when the hit lands on the open list.
		dd.menu.ClickAt(rowPoint(dd, 2))
		if dd.Selected() != 2 {
			t.Errorf("selected = %d, want 2", dd.Selected())
		}
		if fired != 1 {
			t.Errorf("OnSelect fired %d times, want exactly 1", fired)
		}
		if dd.Opened() {
			t.Error("picking a row left the list open")
		}
	})

	t.Run("hit test resolves the list outside the face while open", func(t *testing.T) {
		dd.ClickAt(facePoint(dd))
		if got := dd.HitTest(rowPoint(dd, 1)); got != Widget(dd.menu) {
			t.Errorf("hit = %v, want the item list", got)
		}
		dd.ClickAt(facePoint(dd)) // second face click toggles closed
		if dd.Opened() {
			t.Error("second face click did not close the list")
		}
	})
}

func TestDropdownRouterClick(t *testing.T) {
	dd := newDropdown(t, "Light", "Dark", "System")
	root := NewBox(Row, 0, 0)
	root.Append(dd, false)
	sz := root.Measure(Constraints{Max: Size{W: 400, H: 400}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: sz.W, H: sz.H})
	router := &Router{Root: root}

	fired := 0
	dd.OnSelect = func(int) { fired++ }

	// A pointer click on the face opens; the press also focuses, so
	// the later keyboard test runs against a realistically focused
	// dropdown.
	router.Press(BTNLeft, facePoint(dd))
	router.Release(BTNLeft, facePoint(dd))
	if !dd.Opened() {
		t.Fatal("router click did not open the list")
	}
	if router.Focused() != Widget(dd) {
		t.Fatal("face click did not focus the dropdown")
	}

	router.Press(BTNLeft, rowPoint(dd, 1))
	router.Release(BTNLeft, rowPoint(dd, 1))
	if dd.Selected() != 1 || fired != 1 {
		t.Errorf("router click: selected = %d fired = %d, want 1/1", dd.Selected(), fired)
	}
}

func TestDropdownKeyboard(t *testing.T) {
	t.Run("Enter, Space, and Down open while focused", func(t *testing.T) {
		for _, key := range []struct {
			name   string
			points func(*Dropdown)
		}{
			{"Enter", func(dd *Dropdown) { dd.KeyAction(KeyEnter, 0) }},
			{"Space", func(dd *Dropdown) { dd.InsertRune(' ') }},
			{"Down", func(dd *Dropdown) { dd.KeyAction(KeyDown, 0) }},
		} {
			dd := newDropdown(t, "Light", "Dark", "System")
			key.points(dd)
			if !dd.Opened() {
				t.Errorf("%s did not open the list", key.name)
			}
		}
	})

	t.Run("arrows navigate from the selection, Enter picks once", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark", "System")
		fired := 0
		dd.OnSelect = func(int) { fired++ }

		dd.KeyAction(KeyEnter, 0) // open
		if got := dd.menu.hovered; got != 0 {
			t.Fatalf("list opened with highlight %d, want the selection 0", got)
		}
		dd.KeyAction(KeyDown, 0)
		if got := dd.menu.hovered; got != 1 {
			t.Fatalf("Down moved highlight to %d, want 1", got)
		}
		dd.KeyAction(KeyUp, 0)
		dd.KeyAction(KeyDown, 0) // back to 1
		dd.KeyAction(KeyEnter, 0)

		if dd.Selected() != 1 {
			t.Errorf("selected = %d, want 1", dd.Selected())
		}
		if fired != 1 {
			t.Errorf("OnSelect fired %d times, want exactly 1", fired)
		}
		if dd.Opened() {
			t.Error("Enter left the list open")
		}
	})

	t.Run("Esc cancels without changing the selection", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark", "System")
		fired := 0
		dd.OnSelect = func(int) { fired++ }

		dd.KeyAction(KeyEnter, 0)
		dd.KeyAction(KeyDown, 0)
		dd.KeyAction(KeyDown, 0)
		dd.KeyAction(KeyDismiss, 0)

		if dd.Opened() {
			t.Fatal("Esc did not close the list")
		}
		if dd.Selected() != 0 {
			t.Errorf("cancel changed the selection to %d, want 0", dd.Selected())
		}
		if fired != 0 {
			t.Errorf("cancel fired OnSelect %d times, want 0", fired)
		}
	})

	t.Run("arrows do nothing while closed", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark", "System")
		dd.KeyAction(KeyUp, 0)
		if dd.Opened() || dd.Selected() != 0 {
			t.Errorf("closed arrows: open = %v selected = %d", dd.Opened(), dd.Selected())
		}
	})
}

func TestDropdownOnSelectExactlyOnce(t *testing.T) {
	dd := newDropdown(t, "Light", "Dark", "System")
	fired := 0
	dd.OnSelect = func(int) { fired++ }

	t.Run("re-opening and re-picking the selected row fires nothing", func(t *testing.T) {
		dd.ClickAt(facePoint(dd))        // open
		dd.menu.ClickAt(rowPoint(dd, 0)) // pick the already-selected row
		if fired != 0 {
			t.Errorf("no-change pick fired %d times, want 0", fired)
		}
	})

	t.Run("open plus Enter on the seeded highlight fires nothing", func(t *testing.T) {
		dd.KeyAction(KeyEnter, 0) // open, highlight on the selection
		dd.KeyAction(KeyEnter, 0) // re-pick it
		if fired != 0 {
			t.Errorf("re-pick fired %d times, want 0", fired)
		}
	})

	t.Run("each real change fires exactly once, pointer or not", func(t *testing.T) {
		dd.SetSelected(2)
		dd.SetSelected(2)
		if fired != 1 {
			t.Errorf("SetSelected fired %d times, want 1", fired)
		}
		dd.KeyAction(KeyEnter, 0)
		dd.KeyAction(KeyUp, 0) // 2 -> 1
		dd.KeyAction(KeyEnter, 0)
		if fired != 2 {
			t.Errorf("keyboard change fired %d times total, want 2", fired)
		}
		if dd.Selected() != 1 {
			t.Errorf("selected = %d, want 1", dd.Selected())
		}
	})
}

func TestDropdownOf(t *testing.T) {
	type mode string
	items := []DropdownItem[mode]{
		{Label: "Light", Value: "light"},
		{Label: "Dark", Value: "dark"},
		{Label: "System", Value: "system"},
	}
	dd := NewDropdownOf(items, 0)
	dd.SetFace(entryFace(t), 14)
	sz := dd.Measure(Constraints{Max: Size{W: 400, H: 100}})
	dd.Arrange(render.Rect{X: 10, Y: 20, W: sz.W, H: sz.H})

	if got := dd.Value(); got != "light" {
		t.Fatalf("initial value = %q, want light", got)
	}

	calls := 0
	var got mode
	dd.OnSelect = func(i int, v mode) {
		calls++
		got = v
	}

	dd.KeyAction(KeyEnter, 0)
	dd.KeyAction(KeyDown, 0)
	dd.KeyAction(KeyEnter, 0)
	if calls != 1 || got != "dark" {
		t.Fatalf("keyboard pick: calls = %d value = %q, want 1/dark", calls, got)
	}
	if v := dd.Value(); v != "dark" {
		t.Errorf("Value = %q, want dark", v)
	}

	dd.SetSelected(2)
	if v := dd.Value(); v != "system" {
		t.Errorf("Value after SetSelected = %q, want system", v)
	}
	if got := dd.Selected(); got != 2 {
		t.Errorf("Selected = %d, want 2", got)
	}
}

func TestDropdownChildren(t *testing.T) {
	dd := newDropdown(t, "Light", "Dark", "System")
	root := NewBox(Row, 0, 0)
	root.Append(dd, false)
	sz := root.Measure(Constraints{Max: Size{W: 400, H: 400}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: sz.W, H: sz.H})
	router := &Router{Root: root}

	t.Run("closed hides the list from traversal", func(t *testing.T) {
		if kids := dd.Children(); kids != nil {
			t.Errorf("closed children = %v, want none", kids)
		}
		// Focus the face without clicking it open: a press focuses,
		// and CancelPress drops the gesture before it becomes one.
		router.Press(BTNLeft, facePoint(dd))
		router.CancelPress()
		if router.Focused() != Widget(dd) {
			t.Fatal("press did not focus the dropdown")
		}
		router.FocusNext()
		if got := router.Focused(); got != Widget(dd) {
			t.Errorf("closed traversal focused %v, want the dropdown to stay put", got)
		}
	})

	t.Run("open exposes exactly the list for traversal", func(t *testing.T) {
		dd.ClickAt(facePoint(dd))
		kids := dd.Children()
		if len(kids) != 1 || kids[0] != Widget(dd.menu) {
			t.Fatalf("open children = %v, want exactly the item list", kids)
		}
		router.FocusNext()
		if got := router.Focused(); got != Widget(dd.menu) {
			t.Errorf("open traversal focused %v, want the item list", got)
		}
	})
}

func TestDropdownDisabled(t *testing.T) {
	t.Run("disabled ignores clicks and keys", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark", "System")
		dd.SetEnabled(false)
		if dd.Enabled() {
			t.Fatal("SetEnabled(false) left the dropdown enabled")
		}

		dd.ClickAt(facePoint(dd))
		dd.KeyAction(KeyEnter, 0)
		dd.KeyAction(KeyDown, 0)
		dd.InsertRune(' ')
		if dd.Opened() {
			t.Error("a disabled dropdown opened")
		}
		dd.SetSelected(1) // programmatic changes still apply
		if dd.Selected() != 1 {
			t.Errorf("SetSelected on disabled = %d, want 1", dd.Selected())
		}
	})

	t.Run("disabling closes an open list", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark", "System")
		dd.KeyAction(KeyEnter, 0)
		dd.SetEnabled(false)
		if dd.Opened() || dd.Children() != nil {
			t.Error("disabling left the list open or traversable")
		}
	})
}

func TestDropdownDamage(t *testing.T) {
	dd := newDropdown(t, "Light", "Dark", "System")

	// Opening must owe the list's pixels; closing must owe them back,
	// even though the closed list is no longer exposed as a child.
	dd.ClickAt(facePoint(dd))
	rects, any := CollectDamage(dd)
	if !any {
		t.Fatal("opening owed no damage")
	}
	list := dd.menu.Bounds()
	if !slices.Contains(rects, list) {
		t.Errorf("open damage %v missed the list rect %v", rects, list)
	}
	CollectDamage(dd) // drain

	dd.ClickAt(facePoint(dd)) // close
	rects, any = CollectDamage(dd)
	if !any {
		t.Fatal("closing owed no damage")
	}
	if !slices.Contains(rects, list) {
		t.Errorf("close damage %v missed the old list rect %v", rects, list)
	}
	if _, any := CollectDamage(dd); any {
		t.Error("idle dropdown still owes damage")
	}
}

func TestDropdownPaintAndRole(t *testing.T) {
	t.Run("open list paints ink on every row", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark", "System")
		dd.ClickAt(facePoint(dd))
		b := dd.Bounds().Union(dd.menu.Bounds())
		data := make([]byte, render.Stride(b.W)*(b.H+2))
		cv := render.New(data, render.Stride(b.W), b.W, b.H+2)
		dd.Arrange(render.Rect{X: 0, Y: 0, W: dd.Bounds().W, H: dd.Bounds().H})
		dd.Paint(cv)
		for row := range 3 {
			y := dd.menu.Bounds().Y + 4 + row*dd.menu.itemH + dd.menu.itemH/2
			ink := 0
			for x := range dd.menu.Bounds().W {
				if render.ColorFromBytes(data[y*render.Stride(b.W)+x*4:]).A() > 0 {
					ink++
				}
			}
			if ink == 0 {
				t.Errorf("row %d painted nothing", row)
			}
		}
	})

	t.Run("face-less dropdown paints and stays closed", func(t *testing.T) {
		dd := NewDropdown([]string{"a", "b"}, 0)
		sz := dd.Measure(Constraints{Max: Size{W: 400, H: 100}})
		if sz.W <= 0 || sz.H <= 0 {
			t.Fatalf("face-less measure = %v", sz)
		}
		dd.Arrange(render.Rect{X: 0, Y: 0, W: sz.W, H: sz.H})
		dd.ClickAt(facePoint(dd))
		if dd.Opened() {
			t.Error("a dropdown without a face opened a list")
		}
		cv := render.New(make([]byte, render.Stride(sz.W)*sz.H), render.Stride(sz.W), sz.W, sz.H)
		dd.Paint(cv)
	})

	t.Run("role is combo-box and the name is the selection", func(t *testing.T) {
		dd := newDropdown(t, "Light", "Dark")
		dd.SetSelected(1)
		st := Describe(dd)
		if st.Role != RoleComboBox || st.Role.String() != "combo-box" {
			t.Errorf("role = %v, want combo-box", st.Role)
		}
		if st.Name != "Dark" {
			t.Errorf("a11y name = %q, want the selection", st.Name)
		}
	})
}
