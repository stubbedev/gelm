package widget

import (
	"hash/fnv"
	"image"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/stubbedev/gelm/render"
)

// The libadwaita miscellany of #95, each composed from the shared
// shapes: ToggleGroup (the segmented control ViewSwitcher is built
// on), ButtonContent, StatusPage, Banner, Avatar, and SplitButton.
// BottomSheet lives in bottomsheet.go, WrapBox is FlowBox's justify
// option.

// ToggleGroup is the segmented control (adw 1.7): a pill of buttons of
// which exactly one is active, painted with the selected fill and
// :checked. Clicks and Left/Right activate; SetActive drives it from
// code. ViewSwitcher is a ToggleGroup bound to a Stack.
type ToggleGroup struct {
	composite
	row     *Box
	buttons []*Button
	active  int

	// OnChanged fires after every change of the active toggle, from a
	// click, a key, or SetActive.
	OnChanged func(i int)
}

// NewToggleGroup returns a group over items (labels, ButtonContents,
// icons), the first active.
func NewToggleGroup(items ...Widget) *ToggleGroup {
	g := &ToggleGroup{}
	g.SetElement("togglegroup")
	g.row = NewBox(Row, 4, 4)
	g.initComposite(g, g.row)
	g.surface, g.surfaceRadius = surfaceFill, 999
	for _, it := range items {
		g.Append(it)
	}
	return g
}

// Append adds a toggle; the first one appended is active.
func (g *ToggleGroup) Append(content Widget) {
	i := len(g.buttons)
	btn := NewButton(content, 6, 999)
	btn.OnClick = func() { g.SetActive(i) }
	g.buttons = append(g.buttons, btn)
	g.row.Append(btn, false)
	g.markActive()
	g.InvalidateLayout()
}

// Clear removes every toggle.
func (g *ToggleGroup) Clear() {
	g.row.Clear()
	g.buttons, g.active = nil, 0
	g.InvalidateLayout()
}

// Len reports the toggle count.
func (g *ToggleGroup) Len() int { return len(g.buttons) }

// Active reports the active toggle (-1 when empty).
func (g *ToggleGroup) Active() int {
	if len(g.buttons) == 0 {
		return -1
	}
	return g.active
}

// SetActive activates toggle i (clamped) and fires OnChanged when it
// moved.
func (g *ToggleGroup) SetActive(i int) {
	if len(g.buttons) == 0 {
		return
	}
	i = min(max(i, 0), len(g.buttons)-1)
	if i == g.active {
		g.markActive()
		return
	}
	g.active = i
	g.markActive()
	if g.OnChanged != nil {
		g.OnChanged(i)
	}
}

// markActive paints the active toggle's mark and clears the others.
func (g *ToggleGroup) markActive() {
	for i, b := range g.buttons {
		markChecked(b, i == g.active)
	}
}

// KeyAction moves the active toggle with Left/Right.
func (g *ToggleGroup) KeyAction(a KeyAction, _ Mods) {
	switch a {
	case KeyLeft:
		g.SetActive(g.active - 1)
	case KeyRight:
		g.SetActive(g.active + 1)
	}
}

// markChecked marks a toggle as the active one: the :checked state for
// stylesheets, and the theme's selected fill so an unstyled group shows
// which is current. Unchecking returns the unset fill.
func markChecked(b *Button, on bool) {
	b.SetState(StateChecked, on)
	b.BgExplicit = on
	b.Bg, b.BgHover, b.BgPressed = 0, 0, 0
	if on {
		th := Current()
		b.Bg, b.BgHover, b.BgPressed = th.SurfaceHover, th.SurfaceHover, th.SurfacePressed
	}
	b.Invalidate()
}

// ButtonContent is the icon-and-label pair buttons wear (adw
// ButtonContent): either half may be empty.
type ButtonContent struct {
	composite
	row   *Box
	face  render.Font
	size  float64
	icon  *Icon
	label *Label
}

// NewButtonContent returns the pair; an empty icon name or label drops
// that half.
func NewButtonContent(face render.Font, sizePx float64, iconName, label string) *ButtonContent {
	face = requireFace("widget.NewButtonContent", face)
	c := &ButtonContent{face: face, size: sizePx}
	c.row = NewBox(Row, 6, 0)
	c.initComposite(c, c.row)
	c.label = NewLabel(face, sizePx, label, Current().Text)
	c.SetIconName(iconName)
	return c
}

// SetLabel replaces the label text.
func (c *ButtonContent) SetLabel(s string) {
	c.label.SetText(s)
	c.rebuild()
}

// SetIconName replaces the icon; empty drops it.
func (c *ButtonContent) SetIconName(name string) {
	c.icon = nil
	if name != "" {
		c.icon = NewThemeIcon(name, int(c.size))
	}
	c.rebuild()
}

// rebuild lays out whichever halves exist.
func (c *ButtonContent) rebuild() {
	c.row.Clear()
	if c.icon != nil {
		c.row.AppendAligned(c.icon, false, AlignCenter)
	}
	if c.label.Text() != "" {
		c.row.AppendAligned(c.label, false, AlignCenter)
	}
	c.InvalidateLayout()
}

// StatusPage is the empty/error state page (adw StatusPage): a large
// icon, a title, a wrapped description, and an optional child (a
// button), centered in whatever space it gets.
type StatusPage struct {
	composite
	column *Box
	title  *Label
	desc   *Label
}

// NewStatusPage returns the page; empty icon or description drops it.
func NewStatusPage(face render.Font, iconName, title, description string) *StatusPage {
	face = requireFace("widget.NewStatusPage", face)
	th := Current()
	p := &StatusPage{}
	p.column = NewBox(Column, 12, 24)
	p.column.Append(NewSpacer(0, 0), true)
	if iconName != "" {
		p.column.AppendAligned(NewThemeIcon(iconName, 96), false, AlignCenter)
	}
	p.title = NewLabel(face, 26, title, th.Text)
	p.title.SetAlignment(render.AlignCenter)
	p.column.AppendAligned(p.title, false, AlignCenter)
	p.desc = NewLabel(face, 14, description, th.TextMuted)
	p.desc.SetWrap(true)
	p.desc.SetAlignment(render.AlignCenter)
	if description != "" {
		p.column.AppendAligned(p.desc, false, AlignFill)
	}
	p.column.Append(NewSpacer(0, 0), true)
	p.initComposite(p, NewClamp(560, p.column))
	return p
}

// SetChild places w (a call-to-action button, typically) under the
// description.
func (p *StatusPage) SetChild(w Widget) {
	n := len(p.column.Children())
	p.column.InsertAt(n-1, w, false)
	p.InvalidateLayout()
}

// Banner is the strip that slides down to announce a state (adw
// Banner): a title, an optional action button, revealed and hidden
// through SetRevealed on a slide.
type Banner struct {
	composite
	reveal *Revealer
	title  *Label
	button *Button

	// OnButton fires when the action button is clicked.
	OnButton func()
}

// NewBanner returns a hidden banner; an empty button label shows no
// button.
func NewBanner(face render.Font, sizePx float64, title, buttonLabel string) *Banner {
	face = requireFace("widget.NewBanner", face)
	th := Current()
	b := &Banner{}
	b.SetElement("banner")
	b.title = NewLabel(face, sizePx, title, th.Text)
	row := NewBox(Row, 12, 6)
	row.Append(NewSpacer(0, 0), true)
	row.AppendAligned(b.title, false, AlignCenter)
	if buttonLabel != "" {
		b.button = NewButton(NewLabel(face, sizePx, buttonLabel, th.Text), 6, 4)
		b.button.OnClick = func() {
			if b.OnButton != nil {
				b.OnButton()
			}
		}
		row.AppendAligned(b.button, false, AlignCenter)
	}
	row.Append(NewSpacer(0, 0), true)
	b.reveal = NewRevealer(row)
	b.reveal.SetTransition(RevealSlideDown)
	b.reveal.SetDuration(200 * time.Millisecond)
	b.reveal.SetRevealed(false)
	b.initComposite(b, b.reveal)
	b.fillWidth = true
	b.surface = func(th *Theme) Color { return th.HoverSurface() }
	return b
}

// SetTitle replaces the message.
func (b *Banner) SetTitle(s string) { b.title.SetText(s) }

// SetRevealed slides the banner in or out.
func (b *Banner) SetRevealed(on bool) { b.reveal.SetRevealed(on) }

// Revealed reports whether the banner is showing.
func (b *Banner) Revealed() bool { return b.reveal.Revealed() }

// Avatar is the round portrait (adw Avatar): an image cropped to a
// circle, or the name's initials on a color picked from the name, so
// one person keeps one color everywhere.
type Avatar struct {
	node
	face render.Font
	size int
	name string
	img  image.Image
}

// avatarColors is the initials palette, picked by name hash.
var avatarColors = []Color{
	render.RGB(0x83, 0xb6, 0xec), render.RGB(0x7a, 0xd9, 0xbe),
	render.RGB(0xf6, 0xd3, 0x2d), render.RGB(0xff, 0xa3, 0x48),
	render.RGB(0xf6, 0x61, 0x51), render.RGB(0xdc, 0x8a, 0xdd),
	render.RGB(0xcd, 0xab, 0x8f), render.RGB(0x9a, 0x99, 0x96),
}

// NewAvatar returns a size-pixel avatar for name.
func NewAvatar(face render.Font, size int, name string) *Avatar {
	return &Avatar{face: requireFace("widget.NewAvatar", face), size: size, name: name}
}

// SetImage shows img instead of the initials; nil returns to them.
func (a *Avatar) SetImage(img image.Image) {
	a.img = img
	a.Invalidate()
}

// SetName changes the name (initials and color follow).
func (a *Avatar) SetName(name string) {
	a.name = name
	a.Invalidate()
}

// Initials are the name's first letters, at most two: the first and
// the last word's, uppercased.
func (a *Avatar) Initials() string {
	words := strings.FieldsFunc(a.name, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	if len(words) == 0 {
		return ""
	}
	first, _ := utf8.DecodeRuneInString(words[0])
	out := string(unicode.ToUpper(first))
	if len(words) > 1 {
		last, _ := utf8.DecodeRuneInString(words[len(words)-1])
		out += string(unicode.ToUpper(last))
	}
	return out
}

// Color is the initials background for the current name.
func (a *Avatar) Color() Color {
	h := fnv.New32a()
	_, _ = h.Write([]byte(a.name))
	return avatarColors[h.Sum32()%uint32(len(avatarColors))]
}

// Measure wants the avatar's square.
func (a *Avatar) Measure(con Constraints) Size {
	return clampSize(Size{W: a.size, H: a.size}, con)
}

// Arrange records the rect.
func (a *Avatar) Arrange(r render.Rect) { a.node.Arrange(r) }

// Paint draws the cropped image or the colored disc with initials.
func (a *Avatar) Paint(cv *render.Canvas) {
	d := min(a.bounds.W, a.bounds.H)
	r := render.Rect{X: a.bounds.X + (a.bounds.W-d)/2, Y: a.bounds.Y + (a.bounds.H-d)/2, W: d, H: d}
	if a.img != nil {
		cv.DrawImageCover(a.img, r, render.UniformCorners(d/2))
		return
	}
	cv.RoundedRect(r, d/2, a.Color())
	a.face.DrawAligned(cv, a.Initials(), r, float64(d)*0.4, render.RGBA(0, 0, 0, 0xb3), render.AlignCenter)
}

// HitTest resolves inside the bounds.
func (a *Avatar) HitTest(p Point) Widget { return a.HitLeaf(a, p) }

// SplitButton is a button with a menu side (adw SplitButton): the main
// half fires OnClick, the arrow half reports its anchor through OnMenu
// - the app opens the menu popover there (app.AttachSplitButton), the
// same reporting shape a MenuBar's roots use.
type SplitButton struct {
	composite
	main  *Button
	arrow *Button

	// OnClick fires for the main half; OnMenu for the arrow, with the
	// arrow as the popover anchor.
	OnClick func()
	OnMenu  func(anchor Boundser)
}

// NewSplitButton returns the pair over content (a label or
// ButtonContent).
func NewSplitButton(content Widget, sizePx float64) *SplitButton {
	s := &SplitButton{}
	s.SetElement("splitbutton")
	s.main = NewButton(content, 6, 4)
	s.main.OnClick = func() {
		if s.OnClick != nil {
			s.OnClick()
		}
	}
	s.arrow = NewButton(NewSymbol(SymbolChevronDown, int(sizePx)), 4, 4)
	s.arrow.OnClick = func() {
		if s.OnMenu != nil {
			s.OnMenu(s.arrow)
		}
	}
	row := NewBox(Row, 1, 0)
	row.AppendAligned(s.main, false, AlignFill)
	row.AppendAligned(s.arrow, false, AlignFill)
	s.initComposite(s, row)
	return s
}

// Arrow returns the menu half, the popover anchor.
func (s *SplitButton) Arrow() Boundser { return s.arrow }
