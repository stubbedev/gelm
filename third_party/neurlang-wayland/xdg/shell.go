// Package xdg implements the stable XDG Window Manager Base protocol
package xdg

//go:generate ../../../../../bin/go-wayland-scanner -pkg xdg -i xdg-shell.xml -o xdg-shell.xml.go

import "github.com/stubbedev/gelm/third_party/neurlang-wayland/wl"

type (
	WlSurface = wl.Surface
	BaseProxy = wl.BaseProxy
	Event     = wl.Event
	Context   = wl.Context
	Proxy     = wl.Proxy
)

func (s *Surface) AddListener(h SurfaceConfigureHandler) {
	s.AddConfigureHandler(h)
}

type (
	Seat   = wl.Seat
	Output = wl.Output
)

func NewShell(ctx *Context) *WmBase {
	ret := new(WmBase)
	ret.privateWmBasePings = make(map[WmBasePingHandler]struct{})
	ctx.Register(ret)
	return ret
}

func WmBaseAddListener(s *WmBase, h WmBasePingHandler) {
	s.AddPingHandler(h)
}

type ToplevelListener interface {
	ToplevelConfigureHandler
	ToplevelCloseHandler
}

func ToplevelAddListener(tl *Toplevel, h ToplevelListener) {
	tl.AddConfigureHandler(h)
	tl.AddCloseHandler(h)
}
