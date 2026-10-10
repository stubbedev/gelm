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

type fetcher struct {
	got       []string
	cancelled chan struct{}
}

func (f *fetcher) Init(cx *component.Context[string, struct{}]) widget.Widget {
	cx.Oneshot(func(ctx context.Context) string {
		time.Sleep(time.Second)
		return "fetched"
	})
	cx.Spawn(func(ctx context.Context, emit func(string)) {
		for i := range 3 {
			select {
			case <-ctx.Done():
				close(f.cancelled)
				return
			case <-time.After(time.Minute):
				emit("tick" + string(rune('0'+i)))
			}
		}
	})
	return widget.NewSpacer(1, 1)
}

func (f *fetcher) Update(_ *component.Context[string, struct{}], msg string) {
	f.got = append(f.got, msg)
}

func TestCommandsDeliverToUpdateAndStopAtShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		f := &fetcher{cancelled: make(chan struct{})}
		ctrl := component.Launch(&loop, f)

		time.Sleep(time.Second)
		synctest.Wait()
		loop.Settle()
		if !slices.Equal(f.got, []string{"fetched"}) {
			t.Fatalf("after 1s got %v, want the oneshot result", f.got)
		}

		time.Sleep(2 * time.Minute)
		synctest.Wait()
		loop.Settle()
		if !slices.Equal(f.got, []string{"fetched", "tick0", "tick1"}) {
			t.Fatalf("after 2m got %v, want two spawned ticks", f.got)
		}

		ctrl.Shutdown()
		synctest.Wait()
		select {
		case <-f.cancelled:
		default:
			t.Fatal("shutdown did not cancel the spawned command")
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		loop.Settle()
		if len(f.got) != 3 {
			t.Errorf("a command delivered after shutdown: %v", f.got)
		}
	})
}

type lateOneshot struct{ got int }

func (l *lateOneshot) Init(cx *component.Context[int, struct{}]) widget.Widget {
	cx.Oneshot(func(context.Context) int {
		time.Sleep(time.Second)
		return 1
	})
	return widget.NewSpacer(1, 1)
}

func (l *lateOneshot) Update(_ *component.Context[int, struct{}], msg int) { l.got += msg }

func TestOneshotResultAfterShutdownIsDropped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		l := &lateOneshot{}
		ctrl := component.Launch(&loop, l)
		ctrl.Shutdown()
		time.Sleep(time.Second)
		synctest.Wait()
		loop.Settle()
		if l.got != 0 {
			t.Errorf("a oneshot finishing after shutdown was delivered")
		}
	})
}

type loader struct {
	data    string
	label   *widget.Spacer
	inits   int
	got     []int
	sawData []string
}

func (l *loader) Loading() widget.Widget { return widget.NewSpacer(2, 2) }

func (l *loader) Load(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		l.data = "loaded"
	}
}

func (l *loader) Init(*component.Context[int, struct{}]) widget.Widget {
	l.inits++
	l.label = widget.NewSpacer(5, 5)
	return l.label
}

func (l *loader) Update(_ *component.Context[int, struct{}], msg int) {
	l.got = append(l.got, msg)
	l.sawData = append(l.sawData, l.data)
}

func TestLoaderShowsLoadingThenSwapsAndReleasesHeldInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		l := &loader{}
		ctrl := component.Launch(&loop, l)
		slot, ok := ctrl.Widget().(*widget.Stack)
		if !ok || slot.Visible() != "loading" {
			t.Fatalf("root = %T showing %q, want the loading page", ctrl.Widget(), slot.Visible())
		}
		ctrl.Send(1)
		ctrl.Send(2)
		loop.Settle()
		if l.inits != 0 || len(l.got) != 0 {
			t.Fatalf("Init ran %d times and Update saw %v before Load returned", l.inits, l.got)
		}

		time.Sleep(time.Second)
		synctest.Wait()
		loop.Settle()
		if l.inits != 1 || slot.Visible() != "ready" || slices.Contains(slot.Order(), "loading") {
			t.Fatalf("after load: inits=%d visible=%q pages=%v", l.inits, slot.Visible(), slot.Order())
		}
		if !slices.Equal(l.got, []int{1, 2}) || !slices.Equal(l.sawData, []string{"loaded", "loaded"}) {
			t.Errorf("held input delivered %v seeing %v, want [1 2] after the load", l.got, l.sawData)
		}
	})
}

func TestLoaderShutDownWhileLoadingNeverInits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop componenttest.Loop
		l := &loader{}
		ctrl := component.Launch(&loop, l)
		ctrl.Shutdown()
		synctest.Wait()
		loop.Settle()
		if l.inits != 0 || l.data != "" {
			t.Errorf("a loader shut down mid-load ran Init %d times, data %q", l.inits, l.data)
		}
	})
}
