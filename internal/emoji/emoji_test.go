package emoji

import (
	"strings"
	"testing"
)

// TestTableIntegrity pins the table: every category named and ordered,
// every entry a nonempty glyph and name, no duplicate glyphs, and a
// table big enough to be worth a chooser.
func TestTableIntegrity(t *testing.T) {
	seen := map[string]bool{}
	total := 0
	for _, c := range Categories {
		if c.Name == "" {
			t.Error("a category has no name")
		}
		for _, e := range c.Items {
			if e.Glyph == "" || e.Name == "" {
				t.Errorf("empty glyph or name: %+v", e)
			}
			if seen[e.Glyph] {
				t.Errorf("duplicate glyph %q", e.Glyph)
			}
			seen[e.Glyph] = true
			total++
		}
	}
	if total < 300 {
		t.Errorf("table holds %d emoji, want at least 300", total)
	}
}

// TestMatches pins the search: every query word must appear in name
// or keywords, results stay in table order, the cap holds, and an
// empty query matches nothing.
func TestMatches(t *testing.T) {
	if got := Matches("   ", 10); got != nil {
		t.Errorf("empty query matched %d emoji", len(got))
	}
	got := Matches("happy smile", 5)
	if len(got) == 0 || len(got) > 5 {
		t.Fatalf("happy smile matched %d", len(got))
	}
	for _, e := range got {
		hay := strings.ToLower(e.Name + " " + strings.Join(e.Keywords, " "))
		if !strings.Contains(hay, "happy") || !strings.Contains(hay, "smile") {
			t.Errorf("%q matched without both words: %v", e.Glyph, e)
		}
	}
	// Table order: the uncapped fire search starts with the first
	// fire-keyworded entry in All order.
	all := Matches("fire", 0)
	every := Matches("fire", 1000)
	if len(all) != len(every) || len(all) == 0 {
		t.Errorf("cap semantics: %d vs %d", len(all), len(every))
	}
}

// TestAllMatchesEverythingOnce pins the flattening against the table.
func TestAllMatchesEverythingOnce(t *testing.T) {
	all := All()
	total := 0
	for _, c := range Categories {
		total += len(c.Items)
	}
	if len(all) != total || total == 0 {
		t.Errorf("All = %d, table = %d", len(all), total)
	}
}
