package widget

import (
	"testing"
)

// CenterBox centers the middle slot however wide the sides run: the
// start slot hugs the left edge, the end slot the right, the center
// slot sits at the allocation's midpoint.
func TestCenterBoxCenters(t *testing.T) {
	left := NewLabel(entryFace(t), 10, "left", 0)
	mid := NewBox(Row, 0, 0)
	mid.Append(NewLabel(entryFace(t), 10, "mid", 0), false)
	right := NewLabel(entryFace(t), 10, "right!", 0)
	cb := NewCenterBox(left, mid, right)
	arrangeTree(t, cb, 200, 30)
	lb, mb, rb := left.Bounds(), mid.Bounds(), right.Bounds()
	if lb.X != 0 {
		t.Errorf("start slot at %d, want the left edge", lb.X)
	}
	if rb.X+rb.W != 200 {
		t.Errorf("end slot ends at %d, want the right edge", rb.X+rb.W)
	}
	got := float64(mb.X) + float64(mb.W)/2
	if got < 99 || got > 101 {
		t.Errorf("center slot midpoint %v, want the allocation center", got)
	}
}

// Nil slots measure and arrange without incident.
func TestCenterBoxNilSlots(t *testing.T) {
	mid := NewLabel(entryFace(t), 10, "mid", 0)
	cb := NewCenterBox(nil, mid, nil)
	arrangeTree(t, cb, 80, 20)
	mb := mid.Bounds()
	if got := float64(mb.X) + float64(mb.W)/2; got < 39 || got > 41 {
		t.Errorf("center slot midpoint %v, want 40", got)
	}
	if cb.Measure(Constraints{Max: Size{W: 100, H: 40}}).W == 0 {
		t.Error("the box measured to nothing")
	}
}
