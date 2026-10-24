package widget

import (
	"testing"

	"github.com/stubbedev/lys-ui/widget/css"
)

// An alert shows its description only while it has one, carries the
// destructive class for that variant, and announces itself by title.
func TestAlert(t *testing.T) {
	a := NewAlert(goldenFace(t), "Heads up!", "")
	if len(a.text.Children()) != 1 {
		t.Fatal("an empty description is shown")
	}
	a.SetDescription("Details")
	a.SetDescription("More details")
	if len(a.text.Children()) != 2 || a.Description() != "More details" {
		t.Fatal("the description is not shown once")
	}
	a.SetDescription("")
	if len(a.text.Children()) != 1 {
		t.Fatal("clearing the description kept it")
	}
	a.SetVariant(AlertDestructive)
	if !a.row.HasClass(css.Destructive) {
		t.Error("the destructive variant lacks its class")
	}
	a.SetVariant(AlertDefault)
	if a.row.HasClass(css.Destructive) {
		t.Error("the default variant kept the destructive class")
	}
	if st := Describe(a); st.Role != RoleAlert || st.Name != "Heads up!" {
		t.Errorf("a11y = %v %q, want alert named by its title", st.Role, st.Name)
	}
}
