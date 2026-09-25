package app

import (
	"testing"
	"time"

	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

func TestTooltipShouldOpen(t *testing.T) {
	base := time.Now()

	t.Run("dwell past the delay opens", func(t *testing.T) {
		if !tooltipShouldOpen(false, &widget.Label{}, "text", base, base.Add(tooltipDelay)) {
			t.Error("resting hover with text did not open at the delay")
		}
	})

	t.Run("before the delay nothing opens", func(t *testing.T) {
		if tooltipShouldOpen(false, &widget.Label{}, "text", base, base.Add(tooltipDelay-time.Millisecond)) {
			t.Error("opened before the dwell elapsed")
		}
	})

	t.Run("no text never opens", func(t *testing.T) {
		if tooltipShouldOpen(false, &widget.Label{}, "", base, base.Add(time.Hour)) {
			t.Error("opened without tooltip text")
		}
	})

	t.Run("no hover never opens", func(t *testing.T) {
		if tooltipShouldOpen(false, nil, "text", base, base.Add(time.Hour)) {
			t.Error("opened without a hovered widget")
		}
	})

	t.Run("an open tooltip blocks a second", func(t *testing.T) {
		if tooltipShouldOpen(true, &widget.Label{}, "text", base, base.Add(time.Hour)) {
			t.Error("opened a second tooltip while one was up")
		}
	})
}

func TestTooltipShouldClose(t *testing.T) {
	t.Run("hover change closes", func(t *testing.T) {
		if !tooltipShouldClose(true, true, true) {
			t.Error("hover change kept the tooltip")
		}
	})

	t.Run("lost text closes", func(t *testing.T) {
		if !tooltipShouldClose(true, false, false) {
			t.Error("tooltip stayed after its text vanished")
		}
	})

	t.Run("nothing open closes nothing", func(t *testing.T) {
		if tooltipShouldClose(false, true, true) {
			t.Error("closed a tooltip that was not open")
		}
	})
}

// tipTarget is a hoverable widget carrying a tooltip, for router tests.
type tipTarget struct {
	bounds  render.Rect
	tooltip string
}

func newTipTarget(text string) *tipTarget { return &tipTarget{tooltip: text} }

func (t *tipTarget) Measure(con widget.Constraints) widget.Size {
	if con.Max.W < 8 {
		return con.Max
	}
	if con.Max.H < 8 {
		return con.Max
	}
	return widget.Size{W: 8, H: 8}
}

func (t *tipTarget) Arrange(r render.Rect)   { t.bounds = r }
func (t *tipTarget) Paint(cv *render.Canvas) {}

func (t *tipTarget) HitTest(p widget.Point) widget.Widget {
	if t.bounds.Contains(p.X, p.Y) {
		return t
	}
	return nil
}

func (t *tipTarget) TooltipText() string { return t.tooltip }

// stub is a popup double recording closes.
type stub struct{ closed int }

func (s *stub) Closed() bool { return false }
func (s *stub) Close()       { s.closed++ }

func TestTooltipCtlUpdate(t *testing.T) {
	target := newTipTarget("hover text")
	target.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 20})
	router := &widget.Router{Root: target}

	var openStub *stub
	var openedText string
	opener := func(w widget.Widget, text string) tooltipWindow {
		openedText = text
		openStub = &stub{}
		return openStub
	}

	ctl := &tooltipCtl{}
	base := time.Now()

	t.Run("arrival does not open early", func(t *testing.T) {
		router.Move(widget.Point{X: 10, Y: 10})
		ctl.update(router, base, opener)
		if openStub != nil {
			t.Error("opened without dwell")
		}
	})

	t.Run("dwell opens exactly once with the widget's text", func(t *testing.T) {
		ctl.update(router, base.Add(tooltipDelay), opener)
		if openStub == nil {
			t.Fatal("dwell did not open the tooltip")
		}
		if openedText != "hover text" {
			t.Errorf("opener text = %q, want the widget's tooltip", openedText)
		}
		first := openStub
		ctl.update(router, base.Add(2*tooltipDelay), opener)
		if openStub != first {
			t.Error("a second tooltip replaced the first while the hover held")
		}
	})

	t.Run("hover change closes and re-arms the dwell", func(t *testing.T) {
		router.Move(widget.Point{X: 10, Y: 200})
		ctl.update(router, base.Add(2*tooltipDelay+time.Second), opener)
		if openStub.closed != 1 {
			t.Errorf("closes = %d, want 1 after hover change", openStub.closed)
		}
		router.Move(widget.Point{X: 10, Y: 10})
		ctl.update(router, base.Add(3*tooltipDelay), opener)
		if openStub == nil || openStub.closed != 1 {
			t.Error("state lost across the hover change")
		}
	})
}
