// XDG activation: optional xdg_activation_v1 support — the
// compositor-consented focus request. A client asks for a token
// anchored to a real user interaction (the serial of a pointer press
// or keyboard event); once the compositor issues it, the token may be
// spent to raise and focus a surface, or handed to a spawned app
// through XDG_ACTIVATION_TOKEN so the child can focus itself. The
// manager global is feature-detected: without it the requests are
// no-ops.
package wlsession

import (
	"slices"

	"github.com/neurlang/wayland/wl"

	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/wlr"
)

// maxActivationVersion is the xdg_activation_v1 version we bind: the
// protocol stopped at version 1.
const maxActivationVersion = 1

// activationAPI is the request side of xdg_activation_v1, seen
// through the narrow interface below so tests can record the
// requests. The wire proxy satisfies it through wireActivation.
type activationAPI interface {
	GetActivationToken() (activationTokenAPI, error)
	Activate(token string, surface *wl.Surface) error
}

// activationTokenAPI is the request and listener side of
// xdg_activation_token_v1. *wlr.ActivationTokenV1 satisfies it;
// tests substitute a recorder.
type activationTokenAPI interface {
	SetSerial(serial uint32, seat *wl.Seat) error
	SetAppId(appID string) error
	SetSurface(surface *wl.Surface) error
	Commit() error
	Destroy() error
	AddDoneHandler(h wlr.ActivationTokenV1DoneHandler)
}

// wireActivation adapts the generated xdg-activation proxies to the
// narrow interfaces above; tests substitute recorders.
type wireActivation struct{ mgr *wlr.ActivationV1 }

// GetActivationToken implements activationAPI.
func (a wireActivation) GetActivationToken() (activationTokenAPI, error) {
	t, err := a.mgr.GetActivationToken()
	if err != nil {
		return nil, err
	}
	return t, nil
}

// Activate implements activationAPI.
func (a wireActivation) Activate(token string, surface *wl.Surface) error {
	return a.mgr.Activate(token, surface)
}

// ActivationAvailable reports whether the compositor advertised
// xdg_activation_v1. When false, every activation call on the session
// is a no-op.
func (s *Session) ActivationAvailable() bool { return s.activation != nil }

// bindActivation binds the optional activation global.
func (s *Session) bindActivation(ev wl.RegistryGlobalEvent) {
	ctx, _ := wl.GetUserData[wl.Context](s.registry)
	mgr := wlr.NewActivationV1(ctx)
	if err := s.registry.Bind(ev.Name, ev.Interface, bindVersion(ev.Version, maxActivationVersion), mgr); err != nil {
		return
	}
	s.activation = wireActivation{mgr: mgr}
	debug.Log("shell", "xdg-activation-v1 bound")
}

// RequestActivationToken begins a token request anchored to the
// user-interaction serial of a pointer press or keyboard event (0
// means unanchored, which compositors may refuse to honor with
// focus). The issued token arrives through OnActivationToken; a
// launcher typically writes it into XDG_ACTIVATION_TOKEN for the app
// it spawns. No-op without the protocol.
func (s *Session) RequestActivationToken(surface *wl.Surface, serial uint32) {
	req := s.activation
	if req == nil {
		return
	}
	tokReq, err := req.GetActivationToken()
	if err != nil {
		debug.Log("shell", "activation token: %v", err)
		return
	}
	tok := &activationToken{sess: s, req: tokReq}
	s.activationTokens = append(s.activationTokens, tok)
	tokReq.AddDoneHandler(tok)
	if serial != 0 && s.seat != nil {
		_ = tokReq.SetSerial(serial, s.seat)
	}
	if surface != nil {
		_ = tokReq.SetSurface(surface)
	}
	if err := tokReq.Commit(); err != nil {
		s.removeActivationToken(tok)
		_ = tokReq.Destroy()
		debug.Log("shell", "activation token commit: %v", err)
	}
}

// Activate raises and focuses surface with a token the compositor
// issued through RequestActivationToken (the one the launcher handed
// to the spawned app). No-op without the protocol.
func (s *Session) Activate(surface *wl.Surface, token string) {
	if s.activation == nil || token == "" || surface == nil {
		return
	}
	_ = s.activation.Activate(token, surface)
}

// activationToken tracks one in-flight token request.
type activationToken struct {
	sess *Session
	req  activationTokenAPI
}

// HandleActivationTokenV1Done implements
// wlr.ActivationTokenV1DoneHandler: the compositor issued the token;
// hand it to the host and tear the object down.
func (t *activationToken) HandleActivationTokenV1Done(ev wlr.ActivationTokenV1DoneEvent) {
	t.sess.removeActivationToken(t)
	_ = t.req.Destroy()
	if t.sess.OnActivationToken != nil {
		t.sess.OnActivationToken(ev.Token)
	}
}

// removeActivationToken drops a finished or failed request from the
// in-flight list.
func (s *Session) removeActivationToken(tok *activationToken) {
	for i, t := range s.activationTokens {
		if t == tok {
			s.activationTokens = slices.Delete(s.activationTokens, i, i+1)
			return
		}
	}
}
