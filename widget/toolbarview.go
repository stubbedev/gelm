package widget

// ToolbarStyle is how a ToolbarView's bars sit against the content.
type ToolbarStyle uint8

const (
	// ToolbarFlat draws the bars flush with the content.
	ToolbarFlat ToolbarStyle = iota
	// ToolbarRaised raises the bars with a shadow.
	ToolbarRaised
	// ToolbarRaisedBorder raises the bars and draws a border.
	ToolbarRaisedBorder
)

func (s ToolbarStyle) class() string {
	switch s {
	case ToolbarRaised:
		return "raised"
	case ToolbarRaisedBorder:
		return "raised-border"
	default:
		return "flat"
	}
}

// ToolbarView is libadwaita's AdwToolbarView: top and bottom bars
// around a content widget. The bars reveal and hide with a slide, take
// a flat, raised or raised-with-border style, and the content can
// extend under them (under translucent bars, scrolling beneath them).
// It styles as `toolbarview` with `.top-bar` and `.bottom-bar` parts
// carrying the style as a class.
type ToolbarView struct {
	composite
	top, bottom       *Box
	topReveal         *Revealer
	bottomReveal      *Revealer
	content           Widget
	topStyle          ToolbarStyle
	bottomStyle       ToolbarStyle
	extendTop, extend bool
}

// NewToolbarView returns a toolbar view around content.
func NewToolbarView(content Widget) *ToolbarView {
	t := &ToolbarView{top: NewBox(Column, 0, 0), bottom: NewBox(Column, 0, 0), content: content}
	t.top.AddClass("top-bar")
	t.bottom.AddClass("bottom-bar")
	t.topReveal, t.bottomReveal = NewRevealer(t.top), NewRevealer(t.bottom)
	t.topReveal.SetTransition(RevealSlideDown)
	t.bottomReveal.SetTransition(RevealSlideUp)
	t.topReveal.SetCollapse(true)
	t.bottomReveal.SetCollapse(true)
	t.topReveal.SetRevealed(true)
	t.bottomReveal.SetRevealed(true)
	t.topReveal.Finish()
	t.bottomReveal.Finish()
	t.SetElement("toolbarview")
	t.initComposite(t, nil)
	t.syncStyles()
	t.layout()
	return t
}

// AddTopBar appends a bar to the top stack.
func (t *ToolbarView) AddTopBar(bar Widget) { t.top.Append(bar, false) }

// AddBottomBar appends a bar to the bottom stack.
func (t *ToolbarView) AddBottomBar(bar Widget) { t.bottom.Append(bar, false) }

// SetContent replaces the content.
func (t *ToolbarView) SetContent(w Widget) {
	t.content = w
	t.layout()
}

// Content returns the content.
func (t *ToolbarView) Content() Widget { return t.content }

// SetRevealTopBars shows or hides the top bars with a slide.
func (t *ToolbarView) SetRevealTopBars(on bool) { t.topReveal.SetRevealed(on) }

// SetRevealBottomBars shows or hides the bottom bars with a slide.
func (t *ToolbarView) SetRevealBottomBars(on bool) { t.bottomReveal.SetRevealed(on) }

// RevealTopBars reports whether the top bars are shown.
func (t *ToolbarView) RevealTopBars() bool { return t.topReveal.Revealed() }

// RevealBottomBars reports whether the bottom bars are shown.
func (t *ToolbarView) RevealBottomBars() bool { return t.bottomReveal.Revealed() }

// SetTopBarStyle sets the top bars' style.
func (t *ToolbarView) SetTopBarStyle(s ToolbarStyle) {
	t.topStyle = s
	t.syncStyles()
}

// SetBottomBarStyle sets the bottom bars' style.
func (t *ToolbarView) SetBottomBarStyle(s ToolbarStyle) {
	t.bottomStyle = s
	t.syncStyles()
}

// SetExtendContentToTopEdge lets the content run under the top bars.
func (t *ToolbarView) SetExtendContentToTopEdge(on bool) {
	if t.extendTop != on {
		t.extendTop = on
		t.layout()
	}
}

// SetExtendContentToBottomEdge lets the content run under the bottom
// bars.
func (t *ToolbarView) SetExtendContentToBottomEdge(on bool) {
	if t.extend != on {
		t.extend = on
		t.layout()
	}
}

func (t *ToolbarView) syncStyles() {
	for _, part := range []struct {
		box   *Box
		style ToolbarStyle
	}{{t.top, t.topStyle}, {t.bottom, t.bottomStyle}} {
		part.box.RemoveClass("flat", "raised", "raised-border")
		part.box.AddClass(part.style.class())
	}
}

func (t *ToolbarView) layout() {
	column := NewBox(Column, 0, 0)
	if !t.extendTop && !t.extend {
		column.Append(t.topReveal, false)
		if t.content != nil {
			column.Append(t.content, true)
		}
		column.Append(t.bottomReveal, false)
		t.setRoot(column)
		return
	}
	over := NewOverlay()
	if t.content != nil {
		over.Append(t.content)
	}
	if t.extendTop {
		over.AppendAligned(t.topReveal, AlignFill, AlignStart)
	} else {
		column.Append(t.topReveal, false)
	}
	column.Append(over, true)
	if t.extend {
		over.AppendAligned(t.bottomReveal, AlignFill, AlignEnd)
	} else {
		column.Append(t.bottomReveal, false)
	}
	t.setRoot(column)
}
