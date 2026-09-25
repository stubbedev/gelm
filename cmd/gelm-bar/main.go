// Command gelm-bar is the M0 scaffold demo: a top bar anchored across the
// output, CPU-rasterized into pooled wl_shm ARGB8888 buffers and kept
// current with damage-tracked repaints driven by frame callbacks.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"
	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/layersurface"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
)

const (
	barHeight    = 32
	poolCapacity = 3
)

var (
	bgColor     = uint32(0xFF1E1E2E)
	accentColor = uint32(0xFF89B4FA)
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
	lastSecond := -1
	lastBufW, lastBufH, lastScale := 0, 0, out.Scale
	full := true

	for !ls.Closed() {
		if out.Scale != lastScale {
			if err := surf.SetBufferScale(int32(out.Scale)); err != nil {
				return fmt.Errorf("gelm-bar: set buffer scale: %w", err)
			}
			lastScale = out.Scale
			full = true
		}
		bufW, bufH := w*out.Scale, h*out.Scale
		if bufW != lastBufW || bufH != lastBufH {
			pool.Resize(create)
			lastBufW, lastBufH = bufW, bufH
			full = true
		}

		second := time.Now().Second()
		if !full && second == lastSecond {
			next := time.Now().Truncate(time.Second).Add(time.Second)
			time.Sleep(time.Until(next))
			continue
		}

		dirty := dirtyRects(full, lastSecond, second, bufW, bufH, out.Scale)
		lastSecond = second
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

		active := notchRect(second, bufW, bufH, out.Scale)
		paint(b.Data, b.Stride, dirty, active)

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

// dirtyRects returns the regions to repaint: everything on the first or
// resized frame, otherwise just the old and new notch.
func dirtyRects(full bool, lastSecond, second, bufW, bufH, scale int) []render.Rect {
	if full {
		return []render.Rect{{X: 0, Y: 0, W: bufW, H: bufH}}
	}
	old := notchRect(lastSecond, bufW, bufH, scale)
	new := notchRect(second, bufW, bufH, scale)
	return append(old.Subtract(new), new.Subtract(old)...)
}

// notchRect is the moving second indicator, in buffer pixels. It clamps to
// the buffer, so a too-small bar yields an empty rect and nothing repaints.
func notchRect(second, bufW, bufH, scale int) render.Rect {
	margin := 8 * scale
	notchW := 4 * scale
	pad := 8 * scale
	x := margin + second*(bufW-2*margin-notchW)/59
	r := render.Rect{X: x, Y: pad, W: notchW, H: bufH - 2*pad}
	return r.Intersect(render.Rect{X: 0, Y: 0, W: bufW, H: bufH})
}

// paint fills every dirty rect with the background, overlaying the active
// notch. Data rows are ARGB8888 premultiplied, byte order B, G, R, A.
func paint(data []byte, stride int, dirty []render.Rect, active render.Rect) {
	for _, dr := range dirty {
		for y := dr.Y; y < dr.Y+dr.H; y++ {
			row := data[y*stride:]
			for x := dr.X; x < dr.X+dr.W; x++ {
				c := bgColor
				if active.Contains(x, y) {
					c = accentColor
				}
				binary.LittleEndian.PutUint32(row[x*4:x*4+4], c)
			}
		}
	}
}
