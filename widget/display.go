package widget

import (
	"github.com/stubbedev/lys-ui/render"
	"github.com/stubbedev/lys-ui/widget/css"
)

// BadgeVariant picks a badge's look, the shadcn/ui variants.
type BadgeVariant uint8

// The badge variants.
const (
	// BadgeDefault is the primary fill.
	BadgeDefault BadgeVariant = iota
	// BadgeSecondary is the quiet fill.
	BadgeSecondary
	// BadgeDestructive is the destructive fill.
	BadgeDestructive
	// BadgeOutline is a hairline on the background.
	BadgeOutline
)

// badgeClasses are the variants' style classes on the badge element.
var badgeClasses = [...]string{BadgeDefault: "", BadgeSecondary: css.Secondary, BadgeDestructive: css.Destructive, BadgeOutline: css.Outline}

// Badge is a small inline pill label - a status, a count, a tag - with
// an optional leading icon (shadcn/ui's Badge). It styles as the
// `badge` element with the variant's class.
type Badge struct {
	composite
	row     *Box
	label   *Label
	icon    Widget
	variant BadgeVariant
}

// NewBadge returns a default-variant badge reading text.
func NewBadge(face render.Font, text string) *Badge {
	face = requireFace("widget.NewBadge", face)
	b := &Badge{row: NewBox(Row, 4, 0)}
	b.row.SetElement("badge")
	b.label = NewLabel(face, 12, text, 0)
	b.row.AppendAligned(b.label, false, AlignCenter)
	b.initComposite(b, b.row)
	return b
}

// SetText replaces the badge's text.
func (b *Badge) SetText(text string) { b.label.SetText(text) }

// Text returns the badge's text.
func (b *Badge) Text() string { return b.label.Text() }

// SetVariant switches the badge's look.
func (b *Badge) SetVariant(v BadgeVariant) {
	if old := badgeClasses[b.variant]; old != "" {
		b.row.RemoveClass(old)
	}
	if c := badgeClasses[v]; c != "" {
		b.row.AddClass(c)
	}
	b.variant = v
}

// Variant returns the badge's look.
func (b *Badge) Variant() BadgeVariant { return b.variant }

// SetIcon puts w (an icon, typically 12px) before the text; nil
// removes it.
func (b *Badge) SetIcon(w Widget) {
	if b.icon != nil {
		b.row.Remove(b.icon)
	}
	b.icon = w
	if w != nil {
		b.row.InsertAligned(0, w, false, AlignCenter)
	}
	b.InvalidateLayout()
}

// NewKbd draws one key as a keycap (shadcn/ui's Kbd): a .keycap label.
func NewKbd(face render.Font, key string) *Label {
	l := NewLabel(requireFace("widget.NewKbd", face), 12, key, 0)
	l.AddClass(css.Keycap)
	return l
}

// NewKbdGroup draws a key combination as keycaps side by side
// (shadcn/ui's KbdGroup): NewKbdGroup(face, "Ctrl", "Shift", "P").
func NewKbdGroup(face render.Font, keys ...string) *Box {
	row := NewBox(Row, 3, 0)
	for _, k := range keys {
		row.AppendAligned(NewKbd(face, k), false, AlignCenter)
	}
	return row
}

// NewSkeleton is a w x h placeholder block that pulses while content
// loads (shadcn/ui's Skeleton): the `skeleton` element, a muted
// rounded fill whose pulse stops under reduced motion.
func NewSkeleton(w, h int) *Box {
	b := NewBox(Column, 0, 0)
	b.SetElement("skeleton")
	b.SetSizeRequest(w, h)
	return b
}

// NewBulletList is a typography list: each item a wrapped label behind
// a bullet, indented like shadcn/ui's ul.
func NewBulletList(face render.Font, sizePx float64, items ...string) *Box {
	face = requireFace("widget.NewBulletList", face)
	list := NewBox(Column, 6, 0)
	list.SetElement("bullets")
	for _, it := range items {
		row := NewBox(Row, 10, 0)
		row.AppendAligned(NewLabel(face, sizePx, "•", 0), false, AlignStart)
		item := NewLabel(face, sizePx, it, 0)
		item.SetWrap(true)
		row.Append(item, true)
		list.Append(row, false)
	}
	return list
}
