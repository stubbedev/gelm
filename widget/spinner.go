package widget

import (
	"math"
	"time"

	"github.com/stubbedev/gelm/internal/anim"
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
	size     int
	spinning bool
	// angle is the arc's leading edge in radians, wrapped to [0, 2π).
	angle  float64
	cancel anim.Cancel
}

// NewSpinner returns a spinner sized size×size pixels.
func NewSpinner(size int) *Spinner { return &Spinner{size: size} }

// Spinning reports whether the rotation is on.
func (s *Spinner) Spinning() bool { return s.spinning }

// SetSpinning turns the rotation on or off. Stopping cancels the tween
// at once — the frozen angle stays where it stopped and no wake
// remains scheduled. Starting takes effect once the spinner is
// arranged into a visible rect; until then nothing is scheduled.
func (s *Spinner) SetSpinning(on bool) {
	if on == s.spinning {
		return
	}
	s.spinning = on
	if on {
		s.maybeStart()
		return
	}
	s.stop()
}

// start launches one revolution; the landing tick relaunches while
// the spinner is still spinning and on screen (anim callbacks may
// launch — Tick runs them with its lock released). A stopped or
// hidden spinner relaunches nothing and holds no timer.
func (s *Spinner) start() {
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel = anim.Play(anim.Animate(spinnerPeriod, func(t float64) {
		if t >= 1 {
			s.angle = 0
			if s.spinning && !s.bounds.Empty() {
				s.start()
			}
			return
		}
		s.angle = 2 * math.Pi * t
		s.Invalidate()
	}).Easing(anim.Linear))
}

// stop cancels the rotation; a stopped spinner holds no timer.
func (s *Spinner) stop() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// maybeStart starts the rotation only when a visible rect says the
// spinner is on screen.
func (s *Spinner) maybeStart() {
	if s.bounds.Empty() {
		return
	}
	s.start()
}

// Arrange records the rect and syncs the tween with visibility: going
// off screen stops (nothing animates while hidden), coming back
// resumes when spinning. The rect does not move frame to frame, so a
// no-change Arrange never restarts the rotation — only the empty/
// non-empty transition does.
func (s *Spinner) Arrange(r render.Rect) {
	s.node.Arrange(r)
	switch {
	case r.Empty():
		s.stop()
	case s.spinning && s.cancel == nil:
		s.start()
	}
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
