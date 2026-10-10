package widget

import "testing"

func TestPreferencesSearchListsMatchingRowsAcrossPages(t *testing.T) {
	face := testFace(t)
	v := NewPreferencesView(face, 13)
	general, appearance := NewPreferencesPage(), NewPreferencesPage()
	general.SetTitle("General")
	appearance.SetTitle("Appearance")
	g1 := NewPreferencesGroup(face, 13, "Startup")
	autostart := NewSwitchRow(face, 13, "Launch at login", "Start in the background", nil)
	hidden := NewActionRow(face, 13, "Login secret", "")
	hidden.SetSearchable(false)
	g1.Add(autostart)
	g1.Add(hidden)
	general.Add(g1)
	g2 := NewPreferencesGroup(face, 13, "Theme")
	dark := NewSwitchRow(face, 13, "Dark style", "Follow the login screen", nil)
	g2.Add(dark)
	appearance.Add(g2)
	v.Add(general)
	v.Add(appearance)
	if v.VisiblePage() != general || len(v.switcher.Children()) != 1 {
		t.Fatalf("visible %v, switcher %d", v.VisiblePage(), len(v.switcher.Children()))
	}

	v.Search("LOGIN")
	if v.VisiblePage() != nil || len(v.results.Children()) != 2 {
		t.Fatalf("search for login: %d results, want the two searchable rows mentioning it", len(v.results.Children()))
	}
	second := v.results.Children()[1].(*ActionRow)
	if second.Title() != "Dark style" || second.Subtitle() != "Appearance › Follow the login screen" {
		t.Errorf("result %q / %q", second.Title(), second.Subtitle())
	}
	second.OnActivate()
	if v.VisiblePage() != appearance || appearance.reveal != Widget(dark) {
		t.Error("activating a result did not show its page and reveal the row")
	}

	v.Search("nothing matches this")
	if got := v.results.Children(); len(got) != 1 {
		t.Errorf("an empty search showed %d children, want the no-results label", len(got))
	}
	v.Search("")
	if v.VisiblePage() != appearance {
		t.Error("clearing the search did not return to the page shown before")
	}
}
