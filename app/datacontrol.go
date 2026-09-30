// Data control: the clipboard-manager protocols, for apps that watch
// or own the seat's selections without a focused window — a
// clipboard history, a bar's clipboard indicator, a remote-desktop
// bridge. See internal/datacontrol for the protocol side.
package app

import (
	"github.com/stubbedev/gelm/internal/datacontrol"
	"github.com/stubbedev/gelm/internal/xfer"
)

// DataControl is the seat's data-control device, over
// ext-data-control-v1 or wlr-data-control-unstable-v1. Get it from
// Application.DataControl; call it from the loop goroutine.
type DataControl = datacontrol.Device

// Selection names one of the seat's two selections.
type Selection = datacontrol.Selection

// The seat's selections.
const (
	// SelectionClipboard is the regular ctrl+c / ctrl+v selection.
	SelectionClipboard = datacontrol.Clipboard
	// SelectionPrimary is the middle-click primary selection.
	SelectionPrimary = datacontrol.Primary
)

// DataControlProtocol names the protocol a DataControl speaks.
type DataControlProtocol = datacontrol.Protocol

// The data-control protocols.
const (
	// DataControlExt is the staging ext-data-control-v1.
	DataControlExt = datacontrol.ProtocolExt
	// DataControlWlr is wlr-data-control-unstable-v1.
	DataControlWlr = datacontrol.ProtocolWlr
)

// SelectionOffer is one selection's content as its owner advertised
// it: the mime types in the owner's order, readable until the
// compositor replaces the selection.
type SelectionOffer = datacontrol.Offer

// SelectionSource is what a SetSelection claim offers: mime types in
// preference order, and the payload for each.
type SelectionSource = datacontrol.Source

// SelectionClaim is our ownership of one selection.
type SelectionClaim = datacontrol.Claim

var (
	// ErrDataControlUnavailable reports a compositor that offers
	// neither data-control protocol.
	ErrDataControlUnavailable = datacontrol.ErrUnavailable
	// ErrPrimaryUnavailable reports a primary-selection request the
	// bound protocol version cannot carry.
	ErrPrimaryUnavailable = datacontrol.ErrPrimaryUnavailable
	// ErrDataControlFinished reports a device the compositor retired.
	ErrDataControlFinished = datacontrol.ErrFinished
	// ErrStaleOffer reports a read on a replaced selection offer.
	ErrStaleOffer = datacontrol.ErrStaleOffer
	// ErrMimeNotOffered reports a read under an unadvertised mime.
	ErrMimeNotOffered = datacontrol.ErrMimeNotOffered
	// ErrTransferTooLarge reports a selection read that streamed past
	// its limit; the payload is refused whole.
	ErrTransferTooLarge = xfer.ErrTooLarge
	// ErrTransferTimeout reports a selection read or write cut off by
	// the transfer deadline.
	ErrTransferTimeout = xfer.ErrTimeout
)

// DataControl returns the seat's data-control device, creating it on
// first use; later calls return the same device. The compositor
// announces the current selections right after creation, so register
// OnSelection before the loop next dispatches. Fails with
// ErrDataControlUnavailable when the compositor lacks both protocols.
func (a *Application) DataControl() (*DataControl, error) {
	if a.dataControl != nil {
		return a.dataControl, nil
	}
	d, err := datacontrol.Bind(a.sess, a.Invoke)
	if err != nil {
		return nil, err
	}
	a.dataControl = d
	return d, nil
}
