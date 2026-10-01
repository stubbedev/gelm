package popup

import (
	"testing"

	"github.com/neurlang/wayland/xdg"

	"github.com/stubbedev/gelm/render"
)

// A point anchors as a 1x1 rect the popup grows down-right of, free to
// slide on both axes and flip up (menus at the pointer, tooltips).
func TestPlacementForAPoint(t *testing.T) {
	p := placementFor(Config{X: 40, Y: 60})
	want := placement{
		rect:   render.Rect{X: 40, Y: 60, W: 1, H: 1},
		anchor: xdg.PositionerAnchorTopLeft, gravity: xdg.PositionerGravityBottomRight,
		adjust: xdg.PositionerConstraintAdjustmentSlideX | xdg.PositionerConstraintAdjustmentSlideY | xdg.PositionerConstraintAdjustmentFlipY,
	}
	if p != want {
		t.Errorf("point = %+v, want %+v", p, want)
	}
}

// A rect anchors edge-aligned on the gravity side and may flip across
// the anchor on that axis (a top bar's dropdown with no room below
// opens above, never over the bar) and slide along the other; the
// gutter offset keeps the content edge on the anchor edge.
func TestPlacementForARect(t *testing.T) {
	rect := render.Rect{X: 400, Y: 6, W: 180, H: 42}
	const g = 10
	flipY := uint32(xdg.PositionerConstraintAdjustmentSlideX | xdg.PositionerConstraintAdjustmentFlipY)
	flipX := uint32(xdg.PositionerConstraintAdjustmentSlideY | xdg.PositionerConstraintAdjustmentFlipX)
	for _, tc := range []struct {
		gravity         Gravity
		anchor, grow    uint32
		adjust          uint32
		offsetX, offset int32
	}{
		{GravityBottom, xdg.PositionerAnchorBottomLeft, xdg.PositionerGravityBottomRight, flipY, -g, -g},
		{GravityTop, xdg.PositionerAnchorTopLeft, xdg.PositionerGravityTopRight, flipY, -g, g},
		{GravityRight, xdg.PositionerAnchorTopRight, xdg.PositionerGravityBottomRight, flipX, -g, -g},
		{GravityLeft, xdg.PositionerAnchorTopLeft, xdg.PositionerGravityBottomLeft, flipX, g, -g},
	} {
		p := placementFor(Config{AnchorRect: rect, Gravity: tc.gravity, Gutter: g, X: 999, Y: 999})
		want := placement{rect: rect, anchor: tc.anchor, gravity: tc.grow, adjust: tc.adjust, offsetX: tc.offsetX, offsetY: tc.offset}
		if p != want {
			t.Errorf("gravity %d = %+v, want %+v", tc.gravity, p, want)
		}
	}
	// No gutter, no offset.
	if p := placementFor(Config{AnchorRect: rect}); p.offsetX != 0 || p.offsetY != 0 {
		t.Errorf("gutterless offset = %d,%d", p.offsetX, p.offsetY)
	}
}
