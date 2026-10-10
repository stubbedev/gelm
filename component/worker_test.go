package component_test

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stubbedev/gelm/component"
	"github.com/stubbedev/gelm/component/componenttest"
	"github.com/stubbedev/gelm/widget"
)

type squarer struct {
	seen    []int
	stopped bool
}

func (s *squarer) Update(n int, emit func(int)) {
	time.Sleep(time.Second)
	s.seen = append(s.seen, n)
	emit(n * n)
}

func (s *squarer) Shutdown() { s.stopped = true }

func TestWorkerRunsOffLoopAndForwardsOnLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		w := &squarer{}
		var got []int
		ctrl := component.LaunchWorker(&loop, w).Forward(func(n int) { got = append(got, n) })
		for i := range 3 {
			ctrl.Send(i + 1)
		}
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if !slices.Equal(w.seen, []int{1, 2, 3}) {
			t.Fatalf("worker saw %v without the loop pumping, want [1 2 3]", w.seen)
		}
		if len(got) != 0 {
			t.Fatalf("outputs %v were delivered off the loop", got)
		}
		loop.Settle()
		if !slices.Equal(got, []int{1, 4, 9}) {
			t.Errorf("forwarded %v, want [1 4 9]", got)
		}

		ctrl.Send(4)
		ctrl.Send(5)
		synctest.Wait()
		ctrl.Shutdown()
		time.Sleep(time.Hour)
		synctest.Wait()
		loop.Settle()
		if !w.stopped {
			t.Error("the shutdown hook did not run")
		}
		if !slices.Equal(w.seen, []int{1, 2, 3, 4}) {
			t.Errorf("after shutdown the worker saw %v: the update in progress finishes, queued input is dropped", w.seen)
		}
		if !slices.Equal(got, []int{1, 4, 9}) {
			t.Errorf("an output emitted after shutdown was delivered: %v", got)
		}
	})
}

type owner struct {
	worker *component.WorkerController[int, int]
	w      *squarer
	got    []int
}

func (o *owner) Init(cx *component.Context[int, struct{}]) widget.Widget {
	o.worker = cx.LaunchWorker(o.w).ForwardTo(cx.Sender(), func(n int) int { return -n })
	return widget.NewSpacer(1, 1)
}

func (o *owner) Update(_ *component.Context[int, struct{}], msg int) { o.got = append(o.got, msg) }

func TestComponentWorkerForwardsAndStopsWithItsOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		o := &owner{w: &squarer{}}
		ctrl := component.Launch(&loop, o)
		o.worker.Send(3)
		time.Sleep(time.Second)
		synctest.Wait()
		loop.Settle()
		if !slices.Equal(o.got, []int{-9}) {
			t.Fatalf("component heard %v from its worker, want [-9]", o.got)
		}
		ctrl.Shutdown()
		synctest.Wait()
		if !o.w.stopped {
			t.Error("the owner's shutdown did not stop its worker")
		}
	})
}

func TestDetachedWorkerStopsWithTheLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		o := &owner{w: &squarer{}}
		ctrl := component.Launch(&loop, o)
		o.worker.Detach()
		ctrl.Shutdown()
		synctest.Wait()
		if o.w.stopped {
			t.Fatal("the owner's shutdown reached a detached worker")
		}
		loop.Stop()
		synctest.Wait()
		if !o.w.stopped {
			t.Error("the loop's stop did not stop the detached worker")
		}
	})
}
