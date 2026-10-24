package atspi

import (
	"slices"
	"testing"

	"github.com/stubbedev/lys-ui/widget"
)

// A toggle button serves AT-SPI's toggle-button role and the pressed
// state while active, as GTK's does; an inactive one is not pressed.
func TestToggleButtonRoleAndPressedState(t *testing.T) {
	if got := atspiRole(widget.RoleToggleButton); got != roleToggleButton || roleToggleButton != 62 {
		t.Errorf("role = %d, want ATSPI_ROLE_TOGGLE_BUTTON (62)", got)
	}
	b := &Bridge{}
	tr := &tree{nodes: map[int32]*anode{
		1: {id: 1, st: widget.A11yState{Role: widget.RoleToggleButton, Enabled: true, Pressed: true}},
		2: {id: 2, st: widget.A11yState{Role: widget.RoleToggleButton, Enabled: true}},
	}}
	if !slices.Contains(b.statesOf(tr, 1), statePressed) || statePressed != 20 {
		t.Error("an active toggle does not serve ATSPI_STATE_PRESSED (20)")
	}
	if slices.Contains(b.statesOf(tr, 2), statePressed) {
		t.Error("an inactive toggle serves pressed")
	}
}

// Every semantic role serves its own AT-SPI role and name: a role the
// table lacks would read as an unlabeled filler, as toggle buttons and
// level bars once did.
func TestEverySemanticRoleMapsToAtspi(t *testing.T) {
	for r := widget.RoleNone + 1; r.String() != "none"; r++ {
		if got := atspiRoleOf(r); got.id == roleFiller || got.name == "filler" {
			t.Errorf("%v serves %+v, want its own role", r, got)
		}
	}
	if got := atspiRoleName(widget.RoleToggleButton); got != "toggle button" {
		t.Errorf("toggle button role name = %q", got)
	}
}
