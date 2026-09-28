package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/gelm/render"
)

// calFrame measures and arranges a calendar and returns it.
func calFrame(c *Calendar) *Calendar {
	sz := c.Measure(Constraints{Max: Size{W: 4096, H: 4096}})
	c.Arrange(render.Rect{X: 0, Y: 0, W: sz.W, H: sz.H})
	return c
}

// TestCalendarMonthLayout pins the grid arithmetic (#73): the first
// day lands at its weekday offset, days map to cells round-trip, and
// month lengths (leap Februaries included) fit the six rows.
func TestCalendarMonthLayout(t *testing.T) {
	for _, tc := range []struct {
		year  int
		month time.Month
	}{
		{2026, time.January},  // starts Thursday, 31 days
		{2026, time.February}, // 28 days
		{2024, time.February}, // leap year, 29 days
		{2026, time.March},
	} {
		c := calFrame(NewCalendar(entryFace(t), 13,
			time.Date(tc.year, tc.month, 1, 0, 0, 0, 0, time.UTC)))
		offset := c.firstOffset()
		days := daysIn(tc.year, tc.month)
		if offset+days > calendarCols*calendarRows {
			t.Errorf("%d/%02d: %d days at offset %d overflow the six rows", tc.year, tc.month, days, offset)
		}
		// Every day's cell center maps back to the day.
		for d := 1; d <= days; d++ {
			r := c.cellRect(d, offset)
			if got := c.dayAt(Point{X: r.X + r.W/2, Y: r.Y + r.H/2}); got != d {
				t.Errorf("%d/%02d day %d round-trips to %d", tc.year, tc.month, d, got)
			}
		}
		// A blank leading cell maps to no day.
		if offset > 0 {
			r := c.cellRect(1, offset)
			if got := c.dayAt(Point{X: r.X - c.cellW/2, Y: r.Y + r.H/2}); got != 0 {
				t.Errorf("%d/%02d: a blank cell mapped to day %d", tc.year, tc.month, got)
			}
		}
	}
}

// TestCalendarSelectionAndNavigation pins selection and the four
// navigation directions across month and year boundaries: arrows cross
// months with the view following, PageUp/PageDown step months, and the
// clamps hold (Feb 29 survives where it can, clamps where it cannot).
func TestCalendarSelectionAndNavigation(t *testing.T) {
	var picked []time.Time
	c := NewCalendar(entryFace(t), 13, time.Date(2026, time.November, 30, 0, 0, 0, 0, time.UTC))
	c.OnSelect = func(d time.Time) { picked = append(picked, d) }
	calFrame(c)

	t.Run("arrows cross the month boundary", func(t *testing.T) {
		c.KeyAction(KeyRight, 0) // Nov 30 -> Dec 1
		want := time.Date(2026, time.December, 1, 0, 0, 0, 0, time.UTC)
		if c.Selection() != want {
			t.Fatalf("selection = %v, want %v", c.Selection(), want)
		}
		if c.view.Month() != time.December {
			t.Fatalf("view stayed on %v", c.view.Month())
		}
		c.KeyAction(KeyLeft, 0)
		if c.Selection().Day() != 30 || c.view.Month() != time.November {
			t.Fatalf("back across the boundary = %v on %v", c.Selection(), c.view.Month())
		}
	})

	t.Run("vertical motion crosses the year boundary", func(t *testing.T) {
		c.Select(time.Date(2026, time.January, 3, 0, 0, 0, 0, time.UTC))
		picked = picked[:0]
		c.KeyAction(KeyUp, 0) // Jan 3 2026 - 7 = Dec 27 2025
		want := time.Date(2025, time.December, 27, 0, 0, 0, 0, time.UTC)
		if c.Selection() != want {
			t.Fatalf("week-up from Jan 3 = %v, want %v", c.Selection(), want)
		}
		if c.view.Year() != 2025 {
			t.Fatalf("view year = %d, want 2025", c.view.Year())
		}
	})

	t.Run("page steps clamp the day", func(t *testing.T) {
		c.Select(time.Date(2024, time.January, 31, 0, 0, 0, 0, time.UTC))
		c.KeyAction(KeyNextPage, 0) // Jan 31 -> Feb 29 (leap year clamps)
		if got := c.Selection(); got.Month() != time.February || got.Day() != 29 || got.Year() != 2024 {
			t.Fatalf("Jan 31 2024 paged = %v, want Feb 29 2024", got)
		}
		c.KeyAction(KeyNextPage, 0) // Feb 29 2024 -> Mar 29
		if got := c.Selection(); got.Month() != time.March || got.Day() != 29 {
			t.Fatalf("Feb 29 paged = %v, want Mar 29", got)
		}
	})

	t.Run("the year buttons cross the leap day", func(t *testing.T) {
		c.Select(time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC))
		c.stepView(1, 0)
		if got := c.Selection(); got.Month() != time.February || got.Day() != 28 || got.Year() != 2025 {
			t.Fatalf("Feb 29 2024 stepped a year = %v, want Feb 28 2025", got)
		}
	})

	if len(picked) == 0 {
		t.Error("OnSelect never fired across the navigation")
	}
}

// TestCalendarClickSelects pins the pointer path: a click on a day
// selects it and fires OnSelect with exactly that date, and clicks on
// blanks or the grid rules change nothing.
func TestCalendarClickSelects(t *testing.T) {
	var picked []time.Time
	c := NewCalendar(entryFace(t), 13, time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC))
	c.OnSelect = func(d time.Time) { picked = append(picked, d) }
	calFrame(c)

	// May 15 2026: a Friday in the middle of the grid.
	cell := c.cellRect(15, c.firstOffset())
	c.ClickAt(Point{X: cell.X + cell.W/2, Y: cell.Y + cell.H/2})
	want := time.Date(2026, time.May, 15, 0, 0, 0, 0, time.UTC)
	if c.Selection() != want || len(picked) != 1 || picked[0] != want {
		t.Fatalf("click selected %v (notified %v), want %v once", c.Selection(), picked, want)
	}

	// The title area is not a day.
	c.ClickAt(Point{X: c.bounds.X + c.bounds.W/2, Y: c.bounds.Y + 2})
	if len(picked) != 1 {
		t.Errorf("a title click selected a day: %v", picked)
	}
}

// mondayEN starts weeks on Monday.
type mondayEN struct{ CalendarENUS }

func (mondayEN) FirstWeekday() time.Weekday { return time.Monday }

// TestCalendarNamesSwap pins the locale seam: a custom CalendarNames
// set changes the first-weekday offset and the names the header and
// title render from; nil restores en-US.
func TestCalendarNamesSwap(t *testing.T) {
	c := NewCalendar(entryFace(t), 13, time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC))
	// May 1 2026 is a Friday: Sunday-first offset 5, Monday-first 4.
	if got := c.firstOffset(); got != 5 {
		t.Fatalf("en-US offset = %d, want 5 (Friday, Sunday start)", got)
	}
	c.SetNames(mondayEN{})
	if got := c.firstOffset(); got != 4 {
		t.Fatalf("Monday-first offset = %d, want 4", got)
	}
	c.SetNames(nil)
	if got := c.firstOffset(); got != 5 {
		t.Fatalf("nil names did not restore en-US: offset %d", got)
	}
}
