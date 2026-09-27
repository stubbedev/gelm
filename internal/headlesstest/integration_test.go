package headlesstest

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// The compositor-in-the-loop suite: attaches to the private headless
// sway session the just headless recipe booted (TestMain, only when
// GELM_HEADLESS is set), runs the widget showcase inside it, and drives
// a synthetic seat against it. Every
// assertion lands on the client's own trace log - the input must have
// traversed compositor, wire, session and widget tree for a trace to
// appear, which is what makes this a regression net for live-input
// bugs (#1) that widget-tree unit tests cannot see.

// run gates the suite: without GELM_HEADLESS the integration tests
// skip (local dev has no compositor), with it set the env is required
// and a missing compositor fails the package - a skip behind
// GELM_HEADLESS would be a silent hole in CI.
func run(m *testing.M) int {
	if os.Getenv("GELM_HEADLESS") == "" {
		return m.Run()
	}
	env, err := Attach()
	if err != nil {
		fmt.Fprintf(os.Stderr, "headlesstest: %v\n", err)
		return 1
	}
	testEnv = env
	bin, err := BuildShowcase(env.Dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "headlesstest: %v\n", err)
		return 1
	}
	testBin = bin
	return m.Run()
}

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

var (
	testEnv *Env
	testBin string
)

// traceTimeout bounds every single-trace wait. Compositor scheduling
// adds jitter; nothing here should ever need this long, so exceeding
// it is a real failure.
const traceTimeout = 8 * time.Second

// dwellTime is how long the pointer rests on a widget in the tooltip
// test: past app's 500ms tooltip delay, short of test patience.
const dwellTime = 900 * time.Millisecond

func requireEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("GELM_HEADLESS") == "" {
		t.Skip("GELM_HEADLESS is not set; compositor input tests run under just check-headless")
	}
}

// startShowcase launches a fresh showcase client (its own log, its own
// window) and returns it with a trace watcher and the control centers
// the client itself traced after mapping.
func startShowcase(t *testing.T) (*Client, *LogWatcher, map[string][2]int) {
	t.Helper()
	c, err := testEnv.StartShowcase(testBin, "client-"+t.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c.Stop()
		// On failure, the whole client log is the evidence; the trace
		// tails a Wait error carries cover only what was read in time.
		if t.Failed() {
			if data, err := os.ReadFile(c.LogPath); err == nil {
				lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
				if len(lines) > 120 {
					lines = lines[len(lines)-120:]
				}
				t.Logf("client log tail:\n\t%s", strings.Join(lines, "\n\t"))
			}
		}
	})
	w, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Wait("demo", fmt.Sprintf("mapped %dx%d", showcaseW, showcaseH), traceTimeout); err != nil {
		t.Fatalf("showcase never mapped at the pinned %dx%d: %v\ncompositor log tail:\n\t%s",
			showcaseW, showcaseH, err, strings.ReplaceAll(tailFile(testEnv.Dir+"/sway.log", 15), "\n", "\n\t"))
	}
	centers := make(map[string][2]int)
	for range 7 {
		tr, err := w.Wait("demo", "control ", traceTimeout)
		if err != nil {
			t.Fatalf("control center trace missing: %v", err)
		}
		name, xy, err := parseCenter(tr.Message)
		if err != nil {
			t.Fatalf("bad control trace %q: %v", tr.Message, err)
		}
		centers[name] = xy
	}
	return c, w, centers
}

func parseCenter(msg string) (string, [2]int, error) {
	var name string
	var x, y int
	if _, err := fmt.Sscanf(msg, "control %s center (%d,%d)", &name, &x, &y); err != nil {
		return "", [2]int{}, err
	}
	return name, [2]int{x, y}, nil
}

// newInput connects a synthetic seat to the test compositor.
func newInput(t *testing.T) *VirtualInput {
	t.Helper()
	v, err := Dial(os.Getenv("WAYLAND_DISPLAY"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	return v
}

// newPointerOnlyInput connects a synthetic seat with no keyboard.
func newPointerOnlyInput(t *testing.T) *VirtualInput {
	t.Helper()
	v, err := DialPointerOnly(os.Getenv("WAYLAND_DISPLAY"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	return v
}

// TestHeadlessClicksOnShowcaseControls is the regression net for #1:
// synthetic clicks must land on real widgets and the showcase's own
// handlers must run. A broken click path dies here before it can be
// dead-on-hardware.
func TestHeadlessClicksOnShowcaseControls(t *testing.T) {
	requireEnv(t)
	_, w, centers := startShowcase(t)
	in := newInput(t)

	click := func(name string) [2]int {
		t.Helper()
		c, ok := centers[name]
		if !ok {
			t.Fatalf("no traced center for %s", name)
		}
		return c
	}

	// The button: two clicks, two activations.
	btn := click("button")
	in.ClickAt(btn[0], btn[1], BTNLeft)
	if _, err := w.Wait("demo", "button clicked 1 times", traceTimeout); err != nil {
		t.Errorf("first click never reached the button: %v", err)
	}
	in.ClickAt(btn[0], btn[1], BTNLeft)
	if _, err := w.Wait("demo", "button clicked 2 times", traceTimeout); err != nil {
		t.Errorf("second click never reached the button: %v", err)
	}

	// The switch starts on; one click turns it off.
	in.ClickAt(click("switch")[0], click("switch")[1], BTNLeft)
	if _, err := w.Wait("demo", "switch false", traceTimeout); err != nil {
		t.Errorf("click never toggled the switch: %v", err)
	}

	// The checkbox starts off; one click turns it on.
	in.ClickAt(click("checkbox")[0], click("checkbox")[1], BTNLeft)
	if _, err := w.Wait("demo", "checkbox true", traceTimeout); err != nil {
		t.Errorf("click never checked the checkbox: %v", err)
	}

	// A drag mid-track moves the slider: this slider changes value on
	// drag and arrows only (a plain click on the trough is a no-op).
	s := click("slider")
	if err := in.DragTo(s[0], s[1], s[0]+80, s[1], 4); err != nil {
		t.Fatalf("drag slider: %v", err)
	}
	if _, err := w.Wait("demo", "slider at ", traceTimeout); err != nil {
		t.Errorf("drag never set the slider: %v", err)
	}

	// Clicking the entry focuses it; keys then land in it.
	in.ClickAt(click("entry")[0], click("entry")[1], BTNLeft)
	if err := in.TypeText("hi"); err != nil {
		t.Fatalf("type hi: %v", err)
	}
	if _, err := w.Wait("demo", `entry: "hi"`, traceTimeout); err != nil {
		t.Errorf("typed keys never reached the focused entry: %v", err)
	}

	// The text area receives presses (no demo hook fires for it, so
	// assert the routed press itself).
	in.ClickAt(click("textarea")[0], click("textarea")[1], BTNLeft)
	if _, err := w.Wait("input", "over *widget.TextArea", traceTimeout); err != nil {
		t.Errorf("press never routed to the text area: %v", err)
	}

	// A wheel tick over the list must scroll it: assert the offset the
	// showcase reports, not just the routed hover (which names the leaf
	// label inside the viewport and would pass on an ignored wheel).
	in.ScrollAt(click("scroll")[0], click("scroll")[1], 60)
	if _, err := w.Wait("demo", "list scrolled to 0,", traceTimeout); err != nil {
		t.Errorf("wheel event never scrolled the list: %v", err)
	}
}

// TestHeadlessPopupGrabOpenAndDismiss covers the context menu: open on
// right-click, dismiss by outside click (the popup grab), dismiss by
// Escape, and keyboard activation through the grab.
func TestHeadlessPopupGrabOpenAndDismiss(t *testing.T) {
	requireEnv(t)
	_, w, centers := startShowcase(t)
	in := newInput(t)
	btn := centers["button"]

	openMenu := func(when string) {
		t.Helper()
		if err := in.ClickAt(btn[0], btn[1], BTNRight); err != nil {
			t.Fatalf("%s: right-click: %v", when, err)
		}
		if _, err := w.Wait("demo", "menu open ", traceTimeout); err != nil {
			t.Fatalf("%s: menu never opened: %v", when, err)
		}
		if _, err := w.Wait("input", "popup surface ", traceTimeout); err != nil {
			t.Fatalf("%s: popup surface never created: %v", when, err)
		}
		// A popup whose grab protocol state is wrong (grab after commit,
		// say) gets killed by strict compositors; a rejected grab must
		// never pass for success.
		for _, tr := range w.Tail(50) {
			if strings.Contains(tr.Message, "compositor fatal") {
				t.Fatalf("%s: compositor rejected the popup: %s", when, tr)
			}
		}
	}

	// Outside click while the grab is live dismisses the menu.
	openMenu("first")
	if err := in.ClickAt(10, showcaseH-10, BTNLeft); err != nil {
		t.Fatalf("outside click: %v", err)
	}
	if _, err := w.Wait("input", " closed", traceTimeout); err != nil {
		t.Errorf("popup grab never dismissed on outside click: %v", err)
	}

	// Escape dismisses too.
	openMenu("second")
	if err := in.Tap(KeyEscape); err != nil {
		t.Fatalf("escape: %v", err)
	}
	if _, err := w.Wait("input", " closed", traceTimeout); err != nil {
		t.Errorf("popup never dismissed on escape: %v", err)
	}

	// Keyboard through the grab: the pointer rests at the popup's
	// anchor, so its hover has row 0; Down moves to the second row and
	// Enter activates it.
	openMenu("third")
	if err := in.Tap(KeyDown); err != nil {
		t.Fatalf("down: %v", err)
	}
	if err := in.Tap(KeyEnter); err != nil {
		t.Fatalf("enter: %v", err)
	}
	if _, err := w.Wait("demo", "theme light", traceTimeout); err != nil {
		t.Errorf("menu item never activated through the grab: %v", err)
	}
}

// TestHeadlessTooltipLifecycle: dwell opens the tooltip, moving on
// closes it, and a new dwell opens the next widget's tooltip. The
// client must survive the whole cycle - a tooltip popup that takes the
// connection down (a second wp_viewport on the popup surface once did
// exactly that) shows up as a dead client and a click that never
// lands, not just as a missing trace.
func TestHeadlessTooltipLifecycle(t *testing.T) {
	requireEnv(t)
	c, w, centers := startShowcase(t)
	in := newInput(t)
	exited := c.Exited()
	btn, entry := centers["button"], centers["entry"]

	alive := func(when string) {
		t.Helper()
		select {
		case <-exited:
			t.Fatalf("client died %s; log tail:\n%s", when, tailTraces(w, 15))
		default:
		}
	}
	assertNoFatal := func(when string) {
		t.Helper()
		for _, tr := range w.Tail(500) {
			if strings.Contains(tr.Message, "compositor fatal") {
				t.Fatalf("compositor rejected the client %s: %s", when, tr)
			}
		}
	}

	if err := in.MoveTo(btn[0], btn[1]); err != nil {
		t.Fatalf("move to button: %v", err)
	}
	time.Sleep(dwellTime)
	if _, err := w.Wait("input", `tooltip open "increments the counter`, traceTimeout); err != nil {
		t.Errorf("dwell never opened the button tooltip: %v", err)
	}
	alive("after the button tooltip opened")
	assertNoFatal("after the button tooltip opened")

	// Hovering something else closes the old tooltip.
	if err := in.MoveTo(entry[0], entry[1]); err != nil {
		t.Fatalf("move to entry: %v", err)
	}
	if _, err := w.Wait("input", "tooltip closed", traceTimeout); err != nil {
		t.Errorf("tooltip never closed on hover change: %v", err)
	}

	// And the entry earns its own after its dwell.
	time.Sleep(dwellTime)
	if _, err := w.Wait("input", `tooltip open "single-line entry`, traceTimeout); err != nil {
		t.Errorf("dwell never opened the entry tooltip: %v", err)
	}
	alive("after the entry tooltip opened")

	// Input still flows after the tooltip lifecycle: clicking the
	// button through the closing tooltip must activate it.
	in.ClickAt(btn[0], btn[1], BTNLeft)
	if _, err := w.Wait("demo", "button clicked 1 times", traceTimeout); err != nil {
		t.Errorf("click after the tooltip lifecycle never reached the button: %v", err)
	}
	alive("after the post-tooltip click")
	assertNoFatal("after the post-tooltip click")
}

// tailTraces renders up to n recent traces for failure messages.
func tailTraces(w *LogWatcher, n int) string {
	var b strings.Builder
	for _, tr := range w.Tail(n) {
		b.WriteString("  ")
		b.WriteString(tr.String())
		b.WriteByte('\n')
	}
	return b.String()
}

// TestHeadlessKeyboardTraversal: Tab walks the controls in order and
// Enter/Space/arrows drive them, all through the compositor's real
// keyboard path; Escape closes the window and the client exits 0.
func TestHeadlessKeyboardTraversalAndEscape(t *testing.T) {
	requireEnv(t)
	c, w, _ := startShowcase(t)
	in := newInput(t)

	// Tab lands on the button; Enter clicks it.
	if err := in.Tap(KeyTab); err != nil {
		t.Fatalf("tab: %v", err)
	}
	if err := in.Tap(KeyEnter); err != nil {
		t.Fatalf("enter: %v", err)
	}
	if _, err := w.Wait("demo", "button clicked 1 times", traceTimeout); err != nil {
		t.Errorf("Tab+Enter never clicked the button: %v", err)
	}

	// Tab reaches the slider; the right arrow raises it.
	if err := in.Tap(KeyTab); err != nil {
		t.Fatalf("tab: %v", err)
	}
	if err := in.Tap(KeyRight); err != nil {
		t.Fatalf("right: %v", err)
	}
	if _, err := w.Wait("demo", "slider at ", traceTimeout); err != nil {
		t.Errorf("arrow key never moved the focused slider: %v", err)
	}

	// Tab reaches the switch; Enter toggles it off.
	if err := in.Tap(KeyTab); err != nil {
		t.Fatalf("tab: %v", err)
	}
	if err := in.Tap(KeyEnter); err != nil {
		t.Fatalf("enter: %v", err)
	}
	if _, err := w.Wait("demo", "switch false", traceTimeout); err != nil {
		t.Errorf("Enter never toggled the focused switch: %v", err)
	}

	// Tab reaches the checkbox; Space toggles it on.
	if err := in.Tap(KeyTab); err != nil {
		t.Fatalf("tab: %v", err)
	}
	if err := in.Tap(KeySpace); err != nil {
		t.Fatalf("space: %v", err)
	}
	if _, err := w.Wait("demo", "checkbox true", traceTimeout); err != nil {
		t.Errorf("Space never toggled the focused checkbox: %v", err)
	}

	// Escape closes the window and the showcase exits cleanly.
	if err := in.Tap(KeyEscape); err != nil {
		t.Fatalf("escape: %v", err)
	}
	select {
	case <-c.Exited():
		if err := c.Wait(); err != nil {
			t.Errorf("escape did not close the showcase cleanly: %v\nlog tail:\n%s", err, tailTraces(w, 15))
		}
	case <-time.After(traceTimeout):
		t.Errorf("escape did not close the showcase within %s; log tail:\n%s", traceTimeout, tailTraces(w, 25))
	}
}

// TestHeadlessPointerOnlySeat runs the tooltip lifecycle and a click
// on a keyboard-less seat: the shape that once killed the client
// outright. Hover surfaces must open, the client must live through
// them, and the pointer must keep working afterwards.
func TestHeadlessPointerOnlySeat(t *testing.T) {
	requireEnv(t)
	c, w, centers := startShowcase(t)
	in := newPointerOnlyInput(t)
	exited := c.Exited()
	btn := centers["button"]

	if err := in.MoveTo(btn[0], btn[1]); err != nil {
		t.Fatalf("move to button: %v", err)
	}
	time.Sleep(dwellTime)
	if _, err := w.Wait("input", `tooltip open "increments the counter`, traceTimeout); err != nil {
		t.Errorf("tooltip never opened on a pointer-only seat: %v", err)
	}
	select {
	case <-exited:
		t.Fatalf("client died when the tooltip opened; log tail:\n%s", tailTraces(w, 15))
	default:
	}

	// The pointer still works after the tooltip cycle.
	if err := in.ClickAt(btn[0], btn[1], BTNLeft); err != nil {
		t.Fatalf("click: %v", err)
	}
	if _, err := w.Wait("demo", "button clicked 1 times", traceTimeout); err != nil {
		t.Errorf("click never reached the button on a pointer-only seat: %v", err)
	}
	select {
	case <-exited:
		t.Fatalf("client died before the test ended; log tail:\n%s", tailTraces(w, 15))
	default:
	}
}

// TestHeadlessClipboardRoundtrip: type into the entry, copy the
// selection to the compositor clipboard, paste it back - the text only
// doubles if the client's set_selection and the compositor's offer
// pipeline both work.
func TestHeadlessClipboardRoundtrip(t *testing.T) {
	requireEnv(t)
	_, w, centers := startShowcase(t)
	in := newInput(t)
	entry := centers["entry"]

	if err := in.ClickAt(entry[0], entry[1], BTNLeft); err != nil {
		t.Fatalf("focus entry: %v", err)
	}
	if err := in.TypeText("abc"); err != nil {
		t.Fatalf("type abc: %v", err)
	}
	if _, err := w.Wait("demo", `entry: "abc"`, traceTimeout); err != nil {
		t.Fatalf("typing never reached the entry: %v", err)
	}

	if err := in.Combo(KeyA); err != nil { // select all
		t.Fatalf("ctrl+a: %v", err)
	}
	if err := in.Combo(KeyC); err != nil { // copy
		t.Fatalf("ctrl+c: %v", err)
	}
	if err := in.Tap(KeyEnd); err != nil { // collapse to end-of-text
		t.Fatalf("end: %v", err)
	}
	if err := in.Combo(KeyV); err != nil { // paste
		t.Fatalf("ctrl+v: %v", err)
	}
	if _, err := w.Wait("demo", `entry: "abcabc"`, traceTimeout); err != nil {
		t.Errorf("clipboard roundtrip failed: text never came back doubled: %v", err)
	}
}
