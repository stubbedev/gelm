// The popup's tween paint wrapper: enter's first frame paints nothing
// (the zero-alpha skip proof), tween frames paint ink at partial
// reveal, completion rests exactly on the content rect, and the slide
// shifts toward the anchor without ever leaving the surface bounds.
package popup

import (
	"testing"
	"time"

	"github.com/neurlang/wayland/xdg"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/internal/surfx"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// inkProbe is a leaf that fills its bounds — the paint-count hook.
// (Internal packages cannot embed widget.node, so this is hand-rolled.)
type inkProbe struct {
	bounds render.Rect
	paints int
}

func (p *inkProbe) Measure(con widget.Constraints) widget.Size {
	return widget.Size{W: min(60, con.Max.W), H: min(20, con.Max.H)}
}

func (p *inkProbe) Arrange(r render.Rect) { p.bounds = r }

func (p *inkProbe) Paint(cv *render.Canvas) {
	p.paints++
	cv.FillRect(p.bounds, render.RGB(200, 100, 0))
}

func (p *inkProbe) HitTest(widget.Point) widget.Widget { return p }

// newTweenView builds the anim view over an ink probe at (0,0) sized
// 60x20, laid out the way Run lays the popup root out.
func newTweenView(t *testing.T, reveal float64, g Gravity) (*animView, *inkProbe) {
	t.Helper()
	probe := &inkProbe{}
	v := newAnimView(probe, reveal, g)
	v.slide.Measure(widget.Constraints{Max: widget.Size{W: 100, H: 100}})
	v.slide.Arrange(render.Rect{X: 0, Y: 0, W: 60, H: 20})
	return v, probe
}

func TestAnimViewFirstFramePaintsNothing(t *testing.T) {
	v, probe := newTweenView(t, 0, GravityBottom)
	cv := render.NewScaled(make([]byte, render.Stride(64)*24), render.Stride(64), 64, 24, 1, 120)
	before := cv.Touched()
	v.slide.Paint(cv)
	if probe.paints != 0 || cv.Touched() != before {
		t.Errorf("enter's first frame painted: paints = %d touched delta = %d, want nothing written",
			probe.paints, cv.Touched()-before)
	}
	// And the same holds for a recycled buffer that already holds the
	// last opaque frame: Run raw-clears before the faded paint.
	v.setProgress(1)
	cv.ClearDevice(cv.Rect(), render.Color(0)) // raw clear, uncounted here
	v.slide.Paint(cv)
	if cv.Touched() == before {
		t.Fatal("rest state painted nothing; the probe lost ink")
	}
	opaque := cv.Touched()
	before = cv.Touched()
	v.setProgress(0.25)
	v.slide.Paint(cv)
	if cv.Touched() <= before || cv.Touched() < opaque/2 {
		t.Errorf("mid-exit frame painted no ink (touched %d, before %d)", cv.Touched(), before)
	}
}

func TestAnimViewRestMatchesContentRect(t *testing.T) {
	_, probe := newTweenView(t, 1, GravityBottom)
	if probe.bounds != (render.Rect{X: 0, Y: 0, W: 60, H: 20}) {
		t.Errorf("rest bounds = %v, want the exact content rect", probe.bounds)
	}
	// GravityBottom grows from the top edge: hidden content sits above
	// its rest rect (shifted toward the anchor), clipped at paint time.
	_, probe2 := newTweenView(t, 0, GravityBottom)
	if probe2.bounds.Y >= 0 {
		t.Errorf("hidden gravity-bottom content at y=%d, want shifted up toward the anchor", probe2.bounds.Y)
	}
	// GravityTop is the mirror: hidden content sits below its rest.
	_, probe3 := newTweenView(t, 0, GravityTop)
	if probe3.bounds.Y <= 0 {
		t.Errorf("hidden gravity-top content at y=%d, want shifted down", probe3.bounds.Y)
	}
}

// wireFreePopup is a Popup whose wire objects are nil: the state
// machine runs (sealing, dismissal flags, callbacks) while the wire
// requests no-op. The animation clock is pinned, so tween drives are
// exact and the exit keeps the surface mapped until it lands.
func wireFreePopup(t *testing.T, kind surfx.Kind) (*Popup, func()) {
	t.Helper()
	base := time.Unix(1750000000, 0)
	t.Cleanup(anim.SetClock(func() time.Time { return base }))
	anim.Reset()
	t.Cleanup(anim.Reset)
	now := base
	drive := func() {
		for i := 0; ; i++ {
			wake, ok := anim.Next()
			if !ok {
				return
			}
			if i > 1000 {
				panic("schedule did not drain")
			}
			if wake.After(now) {
				now = wake
			}
			anim.Tick(now)
		}
	}
	p := &Popup{}
	p.fx = surfx.NewCoordinator(kind, p, nil)
	p.fx.Enter()
	drive()
	return p, drive
}

// TestPopupDoneRoutesThroughDismiss pins the no-fast-path rule: the
// outside-click dismissal (popup_done) enters the same state machine
// as Esc and programmatic Dismiss — the logical state flips, callbacks
// fire once, and repeated popup_done mid-exit is a no-op.
func TestPopupDoneRoutesThroughDismiss(t *testing.T) {
	p, drive := wireFreePopup(t, surfx.KindMenu)
	p.HandlePopupPopupDone(xdg.PopupPopupDoneEvent{})
	if !p.Dismissed() {
		t.Fatal("popup_done did not dismiss the popup")
	}
	// The surface stays mapped for the exit tween: destroyed only after
	// the last frame lands — never on the spot (that was the old fast
	// path this machine replaces).
	if p.Destroyed() {
		t.Fatal("popup_done destroyed the surface immediately; no exit window")
	}
	drive()
	if !p.Destroyed() {
		t.Error("the exit tween landed but the wire teardown never ran")
	}
	// A popup_done again mid-exit (or after) must be a no-op.
	closed := 0
	p2, drive2 := wireFreePopup(t, surfx.KindMenu)
	p2.SetOnClosed(func() { closed++ })
	p2.HandlePopupPopupDone(xdg.PopupPopupDoneEvent{})
	p2.HandlePopupPopupDone(xdg.PopupPopupDoneEvent{})
	drive2()
	if closed != 1 {
		t.Errorf("OnClosed fired %d times for repeated popup_done, want exactly 1", closed)
	}
}

// TestInputDeadDuringExit pins the click-through contract on the client
// side: after Dismiss the surface's input handlers go inert, so a
// dying popup's tree takes no presses, no hover, and schedules no
// repaints — beside the empty input region that stops the compositor
// from routing events here in the first place.
func TestInputDeadDuringExit(t *testing.T) {
	p, _ := wireFreePopup(t, surfx.KindMenu)
	p.fx.Enter()

	probe := &inkProbe{}
	router := &widget.Router{Root: probe}
	dirties := 0
	input := &popupInput{
		dismissed: p.Dismissed,
		router:    router,
		pointer:   &struct{ x, y float64 }{},
		markDirty: func() { dirties++ },
	}
	input.HandlePointerEnter(5, 5)
	input.HandlePointerButton(0x110, 1, 7)
	if dirties != 2 || router.Hovered() == nil {
		t.Fatalf("sanity: live popup dropped input (dirties = %d hovered = %v)", dirties, router.Hovered())
	}

	p.Dismiss()
	input.HandlePointerEnter(6, 6)
	input.HandlePointerButton(0x110, 1, 8)
	input.HandlePointerLeave()
	if dirties != 2 {
		t.Errorf("dismissed popup took input: dirties = %d, want none extra", dirties)
	}
}

// TestPaintPlateShadowGutter pins the popup plate: with a gutter, the
// theme's elevation paints around the content rect, the plate rounds
// inside it, and the gutter's far corner stays transparent — the
// falloff must blend over whatever the popup floats above. Without a
// gutter the plate is the legacy whole-surface fill.
func TestPaintPlateShadowGutter(t *testing.T) {
	defer widget.SetTheme(widget.DarkTheme())
	widget.SetTheme(widget.DarkTheme().WithShadowBlur(6))
	bg := render.RGB(30, 30, 40)

	at := func(data []byte, stride, x, y int) render.Color {
		return render.ColorFromBytes(data[y*stride+x*4:])
	}

	t.Run("gutter carries the shadow, corner stays transparent", func(t *testing.T) {
		pc := &Painter{bg: bg, gutter: 6}
		data := make([]byte, render.Stride(72)*52)
		pc.paintPlate(render.New(data, render.Stride(72), 72, 52), render.Rect{X: 6, Y: 6, W: 60, H: 40})

		if got := at(data, render.Stride(72), 0, 0); got != 0 {
			t.Errorf("gutter corner = %#08x, want untouched transparent", uint32(got))
		}
		if got := at(data, render.Stride(72), 2, 26); got.A() == 0 {
			t.Error("no falloff 3px into the gutter; the plate painted no elevation")
		}
		if got := at(data, render.Stride(72), 36, 26); got != bg {
			t.Errorf("plate center = %#08x, want the popup background %#08x", uint32(got), uint32(bg))
		}
		if got := at(data, render.Stride(72), 7, 7); got == bg {
			t.Error("cut plate corner painted plate color; the plate must round with the shadow")
		}
	})

	t.Run("no gutter fills the surface as before", func(t *testing.T) {
		pc := &Painter{bg: bg}
		w, h := 40, 30
		data := make([]byte, render.Stride(w)*h)
		pc.paintPlate(render.New(data, render.Stride(w), w, h), render.Rect{X: 0, Y: 0, W: w, H: h})

		for _, p := range [][2]int{{0, 0}, {39, 29}, {20, 15}} {
			if got := at(data, render.Stride(w), p[0], p[1]); got != bg {
				t.Errorf("plate at (%d,%d) = %#08x, want the background", p[0], p[1], uint32(got))
			}
		}
	})
}
