package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// layoutColumn measures and arranges a column in a w x h rect.
func layoutColumn(b *Box, w, h int) {
	b.Measure(Constraints{Max: Size{W: w, H: h}})
	b.Arrange(render.Rect{W: w, H: h})
}

// TestColumnShrinksItsScroll pins GTK's give-way: a column short of
// room takes the shortfall from its expanding Scroll, so the header and
// footer around it stay inside the box and the scroll scrolls.
// Regression: a Box never shrank a child, so a form in a fixed-height
// popover ran its buttons off the bottom.
func TestColumnShrinksItsScroll(t *testing.T) {
	header, footer := newStub(50, 20), newStub(50, 30)
	scroll := NewScroll(newStub(50, 200))
	col := NewBox(Column, 0, 0)
	col.Append(header, false)
	col.Append(scroll, true)
	col.Append(footer, false)
	layoutColumn(col, 60, 150)
	if got := scroll.Bounds().H; got != 100 {
		t.Errorf("scroll height = %d, want 100 (150 less the 50 around it)", got)
	}
	if b := footer.Bounds(); b.Y+b.H != 150 {
		t.Errorf("footer ends at %d, want inside the 150 box", b.Y+b.H)
	}

	// Never below the floor: past it the column overflows as before.
	layoutColumn(col, 60, 60)
	if got := scroll.Bounds().H; got != scrollFloor {
		t.Errorf("squeezed scroll = %d, want its floor %d", got, scrollFloor)
	}

	// Room to spare: natural plus the extra, as before.
	layoutColumn(col, 60, 400)
	if got := scroll.Bounds().H; got != 350 {
		t.Errorf("roomy scroll = %d, want 350", got)
	}
}

// TestEveryShrinkerGivesWay pins GTK's distribution: a column short
// of room takes the shortfall from every child that can give it, not
// only the expanding ones, each never below its floor.
func TestEveryShrinkerGivesWay(t *testing.T) {
	fixed := NewScroll(newStub(50, 100))
	plain := newStub(50, 100)
	col := NewBox(Column, 0, 0)
	col.Append(fixed, false)
	col.Append(plain, true)
	layoutColumn(col, 60, 120)
	if fixed.Bounds().H >= 100 || plain.Bounds().H != 100 {
		t.Errorf("heights %d, %d: want the scroll short of natural and the stub untouched", fixed.Bounds().H, plain.Bounds().H)
	}
	if col.Shrinkable() == 0 {
		t.Error("shrinkable = 0, want the fixed scroll's capacity")
	}
}

// TestShrinkReachesANestedScroll pins the propagation: a stack of
// pages and the boxes inside it report what their scrolls can give,
// so the outer column shrinks the page that holds one.
func TestShrinkReachesANestedScroll(t *testing.T) {
	inner := NewScroll(newStub(50, 300))
	page := NewBox(Column, 0, 0)
	page.Append(newStub(50, 40), false)
	page.Append(inner, true)
	other := newStub(50, 80)
	stack := NewStack()
	stack.Add("form", page)
	stack.Add("list", other)
	outer := NewBox(Column, 0, 0)
	outer.Append(newStub(50, 20), false)
	outer.Append(stack, true)
	layoutColumn(outer, 60, 200)
	if b := stack.Bounds(); b.H != 180 {
		t.Errorf("stack height = %d, want 180", b.H)
	}
	if got := inner.Bounds().H; got != 140 {
		t.Errorf("nested scroll = %d, want 140 (the page's 180 less its header)", got)
	}
	// The stack shrinks only as far as its least shrinkable page.
	stack.Measure(Constraints{Max: Size{W: 60, H: 1000}})
	if got, want := stack.Shrinkable(), 340-max(scrollFloor+40, 80); got != want {
		t.Errorf("stack shrinkable = %d, want %d", got, want)
	}
	row := NewBox(Row, 0, 0)
	row.Append(NewScroll(newStub(10, 100)), false)
	row.Append(newStub(10, 60), false)
	row.Measure(Constraints{Max: Size{W: 100, H: 500}})
	if got := row.Shrinkable(); got != 40 {
		t.Errorf("row shrinkable = %d, want 40 (down to its 60 child)", got)
	}
}

// TestShrinkSplitsByCapacity pins the split: two scrolls give in
// proportion to what each can, and the rounding lands somewhere.
func TestShrinkSplitsByCapacity(t *testing.T) {
	a := NewScroll(newStub(50, 132)) // can give 100
	b := NewScroll(newStub(50, 232)) // can give 200
	col := NewBox(Column, 0, 0)
	col.Append(a, true)
	col.Append(b, true)
	layoutColumn(col, 60, 232)
	if a.Bounds().H+b.Bounds().H != 232 {
		t.Errorf("total = %d, want the box's 232", a.Bounds().H+b.Bounds().H)
	}
	if ga, gb := 132-a.Bounds().H, 232-b.Bounds().H; ga != 44 || gb != 88 {
		t.Errorf("gave %d and %d, want 44 and 88, in proportion to 100 and 200", ga, gb)
	}
}
