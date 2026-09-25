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

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/internal/clipboard"
	"github.com/stubbedev/gelm/internal/layersurface"
	"github.com/stubbedev/gelm/internal/sysfont"
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

// loadFont resolves the system sans-serif face.
func loadFont() (*render.Typeface, error) {
	return sysfont.Sans()
}

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
	root.Append(widget.NewLabel(tf, "Quick note", 12, muted), false)
	root.Append(entry, false)
	root.Append(widget.NewLabel(tf, "Notes", 12, muted), false)
	notes := widget.NewTextArea(tf, 13, textColor)
	notes.SetPlaceholder("multi-line...")
	root.Append(notes, false)
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

	return app.Run(app.Config{
		Session:    sess,
		Host:       ls,
		Scale:      out.Scale,
		Root:       root,
		Background: bgColor,
		Clipboard:  clipboard.New(sess),
	})
}
