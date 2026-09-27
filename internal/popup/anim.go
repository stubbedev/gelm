// The enter/exit visual half of a popup: a slide-and-fade wrapper the
// coordinator drives. The fade lives in widget.Fader (#38's subtree
// opacity), the slide in a tiny offset wrapper that keeps the motion
// inside the popup's own bounds — the toast's masked-slide trick — so
// tween frames never paint outside the surface's damage rect.
package popup

import (
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// slideDist is how far the content travels toward its anchor over the
// enter (and back over the exit), in logical pixels. Small on purpose:
// the motion reads as a grow-from-anchor without the content ever
// leaving the popup's surface.
const slideDist = 12

// animView is the popup's animated paint wrapper: fader under a slide,
// built around the content root Run was handed.
type animView struct {
	slide *animRoot
	fader *widget.Fader
}

// newAnimView wraps root for tween painting. reveal seeds the wrapper's
// state — the coordinator may already hold a progress from an Enter
// that ran before the content existed (zero-duration enters land
// inside New).
func newAnimView(root widget.Widget, reveal float64, g Gravity) *animView {
	f := widget.NewFader(root)
	f.SetOpacity(reveal)
	dx, dy := slideVec(g)
	return &animView{
		fader: f,
		slide: &animRoot{child: f, reveal: reveal, dx: dx, dy: dy},
	}
}

// slideVec picks the slide direction for a gravity: content travels
// from its anchor edge toward rest, so a bottom gravity slides down
// into place (starts shifted up), a top gravity slides up, and so on.
func slideVec(g Gravity) (int, int) {
	switch g {
	case GravityTop:
		return 0, slideDist
	case GravityLeft:
		return slideDist, 0
	case GravityRight:
		return -slideDist, 0
	default: // GravityBottom: the anchor is above, so rise from it
		return 0, -slideDist
	}
}

// setProgress applies one tween frame: opacity through the fader, the
// offset through the slide wrapper.
func (a *animView) setProgress(reveal float64) {
	a.fader.SetOpacity(reveal)
	a.slide.reveal = reveal
}

// animRoot is the offset half: a hand-rolled widget (internal packages
// cannot embed widget.node) that arranges its child shifted toward the
// anchor by the unrevealed fraction of slideDist and clips the paint to
// its own bounds, so the shift never spills outside the surface.
type animRoot struct {
	child  widget.Widget
	bounds render.Rect
	reveal float64
	dx, dy int
}

// Measure passes through: the popup is sized from the content, not the
// animation.
func (a *animRoot) Measure(con widget.Constraints) widget.Size { return a.child.Measure(con) }

// Arrange shifts the child's rect by the unrevealed slide; at rest the
// rect is exact.
func (a *animRoot) Arrange(r render.Rect) {
	a.bounds = r
	dy := int((1 - a.reveal) * float64(a.dy))
	dx := int((1 - a.reveal) * float64(a.dx))
	a.child.Arrange(render.Rect{X: r.X + dx, Y: r.Y + dy, W: r.W, H: r.H})
}

// Paint clips to the popup's bounds and paints the (shifted, faded)
// child.
func (a *animRoot) Paint(cv *render.Canvas) {
	if a.reveal >= 1 {
		a.child.Paint(cv)
		return
	}
	prev := cv.PushClip(a.bounds)
	a.child.Paint(cv)
	cv.PopClip(prev)
}

// HitTest resolves through the child: a mid-enter popup is as
// clickable as one at rest (the visual is the only thing animating).
func (a *animRoot) HitTest(p widget.Point) widget.Widget { return a.child.HitTest(p) }
