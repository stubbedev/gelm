// Drag and drop glue: the press+motion gesture that turns a widget's
// declared content into a wl_data_device start_drag, the routing of
// data-device events into the widget tree, and the drag icon snapshot.
// The wire-level state machine lives in internal/dragdrop.
package app

import (
	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/dragdrop"
	"github.com/stubbedev/gelm/internal/scale"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/render"
	"github.com/stubbedev/gelm/widget"
)

// dragThreshold is the pointer travel, in logical pixels, before a
// press becomes a drag — a click never is. Close to GTK's threshold.
const dragThreshold = 8.0

// maxDragIcon bounds the drag icon snapshot; a source widget larger
// than this is cropped in the icon.
const maxDragIcon = 512

// dragController is the slice of the drag-and-drop controller the
// surface input drives. *dragdrop.Controller satisfies it; tests
// substitute a recording fake.
type dragController interface {
	StartDrag(cfg dragdrop.StartConfig) error
	Dragging() bool
	ReadPayload(mime string) ([]byte, error)
}

// startDrag promotes the active press into a data-device drag when it
// crossed the threshold over a widget.DragSource. Called from the
// pointer-motion path while the implicit grab is live.
func (in *surfaceInput) startDrag() {
	if in.dragStarted || in.dnd == nil || in.dnd.Dragging() {
		return
	}
	src, ok := in.router.Pressed().(widget.DragSource)
	if !ok {
		return
	}
	dx, dy := in.x-in.pressX, in.y-in.pressY
	if dx*dx+dy*dy < dragThreshold*dragThreshold {
		return
	}
	content := src.DragContent()
	if content == nil {
		return
	}
	err := in.dnd.StartDrag(dragdrop.StartConfig{
		Origin:     in.surf,
		Icon:       renderDragIcon(in.sess, in.router.Pressed(), in.deviceScale()),
		GrabSerial: in.pressSerial,
		Content: dragdrop.Content{
			Mimes:  content.Mimes,
			Write:  content.Write,
			OnDone: content.OnDone,
		},
	})
	if err != nil {
		debug.Log("input", "dnd start: %v", err)
		return
	}
	in.dragStarted = true
	// The gesture became a drag: the release must not click, and the
	// pressed widget must not keep following the pointer as a DragMover.
	in.router.CancelPress()
	in.request()
}

// renderDragIcon paints w standalone into a fresh shm-backed surface
// the compositor moves with the pointer during the drag. frac120 is the
// source window's current 120-based device scale; the icon surface
// scales to match. Any failure leaves the icon nil: start_drag takes a
// nil icon and shows a fallback graphic.
func renderDragIcon(sess *wlsession.Session, w widget.Widget, frac120 uint32) *wl.Surface {
	if sess == nil || sess.Compositor() == nil || sess.Shm() == nil || w == nil {
		return nil
	}
	size := w.Measure(widget.Constraints{Max: widget.Size{W: maxDragIcon, H: maxDragIcon}})
	if size.W <= 0 || size.H <= 0 {
		return nil
	}
	b, err := buffer.NewFile(sess.Shm(),
		scale.DeviceSize(size.W, frac120), scale.DeviceSize(size.H, frac120),
		scale.IntegerScale(frac120))
	if err != nil {
		debug.Log("frame", "dnd icon buffer: %v", err)
		return nil
	}
	w.Arrange(render.Rect{X: 0, Y: 0, W: size.W, H: size.H})
	cv := render.NewScaled(b.Data, b.Stride, b.Width, b.Height, int(frac120), scale.Denom)
	w.Paint(cv)
	surf, err := sess.Compositor().CreateSurface()
	if err != nil {
		debug.Log("frame", "dnd icon surface: %v", err)
		return nil
	}
	sc := scale.New(sess, surf, nil)
	_ = sc.Apply(frac120, size.W, size.H)
	attach := func(err error, what string) bool {
		if err != nil {
			debug.Log("frame", "dnd icon %s: %v", what, err)
			_ = surf.Destroy()
			return false
		}
		return true
	}
	if !attach(surf.Attach(b.WL, 0, 0), "attach") ||
		!attach(surf.DamageBuffer(0, 0, int32(b.Width), int32(b.Height)), "damage") ||
		!attach(surf.Commit(), "commit") {
		return nil
	}
	return surf
}

// DragEnter implements dragdrop.Target: route through the widget tree
// and report the accepted mime. A modal dialog over this window owns
// the pointer, so its drags are rejected too.
func (in *surfaceInput) DragEnter(mimes []string, x, y float64) string {
	if in.dropInput() {
		return ""
	}
	mime := in.router.DragEnter(mimes, in.dropPoint(x, y))
	in.request()
	return mime
}

// DragMotion implements dragdrop.Target.
func (in *surfaceInput) DragMotion(x, y float64) string {
	if in.dropInput() {
		return ""
	}
	mime := in.router.DragHover(in.dropPoint(x, y))
	in.request()
	return mime
}

// DragLeave implements dragdrop.Target.
func (in *surfaceInput) DragLeave() {
	in.router.DragLeave()
	in.request()
}

// Drop implements dragdrop.Target: fetch the payload for the accepted
// mime and hand it to the widget. Same-process drops resolve inside
// ReadPayload without touching the wire; cross-process ones go through
// the offer pipe.
func (in *surfaceInput) Drop(x, y float64) {
	if in.dropInput() {
		return
	}
	mime := in.router.DragMime()
	if mime == "" {
		return
	}
	data, err := in.dnd.ReadPayload(mime)
	if err != nil {
		debug.Log("input", "dnd drop: %v", err)
		return
	}
	in.router.Drop(mime, data, in.dropPoint(x, y))
	in.request()
}

// deviceScale reports the host window's 120-based device scale for
// one-shot surfaces; 1x when unset (wire-free tests).
func (in *surfaceInput) deviceScale() uint32 {
	if in.frac == nil {
		return scale.Denom
	}
	if f := in.frac(); f != 0 {
		return f
	}
	return scale.Denom
}

// dropPoint maps surface coordinates into router (root) coordinates,
// the same mapping the pointer path applies: both are logical pixels,
// so it truncates to the tree's integer grid.
func (in *surfaceInput) dropPoint(x, y float64) widget.Point {
	return widget.Point{X: int(x), Y: int(y)}
}
