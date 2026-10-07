package widget

import (
	"time"

	"github.com/stubbedev/gelm/render"
)

// calendarCells is the fixed grid: seven weekday columns, six week
// rows (a month whose first day lands late spans six weeks), plus the
// weekday header row and the title row above it.
const (
	calendarCols = 7
	calendarRows = 6
)

// CalendarNames supplies a Calendar's localized pieces. A nil
// interface (the default) uses the en-US names below; supply your own
// to localize.
type CalendarNames interface {
	// MonthName is the full month name for the title.
	MonthName(time.Month) string
	// WeekdayName is the short weekday label for the header.
	WeekdayName(time.Weekday) string
	// FirstWeekday is the week's first column (Sunday for en-US).
	FirstWeekday() time.Weekday
}

// CalendarENUS is the built-in en-US name set: full English month
// names, three-letter weekday abbreviations, weeks starting Sunday.
type CalendarENUS struct{}

// MonthName implements CalendarNames.
func (CalendarENUS) MonthName(m time.Month) string { return m.String() }

// WeekdayName implements CalendarNames.
func (CalendarENUS) WeekdayName(w time.Weekday) string { return w.String()[:3] }

// FirstWeekday implements CalendarNames.
func (CalendarENUS) FirstWeekday() time.Weekday { return time.Sunday }

// Calendar is a month grid with single selection: a title row with
// previous/next month and year navigation, the weekday header, and the
// days of the displayed month. Today carries a marker, the selection
// an accent cell, and both come from the typed theme - no hardcoded
// colors.
//
// Pointer: a click selects its day; the four title buttons step the
// view a month or a year at a time (the selection follows the view).
// Keyboard: arrows move the selection a day (or a week vertically)
// across month boundaries - the view follows - and PageUp/PageDown
// step the month. OnSelect fires once per applied selection with the
// chosen date.
//
// Names come from CalendarNames: nil uses the en-US set; SetNames
// swaps in another locale.
type Calendar struct {
	node
	face   render.Font
	sizePx float64
	names  CalendarNames

	view  time.Time // first day of the displayed month
	sel   time.Time // the selection; zero when none
	today time.Time

	// nav holds the four title buttons in paint order: year back,
	// month back, month forward, year forward. Their rects are the
	// cells of navRow.
	nav [4]*Button

	cellW, cellH int
	titleH       int

	// marks are the marked dates (MarkDay); hoverDay is the day of the
	// displayed month under the pointer, 0 when none.
	marks    map[calendarDate]bool
	hoverDay int

	// Detail, when set, describes a date - an event title, a holiday -
	// shown as the tooltip of the day under the pointer ("" for none).
	Detail func(date time.Time) string

	// OnSelect fires with the chosen date on every applied selection -
	// click, keyboard motion, or Select.
	OnSelect func(time.Time)
}

// NewCalendar returns a calendar showing the month of initial, its
// selection on initial's day, and today's marker taken at construction.
func NewCalendar(face render.Font, sizePx float64, initial time.Time) *Calendar {
	face = requireFace("widget.NewCalendar", face)
	c := &Calendar{
		face:   face,
		sizePx: sizePx,
		view:   time.Date(initial.Year(), initial.Month(), 1, 0, 0, 0, 0, initial.Location()),
		sel:    initial,
		today:  time.Now(),
	}
	glyph := face.Shape("lg", sizePx).LineHeight() - 2
	for i, kind := range [4]SymbolKind{SymbolDoubleLeft, SymbolChevronLeft, SymbolChevronRight, SymbolDoubleRight} {
		c.nav[i] = NewButton(NewSymbol(kind, glyph), 2, 4)
	}
	c.nav[0].OnClick = func() { c.stepView(-1, 0) }
	c.nav[1].OnClick = func() { c.stepView(0, -1) }
	c.nav[2].OnClick = func() { c.stepView(0, 1) }
	c.nav[3].OnClick = func() { c.stepView(1, 0) }
	c.cellH = face.Shape("lg", sizePx).LineHeight() + 8
	c.cellW = c.cellH + 4
	c.titleH = face.Shape("lg", sizePx).LineHeight() + 14
	return c
}

// SetNames swaps the locale's names; nil restores the en-US set.
func (c *Calendar) SetNames(names CalendarNames) {
	c.names = names
	c.Invalidate()
}

// calendarDate is a date without time or zone, the mark key.
type calendarDate struct {
	year  int
	month time.Month
	day   int
}

// dateOf keys t.
func dateOf(t time.Time) calendarDate { return calendarDate{t.Year(), t.Month(), t.Day()} }

// MarkDay marks t's date (GTK mark_day): a dot under its number. Marks
// are dates, so they stay with their month as the view moves.
func (c *Calendar) MarkDay(t time.Time) {
	if c.marks == nil {
		c.marks = map[calendarDate]bool{}
	}
	c.marks[dateOf(t)] = true
	c.Invalidate()
}

// UnmarkDay removes t's mark.
func (c *Calendar) UnmarkDay(t time.Time) {
	delete(c.marks, dateOf(t))
	c.Invalidate()
}

// ClearMarks removes every mark.
func (c *Calendar) ClearMarks() {
	clear(c.marks)
	c.Invalidate()
}

// DayMarked reports whether t's date is marked.
func (c *Calendar) DayMarked(t time.Time) bool { return c.marks[dateOf(t)] }

// dateAt is day d of the displayed month.
func (c *Calendar) dateAt(d int) time.Time {
	return time.Date(c.view.Year(), c.view.Month(), d, 0, 0, 0, 0, c.view.Location())
}

// HoverMove tracks the day under the pointer for its Detail tooltip.
func (c *Calendar) HoverMove(p Point) { c.hoverDay = c.dayAt(p) }

// SetHovered forgets the day when the pointer leaves.
func (c *Calendar) SetHovered(on bool) {
	if !on {
		c.hoverDay = 0
	}
}

// TooltipText is the hovered day's Detail, else the calendar's own
// tooltip.
func (c *Calendar) TooltipText() string {
	if c.Detail != nil && c.hoverDay > 0 {
		if s := c.Detail(c.dateAt(c.hoverDay)); s != "" {
			return s
		}
	}
	return c.node.TooltipText()
}

// Selection returns the selected date, the zero time when none.
func (c *Calendar) Selection() time.Time { return c.sel }

// Select applies t as the selection, moves the view to its month, and
// fires OnSelect once.
func (c *Calendar) Select(t time.Time) {
	c.sel = t
	c.view = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	c.Invalidate()
	if c.OnSelect != nil {
		c.OnSelect(t)
	}
}

// namesOf returns the active name set.
func (c *Calendar) namesOf() CalendarNames {
	if c.names != nil {
		return c.names
	}
	return CalendarENUS{}
}

// stepView moves the displayed month by (years, months) and carries
// the selection to the same day of the new month, clamped to its
// length (February clamps the 31st; leap day clamps on dry years).
func (c *Calendar) stepView(years, months int) {
	base := c.view.Year()*12 + int(c.view.Month()) - 1
	m := base + months + 12*years
	norm := time.Date(m/12, time.Month(m%12+1), 1, 0, 0, 0, 0, c.view.Location())
	day := 1
	if !c.sel.IsZero() && c.sel.Month() == c.view.Month() && c.sel.Year() == c.view.Year() {
		day = c.sel.Day()
	}
	if day > daysIn(norm.Year(), norm.Month()) {
		day = daysIn(norm.Year(), norm.Month())
	}
	c.view = norm
	c.sel = time.Date(norm.Year(), norm.Month(), day, 0, 0, 0, 0, norm.Location())
	c.Invalidate()
	if c.OnSelect != nil {
		c.OnSelect(c.sel)
	}
}

// daysIn is the number of days in a month.
func daysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// firstOffset is the number of blank cells before day 1 in the header's
// week order.
func (c *Calendar) firstOffset() int {
	first := c.dateAt(1).Weekday()
	start := c.namesOf().FirstWeekday()
	return (int(first-start) + 7) % 7
}

// weekdayOrder lists the weekday header columns left to right.
func (c *Calendar) weekdayOrder() [7]time.Weekday {
	var order [7]time.Weekday
	start := c.namesOf().FirstWeekday()
	for i := range order {
		order[i] = time.Weekday((int(start) + i) % 7)
	}
	return order
}

// gridOrigin is the top-left of the day grid, below title and header.
func (c *Calendar) gridOrigin() (int, int) {
	return c.bounds.X, c.bounds.Y + c.titleH + c.cellH
}

// Measure wants the title and header rows plus six week rows, and the
// seven columns, clamped to con.
func (c *Calendar) Measure(con Constraints) Size {
	if sz, ok := c.measureHit(con); ok {
		return sz
	}
	w := calendarCols * c.cellW
	h := c.titleH + c.cellH + calendarRows*c.cellH
	return c.measureStore(con, clampSize(Size{W: w, H: h}, con))
}

// Arrange records the rect and places the four title buttons beside the
// month title: year steps at the outer edges, month steps beside them.
func (c *Calendar) Arrange(r render.Rect) {
	c.node.Arrange(r)
	lineH := c.face.Shape("lg", c.sizePx).LineHeight()
	buttonH := lineH + 4
	var w [4]int
	for i, b := range c.nav {
		w[i] = b.Measure(Constraints{Max: Size{W: 1 << 20, H: 1 << 20}}).W
	}
	const edge, gap = 4, 2
	left, right := r.X+edge, r.X+r.W-edge
	xs := [4]int{left, left + w[0] + gap, right - w[3] - gap - w[2], right - w[3]}
	for i, b := range c.nav {
		b.Arrange(render.Rect{X: xs[i], Y: c.bounds.Y + (c.titleH-buttonH)/2, W: w[i], H: buttonH})
		setParents(c, b)
	}
}

// Children exposes the four navigation buttons for focus traversal and
// the damage walk; the day cells are painted geometry, not widgets.
func (c *Calendar) Children() []Widget {
	return []Widget{c.nav[0], c.nav[1], c.nav[2], c.nav[3]}
}

// appendChildren appends the navigation buttons, matching Children.
func (c *Calendar) appendChildren(buf []Widget) []Widget {
	return append(buf, c.nav[0], c.nav[1], c.nav[2], c.nav[3])
}

// Paint draws the title, the weekday header, the day grid with its
// rules, today's marker, and the selected cell - every color from the
// theme.
func (c *Calendar) Paint(cv *render.Canvas) {
	th := Current()
	names := c.namesOf()

	title := names.MonthName(c.view.Month()) + " " + itoa(c.view.Year())
	c.face.DrawAligned(cv, title, render.Rect{
		X: c.bounds.X, Y: c.bounds.Y, W: c.bounds.W, H: c.titleH,
	}, c.sizePx, th.Text, render.AlignCenter)

	gx, gy := c.gridOrigin()
	for i, wd := range c.weekdayOrder() {
		c.face.DrawAligned(cv, names.WeekdayName(wd), render.Rect{
			X: gx + i*c.cellW, Y: c.bounds.Y + c.titleH, W: c.cellW, H: c.cellH,
		}, c.sizePx, th.TextMuted, render.AlignCenter)
	}

	offset := c.firstOffset()
	days := daysIn(c.view.Year(), c.view.Month())
	for d := 1; d <= days; d++ {
		cell := c.cellRect(d, offset)
		date := c.dateAt(d)
		selected := !c.sel.IsZero() && sameDate(c.sel, date)
		today := !c.today.IsZero() && sameDate(c.today, date)
		switch {
		case selected:
			cv.RoundedRect(cell, min(6, c.cellH/4), th.Accent)
		case today:
			cv.RoundedRect(cell, min(6, c.cellH/4), th.SurfaceHover)
		}
		col := th.Text
		if selected {
			col = th.OnAccent
		}
		c.face.DrawAligned(cv, dayNumber(d), cell, c.sizePx, col, render.AlignCenter)
		if today && !selected {
			cv.BorderRect(cell, 1, th.Accent)
		}
		if c.marks[dateOf(date)] {
			dot := render.Rect{X: cell.X + cell.W/2 - 2, Y: cell.Y + cell.H - 6, W: 4, H: 4}
			cv.RoundedRect(dot, 2, col)
		}
	}

	// The grid rules: a hairline between week rows, theme border color.
	for row := 0; row <= calendarRows; row++ {
		y := gy + row*c.cellH
		cv.Line(gx, y, gx+calendarCols*c.cellW, y, 1, th.Border)
	}
	for col := 0; col <= calendarCols; col++ {
		x := gx + col*c.cellW
		cv.Line(x, gy, x, gy+calendarRows*c.cellH, 1, th.Border)
	}
	for _, b := range c.nav {
		PaintChild(cv, b)
	}
}

// cellRect is day d's cell with the month's leading blank offset.
func (c *Calendar) cellRect(d, offset int) render.Rect {
	gx, gy := c.gridOrigin()
	i := offset + d - 1
	return render.Rect{
		X: gx + (i%calendarCols)*c.cellW,
		Y: gy + (i/calendarCols)*c.cellH,
		W: c.cellW, H: c.cellH,
	}
}

// dayNumbers holds "1".."31": the day grid shapes the same strings
// every frame, and a fresh conversion per cell per frame was 31
// allocations on the calendar's paint path.
var dayNumbers = [...]string{
	"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
	"11", "12", "13", "14", "15", "16", "17", "18", "19", "20",
	"21", "22", "23", "24", "25", "26", "27", "28", "29", "30", "31",
}

func dayNumber(d int) string { return dayNumbers[d-1] }

// itoa renders a small integer without fmt.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// sameDate compares year/month/day only.
func sameDate(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

// Role implements Roleer.
func (c *Calendar) Role() Role { return RoleCalendar }

// HitTest returns a navigation button under p, else the calendar (the
// day cells are its own geometry).
func (c *Calendar) HitTest(p Point) Widget {
	for _, b := range c.nav {
		if b.HitTest(p) != nil {
			return b
		}
	}
	if c.bounds.Contains(p.X, p.Y) {
		return c
	}
	return nil
}

// ClickAt selects the day under p; clicks outside the day grid change
// nothing.
func (c *Calendar) ClickAt(p Point) {
	if !IsEnabled(c) {
		return
	}
	d := c.dayAt(p)
	if d <= 0 {
		return
	}
	c.Select(c.dateAt(d))
}

// dayAt maps p to a day of the displayed month, 0 outside the grid or
// on a blank cell.
func (c *Calendar) dayAt(p Point) int {
	gx, gy := c.gridOrigin()
	col := (p.X - gx) / c.cellW
	row := (p.Y - gy) / c.cellH
	if col < 0 || col >= calendarCols || row < 0 || row >= calendarRows {
		return 0
	}
	d := row*calendarCols + col + 1 - c.firstOffset()
	if d < 1 || d > daysIn(c.view.Year(), c.view.Month()) {
		return 0
	}
	return d
}

// KeyAction implements KeyActionHandler: arrows move the selection a
// day (a week for vertical motion) across month boundaries with the
// view following, PageUp/PageDown step the month. Disabled calendars
// ignore keys.
func (c *Calendar) KeyAction(a KeyAction, mods Mods) {
	if !IsEnabled(c) {
		return
	}
	cur := c.sel
	if cur.IsZero() {
		cur = c.view
	}
	switch a {
	case KeyLeft:
		c.Select(cur.AddDate(0, 0, -1))
	case KeyRight:
		c.Select(cur.AddDate(0, 0, 1))
	case KeyUp:
		c.Select(cur.AddDate(0, 0, -7))
	case KeyDown:
		c.Select(cur.AddDate(0, 0, 7))
	case KeyPriorPage:
		c.stepView(0, -1)
	case KeyNextPage:
		c.stepView(0, 1)
	}
}
