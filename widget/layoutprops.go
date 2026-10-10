package widget

import "github.com/stubbedev/gelm/render"

type expandSetting uint8

const (
	expandInherit expandSetting = iota
	expandOn
	expandOff
)

type layoutProps struct {
	hexpand, vexpand expandSetting
	halign, valign   Align
	margin           render.Insets
	expandValid      bool
	expandH, expandV bool
}

// SetHExpand says whether the widget wants extra horizontal space,
// GTK's hexpand: a row Box gives it a share of its leftover width.
// Unset, a widget wants it when a visible child does.
func (n *node) SetHExpand(on bool) { n.setExpand(&n.layout.hexpand, on) }

// SetVExpand is SetHExpand for vertical space.
func (n *node) SetVExpand(on bool) { n.setExpand(&n.layout.vexpand, on) }

func (n *node) setExpand(s *expandSetting, on bool) {
	want := expandOff
	if on {
		want = expandOn
	}
	if *s == want {
		return
	}
	*s = want
	n.InvalidateLayout()
}

// SetHAlign places the widget horizontally inside the space its parent
// gives it, GTK's halign: AlignFill (the default) stretches it, the
// others keep its natural width at the start, center or end.
func (n *node) SetHAlign(a Align) {
	if n.layout.halign != a {
		n.layout.halign = a
		n.InvalidateLayout()
	}
}

// SetVAlign is SetHAlign for the vertical axis.
func (n *node) SetVAlign(a Align) {
	if n.layout.valign != a {
		n.layout.valign = a
		n.InvalidateLayout()
	}
}

// SetMargin sets the space kept clear around the widget inside its
// parent's allocation, GTK's margin-*. It adds to a stylesheet margin.
func (n *node) SetMargin(m render.Insets) {
	if n.layout.margin != m {
		n.layout.margin = m
		n.InvalidateLayout()
	}
}

// HAlign returns the horizontal placement.
func (n *node) HAlign() Align { return n.layout.halign }

// VAlign returns the vertical placement.
func (n *node) VAlign() Align { return n.layout.valign }

// Margin returns the programmatic margin.
func (n *node) Margin() render.Insets { return n.layout.margin }

func (n *node) layoutNode() *node { return n }

type layoutCarrier interface{ layoutNode() *node }

func layoutOf(w Widget) *layoutProps {
	if c, ok := w.(layoutCarrier); ok {
		return &c.layoutNode().layout
	}
	return nil
}

// WantsExpand reports whether w wants extra space on axis: its own
// setting when set, else whether any visible child wants it, GTK's
// computed expand. The result is cached until the subtree's layout
// changes.
func WantsExpand(w Widget, axis Axis) bool {
	l := layoutOf(w)
	if l == nil {
		return false
	}
	if !l.expandValid {
		l.expandH = computeExpand(w, l.hexpand, Row)
		l.expandV = computeExpand(w, l.vexpand, Column)
		l.expandValid = true
	}
	if axis == Row {
		return l.expandH
	}
	return l.expandV
}

func computeExpand(w Widget, s expandSetting, axis Axis) bool {
	switch s {
	case expandOn:
		return true
	case expandOff:
		return false
	}
	p, ok := w.(interface{ Children() []Widget })
	if !ok {
		return false
	}
	for _, c := range p.Children() {
		if IsVisible(c) && WantsExpand(c, axis) {
			return true
		}
	}
	return false
}

func (l *layoutProps) dropExpand() { l.expandValid = false }

func layoutMarginOf(w Widget) render.Insets {
	if l := layoutOf(w); l != nil {
		return l.margin
	}
	return render.Insets{}
}

func measureWithMargin(w Widget, con Constraints) Size {
	m := layoutMarginOf(w)
	if m.Zero() {
		return w.Measure(con)
	}
	mw, mh := m.Left+m.Right, m.Top+m.Bottom
	inner := Constraints{
		Min: Size{W: max(0, con.Min.W-mw), H: max(0, con.Min.H-mh)},
		Max: Size{W: max(0, con.Max.W-mw), H: max(0, con.Max.H-mh)},
	}
	s := w.Measure(inner)
	return Size{W: min(s.W+mw, con.Max.W), H: min(s.H+mh, con.Max.H)}
}

// arrangeChild arranges a container's child inside r with its margin
// and alignment: the margin shrinks r, and a non-fill alignment keeps
// the child's natural size at the start, center or end of what is left.
func arrangeChild(w Widget, r render.Rect) {
	l := layoutOf(w)
	if l == nil || (l.margin.Zero() && l.halign == AlignFill && l.valign == AlignFill) {
		w.Arrange(r)
		return
	}
	r = l.margin.Shrink(r)
	if l.halign != AlignFill || l.valign != AlignFill {
		nat := w.Measure(Constraints{Max: Size{W: r.W, H: r.H}})
		h, v := l.halign, l.valign
		if h == AlignBaseline {
			h = AlignStart
		}
		if v == AlignBaseline {
			v = AlignStart
		}
		r = alignRect(r, nat, h, v)
	}
	w.Arrange(r)
}
