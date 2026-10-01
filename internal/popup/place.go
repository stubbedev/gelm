package popup

import (
	"github.com/neurlang/wayland/xdg"

	"github.com/stubbedev/gelm/render"
)

// placement is one xdg_positioner setup: the anchor rect, which point
// of it the popup hangs from, the direction it grows, how the
// compositor may adjust it against the output, and the offset that
// keeps the shadow gutter outside the content's alignment.
type placement struct {
	rect             render.Rect
	anchor, gravity  uint32
	adjust           uint32
	offsetX, offsetY int32
}

// placementFor maps a Config onto the positioner. A point (menus,
// tooltips) is a 1x1 rect the popup grows down and right of, sliding
// on both axes and flipping up. A rect (popovers) is edge-aligned on
// the Gravity side and may flip across the anchor on that axis and
// slide along the other, so a bar's dropdown that has no room below
// the screen edge opens above the bar instead of over it.
func placementFor(cfg Config) placement {
	const (
		slideX = xdg.PositionerConstraintAdjustmentSlideX
		slideY = xdg.PositionerConstraintAdjustmentSlideY
		flipX  = xdg.PositionerConstraintAdjustmentFlipX
		flipY  = xdg.PositionerConstraintAdjustmentFlipY
	)
	if cfg.AnchorRect.Empty() {
		return placement{
			rect:    render.Rect{X: cfg.X, Y: cfg.Y, W: 1, H: 1},
			anchor:  xdg.PositionerAnchorTopLeft,
			gravity: xdg.PositionerGravityBottomRight,
			adjust:  slideX | slideY | flipY,
		}
	}
	g := int32(cfg.Gutter)
	p := placement{rect: cfg.AnchorRect}
	switch cfg.Gravity {
	case GravityTop:
		p.anchor, p.gravity, p.adjust = xdg.PositionerAnchorTopLeft, xdg.PositionerGravityTopRight, slideX|flipY
		p.offsetX, p.offsetY = -g, g
	case GravityRight:
		p.anchor, p.gravity, p.adjust = xdg.PositionerAnchorTopRight, xdg.PositionerGravityBottomRight, slideY|flipX
		p.offsetX, p.offsetY = -g, -g
	case GravityLeft:
		p.anchor, p.gravity, p.adjust = xdg.PositionerAnchorTopLeft, xdg.PositionerGravityBottomLeft, slideY|flipX
		p.offsetX, p.offsetY = g, -g
	default:
		p.anchor, p.gravity, p.adjust = xdg.PositionerAnchorBottomLeft, xdg.PositionerGravityBottomRight, slideX|flipY
		p.offsetX, p.offsetY = -g, -g
	}
	return p
}

// place applies the placement to a positioner.
func place(pos *xdg.Positioner, cfg Config) error {
	p := placementFor(cfg)
	r := p.rect
	if err := pos.SetAnchorRect(int32(r.X), int32(r.Y), int32(r.W), int32(r.H)); err != nil {
		return err
	}
	if err := pos.SetAnchor(p.anchor); err != nil {
		return err
	}
	if err := pos.SetGravity(p.gravity); err != nil {
		return err
	}
	if err := pos.SetConstraintAdjustment(p.adjust); err != nil {
		return err
	}
	if p.offsetX != 0 || p.offsetY != 0 {
		return pos.SetOffset(p.offsetX, p.offsetY)
	}
	return nil
}
