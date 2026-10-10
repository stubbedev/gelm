// Command gelm-hello is the showcase demo, built the way an app outside
// this module builds one: a component whose view is declared with the
// typed ui builders, hosted in one toplevel window. It exercises the
// core widgets - labels, button, slider, progress bar, switch,
// checkbox, entry, text area, a scrollable list - plus tooltips, an
// animated tween, a right-click context menu bound to typed actions,
// Tab focus traversal, drag-to-move on the chrome, and Escape to close.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"time"

	"github.com/stubbedev/gelm/anim"
	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/component"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/ui"
	"github.com/stubbedev/gelm/widget"
)

const width, height = 640, 470

func main() {
	err := run()
	switch {
	case err == nil, errors.Is(err, app.ErrClosed):
	case errors.Is(err, app.ErrDisconnected):
		os.Exit(app.DisconnectExitCode)
	default:
		log.Fatal(err)
	}
}

func run() error {
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	face, err := app.Font("sans-serif", 15)
	if err != nil {
		return err
	}

	application := app.NewApplication(sess)
	application.SetTooltipFace(face)
	application.SetClipboard(app.NewClipboard(sess))
	application.SetInspect(true)

	show := &showcase{env: ui.Env{Face: app.FontFallback(face), Size: 14}}
	theme := widget.NewStateAction("theme", "dark", func(name string) {
		if name == "light" {
			widget.SetTheme(widget.LightTheme())
		} else {
			widget.SetTheme(widget.DarkTheme())
		}
		app.Trace("demo", "theme %s", name)
	})

	var win *app.Window
	var ctrl *component.Controller[showMsg, struct{}]
	pointer := render.Rect{W: 1, H: 1}
	menu := func() []widget.MenuItem {
		return []widget.MenuItem{
			widget.ActionItem("Say hello", widget.NewAction("hello", func() { ctrl.Send(bump{}) })),
			widget.RadioItem("Light theme", theme.Target("light")),
			widget.RadioItem("Dark theme", theme.Target("dark")),
			widget.MenuSeparator(),
			widget.ActionItem("Close window", widget.NewAction("close", func() { win.Close() })),
		}
	}
	cfg := app.WindowConfig{
		Title: "gelm showcase", AppID: "dev.stubbe.gelm.hello",
		Width: width, Height: height,
		Background:    widget.Current().Bg,
		OnPointerMove: func(x, y float64) { pointer.X, pointer.Y = int(x), int(y) },
		OnResize:      func(w, h int) { show.mapped(w, h) },
		OnPress: func(button, serial uint32, over widget.Widget) {
			switch button {
			case widget.BTNLeft:
				if !widget.IsInteractive(over) {
					win.BeginMove()
				}
			case widget.BTNRight:
				app.Trace("demo", "menu open at (%d,%d)", pointer.X, pointer.Y)
				if _, err := application.OpenMenuPopover(win, app.MenuPopoverConfig{
					Anchor: anchor(pointer), Face: show.env.Face, SizePx: 13,
					Items: menu(), Serial: serial,
				}); err != nil {
					log.Printf("gelm-hello: menu: %v", err)
				}
			}
		},
		OnKey: func(_ *widget.Router, code uint32, _ app.Mods) {
			app.Trace("demo", "app key code=%d sym=%v", code, application.KeySym(code))
		},
	}
	ctrl, win, err = component.Window(application, cfg, show)
	if err != nil {
		return err
	}
	if err := application.AddScopedAccel(ctrl.Widget(), "Escape", widget.NewAction("close", win.Close)); err != nil {
		return err
	}
	if ms, err := strconv.Atoi(os.Getenv("GELM_DEMO_IDLE_MS")); err == nil && ms > 0 {
		idle := func() { app.Trace("demo", "idle") }
		resume := func() { app.Trace("demo", "resumed") }
		if _, err := application.OnIdle(time.Duration(ms)*time.Millisecond, idle, resume); err != nil {
			app.Trace("demo", "idle notify unavailable: %v", err)
		}
	}
	return application.Run()
}

type anchor render.Rect

func (a anchor) Bounds() render.Rect { return render.Rect(a) }

type showMsg interface{ showMsg() }

type bump struct{}

type noted struct{ text string }

func (bump) showMsg()  {}
func (noted) showMsg() {}

type showcase struct {
	env    ui.Env
	count  int
	status string
	root   widget.Widget
	shown  bool

	button   *widget.Button
	progress *widget.ProgressBar
	slider   *widget.Slider
	sw       *widget.Switch
	check    *widget.CheckButton
	entry    *widget.Entry
	area     *widget.TextArea
	scrolled *widget.Scroll
}

func (s *showcase) Init(cx *component.Context[showMsg, struct{}]) widget.Widget {
	s.status = "events land here"
	note := func(format string, args ...any) func() {
		return func() { cx.Input(noted{text: fmt.Sprintf(format, args...)}) }
	}
	muted := widget.Current().TextMuted
	caption := func(text string) ui.Node { return ui.Label(text).Font(nil, 11).Ink(muted) }

	servers := make([]string, 48)
	for i := range servers {
		servers[i] = fmt.Sprintf("server-%02d.example   up   41ms", i+1)
	}

	header := ui.Column(
		ui.Label("gelm showcase").Font(nil, 18).Ink(widget.Current().Accent),
		caption("every widget in one window; drag the chrome to move, esc closes"),
		ui.Label("fallback check: 你好 world 😀 Привет").Font(nil, 13),
	).Spacing(2)

	controls := ui.Column(
		ui.Button(ui.Row(ui.Label("click me").Font(nil, 15)).Spacing(8), 10, 8).Ref(&s.button).
			OnClick(func() { cx.Input(bump{}) }).
			Tooltip("increments the counter and animates the bar").DebugName("demo:increment"),
		ui.Label("").Font(nil, 15).WatchText(func() string { return fmt.Sprintf("clicked %d times", s.count) }).
			Tooltip("your click total").DebugName("demo:count"),
		ui.ProgressBar(0).Ref(&s.progress),
		ui.Slider(0, 1, 0.05, 0).Ref(&s.slider).
			OnChanged(func(v float64) {
				s.progress.SetValue(v)
				note("slider at %.0f%%", v*100)()
			}).
			Tooltip("drag, or Tab here and use the arrows").DebugName("demo:slider"),
		ui.Row(
			ui.Label("notifications"),
			ui.Switch(true).Ref(&s.sw).OnChanged(func(on bool) { note("switch %v", on)() }).Tooltip("toggles a boolean"),
		).Spacing(8),
		ui.Row(
			ui.CheckButton(false).Ref(&s.check).OnChanged(func(c bool) { note("checkbox %v", c)() }).Tooltip("checkbox state"),
			ui.Label("remember me"),
		).Spacing(8),
	).Spacing(10)

	texts := ui.Column(
		caption("entry"),
		ui.Entry().Ref(&s.entry).Placeholder("type here; ctrl+c/x/v work").
			OnChanged(func(text string) { note("entry: %q", text)() }).
			Tooltip("single-line entry; double-click selects a word").DebugName("demo:entry"),
		caption("text area"),
		ui.TextArea().Font(nil, 13).Ref(&s.area).
			Text("multi-line text area:\nenter splits, backspace joins,\nselection spans lines.").
			Tooltip("multi-line editing"),
		caption("list"),
		ui.Expand(ui.Scroll(ui.Column(ui.Each(slices.Values(servers), func(line string) ui.Node {
			return ui.Label(line).Font(nil, 12)
		})...).Spacing(4)).Ref(&s.scrolled).ShowBars(true).
			OnScrolled(func(x, y int) { note("list scrolled to %d,%d", x, y)() }).
			Tooltip("scrollable list").DebugName("demo:list")),
	).Spacing(6)

	s.root = ui.Mount(cx, s.env, ui.Column(
		header,
		ui.Expand(ui.Row(ui.Expand(controls), ui.Expand(texts)).Spacing(24)),
		ui.Label("").Font(nil, 12).Ink(muted).WatchText(func() string { return s.status }),
	).Spacing(12).Padding(render.UniformInsets(16)))
	return s.root
}

func (s *showcase) Update(_ *component.Context[showMsg, struct{}], msg showMsg) {
	switch m := msg.(type) {
	case bump:
		s.count++
		s.progress.SetValue(0)
		anim.Start(600*time.Millisecond, s.progress.SetValue)
		s.note(fmt.Sprintf("button clicked %d times", s.count))
	case noted:
		s.note(m.text)
	}
}

func (s *showcase) note(text string) {
	s.status = text
	app.Trace("demo", "%s", text)
}

type control struct {
	name string
	w    interface{ Bounds() render.Rect }
}

func (s *showcase) controls() []control {
	return []control{
		{"button", s.button},
		{"slider", s.slider},
		{"switch", s.sw},
		{"checkbox", s.check},
		{"entry", s.entry},
		{"textarea", s.area},
		{"scroll", s.scrolled},
	}
}

func (s *showcase) mapped(w, h int) {
	if s.shown {
		return
	}
	s.shown = true
	log.Printf("gelm-hello: mapped at %dx%d", w, h)
	app.Trace("demo", "mapped %dx%d", w, h)
	s.root.Measure(widget.Constraints{Max: widget.Size{W: w, H: h}})
	s.root.Arrange(render.Rect{W: w, H: h})
	for _, c := range s.controls() {
		b := c.w.Bounds()
		app.Trace("demo", "control %s center (%d,%d)", c.name, b.X+b.W/2, b.Y+b.H/2)
	}
}
