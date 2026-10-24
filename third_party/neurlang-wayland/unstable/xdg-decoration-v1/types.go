package xdg

import (
	"github.com/stubbedev/lys-ui/third_party/neurlang-wayland/wl"
	xdgshell "github.com/stubbedev/lys-ui/third_party/neurlang-wayland/xdg"
)

type (
	BaseProxy = wl.BaseProxy
	Context   = wl.Context
	Event     = wl.Event
	Toplevel  = xdgshell.Toplevel
)
