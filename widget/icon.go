package widget

import (
	"github.com/stubbedev/gelm/render"
)

// Icon paints a pre-rasterized icon at its natural size.
type Icon struct {
	node
	ic *render.Icon
}

// NewIcon returns an icon widget around a rasterized icon.
func NewIcon(ic *render.Icon) *Icon {
	return &Icon{ic: ic}
}

// Measure returns the icon's pixel size, clamped to con.
func (i *Icon) Measure(con Constraints) Size {
	w, h := i.ic.Size()
	return clampSize(Size{W: w, H: h}, con)
}

// Paint draws the icon at the top-left of the arranged rect.
func (i *Icon) Paint(cv *render.Canvas) {
	i.ic.Draw(cv, i.bounds.X, i.bounds.Y)
}

// HitTest returns the icon when p is inside its bounds.
func (i *Icon) HitTest(p Point) Widget {
	return i.HitLeaf(i, p)
}
