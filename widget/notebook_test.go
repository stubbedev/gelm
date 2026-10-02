package widget

import (
	"testing"

	"github.com/stubbedev/gelm/internal/style"
	"github.com/stubbedev/gelm/render"
)

func newNotebook(t *testing.T) *Notebook {
	t.Helper()
	n := NewNotebook(entryFace(t))
	for _, name := range []string{"one", "two", "three"} {
		n.AppendTab(name, newTextArea(t, "page "+name))
	}
	n.Arrange(render.Rect{X: 0, Y: 0, W: 400, H: 300})
	return n
}

// TestNotebookSelect pins page switching: select shows the named page,
// unknown names are no-ops, and OnSelect fires only on real changes.
func TestNotebookSelect(t *testing.T) {
	n := newNotebook(t)

	t.Run("select switches the visible child", func(t *testing.T) {
		kids := n.Children()
		if len(kids) != 1 {
			t.Fatalf("children = %d, want exactly the visible page", len(kids))
		}
		n.SelectTab("two")
		kids = n.Children()
		if len(kids) != 1 || kids[0] != n.tabs[1].w {
			t.Errorf("children did not switch to page two")
		}
	})

	t.Run("selecting an unknown tab is a no-op", func(t *testing.T) {
		n.SelectTab("two")
		n.SelectTab("nope")
		if got := n.SelectedTab(); got != "two" {
			t.Errorf("selected = %q, want two after unknown no-op", got)
		}
	})

	t.Run("OnSelect fires on change and not on re-select", func(t *testing.T) {
		fired := 0
		n.OnSelect = func(string) { fired++ }
		n.SelectTab("three")
		n.SelectTab("three")
		if fired != 1 {
			t.Errorf("OnSelect fired %d times, want 1", fired)
		}
	})
}

func TestNotebookPaintAndHit(t *testing.T) {
	n := newNotebook(t)
	cv := render.New(make([]uint8, 400*300*4), 400*4, 400, 300)

	t.Run("paint runs with a selected page", func(t *testing.T) {
		n.SelectTab("one")
		n.Paint(cv)
	})

	t.Run("tab strip hit selects on click", func(t *testing.T) {
		// Third tab strip: x = 2*tabWidth, inside the bar height.
		p := Point{X: 2*tabWidth + 10, Y: 10}
		if got := n.HitTest(p); got != Widget(n) {
			t.Errorf("tab strip hit = %v, want the notebook", got)
		}
		n.ClickAt(p)
		if got := n.SelectedTab(); got != "three" {
			t.Errorf("selected = %q, want three", got)
		}
	})

	t.Run("close box fires the close hook instead of selecting", func(t *testing.T) {
		n.Closable = true
		closed := ""
		n.OnTabClose = func(name string) { closed = name }
		rect := n.tabRect(1)
		n.ClickAt(Point{X: rect.X + rect.W - 10, Y: rect.Y + 12})
		if closed != "two" {
			t.Errorf("OnTabClose = %q, want two", closed)
		}
		if got := n.SelectedTab(); got == "two" {
			t.Error("close box selected the tab instead of closing")
		}
	})
}

// TestNotebookTraversal pins that keyboard traversal only ever sees
// the visible page, and ctrl+PageUp/PageDown cycle tabs.
func TestNotebookTraversal(t *testing.T) {
	n := newNotebook(t)
	n.SelectTab("two")

	var visited []string
	walker := func(w Widget) {
		if e, ok := w.(*TextArea); ok {
			visited = append(visited, e.Text())
		}
	}
	focusWalker(n, walker)
	if len(visited) != 1 || visited[0] != "page two" {
		t.Errorf("traversal visited %v, want only the visible page", visited)
	}

	n.KeyAction(KeyPriorPage, ModCtrl)
	if got := n.SelectedTab(); got != "one" {
		t.Errorf("after ctrl+PageUp selected = %q, want one", got)
	}
	n.KeyAction(KeyNextPage, ModCtrl)
	n.KeyAction(KeyNextPage, ModCtrl)
	if got := n.SelectedTab(); got != "three" {
		t.Errorf("after ctrl+PageDown x2 selected = %q, want three", got)
	}
}

// The tab bar styles as `notebook > header > tabs > tab`: the header
// and each tab take their own backgrounds, the selected tab is
// :checked, and its label takes the tab's color.
func TestNotebookTabNodes(t *testing.T) {
	loadCSS(t, `notebook header { background-color: #010203; } notebook header tabs tab { color: #0a0b0c; } notebook header tabs tab:checked { background-color: #0d0e0f; color: #112233; }`)
	n := NewNotebook(testFace(t))
	n.AppendTab("one", NewSpacer(20, 20))
	n.AppendTab("two", NewSpacer(20, 20))
	host := NewBox(Column, 0, 0)
	host.Append(n, true)
	frame(t, host, 200, 60)
	if got := n.header.style(&n.header).Background; got != render.RGB(0x01, 0x02, 0x03) {
		t.Errorf("header background %v, want the header rule", got)
	}
	first, second := &n.header.tabs.tab[0], &n.header.tabs.tab[1]
	if got := first.style(first).Background; got != render.RGB(0x0d, 0x0e, 0x0f) {
		t.Errorf("selected tab background %v, want the :checked rule", got)
	}
	if got := pickc(0, first.style(first), style.PropColor, 0); got != render.RGB(0x11, 0x22, 0x33) {
		t.Errorf("selected tab color %v, want the :checked rule", got)
	}
	if !first.HasState(StateChecked) || second.HasState(StateChecked) {
		t.Errorf("checked states: first %v second %v", first.HasState(StateChecked), second.HasState(StateChecked))
	}
	if got := second.style(second).Background; got != 0 {
		t.Errorf("unselected tab background %v, want none", got)
	}
	n.SelectTab("two")
	if second.style(second).Background != render.RGB(0x0d, 0x0e, 0x0f) || first.style(first).Background != 0 {
		t.Errorf("after the move: first %v second %v", first.style(first).Background, second.style(second).Background)
	}
}
