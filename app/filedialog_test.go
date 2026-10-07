package app

import (
	"testing"
)

// TestDialogValidateResponseVeto pins the file-dialog veto mechanism:
// a refused response leaves the dialog open and retryable, the next
// valid one responds and closes.
func TestDialogValidateResponseVeto(t *testing.T) {
	a := &Application{}
	responses := 0
	vetoes := 0
	d := &Dialog{
		app: a,
		win: &Window{app: a},
		cfg: DialogConfig{
			ValidateResponse: func(resp string) bool {
				if resp == "bad" {
					vetoes++
					return false
				}
				return true
			},
			OnResponse: func(string) { responses++ },
		},
	}
	d.Respond("bad")
	if vetoes != 1 || responses != 0 {
		t.Fatalf("after the veto: vetoes=%d responses=%d, want 1 and 0", vetoes, responses)
	}
	if d.Closed() {
		t.Error("a vetoed response closed the dialog")
	}
	d.Respond("ok")
	if responses != 1 || !d.Closed() {
		t.Errorf("after the valid response: responses=%d closed=%v, want 1 and true", responses, d.Closed())
	}
}
