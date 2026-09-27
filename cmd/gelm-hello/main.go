// Command gelm-hello is the showcase demo: one xdg_toplevel window
// exercising every widget - labels, button, slider, progress bar,
// switch, checkbox, text entry, multi-line text area, a scrollable
// list, hover tooltips, animated tweens, the right-click context menu,
// Tab focus traversal with the focus ring, drag-to-move, and Escape to
// close.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/internal/anim"
	"github.com/stubbedev/gelm/internal/clipboard"
	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/popup"
	"github.com/stubbedev/gelm/internal/scale"
	"github.com/stubbedev/gelm/internal/sysfont"
	"github.com/stubbedev/gelm/internal/window"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	sess, err := wlsession.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	if sess.WmBase() == nil {
		return errors.New("gelm-hello: compositor has no xdg_wm_base; windows unsupported")
	}
	tf, err := sysfont.Sans()
	if err != nil {
		return err
	}
	// Fallback chains cover what the default sans lacks: CJK, emoji,
	// Cyrillic — anything the system store has a face for.
	font := sysfont.Fallback(tf)

	surf, err := sess.Compositor().CreateSurface()
	if err != nil {
		return fmt.Errorf("gelm-hello: create surface: %w", err)
	}
	win, err := window.New(sess.WmBase(), surf, window.Config{
		Title:  "gelm showcase",
		AppID:  "dev.stubbe.gelm.hello",
		Width:  640,
		Height: 470,
	})
	if err != nil {
		return err
	}

	show := buildUI(font)

	if err := surf.Commit(); err != nil {
		return fmt.Errorf("gelm-hello: initial commit: %w", err)
	}
	// The configure events can land after a sync callback completes, so
	// dispatch until the handshake finishes.
	for range 20 {
		if win.EnsureUsable() == nil {
			break
		}
		if err := sess.Roundtrip(); err != nil {
			return fmt.Errorf("gelm-hello: configure roundtrip: %w", err)
		}
	}
	if err := win.EnsureUsable(); err != nil {
		return fmt.Errorf("gelm-hello: %w", err)
	}
	w, h := win.Size()
	if err := win.Decorate(sess.DecorationManager()); err != nil {
		log.Printf("gelm-hello: server decorations unavailable: %v", err)
	}
	log.Printf("gelm-hello: mapped at %dx%d", w, h)

	// Deterministic geometry for the headless input tests
	// (internal/headlesstest): lay the tree out at the mapped size and
	// trace each control's center, so the harness clicks real widgets
	// instead of hardcoded coordinates. app.Run re-arranges at the same
	// size on every draw, so this pre-arrange changes nothing.
	debug.Log("demo", "mapped %dx%d", w, h)
	show.root.Measure(widget.Constraints{Max: widget.Size{W: w, H: h}})
	show.root.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
	for _, c := range []struct {
		name string
		w    interface {
			Bounds() render.Rect
		}
	}{
		{"button", show.button},
		{"slider", show.slider},
		{"switch", show.sw},
		{"checkbox", show.check},
		{"entry", show.entry},
		{"textarea", show.area},
		{"scroll", show.scrolled},
	} {
		b := c.w.Bounds()
		debug.Log("demo", "control %s center (%d,%d)", c.name, b.X+b.W/2, b.Y+b.H/2)
	}

	sess.OnWmBasePing = win.Pong

	posX, posY := 0, 0
	var menuPopup *popup.Popup
	onPress := func(btn, serial uint32, over widget.Widget) {
		// A press landing on the window while the menu's grab is live is
		// an outside click: sway passes same-client presses through an
		// xdg_popup grab without a popup_done, so the dismissal is ours
		// to make. Closing the popup ends popup.Run and unwinds the grab.
		if menuPopup != nil && !menuPopup.Closed() {
			menuPopup.Close()
			menuPopup = nil
			return
		}
		switch btn {
		case widget.BTNLeft:
			// Presses on plain chrome move the window; presses on or
			// inside an interactive control belong to the widgets.
			// Hit tests return the deepest widget (a label inside the
			// button, a row inside the scroll), so walk the parents.
			if !widget.IsInteractive(over) {
				_ = win.Toplevel.Move(sess.Seat(), serial)
			}
		case widget.BTNRight:
			items := []widget.MenuItem{
				{Label: "Say hello", OnClick: show.bump},
				{Label: "Light theme", OnClick: func() {
					widget.SetTheme(widget.LightTheme())
					// Traced for the headless input tests, which assert
					// keyboard activation through the popup grab on it.
					debug.Log("demo", "theme light")
				}},
				{Label: "Dark theme", OnClick: func() {
					widget.SetTheme(widget.DarkTheme())
					debug.Log("demo", "theme dark")
				}},
				{},
				{Label: "Close window", OnClick: win.Close},
			}
			menu := widget.NewMenu(font, 13, items...)
			mSize := menu.Measure(widget.Constraints{Max: widget.Size{W: 200, H: 400}})
			debug.Log("demo", "menu open %dx%d at (%d,%d)", mSize.W, mSize.H, posX, posY)
			p, err := popup.New(sess, popup.Config{
				Parent: win.XdgSurface,
				X:      posX, Y: posY,
				Width: mSize.W, Height: mSize.H,
				Serial: serial,
			})
			if err != nil {
				log.Printf("gelm-hello: popup: %v", err)
				return
			}
			menu.OnDismiss = p.Close
			menuPopup = p
			_ = popup.Run(sess, p, scale.Denom, menu, widget.Current().Surface, menu)
			menuPopup = nil
		}
	}

	if err := app.Run(app.Config{
		Session:       sess,
		Host:          win,
		Scale:         1,
		Root:          show.root,
		Background:    widget.Current().Bg,
		OnPress:       onPress,
		OnPointerMove: func(x, y float64) { posX, posY = int(x), int(y) },
		TooltipFace:   tf,
		Clipboard:     clipboard.New(sess),
		// The showcase arms the debug inspector: ctrl+shift+i toggles
		// the widget-tree overlay, ctrl+shift+d dumps the tree to
		// stdout. GELM_INSPECT=1 does the same for any gelm app.
		Inspect: true,
		OnKey: func(_ *widget.Router, code uint32, _ wlsession.Mods) {
			debug.Log("demo", "app key code=%d sym=%v", code, sess.KeySym(code))
			if sess.KeySym(code) == xkb.KeyEscape {
				win.Close()
			}
		},
	}); err != nil {
		switch {
		case errors.Is(err, app.ErrClosed):
			// Normal close: a window ended the loop.
		case errors.Is(err, app.ErrDisconnected):
			// The compositor went away (restart, crash, reload). Exit
			// with the distinct disconnect code so a supervisor
			// (systemd Restart=on-failure, a wayle supervisor) respawns
			// us on the new session.
			os.Exit(app.DisconnectExitCode)
		default:
			return err
		}
	}
	return nil
}

// showcase bundles the widgets the input hooks need.
type showcase struct {
	root     widget.Widget
	bump     func()
	button   *widget.Button
	slider   *widget.Slider
	sw       *widget.Switch
	check    *widget.CheckButton
	entry    *widget.Entry
	area     *widget.TextArea
	scrolled *widget.Scroll
}

// buildUI assembles the full widget showcase: controls on the left,
// text and a scrollable list on the right, an event line below. Font is
// a fallback chain, so mixed-script demo text renders past .notdef.
func buildUI(font render.Font) showcase {
	t := widget.Current()
	status := widget.NewLabel(font, "events land here", 12, t.TextMuted)
	note := func(format string, args ...any) {
		status.SetText(fmt.Sprintf(format, args...))
		// Trace what the demo did, so the headless input tests can assert
		// on the showcase's own reactions (gelmdebug builds only).
		debug.Log("demo", format, args...)
	}

	count := 0
	countLabel := widget.NewLabel(font, "clicked 0 times", 15, t.Text)
	progress := widget.NewProgressBar(0)
	button := widget.NewButton(
		widget.NewBox(widget.Row, 8, 0).
			Append(widget.NewLabel(font, "click me", 15, t.Text), false),
		10, 8)
	bump := func() {
		count++
		countLabel.SetText(fmt.Sprintf("clicked %d times", count))
		note("button clicked %d times", count)
	}
	button.OnClick = func() {
		bump()
		progress.SetValue(0)
		anim.Start(600*time.Millisecond, func(v float64) {
			progress.SetValue(v)
		})
	}
	button.SetTooltip("increments the counter and animates the bar")
	button.SetDebugName("demo:increment")
	countLabel.SetTooltip("your click total")
	countLabel.SetDebugName("demo:count")

	slider := widget.NewSlider(0, 1, 0.05, 0)
	slider.OnChanged = func(v float64) {
		progress.SetValue(v)
		note("slider at %.0f%%", v*100)
	}
	slider.SetTooltip("drag, or Tab here and use the arrows")
	slider.SetDebugName("demo:slider")

	sw := widget.NewSwitch(true)
	sw.OnChanged = func(on bool) { note("switch %v", on) }
	sw.SetTooltip("toggles a boolean")
	swLabel := widget.NewLabel(font, "notifications", 14, t.Text)

	check := widget.NewCheckButton(false)
	check.OnChanged = func(c bool) { note("checkbox %v", c) }
	check.SetTooltip("checkbox state")
	checkLabel := widget.NewLabel(font, "remember me", 14, t.Text)

	entry := widget.NewEntry(font, 14, t.Text)
	entry.SetPlaceholder("type here; ctrl+c/x/v work")
	entry.SetTooltip("single-line entry; double-click selects a word")
	entry.SetDebugName("demo:entry")
	entry.OnChanged = func(s string) { note("entry: %q", s) }

	area := widget.NewTextArea(font, 13, t.Text)
	area.SetText("multi-line text area:\nenter splits, backspace joins,\nselection spans lines.")
	area.SetTooltip("multi-line editing")

	list := widget.NewBox(widget.Column, 4, 0)
	for i := 1; i <= 48; i++ {
		list.Append(widget.NewLabel(font,
			fmt.Sprintf("server-%02d.example   up   41ms", i), 12, t.Text), false)
	}
	scrolled := widget.NewScroll(list)
	scrolled.ShowBars = true
	scrolled.SetTooltip("scrollable list")
	scrolled.SetDebugName("demo:list")
	scrolled.OnScrolled = func(x, y int) {
		note("list scrolled to %d,%d", x, y)
	}

	header := widget.NewBox(widget.Column, 2, 0)
	header.Append(widget.NewLabel(font, "gelm showcase", 18, t.Accent), false)
	header.Append(widget.NewLabel(font, "every widget in one window; drag the chrome to move, esc closes", 11, t.TextMuted), false)
	header.Append(widget.NewLabel(font, "fallback check: 你好 world 😀 Привет", 13, t.Text), false)

	left := widget.NewBox(widget.Column, 10, 0)
	left.Append(button, false)
	left.Append(countLabel, false)
	left.Append(progress, false)
	left.Append(slider, false)
	left.Append(widget.NewBox(widget.Row, 8, 0).
		Append(swLabel, false).Append(sw, false), false)
	left.Append(widget.NewBox(widget.Row, 8, 0).
		Append(check, false).Append(checkLabel, false), false)

	right := widget.NewBox(widget.Column, 6, 0)
	right.Append(widget.NewLabel(font, "entry", 11, t.TextMuted), false)
	right.Append(entry, false)
	right.Append(widget.NewLabel(font, "text area", 11, t.TextMuted), false)
	right.Append(area, false)
	right.Append(widget.NewLabel(font, "list", 11, t.TextMuted), false)
	right.Append(scrolled, true)

	columns := widget.NewBox(widget.Row, 24, 0)
	columns.Append(left, true)
	columns.Append(right, true)

	root := widget.NewBox(widget.Column, 12, 16)
	root.Append(header, false)
	root.Append(columns, true)
	root.Append(status, false)

	return showcase{
		root: root, bump: bump,
		button: button, slider: slider, sw: sw, check: check,
		entry: entry, area: area, scrolled: scrolled,
	}
}
