package app

import (
	"testing"

	"github.com/stubbedev/gelm/anim"
	"github.com/stubbedev/gelm/widget"
)

func TestAdaptiveDialogAsABottomSheetWrapsAndRestoresTheParent(t *testing.T) {
	defer anim.SetInstant(true)()
	a := &Application{}
	parent := &Window{app: a}
	root := widget.NewSpacer(100, 100)
	hw := &hostWindow{router: &widget.Router{Root: root}, win: parent}
	a.windows = []*hostWindow{hw}

	var responses []string
	closed := 0
	d, err := a.AdaptiveDialog(parent, AdaptiveDialogConfig{
		Bare: true, Content: widget.NewSpacer(50, 50),
		CancelResponse: "cancel",
		OnResponse:     func(r string) { responses = append(responses, r) },
		OnClosed:       func() { closed++ },
		Presentation:   PresentBottomSheet,
	})
	if err != nil {
		t.Fatal(err)
	}
	sheet, ok := hw.router.Root.(*widget.BottomSheet)
	if !ok || !sheet.Open() || sheet.Content() != widget.Widget(root) {
		t.Fatalf("the parent's tree is not wrapped in an open sheet: %T", hw.router.Root)
	}
	d.Respond("ok")
	d.Respond("again")
	if len(responses) != 1 || responses[0] != "ok" || closed != 1 || hw.router.Root != widget.Widget(root) {
		t.Errorf("responses %v closed %d root restored=%v", responses, closed, hw.router.Root == widget.Widget(root))
	}

	d2, err := a.AdaptiveDialog(parent, AdaptiveDialogConfig{
		Bare: true, Content: widget.NewSpacer(50, 50), CancelResponse: "cancel",
		OnResponse:   func(r string) { responses = append(responses, r) },
		Presentation: PresentBottomSheet,
	})
	if err != nil {
		t.Fatal(err)
	}
	hw.router.Root.(*widget.BottomSheet).KeyAction(widget.KeyDismiss, 0)
	if !d2.Closed() || len(responses) != 2 || responses[1] != "cancel" {
		t.Errorf("Esc on the sheet: closed=%v responses %v, want the cancel response", d2.Closed(), responses)
	}
}
