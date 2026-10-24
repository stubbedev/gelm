package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/anim"
)

// newTestSidebar builds a sidebar of a group with an active item, a
// badge and a submenu, over a content label, laid out width wide.
func newTestSidebar(t *testing.T, width int) (*Sidebar, *[]string) {
	t.Helper()
	t.Cleanup(anim.SetInstant(true))
	face := goldenFace(t)
	var picked []string
	pick := func(s string) func() { return func() { picked = append(picked, s) } }
	s := NewSidebar(face, NewLabel(face, 14, "Content", 0))
	s.SetHeader(NewLabel(face, 14, "Acme Inc", 0))
	s.AddGroup("Platform",
		SidebarItem{Label: "Home", Icon: NewSymbol(SymbolMaximize, 16), Active: true, OnActivate: pick("home")},
		SidebarItem{Label: "Inbox", Icon: NewSymbol(SymbolSearch, 16), Badge: "24", OnActivate: pick("inbox")},
		SidebarItem{Label: "Settings", Icon: NewSymbol(SymbolCrosshair, 16), Items: []SidebarItem{
			{Label: "General", OnActivate: pick("general")},
			{Label: "Team", OnActivate: pick("team")},
		}},
	)
	s.SetFooter(NewLabel(face, 14, "shadcn", 0))
	arrangeTree(t, s, width, 400)
	arrangeTree(t, s, width, 400)
	return s, &picked
}

// Picking an item makes it the only active one; a parent toggles its
// submenu.
func TestSidebarItems(t *testing.T) {
	s, picked := newTestSidebar(t, 900)
	home, inbox, settings := s.rows[0], s.rows[1], s.rows[2]
	inbox.button.OnClick()
	if !inbox.button.HasState(StateChecked) || home.button.HasState(StateChecked) || len(*picked) != 1 {
		t.Fatalf("after picking Inbox: home %v inbox %v picked %v", home.button.HasState(StateChecked), inbox.button.HasState(StateChecked), *picked)
	}
	settings.button.OnClick()
	if !settings.sub.Visible() {
		t.Error("the submenu did not open")
	}
	s.rows[3].button.OnClick()
	if (*picked)[1] != "general" || !s.rows[3].button.HasState(StateChecked) {
		t.Errorf("submenu pick %v", *picked)
	}
}

// A wide sidebar collapses off canvas or to an icon rail and expands
// back; ToggleAction toggles it.
func TestSidebarCollapses(t *testing.T) {
	s, _ := newTestSidebar(t, 900)
	if s.dock.Bounds().W != sidebarWidth {
		t.Fatalf("expanded dock %d wide", s.dock.Bounds().W)
	}
	s.ToggleAction().Activate()
	arrangeTree(t, s, 900, 400)
	if s.Expanded() || s.dock.Visible() {
		t.Fatal("an off-canvas collapse left the dock showing")
	}
	s.Toggle()
	s.SetCollapse(SidebarIcon)
	s.Toggle()
	arrangeTree(t, s, 900, 400)
	if s.dock.Bounds().W != sidebarRail || s.rows[0].label.Visible() {
		t.Errorf("the icon rail is %d wide with labels %v", s.dock.Bounds().W, s.rows[0].label.Visible())
	}
	s.Toggle()
	arrangeTree(t, s, 900, 400)
	if !s.rows[0].label.Visible() || s.dock.Bounds().W != sidebarWidth {
		t.Error("expanding did not bring the labels and width back")
	}
}

// Below the narrow width the sidebar hides and Toggle opens it as a
// sheet, which a pick closes.
func TestSidebarNarrow(t *testing.T) {
	s, _ := newTestSidebar(t, 500)
	if !s.Narrow() || s.dock.Visible() {
		t.Fatal("a narrow sidebar still docks")
	}
	s.Toggle()
	if !s.sheet.Open() {
		t.Fatal("Toggle did not open the sheet")
	}
	s.rows[1].button.OnClick()
	if s.sheet.Open() {
		t.Error("a pick left the sheet open")
	}
	s.SetNarrowWidth(400)
	arrangeTree(t, s, 500, 400)
	if s.Narrow() || !s.dock.Visible() {
		t.Error("a lower narrow width did not dock the sidebar at 500px")
	}
}

// TestGoldenShadcnSidebar pins the expanded sidebar and the icon rail,
// in both palettes.
func TestGoldenShadcnSidebar(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *Theme
	}{{"dark", DarkTheme()}, {"light", LightTheme()}} {
		t.Run(tc.name, func(t *testing.T) {
			SetTheme(tc.th)
			defer SetTheme(nil)
			wide, _ := newTestSidebar(t, 800)
			rail, _ := newTestSidebar(t, 800)
			rail.SetCollapse(SidebarIcon)
			rail.SetExpanded(false)
			row := NewBox(Row, 16, 0).Append(wide, true).Append(rail, true)
			row.SetHomogeneous(true)
			NewGolden(t, row, "shadcn-sidebar-"+tc.name, goldenTheme(tc.th), goldenFrame(1616, 320))
		})
	}
}
