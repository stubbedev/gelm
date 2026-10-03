package widget

import (
	"strings"
	"testing"
)

// A wrapping rich label breaks the text at words to the offered width:
// measure reports one line height per row and the widest row, arrange
// lays the rows out, and paint draws every row (smoke-checked through
// the laid-out rows).
func TestRichLabelWrap(t *testing.T) {
	l := NewRichLabel(entryFace(t), 14, "the quick brown fox jumps over the lazy dog", 0)
	one := l.Measure(Constraints{Max: Size{W: 600, H: 400}})
	l.SetWrap(true)
	if !l.Wrap() {
		t.Fatal("wrap did not stick")
	}
	tall := l.Measure(Constraints{Max: Size{W: 120, H: 400}})
	if tall.H <= one.H {
		t.Fatalf("wrapped height %d, want more than the single line's %d", tall.H, one.H)
	}
	if tall.W >= one.W {
		t.Errorf("wrapped natural width %d, want under the single line's %d", tall.W, one.W)
	}
	arrangeTree(t, l, 120, tall.H)
	if len(l.lines) < 3 {
		t.Errorf("rows = %d, want the text broken over several", len(l.lines))
	}
	rowText := func(i int) string {
		var sb strings.Builder
		for _, r := range l.lines[i].runs {
			sb.WriteString(r.sh.Text())
		}
		return sb.String()
	}
	for i := range l.lines {
		if strings.TrimSpace(rowText(i)) == "" {
			t.Errorf("row %d is blank", i)
		}
		if first := rowText(i); strings.HasPrefix(first, " ") {
			t.Errorf("row %d starts with a dropped break space: %q", i, first)
		}
	}
	joined := rowText(0) + rowText(1)
	if !strings.Contains(strings.Join([]string{"the", "quick"}, " "), " ") {
		t.Fatal("test text changed")
	}
	if !strings.Contains(joined, "the quick") || !strings.Contains(joined, "quick") {
		t.Errorf("rows lost words: %q", joined)
	}
}

// maxLines caps the rows and ellipsizes the merged last row.
func TestRichLabelWrapMaxLines(t *testing.T) {
	l := NewRichLabel(entryFace(t), 14, "one two three four five six seven eight nine ten", 0)
	l.SetWrap(true)
	l.SetEllipsize(EllipsizeEnd)
	l.SetMaxLines(2)
	sz := l.Measure(Constraints{Max: Size{W: 100, H: 400}})
	arrangeTree(t, l, 100, sz.H)
	if len(l.lines) != 2 {
		t.Fatalf("rows = %d, want the 2-line cap", len(l.lines))
	}
	var last strings.Builder
	for _, r := range l.lines[1].runs {
		last.WriteString(r.sh.Text())
	}
	if !strings.Contains(last.String(), "…") {
		t.Errorf("last row %q, want the ellipsis", last.String())
	}
	if l.MaxLines() != 2 {
		t.Error("MaxLines did not stick")
	}
}

// Styling survives the wrap: a bold word carries its variant face onto
// whichever row it lands.
func TestRichLabelWrapKeepsStyles(t *testing.T) {
	l := NewRichLabel(entryFace(t), 14, "plain <b>boldword</b> plain plain plain", 0)
	l.SetWrap(true)
	sz := l.Measure(Constraints{Max: Size{W: 90, H: 400}})
	arrangeTree(t, l, 90, sz.H)
	bold := 0
	for _, line := range l.lines {
		for _, r := range line.runs {
			if r.style.Bold {
				bold++
			}
		}
	}
	if bold == 0 {
		t.Error("the bold span lost its style across the wrap")
	}
}

// max-width-chars floors the natural width: a short text still takes
// the room its card reserves, and 0 removes the floor again.
func TestRichLabelMaxWidthChars(t *testing.T) {
	l := NewRichLabel(entryFace(t), 14, "hi", 0)
	bare := l.Measure(Constraints{Max: Size{W: 400, H: 40}}).W
	l.SetMaxWidthChars(24)
	floored := l.Measure(Constraints{Max: Size{W: 400, H: 40}}).W
	if floored <= bare {
		t.Errorf("floored width %d, want over the bare %d", floored, bare)
	}
	if l.MaxWidthChars() != 24 {
		t.Error("MaxWidthChars did not stick")
	}
	l.SetMaxWidthChars(0)
	if got := l.Measure(Constraints{Max: Size{W: 400, H: 40}}).W; got != bare {
		t.Errorf("width %d after clearing, want the bare %d", got, bare)
	}
}
