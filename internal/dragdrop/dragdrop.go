// Package dragdrop implements drag and drop over the wl_data_device
// protocol: the source side (data source + start_drag with an icon),
// the destination side (offer bookkeeping, accept/reject, payload
// transfer), and the routing of enter/motion/leave/drop to the target
// registered for the surface under the drag. It coexists with
// internal/clipboard, which handles the same device's selection
// events: offers are tracked per object, so a drag's offer never
// touches the selection.
package dragdrop

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/internal/xfer"
)

// minActionVersion is the wl_data_device_manager version that gained
// the dnd action requests and events (set_actions, finish,
// dnd_finished); below it drags run in the v1 subset.
const minActionVersion = 3

// ErrUnavailable reports that the session has no data device, so no
// drag can start.
var ErrUnavailable = errors.New("dragdrop: no data device")

// ErrDragActive reports a start_drag while one is already running.
var ErrDragActive = errors.New("dragdrop: a drag is already active")

// ErrNoPayload reports a payload read with no accepted drag behind it.
var ErrNoPayload = errors.New("dragdrop: no drag payload available")

// transferTimeout bounds one payload transfer against a peer that
// stalls (a var so tests can shorten it); the read is also capped by
// xfer.MaxPayload, so a hostile source can neither hang the drop nor
// stream unbounded bytes into memory.
var transferTimeout = xfer.DefaultTimeout

// Content is the payload a drag offers: mime types best first and a
// provider that writes the bytes for one mime on demand. OnDone fires
// once the drag concluded — dropped and handed off, or cancelled.
type Content struct {
	Mimes []string
	Write func(mime string, w io.Writer) error
	// OnDone is optional; dropped reports whether a destination took
	// the payload (true) or the drag was cancelled (false).
	OnDone func(dropped bool)
}

// StartConfig describes one start_drag request.
type StartConfig struct {
	// Origin is the surface the implicit grab belongs to.
	Origin *wl.Surface
	// Icon is an optional drag icon surface; the compositor moves it
	// with the pointer. Nil shows a fallback cursor graphic.
	Icon *wl.Surface
	// GrabSerial is the pointer button-press serial of the gesture.
	GrabSerial uint32
	// Content is the offered payload.
	Content Content
}

// Target receives the drag-and-drop events for one bound surface,
// with coordinates in that surface's logical space. DragEnter and
// DragMotion return the accepted mime type, or "" to reject; mimes
// arrives in advertised (best-first) order.
type Target interface {
	DragEnter(mimes []string, x, y float64) string
	DragMotion(x, y float64) string
	DragLeave()
	Drop(x, y float64)
}

// Controller owns the drag-and-drop half of the session's data device.
// Create one per session with New; bind every host surface that can
// take a drop; start drags with StartDrag when a gesture earns one.
// It is not safe for concurrent use.
type Controller struct {
	sess *wlsession.Session
	// dev and maker are the wire objects the controller drives, seen
	// through the narrow interfaces below; tests substitute a fake
	// data device that records the requests.
	dev     dataDeviceAPI
	maker   dataSourceMaker
	version uint32

	// offers maps every advertised offer to its mime list. Selection
	// offers (internal/clipboard) land here too; a new drag prunes the
	// stale ones.
	offers map[*wl.DataOffer]*offerMimes

	// drag state on the destination side: the offer the compositor
	// delivered with the latest enter, its mimes, the target the drag
	// is over, the accepted mime, and the enter serial accept replies
	// with.
	dragOffer offerAPI
	dragMimes []string
	target    Target
	accepted  string
	serial    uint32
	x, y      float64

	// drag state on the source side: src is the wire proxy (the
	// source events attach to it and StartDrag names it), srcReq its
	// request side, srcData the offered content, icon the drag icon
	// surface (destroy-on-finish seam so tests can record it).
	src     *wl.DataSource
	srcReq  sourceAPI
	srcData Content
	icon    interface{ Destroy() error }
}

// dataDeviceAPI is the slice of wl.DataDevice the controller sends.
// *wl.DataDevice satisfies it; tests substitute a recorder.
type dataDeviceAPI interface {
	StartDrag(source *wl.DataSource, origin, icon *wl.Surface, serial uint32) error
}

// dataSourceMaker creates fresh data sources. The wire proxy and its
// request side are the same object in production; they are separate so
// tests can record the requests.
type dataSourceMaker interface {
	CreateDataSource() (*wl.DataSource, sourceAPI, error)
}

// sourceAPI is the request and listener side of a wl_data_source:
// offer the mimes, declare the actions, and receive the transfer
// events. *wl.DataSource satisfies it; tests substitute a recorder.
type sourceAPI interface {
	Offer(mimeType string) error
	SetActions(dndActions uint32) error
	Destroy() error
	AddSendHandler(h wl.DataSourceSendHandler)
	AddCancelledHandler(h wl.DataSourceCancelledHandler)
	AddDndDropPerformedHandler(h wl.DataSourceDndDropPerformedHandler)
	AddDndFinishedHandler(h wl.DataSourceDndFinishedHandler)
}

// offerAPI is the request side of a wl_data_offer: accept with the
// enter serial, transfer on request, acknowledge with finish.
// *wl.DataOffer satisfies it; tests substitute a recorder.
type offerAPI interface {
	Accept(serial uint32, mimeType string) error
	SetActions(dndActions, preferredAction uint32) error
	Receive(mimeType string, fd uintptr) error
	Finish() error
}

// offerMimes collects the mime types one wl_data_offer advertises, in
// arrival order. The binding dispatches offer events without naming
// their object, so each offer carries its own listener.
type offerMimes struct {
	mimes []string
}

// HandleDataOfferOffer implements wl.DataOfferOfferHandler: one
// advertised mime type of this offer.
func (o *offerMimes) HandleDataOfferOffer(ev wl.DataOfferOfferEvent) {
	o.mimes = append(o.mimes, ev.MimeType)
}

// wireSourceMaker drives a real wl.DataDeviceManager.
type wireSourceMaker struct{ mgr *wl.DataDeviceManager }

// CreateDataSource implements dataSourceMaker.
func (m wireSourceMaker) CreateDataSource() (*wl.DataSource, sourceAPI, error) {
	s, err := m.mgr.CreateDataSource()
	return s, s, err
}

// New wires a controller to the session's data device and starts
// tracking advertised offers. Safe to call when the device is missing
// (compositor without wl_data_device_manager): every request then
// fails with ErrUnavailable.
func New(sess *wlsession.Session) *Controller {
	c := &Controller{
		sess:    sess,
		version: sess.DataDeviceVersion(),
		offers:  make(map[*wl.DataOffer]*offerMimes),
	}
	if dev := sess.DataDevice(); dev != nil {
		c.dev = dev
		if mgr := sess.DataDeviceManager(); mgr != nil {
			c.maker = wireSourceMaker{mgr: mgr}
		}
		dev.AddDataOfferHandler(c)
	}
	return c
}

// Bind registers t as the drop target of surf; t == nil unbinds.
// Mirrors wlsession.SetSurfaceInput for drops.
func (c *Controller) Bind(surf *wl.Surface, t Target) {
	if surf == nil {
		return
	}
	if t == nil {
		c.sess.SetSurfaceDrop(surf, nil)
		return
	}
	c.sess.SetSurfaceDrop(surf, &surfaceTarget{c: c, t: t})
}

// surfaceTarget adapts one bound surface: the session routes events
// per surface, and the adapter hands them to the controller.
type surfaceTarget struct {
	c *Controller
	t Target
}

// HandleDragEnter implements wlsession.SurfaceDropHandler.
func (s *surfaceTarget) HandleDragEnter(x, y float64, serial uint32, offer *wl.DataOffer) {
	s.c.enter(s.t, x, y, serial, offer)
}

// HandleDragMotion implements wlsession.SurfaceDropHandler.
func (s *surfaceTarget) HandleDragMotion(x, y float64) { s.c.motion(x, y) }

// HandleDragLeave implements wlsession.SurfaceDropHandler.
func (s *surfaceTarget) HandleDragLeave() { s.c.leave() }

// HandleDrop implements wlsession.SurfaceDropHandler.
func (s *surfaceTarget) HandleDrop() { s.c.drop() }

// maxTrackedOffers bounds the offer bookkeeping: compositor-side
// objects are destroyed by the server, but the client map must not
// grow without bound when drags or selection changes are frequent.
const maxTrackedOffers = 32

// HandleDataDeviceDataOffer implements wl.DataDeviceDataOfferHandler:
// a new offer appears; start collecting its mime types. Drag and
// selection offers both pass here; each is tracked independently.
func (c *Controller) HandleDataDeviceDataOffer(ev wl.DataDeviceDataOfferEvent) {
	if ev.Id == nil {
		return
	}
	if len(c.offers) >= maxTrackedOffers {
		c.offers = make(map[*wl.DataOffer]*offerMimes)
	}
	m := &offerMimes{}
	c.offers[ev.Id] = m
	ev.Id.AddOfferHandler(m)
}

// enter records a drag entering a surface and asks its target for an
// accept/reject decision by mime.
func (c *Controller) enter(t Target, x, y float64, serial uint32, offer *wl.DataOffer) {
	c.serial = serial
	c.x, c.y = x, y
	// A drag without a data source enters with a nil offer: keep the
	// interface nil, not a typed nil, so accept stays a no-op. The
	// mime lookup still runs — offers are keyed by the wire proxy, and
	// tests may seed the nil key.
	c.dragOffer = nil
	c.dragMimes = nil
	if offer != nil {
		c.dragOffer = offer
	}
	if m := c.offers[offer]; m != nil {
		c.dragMimes = m.mimes
	}
	c.target = t
	c.accepted = ""
	if t != nil {
		c.accepted = t.DragEnter(c.dragMimes, x, y)
	}
	c.accept(c.accepted)
	debug.Log("input", "dnd enter mimes=%v accepted=%q", c.dragMimes, c.accepted)
}

// motion feeds drag movement to the current target and re-accepts,
// letting a position-aware target change its decision mid-hover.
func (c *Controller) motion(x, y float64) {
	c.x, c.y = x, y
	if c.target == nil {
		return
	}
	mime := c.target.DragMotion(x, y)
	if mime != c.accepted {
		c.accepted = mime
		c.accept(mime)
	}
}

// leave ends the drag over the target without a drop.
func (c *Controller) leave() {
	if t := c.target; t != nil {
		t.DragLeave()
	}
	c.resetDrag()
}

// drop delivers the drop to the current target; the payload transfers
// when the target asks for it through ReadPayload.
func (c *Controller) drop() {
	t, x, y := c.target, c.x, c.y
	if t == nil || c.accepted == "" {
		c.resetDrag()
		return
	}
	t.Drop(x, y)
	c.resetDrag()
}

// resetDrag clears the destination-side state; the transfer's offer
// bookkeeping goes with it.
func (c *Controller) resetDrag() {
	c.target = nil
	c.dragOffer = nil
	c.dragMimes = nil
	c.accepted = ""
	c.offers = make(map[*wl.DataOffer]*offerMimes)
}

// accept replies to the compositor with the accepted mime ("" rejects)
// using the enter serial, and declares the copy action on version 3+.
func (c *Controller) accept(mime string) {
	if c.dragOffer == nil {
		return
	}
	_ = c.dragOffer.Accept(c.serial, mime)
	if c.version >= minActionVersion {
		_ = c.dragOffer.SetActions(wl.DataDeviceManagerDndActionCopy, wl.DataDeviceManagerDndActionCopy)
	}
}

// StartDrag claims the seat's drag-and-drop with the given content:
// the data source offers the mimes, and the compositor moves the icon
// until the drag drops or cancels. The grab serial is the pointer
// press that started the gesture.
func (c *Controller) StartDrag(cfg StartConfig) error {
	if c.dev == nil || c.maker == nil {
		return ErrUnavailable
	}
	if c.src != nil {
		return ErrDragActive
	}
	if len(cfg.Content.Mimes) == 0 || cfg.Content.Write == nil {
		return errors.New("dragdrop: drag content needs mimes and a writer")
	}
	source, req, err := c.maker.CreateDataSource()
	if err != nil {
		return fmt.Errorf("dragdrop: create source: %w", err)
	}
	if source == nil || req == nil {
		return errors.New("dragdrop: data source unavailable")
	}
	c.src = source
	c.srcReq = req
	c.srcData = cfg.Content
	for _, m := range cfg.Content.Mimes {
		if err := req.Offer(m); err != nil {
			c.concludeSource(false)
			return fmt.Errorf("dragdrop: offer %q: %w", m, err)
		}
	}
	if c.version >= minActionVersion {
		if err := req.SetActions(wl.DataDeviceManagerDndActionCopy); err != nil {
			c.concludeSource(false)
			return fmt.Errorf("dragdrop: set actions: %w", err)
		}
	}
	// Keep the icon interface nil for a nil surface: a typed nil would
	// fail the non-nil check and panic on Destroy.
	c.icon = nil
	if cfg.Icon != nil {
		c.icon = cfg.Icon
	}
	req.AddSendHandler(c)
	req.AddCancelledHandler(c)
	req.AddDndDropPerformedHandler(c)
	req.AddDndFinishedHandler(c)

	if err := c.dev.StartDrag(source, cfg.Origin, cfg.Icon, cfg.GrabSerial); err != nil {
		c.concludeSource(false)
		return fmt.Errorf("dragdrop: start drag: %w", err)
	}
	debug.Log("input", "dnd start mimes=%v serial=%d", cfg.Content.Mimes, cfg.GrabSerial)
	return nil
}

// Dragging reports whether a drag started by this process is running.
func (c *Controller) Dragging() bool { return c.src != nil }

// ReadPayload returns the drop's bytes for mime. A drag that
// originated in this process short-circuits the wire: the content
// provider hands over the bytes directly, with no fd round-trip — the
// source and destination share an address space, so the pipe detour
// only adds latency. Cross-process drops read through the offer pipe
// and, on version 3+, acknowledge with finish.
func (c *Controller) ReadPayload(mime string) ([]byte, error) {
	if mime == "" {
		return nil, ErrNoPayload
	}
	if c.src != nil && c.srcData.Write != nil && c.offeredBySelf(mime) {
		var buf bytes.Buffer
		err := c.srcData.Write(mime, &buf)
		data := make([]byte, buf.Len())
		copy(data, buf.Bytes())
		// No wl_data_offer.receive will ever arrive for this drop —
		// both ends are us — so the source concludes locally instead
		// of waiting for a dnd_finished that will not come.
		c.concludeSource(true)
		return data, err
	}
	if c.dragOffer == nil || c.accepted == "" {
		return nil, ErrNoPayload
	}
	return c.receive(c.dragOffer, mime)
}

// offeredBySelf reports whether mime is one of our active source's
// offers.
func (c *Controller) offeredBySelf(mime string) bool {
	return slices.Contains(c.srcData.Mimes, mime)
}

// receive transfers the payload through the offer pipe: request, flush
// so the compositor dups the fd and asks the source to write, then read
// to EOF under the hostile-peer guard.
func (c *Controller) receive(offer offerAPI, mime string) ([]byte, error) {
	return receivePayload(offer, mime, c.sess.Roundtrip, c.version)
}

// receivePayload is the wire choreography of receive, split from the
// controller so tests can play both the offer and the flush. The
// transfer is capped at xfer.MaxPayload and bounded by the deadline;
// an oversize or stalled source surfaces as an error, with the fd
// drained and closed either way so its pipe never blocks forever. A
// truncated transfer is not finished: the drag transaction unwinds
// through the compositor's cancel path instead.
func receivePayload(offer offerAPI, mime string, flush func() error, version uint32) ([]byte, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("dragdrop: pipe: %w", err)
	}
	defer r.Close()
	if err := offer.Receive(mime, w.Fd()); err != nil {
		return nil, fmt.Errorf("dragdrop: receive: %w", err)
	}
	if err := flush(); err != nil {
		return nil, fmt.Errorf("dragdrop: flush: %w", err)
	}
	_ = w.Close()

	data, err := xfer.Read(r, xfer.MaxPayload, transferTimeout)
	if err != nil {
		return nil, fmt.Errorf("dragdrop: transfer: %w", err)
	}
	if version >= minActionVersion {
		_ = offer.Finish()
	}
	return data, nil
}

// concludeSource tears down the source side of a drag and reports the
// outcome through Content.OnDone.
func (c *Controller) concludeSource(dropped bool) {
	if c.icon != nil {
		_ = c.icon.Destroy()
		c.icon = nil
	}
	if done := c.srcData.OnDone; done != nil {
		c.srcData.OnDone = nil
		done(dropped)
	}
	if c.srcReq != nil {
		_ = c.srcReq.Destroy()
	}
	c.src = nil
	c.srcReq = nil
	c.srcData = Content{}
}

// HandleDataSourceSend implements wl.DataSourceSendHandler: a
// cross-process consumer asked for our data; write it bounded by the
// deadline — a consumer that stops reading cannot stall the dispatch
// loop — and close so the reader sees EOF whatever the outcome. A
// failed write (EPIPE, deadline) leaves the consumer a short payload
// plus EOF, evidence the transfer broke.
func (c *Controller) HandleDataSourceSend(ev wl.DataSourceSendEvent) {
	if ev.FdError != nil || c.srcData.Write == nil {
		return
	}
	f, err := xfer.DeadlineWriter(ev.Fd, transferTimeout)
	if err != nil {
		debug.Log("input", "dnd send: %v", err)
		return
	}
	if err := c.srcData.Write(ev.MimeType, f); err != nil {
		debug.Log("input", "dnd send %q: %v", ev.MimeType, err)
	}
	_ = f.Close()
}

// HandleDataSourceCancelled implements wl.DataSourceCancelledHandler:
// the compositor aborted the drag.
func (c *Controller) HandleDataSourceCancelled(wl.DataSourceCancelledEvent) {
	c.concludeSource(false)
}

// HandleDataSourceDndDropPerformed implements
// wl.DataSourceDndDropPerformedHandler: the pointer released; the icon
// goes away while the transfer may still run.
func (c *Controller) HandleDataSourceDndDropPerformed(wl.DataSourceDndDropPerformedEvent) {
	if c.icon != nil {
		_ = c.icon.Destroy()
		c.icon = nil
	}
	// Below version 3 there is no dnd_finished: conclude as dropped.
	if c.version < minActionVersion {
		c.concludeSource(true)
	}
}

// HandleDataSourceDndFinished implements wl.DataSourceDndFinishedHandler:
// the destination acknowledged the transfer.
func (c *Controller) HandleDataSourceDndFinished(wl.DataSourceDndFinishedEvent) {
	c.concludeSource(true)
}
