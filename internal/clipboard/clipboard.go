// Package clipboard implements text copy and paste over the core
// wl_data_device protocol and, when the compositor offers it, the
// zwp_primary_selection_unstable_v1 protocol: the offer pipeline for
// reading a selection, and data sources for claiming either. The other
// end of every transfer is a foreign client, so reads and writes are
// bounded in size and time through internal/xfer — a hostile peer can
// cost an error on a paste, never unbounded memory or a hung loop.
package clipboard

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/internal/xfer"
	"github.com/stubbedev/gelm/wlr"
)

// mimePriority lists the text mime types we offer and accept, best
// first.
var mimePriority = []string{
	"text/plain;charset=utf-8",
	"UTF8_STRING",
	"text/plain",
	"COMPOUND_TEXT",
	"TEXT",
	"STRING",
}

// imageMimePriority lists the image mime types we accept on paste,
// best first: PNG round-trips our own writes exactly; WebP, JPEG and
// GIF are read-only (decoded on paste, never offered).
var imageMimePriority = []string{
	"image/png",
	"image/webp",
	"image/jpeg",
	"image/gif",
}

// offeredImageMimes are the mimes a written image claims.
var offeredImageMimes = []string{"image/png"}

// ErrUnavailable reports that there is no selection or none in a
// format we can read.
var ErrUnavailable = errors.New("clipboard: no selection available")

// pickImageMime returns the best image mime type among those present,
// or "" when none qualify.
func pickImageMime(present func(string) bool) string {
	for _, m := range imageMimePriority {
		if present(m) {
			return m
		}
	}
	return ""
}

// transferTimeout bounds one offer read or source write against a
// peer that stalls (a var so tests can shorten it): a silent peer is
// cut off with an error instead of hanging a paste, and a consumer
// that stops reading cannot freeze the dispatch loop.
var transferTimeout = xfer.DefaultTimeout

// pickTextMime returns the best text mime type among those present,
// or "" when none qualify.
func pickTextMime(present func(string) bool) string {
	for _, m := range mimePriority {
		if present(m) {
			return m
		}
	}
	return ""
}

// Clipboard tracks the seat's selections and claims them on behalf of
// the app. Create one after connecting; it is not safe for concurrent
// use.
//
// The regular selection (ctrl+c/x/v) travels over wl_data_device. The
// primary selection (the X11-style PRIMARY that a middle click pastes)
// travels over zwp_primary_selection_unstable_v1 when the compositor
// advertises it; without it every primary call reports
// ErrUnavailable. The two selections stay independent — their offer
// mime sets and source payloads never mix.
//
// The data device also carries drag-and-drop (internal/dragdrop),
// whose offers advertise mime types through the same wl_data_offer
// objects. Each offer therefore tracks its own mime set (offerMimes),
// and only an offer the compositor promotes with a selection event can
// ever satisfy ReadText: a drag passing through never touches the
// selection.
type Clipboard struct {
	sess   *wlsession.Session
	offers map[*wl.DataOffer]*offerMimes
	// selection is the offer the compositor named as the selection;
	// selectionMimes its advertised set.
	selection      *wl.DataOffer
	selectionMimes map[string]bool
	source         *wl.DataSource
	out            string

	// The primary selection mirrors the above over its own protocol
	// objects; separate mimes map and payload, per-selection state.
	primaryOffers map[*wlr.ZwpPrimarySelectionOfferV1]*offerMimes
	primary       *wlr.ZwpPrimarySelectionOfferV1
	primaryMimes  map[string]bool
	// primarySource is the primary claim, primaryOut its text payload;
	// images never ride the primary selection (copy-on-select keeps its
	// text semantics).
	primarySource *wlr.ZwpPrimarySelectionSourceV1
	primaryOut    string

	// imgOut is the encoded PNG payload of an image claim; imgSource is
	// its data source. A claim is text or image - the last write wins,
	// exactly one source lives at a time.
	imgOut    []byte
	imgSource *wl.DataSource
}

// offerMimes collects the mime types one advertised offer carries.
// The bindings dispatch offer events without naming their object, so
// each offer gets its own listener.
type offerMimes struct {
	mimes map[string]bool
}

// HandleDataOfferOffer implements wl.DataOfferOfferHandler: one
// advertised mime type of this wl_data_offer.
func (o *offerMimes) HandleDataOfferOffer(ev wl.DataOfferOfferEvent) {
	o.mimes[ev.MimeType] = true
}

// HandleZwpPrimarySelectionOfferV1Offer implements
// wlr.ZwpPrimarySelectionOfferV1OfferHandler: one advertised mime type
// of this primary selection offer.
func (o *offerMimes) HandleZwpPrimarySelectionOfferV1Offer(ev wlr.ZwpPrimarySelectionOfferV1OfferEvent) {
	o.mimes[ev.MimeType] = true
}

// New wires the clipboard to the session's data device — and to the
// primary selection device when the compositor has one — and starts
// tracking selection offers.
func New(sess *wlsession.Session) *Clipboard {
	c := &Clipboard{
		sess:          sess,
		offers:        make(map[*wl.DataOffer]*offerMimes),
		primaryOffers: make(map[*wlr.ZwpPrimarySelectionOfferV1]*offerMimes),
	}
	if dev := sess.DataDevice(); dev != nil {
		dev.AddDataOfferHandler(c)
		dev.AddSelectionHandler(c)
	}
	if dev := sess.PrimarySelectionDevice(); dev != nil {
		dev.AddDataOfferHandler(c)
		dev.AddSelectionHandler(c)
	}
	return c
}

// HandleDataDeviceDataOffer implements wl.DataDeviceDataOfferHandler:
// a new offer appears; start listening for its mime types. Selection
// and drag offers both pass here; only a later selection event makes
// one the clipboard's.
func (c *Clipboard) HandleDataDeviceDataOffer(ev wl.DataDeviceDataOfferEvent) {
	if ev.Id == nil {
		return
	}
	m := &offerMimes{mimes: make(map[string]bool)}
	c.offers[ev.Id] = m
	ev.Id.AddOfferHandler(m)
}

// HandleDataDeviceSelection implements wl.DataDeviceSelectionHandler:
// the offer is now the selection; a nil offer clears it.
func (c *Clipboard) HandleDataDeviceSelection(ev wl.DataDeviceSelectionEvent) {
	c.selection = ev.Id
	if m := c.offers[ev.Id]; m != nil {
		c.selectionMimes = m.mimes
	} else {
		c.selectionMimes = nil
	}
}

// HandleZwpPrimarySelectionDeviceV1DataOffer implements
// wlr.ZwpPrimarySelectionDeviceV1DataOfferHandler: a new primary
// selection offer appears; start listening for its mime types.
func (c *Clipboard) HandleZwpPrimarySelectionDeviceV1DataOffer(ev wlr.ZwpPrimarySelectionDeviceV1DataOfferEvent) {
	if ev.Offer == nil {
		return
	}
	m := &offerMimes{mimes: make(map[string]bool)}
	c.primaryOffers[ev.Offer] = m
	ev.Offer.AddOfferHandler(m)
}

// HandleZwpPrimarySelectionDeviceV1Selection implements
// wlr.ZwpPrimarySelectionDeviceV1SelectionHandler: the offer is now
// the primary selection; a nil offer clears it.
func (c *Clipboard) HandleZwpPrimarySelectionDeviceV1Selection(ev wlr.ZwpPrimarySelectionDeviceV1SelectionEvent) {
	c.primary = ev.Id
	if m := c.primaryOffers[ev.Id]; m != nil {
		c.primaryMimes = m.mimes
	} else {
		c.primaryMimes = nil
	}
}

// readOfferText drains a selection offer through a pipe: receive
// starts the transfer on the write end, flush is a roundtrip so the
// compositor dups the fd, and dropping our write end yields EOF at the
// sender's last byte. The transfer itself is bounded against hostile
// peers by xfer: capped at MaxPayload and cut off by the deadline,
// with the fd drained and closed either way so the peer's pipe never
// blocks forever.
func (c *Clipboard) readOfferText(receive func(string, uintptr) error, flush func() error, mimes map[string]bool) (string, error) {
	if len(mimes) == 0 {
		return "", ErrUnavailable
	}
	mime := pickTextMime(func(m string) bool { return mimes[m] })
	if mime == "" {
		return "", ErrUnavailable
	}
	r, w, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("clipboard: pipe: %w", err)
	}
	defer r.Close()
	if err := receive(mime, w.Fd()); err != nil {
		return "", fmt.Errorf("clipboard: receive: %w", err)
	}
	// Flush the request so the compositor dups the fd before we drop
	// our write end; dropping it is what eventually yields EOF. The
	// close error carries no signal we could act on.
	if err := flush(); err != nil {
		return "", fmt.Errorf("clipboard: flush: %w", err)
	}
	_ = w.Close()

	data, err := xfer.Read(r, xfer.MaxPayload, transferTimeout)
	if err != nil {
		// An oversize or stalled peer lands here: the payload is
		// refused whole, and the fd has been drained and closed, so
		// the peer is not left blocked on the pipe.
		return "", fmt.Errorf("clipboard: transfer: %w", err)
	}
	return string(data), nil
}

// ReadText returns the current selection as text, blocking until the
// compositor delivers it. Errors with ErrUnavailable when there is
// nothing to read. The transfer is bounded against hostile peers: a
// selection past xfer.MaxPayload is refused (truncated at the cap, the
// fd drained so the offering peer never blocks) and a peer that stalls
// is cut off after transferTimeout — both surface here as errors, so
// paste callers react to a rejected paste instead of an OOM or a hung
// window.
func (c *Clipboard) ReadText() (string, error) {
	if c.selection == nil {
		return "", ErrUnavailable
	}
	return c.readOfferText(c.selection.Receive, c.sess.Roundtrip, c.selectionMimes)
}

// ReadPrimary returns the current primary selection as text, blocking
// until the compositor delivers it. Errors with ErrUnavailable when
// the protocol is missing or there is nothing to read; oversize and
// stalled peers surface as errors exactly as in ReadText — the two
// selections share one hardened transfer.
func (c *Clipboard) ReadPrimary() (string, error) {
	if c.primary == nil {
		return "", ErrUnavailable
	}
	return c.readOfferText(c.primary.Receive, c.sess.Roundtrip, c.primaryMimes)
}

// WriteText claims the selection with s as its text content. The data
// source stays alive until another client or app replaces the
// selection.
func (c *Clipboard) WriteText(s string) error {
	mgr := c.sess.DataDeviceManager()
	dev := c.sess.DataDevice()
	if mgr == nil || dev == nil {
		return ErrUnavailable
	}
	source, err := mgr.CreateDataSource()
	if err != nil {
		return fmt.Errorf("clipboard: create source: %w", err)
	}
	for _, m := range mimePriority {
		if err := source.Offer(m); err != nil {
			return fmt.Errorf("clipboard: offer: %w", err)
		}
	}
	c.dropClaim()
	c.out = s
	c.source = source
	source.AddSendHandler(c)
	source.AddCancelledHandler(c)

	if err := dev.SetSelection(source, c.sess.KeyboardSerial()); err != nil {
		return fmt.Errorf("clipboard: set selection: %w", err)
	}
	return nil
}

// WritePrimary claims the primary selection with s as its text
// content. serial is the serial of the input event that triggered the
// claim — the protocol asks for the triggering press, so callers pass
// the pointer (or keyboard) serial they received. The source stays
// alive until another client or app replaces the selection.
func (c *Clipboard) WritePrimary(s string, serial uint32) error {
	mgr := c.sess.PrimarySelectionManager()
	dev := c.sess.PrimarySelectionDevice()
	if mgr == nil || dev == nil {
		return ErrUnavailable
	}
	source, err := mgr.CreateSource()
	if err != nil {
		return fmt.Errorf("clipboard: create primary source: %w", err)
	}
	for _, m := range mimePriority {
		if err := source.Offer(m); err != nil {
			return fmt.Errorf("clipboard: offer: %w", err)
		}
	}
	c.primaryOut = s
	c.primarySource = source
	source.AddSendHandler(c)
	source.AddCancelledHandler(c)

	if err := dev.SetSelection(source, serial); err != nil {
		return fmt.Errorf("clipboard: set primary selection: %w", err)
	}
	return nil
}

// ReadImageBytes returns the current selection's image payload -
// PNG or JPEG, whichever the offer carries best - as encoded bytes
// with its mime type, blocking until the compositor delivers it.
// Errors with ErrUnavailable when the selection holds no image; the
// same xfer bounds as text apply (the 16 MiB cap is the image-sized
// one: text never approaches it). Decoding is the caller's - the
// widget pipeline decodes off the loop goroutine.
func (c *Clipboard) ReadImageBytes() ([]byte, string, error) {
	if c.selection == nil {
		return nil, "", ErrUnavailable
	}
	return c.readOfferImage(c.selection.Receive, c.sess.Roundtrip, c.selectionMimes)
}

// readOfferImage is ReadImageBytes against an injectable receive/flush
// pair, mirroring readOfferText for tests.
func (c *Clipboard) readOfferImage(receive func(string, uintptr) error, flush func() error, mimes map[string]bool) ([]byte, string, error) {
	if len(mimes) == 0 {
		return nil, "", ErrUnavailable
	}
	mime := pickImageMime(func(m string) bool { return mimes[m] })
	if mime == "" {
		return nil, "", ErrUnavailable
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, "", fmt.Errorf("clipboard: pipe: %w", err)
	}
	defer r.Close()
	if err := receive(mime, w.Fd()); err != nil {
		return nil, "", fmt.Errorf("clipboard: receive: %w", err)
	}
	if err := flush(); err != nil {
		return nil, "", fmt.Errorf("clipboard: flush: %w", err)
	}
	_ = w.Close()
	data, err := xfer.Read(r, xfer.MaxPayload, transferTimeout)
	if err != nil {
		return nil, "", fmt.Errorf("clipboard: transfer: %w", err)
	}
	return data, mime, nil
}

// WriteImage claims the regular selection with img's PNG encoding:
// encoding happens here, on the caller's goroutine, so the send path
// writes finished bytes and never encodes under a peer's deadline.
// The primary selection is untouched - copy-on-select keeps its text
// semantics; images ride the regular clipboard only.
func (c *Clipboard) WriteImage(img image.Image) error {
	if b := img.Bounds(); b.Dx() <= 0 || b.Dy() <= 0 {
		return errors.New("clipboard: image claim needs a non-empty image")
	}
	if c.sess == nil {
		return ErrUnavailable
	}
	mgr := c.sess.DataDeviceManager()
	dev := c.sess.DataDevice()
	if mgr == nil || dev == nil {
		return ErrUnavailable
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fmt.Errorf("clipboard: encode png: %w", err)
	}
	source, err := mgr.CreateDataSource()
	if err != nil {
		return fmt.Errorf("clipboard: create source: %w", err)
	}
	for _, m := range offeredImageMimes {
		if err := source.Offer(m); err != nil {
			return fmt.Errorf("clipboard: offer: %w", err)
		}
	}
	c.dropClaim()
	c.imgOut, c.imgSource = buf.Bytes(), source
	source.AddSendHandler(c)
	source.AddCancelledHandler(c)
	if err := dev.SetSelection(source, c.sess.KeyboardSerial()); err != nil {
		return fmt.Errorf("clipboard: set selection: %w", err)
	}
	return nil
}

// dropClaim tears down whichever claim (text or image) holds the
// regular selection, if any.
func (c *Clipboard) dropClaim() {
	if c.source != nil {
		_ = c.source.Destroy()
	}
	c.source, c.out = nil, ""
	if c.imgSource != nil {
		_ = c.imgSource.Destroy()
	}
	c.imgSource, c.imgOut = nil, nil
}

// fd and closes it, so the reader sees EOF. The write is bounded by
// the deadline — a consumer that never reads would otherwise stall
// the dispatch loop on a full pipe — and EPIPE from one that closed
// early ends it at once. A failed write leaves the consumer a short
// payload plus EOF: the transfer visibly broke instead of hanging.
func (c *Clipboard) sendPayload(out string, fd uintptr) {
	w, err := xfer.DeadlineWriter(fd, transferTimeout)
	if err != nil {
		// A zero fd (the binding failed to dup the descriptor) lands
		// here too; both are routine peer-side breakage.
		debug.Log("input", "clipboard send: %v", err)
		return
	}
	if _, err := io.WriteString(w, out); err != nil {
		debug.Log("input", "clipboard send: %v", err)
	}
	_ = w.Close()
}

// HandleDataSourceSend implements wl.DataSourceSendHandler: a consumer
// asked for the regular selection's data.
func (c *Clipboard) HandleDataSourceSend(ev wl.DataSourceSendEvent) {
	if ev.FdError != nil {
		return
	}
	// One source serves its claim's payload by the requested mime: the
	// image bytes for image/png, the text for the text mimes.
	if ev.MimeType == "image/png" && c.imgSource != nil {
		c.sendPayload(string(c.imgOut), ev.Fd)
		return
	}
	c.sendPayload(c.out, ev.Fd)
}

// HandleDataSourceCancelled implements wl.DataSourceCancelledHandler.
func (c *Clipboard) HandleDataSourceCancelled(wl.DataSourceCancelledEvent) {
	if c.source != nil {
		_ = c.source.Destroy()
		c.source = nil
	}
	if c.imgSource != nil {
		_ = c.imgSource.Destroy()
		c.imgSource = nil
	}
}

// HandleZwpPrimarySelectionSourceV1Send implements
// wlr.ZwpPrimarySelectionSourceV1SendHandler: a consumer asked for the
// primary selection's data.
func (c *Clipboard) HandleZwpPrimarySelectionSourceV1Send(ev wlr.ZwpPrimarySelectionSourceV1SendEvent) {
	if ev.FdError != nil {
		return
	}
	c.sendPayload(c.primaryOut, ev.Fd)
}

// HandleZwpPrimarySelectionSourceV1Cancelled implements
// wlr.ZwpPrimarySelectionSourceV1CancelledHandler.
func (c *Clipboard) HandleZwpPrimarySelectionSourceV1Cancelled(wlr.ZwpPrimarySelectionSourceV1CancelledEvent) {
	if c.primarySource != nil {
		_ = c.primarySource.Destroy()
		c.primarySource = nil
	}
}
