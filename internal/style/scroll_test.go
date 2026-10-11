package style

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// scrollRun computes css against one box and returns its values.
func scrollRun(t *testing.T, css string) Values {
	t.Helper()
	var sc Scratch
	s := parseOne(t, css)
	var v Values
	Compute([]Layer{{Sheet: s}}, el("box"), nil, nil, Env{}, &sc, &v)
	return v
}

// The scroll-container properties compute: scroll-behavior, the
// scroll-padding sides, scroll-snap-type's axes and strictness,
// overscroll-behavior's axis pair, and the scrollbar trio.
func TestScrollContainerProps(t *testing.T) {
	v := scrollRun(t, `box {
		scroll-behavior: smooth;
		scroll-padding: 1px 2px 3px;
		scroll-snap-type: y mandatory;
		overscroll-behavior: contain none;
		scrollbar-color: #ff0000 #0000ff;
		scrollbar-width: thin;
	}`)
	if !v.ScrollSmooth {
		t.Error("scroll-behavior: smooth did not compute")
	}
	if want := (Sides{Top: 1, Right: 2, Bottom: 3, Left: 2}); v.ScrollPad != want {
		t.Errorf("scroll-padding = %v, want %v", v.ScrollPad, want)
	}
	if !v.Snap.Vertical() || v.Snap.Horizontal() || !v.Snap.Mandatory {
		t.Errorf("scroll-snap-type = %+v, want y mandatory", v.Snap)
	}
	if v.Overscroll[0] != OverscrollContain || v.Overscroll[1] != OverscrollNone {
		t.Errorf("overscroll-behavior = %v, want contain none", v.Overscroll)
	}
	if v.BarThumb != render.RGB(255, 0, 0) || v.BarTrack != render.RGB(0, 0, 255) || v.BarColorAuto {
		t.Errorf("scrollbar-color = %x %x auto=%v, want red over blue", v.BarThumb, v.BarTrack, v.BarColorAuto)
	}
	if v.BarWidth != BarWidthThin {
		t.Errorf("scrollbar-width = %v, want thin", v.BarWidth)
	}

	v = scrollRun(t, `box { scroll-snap-type: inline proximity; scrollbar-color: auto; }`)
	if !v.Snap.Horizontal() || v.Snap.Vertical() || v.Snap.Mandatory {
		t.Errorf("scroll-snap-type inline = %+v, want inline only, proximity", v.Snap)
	}
	if !v.BarColorAuto {
		t.Error("scrollbar-color: auto did not compute")
	}
	v = scrollRun(t, `box { scroll-snap-type: both; overscroll-behavior-y: none; }`)
	if !v.Snap.Vertical() || !v.Snap.Horizontal() || v.Snap.Mandatory {
		t.Errorf("scroll-snap-type both = %+v, want both axes, proximity", v.Snap)
	}
	if v.Overscroll[1] != OverscrollNone || v.Overscroll[0] != OverscrollAuto {
		t.Errorf("overscroll-behavior-y none = %v, want auto none", v.Overscroll)
	}
}

// The scrolled-child properties compute: the scroll-margin sides
// (negatives allowed), scroll-snap-align's one- and two-value forms,
// and scroll-snap-stop.
func TestScrollChildProps(t *testing.T) {
	v := scrollRun(t, `box {
		scroll-margin: -4px 0 2px;
		scroll-snap-align: center;
		scroll-snap-stop: always;
	}`)
	if want := (Sides{Top: -4, Right: 0, Bottom: 2, Left: 0}); v.ScrollMargin != want {
		t.Errorf("scroll-margin = %v, want %v", v.ScrollMargin, want)
	}
	if v.SnapAlign.Block != SnapCenter || v.SnapAlign.Inline != SnapCenter {
		t.Errorf("scroll-snap-align = %+v, want center both", v.SnapAlign)
	}
	if !v.SnapStopAlways {
		t.Error("scroll-snap-stop: always did not compute")
	}
	v = scrollRun(t, `box { scroll-snap-align: start end; }`)
	if v.SnapAlign.Block != SnapStart || v.SnapAlign.Inline != SnapEnd {
		t.Errorf("scroll-snap-align start end = %+v, want block start inline end", v.SnapAlign)
	}
	v = scrollRun(t, `box { scroll-margin-top: 6px; }`)
	if v.ScrollMargin.Top != 6 || v.ScrollMargin.Bottom != 0 {
		t.Errorf("scroll-margin-top = %v, want top only", v.ScrollMargin)
	}
}

// scrollbar-color and scrollbar-width inherit; the container group
// does not.
func TestScrollbarPropsInherit(t *testing.T) {
	s := parseOne(t, `box { scrollbar-color: #101010 #202020; scrollbar-width: thin; scroll-behavior: smooth; }`)
	var sc Scratch
	var root, child Values
	Compute([]Layer{{Sheet: s}}, el("box"), nil, nil, Env{}, &sc, &root)
	Compute([]Layer{{Sheet: s}}, el("kid"), &root, nil, Env{}, &sc, &child)
	if !child.Has(PropScrollbarColor) || child.BarThumb != render.RGB(0x10, 0x10, 0x10) {
		t.Error("scrollbar-color did not inherit")
	}
	if !child.Has(PropScrollbarWidth) || child.BarWidth != BarWidthThin {
		t.Error("scrollbar-width did not inherit")
	}
	if child.Has(PropScrollBehavior) {
		t.Error("scroll-behavior inherited")
	}
}

// steps() takes its jump position, and linear() parses its point list
// with optional percent stops, wherever a timing function goes.
func TestTimingStepsAndLinear(t *testing.T) {
	v := scrollRun(t, `box { transition: opacity 1s steps(3, jump-start); }`)
	if v.Transition.Timing.Steps != 3 || v.Transition.Timing.StepPos != uint8(jumpPosOf("jump-start")) {
		t.Errorf("steps timing = %+v, want steps(3, jump-start)", v.Transition.Timing)
	}
	v = scrollRun(t, `box { transition-timing-function: steps(2, jump-both); }`)
	if v.Transition.Timing.Steps != 2 || v.Transition.Timing.StepPos != uint8(jumpPosOf("jump-both")) {
		t.Errorf("jump-both timing = %+v", v.Transition.Timing)
	}
	v = scrollRun(t, `box { transition-timing-function: steps(4); }`)
	if v.Transition.Timing.StepPos != 0 {
		t.Errorf("bare steps = %+v, want the jump-end default", v.Transition.Timing)
	}
	v = scrollRun(t, `box { transition: opacity 1s linear(0, 0.5 25%, 1); }`)
	tm := v.Transition.Timing
	if tm.LinN != 3 || !tm.Lin[0].HasPos == false || !tm.Lin[1].HasPos || tm.Lin[1].Pos != 0.25 || tm.Lin[2].Value != 1 {
		t.Errorf("linear() timing = %+v, want 0, 0.5@25%%, 1", tm)
	}
	v = scrollRun(t, `@keyframes k { to { opacity: 1; } } box { animation: k 1s linear(1 10%, 0 90%, 1); }`)
	if first(v).Timing.LinN != 3 || first(v).Timing.Lin[0].Pos != 0.1 || first(v).Timing.Lin[1].Pos != 0.9 {
		t.Errorf("animation linear() = %+v", first(v).Timing)
	}
}

// jumpPosOf maps a keyword to its StepPos value for the assertions.
func jumpPosOf(kw string) int {
	p, _ := timingJump(kw)
	return int(p)
}

// transition-behavior parses alone and inside the shorthand.
func TestTransitionBehavior(t *testing.T) {
	v := scrollRun(t, `box { transition-duration: 100ms; transition-behavior: allow-discrete; }`)
	if !v.Transition.Discrete {
		t.Error("transition-behavior: allow-discrete did not compute")
	}
	v = scrollRun(t, `box { transition: opacity 100ms allow-discrete; }`)
	if !v.Transition.Discrete || v.Transition.Duration != 0.1 {
		t.Errorf("shorthand = %+v, want 100ms allow-discrete", v.Transition)
	}
	v = scrollRun(t, `box { transition: opacity 100ms allow-discrete; transition-behavior: normal; }`)
	if v.Transition.Discrete {
		t.Error("the longhand did not reset the shorthand")
	}
}

// animation-composition parses alone, inside the shorthand, and zips
// onto the group.
func TestAnimationComposition(t *testing.T) {
	v := scrollRun(t, `@keyframes k { to { opacity: 0; } } box { animation: k 1s add; }`)
	if first(v).Composition != CompAdd {
		t.Errorf("shorthand composition = %v, want add", first(v).Composition)
	}
	v = scrollRun(t, `box { animation: k 1s; animation-composition: accumulate; }`)
	if first(v).Composition != CompAccumulate {
		t.Errorf("longhand composition = %v, want accumulate", first(v).Composition)
	}
	v = scrollRun(t, `box { animation: k 1s; animation-composition: replace; }`)
	if first(v).Composition != CompReplace {
		t.Errorf("longhand composition = %v, want replace", first(v).Composition)
	}
}

// @starting-style parses inside a rule and computes as the entry
// transition's values; a rule without one computes none.
func TestStartingStyle(t *testing.T) {
	v := scrollRun(t, `box {
		opacity: 1;
		transition: opacity 200ms;
		@starting-style { opacity: 0; background-color: #00ff00; }
	}`)
	if v.Starting == nil {
		t.Fatal("the @starting-style block did not compute")
	}
	if !v.Starting.Has(PropOpacity) || v.Starting.Opacity != 0 {
		t.Error("the starting opacity did not compute")
	}
	if !v.Starting.Has(PropBackgroundColor) || v.Starting.Background != render.RGB(0, 255, 0) {
		t.Error("the starting background did not compute")
	}
	if v.Starting.Has(PropColor) {
		t.Error("the starting block declared color")
	}
	v = scrollRun(t, `box { opacity: 1; }`)
	if v.Starting != nil {
		t.Error("a rule without @starting-style computed a starting style")
	}
}
