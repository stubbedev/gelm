package app

import (
	"testing"
)

// TestAboutSections pins the about page's block layout: release notes,
// credit groups in config order, then the license - empty groups and
// absent blocks drop out entirely.
func TestAboutSections(t *testing.T) {
	cfg := AboutConfig{
		Name:         "gelm",
		Version:      "1.0",
		ReleaseNotes: "Fourth pass.\nTyped messaging.\nFile dialogs.",
		Credits: []AboutCredit{
			{Role: "Authors", Names: []string{"A", "B"}},
			{Role: "Artists"}, // empty groups drop
			{Role: "Documenters", Names: []string{"C"}},
		},
		License: "MIT\nPermission is hereby granted",
	}
	sections := aboutSections(cfg)
	if len(sections) != 4 {
		t.Fatalf("sections = %d, want notes, two credit groups, license", len(sections))
	}
	want := []struct {
		heading string
		lines   int
	}{
		{"What's New", 3},
		{"Authors", 2},
		{"Documenters", 1},
		{"Legal", 2},
	}
	for i, w := range want {
		if sections[i].Heading != w.heading || len(sections[i].Lines) != w.lines {
			t.Errorf("section %d = %+v, want %s x%d", i, sections[i], w.heading, w.lines)
		}
	}
	if sections[2].Lines[0] != "C" {
		t.Errorf("credit group content = %+v", sections[2])
	}
}

// TestAboutSectionsEmpty covers the title-card case: no notes, no
// credits, no license means no scrolled body at all.
func TestAboutSectionsEmpty(t *testing.T) {
	if sections := aboutSections(AboutConfig{Name: "gelm", Version: "1.0"}); len(sections) != 0 {
		t.Errorf("bare config produced sections: %+v", sections)
	}
}
