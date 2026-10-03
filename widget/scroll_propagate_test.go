package widget

import (
	"testing"
)

// propagate-natural-height: the scroll stops offering its shortfall —
// the panel-reserve math (measure less Shrinkable, wayle's
// dropdown panel) then grows the surface to the content's natural
// height instead of scrolling at the base height. A plain scroll gives
// the shortfall back and scrolls.
func TestScrollPropagateNaturalHeight(t *testing.T) {
	build := func() (*Box, *Scroll) {
		col := NewBox(Column, 0, 0)
		sc := NewScroll(NewSpacer(100, 400))
		sc.VerticalOnly = true
		col.Append(sc, true)
		return col, sc
	}
	con := Constraints{Max: Size{W: 200, H: 350}}

	col, sc := build()
	arrangeTree(t, col, 200, 350)
	if sc.Shrinkable() <= 0 {
		t.Fatal("a plain scroll cannot shrink; the test needs the floor below the content")
	}
	if got := sc.Bounds().H; got != 350 {
		t.Errorf("plain scroll took %d of a 350 column, want the whole room", got)
	}
	// The wayle reserve: measured height less what it gives up.
	if grow := sc.Measure(con).H - sc.Shrinkable(); grow != scrollFloor {
		t.Errorf("plain reserve %d, want just the scroll floor", grow)
	}

	col2, sc2 := build()
	sc2.PropagateNaturalHeight = true
	arrangeTree(t, col2, 200, 350)
	if sc2.Shrinkable() != 0 {
		t.Error("a propagating scroll still offers to shrink")
	}
	if grow := sc2.Measure(con).H - sc2.Shrinkable(); grow != 350 {
		t.Errorf("propagating reserve %d, want the whole measured 350 (the panel grows past it via the natural, capped by its own ceiling)", grow)
	}
}
