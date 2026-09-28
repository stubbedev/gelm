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

// attemptTimeout bounds one tap's effect in tests that retry taps:
// synthetic input on a loaded compositor can be lost, so a few short
// attempts beat one long wait.
const attemptTimeout = 3 * time.Second

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

// clickUntil clicks (x, y) until the expected trace lands: a freshly
// booted compositor eats the first synthetic presses (the socket is
// pollable long before the seat reliably delivers buttons), so one
// click there flakes the gate on lost input instead of a real
// regression. A few short attempts beat one long wait - the same trade
// tapUntil makes for keys. A lost press leaves no trace, so a retry is
// honest: the effect shows up exactly once, on the attempt that lands.
func clickUntil(w *LogWatcher, in *VirtualInput, category string, x, y int, button uint32, want string) error {
	for range 3 {
		if err := in.ClickAt(x, y, button); err != nil {
			return fmt.Errorf("click: %w", err)
		}
		if err := w.WaitAll(category, attemptTimeout, want); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no trace [%s] %q after 3 attempts; last tail:\n%s", category, want, w.Tail(25))
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
	if err := clickUntil(w, in, "demo", btn[0], btn[1], BTNLeft, "button clicked 1 times"); err != nil {
		t.Errorf("first click never reached the button: %v", err)
	}
	if err := clickUntil(w, in, "demo", btn[0], btn[1], BTNLeft, "button clicked 2 times"); err != nil {
		t.Errorf("second click never reached the button: %v", err)
	}

	// The switch starts on; one click turns it off.
	sw := click("switch")
	if err := clickUntil(w, in, "demo", sw[0], sw[1], BTNLeft, "switch false"); err != nil {
		t.Errorf("click never toggled the switch: %v", err)
	}

	// The checkbox starts off; one click turns it on.
	check := click("checkbox")
	if err := clickUntil(w, in, "demo", check[0], check[1], BTNLeft, "checkbox true"); err != nil {
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
		if err := clickUntil(w, in, "demo", btn[0], btn[1], BTNRight, "menu open "); err != nil {
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

	// Focusing the entry is a press, and a fresh compositor can eat
	// it; the typed text is the proof of focus, so retry the pair and
	// let the assert name the failure if focus never took.
	for attempt := range 3 {
		if err := in.ClickAt(entry[0], entry[1], BTNLeft); err != nil {
			t.Fatalf("focus entry: %v", err)
		}
		if err := in.TypeText("abc"); err != nil {
			t.Fatalf("type abc: %v", err)
		}
		if _, err := w.Wait("demo", `entry: "abc"`, attemptTimeout); err == nil {
			break
		} else if attempt == 2 {
			t.Fatalf("typing never reached the entry: %v", err)
		}
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

// startStatesClient launches the window-state client (cmd/gelm-states)
// with demo (the client's confirmed-state traces) and shell (the
// window's own state-change traces) on, and attaches a watcher.
func startStatesClient(t *testing.T) (*Client, *LogWatcher) {
	t.Helper()
	bin, err := BuildClient(testEnv.Dir, "./cmd/gelm-states", "gelm-states")
	if err != nil {
		t.Fatal(err)
	}
	c, err := testEnv.StartClient(bin, "client-"+t.Name(), "input,frame,demo,shell")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Stop)
	w, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	return c, w
}

// TestHeadlessWindowStates is the acceptance proof for the window
// state model (#47) against a live compositor: each chord's request is
// judged only by the state the compositor CONFIRMS in a configure -
// including a refusal (sway, like i3, ignores set_maximized on a
// floating window, and the reported state must stay what was confirmed
// before) - the maximize/fullscreen confirmations arrive as
// output-sized configures whose frames draw at the new size through the
// one relayout path, and a compositor-initiated close while fullscreen
// tears the client down cleanly.
func TestHeadlessWindowStates(t *testing.T) {
	requireEnv(t)
	c, w := startStatesClient(t)
	exited := c.Exited()
	var in *VirtualInput
	tap := func(rs ...rune) {
		t.Helper()
		for _, r := range rs {
			code, _, err := KeyFor(r)
			if err != nil {
				t.Fatal(err)
			}
			if err := in.Tap(code); err != nil {
				t.Fatalf("tap %q: %v", r, err)
			}
		}
	}
	// tapUntil taps r until the expected effect traces appear (each
	// substr matched by one line, in any order), retrying a few times:
	// synthetic input on a loaded compositor can be lost, and the
	// assertions here judge confirmed state, not first-attempt delivery.
	// Halfway through, the virtual seat is re-dialed: a fresh keyboard
	// re-arms focus delivery when the old device stops producing keys.
	tapUntil := func(r rune, want ...string) {
		t.Helper()
		for attempt := 0; ; attempt++ {
			if attempt == 4 {
				t.Fatalf("tap %q never produced %v: %s", r, want, tailTraces(w, 15))
			}
			if attempt == 2 {
				in = newInput(t)
			}
			tap(r)
			if err := w.WaitAll("", attemptTimeout, want...); err == nil {
				return
			}
		}
	}
	// Confirmed-state traces read "state maximized=<bool> fullscreen=<bool>
	// <w>x<h>" (flag-form, so one bit matches regardless of the tiled_*/
	// activated bits sway carries alongside); the poll chord reads
	// "polled ...". The waits match the bare flag pair, since maximized=
	// always precedes fullscreen= in the print order.

	// The recipe pins the client to a floating 420x280 window; that size
	// in the first confirmed-state trace is the pin assertion. The
	// virtual seat is dialed only after the map, so keyboard delivery
	// starts from a mapped, focused surface.
	if _, err := w.Wait("demo", "420x280", traceTimeout); err != nil {
		t.Fatalf("states client never mapped at the pinned %dx%d: %v", statesW, statesH, err)
	}
	in = newInput(t)
	// A refusing compositor, pinned live: sway 1.11 never honors
	// set_maximized - its request handler only schedules a configure
	// that carries the state UNCHANGED, which is exactly the issue's
	// rule that a configure without the requested state reports the
	// request unconfirmed. The on-demand poll reads the reported state
	// after the refused request, and a short negative wait fails if a
	// confirm ever sneaks in (update this test if sway changes).
	tapUntil('m', "requested maximize")
	tapUntil('p', "polled maximized=false")
	if _, err := w.Wait("demo", "maximized=true", time.Second); err == nil {
		t.Errorf("the maximize was confirmed; the suite expected sway to refuse it")
	}

	// Fullscreen from floating: the output-sized configure is a REAL
	// relayout (420x280 -> 1280x800), so the first frame drawn at the
	// fullscreen size is the live proof of the syncSize path.
	tapUntil('f', "fullscreen=true", fmt.Sprintf("draw %dx%d", outputW, outputH))
	select {
	case <-exited:
		t.Fatalf("client died during the fullscreen transition; log tail:\n%s", tailTraces(w, 15))
	default:
	}

	// Unfullscreen restores the pinned floating size.
	tapUntil('g', fmt.Sprintf("fullscreen=false %dx%d", statesW, statesH))

	// Close while fullscreen: fullscreen again, then the COMPOSITOR
	// closes the window (sway kill, the real xdg_toplevel.close event).
	// The client must tear down cleanly - no panic, exit 0.
	tapUntil('f', "fullscreen=true")
	if err := testEnv.SwayCommand(`[app_id="` + StatesAppID + `"] kill`); err != nil {
		t.Fatalf("sway close: %v", err)
	}
	select {
	case <-exited:
		if err := c.Wait(); err != nil {
			t.Errorf("close while fullscreen did not exit cleanly: %v\nlog tail:\n%s", err, tailTraces(w, 15))
		}
	case <-time.After(traceTimeout):
		t.Errorf("client outlived the compositor close while fullscreen; log tail:\n%s", tailTraces(w, 25))
	}
}

// startMultilistClient launches the multi-select list client
// (cmd/gelm-multilist) and returns it with a trace watcher and the row
// centers the client traced after mapping at the pinned size.
func startMultilistClient(t *testing.T) (*Client, *LogWatcher, [][2]int) {
	t.Helper()
	bin, err := BuildClient(testEnv.Dir, "./cmd/gelm-multilist", "gelm-multilist")
	if err != nil {
		t.Fatal(err)
	}
	c, err := testEnv.StartClient(bin, "client-"+t.Name(), "input,frame,demo")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Stop)
	w, err := c.Watch()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Wait("demo", fmt.Sprintf("mapped %dx%d", multilistW, multilistH), traceTimeout); err != nil {
		t.Fatalf("multilist never mapped at the pinned %dx%d: %v", multilistW, multilistH, err)
	}
	rows := make([][2]int, multilistRows)
	for i := range rows {
		tr, err := w.Wait("demo", fmt.Sprintf("row %d center ", i), traceTimeout)
		if err != nil {
			t.Fatalf("row %d center trace missing: %v", i, err)
		}
		var x, y int
		if _, err := fmt.Sscanf(tr.Message, fmt.Sprintf("row %d center (%%d,%%d)", i), &x, &y); err != nil {
			t.Fatalf("bad row trace %q: %v", tr.Message, err)
		}
		rows[i] = [2]int{x, y}
	}
	return c, w, rows
}

// TestHeadlessListMultiSelect drives the multiple-selection list model
// (#60) through the compositor: clicks toggle, ctrl+space toggles the
// cursor row, shift+arrows extend from the anchor, ctrl+a selects
// everything, a pointer drag rubber-bands a range, and a rapid second
// click activates the row instead of toggling it back out.
func TestHeadlessListMultiSelect(t *testing.T) {
	requireEnv(t)
	_, w, rows := startMultilistClient(t)
	in := newInput(t)

	// Click row 2: the toggle lands through the compositor's pointer
	// path and focuses the list for the keyboard half of the test.
	if err := clickUntil(w, in, "demo", rows[2][0], rows[2][1], BTNLeft, "selection [2]"); err != nil {
		t.Fatalf("click on row 2 never toggled it in: %v", err)
	}

	// ctrl+space toggles the cursor row back out, then in.
	for pass := range 2 {
		expect := "selection [none]"
		if pass == 1 {
			expect = "selection [2]"
		}
		if err := in.Combo(KeySpace); err != nil {
			t.Fatalf("ctrl+space: %v", err)
		}
		if _, err := w.Wait("demo", expect, traceTimeout); err != nil {
			t.Errorf("ctrl+space pass %d never produced %q: %v", pass+1, expect, err)
		}
	}

	// shift+Down extends the anchor range.
	if err := in.mods(ModShift); err != nil {
		t.Fatalf("shift: %v", err)
	}
	if err := in.Tap(KeyDown); err != nil {
		t.Fatalf("shift+down: %v", err)
	}
	if err := in.mods(0); err != nil {
		t.Fatalf("shift release: %v", err)
	}
	if _, err := w.Wait("demo", "selection [2,3]", traceTimeout); err != nil {
		t.Errorf("shift+down never extended the anchor range: %v", err)
	}

	// ctrl+a takes everything through the select-all route.
	if err := in.Combo(KeyA); err != nil {
		t.Fatalf("ctrl+a: %v", err)
	}
	if _, err := w.Wait("demo", "selection [0,1,2,3,4,5,6,7,8,9]", traceTimeout); err != nil {
		t.Errorf("ctrl+a never selected everything: %v", err)
	}

	// A pointer drag rubber-bands a range and reports it once.
	if err := in.DragTo(rows[5][0], rows[5][1], rows[7][0], rows[7][1], 4); err != nil {
		t.Fatalf("drag rows 5..7: %v", err)
	}
	if _, err := w.Wait("demo", "selection [5,6,7]", traceTimeout); err != nil {
		t.Errorf("the drag never rubber-banded [5,6,7]: %v", err)
	}

	// A rapid second click on a row activates it (double-click opens)
	// without toggling its membership back out: the first click removes
	// row 6 from the set, the pair then activates it.
	if err := in.ClickAt(rows[6][0], rows[6][1], BTNLeft); err != nil {
		t.Fatalf("first click on row 6: %v", err)
	}
	if _, err := w.Wait("demo", "selection [5,7]", traceTimeout); err != nil {
		t.Errorf("the first click of the pair never toggled row 6 out: %v", err)
	}
	if err := in.ClickAt(rows[6][0], rows[6][1], BTNLeft); err != nil {
		t.Fatalf("second click on row 6: %v", err)
	}
	if _, err := w.Wait("demo", "activated 6", traceTimeout); err != nil {
		t.Errorf("the rapid pair never activated row 6: %v", err)
	}
}
