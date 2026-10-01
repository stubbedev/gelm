package app

import (
	"errors"
	"testing"
	"time"

	"github.com/unxed/xkb-go"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// fakePopSurface records the loop's calls on a popover's popup.
type fakePopSurface struct {
	destroyed, dismissed bool
	frames               int
}

func (f *fakePopSurface) Destroyed() bool { return f.destroyed }
func (f *fakePopSurface) Dismissed() bool { return f.dismissed }
func (f *fakePopSurface) MarkFrame()      { f.frames++ }

// fakePopPainter records passes.
type fakePopPainter struct {
	passes, closes int
	err            error
}

func (f *fakePopPainter) Pass() (bool, error) { f.passes++; return true, f.err }
func (f *fakePopPainter) Close()              { f.closes++ }

func newLoopPopover(content widget.Widget) (*openPopover, *fakePopSurface, *fakePopPainter, *int) {
	surf, painter := &fakePopSurface{}, &fakePopPainter{}
	root := &popoverKeyRoot{onDismiss: func() {}, content: content}
	closed := 0
	op := &openPopover{
		host: &fakeHost{w: 100, h: 100}, pop: surf, painter: painter,
		router: &widget.Router{Root: root}, keyRoot: root,
		popover: &Popover{}, detach: func() {}, fireClosed: func() { closed++ },
	}
	return op, surf, painter, &closed
}

func TestDrivePopoversPaintsOnDamageAndReapsDestroyed(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	label := widget.NewLabel(face, 14, "a", render.RGB(255, 255, 255))
	op, surf, painter, closed := newLoopPopover(label)
	op.keyRoot.Arrange(render.Rect{W: 100, H: 30})
	widget.CollectDamage(op.keyRoot)
	a := &Application{openPopovers: []*openPopover{op}}
	a.popovers.openOrReplace(op.host, op.popover)

	// Nothing changed: a pass, no repaint request.
	if err := a.drivePopovers(false); err != nil {
		t.Fatal(err)
	}
	if painter.passes != 1 || surf.frames != 0 {
		t.Fatalf("idle: passes %d frames %d", painter.passes, surf.frames)
	}
	// Loop work ran (an Invoke): the content changed, so it repaints.
	label.SetText("b")
	if err := a.drivePopovers(false); err != nil {
		t.Fatal(err)
	}
	if surf.frames != 1 {
		t.Errorf("damaged content: frames %d, want a repaint", surf.frames)
	}
	// Work that ran is always a repaint request, damage or not.
	if err := a.drivePopovers(true); err != nil {
		t.Fatal(err)
	}
	if surf.frames != 2 {
		t.Errorf("worked pass: frames %d", surf.frames)
	}
	// The exit tween landed: reaped, closed once, registry cleared.
	surf.destroyed = true
	if err := a.drivePopovers(false); err != nil {
		t.Fatal(err)
	}
	if len(a.openPopovers) != 0 || painter.closes != 1 || *closed != 1 || a.popovers.open[op.host] != nil {
		t.Errorf("reap: %d open, %d closes, %d closed, registry %v", len(a.openPopovers), painter.closes, *closed, a.popovers.open[op.host])
	}
	if err := a.drivePopovers(false); err != nil || painter.passes != 3 {
		t.Errorf("a reaped popover was driven again: %d passes", painter.passes)
	}
}

func TestDrivePopoversSurfacesPaintErrors(t *testing.T) {
	op, _, painter, _ := newLoopPopover(widget.NewSpacer(1, 1))
	painter.err = errors.New("wire")
	a := &Application{openPopovers: []*openPopover{op}}
	if err := a.drivePopovers(false); err == nil {
		t.Error("a paint failure was swallowed")
	}
}

// A focused Entry inside a popover takes typed text and editing keys;
// Esc dismisses anyway; without focus, actions reach the content.
func TestPopoverKeysReachAFocusedEntry(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	entry := widget.NewEntry(face, 13, render.RGB(255, 255, 255))
	op, surf, _, _ := newLoopPopover(entry)
	dismissed := 0
	op.keyRoot.onDismiss = func() { dismissed++ }
	op.keyRoot.Arrange(render.Rect{W: 200, H: 30})
	tr := &fakeTranslator{
		text: map[uint32]string{30: "a", 48: "b"},
		syms: map[uint32]xkb.Keysym{30: xkb.Keysym('a'), 48: xkb.Keysym('b'), 14: xkb.KeyBackSpace, 1: xkb.KeyEscape},
	}
	a := &Application{accels: newAccelTable()}

	// Unfocused: a printable key does not type.
	a.deliverPopoverKey(tr, op, 30, 0)
	if entry.Text() != "" {
		t.Fatalf("unfocused entry got %q", entry.Text())
	}
	// Click to focus, then type and edit.
	op.router.Press(widget.BTNLeft, widget.Point{X: 5, Y: 5})
	op.router.Release(widget.BTNLeft, widget.Point{X: 5, Y: 5})
	a.deliverPopoverKey(tr, op, 30, 0)
	a.deliverPopoverKey(tr, op, 48, 0)
	a.deliverPopoverKey(tr, op, 14, 0)
	if entry.Text() != "a" {
		t.Errorf("typed text = %q, want a", entry.Text())
	}
	if surf.frames < 3 {
		t.Errorf("keys did not request repaints: %d", surf.frames)
	}
	a.deliverPopoverKey(tr, op, 1, 0)
	if dismissed != 1 {
		t.Errorf("Esc with a focused entry: dismissed %d", dismissed)
	}
}

func TestKeyPopoverIsTheNewestLiveOne(t *testing.T) {
	older, oldSurf, _, _ := newLoopPopover(widget.NewSpacer(1, 1))
	newer, newSurf, _, _ := newLoopPopover(widget.NewSpacer(1, 1))
	a := &Application{}
	if a.keyPopover() != nil {
		t.Error("no popovers: a key target")
	}
	a.openPopovers = []*openPopover{older, newer}
	if a.keyPopover() != newer {
		t.Error("the newest popover does not hold the keyboard")
	}
	newSurf.dismissed = true
	if a.keyPopover() != older {
		t.Error("a dismissing popover still takes keys")
	}
	oldSurf.dismissed = true
	if a.keyPopover() != nil {
		t.Error("all dismissing: still a key target")
	}
}

func TestPumpReportsWork(t *testing.T) {
	a := testApp(nil)
	if a.pump(time.Now()) {
		t.Error("an empty pump reported work")
	}
	a.Invoke(func() {})
	if !a.pump(time.Now()) {
		t.Error("an Invoke did not report work")
	}
}

func TestPopoverSetFocusRoutesThroughItsRouter(t *testing.T) {
	face, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	a, b := widget.NewEntry(face, 13, 0), widget.NewEntry(face, 13, 0)
	content := widget.NewBox(widget.Column, 0, 0)
	content.Append(a, false)
	content.Append(b, false)
	op, surf, _, _ := newLoopPopover(content)
	p := op.popover
	p.focus = func(w widget.Widget) { op.router.SetFocus(w); op.pop.MarkFrame() }
	p.SetFocus(b)
	if op.router.Focused() != b || surf.frames != 1 {
		t.Errorf("focused %v frames %d, want b and a repaint", op.router.Focused(), surf.frames)
	}
	p.markClosed()
	p.SetFocus(a)
	if op.router.Focused() != b {
		t.Error("a closed popover moved focus")
	}
	var none *Popover
	none.SetFocus(a) // nil-safe
}
