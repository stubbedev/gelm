// Package clipboard implements text copy and paste over the core
// wl_data_device protocol: the offer pipeline for reading the current
// selection, and a data source for claiming it.
package clipboard

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/internal/wlsession"
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

// ErrUnavailable reports that there is no selection or none in a text
// format we can read.
var ErrUnavailable = errors.New("clipboard: no text selection available")

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

// Clipboard tracks the seat's selection and claims it on behalf of the
// app. Create one after connecting; it is not safe for concurrent use.
type Clipboard struct {
	sess   *wlsession.Session
	offer  *wl.DataOffer
	mimes  map[string]bool
	source *wl.DataSource
	out    string
}

// New wires the clipboard to the session's data device and starts
// tracking selection offers.
func New(sess *wlsession.Session) *Clipboard {
	c := &Clipboard{sess: sess, mimes: make(map[string]bool)}
	if dev := sess.DataDevice(); dev != nil {
		dev.AddDataOfferHandler(c)
		dev.AddSelectionHandler(c)
	}
	return c
}

// HandleDataDeviceDataOffer implements wl.DataDeviceDataOfferHandler: a
// new offer appears; listen for its mime types.
func (c *Clipboard) HandleDataDeviceDataOffer(ev wl.DataDeviceDataOfferEvent) {
	c.mimes = make(map[string]bool)
	c.offer = ev.Id
	if ev.Id != nil {
		ev.Id.AddOfferHandler(c)
	}
}

// HandleDataOffer implements wl.DataOfferOfferHandler: one advertised
// mime type.
func (c *Clipboard) HandleDataOfferOffer(ev wl.DataOfferOfferEvent) {
	if c.mimes != nil {
		c.mimes[ev.MimeType] = true
	}
}

// HandleDataDeviceSelection implements wl.DataDeviceSelectionHandler:
// the offer is now the selection; a nil offer clears it.
func (c *Clipboard) HandleDataDeviceSelection(ev wl.DataDeviceSelectionEvent) {
	c.offer = ev.Id
	if ev.Id == nil {
		c.mimes = nil
	}
}

// ReadText returns the current selection as text, blocking until the
// compositor delivers it. Errors with ErrUnavailable when there is
// nothing to read.
func (c *Clipboard) ReadText() (string, error) {
	if c.offer == nil || len(c.mimes) == 0 {
		return "", ErrUnavailable
	}
	mime := pickTextMime(func(m string) bool { return c.mimes[m] })
	if mime == "" {
		return "", ErrUnavailable
	}

	r, w, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("clipboard: pipe: %w", err)
	}
	defer r.Close()
	if err := c.offer.Receive(mime, w.Fd()); err != nil {
		return "", fmt.Errorf("clipboard: receive: %w", err)
	}
	// Flush the request so the compositor dups the fd before we drop
	// our write end; dropping it is what eventually yields EOF. The
	// close error carries no signal we could act on.
	if err := c.sess.Roundtrip(); err != nil {
		return "", fmt.Errorf("clipboard: flush: %w", err)
	}
	_ = w.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("clipboard: read: %w", err)
	}
	return string(data), nil
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
	c.out = s
	c.source = source
	source.AddSendHandler(c)
	source.AddCancelledHandler(c)

	if err := dev.SetSelection(source, c.sess.KeyboardSerial()); err != nil {
		return fmt.Errorf("clipboard: set selection: %w", err)
	}
	return nil
}

// HandleDataSourceSend implements wl.DataSourceSendHandler: a consumer
// asked for our data; write it and close so the reader sees EOF.
func (c *Clipboard) HandleDataSourceSend(ev wl.DataSourceSendEvent) {
	if ev.FdError != nil || ev.Fd == 0 {
		return
	}
	f := os.NewFile(ev.Fd, "clipboard-send")
	_, _ = io.WriteString(f, c.out)
	_ = f.Close()
}

// HandleDataSourceCancelled implements wl.DataSourceCancelledHandler.
func (c *Clipboard) HandleDataSourceCancelled(wl.DataSourceCancelledEvent) {
	if c.source != nil {
		_ = c.source.Destroy()
		c.source = nil
	}
}
