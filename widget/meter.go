package widget

import (
	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// meter is the trough-and-fill core ProgressBar and LevelBar share:
// GTK's `<bar> > trough > <fill>` node tree, a value in [0, 1], a
// 160x10 trough floored by the trough's min sizes, and the paint of
// the trough with the fill over a span of it - the value from the
// start, or wherever the embedder's span says (a pulse block).
type meter struct {
	node
	value  float64
	trough meterTrough
	// troughFill and fillFill color the parts the cascade gives no
	// background; nil leaves them cascade-only.
	troughFill, fillFill func(*Theme) Color
	// roundTrough defaults the trough's radius to half its height.
	roundTrough bool
}

// meterTrough is the trough node and its fill.
type meterTrough struct {
	stylePart
	fill stylePart
}

func (t *meterTrough) styleChildren() []Widget { return []Widget{&t.fill} }

// initMeter names the nodes and links them under self.
func (m *meter) initMeter(self Widget, value float64, fill string) {
	m.value = math01(value)
	m.trough.SetElement("trough")
	m.trough.fill.SetElement(fill)
	setParents(self, &m.trough)
	setParents(&m.trough, &m.trough.fill)
}

// styleChildren is the trough (styleKids).
func (m *meter) styleChildren() []Widget { return []Widget{&m.trough} }

// Value returns the fill fraction in [0, 1].
func (m *meter) Value() float64 { return m.value }

// setValue clamps v to [0, 1] and reports whether it changed,
// invalidating the bar if so.
func (m *meter) setValue(v float64) bool {
	v = math01(v)
	if v == m.value {
		return false
	}
	m.value = v
	m.Invalidate()
	return true
}

// measureMeter wants the 160x10 trough, floored by the trough's min
// sizes, inside self's CSS box, clamped to con.
func (m *meter) measureMeter(self Widget, con Constraints) Size {
	if sz, ok := m.measureHit(con); ok {
		return sz
	}
	tv := m.trough.style(&m.trough)
	w, h := picki(tv, style.PropMinWidth, 160), picki(tv, style.PropMinHeight, 10)
	v := m.style(self)
	return m.measureStore(con, measureBox(v, boxOf(v, render.Insets{}), con, func(inner Constraints) Size {
		return clampSize(Size{W: w, H: h}, inner)
	}))
}

// Arrange records the bar's box; the trough's border box is it.
func (m *meter) Arrange(r render.Rect) {
	m.node.Arrange(r)
	m.trough.Arrange(r)
}

// paintMeter draws the trough, then the fill over the [from, to)
// fraction of the trough's content box, each from its cascade.
func (m *meter) paintMeter(cv *render.Canvas, from, to float64) {
	t := Current()
	tv := m.trough.style(&m.trough)
	fx := pushEffects(cv, tv)
	defer fx.pop(cv)
	tr := 0
	if m.roundTrough {
		tr = m.bounds.H / 2
	}
	_, inner := boxRects(boxOf(tv, render.Insets{}), m.bounds)
	paintBoxBehind(cv, tv, m.bounds, radiusOr(tv, tr), borderOf(tv), pickc(0, tv, style.PropBackgroundColor, fallback(m.troughFill, t)))
	x0, x1 := int(float64(inner.W)*from), int(float64(inner.W)*to)
	if x1 <= x0 {
		return
	}
	fr := render.Rect{X: inner.X + x0, Y: inner.Y, W: x1 - x0, H: inner.H}
	m.trough.fill.Arrange(fr)
	fv := m.trough.fill.style(&m.trough.fill)
	fxp := pushEffects(cv, fv)
	paintBoxBehind(cv, fv, fr, radiusOr(fv, inner.H/2), borderOf(fv), pickc(0, fv, style.PropBackgroundColor, fallback(m.fillFill, t)))
	fxp.pop(cv)
}

// fallback resolves an optional themed color.
func fallback(fn func(*Theme) Color, t *Theme) Color {
	if fn == nil {
		return 0
	}
	return fn(t)
}

// HitTest is the leaf hit: the whole bar.
func (m *meter) hitMeter(self Widget, p Point) Widget { return m.HitLeaf(self, p) }

func math01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
