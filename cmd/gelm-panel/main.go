// Command gelm-panel is the M2+M4 interactive demo: a right-anchored panel
// with a slider (drag), progress bar, switch, checkbox, text entry, and a
// scrollable list, all live through the pointer and keyboard input stack.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/fontscan"

	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/layersurface"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

const (
	panelWidth   = 260
	poolCapacity = 3
)

var (
	bgColor   = render.RGB(0x1E, 0x1E, 0x2E)
	accent    = render.RGB(0x89, 0xB4, 0xFA)
	textColor = render.RGB(0xCD, 0xD6, 0xF4)
	muted     = render.RGB(0xA6, 0xAD, 0xC3)
)

func main() {
	dump := flag.String("dump", "", "render one panel frame to a PNG file instead of mapping on the compositor")
	flag.Parse()
	var err error
	if *dump != "" {
		err = dumpFrame(*dump)
	} else {
		err = run()
	}
	if err != nil {
		log.Fatal(err)
	}
}

// dumpFrame renders a single panel frame offscreen and saves it as PNG.
func dumpFrame(path string) error {
	tf, err := loadFont()
	if err != nil {
		return err
	}
	const dumpH = 480
	root := buildPanel(tf)
	data := make([]byte, render.Stride(panelWidth)*dumpH)
	cv := render.New(data, render.Stride(panelWidth), panelWidth, dumpH)
	cv.Clear(cv.Rect(), bgColor)
	root.Measure(widget.Constraints{Max: widget.Size{W: panelWidth, H: dumpH}})
	root.Arrange(render.Rect{X: 0, Y: 0, W: panelWidth, H: dumpH})
	root.Paint(cv)
	return writePNG(data, panelWidth, dumpH, path)
}

// writePNG converts a premultiplied ARGB8888 buffer to a PNG file.
func writePNG(data []byte, w, h int, path string) error {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := render.ColorFromBytes(data[y*render.Stride(w)+x*4 : y*render.Stride(w)+x*4+4])
			straight := c.Straight()
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = straight[0], straight[1], straight[2], straight[3]
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o600)
}

// loadFont finds and parses the system UI font.
func loadFont() (*render.Typeface, error) {
	fontData, err := findSansFont()
	if err != nil {
		return nil, err
	}
	return render.LoadFont(fontData)
}

// findSansFont locates a sans-serif system font without cgo.
func findSansFont() ([]byte, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	fonts, err := fontscan.SystemFonts(quietLogger{}, filepath.Join(cacheDir, "gelm-fontscan"))
	if err != nil {
		return nil, fmt.Errorf("gelm-panel: scan fonts: %w", err)
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
		return nil, errors.New("gelm-panel: no usable system font found")
	}
	return os.ReadFile(fonts[pick].Location.File)
}

// quietLogger discards fontscan warnings.
type quietLogger struct{}

// Printf implements fontscan.Logger.
func (quietLogger) Printf(string, ...any) {}

// buildPanel assembles the panel widget tree with all the interactive
// wiring.
func buildPanel(tf *render.Typeface) *widget.Box {
	progress := widget.NewProgressBar(0.5)
	status := widget.NewLabel(tf, "50%", 12, muted)
	entry := widget.NewEntry(tf, 13, textColor)

	slider := widget.NewSlider(0, 100, 1, 50)
	slider.OnChanged = func(v float64) {
		progress.SetValue(v / 100)
		status.SetText(fmt.Sprintf("%d%%", int(v)))
	}

	list := widget.NewBox(widget.Column, 2, 4)
	for i := range 14 {
		list.Append(widget.NewLabel(tf, fmt.Sprintf("server-%02d.example", i+1), 13, textColor), false)
	}
	scroll := widget.NewScroll(list)
	scroll.ShowBars = true

	notif := widget.NewCheckButton(true)
	night := widget.NewSwitch(false)

	row := func(kids ...widget.Widget) widget.Widget {
		b := widget.NewBox(widget.Row, 8, 0)
		for _, k := range kids {
			b.Append(k, false)
		}
		return b
	}
	root := widget.NewBox(widget.Column, 12, 12)
	root.Append(widget.NewLabel(tf, "gelm panel", 17, accent), false)
	root.Append(widget.NewLabel(tf, "Brightness", 12, muted), false)
	root.Append(slider, false)
	root.Append(progress, false)
	root.Append(status, false)
	root.Append(widget.NewLabel(tf, "Preferences", 12, muted), false)
	root.Append(row(notif, widget.NewLabel(tf, "Enable notifications", 13, textColor)), false)
	root.Append(row(night, widget.NewLabel(tf, "Night light", 13, textColor)), false)
	root.Append(widget.NewLabel(tf, "Notes", 12, muted), false)
	root.Append(entry, false)
	root.Append(widget.NewLabel(tf, "Servers (scroll me)", 12, muted), false)
	root.Append(scroll, true)
	entry.SetPlaceholder("type here")
	return root
}

func run() error {
	sess, err := wlsession.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()

	tf, err := loadFont()
	if err != nil {
		return err
	}

	outputs := sess.Outputs()
	if len(outputs) == 0 {
		return errors.New("gelm-panel: no output to draw on")
	}
	out := outputs[0]

	surf, err := sess.Compositor().CreateSurface()
	if err != nil {
		return fmt.Errorf("gelm-panel: create surface: %w", err)
	}
	if err := surf.SetBufferScale(int32(out.Scale)); err != nil {
		return fmt.Errorf("gelm-panel: set buffer scale: %w", err)
	}

	ls, err := layersurface.New(sess.LayerShell(), surf, out.WL, layersurface.Config{
		Layer:         layersurface.LayerTop,
		Anchor:        layersurface.AnchorTop | layersurface.AnchorBottom | layersurface.AnchorRight,
		Width:         panelWidth,
		ExclusiveZone: panelWidth,
		Keyboard:      layersurface.KeyboardOnDemand,
		Namespace:     "gelm-panel",
	})
	if err != nil {
		return err
	}

	create := func() (*buffer.Buffer, error) {
		w, h := ls.Size()
		return buffer.NewFile(sess.Shm(), w*out.Scale, h, out.Scale)
	}
	pool := buffer.New(create, poolCapacity)

	root := buildPanel(tf)

	if err := surf.Commit(); err != nil {
		return fmt.Errorf("gelm-panel: initial commit: %w", err)
	}
	if err := sess.Roundtrip(); err != nil {
		return fmt.Errorf("gelm-panel: configure roundtrip: %w", err)
	}
	if err := ls.EnsureUsable(); err != nil {
		return fmt.Errorf("gelm-panel: %w", err)
	}
	w, h := ls.Size()
	log.Printf("gelm-panel: mapped at %dx%d, scale %d", w, h, out.Scale)

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
		bufW, bufH := w*out.Scale, h*out.Scale
		root.Measure(widget.Constraints{Max: widget.Size{W: bufW, H: bufH}})
		root.Arrange(render.Rect{X: 0, Y: 0, W: bufW, H: bufH})
	}

	sess.OnPointerMove = func(x, y float64) {
		pointer.x, pointer.y = x, y
		router.Move(widget.Point{X: int(x) * out.Scale, Y: int(y) * out.Scale})
		requestRedraw()
	}
	sess.OnPointerButton = func(button, state, _ uint32) {
		p := widget.Point{X: int(pointer.x) * out.Scale, Y: int(pointer.y) * out.Scale}
		if state == 1 {
			router.Press(button, p)
		} else {
			router.Release(button, p)
		}
		requestRedraw()
	}
	sess.OnPointerAxis = func(dy float64) {
		steps := int(dy / 10)
		if dy != 0 && steps == 0 {
			steps = 1
			if dy < 0 {
				steps = -1
			}
		}
		router.Axis(float64(steps))
		requestRedraw()
	}
	sess.OnKey = func(keycode uint32, shift bool) {
		ch, action, ok := mapKey(keycode, shift)
		switch {
		case ok && ch != 0:
			router.Type(ch)
		case ok:
			router.KeyAction(action)
		}
		requestRedraw()
	}

	draw := func(b *buffer.Buffer) {
		cv := render.New(b.Data, b.Stride, b.Width, b.Height)
		cv.Clear(cv.Rect(), bgColor)
		layOut()
		root.Paint(cv)
	}

	for !ls.Closed() {
		b, err := pool.Acquire()
		if errors.Is(err, buffer.ErrBusy) {
			if err := sess.Roundtrip(); err != nil {
				return fmt.Errorf("gelm-panel: dispatch while busy: %w", err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("gelm-panel: acquire buffer: %w", err)
		}
		wlclientBufferListener(b)

		draw(b)

		if err := surf.Attach(b.WL, 0, 0); err != nil {
			return fmt.Errorf("gelm-panel: attach: %w", err)
		}
		if err := surf.DamageBuffer(0, 0, int32(b.Width), int32(b.Height)); err != nil {
			return fmt.Errorf("gelm-panel: damage: %w", err)
		}
		if err := surf.Commit(); err != nil {
			return fmt.Errorf("gelm-panel: commit: %w", err)
		}

		cb, err := surf.Frame()
		if err != nil {
			return fmt.Errorf("gelm-panel: frame callback: %w", err)
		}
		frameReady := false
		wlclientCallbackListener(cb, &frameReady)
		for !frameReady && !ls.Closed() {
			if err := sess.Roundtrip(); err != nil {
				return fmt.Errorf("gelm-panel: frame dispatch: %w", err)
			}
		}

		if !waitRedraw(sess, redraw, ls) {
			break
		}
	}
	return nil
}

// waitRedraw blocks until another frame is requested (polling the
// connection while it waits) and reports whether the loop should continue.
func waitRedraw(sess *wlsession.Session, redraw chan struct{}, ls *layersurface.Surface) bool {
	deadline := time.Now().Add(50 * time.Millisecond)
	for time.Now().Before(deadline) && !ls.Closed() {
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
	return !ls.Closed()
}
