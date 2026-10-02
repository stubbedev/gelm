package widget

import (
	"testing"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

// The check styles as `checkbutton > check`: its box layers paint the
// indicator, its min sizes size the checkbox, and :checked and
// :indeterminate reach it through the checkbox's state.
func TestCheckButtonCheckNode(t *testing.T) {
	loadCSS(t, `checkbutton check { min-width: 24; min-height: 26; border-width: 3; border-style: solid; border-radius: 5; background-color: #010203; }`)
	c := NewCheckButton(false)
	arrangeTree(t, c, 100, 40)
	if got := c.Measure(Constraints{Max: Size{W: 100, H: 40}}); got.W != 24 || got.H != 26 {
		t.Errorf("checkbox %v, want the check's 24x26", got)
	}
	kv := c.check.style(&c.check)
	if got := kv.Background; got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("check background %v, want the `checkbutton check` rule", got)
	}
	if kv.Radius.TopLeft != 5 {
		t.Errorf("check radius %d, want 5", kv.Radius.TopLeft)
	}
	if got := kv.EffBorder().Top; got != 3 {
		t.Errorf("check border %d, want 3", got)
	}

	loadCSS(t, `checkbutton check { background-color: #010203; } checkbutton:checked check { background-color: #0a0b0c; } checkbutton:indeterminate check { background-color: #0d0e0f; }`)
	c = NewCheckButton(false)
	arrangeTree(t, c, 100, 40)
	if got := c.check.style(&c.check).Background; got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("resting check background %v, want the unchecked rule", got)
	}
	c.SetChecked(true)
	if got := c.check.style(&c.check).Background; got != render.RGB(0x0a, 0x0b, 0x0c) {
		t.Errorf("checked check background %v, want the :checked rule", got)
	}
	c.SetChecked(false)
	c.SetInconsistent(true)
	if got := c.check.style(&c.check).Background; got != render.RGB(0x0d, 0x0e, 0x0f) {
		t.Errorf("inconsistent check background %v, want the :indeterminate rule", got)
	}
	if !c.Inconsistent() || c.Inconsistent() == c.Checked() && c.Checked() {
		t.Errorf("inconsistent %v, checked %v", c.Inconsistent(), c.Checked())
	}
}

// The mark's ink is the check's color, the way a GTK theme sets
// `-gtk-icon-source` and color together.
func TestCheckButtonCheckMarkColor(t *testing.T) {
	loadCSS(t, `checkbutton check { color: #112233; }`)
	c := NewCheckButton(true)
	arrangeTree(t, c, 100, 40)
	if got := pickc(0, c.check.style(&c.check), style.PropColor, 0); got != render.RGB(0x11, 0x22, 0x33) {
		t.Errorf("mark color %v, want the check's color rule", got)
	}
}
