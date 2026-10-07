package widget

import (
	"testing"
	"time"

	"github.com/stubbedev/gelm/render"
)

// TestCalendarMarksAndDetail pins marks as dates (they survive a view
// change and stay with their month) and the per-day detail tooltip
// following the hovered day.
func TestCalendarMarksAndDetail(t *testing.T) {
	face := chromeFace(t)
	day := func(d int) time.Time { return time.Date(2026, time.September, d, 0, 0, 0, 0, time.UTC) }
	c := NewCalendar(face, 13, day(14))
	c.MarkDay(day(3))
	c.MarkDay(day(20))
	c.UnmarkDay(day(20))
	if !c.DayMarked(day(3)) || c.DayMarked(day(20)) || c.DayMarked(day(3).AddDate(0, 1, 0)) {
		t.Error("marks are not per date")
	}
	c.Detail = func(d time.Time) string {
		if d.Day() == 14 {
			return "Release"
		}
		return ""
	}
	c.SetTooltip("calendar")
	c.Measure(Constraints{Max: Size{W: 400, H: 400}})
	c.Arrange(render.Rect{W: 400, H: 400})
	cell := c.cellRect(14, c.firstOffset())
	c.HoverMove(Point{X: cell.X + 2, Y: cell.Y + 2})
	if got := c.TooltipText(); got != "Release" {
		t.Errorf("tooltip over the 14th = %q", got)
	}
	cell = c.cellRect(15, c.firstOffset())
	c.HoverMove(Point{X: cell.X + 2, Y: cell.Y + 2})
	if got := c.TooltipText(); got != "calendar" {
		t.Errorf("tooltip over a day without detail = %q, want the calendar's own", got)
	}
	c.ClearMarks()
	if c.DayMarked(day(3)) {
		t.Error("ClearMarks kept a mark")
	}
}

// TestGoldenCalendarMarks pins the mark dots, on a plain day and on
// the selected one.
func TestGoldenCalendarMarks(t *testing.T) {
	face := goldenFace(t)
	th := DarkTheme()
	day := func(d int) time.Time { return time.Date(2026, time.September, d, 0, 0, 0, 0, time.UTC) }
	cal := NewCalendar(face, 13, day(14))
	cal.today = day(3)
	for _, d := range []int{8, 14, 22} {
		cal.MarkDay(day(d))
	}
	NewGolden(t, cal, "calendar-marks", goldenTheme(th))
}
