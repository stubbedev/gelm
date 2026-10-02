package widget

import (
	"slices"
	"time"

	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/render"
)

// ListModel supplies rows to a List. Row may return a fresh widget per
// call; the List caches the ones it is currently showing. Len and Row
// are re-queried on List.Changed.
type ListModel[W Widget] interface {
	Len() int
	Row(i int) W
}

// SelectionMode is how pointer and keyboard gestures turn into a
// selected row set on a List.
type SelectionMode uint8

const (
	// SelectionSingle keeps zero or one selected row: a click or a
	// plain motion key selects exactly that row, Enter activates it.
	// The zero value, so a List is single-select unless told otherwise.
	SelectionSingle SelectionMode = iota
	// SelectionNone ignores every selection gesture and Select call;
	// rows keep only their own widget behavior. Keyboard selection
	// keys do nothing.
	SelectionNone
	// SelectionBrowse is single-select that never sits empty once it
	// has selected: pointer motion over a row selects it (focus
	// follows the pointer, GTK's browse mode) and gestures cannot
	// clear the selection, only move it.
	SelectionBrowse
	// SelectionMultiple keeps any set of rows. Clicks toggle rows,
	// drags rubber-band select, shift+arrows extend from the anchor,
	// ctrl+arrows move the keyboard cursor without selecting,
	// ctrl+space toggles the cursor row, ctrl+a selects everything.
	SelectionMultiple
)

// List is a virtualized list view: only the rows inside the viewport
// are requested from the model, measured, and painted, so ten
// thousand rows cost about the same as ten. Selection is pointer and
// keyboard driven, clamps at the ends, and Enter activates.
//
// The selection model is SetSelectionMode: single (the default),
// none, browse, or multiple. Selection reports the whole selected set
// and OnSelectionChanged fires once per gesture with it; OnSelect
// keeps its single-row meaning in single and browse mode and stays
// silent in multiple mode.
//
// Pointer gestures land on a row proxy the List keeps over every
// visible row: the proxy forwards what the row widget itself
// implements (clicks, hover and press tracking, tooltips, cursor
// names) while the List owns selection. In selectable modes the
// proxy's drag is the List's (rubber-band in multiple, motion-select
// in single and browse); rows that need their own drag behavior
// belong in a SelectionNone list. In multiple mode a row that
// contains a CheckButton cedes the check to the selection model: the
// check reflects row membership and clicking it toggles membership
// instead of the check's own state.
type List struct {
	node
	model listModel
	rowH  int
	// autoH derives rowH from a styled row (a zero rowHeight): probe is
	// that row, parented below the list so the stylesheets reach it.
	autoH bool
	probe *listRow
	// maxH caps the natural height (SetMaxHeight).
	maxH int
	// pxFrac is pixel scrolling below a whole pixel (ScrollPixels).
	pxFrac float64
	// singleClick activates a row on a plain click (SetSingleClickActivate).
	singleClick bool
	// cellW is the grid mode's minimum cell width (0 is a plain
	// list); cols is how many cells the arranged width fits.
	cellW int
	cols  int

	viewW, viewH int
	offY         int
	hover        int
	rows         map[int]*listRow

	mode   SelectionMode
	sel    int
	multi  map[int]struct{}
	cursor int

	// pointer gesture state: dragging while a press is down, the row
	// the press anchored on (-1 until a point resolves it), the last
	// drag point, whether the band ever left the anchor row (a click
	// after that ends the gesture instead of toggling), and the
	// deferred-notification flag that fires OnSelectionChanged once at
	// gesture end.
	dragging   bool
	gestureRow int
	dragPoint  Point
	dragMoved  bool
	pending    bool

	// drag auto-scroll: direction and the running tween's cancel.
	scrollDir  int
	scrollAnim anim.Cancel

	OnSelect           func(i int)
	OnActivate         func(i int)
	OnSelectionChanged func(rows []int)
}

// Edge bands that engage auto-scroll during a drag, and the time one
// rubber-band auto-scroll step takes per row.
const (
	listEdgeScroll = 24
	listScrollStep = 90 * time.Millisecond
)

// listModel erases the model's type parameter; the List only needs
// widgets out of it.
type listModel interface {
	len() int
	row(i int) Widget
}

// modelAdapter binds a typed model to the erased interface.
type modelAdapter[W Widget] struct {
	m ListModel[W]
}

func (a modelAdapter[W]) len() int { return a.m.Len() }

func (a modelAdapter[W]) row(i int) Widget { return a.m.Row(i) }

// NewList wraps a model with a uniform row height in pixels; zero
// derives the height from measuring the first row.
func NewList[W Widget](model ListModel[W], rowHeight int) *List {
	return &List{
		model: modelAdapter[W]{m: model},
		rowH:  rowHeight,
		autoH: rowHeight <= 0,
		sel:   -1,
		hover: -1,
		rows:  make(map[int]*listRow),
	}
}

// SetCellWidth switches the list into a grid: items flow left to
// right in as many equal columns of at least w pixels as the width
// fits, every line the row height tall, still virtualized by line. Up
// and down move a line, left and right one item. Zero is a plain list.
func (l *List) SetCellWidth(w int) {
	if w == l.cellW {
		return
	}
	l.cellW = max(0, w)
	l.InvalidateLayout()
}

// columns is how many items share a line.
func (l *List) columns() int { return max(1, l.cols) }

// lines is how many lines n items take.
func (l *List) lines(n int) int {
	c := l.columns()
	return (n + c - 1) / c
}

// fitColumns is how many cells of the grid's cell width fit in w.
func (l *List) fitColumns(w int) int {
	if l.cellW <= 0 {
		return 1
	}
	return max(1, w/l.cellW)
}

// cellRect is item i's rect in root coordinates; the last column
// takes the width's remainder.
func (l *List) cellRect(i int) render.Rect {
	c := l.columns()
	w := l.viewW / c
	x := l.bounds.X + (i%c)*w
	if i%c == c-1 {
		w = l.viewW - (c-1)*w
	}
	return render.Rect{X: x, Y: l.bounds.Y + (i/c)*l.rowH - l.offY, W: w, H: l.rowH}
}

// SetSelectionMode switches the selection model. Leaving multiple
// mode keeps the lowest selected row as the single selection (or
// clears it when the set was empty); entering it seeds the set with
// the current selection and anchors there. OnSelectionChanged fires
// when the switch changes the set.
func (l *List) SetSelectionMode(mode SelectionMode) {
	if mode == l.mode {
		return
	}
	before := l.Selection()
	switch mode {
	case SelectionMultiple:
		l.multi = make(map[int]struct{})
		if l.sel >= 0 {
			l.multi[l.sel] = struct{}{}
		}
		l.cursor = l.sel
	default:
		l.sel = -1
		if len(l.multi) > 0 {
			l.sel = l.Selection()[0]
		}
		l.multi = nil
		l.cursor = -1
	}
	l.mode = mode
	l.syncChecks()
	l.Invalidate()
	if after := l.Selection(); !slices.Equal(before, after) {
		l.notifySelection()
	}
}

// SelectionMode returns the current selection model.
func (l *List) SelectionMode() SelectionMode { return l.mode }

// Selection returns the whole selected row set, ascending; an empty
// slice when nothing is selected.
func (l *List) Selection() []int {
	switch l.mode {
	case SelectionMultiple:
		rows := make([]int, 0, len(l.multi))
		for i := range l.multi {
			rows = append(rows, i)
		}
		slices.Sort(rows)
		return rows
	case SelectionBrowse, SelectionSingle:
		if l.sel >= 0 {
			return []int{l.sel}
		}
	}
	return nil
}

// Select moves the selection to row i (clamped, -1 clears) and fires
// OnSelect on change. In multiple mode it collapses the set to the
// one row (-1 clears it); in none mode it does nothing.
func (l *List) Select(i int) {
	switch l.mode {
	case SelectionNone:
		return
	case SelectionMultiple:
		n := l.model.len()
		i = min(max(i, -1), n-1)
		set := make(map[int]struct{})
		if i >= 0 {
			set[i] = struct{}{}
		}
		l.sel, l.cursor = i, i
		l.applyMulti(set)
		l.Invalidate()
		l.notifySelection()
	default:
		l.selectRow(i)
	}
}

// Selected returns the selected row in single and browse mode; in
// multiple mode the anchor row the last gesture or Select settled
// on, -1 when the set is empty. Selection reports the whole set.
func (l *List) Selected() int { return l.sel }

// SelectAll selects every row. It is the ctrl+a target through
// Router.SelectAll and does nothing outside multiple mode, where
// "all" is not a representable selection.
func (l *List) SelectAll() {
	if l.mode != SelectionMultiple {
		return
	}
	n := l.model.len()
	if n == 0 {
		return
	}
	set := make(map[int]struct{}, n)
	for i := range n {
		set[i] = struct{}{}
	}
	l.applyMulti(set)
	l.notifySelection()
}

// selectRow is the single- and browse-mode selection core: clamp,
// move, scroll, repaint, and notify on change. Browse keeps one row:
// a clearing move pins to the first row instead.
func (l *List) selectRow(i int) {
	n := l.model.len()
	i = min(max(i, -1), n-1)
	if l.mode == SelectionBrowse && i < 0 && n > 0 {
		i = 0
	}
	if i == l.sel {
		return
	}
	l.sel = i
	if i >= 0 {
		l.scrollTo(i)
	}
	l.Invalidate()
	if l.OnSelect != nil {
		l.OnSelect(i)
	}
	l.notifySelection()
}

// isSelected reports row membership under the active mode.
func (l *List) isSelected(i int) bool {
	if l.mode == SelectionMultiple {
		_, ok := l.multi[i]
		return ok
	}
	return i == l.sel
}

// applyMulti replaces the multiple-mode set wholesale, syncing the
// checkbox rows and repainting; callers notify.
func (l *List) applyMulti(set map[int]struct{}) {
	l.multi = set
	l.syncChecks()
	l.Invalidate()
}

// toggleRow flips one row's membership and anchors there.
func (l *List) toggleRow(i int) {
	set := make(map[int]struct{}, len(l.multi)+1)
	for j := range l.multi {
		set[j] = struct{}{}
	}
	if _, ok := set[i]; ok {
		delete(set, i)
	} else {
		set[i] = struct{}{}
	}
	l.sel, l.cursor = i, i
	l.applyMulti(set)
	l.notifySelection()
}

// notifySelection fires OnSelectionChanged with the current set once
// per gesture: while a pointer gesture is in flight the notification
// is held and lands at its end (PressEnd), so a rubber-band drag
// that repaints through many intermediate sets still reports exactly
// one, with the full final set.
func (l *List) notifySelection() {
	if l.dragging {
		l.pending = true
		return
	}
	if l.OnSelectionChanged != nil {
		l.OnSelectionChanged(l.Selection())
	}
}

// Changed re-queries the model after its data changed, keeping cached
// row widgets whose indices still exist.
func (l *List) Changed() {
	n := l.model.len()
	for i := range l.rows {
		if i >= n {
			delete(l.rows, i)
		}
	}
	l.offY = min(l.offY, l.scrollMax())
	if l.sel >= n {
		l.sel = n - 1
	}
	if l.cursor >= n {
		l.cursor = n - 1
	}
	for i := range l.multi {
		if i >= n {
			delete(l.multi, i)
		}
	}
	l.syncChecks()
	l.InvalidateLayout()
}

// Refresh re-queries every row widget, keeping the selection and the
// scroll: the rows' data changed in place (a column resized, a
// thumbnail arrived), where Changed would keep showing the cached
// widgets.
func (l *List) Refresh() {
	clear(l.rows)
	l.probe = nil
	l.Changed()
}

// Reset is for a model whose rows were replaced wholesale (a folder
// listing for another folder): every cached row widget, the selection
// and the scroll go, and OnSelectionChanged fires when a selection was
// dropped.
func (l *List) Reset() {
	had := len(l.Selection()) > 0
	clear(l.rows)
	l.probe = nil
	l.offY, l.sel, l.cursor, l.hover = 0, -1, -1, -1
	if l.multi != nil {
		l.multi = map[int]struct{}{}
	}
	l.Changed()
	if had {
		l.notifySelection()
	}
}

// scrollTo nudges the offset so row i is inside the viewport.
func (l *List) scrollTo(i int) {
	top, bottom := l.offY, l.offY+l.viewH
	y := (i / l.columns()) * l.rowH
	if y < top {
		l.offY = y
	} else if y+l.rowH > bottom {
		l.offY = y + l.rowH - l.viewH
	}
}

// measureRowH is the row height: the fixed one, or the first row's as
// styled - measured on a probe row parented below the list, so the
// stylesheet's padding and fonts count (GTK sizes rows by their
// styled natural height), and re-measured as they change.
func (l *List) measureRowH() int {
	if !l.autoH {
		return max(1, l.rowH)
	}
	if l.model.len() == 0 {
		l.rowH = 1
		return 1
	}
	if l.probe == nil {
		l.probe = l.newRow(0)
	}
	setParents(l, l.probe)
	w := l.viewW
	if w <= 0 {
		w = 1 << 20 // not arranged yet: the natural width
	}
	s := l.probe.Measure(Constraints{Max: Size{W: w, H: 1 << 20}})
	l.rowH = max(1, s.H)
	return l.rowH
}

// Measure reports the offered width by the full content height,
// clamped to the constraints: the list is the viewport, not the
// content.
func (l *List) Measure(con Constraints) Size {
	if sz, ok := l.measureHit(con); ok {
		return sz
	}
	lines, w := l.model.len(), 120
	if l.cellW > 0 {
		c := l.fitColumns(con.Max.W)
		lines, w = (lines+c-1)/c, l.cellW
	}
	h := l.measureRowH() * lines
	if l.maxH > 0 {
		h = min(h, l.maxH)
	}
	return l.measureStore(con, clampSize(Size{W: w, H: h}, con))
}

// SetMaxHeight caps the list's natural height at h pixels (0 lifts the
// cap): a longer model scrolls inside it, still virtualized - GTK's
// list in a scrolled window with a max content height.
func (l *List) SetMaxHeight(h int) {
	if l.maxH == h {
		return
	}
	l.maxH = max(h, 0)
	l.InvalidateLayout()
}

// Arrange lays out the viewport.
func (l *List) Arrange(r render.Rect) {
	l.node.Arrange(r)
	l.viewW, l.viewH = r.W, r.H
	l.cols = l.fitColumns(r.W)
	l.measureRowH()
	l.offY = min(l.offY, l.scrollMax())
}

// visible returns the half-open range of rows inside the viewport and
// caches their proxy widgets, evicting everything off-screen.
func (l *List) visible() (first, last int) {
	n, c := l.model.len(), l.columns()
	line := l.offY / l.rowH
	first = min(line*c, max(0, n-1))
	last = min(n, (line+l.viewH/l.rowH+1)*c)
	for i := range l.rows {
		if i < first || i >= last {
			delete(l.rows, i)
		}
	}
	for i := first; i < last; i++ {
		if _, ok := l.rows[i]; !ok {
			l.rows[i] = l.newRow(i)
		}
	}
	return first, last
}

// newRow wraps model row i in its proxy, claiming the checkbox of a
// checkbox row in multiple mode and syncing it to membership.
func (l *List) newRow(i int) *listRow {
	r := &listRow{list: l, idx: i, row: l.model.row(i)}
	if l.mode == SelectionMultiple {
		r.check = findCheckButton(r.row)
		if r.check != nil {
			r.check.SetChecked(l.isSelected(i))
		}
	}
	return r
}

// syncChecks mirrors membership into the checkbox rows currently in
// the cache; virtualization means only visible checks exist to sync,
// and newRow syncs the rest as they scroll in.
func (l *List) syncChecks() {
	if l.mode != SelectionMultiple {
		return
	}
	for i, r := range l.rows {
		if r.check != nil {
			r.check.SetChecked(l.isSelected(i))
		}
	}
}

// findCheckButton returns the first CheckButton at or below w.
func findCheckButton(w Widget) *CheckButton {
	if c, ok := w.(*CheckButton); ok {
		return c
	}
	if c, ok := w.(childser); ok {
		for _, k := range c.Children() {
			if found := findCheckButton(k); found != nil {
				return found
			}
		}
	}
	return nil
}

// Paint draws the viewport background, the selection, cursor, and
// hover bands, and exactly the visible rows.
func (l *List) Paint(cv *render.Canvas) {
	th := Current()
	prev := cv.PushClip(l.bounds)
	cv.FillRect(l.bounds, th.Surface)

	first, last := l.visible()
	for i := first; i < last; i++ {
		w := l.rows[i]
		rect := l.cellRect(i)
		// Parent first: the row measures and styles inside Arrange, and
		// its cascade must reach the stylesheets above the list.
		setParents(l, w)
		w.Arrange(rect)
		switch {
		case l.isSelected(i):
			hl := th.Accent
			cv.FillRect(rect, render.RGBA(hl.R(), hl.G(), hl.B(), 70))
		case i == l.hover:
			tint := th.SurfaceHover
			cv.FillRect(rect, render.RGBA(tint.R(), tint.G(), tint.B(), 120))
		}
		if l.mode == SelectionMultiple && i == l.cursor {
			cv.BorderRect(rect, 1, th.Accent)
		}
		PaintChild(cv, w)
	}
	cv.PopClip(prev)
}

// Role implements Roleer.
func (l *List) Role() Role { return RoleList }

// HitTest returns the row proxy under p, or the list itself.
func (l *List) HitTest(p Point) Widget {
	if !l.bounds.Contains(p.X, p.Y) {
		return nil
	}
	i := l.rowAt(p)
	if w, ok := l.rows[i]; ok {
		return w
	}
	return l
}

// rowAt maps a point to a row index, clamped into the model.
func (l *List) rowAt(p Point) int {
	line := max((p.Y-l.bounds.Y+l.offY)/l.rowH, 0)
	return min(line*l.columns()+l.columnAt(p), l.model.len()-1)
}

// columnAt maps a point to its grid column, clamped.
func (l *List) columnAt(p Point) int {
	c := l.columns()
	return min(max((p.X-l.bounds.X)/max(1, l.viewW/c), 0), c-1)
}

// rowAtExact maps a point to a row index only when one is under it:
// the trailing band below the last row hits no row, where rowAt
// clamps into the last one.
func (l *List) rowAtExact(p Point) (int, bool) {
	y := p.Y - l.bounds.Y + l.offY
	if y < 0 {
		return 0, false
	}
	i := (y/l.rowH)*l.columns() + l.columnAt(p)
	if i >= l.model.len() {
		return 0, false
	}
	return i, true
}

// gestureStart records the press that begins a pointer gesture; row
// -1 defers the anchor to the first point that resolves one.
func (l *List) gestureStart(row int) {
	l.dragging = true
	l.dragMoved = false
	l.gestureRow = row
}

// gestureEnd closes a pointer gesture: auto-scroll stops and a held
// notification lands, exactly once, with the full final set.
func (l *List) gestureEnd() {
	if !l.dragging {
		return
	}
	l.dragging = false
	l.stopAutoScroll()
	if !l.pending {
		return
	}
	l.pending = false
	if l.OnSelectionChanged != nil {
		l.OnSelectionChanged(l.Selection())
	}
}

// rowClick applies the click half of a pointer gesture on row r.idx.
// A click after the band left the anchor row ends the gesture without
// toggling: the drag already applied its selection. Checkbox rows in
// multiple mode toggle membership through the check and do not reach
// the row's own click handling; every other row forwards to it after
// the selection applies, so interactive rows keep working.
func (l *List) rowClick(r *listRow, p Point) {
	if !IsEnabled(r.row) {
		return
	}
	if l.dragMoved {
		return
	}
	switch l.mode {
	case SelectionSingle, SelectionBrowse:
		l.selectRow(r.idx)
	case SelectionMultiple:
		l.toggleRow(r.idx)
	}
	if l.singleClick && l.mode != SelectionMultiple {
		l.activate(r.idx)
	}
	if r.check == nil || l.mode != SelectionMultiple {
		if c, ok := r.row.(Clicker); ok {
			c.ClickAt(p)
		}
	}
}

// rowDoubleClick applies the second click of a pair: the first click
// already applied the selection, so the pair activates the row
// (double-click opens, GTK-style) instead of toggling it back off.
func (l *List) rowDoubleClick(r *listRow, p Point) {
	if !IsEnabled(r.row) || l.mode == SelectionNone {
		return
	}
	l.activate(r.idx)
	if d, ok := r.row.(DoubleClicker); ok {
		d.DoubleClickAt(p)
	}
}

// dragApply applies the motion half of a pointer gesture at p:
// rubber-band from the anchor row in multiple mode, motion-select in
// single and browse. The band remembers leaving the anchor row, and
// the edge bands engage auto-scroll.
func (l *List) dragApply(p Point) {
	l.dragPoint = p
	if l.gestureRow < 0 {
		if i, ok := l.rowAtExact(p); ok {
			l.gestureRow = i
		} else {
			return
		}
	}
	if row := l.rowAt(p); row != l.gestureRow {
		l.dragMoved = true
	}
	l.applyDragSelection()
	l.updateAutoScroll()
}

// applyDragSelection applies the pointer gesture's selection at the
// current drag point: rubber-band from the anchor in multiple mode,
// motion-select in single and browse. Auto-scroll frames call it with
// the scrolled offset already in place, so it never touches the
// scroll state.
func (l *List) applyDragSelection() {
	row := l.rowAt(l.dragPoint)
	switch l.mode {
	case SelectionMultiple:
		lo, hi := min(l.gestureRow, row), max(l.gestureRow, row)
		set := make(map[int]struct{}, hi-lo+1)
		for i := lo; i <= hi; i++ {
			set[i] = struct{}{}
		}
		l.applyMulti(set)
		l.notifySelection()
	case SelectionSingle, SelectionBrowse:
		l.selectRow(row)
	}
}

// stopAutoScroll drops the drag auto-scroll tween, if any.
func (l *List) stopAutoScroll() {
	if l.scrollAnim != nil {
		l.scrollAnim()
		l.scrollAnim = nil
	}
	l.scrollDir = 0
}

// scrollMax is the largest viewport offset.
func (l *List) scrollMax() int {
	return max(0, l.lines(l.model.len())*l.rowH-l.viewH)
}

// updateAutoScroll runs or stops the edge auto-scroll for the current
// drag point: one row per step while the pointer sits in an edge band
// (or beyond it), nothing elsewhere or when the offset is already
// pinned at that end. Under reduced motion (anim.Instant) each call
// steps one row discretely instead of animating, so no tween runs.
func (l *List) updateAutoScroll() {
	if !l.dragging {
		l.stopAutoScroll()
		return
	}
	dir := 0
	switch {
	case l.dragPoint.Y < l.bounds.Y+listEdgeScroll:
		dir = -1
	case l.dragPoint.Y >= l.bounds.Y+l.bounds.H-listEdgeScroll:
		dir = 1
	}
	row := max(1, l.rowH)
	if dir == 0 || l.offY == min(max(0, l.offY+dir*row), l.scrollMax()) {
		l.stopAutoScroll()
		return
	}
	if anim.Instant() {
		l.offY = min(max(0, l.offY+dir*row), l.scrollMax())
		l.applyDragSelection()
		l.Invalidate()
		return
	}
	if l.scrollAnim != nil && l.scrollDir == dir {
		return
	}
	l.stopAutoScroll()
	l.scrollDir = dir
	base := l.offY
	l.scrollAnim = anim.Play(anim.Animate(listScrollStep, func(t float64) {
		l.offY = min(max(0, base+int(float64(dir*row)*t)), l.scrollMax())
		l.applyDragSelection()
		l.Invalidate()
		if t >= 1 {
			l.scrollAnim = nil
			l.updateAutoScroll()
		}
	}).Easing(anim.Linear))
}

// HoverMove implements HoverMover: tracks the hovered row for styling
// and per-row tooltips. Browse mode selects the hovered row (focus
// follows the pointer). Disabled lists highlight nothing.
func (l *List) HoverMove(p Point) {
	if !IsEnabled(l) {
		return
	}
	i := l.rowAt(p)
	if i == l.hover {
		return
	}
	l.hover = i
	l.Invalidate()
	if l.mode == SelectionBrowse && i >= 0 {
		l.selectRow(i)
	}
}

// ScrollBy implements ScrollHandler. Disabled lists do not scroll.
func (l *List) ScrollBy(dx, dy int) {
	if !IsEnabled(l) {
		return
	}
	l.offY = min(max(0, l.offY+dy*scrollStepPx), l.scrollMax())
	l.Invalidate()
}

// ClickAt applies a press-and-release on the list itself: a row hit
// that no proxy covers (a partially visible last row, say) selects
// that row per the mode.
func (l *List) ClickAt(p Point) {
	if !IsEnabled(l) {
		return
	}
	if i, ok := l.rowAtExact(p); ok {
		l.rowClick(&listRow{list: l, idx: i, row: l.model.row(i)}, p)
	}
}

// DragMove implements DragMover for presses on the list itself; see
// dragApply.
func (l *List) DragMove(p Point) {
	if !IsEnabled(l) {
		return
	}
	l.dragApply(p)
}

// SetPressed implements PressSetter: true begins a pointer gesture on
// the list itself, false only clears the row's own pressed tracking.
func (l *List) SetPressed(on bool) {
	if on {
		l.gestureStart(-1)
	}
}

// PressEnd implements PressEnder: closes the gesture started by the
// press, landing the held selection notification.
func (l *List) PressEnd() {
	l.gestureEnd()
}

// KeyAction implements KeyActionHandler: selection motion clamps at
// the ends (pinned), PageUp/PageDown move a viewport of rows, and
// Enter activates exactly once per press. Multiple mode adds the
// cursor/anchor gestures; none mode ignores selection keys. Disabled
// lists ignore keys.
func (l *List) KeyAction(a KeyAction, mods Mods) {
	if !IsEnabled(l) {
		return
	}
	n := l.model.len()
	if n == 0 || l.mode == SelectionNone {
		return
	}
	if l.mode == SelectionMultiple {
		l.keyActionMultiple(a, mods, n)
		return
	}
	line, item := l.steps()
	page := max(1, l.viewH/l.rowH) * line
	switch a {
	case KeyUp:
		l.Select(max(0, l.sel-line))
	case KeyDown:
		l.Select(min(n-1, l.sel+line))
	case KeyLeft:
		l.Select(max(0, l.sel-item))
	case KeyRight:
		l.Select(min(n-1, l.sel+item))
	case KeyHome:
		l.Select(0)
	case KeyEnd:
		l.Select(n - 1)
	case KeyPriorPage:
		l.Select(max(0, l.sel-page))
	case KeyNextPage:
		l.Select(min(n-1, l.sel+page))
	case KeyEnter:
		l.activate(l.sel)
	}
}

// keyActionMultiple routes the multiple-mode keyboard: plain motion
// keys collapse to the cursor row, shift extends the anchor range,
// ctrl moves the cursor without selecting, ctrl+space toggles the
// cursor row, and Enter activates every selected row once.
func (l *List) keyActionMultiple(a KeyAction, mods Mods, n int) {
	line, item := l.steps()
	page := max(1, l.viewH/l.rowH) * line
	ctrl := mods&ModCtrl != 0
	shift := mods&ModShift != 0
	move := func(target int) {
		target = min(max(target, 0), n-1)
		l.cursor = target
		switch {
		case ctrl && !shift:
			l.Invalidate()
		case shift:
			set := make(map[int]struct{})
			for i := min(l.sel, target); i <= max(l.sel, target); i++ {
				set[i] = struct{}{}
			}
			l.applyMulti(set)
			l.notifySelection()
		default:
			l.sel = target
			set := map[int]struct{}{target: {}}
			l.applyMulti(set)
			l.notifySelection()
		}
		l.scrollTo(target)
	}
	switch a {
	case KeyUp:
		move(l.cursor - line)
	case KeyDown:
		move(l.cursor + line)
	case KeyLeft:
		if item > 0 {
			move(l.cursor - item)
		}
	case KeyRight:
		if item > 0 {
			move(l.cursor + item)
		}
	case KeyHome:
		move(0)
	case KeyEnd:
		move(n - 1)
	case KeyPriorPage:
		move(l.cursor - page)
	case KeyNextPage:
		move(l.cursor + page)
	case KeySpace:
		if ctrl {
			l.toggleRow(l.cursor)
		}
	case KeyEnter:
		for _, i := range l.Selection() {
			l.activate(i)
		}
	}
}

// steps is how far up/down and left/right move: a line and nothing in
// a list, a line of cells and one cell in a grid.
func (l *List) steps() (line, item int) {
	if l.cellW > 0 {
		return l.columns(), 1
	}
	return 1, 0
}

// activate fires OnActivate for one row when it exists.
func (l *List) activate(i int) {
	if i >= 0 && i < l.model.len() && l.OnActivate != nil {
		l.OnActivate(i)
	}
}

// listRow is the interactive proxy the List puts over every visible
// row widget: pointer gestures land on it (the List returns it from
// HitTest), and it forwards what the row itself implements while the
// List owns selection.
type listRow struct {
	node
	list  *List
	idx   int
	row   Widget
	check *CheckButton
}

// Measure delegates to the row widget, parented first so it measures
// as styled.
func (r *listRow) Measure(con Constraints) Size {
	setParents(r, r.row)
	return r.row.Measure(con)
}

// Arrange records the proxy's rect and lays the row widget out in the
// same rect, linking it below the proxy for the ancestor walks. The
// row is measured first: no Measure pass reaches it (the list measures
// as a viewport), and containers arrange from what they measured.
func (r *listRow) Arrange(rect render.Rect) {
	r.node.Arrange(rect)
	setParents(r, r.row)
	r.row.Measure(Constraints{Max: Size{W: rect.W, H: rect.H}})
	r.row.Arrange(rect)
}

// Paint delegates to the row widget.
func (r *listRow) Paint(cv *render.Canvas) { PaintChild(cv, r.row) }

// HitTest returns the proxy while p is inside its band.
func (r *listRow) HitTest(p Point) Widget { return r.HitLeaf(r, p) }

// ClickAt applies the list's click selection, then the row's own.
func (r *listRow) ClickAt(p Point) { r.list.rowClick(r, p) }

// DoubleClickAt activates the row (double-click opens) and forwards
// to the row's own double-click handling. The first click of the pair
// already applied the selection; the second does not toggle it back.
func (r *listRow) DoubleClickAt(p Point) { r.list.rowDoubleClick(r, p) }

// DragMove applies the list's drag selection; in none mode the row's
// own drag handling runs instead.
func (r *listRow) DragMove(p Point) {
	if r.list.mode == SelectionNone {
		if d, ok := r.row.(DragMover); ok {
			d.DragMove(p)
		}
		return
	}
	r.list.dragApply(p)
}

// SetPressed tracks the row's own pressed state and opens the list's
// pointer gesture on the press.
func (r *listRow) SetPressed(on bool) {
	if pr, ok := r.row.(PressSetter); ok {
		pr.SetPressed(on)
	}
	if on {
		r.list.gestureStart(r.idx)
	}
}

// PressEnd closes the list's pointer gesture.
func (r *listRow) PressEnd() { r.list.gestureEnd() }

// SetHovered forwards hover tracking to the row widget.
func (r *listRow) SetHovered(on bool) {
	if h, ok := r.row.(HoverSetter); ok {
		h.SetHovered(on)
	}
}

// HoverMove feeds the list's hover band, then the row's own handling.
func (r *listRow) HoverMove(p Point) {
	r.list.HoverMove(p)
	if h, ok := r.row.(HoverMover); ok {
		h.HoverMove(p)
	}
}

// TooltipText returns the row widget's tooltip, the proxy's own when
// the row carries none.
func (r *listRow) TooltipText() string {
	if t, ok := r.row.(TooltipTexter); ok {
		return t.TooltipText()
	}
	return r.tooltip
}

// CursorName forwards the row widget's cursor shape.
func (r *listRow) CursorName() string {
	if c, ok := r.row.(CursorNamer); ok {
		return c.CursorName()
	}
	return ""
}

// KeyAction forwards keyboard actions into the list: a press on a row
// focuses the proxy, and the list owns the keyboard selection model.
func (r *listRow) KeyAction(a KeyAction, mods Mods) { r.list.KeyAction(a, mods) }

// SelectAll forwards the ctrl+a select-all route into the list.
func (r *listRow) SelectAll() { r.list.SelectAll() }

// styleChildren are the materialized rows and the height probe
// (styleKids).
func (l *List) styleChildren() []Widget {
	out := make([]Widget, 0, len(l.rows)+1)
	if l.probe != nil {
		out = append(out, l.probe)
	}
	for _, r := range l.rows {
		out = append(out, r)
	}
	return out
}

// styleChildren is the row widget, inheriting through the proxy.
func (r *listRow) styleChildren() []Widget { return []Widget{r.row} }

// SetSingleClickActivate makes a plain click activate the row
// (OnActivate) as well as select it, GTK's single-click-activate: a
// picker's rows act on one click. Multiple-selection lists keep
// clicks for membership.
func (l *List) SetSingleClickActivate(on bool) { l.singleClick = on }

// ScrollPixels implements PixelScroller: exact vertical deltas, the
// fraction kept for the next one.
func (l *List) ScrollPixels(_, dy float64) {
	if !IsEnabled(l) {
		return
	}
	l.pxFrac += dy
	w := int(l.pxFrac)
	l.pxFrac -= float64(w)
	if w != 0 {
		l.offY = min(max(0, l.offY+w), l.scrollMax())
		l.Invalidate()
	}
}
