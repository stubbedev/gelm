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
		sel:   -1,
		hover: -1,
		rows:  make(map[int]*listRow),
	}
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
	l.offY = min(l.offY, max(0, n*l.rowH-l.viewH))
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

// scrollTo nudges the offset so row i is inside the viewport.
func (l *List) scrollTo(i int) {
	top, bottom := l.offY, l.offY+l.viewH
	y := i * l.rowH
	if y < top {
		l.offY = y
	} else if y+l.rowH > bottom {
		l.offY = y + l.rowH - l.viewH
	}
}

// measureRowH derives the row height from the first row once.
func (l *List) measureRowH() int {
	if l.rowH > 0 {
		return l.rowH
	}
	if l.model.len() == 0 {
		return 1
	}
	s := l.model.row(0).Measure(Constraints{Max: Size{W: 1 << 20, H: 1 << 20}})
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
	h := l.measureRowH() * l.model.len()
	return l.measureStore(con, clampSize(Size{W: 120, H: h}, con))
}

// Arrange lays out the viewport.
func (l *List) Arrange(r render.Rect) {
	l.node.Arrange(r)
	l.viewW, l.viewH = r.W, r.H
	l.measureRowH()
	l.offY = min(l.offY, max(0, l.model.len()*l.rowH-l.viewH))
}

// visible returns the half-open range of rows inside the viewport and
// caches their proxy widgets, evicting everything off-screen.
func (l *List) visible() (first, last int) {
	n := l.model.len()
	first = min(l.offY/l.rowH, max(0, n-1))
	last = min(n, first+l.viewH/l.rowH+1)
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
		y := l.bounds.Y + i*l.rowH - l.offY
		rect := render.Rect{X: l.bounds.X, Y: y, W: l.viewW, H: l.rowH}
		w.Arrange(rect)
		setParents(l, w)
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
		w.Paint(cv)
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
	return min(max((p.Y-l.bounds.Y+l.offY)/l.rowH, 0), l.model.len()-1)
}

// rowAtExact maps a point to a row index only when one is under it:
// the trailing band below the last row hits no row, where rowAt
// clamps into the last one.
func (l *List) rowAtExact(p Point) (int, bool) {
	n := l.model.len()
	i := (p.Y - l.bounds.Y + l.offY) / l.rowH
	if i < 0 || i >= n {
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
	return max(0, l.model.len()*l.rowH-l.viewH)
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
	l.offY = min(max(0, l.offY+dy*40), l.scrollMax())
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
	page := max(1, l.viewH/l.rowH)
	switch a {
	case KeyUp:
		l.Select(max(0, l.sel-1))
	case KeyDown:
		l.Select(min(n-1, l.sel+1))
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
	page := max(1, l.viewH/l.rowH)
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
		move(l.cursor - 1)
	case KeyDown:
		move(l.cursor + 1)
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

// Measure delegates to the row widget.
func (r *listRow) Measure(con Constraints) Size { return r.row.Measure(con) }

// Arrange records the proxy's rect and arranges the row widget into
// the same rect, linking it below the proxy for the ancestor walks.
func (r *listRow) Arrange(rect render.Rect) {
	r.node.Arrange(rect)
	setParents(r, r.row)
	r.row.Arrange(rect)
}

// Paint delegates to the row widget.
func (r *listRow) Paint(cv *render.Canvas) { r.row.Paint(cv) }

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
