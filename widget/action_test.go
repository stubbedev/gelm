package widget

import (
	"testing"
)

func TestActionEnabledGatesActivationAndNotifies(t *testing.T) {
	runs, notes := 0, 0
	a := NewAction("save", func() { runs++ })
	cancel := a.Subscribe(func() { notes++ })
	a.Activate()
	a.SetEnabled(false)
	a.SetEnabled(false)
	a.Activate()
	if runs != 1 || notes != 1 {
		t.Errorf("runs=%d notes=%d, want 1 and 1", runs, notes)
	}
	cancel()
	a.SetEnabled(true)
	if notes != 1 {
		t.Error("a cancelled subscriber was notified")
	}
}

func TestStateActionTargetsAndToggle(t *testing.T) {
	var changes []string
	mode := NewStateAction("mode", "light", func(s string) { changes = append(changes, s) })
	dark, light := mode.Target("dark"), mode.Target("light")
	if dark.Checked() || !light.Checked() {
		t.Fatal("targets do not reflect the initial state")
	}
	dark.Activate()
	dark.Activate()
	if mode.State() != "dark" || !dark.Checked() || light.Checked() || len(changes) != 1 {
		t.Errorf("state %q changes %v after activating dark twice", mode.State(), changes)
	}
	if dark.Name() != "mode(dark)" {
		t.Errorf("target name %q", dark.Name())
	}

	grid := NewStateAction("grid", false, nil)
	toggle := Toggle(grid)
	toggle.Activate()
	if !grid.State() || !toggle.Checked() {
		t.Error("Toggle did not flip the bool action on")
	}
	grid.SetEnabled(false)
	toggle.Activate()
	if !grid.State() {
		t.Error("a disabled toggle flipped")
	}
}

func TestButtonBindActionFollowsEnabled(t *testing.T) {
	runs := 0
	a := NewAction("go", func() { runs++ })
	b := NewButton(NewSpacer(1, 1), 0, 0)
	unbind := b.BindAction(a)
	b.OnClick()
	a.SetEnabled(false)
	if b.Enabled() {
		t.Error("the button stayed enabled with its action disabled")
	}
	a.SetEnabled(true)
	if !b.Enabled() || runs != 1 {
		t.Errorf("enabled=%v runs=%d", b.Enabled(), runs)
	}
	unbind()
	a.SetEnabled(false)
	if !b.Enabled() || b.OnClick != nil {
		t.Error("an unbound button still follows its action")
	}
}

func TestCheckButtonBindActionIsTwoWayWithoutEcho(t *testing.T) {
	changes := 0
	a := NewStateAction("bold", false, func(bool) { changes++ })
	c := NewCheckButton(false)
	c.BindAction(Toggle(a))
	c.Toggle()
	if !a.State() || !c.Checked() || changes != 1 {
		t.Fatalf("user toggle: action=%v check=%v changes=%d", a.State(), c.Checked(), changes)
	}
	a.SetState(false)
	if c.Checked() || changes != 2 {
		t.Errorf("program change: check=%v changes=%d", c.Checked(), changes)
	}
}

func TestToggleButtonsBindRadioTargets(t *testing.T) {
	align := NewStateAction("align", "left", nil)
	left, right := NewToggleButton(NewSpacer(1, 1), 0, 0), NewToggleButton(NewSpacer(1, 1), 0, 0)
	left.BindAction(align.Target("left"))
	right.BindAction(align.Target("right"))
	right.OnClick()
	if align.State() != "right" || left.Active() || !right.Active() {
		t.Errorf("state %q left=%v right=%v", align.State(), left.Active(), right.Active())
	}
	right.SetActive(false)
	if !right.Active() {
		t.Error("deactivating the selected radio target left it inactive")
	}
}

func TestMenuRowsFollowTheirActions(t *testing.T) {
	ran := 0
	save := NewAction("save", func() { ran++ })
	wrap := NewStateAction("wrap", false, nil)
	m := NewMenu(testFace(t), 12, ActionItem("Save", save), CheckItem("Wrap", Toggle(wrap)))
	save.SetEnabled(false)
	if m.Items()[0].Enabled() || !m.Items()[0].inert() {
		t.Error("a row with a disabled action is live")
	}
	m.activate(0)
	if ran != 0 {
		t.Error("activating a disabled action row ran it")
	}
	m.activate(1)
	if !wrap.State() || !m.Items()[1].IsChecked() {
		t.Error("the check row did not flip its action")
	}
	if !m.rows[1].HasState(StateChecked) {
		t.Error("the check row's :checked state did not follow the action")
	}
}
