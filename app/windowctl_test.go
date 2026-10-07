package app

import (
	"testing"

	"github.com/stubbedev/gelm/internal/window"
)

// BeginMove and BeginResize anchor the grab to the press under way;
// the requests without a wire are harmless.
func TestBeginMoveUsesThePressSerial(t *testing.T) {
	a := accelApp()
	w := &Window{app: a, win: &window.Window{}}
	var moved []uint32
	var resized [][2]uint32
	hw := &hostWindow{
		win:         w,
		input:       &surfaceInput{pressSerial: 77},
		startMove:   func(serial uint32) { moved = append(moved, serial) },
		startResize: func(edges, serial uint32) { resized = append(resized, [2]uint32{edges, serial}) },
	}
	a.windows = []*hostWindow{hw}
	w.BeginMove()
	w.BeginResize(EdgeBottomRight)
	if len(moved) != 1 || moved[0] != 77 {
		t.Errorf("move serials %v, want [77]", moved)
	}
	if len(resized) != 1 || resized[0] != [2]uint32{uint32(EdgeBottomRight), 77} {
		t.Errorf("resizes %v", resized)
	}
	w.FullscreenOn(nil)
	w.SetTransientFor(&Window{app: a, win: &window.Window{}})
	w.SetTransientFor(nil)
	(&Window{app: a}).BeginMove() // not in the loop: nothing
}
