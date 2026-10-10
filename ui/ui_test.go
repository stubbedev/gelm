package ui_test

import (
	"bytes"
	"os"
	"slices"
	"strconv"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/stubbedev/gelm/component"
	"github.com/stubbedev/gelm/component/componenttest"
	"github.com/stubbedev/gelm/internal/uigen"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/ui"
	"github.com/stubbedev/gelm/widget"
)

func env(t *testing.T) ui.Env {
	t.Helper()
	f, err := render.LoadFont(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return ui.Env{Face: f, Size: 13}
}

func TestGeneratedBuildersAreCurrent(t *testing.T) {
	want, err := uigen.Generate("../widget")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("widgets_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("ui/widgets_gen.go is stale: run go generate ./ui")
	}
}

func TestBuildAppliesPropsHooksAndRefs(t *testing.T) {
	var label *widget.Label
	var button *widget.Button
	clicks := 0
	root, _ := ui.Build(env(t), ui.Column(
		ui.Label("hello").Ref(&label).Wrap(true).AddClass("title").Tooltip("greeting"),
		ui.Expand(ui.Button(ui.Label("go"), 8, 6).Ref(&button).OnClick(func() { clicks++ })),
	).Spacing(4))

	box, ok := root.(*widget.Box)
	if !ok || len(box.Children()) != 2 || box.Spacing() != 4 {
		t.Fatalf("root = %T with spacing %d", root, box.Spacing())
	}
	if label.Text() != "hello" || !widget.HasClass(label, "title") || label.TooltipText() != "greeting" {
		t.Errorf("label text=%q class=%v tooltip=%q", label.Text(), widget.HasClass(label, "title"), label.TooltipText())
	}
	if label.Color() != widget.Current().Text {
		t.Errorf("label ink = %v, want the theme text color", label.Color())
	}
	button.OnClick()
	if clicks != 1 {
		t.Errorf("OnClick hook ran %d times", clicks)
	}
}

func TestViewRefreshRunsWatches(t *testing.T) {
	n := 0
	var label *widget.Label
	_, view := ui.Build(env(t), ui.Label("").Ref(&label).WatchText(func() string { return strconv.Itoa(n) }))
	if label.Text() != "0" {
		t.Fatalf("initial watch gave %q", label.Text())
	}
	n = 7
	view.Refresh()
	if label.Text() != "7" {
		t.Errorf("after refresh label = %q, want 7", label.Text())
	}
}

func TestBindingsUnbindOnClose(t *testing.T) {
	text := widget.NewBinding("a")
	var entry *widget.Entry
	_, view := ui.Build(env(t), ui.Entry().Ref(&entry).BindText(text))
	if entry.Text() != "a" {
		t.Fatalf("bound entry shows %q", entry.Text())
	}
	view.Close()
	text.Set("b")
	if entry.Text() != "a" {
		t.Errorf("a closed view still follows its binding: %q", entry.Text())
	}
}

func TestOneBuilderBuildsOnceAcrossReferences(t *testing.T) {
	pages := ui.Stack().Add("one", ui.Label("1")).Add("two", ui.Label("2"))
	var switcher *widget.ViewSwitcher
	var stack *widget.Stack
	root, _ := ui.Build(env(t), ui.Column(
		ui.ViewSwitcher(pages, map[string]string{"one": "One", "two": "Two"}, nil).Ref(&switcher),
		pages.Ref(&stack),
	))
	if stack == nil || switcher == nil {
		t.Fatal("refs not filled")
	}
	if kids := root.(*widget.Box).Children(); len(kids) != 2 || kids[1] != widget.Widget(stack) {
		t.Errorf("the tree holds %v, want the switcher and the one stack it references", kids)
	}
}

type viewMsg int

const (
	bump viewMsg = iota
	toggle
)

type viewModel struct {
	env     ui.Env
	count   *component.Tracked[int]
	shown   bool
	label   *widget.Label
	tracked int
	stack   *widget.Stack
	mode    string
	match   *widget.Stack
}

func (m *viewModel) Init(cx *component.Context[viewMsg, struct{}]) widget.Widget {
	m.count = cx.Tracked(0)
	m.mode = "a"
	return ui.Mount(cx, m.env, ui.Column(
		ui.Label("").Ref(&m.label).WatchText(func() string { return strconv.Itoa(m.count.Get()) }),
		ui.Label("").Track(func(*widget.Label) { m.tracked++ }, m.count),
		ui.If(func() bool { return m.shown }, ui.Label("yes"), nil).Ref(&m.stack),
		ui.Match(func() string { return m.mode },
			ui.When("a", ui.Label("A")),
			ui.When("b", ui.Label("B")),
		).Ref(&m.match),
	))
}

func (m *viewModel) Update(_ *component.Context[viewMsg, struct{}], msg viewMsg) {
	switch msg {
	case bump:
		m.count.Update(func(n *int) { *n++ })
	case toggle:
		m.shown = !m.shown
		m.mode = "b"
	}
}

func TestMountedViewFollowsComponentUpdates(t *testing.T) {
	var loop componenttest.Loop
	m := &viewModel{env: env(t)}
	ctrl := component.Launch(&loop, m)
	if m.label.Text() != "0" || m.tracked != 1 || m.stack.Visible() != "else" || m.match.Visible() != "0" {
		t.Fatalf("initial view: label=%q tracked=%d if=%q match=%q", m.label.Text(), m.tracked, m.stack.Visible(), m.match.Visible())
	}
	ctrl.Send(bump)
	loop.Settle()
	if m.label.Text() != "1" || m.tracked != 2 {
		t.Errorf("after bump: label=%q tracked=%d", m.label.Text(), m.tracked)
	}
	ctrl.Send(toggle)
	loop.Settle()
	if m.tracked != 2 {
		t.Errorf("an update leaving count alone re-ran its track: %d", m.tracked)
	}
	if m.stack.Visible() != "then" || m.match.Visible() != "1" {
		t.Errorf("after toggle: if=%q match=%q", m.stack.Visible(), m.match.Visible())
	}
}

func TestEachAndAligned(t *testing.T) {
	root, _ := ui.Build(env(t), ui.Row(ui.Each(slices.Values([]string{"a", "b", "c"}), func(s string) ui.Node {
		return ui.Aligned(ui.Label(s), widget.AlignCenter)
	})...))
	if got := len(root.(*widget.Box).Children()); got != 3 {
		t.Errorf("Each built %d children, want 3", got)
	}
}

func TestMissingFacePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a label built without a typeface")
		}
	}()
	ui.Build(ui.Env{Size: 12}, ui.Label("x"))
}
