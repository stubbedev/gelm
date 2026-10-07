package widget

import (
	"math"
	"time"

	"github.com/stubbedev/gelm/render"
)

const (
	// spinnerPeriod is the time per full revolution.
	spinnerPeriod = time.Second
	// spinnerArc paints the leading quarter-turn; the muted ring shows
	// the track underneath.
	spinnerArc   = math.Pi / 2
	spinnerWidth = 2
	// spinnerStep is the chord length of the painted arc segments.
	spinnerStep = math.Pi / 18
)

// Spinner is an indeterminate activity indicator: a rotating arc on a
// muted ring. The rotation is animation-clock driven — one frame
// invalidation per step, and no timer at all unless the spinner is
// both spinning and visible. A hidden or stopped spinner schedules no
// wake, so an idle app with a parked spinner costs nothing.
type Spinner struct {
	node
	size int
	// angle is the arc's leading edge in radians, wrapped to [0, 2π).
	angle float64
	spin  loop
}

// NewSpinner returns a spinner sized size×size pixels.
func NewSpinner(size int) *Spinner {
	s := &Spinner{size: size}
	s.spin = loop{period: spinnerPeriod, step: func(t float64) {
		s.angle = 2 * math.Pi * t
		s.Invalidate()
	}}
	return s
}

// Spinning reports whether the rotation is on.
func (s *Spinner) Spinning() bool { return s.spin.on }

// SetSpinning turns the rotation on or off. Stopping cancels the tween
// at once — the frozen angle stays where it stopped and no wake
// remains scheduled. Starting takes effect once the spinner is
// arranged into a visible rect; until then nothing is scheduled.
func (s *Spinner) SetSpinning(on bool) { s.spin.set(on) }

// Arrange records the rect and syncs the rotation with visibility.
func (s *Spinner) Arrange(r render.Rect) {
	s.node.Arrange(r)
	s.spin.arranged(r)
}

// Measure wants a size×size square, clamped to con.
func (s *Spinner) Measure(con Constraints) Size {
	if sz, ok := s.measureHit(con); ok {
		return sz
	}
	return s.measureStore(con, clampSize(Size{W: s.size, H: s.size}, con))
}

// Paint draws the muted track ring and the leading arc at the current
// angle.
func (s *Spinner) Paint(cv *render.Canvas) {
	b := s.bounds
	if b.Empty() {
		return
	}
	th := Current()
	radius := float64(min(b.W, b.H))/2 - spinnerWidth
	if radius <= 0 {
		return
	}
	cx := float64(b.X + b.W/2)
	cy := float64(b.Y + b.H/2)
	strokeArc(cv, cx, cy, radius, 0, 2*math.Pi, spinnerWidth, scaleAlpha(th.TextMuted, 0.35))
	strokeArc(cv, cx, cy, radius, s.angle, s.angle+spinnerArc, spinnerWidth, th.Accent)
}

// strokeArc paints an arc as short chords; the canvas has no curve
// primitive and at spinner sizes the segments read as a smooth ring.
func strokeArc(cv *render.Canvas, cx, cy, radius, from, to float64, width int, col render.Color) {
	pt := func(a float64) (int, int) {
		return int(cx + radius*math.Cos(a) + 0.5), int(cy + radius*math.Sin(a) + 0.5)
	}
	x0, y0 := pt(from)
	for a := from + spinnerStep; a < to; a += spinnerStep {
		x1, y1 := pt(a)
		cv.Line(x0, y0, x1, y1, width, col)
		x0, y0 = x1, y1
	}
	x1, y1 := pt(to)
	cv.Line(x0, y0, x1, y1, width, col)
}

// HitTest returns the spinner when p is inside its bounds.
func (s *Spinner) HitTest(p Point) Widget { return s.HitLeaf(s, p) }
