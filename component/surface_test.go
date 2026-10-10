package component

import (
	"slices"
	"testing"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/component/componenttest"
	"github.com/stubbedev/gelm/widget"
)

type titled struct {
	title   string
	applied *[]string
}

func (m *titled) Init(cx *Context[string, struct{}]) widget.Widget {
	watchChanged(cx, func() string { return m.title }, func(_ *app.Window, t string) {
		*m.applied = append(*m.applied, t)
	})
	return widget.NewSpacer(1, 1)
}

func (m *titled) Update(_ *Context[string, struct{}], t string) { m.title = t }

func TestWindowWatchesWaitForTheWindowAndApplyOnlyChanges(t *testing.T) {
	var loop componenttest.Loop
	var applied []string
	m := &titled{title: "draft", applied: &applied}
	ctrl := Launch(&loop, m)
	if ctrl.cx.Window() != nil || len(applied) != 0 {
		t.Fatalf("a window watch ran before the window existed: %v", applied)
	}
	ctrl.cx.surface.attach(&app.Window{}, nil)
	ctrl.Send("draft")
	loop.Settle()
	ctrl.Send("saved")
	ctrl.Send("saved")
	loop.Settle()
	if !slices.Equal(applied, []string{"draft", "saved"}) {
		t.Errorf("applied %v, want the initial title on attach, then each change once", applied)
	}
}

type parentOfTitled struct{ child *Controller[string, struct{}] }

func (p *parentOfTitled) Init(cx *Context[int, struct{}]) widget.Widget {
	p.child = cx.Launch(&titled{applied: new([]string)})
	return p.child.Widget()
}

func (p *parentOfTitled) Update(*Context[int, struct{}], int) {}

func TestChildrenShareTheirAncestorsWindow(t *testing.T) {
	var loop componenttest.Loop
	p := &parentOfTitled{}
	ctrl := Launch(&loop, p)
	w := &app.Window{}
	ctrl.cx.surface.attach(w, nil)
	if p.child.cx.Window() != w {
		t.Error("a child does not see its parent's window")
	}
}
