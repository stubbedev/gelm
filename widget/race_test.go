//go:build race

package widget

// raceEnabled reports a -race build, whose instrumentation allocates:
// allocation budgets skip under it.
const raceEnabled = true
