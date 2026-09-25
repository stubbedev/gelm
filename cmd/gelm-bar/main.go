// Command gelm-bar is the M1 paint demo: a top bar anchored across the
// output showing a label, a clock, and a moving second indicator, all
// CPU-rasterized (rounded rects, gradient, shaped text) into pooled wl_shm
// ARGB8888 buffers and kept current with damage-tracked repaints driven by
// frame callbacks.
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
	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"

	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/layersurface"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// gelmLogoSVG is the demo module icon: a five-point star on a 24x24 grid.
const gelmLogoSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
  <path fill="#89b4fa" d="M12 2 L14.35 8.76 L21.51 8.91 L15.80 13.24 L17.88 20.09 L12 16 L6.12 20.09 L8.20 13.24 L2.49 8.91 L9.65 8.76 Z"/>
</svg>`

const (
	barHeight    = 32
	poolCapacity = 3
)

var (
	bgColor     = render.RGB(0x1E, 0x1E, 0x2E)
	accentColor = render.RGB(0x89, 0xB4, 0xFA)
	textColor   = render.RGB(0xCD, 0xD6, 0xF4)
	labelColor  = render.RGB(0xBA, 0xB6, 0xC8)
	pillColor   = render.RGB(0x11, 0x11, 0x1B)
)

func main() {
	dump := flag.String("dump", "", "render one bar frame to a PNG file instead of mapping on the compositor")
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

// dumpFrame renders a single bar frame offscreen and saves it as PNG, for
// visual checks without a compositor.
func dumpFrame(path string) error {
	fontData, err := findSansFont()
	if err != nil {
		return err
	}
	tf, err := render.LoadFont(fontData)
	if err != nil {
		return err
	}
	const (
		w, h, scale = 800, 32, 1
	)
	left := buildLeftModule(tf, scale)
	data := make([]byte, render.Stride(w)*h)
	cv := render.New(data, render.Stride(w), w, h)
	cv.Clear(cv.Rect(), bgColor)
	paintElements(cv, cv.Rect(), "09:41", 17, w, h, scale, tf, left)

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

// findSansFont locates a sans-serif system font without cgo, scanning the
// fonts fontconfig knows about (including nix store paths).
func findSansFont() ([]byte, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		cacheDir = os.TempDir()
	}
	fonts, err := fontscan.SystemFonts(quietLogger{}, filepath.Join(cacheDir, "gelm-fontscan"))
	if err != nil {
		return nil, fmt.Errorf("gelm-bar: scan fonts: %w", err)
	}
	pick := -1
	for i, f := range fonts {
		family := strings.ToLower(f.Family)
		if f.Location.File == "" || f.Aspect.Style != font.StyleNormal || f.Aspect.Weight != font.WeightNormal {
			continue
		}
		if strings.Contains(family, "sans") || strings.Contains(family, "dejavu") || strings.Contains(family, "noto") {
			pick = i
			break
		}
		if pick < 0 {
			pick = i
		}
	}
	if pick < 0 {
		return nil, errors.New("gelm-bar: no usable system font found")
	}
	return os.ReadFile(fonts[pick].Location.File)
}

// quietLogger discards fontscan warnings.
type quietLogger struct{}

// Printf implements fontscan.Logger.
func (quietLogger) Printf(string, ...any) {}

// buildLeftModule assembles the left bar module: a button holding the
// logo icon and the gelm label.
func buildLeftModule(tf *render.Typeface, scale int) *widget.Button {
	icon, err := render.LoadSVG([]byte(gelmLogoSVG), 16*scale, 16*scale)
	if err != nil {
		panic(err)
	}
	inner := widget.NewBox(widget.Row, 6*scale, 0)
	inner.Append(widget.NewIcon(icon), false)
	inner.Append(widget.NewLabel(tf, "gelm", float64(14*scale), labelColor), false)
	btn := widget.NewButton(inner, 4*scale, 6*scale)
	btn.Bg = pillColor
	btn.BgHover = render.RGB(0x18, 0x18, 0x25)
	btn.BgPressed = render.RGB(0x0c, 0x0c, 0x14)
	return btn
}

// layOutLeftModule measures and places the left module at the bar's left
// edge, vertically centered.
func layOutLeftModule(btn *widget.Button, bufW, bufH, scale int) widget.Size {
	sz := btn.Measure(widget.Constraints{Max: widget.Size{W: bufW, H: bufH}})
	btn.Arrange(render.Rect{X: 8 * scale, Y: (bufH - sz.H) / 2, W: sz.W, H: sz.H})
	return sz
}

func run() error {
	sess, err := wlsession.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()

	fontData, err := findSansFont()
	if err != nil {
		return err
	}
	typeface, err := render.LoadFont(fontData)
	if err != nil {
		return err
	}

	outputs := sess.Outputs()
	if len(outputs) == 0 {
		return errors.New("gelm-bar: no output to draw on")
	}
	out := outputs[0]

	surf, err := sess.Compositor().CreateSurface()
	if err != nil {
		return fmt.Errorf("gelm-bar: create surface: %w", err)
	}
	if err := surf.SetBufferScale(int32(out.Scale)); err != nil {
		return fmt.Errorf("gelm-bar: set buffer scale: %w", err)
	}

	ls, err := layersurface.New(sess.LayerShell(), surf, out.WL, layersurface.Config{
		Layer:         layersurface.LayerTop,
		Anchor:        layersurface.AnchorTop | layersurface.AnchorLeft | layersurface.AnchorRight,
		Height:        barHeight,
		ExclusiveZone: barHeight,
		Keyboard:      layersurface.KeyboardNone,
		Namespace:     "gelm-bar",
	})
	if err != nil {
		return err
	}

	create := func() (*buffer.Buffer, error) {
		w, h := ls.Size()
		return buffer.NewFile(sess.Shm(), w*out.Scale, h, out.Scale)
	}
	pool := buffer.New(create, poolCapacity)

	if err := surf.Commit(); err != nil {
		return fmt.Errorf("gelm-bar: initial commit: %w", err)
	}
	if err := sess.Roundtrip(); err != nil {
		return fmt.Errorf("gelm-bar: configure roundtrip: %w", err)
	}
	if err := ls.EnsureUsable(); err != nil {
		return fmt.Errorf("gelm-bar: %w", err)
	}
	w, h := ls.Size()
	log.Printf("gelm-bar: mapped at %dx%d, scale %d", w, h, out.Scale)

	var frameReady bool
	leftBtn := buildLeftModule(typeface, out.Scale)
	lastSecond := -1
	lastClock := ""
	lastBufW, lastBufH, lastScale := 0, 0, out.Scale
	full := true

	for !ls.Closed() {
		if out.Scale != lastScale {
			if err := surf.SetBufferScale(int32(out.Scale)); err != nil {
				return fmt.Errorf("gelm-bar: set buffer scale: %w", err)
			}
			lastScale = out.Scale
			leftBtn = buildLeftModule(typeface, out.Scale)
			full = true
		}
		bufW, bufH := w*out.Scale, h*out.Scale
		if bufW != lastBufW || bufH != lastBufH {
			pool.Resize(create)
			lastBufW, lastBufH = bufW, bufH
			full = true
		}

		now := time.Now()
		second := now.Second()
		clock := now.Format("15:04")
		if !full && second == lastSecond && clock == lastClock {
			next := now.Truncate(time.Second).Add(time.Second)
			time.Sleep(time.Until(next))
			continue
		}

		dirty := dirtyRects(full, lastClock, lastSecond, clock, second, bufW, bufH, out.Scale, typeface)
		lastSecond = second
		lastClock = clock
		full = false

		b, err := pool.Acquire()
		if errors.Is(err, buffer.ErrBusy) {
			if err := sess.Roundtrip(); err != nil {
				return fmt.Errorf("gelm-bar: dispatch while busy: %w", err)
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("gelm-bar: acquire buffer: %w", err)
		}
		wlclient.BufferAddListener(b.WL, buffer.ReleaseHandler{B: b})

		cv := render.New(b.Data, b.Stride, b.Width, b.Height)
		for _, r := range dirty {
			cv.Clear(r, bgColor)
			paintElements(cv, r, clock, second, bufW, bufH, out.Scale, typeface, leftBtn)
		}

		if err := surf.Attach(b.WL, 0, 0); err != nil {
			return fmt.Errorf("gelm-bar: attach: %w", err)
		}
		for _, r := range dirty {
			if err := surf.DamageBuffer(int32(r.X), int32(r.Y), int32(r.W), int32(r.H)); err != nil {
				return fmt.Errorf("gelm-bar: damage: %w", err)
			}
		}
		if err := surf.Commit(); err != nil {
			return fmt.Errorf("gelm-bar: commit: %w", err)
		}

		cb, err := surf.Frame()
		if err != nil {
			return fmt.Errorf("gelm-bar: frame callback: %w", err)
		}
		frameReady = false
		wlclient.CallbackAddListener(cb, frameDone{ready: &frameReady})

		for !frameReady && !ls.Closed() {
			if err := sess.Roundtrip(); err != nil {
				return fmt.Errorf("gelm-bar: frame dispatch: %w", err)
			}
		}
	}
	return nil
}

// frameDone flips ready when the compositor reports the frame as taken.
type frameDone struct {
	ready *bool
}

// HandleCallbackDone implements wl.CallbackDoneHandler.
func (f frameDone) HandleCallbackDone(wl.CallbackDoneEvent) {
	*f.ready = true
}

// clockRect is the pill region on the right holding the clock text.
func clockRect(clock string, bufW, bufH, scale int, tf *render.Typeface) render.Rect {
	clockText := tf.Shape(clock, float64(14*scale))
	w := int(clockText.Advance()) + 12*scale
	return render.Rect{X: bufW - w - 8*scale, Y: 0, W: w, H: bufH}
}

// notchRect is the moving second indicator, in buffer pixels. It clamps to
// the buffer, so a too-small bar yields an empty rect and nothing repaints.
func notchRect(second, bufW, bufH, scale int) render.Rect {
	margin := 8 * scale
	notchW := 6 * scale
	pad := 6 * scale
	x := margin + second*(bufW-2*margin-notchW)/59
	r := render.Rect{X: x, Y: pad, W: notchW, H: bufH - 2*pad}
	return r.Intersect(render.Rect{X: 0, Y: 0, W: bufW, H: bufH})
}

// dirtyRects returns the regions to repaint: everything on the first or
// resized frame, otherwise the union of the old and new clock and notch
// regions.
func dirtyRects(full bool, lastClock string, lastSecond int, clock string, second, bufW, bufH, scale int, tf *render.Typeface) []render.Rect {
	if full {
		return []render.Rect{{X: 0, Y: 0, W: bufW, H: bufH}}
	}
	old := render.UnionAll([]render.Rect{clockRect(lastClock, bufW, bufH, scale, tf), notchRect(lastSecond, bufW, bufH, scale)})
	new := render.UnionAll([]render.Rect{clockRect(clock, bufW, bufH, scale, tf), notchRect(second, bufW, bufH, scale)})
	return append(old.Subtract(new), new.Subtract(old)...)
}

// paintElements draws the left module, clock pill, and notch, confined to
// r. The left module only changes on resize, so its widget tree is laid
// out here for every call that could paint it.
func paintElements(cv *render.Canvas, r render.Rect, clock string, second, bufW, bufH, scale int, tf *render.Typeface, left *widget.Button) {
	prev := cv.PushClip(r)
	defer cv.PopClip(prev)

	layOutLeftModule(left, bufW, bufH, scale)
	if !left.Bounds().Intersect(r).Empty() {
		left.Paint(cv)
	}

	textPx := float64(14 * scale)
	pill := clockRect(clock, bufW, bufH, scale, tf)
	cv.RoundedRect(pill, 6*scale, pillColor)
	tf.DrawAligned(cv, clock, pill, textPx, textColor, render.AlignCenter)

	cv.LinearGradient(notchRect(second, bufW, bufH, scale), accentColor, pillColor, false)
}
