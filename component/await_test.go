package component_test

import (
	"context"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stubbedev/gelm/component"
	"github.com/stubbedev/gelm/component/componenttest"
	"github.com/stubbedev/gelm/widget"
)

type saver struct {
	saved     []string
	seen      []string
	busyViews []bool
	cancelled bool
}

func (s *saver) Init(cx *component.Context[string, struct{}]) widget.Widget {
	cx.Watch(func() { s.busyViews = append(s.busyViews, cx.Busy()) })
	return widget.NewSpacer(1, 1)
}

func (s *saver) Update(cx *component.Context[string, struct{}], msg string) {
	s.seen = append(s.seen, msg)
	if msg != "save" {
		return
	}
	cx.Await(func(ctx context.Context) func() {
		select {
		case <-ctx.Done():
			s.cancelled = true
			return func() { s.saved = append(s.saved, "late") }
		case <-time.After(time.Second):
		}
		return func() { s.saved = append(s.saved, "done") }
	})
}

func TestAwaitHoldsLaterInputUntilTheStepLands(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		s := &saver{}
		ctrl := component.Launch(&loop, s)
		ctrl.Send("save")
		ctrl.Send("after")
		loop.Settle()
		if !slices.Equal(s.seen, []string{"save"}) {
			t.Fatalf("seen %v while saving, want the rest of the batch held", s.seen)
		}
		ctrl.Send("later")
		loop.Settle()
		if len(s.seen) != 1 {
			t.Fatalf("input sent during the step ran early: %v", s.seen)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		loop.Settle()
		if !slices.Equal(s.saved, []string{"done"}) || !slices.Equal(s.seen, []string{"save", "after", "later"}) {
			t.Errorf("saved %v seen %v, want the step applied, then the held input in order", s.saved, s.seen)
		}
		if !slices.Equal(s.busyViews, []bool{false, true, false}) {
			t.Errorf("views saw busy %v, want idle, busy at the start, idle after", s.busyViews)
		}
	})
}

func TestAwaitAfterShutdownNeverApplies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		s := &saver{}
		ctrl := component.Launch(&loop, s)
		ctrl.Send("save")
		loop.Settle()
		ctrl.Shutdown()
		synctest.Wait()
		loop.Settle()
		if !s.cancelled || len(s.saved) != 0 {
			t.Errorf("cancelled=%v saved=%v, want the step cancelled and never applied", s.cancelled, s.saved)
		}
	})
}
