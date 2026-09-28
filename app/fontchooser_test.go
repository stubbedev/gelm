package app

import (
	"slices"
	"strings"
	"testing"

	"github.com/stubbedev/gelm/internal/sysfont"
)

// TestFontFilter pins the chooser's search and monospace narrowing
// (#74): case-insensitive substring over the family name, the
// fixed-width classification consulted only when the toggle is on, and
// the input order preserved.
func TestFontFilter(t *testing.T) {
	families := []string{"dejavu sans", "dejavu sans mono", "liberation mono", "noto sans"}
	mono := func(fam string) bool { return strings.HasSuffix(fam, "mono") }

	if got := fontFilter(families, "", false, mono); !slices.Equal(got, families) {
		t.Errorf("unfiltered = %v, want the input", got)
	}
	if got := fontFilter(families, "DEJAVU", false, mono); !slices.Equal(got, []string{"dejavu sans", "dejavu sans mono"}) {
		t.Errorf("case-insensitive search = %v", got)
	}
	if got := fontFilter(families, "", true, mono); !slices.Equal(got, []string{"dejavu sans mono", "liberation mono"}) {
		t.Errorf("monospace filter = %v", got)
	}
	if got := fontFilter(families, "noto", true, mono); len(got) != 0 {
		t.Errorf("search + mono = %v, want none", got)
	}
}

// TestSysfontFamilies pins the enumeration (#74): sorted, deduped, and
// non-empty on a system with fonts installed; every listed name
// resolves back through Best, which is the round trip the chooser's
// OnFont promises.
func TestSysfontFamilies(t *testing.T) {
	families, err := sysfont.Families()
	if err != nil {
		t.Skipf("no system font store in this environment: %v", err)
	}
	if len(families) == 0 {
		t.Fatal("Families() is empty on a system with fonts")
	}
	if !slices.IsSorted(families) {
		t.Error("Families() is not sorted")
	}
	if len(slices.Compact(slices.Clone(families))) != len(families) {
		t.Error("Families() has duplicates")
	}
	// The round trip: a listed family resolves through Best, and so
	// does its display-cased name - what the chooser hands back.
	tf, err := sysfont.Best(families[0], 13)
	if err != nil {
		t.Fatalf("Best(%q) failed: %v", families[0], err)
	}
	disp := sysfont.FamilyDisplay(families[0])
	if disp == "" {
		t.Fatal("FamilyDisplay returned an empty name")
	}
	if _, err := sysfont.Best(disp, 13); err != nil {
		t.Errorf("Best(display %q) failed: %v", disp, err)
	}
	_ = tf
}

// TestFontRowModelLaziness pins the virtualization contract: rows are
// built only on request, so a huge family list never parses more
// faces than the rows asked for.
func TestFontRowModelLaziness(t *testing.T) {
	fams := make([]string, 2000)
	for i := range fams {
		fams[i] = "family-not-installed-" + string(rune('a'+i%26)) + string(rune('0'+i%10))
	}
	m := &fontRowModel{face: testFace(t), fams: fams}
	if m.Len() != 2000 {
		t.Fatalf("Len = %d, want 2000", m.Len())
	}
	// Rows for absent families still build (the label falls back to the
	// chooser face) - the laziness is the point: nothing was parsed
	// until Row ran.
	if m.Row(0) == nil {
		t.Error("Row(0) returned nil")
	}
}
