// The allocation budgets (#77): hard ceilings derived from the measured
// baseline in bench/baseline.md, gated here so a regression fails `just
// check`. Each budget carries 1.2x headroom over the baseline number,
// not invented room: a fix that lowers the baseline should tighten it
// here in the same commit.
package app

import (
	"testing"
	"time"

	"github.com/stubbedev/gelm/internal/animclock"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/widget"
)

// idleBudgetApp builds an application over a zero-value session whose
// single window is a wired fake host: enough loop machinery to run one
// real tick, no compositor.
func idleBudgetApp(tb testing.TB) *Application {
	tb.Helper()
	animclock.Reset()
	a := NewApplication(&wlsession.Session{})
	a.step = func() error { return nil }
	a.wake = func(time.Duration) {}
	a.windows = append(a.windows, &hostWindow{
		host:    &fakeHost{w: 640, h: 470},
		sess:    a.sess,
		router:  &widget.Router{Root: widget.NewBox(widget.Column, 8, 0)},
		tip:     &tooltipCtl{since: time.Now()},
		frac120: 120, scale: 1,
		lastW: 640, lastH: 470,
	})
	return a
}

// TestAllocBudgetIdleLoopTick pins the parked loop's steady tick: an
// idle pass over one wired window — pump, repeat check, tooltip state,
// window bookkeeping, wake computation, one dispatch — must allocate
// nothing. The strace baseline (bench/baseline.md) shows the real loop
// parks at zero syscalls when idle; this is its in-process
// counterpart. The baseline measured 0 allocs/tick, so the budget
// stays at 0.
func TestAllocBudgetIdleLoopTick(t *testing.T) {
	a := idleBudgetApp(t)
	tick := func() {
		if err := a.tick(a.step, time.Now()); err != nil {
			panic(err)
		}
	}
	tick() // warm: tooltip bookkeeping, icon maps, theme stamps
	const budget = 0
	allocs := testing.AllocsPerRun(100, tick)
	if allocs > budget {
		t.Errorf("idle loop tick allocates %v allocs/op, budget %d", allocs, budget)
	}
}
