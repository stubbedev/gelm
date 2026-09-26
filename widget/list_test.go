package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// countingModel is a spy model: it records every Row request so tests
// can pin virtualization.
type countingModel struct {
	rows   int
	called map[int]int
	w      Widget
}

func newCountingModel(rows int) *countingModel {
	return &countingModel{rows: rows, called: make(map[int]int), w: newStub(60, 20)}
}

func (m *countingModel) Len() int { return m.rows }

func (m *countingModel) Row(i int) Widget {
	m.called[i]++
	return m.w
}

func (m *countingModel) distinct() int { return len(m.called) }

func TestListVirtualization(t *testing.T) {
	model := newCountingModel(10000)
	l := NewList(model, 20)
	l.Measure(Constraints{Max: Size{W: 200, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 60}) // 3 rows of 20px

	cv := render.New(make([]uint8, 200*60*4), 200*4, 200, 60)
	l.Paint(cv)

	if got := model.distinct(); got > 5 {
		t.Errorf("painting 3 visible rows requested %d distinct rows, want <= 5", got)
	}

	// Scroll far away: only the new viewport's rows are requested.
	l.ScrollBy(0, 40)
	l.Arrange(render.Rect{X: 0, Y: 0, W: 200, H: 60})
	l.Paint(cv)
	if got := model.distinct(); got > 8 {
		t.Errorf("after scrolling, %d distinct rows were requested cumulatively, want <= 8", got)
	}
}

// TestListSelectionClamps pins the choice that selection motion clamps
// at the ends instead of wrapping, and Enter activates once per press.
func TestListSelectionClamps(t *testing.T) {
	model := newCountingModel(5)
	l := NewList(model, 20)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 100})

	t.Run("motion clamps at the ends", func(t *testing.T) {
		l.Select(0)
		l.KeyAction(KeyUp, 0)
		if got := l.Selected(); got != 0 {
			t.Errorf("up from top = %d, want clamped 0", got)
		}
		l.Select(4)
		l.KeyAction(KeyDown, 0)
		l.KeyAction(KeyDown, 0)
		if got := l.Selected(); got != 4 {
			t.Errorf("down past bottom = %d, want clamped 4", got)
		}
	})

	t.Run("home and end jump", func(t *testing.T) {
		l.Select(2)
		l.KeyAction(KeyHome, 0)
		if got := l.Selected(); got != 0 {
			t.Errorf("home = %d, want 0", got)
		}
		l.KeyAction(KeyEnd, 0)
		if got := l.Selected(); got != 4 {
			t.Errorf("end = %d, want 4", got)
		}
	})

	t.Run("enter activates exactly once per press", func(t *testing.T) {
		activations := 0
		l.OnActivate = func(i int) { activations++ }
		l.Select(2)
		l.KeyAction(KeyEnter, 0)
		l.KeyAction(KeyEnter, 0)
		if activations != 2 {
			t.Errorf("activations = %d after two Enters, want 2", activations)
		}
		l.KeyAction(KeyRight, 0)
		if activations != 2 {
			t.Errorf("non-Enter key activated (%d)", activations)
		}
	})
}

// TestListChanged pins model updates: Changed re-queries the length,
// drops caches past the new end, and keeps the selection clamped.
func TestListChanged(t *testing.T) {
	model := newCountingModel(10)
	l := NewList(model, 20)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 60})
	l.Select(8)

	model.rows = 3
	l.Changed()
	if got := l.Selected(); got != 2 {
		t.Errorf("selection = %d after shrink, want clamped 2", got)
	}
	if len(l.rows) > 3 {
		t.Errorf("row cache holds %d entries past the new length", len(l.rows))
	}

	// A row widget for a surviving index is reused, not rebuilt.
	model.rows = 12
	l.Select(1)
	before := l.rows[1]
	l.Changed()
	if l.rows[1] != before {
		t.Error("Changed rebuilt a widget that survived the update")
	}
}

// TestListRowHit pins that hit-testing a row returns the row widget
// itself, so per-row tooltips and hover styling resolve through the
// standard tooltip machine. The loop paints before any hit can happen;
// painting populates the visible-row cache.
func TestListRowHit(t *testing.T) {
	model := newCountingModel(6)
	l := NewList(model, 20)
	l.Measure(Constraints{Max: Size{W: 100, H: 1 << 20}})
	l.Arrange(render.Rect{X: 0, Y: 0, W: 100, H: 60})
	cv := render.New(make([]uint8, 100*60*4), 100*4, 100, 60)
	l.Paint(cv)

	got := l.HitTest(Point{X: 50, Y: 45})
	row := l.rowAt(Point{X: 50, Y: 45})
	if got != l.rows[row] {
		t.Errorf("hit widget is not the cached row %d", row)
	}
}
