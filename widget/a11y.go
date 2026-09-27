// Accessibility semantics for the widget tree: semantic roles plus a
// state snapshot that assistive technology can consume. This is the
// plain-Go layer only - how it reaches a screen reader is recorded in
// docs/a11y.md.
package widget

import (
	"github.com/stubbedev/gelm/render"
)

// Role classifies a widget for assistive technology. The names follow
// the AT-SPI roles an out-of-process bridge would translate to, so
// callers can switch on them without gelm gaining a dbus dependency.
type Role uint8

const (
	// RoleNone marks widgets with no accessibility semantics: layout
	// containers, spacers, and unclassified leaves.
	RoleNone Role = iota
	RoleButton
	RoleLabel
	RoleEntry
	RoleTextArea
	RoleSlider
	RoleSwitch
	RoleCheckBox
	RoleProgressBar
	RoleScrollArea
	RoleList
	RoleMenu
	RoleTabList
	RoleComboBox
)

// String returns the lowercase role name.
func (r Role) String() string {
	switch r {
	case RoleButton:
		return "button"
	case RoleLabel:
		return "label"
	case RoleEntry:
		return "entry"
	case RoleTextArea:
		return "text-area"
	case RoleSlider:
		return "slider"
	case RoleSwitch:
		return "switch"
	case RoleCheckBox:
		return "check-box"
	case RoleProgressBar:
		return "progress-bar"
	case RoleScrollArea:
		return "scroll-area"
	case RoleList:
		return "list"
	case RoleMenu:
		return "menu"
	case RoleTabList:
		return "tab-list"
	case RoleComboBox:
		return "combo-box"
	default:
		return "none"
	}
}

// Roleer is implemented by widgets carrying a semantic role.
type Roleer interface {
	// Role returns the widget's semantic role.
	Role() Role
}

// RoleOf returns w's role, or RoleNone when w is nil or carries none
// (plain containers and third-party Widget implementations).
func RoleOf(w Widget) Role {
	if r, ok := w.(Roleer); ok {
		return r.Role()
	}
	return RoleNone
}

// A11yState is a point-in-time snapshot of what assistive technology
// needs about one widget: role, name, current value, text contents
// with caret and selection, and state flags.
type A11yState struct {
	Role Role
	// Name is the accessible name: the tooltip text when set, else the
	// visible label text for labeled controls.
	Name string
	// Text is the current contents for text-bearing roles.
	Text string
	// Caret is the text cursor as a rune offset into Text.
	Caret int
	// SelStart and SelEnd bound the selected rune range; HasSelection
	// reports whether one is active.
	SelStart, SelEnd int
	HasSelection     bool
	// Checked is the Switch and CheckButton state.
	Checked bool
	// Min, Max, and Value carry the ranged roles (slider, progress
	// bar); Step is the keyboard increment (slider only, 0 when the
	// slider divides the range itself).
	Min, Max, Value, Step float64
	// Editable reports whether the text can be changed by the user.
	Editable bool
	// Multiline distinguishes text areas from single-line entries.
	Multiline bool
	// Focusable reports whether Tab traversal can land on the widget,
	// which is exactly the set of KeyActionHandler implementations.
	Focusable bool
	// Bounds is the arranged rect, so a bridge can report extents.
	Bounds render.Rect
}

// Describe snapshots w's accessibility state. Focus is deliberately
// absent: it lives on the Router, and callers compare against
// Router.Focused directly.
func Describe(w Widget) A11yState {
	if w == nil {
		return A11yState{}
	}
	st := A11yState{Role: RoleOf(w), Focusable: isFocusable(w)}
	if t, ok := w.(TooltipTexter); ok {
		st.Name = t.TooltipText()
	}
	if b, ok := w.(Boundser); ok {
		st.Bounds = b.Bounds()
	}
	switch v := w.(type) {
	case *Label:
		st.Text = v.Text()
		// A bare label's text is its accessible name.
		if st.Name == "" {
			st.Name = v.Text()
		}
	case *RichLabel:
		st.Text = v.Text()
		if st.Name == "" {
			st.Name = v.Text()
		}
	case *Button:
		if st.Name == "" {
			st.Name = labelText(v.child)
		}
	case *Entry:
		st.Editable = true
		st.Text = v.Text()
		st.Caret = v.Cursor()
		st.SelStart, st.SelEnd, st.HasSelection = v.Selection()
	case *TextArea:
		st.Editable = true
		st.Multiline = true
		st.Text = v.Text()
		st.Caret, st.SelStart, st.SelEnd, st.HasSelection = textAreaOffsets(v)
	case *Slider:
		st.Min, st.Max, st.Value, st.Step = v.min, v.max, v.value, v.step
	case *Switch:
		st.Checked = v.On()
	case *CheckButton:
		st.Checked = v.Checked()
	case *ProgressBar:
		st.Min, st.Max, st.Value = 0, 1, v.Value()
	case *Scroll:
		st.Role = RoleScrollArea
	case *List:
		st.Role = RoleList
	case *Menu:
		st.Role = RoleMenu
	case *Notebook:
		st.Role = RoleTabList
	case *Dropdown:
		// A combo box's accessible name is its current selection.
		if st.Name == "" {
			st.Name = v.Selection()
		}
	}
	return st
}

// isFocusable reports whether Tab traversal can land on w.
func isFocusable(w Widget) bool {
	_, ok := w.(KeyActionHandler)
	return ok
}

// DescribeTree snapshots root and every descendant in paint order, the
// same pre-order focus traversal uses.
func DescribeTree(root Widget) []A11yState {
	var out []A11yState
	focusWalker(root, func(w Widget) { out = append(out, Describe(w)) })
	return out
}

// labelText returns the first label text found under w, for controls
// that wrap a label as their visible name.
func labelText(w Widget) string {
	if l, ok := w.(*Label); ok {
		return l.Text()
	}
	type childser interface{ Children() []Widget }
	if c, ok := w.(childser); ok {
		for _, k := range c.Children() {
			if s := labelText(k); s != "" {
				return s
			}
		}
	}
	return ""
}

// textAreaOffsets flattens a TextArea's cursor and selection into rune
// offsets into Text: every line contributes its runes plus a newline.
func textAreaOffsets(t *TextArea) (caret, selStart, selEnd int, active bool) {
	flat := func(p pos) int {
		off := 0
		for l := 0; l < p.line && l < len(t.lines); l++ {
			off += len(t.lines[l]) + 1
		}
		return off + p.col
	}
	start, end := t.ordered()
	return flat(t.cursor), flat(start), flat(end), start != end
}
