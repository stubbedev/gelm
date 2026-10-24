package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/lys-ui/anim"
	"github.com/stubbedev/lys-ui/internal/animclock"
	"github.com/stubbedev/lys-ui/render"
)

// motionScene is a column of 20px swatches over a 100x120 canvas, with
// a pinned animation clock the test steps.
type motionScene struct {
	root *Box
	data []byte
	cv   *render.Canvas
	now  time.Time
}

func newMotionScene(t *testing.T) *motionScene {
	t.Helper()
	loadCSS(t, `.sw { min-width: 100px; min-height: 20px; background-color: #ff0000; } .new { background-color: #0000ff; } .moving { -lys-layout-motion: smooth; }`)
	s := &motionScene{root: NewBox(Column, 0, 0), now: time.Unix(1750000000, 0)}
	t.Cleanup(animclock.SetClock(func() time.Time { return s.now }))
	animclock.Reset()
	t.Cleanup(animclock.Reset)
	s.data = make([]byte, render.Stride(100)*120)
	s.cv = render.New(s.data, render.Stride(100), 100, 120)
	return s
}

func swatch(classes ...string) *Box {
	b := NewBox(Column, 0, 0)
	b.AddClass(append([]string{"sw"}, classes...)...)
	return b
}

func (s *motionScene) frame() {
	s.root.Measure(Constraints{Max: Size{W: 100, H: 120}})
	s.root.Arrange(render.Rect{W: 100, H: 120})
	CollectDamage(s.root)
	s.cv.Clear(s.cv.Rect(), 0)
	s.root.Paint(s.cv)
}

func (s *motionScene) step() {
	s.now = s.now.Add(animclock.FrameInterval)
	animclock.Tick(s.now)
}

func (s *motionScene) red(y int) bool {
	c := render.ColorFromBytes(s.data[y*render.Stride(100)+50*4:])
	return c.A() > 0 && c.R() > 200 && c.B() < 50
}

// A widget the layout moves springs from where it was shown while
// hit-testing at its destination at once; it lands exactly there (#201).
func TestLayoutMotionSpringsToTheNewRect(t *testing.T) {
	s := newMotionScene(t)
	mover := swatch("moving")
	s.root.Append(mover, false)
	s.frame()
	if !s.red(10) {
		t.Fatal("the swatch did not paint at its first rect")
	}
	s.root.InsertAt(0, swatch("new"), false)
	s.frame()
	if !s.red(10) || s.red(30) {
		t.Errorf("first frame after the move: red at old %v, new %v; want still shown at the old rect", s.red(10), s.red(30))
	}
	if s.root.HitTest(Point{X: 50, Y: 30}) != Widget(mover) {
		t.Error("the moving swatch does not hit-test at its destination")
	}
	for range 120 {
		s.step()
		s.frame()
	}
	if s.red(10) || !s.red(30) {
		t.Errorf("settled: red at old %v, new %v; want it at the new rect", s.red(10), s.red(30))
	}
}

// A widget inserted into a shown parent grows in from no height.
func TestLayoutMotionInsertionGrowsIn(t *testing.T) {
	s := newMotionScene(t)
	s.root.Append(swatch(), false)
	s.frame()
	s.root.Append(swatch("moving"), false)
	s.frame()
	if s.red(30) {
		t.Error("the inserted swatch showed whole on its first frame")
	}
	for range 120 {
		s.step()
		s.frame()
	}
	if !s.red(30) {
		t.Error("the inserted swatch never grew in")
	}
}

// Without a motion, or under reduced motion, a move lands at once; the
// stylesheet outranks the programmatic motion.
func TestLayoutMotionOffOrInstant(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(t *testing.T, w *Box)
	}{
		"no motion": {func(*testing.T, *Box) {}},
		"instant": {func(t *testing.T, w *Box) {
			w.AddClass("moving")
			t.Cleanup(anim.SetInstant(true))
		}},
		"stylesheet none over programmatic": {func(t *testing.T, w *Box) {
			w.SetLayoutMotion(LayoutMotionBouncy)
			w.SetInlineStyle("-lys-layout-motion: none;")
		}},
	} {
		t.Run(name, func(t *testing.T) {
			s := newMotionScene(t)
			w := swatch()
			tc.setup(t, w)
			s.root.Append(w, false)
			s.frame()
			s.root.InsertAt(0, swatch("new"), false)
			s.frame()
			if s.red(10) || !s.red(30) {
				t.Errorf("red at old %v, new %v; want it at the new rect at once", s.red(10), s.red(30))
			}
		})
	}
}
