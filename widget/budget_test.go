// The allocation budgets (#77): hard ceilings derived from the measured
// baseline in bench/baseline.md, gated here so a regression fails `just
// check`. Each budget carries 1.2x headroom over the baseline number,
// not invented room: a fix that lowers the baseline should tighten it
// here in the same commit.
package widget

import (
	"testing"

	"github.com/stubbedev/gelm/render"
)

// TestAllocBudgetGalleryFrame pins the steady frame: one full
// measure+arrange+damage+paint pass over the showcase gallery. The
// baseline measured 24 allocs/op (bench/baseline.md); the budget is
// ceil(24*1.2).
func TestAllocBudgetGalleryFrame(t *testing.T) {
	show := buildShowcase(t)
	cv := render.New(make([]byte, render.Stride(showW)*showH), render.Stride(showW), showW, showH)
	stale := render.Rect{W: showW, H: showH}
	const budget = 29
	paintFrame(cv, show.root, showW, showH, Current().Bg, stale) // warm the shape and damage paths
	allocs := testing.AllocsPerRun(50, func() {
		paintFrame(cv, show.root, showW, showH, Current().Bg, stale)
	})
	if allocs > budget {
		t.Errorf("gallery frame allocates %v allocs/op, budget %d", allocs, budget)
	}
}
