package widget

import (
	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// LevelBar is GTK's continuous level bar: the levelbar element over a
// trough and one filled block (levelbar > trough > block.filled), the
// node tree stylesheets target for gauges. Every part paints from its
// cascade — there is no programmatic color; a part the stylesheet
// gives no background to paints nothing.
type LevelBar struct {
	node
	value  float64
	trough levelTrough
}

// levelTrough is the bar's trough node and its filled block.
type levelTrough struct {
	stylePart
	block stylePart
}

func (t *levelTrough) styleChildren() []Widget { return []Widget{&t.block} }

// styleChildren is the trough (styleKids).
func (b *LevelBar) styleChildren() []Widget { return []Widget{&b.trough} }

// NewLevelBar returns a continuous level bar at value clamped to
// [0, 1].
func NewLevelBar(value float64) *LevelBar {
	b := &LevelBar{value: math01(value)}
	b.SetElement("levelbar")
	b.trough.SetElement("trough")
	b.trough.block.SetElement("block")
	b.trough.block.AddClass("filled")
	setParents(b, &b.trough)
	setParents(&b.trough, &b.trough.block)
	return b
}

// Value returns the fill fraction in [0, 1].
func (b *LevelBar) Value() float64 { return b.value }

// SetValue clamps v to [0, 1] and invalidates the bar so the next
// frame repaints the block.
func (b *LevelBar) SetValue(v float64) {
	v = math01(v)
	if v == b.value {
		return
	}
	b.value = v
	b.Invalidate()
}

// Measure wants a fixed 160x10 trough, floored by the trough's min
// sizes, inside the bar's CSS box, clamped to con.
func (b *LevelBar) Measure(con Constraints) Size {
	if sz, ok := b.measureHit(con); ok {
		return sz
	}
	tv := b.trough.style(&b.trough)
	w, h := picki(tv, style.PropMinWidth, 160), picki(tv, style.PropMinHeight, 10)
	v := b.style(b)
	return b.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		return clampSize(Size{W: w, H: h}, inner)
	}))
}

// Arrange records the bar's box; the trough's border box is it.
func (b *LevelBar) Arrange(r render.Rect) {
	b.node.Arrange(r)
	b.trough.Arrange(r)
}

// Paint draws the trough and the proportional block through their
// nodes, each from its cascade.
func (b *LevelBar) Paint(cv *render.Canvas) {
	tv := b.trough.style(&b.trough)
	fx := pushEffects(cv, tv)
	defer fx.pop(cv)
	radii := radiusOr(tv, 0)
	_, inner := boxRects(boxOf(tv, render.Insets{}), b.bounds)
	paintBoxBehind(cv, tv, b.bounds, radii, borderOf(tv), pickc(0, tv, style.PropBackgroundColor, 0))
	fw := int(float64(inner.W) * b.value)
	if fw > 0 {
		br := inner
		br.W = fw
		b.trough.block.Arrange(br)
		bv := b.trough.block.style(&b.trough.block)
		fxp := pushEffects(cv, bv)
		paintBoxBehind(cv, bv, br, radiusOr(bv, inner.H/2), borderOf(bv), pickc(0, bv, style.PropBackgroundColor, 0))
		fxp.pop(cv)
	}
}

// Role implements Roleer.
func (b *LevelBar) Role() Role { return RoleLevelBar }

// HitTest returns the bar when p is inside its bounds.
func (b *LevelBar) HitTest(pt Point) Widget {
	return b.HitLeaf(b, pt)
}

// Trough and Block expose the levelbar's parts (levelbar > trough >
// block.filled), the nodes a paint test reads or a host needs.
func (b *LevelBar) Trough() Widget { return &b.trough }
func (b *LevelBar) Block() Widget  { return &b.trough.block }
