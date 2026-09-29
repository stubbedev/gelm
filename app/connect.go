package app

import "github.com/stubbedev/gelm/internal/wlsession"

// Connect opens the Wayland session for an Application. The session
// type is unexported on purpose (consumers hold it opaquely and pass
// it to NewApplication); this constructor is the public way in.
func Connect() (*wlsession.Session, error) {
	return wlsession.Connect()
}
