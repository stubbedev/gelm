// Command gelm-hello is the M5 windowing demo: a real xdg_toplevel
// window with a counter button, drag-to-move on the background, and
// Escape to close — proving the window stack end to end.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/fontscan"
	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"

	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/window"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

const (
	winWidth  = 360
	winHeight = 240
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// loadFont finds and parses the system UI font.
func loadFont() (*render.Typeface, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	fonts, err := fontscan.SystemFonts(quietLogger{}, filepath.Join(cacheDir, "gelm-fontscan"))
	if err != nil {
		return nil, fmt.Errorf("gelm-hello: scan fonts: %w", err)
	}
	pick := -1
	for i, f := range fonts {
		if f.Location.File == "" || f.Aspect.Style != font.StyleNormal || f.Aspect.Weight != font.WeightNormal {
			continue
		}
		family := strings.ToLower(f.Family)
		if strings.Contains(family, "sans") || strings.Contains(family, "dejavu") || strings.Contains(family, "noto") {
			pick = i
			break
		}
		if pick < 0 {
			pick = i
		}
	}
	if pick < 0 {
		return nil, errors.New("gelm-hello: no usable system font found")
	}
	data, err := os.ReadFile(fonts[pick].Location.File)
	if err != nil {
		return nil, err
	}
	return render.LoadFont(data)
}

// quietLogger discards fontscan warnings.
type quietLogger struct{}

// Printf implements fontscan.Logger.
func (quietLogger) Printf(string, ...any) {}

func run() error {
	sess, err := wlsession.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	if sess.WmBase() == nil {
		return errors.New("gelm-hello: compositor has no xdg_wm_base; windows unsupported")
	}
	tf, err := loadFont()
	if err != nil {
		return err
	}

	surf, err := sess.Compositor().CreateSurface()
	if err != nil {
		return fmt.Errorf("gelm-hello: create surface: %w", err)
	}
	win, err := window.New(sess.WmBase(), surf, window.Config{
		Title:  "gelm hello",
		AppID:  "dev.stubbe.gelm.hello",
		Width:  winWidth,
		Height: winHeight,
	})
	if err != nil {
		return err
	}

	create := func() (*buffer.Buffer, error) {
		w, h := win.Size()
		return buffer.NewFile(sess.Shm(), w, h, 1)
	}
	pool := buffer.New(create, 3)

	// The widget tree: centered column with a counter button.
	count := 0
	countLabel := widget.NewLabel(tf, "clicked 0 times", 15, widget.Current().Text)
	button := widget.NewButton(
		widget.NewBox(widget.Row, 8, 0).
			Append(widget.NewLabel(tf, "click me", 15, widget.Current().Text), false),
		10, 8)
	button.OnClick = func() {
		count++
		countLabel.SetText(fmt.Sprintf("clicked %d times", count))
	}
	root := widget.NewBox(widget.Column, 12, 16)
	root.Append(widget.NewLabel(tf, "gelm window", 18, widget.Current().Accent), false)
	root.Append(button, false)
	root.Append(countLabel, false)
	root.Append(widget.NewLabel(tf, "drag to move, esc to close", 11, widget.Current().TextMuted), false)

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
	log.Printf("gelm-hello: mapped at %dx%d", w, h)

	sess.OnWmBasePing = win.Pong

	router := &widget.Router{Root: root}
	var pointer struct{ x, y float64 }
	redraw := make(chan struct{}, 1)
	requestRedraw := func() {
		select {
		case redraw <- struct{}{}:
		default:
		}
	}

	layOut := func() {
		root.Measure(widget.Constraints{Max: widget.Size{W: w, H: h}})
		root.Arrange(render.Rect{X: 0, Y: 0, W: w, H: h})
	}

	pressed := false
	pressSerial := uint32(0)
	sess.OnPointerMove = func(x, y float64) {
		pointer.x, pointer.y = x, y
		router.Move(widget.Point{X: int(x), Y: int(y)})
		requestRedraw()
	}
	sess.OnPointerButton = func(button_, state, serial uint32) {
		p := widget.Point{X: int(pointer.x), Y: int(pointer.y)}
		if state == 1 {
			pressed = true
			pressSerial = serial
			// A press that is not on the button starts an
			// interactive move, handled entirely by the compositor.
			if router.Hovered() != button {
				_ = win.Toplevel.Move(sess.Seat(), serial)
			}
			router.Press(button_, p)
		} else {
			pressed = false
			router.Release(button_, p)
		}
		requestRedraw()
	}
	sess.OnKey = func(keycode uint32, _ bool) {
		if keycode == 1 { // KEY_ESC
			win.Close()
		}
	}

	draw := func(b *buffer.Buffer) {
		cv := render.New(b.Data, b.Stride, b.Width, b.Height)
		cv.Clear(cv.Rect(), widget.Current().Bg)
		layOut()
		root.Paint(cv)
	}

	for !win.Closed() {
		b, err := pool.Acquire()
		if errors.Is(err, buffer.ErrBusy) {
			if err := sess.Roundtrip(); err != nil {
				return fmt.Errorf("gelm-hello: dispatch while busy: %w", err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("gelm-hello: acquire buffer: %w", err)
		}
		wlclientBufferListener(b)

		draw(b)

		if err := surf.Attach(b.WL, 0, 0); err != nil {
			return fmt.Errorf("gelm-hello: attach: %w", err)
		}
		if err := surf.DamageBuffer(0, 0, int32(b.Width), int32(b.Height)); err != nil {
			return fmt.Errorf("gelm-hello: damage: %w", err)
		}
		if err := surf.Commit(); err != nil {
			return fmt.Errorf("gelm-hello: commit: %w", err)
		}

		cb, err := surf.Frame()
		if err != nil {
			return fmt.Errorf("gelm-hello: frame callback: %w", err)
		}
		frameReady := false
		wlclientCallbackListener(cb, &frameReady)
		for !frameReady && !win.Closed() {
			if err := sess.Roundtrip(); err != nil {
				return fmt.Errorf("gelm-hello: frame dispatch: %w", err)
			}
		}
		if !waitInput(sess, redraw, win) {
			break
		}
	}
	_ = pressed
	_ = pressSerial
	return nil
}

// waitInput blocks until more input arrives (polling the connection while
// it waits) and reports whether the loop should continue.
func waitInput(sess *wlsession.Session, redraw chan struct{}, win *window.Window) bool {
	deadline := time.Now().Add(80 * time.Millisecond)
	for time.Now().Before(deadline) && !win.Closed() {
		select {
		case <-redraw:
			return true
		default:
		}
		if err := sess.Roundtrip(); err != nil {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
	return !win.Closed()
}

func wlclientBufferListener(b *buffer.Buffer) {
	wlclient.BufferAddListener(b.WL, buffer.ReleaseHandler{B: b})
}

func wlclientCallbackListener(cb *wl.Callback, ready *bool) {
	wlclient.CallbackAddListener(cb, frameDone{ready: ready})
}

type frameDone struct {
	ready *bool
}

// HandleCallbackDone implements wl.CallbackDoneHandler.
func (f frameDone) HandleCallbackDone(wl.CallbackDoneEvent) {
	*f.ready = true
}
