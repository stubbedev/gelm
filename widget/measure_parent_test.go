package widget

import "testing"

func TestRemeasureBeforeArrangeSeesChanges(t *testing.T) {
	face := testFace(t)
	con := Constraints{Max: Size{W: 400, H: 400}}

	row := NewActionRow(face, 13, "Title", "")
	before := row.Measure(con)
	row.SetSubtitle("a subtitle line")
	if after := row.Measure(con); after.H <= before.H {
		t.Errorf("a composite kept its cached size after a change below it: %v then %v", before, after)
	}

	hidden := NewSpacer(10, 30)
	hidden.SetVisible(false)
	box := NewBox(Column, 0, 0)
	box.Append(NewSpacer(10, 10), false).Append(hidden, false)
	before = box.Measure(con)
	hidden.SetVisible(true)
	if after := box.Measure(con); after.H != before.H+30 {
		t.Errorf("showing a hidden child did not reach the box: %v then %v", before, after)
	}
}
