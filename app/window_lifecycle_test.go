// Window lifecycle coverage for the buffer pool: closing a window must
// hand its pool's storage back to the session arena - the teardown that
// used to be missing entirely, leaking every mapped buffer on close.
// Drives the real draw pipeline over fake buffers, no compositor.
package app

import (
	"testing"

	"github.com/stubbedev/gelm/widget"
)

func TestWindowCloseReturnsPool(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	root.Append(widget.NewProgressBar(0), false)
	h := newPaintHarness(root, 320, 200)

	// A couple of frames so the rotation holds real, partly busy state.
	h.frame()
	h.frame()

	// The compositor returned every buffer; then the window closes.
	for _, b := range h.bufs {
		b.Release()
	}
	if h.wnd.pool == nil {
		t.Fatal("harness window has no pool")
	}
	h.wnd.release()
	if got := h.wnd.pool.Pending(); got != 0 {
		t.Errorf("window closed with %d pending buffers: pool storage was not returned", got)
	}
}

func TestWindowCloseWithHeldBuffersDefersThem(t *testing.T) {
	root := widget.NewBox(widget.Row, 0, 0)
	h := newPaintHarness(root, 320, 200)
	h.frame()
	h.wnd.dirty = true
	if !h.wnd.draw() { // one buffer is now handed out (busy)
		t.Fatal(h.wnd.drawErr)
	}

	h.wnd.release()
	if got := h.wnd.pool.Pending(); got != 1 {
		t.Fatalf("pending = %d, want the one buffer the compositor still holds", got)
	}
	for _, b := range h.bufs {
		b.Release() // the release lands after the window is gone
	}
	if got := h.wnd.pool.Pending(); got != 0 {
		t.Errorf("pending = %d after post-close release, want 0", got)
	}
}
