package widget

import (
	"cmp"
	"fmt"
	"time"
)

// Date is a calendar day: a year, month and day with no time of day
// and no zone, so it means the same day wherever it is read. The zero
// Date is no date.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// DateOf is t's calendar day in t's own location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

// Today is the local calendar day now, the one reading of the clock
// and zone the calendar widgets make (see Calendar.SetToday).
func Today() Date { return DateOf(time.Now()) }

// IsZero reports the zero Date.
func (d Date) IsZero() bool { return d == Date{} }

// Time is the day's midnight in UTC.
func (d Date) Time() time.Time { return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC) }

// AddDays is the day n days on (back for negative n).
func (d Date) AddDays(n int) Date { return DateOf(d.Time().AddDate(0, 0, n)) }

// AddMonths is the same day n months on, clamped to that month's
// length: January 31 plus one month is the last of February.
func (d Date) AddMonths(n int) Date {
	first := time.Date(d.Year, d.Month+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	return Date{first.Year(), first.Month(), min(d.Day, daysIn(first.Year(), first.Month()))}
}

// FirstOfMonth is the first day of d's month.
func (d Date) FirstOfMonth() Date { return Date{d.Year, d.Month, 1} }

// Weekday is the day of the week.
func (d Date) Weekday() time.Weekday { return d.Time().Weekday() }

// Compare orders days: -1 when d is earlier than o, 0 when the same,
// +1 when later.
func (d Date) Compare(o Date) int {
	switch {
	case d.Year != o.Year:
		return cmp.Compare(d.Year, o.Year)
	case d.Month != o.Month:
		return cmp.Compare(d.Month, o.Month)
	}
	return cmp.Compare(d.Day, o.Day)
}

// Before reports d earlier than o.
func (d Date) Before(o Date) bool { return d.Compare(o) < 0 }

// String is the ISO 8601 day, 2026-10-24.
func (d Date) String() string { return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day) }

// daysIn is the number of days in a month.
func daysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
