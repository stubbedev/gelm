package main

import (
	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"

	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/widget"
)

func wlclientBufferListener(b *buffer.Buffer) {
	wlclient.BufferAddListener(b.WL, buffer.ReleaseHandler{B: b})
}

func wlclientCallbackListener(cb *wl.Callback, ready *bool) {
	wlclient.CallbackAddListener(cb, frameDone{ready: ready})
}

type frameDone struct {
	ready *bool
}

// HandleCallbackDone implements wl.CallbackDoneHandler.
func (f frameDone) HandleCallbackDone(wl.CallbackDoneEvent) {
	*f.ready = true
}

// keyAction maps an evdev keycode to a widget action; ok reports whether
// the key is mapped at all.
func keyAction(code uint32) (action widget.KeyAction, ok bool) {
	switch code {
	case 14:
		return widget.KeyBackspace, true
	case 119:
		return widget.KeyDelete, true
	case 105:
		return widget.KeyLeft, true
	case 106:
		return widget.KeyRight, true
	case 102:
		return widget.KeyHome, true
	case 107:
		return widget.KeyEnd, true
	case 28:
		return widget.KeyEnter, true
	}
	return 0, false
}

// mapKey maps an evdev keycode plus shift to either a printable rune or a
// widget action, using a compact US-QWERTY layout. It covers the keys a
// panel needs; full xkb support is future work.
func mapKey(code uint32, shift bool) (ch rune, action widget.KeyAction, ok bool) {
	if a, ok := keyAction(code); ok {
		return 0, a, true
	}
	letters := "qwertyuiopasdfghjklzxcvbnm"
	switch {
	case code >= 16 && code <= 25: // qwertyuiop
		return shiftRune(rune(letters[code-16]), shift), 0, true
	case code >= 30 && code <= 38: // asdfghjkl
		return shiftRune(rune(letters[code-30+10]), shift), 0, true
	case code >= 44 && code <= 50: // zxcvbnm
		return shiftRune(rune(letters[code-44+19]), shift), 0, true
	case code >= 2 && code <= 10: // digits row 1..9
		digits := "123456789"
		symbols := "!@#$%^&*("
		if shift {
			return rune(symbols[code-2]), 0, true
		}
		return rune(digits[code-2]), 0, true
	case code == 11:
		if shift {
			return ')', 0, true
		}
		return '0', 0, true
	case code == 12:
		if shift {
			return '_', 0, true
		}
		return '-', 0, true
	case code == 13:
		if shift {
			return '+', 0, true
		}
		return '=', 0, true
	case code == 26:
		if shift {
			return '{', 0, true
		}
		return '[', 0, true
	case code == 27:
		if shift {
			return '}', 0, true
		}
		return ']', 0, true
	case code == 39:
		if shift {
			return ':', 0, true
		}
		return ';', 0, true
	case code == 40:
		if shift {
			return '"', 0, true
		}
		return '\'', 0, true
	case code == 41:
		if shift {
			return '~', 0, true
		}
		return '`', 0, true
	case code == 43:
		if shift {
			return '|', 0, true
		}
		return '\\', 0, true
	case code == 51:
		if shift {
			return '<', 0, true
		}
		return ',', 0, true
	case code == 52:
		if shift {
			return '>', 0, true
		}
		return '.', 0, true
	case code == 53:
		if shift {
			return '?', 0, true
		}
		return '/', 0, true
	case code == 57:
		return ' ', 0, true
	}
	return 0, 0, false
}

func shiftRune(r rune, shift bool) rune {
	if shift {
		return r - 32
	}
	return r
}
