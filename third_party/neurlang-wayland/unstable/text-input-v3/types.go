package text

import "github.com/stubbedev/gelm/third_party/neurlang-wayland/wl"

type (
	BaseProxy = wl.BaseProxy
	Context   = wl.Context
	Event     = wl.Event
	Surface   = wl.Surface
	Seat      = wl.Seat
)

func SafeCast[T any](p wl.Proxy) T {
	return wl.SafeCast[T](p)
}
