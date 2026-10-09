package xdg

import (
	"github.com/stubbedev/gelm/third_party/neurlang-wayland/wl"
	xdgshell "github.com/stubbedev/gelm/third_party/neurlang-wayland/xdg"
)

type (
	BaseProxy = wl.BaseProxy
	Context   = wl.Context
	Event     = wl.Event
	Toplevel  = xdgshell.Toplevel
)
