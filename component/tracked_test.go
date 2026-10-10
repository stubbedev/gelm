package component_test

import (
	"slices"
	"testing"

	"github.com/stubbedev/gelm/component"
	"github.com/stubbedev/gelm/component/componenttest"
	"github.com/stubbedev/gelm/widget"
)

type trackMsg int

const (
	bumpCount trackMsg = iota
	bumpName
	nothing
)

type tracker struct {
	count, name     *component.Tracked[int]
	countRuns       int
	nameRuns        int
	changedInView   []bool
	changedInUpdate []bool
}

func (m *tracker) Init(cx *component.Context[trackMsg, struct{}]) widget.Widget {
	m.count = cx.Tracked(0)
	m.name = cx.Tracked(0)
	cx.Track(func() { m.countRuns++ }, m.count)
	cx.Track(func() { m.nameRuns++ }, m.name, m.count)
	return widget.NewSpacer(1, 1)
}

func (m *tracker) Update(_ *component.Context[trackMsg, struct{}], msg trackMsg) {
	m.changedInUpdate = append(m.changedInUpdate, m.count.Changed())
	switch msg {
	case bumpCount:
		m.count.Update(func(n *int) { *n++ })
	case bumpName:
		m.name.Set(m.name.Get() + 1)
	}
}

func (m *tracker) UpdateView(*component.Context[trackMsg, struct{}]) {
	m.changedInView = append(m.changedInView, m.count.Changed())
}

func TestTrackRunsOnlyWhenADependencyChanged(t *testing.T) {
	var loop componenttest.Loop
	m := &tracker{}
	ctrl := component.Launch(&loop, m)
	if m.countRuns != 1 || m.nameRuns != 1 {
		t.Fatalf("initial runs count=%d name=%d, want 1 each", m.countRuns, m.nameRuns)
	}

	ctrl.Send(nothing)
	loop.Settle()
	if m.countRuns != 1 || m.nameRuns != 1 {
		t.Errorf("an update changing nothing re-ran tracks: count=%d name=%d", m.countRuns, m.nameRuns)
	}

	ctrl.Send(bumpName)
	loop.Settle()
	if m.countRuns != 1 || m.nameRuns != 2 {
		t.Errorf("a name change ran count=%d name=%d, want 1 and 2", m.countRuns, m.nameRuns)
	}

	ctrl.Send(bumpCount)
	ctrl.Send(nothing)
	loop.Settle()
	if m.countRuns != 2 || m.nameRuns != 3 {
		t.Errorf("a count change ran count=%d name=%d, want 2 and 3", m.countRuns, m.nameRuns)
	}
	if m.count.Get() != 1 {
		t.Errorf("count = %d, want 1", m.count.Get())
	}
	if !slices.Equal(m.changedInView, []bool{false, false, true}) {
		t.Errorf("Changed in UpdateView per batch = %v, want [false false true]", m.changedInView)
	}
	if !slices.Equal(m.changedInUpdate, []bool{false, false, false, true}) {
		t.Errorf("Changed in Update per message = %v; a mark must survive within its batch and clear after it", m.changedInUpdate)
	}
}

func TestUnownedTrackedPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a zero Tracked accepted a Set")
		}
	}()
	var v component.Tracked[int]
	v.Set(1)
}
