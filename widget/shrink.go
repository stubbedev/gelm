package widget

// Shrinker is a widget that can be arranged shorter than its natural
// height and still do its job: a Scroll scrolls what no longer fits.
// A Column Box short of room takes the shortfall from its expanding
// children that can give it — GTK's ScrolledWindow giving way inside a
// fixed-size popover — never below each one's floor; whatever still
// does not fit overflows as before. Boxes and stacks report what their
// children can give, so the shrink reaches a Scroll nested inside
// them.
type Shrinker interface {
	// Shrinkable is how many pixels of its last measured height the
	// widget can give up.
	Shrinkable() int
}

// shrinkableOf is w's Shrinkable, 0 for a widget that cannot shrink.
func shrinkableOf(w Widget) int {
	if s, ok := w.(Shrinker); ok {
		return max(0, s.Shrinkable())
	}
	return 0
}

// Shrinkable implements Shrinker: a column gives what its expanding
// children can; a row is as short as its least shrinkable child lets
// it be.
func (b *Box) Shrinkable() int {
	if b.axis == Column {
		total := 0
		for _, c := range b.child {
			if c.expand && IsVisible(c.w) {
				total += shrinkableOf(c.w)
			}
		}
		return total
	}
	tallest, floor := 0, 0
	for _, c := range b.child {
		if !IsVisible(c.w) {
			continue
		}
		tallest = max(tallest, c.nat.H)
		floor = max(floor, c.nat.H-shrinkableOf(c.w))
	}
	return tallest - floor
}

// WidthShrinker is Shrinker's horizontal counterpart: a widget that
// can be arranged narrower than its natural width and still read — an
// ellipsizing label truncates. A Row Box short of room takes the
// shortfall from its expanding children that can give it, as GTK's
// box gives way down to an ellipsized label's minimum, never below
// each one's floor; whatever still does not fit overflows as before.
type WidthShrinker interface {
	// ShrinkableWidth is how many pixels of its last measured width the
	// widget can give up.
	ShrinkableWidth() int
}

// shrinkableWidthOf is w's ShrinkableWidth, 0 for a widget that cannot
// narrow.
func shrinkableWidthOf(w Widget) int {
	if s, ok := w.(WidthShrinker); ok {
		return max(0, s.ShrinkableWidth())
	}
	return 0
}

// ShrinkableWidth implements WidthShrinker: a row gives what its
// expanding children can; a column is as narrow as its least
// shrinkable child lets it be.
func (b *Box) ShrinkableWidth() int {
	if b.axis == Row {
		total := 0
		for _, c := range b.child {
			if c.expand && IsVisible(c.w) {
				total += shrinkableWidthOf(c.w)
			}
		}
		return total
	}
	widest, floor := 0, 0
	for _, c := range b.child {
		if !IsVisible(c.w) {
			continue
		}
		widest = max(widest, c.nat.W)
		floor = max(floor, c.nat.W-shrinkableWidthOf(c.w))
	}
	return widest - floor
}

// shrinkTakes splits a main-axis deficit across the box's expanding
// children in proportion to what each can give, never more than that:
// takes[i] is what child i gives up. A column takes height, a row
// width.
func (b *Box) shrinkTakes(deficit int) []int {
	if deficit <= 0 {
		return nil
	}
	give := shrinkableOf
	if b.axis == Row {
		give = shrinkableWidthOf
	}
	caps := make([]int, len(b.child))
	total := 0
	for i, c := range b.child {
		if c.expand && IsVisible(c.w) {
			caps[i] = give(c.w)
			total += caps[i]
		}
	}
	if total == 0 {
		return nil
	}
	deficit = min(deficit, total)
	takes := make([]int, len(b.child))
	given := 0
	for i, c := range caps {
		takes[i] = deficit * c / total
		given += takes[i]
	}
	// The rounding remainder goes to whoever still has room, in order.
	for i := range takes {
		if given == deficit {
			break
		}
		if more := min(caps[i]-takes[i], deficit-given); more > 0 {
			takes[i] += more
			given += more
		}
	}
	return takes
}

// Shrinkable implements Shrinker: every page is arranged in the same
// rect, so the stack can shrink as far as its least shrinkable page
// lets it.
func (s *Stack) Shrinkable() int {
	tallest, floor := 0, 0
	for _, name := range s.order {
		nat := s.measured[name]
		tallest = max(tallest, nat.H)
		floor = max(floor, nat.H-shrinkableOf(s.kids[name]))
	}
	return tallest - floor
}

// scrollFloor is the least height a Scroll gives way to: room for the
// bar to still read as one.
const scrollFloor = 4 * gutter

// Shrinkable implements Shrinker: a scroll gives up everything above
// its floor and scrolls the rest.
func (s *Scroll) Shrinkable() int { return max(0, s.measured.H-scrollFloor) }
