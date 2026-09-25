// Package wlsession owns the wl_display connection: registry discovery,
// globals binding, output scale tracking, seat input, and the blocking
// event dispatch.
package wlsession

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	deco "github.com/neurlang/wayland/unstable/xdg-decoration-v1"
	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"
	"github.com/neurlang/wayland/xdg"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/wlr"
)

// requiredGlobals are the interfaces gelm cannot run without, in the order
// they are reported as missing.
var requiredGlobals = []string{"wl_compositor", "wl_shm", "wl_output", "zwlr_layer_shell_v1"}

// minCompositorVersion is the wl_surface version SetBufferScale needs
// (set_buffer_scale is version 3).
const minCompositorVersion = 3

// Output is one wl_output and its current integer scale.
type Output struct {
	WL    *wl.Output
	Scale int
}

// Mods is a bitmask of held keyboard modifiers, mirroring the low bits
// of the wayland ModsDepressed map.
type Mods uint8

// Modifier bits.
const (
	ModShift Mods = 1 << iota
	ModCapsLock
	ModCtrl
	ModAlt
)

// Session is a connected display with the globals gelm needs bound.
type Session struct {
	Display *wl.Display

	registry          *wl.Registry
	compositor        *wl.Compositor
	shm               *wl.Shm
	layerShell        *wlr.ZwlrLayerShellV1
	seat              *wl.Seat
	pointer           *wl.Pointer
	keyboard          *wl.Keyboard
	wmBase            *xdg.WmBase
	compositorVersion uint32
	outputs           []*Output
	hasArgb           bool
	globals           map[string]bool
	ifaceNames        map[uint32]string
	mods              uint32
	repeatRate        uint32
	repeatDelay       uint32
	xkbKeymap         *xkb.Keymap
	xkbState          *xkb.State
	dataDeviceManager *wl.DataDeviceManager
	dataDevice        *wl.DataDevice
	keyboardSerial    uint32
	decorationManager *deco.ZxdgDecorationManagerV1

	// OnPointerMove fires with the pointer position in surface
	// (logical) coordinates.
	OnPointerMove func(x, y float64)
	// OnPointerButton fires on button state changes: the wayland button
	// code, 1 for press and 0 for release, and the event serial needed
	// for interactive move and resize requests.
	OnPointerButton func(button, state, serial uint32)
	// OnPointerAxis fires with vertical scroll deltas, positive down.
	OnPointerAxis func(dy float64)
	// OnPointerLeave fires when the pointer leaves the surface.
	OnPointerLeave func()
	// OnKey fires on key presses (never releases) with the evdev
	// keycode and the held modifiers.
	OnKey func(keycode uint32, mods Mods)
	// OnKeyUp fires on key releases with the evdev keycode.
	OnKeyUp func(keycode uint32)
	// OnWmBasePing fires when the compositor pings liveness; reply
	// through Window.Pong.
	OnWmBasePing func(serial uint32)
}

// Connect binds the display, waits for the initial registry burst and the
// shm format list, and fails when a required global, the HiDPI-capable
// compositor version, or the ARGB8888 shm format is missing.
func Connect() (*Session, error) {
	d, err := wl.Connect("")
	if err != nil {
		return nil, fmt.Errorf("wlsession: connect: %w", err)
	}
	s := &Session{
		Display:    d,
		globals:    make(map[string]bool),
		ifaceNames: make(map[uint32]string),
	}

	reg, err := d.GetRegistry()
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("wlsession: registry: %w", err)
	}
	s.registry = reg
	wlclient.RegistryAddListener(reg, s)

	if err := s.Roundtrip(); err != nil {
		s.Close()
		return nil, fmt.Errorf("wlsession: initial roundtrip: %w", err)
	}
	if missing := missingGlobals(s.globals); len(missing) > 0 {
		s.Close()
		return nil, fmt.Errorf("wlsession: missing wayland globals: %v", missing)
	}
	if s.compositorVersion < minCompositorVersion {
		s.Close()
		return nil, fmt.Errorf("wlsession: wl_compositor v%d lacks set_buffer_scale (need v%d)",
			s.compositorVersion, minCompositorVersion)
	}

	if err := s.Roundtrip(); err != nil {
		s.Close()
		return nil, fmt.Errorf("wlsession: globals roundtrip: %w", err)
	}
	if !s.hasArgb {
		s.Close()
		return nil, errors.New("wlsession: compositor lacks ARGB8888 wl_shm support")
	}
	return s, nil
}

// missingGlobals returns every required interface absent from have.
func missingGlobals(have map[string]bool) []string {
	var missing []string
	for _, g := range requiredGlobals {
		if !have[g] {
			missing = append(missing, g)
		}
	}
	return missing
}

// bindVersion caps a bind at the version we implement.
func bindVersion(advertised, want uint32) uint32 {
	if advertised < want {
		return advertised
	}
	return want
}

// HandleRegistryGlobal implements wl.RegistryGlobalHandler: record the
// global and bind what we need immediately.
func (s *Session) HandleRegistryGlobal(ev wl.RegistryGlobalEvent) {
	s.globals[ev.Interface] = true
	s.ifaceNames[ev.Name] = ev.Interface

	switch ev.Interface {
	case "wl_compositor":
		s.compositorVersion = ev.Version
		s.compositor = wlclient.RegistryBindCompositorInterface(s.registry, ev.Name, bindVersion(ev.Version, 4))
	case "wl_shm":
		s.shm = wlclient.RegistryBindShmInterface(s.registry, ev.Name, 1)
		wlclient.ShmAddListener(s.shm, s)
	case "wl_output":
		out := &Output{WL: wlclient.RegistryBindOutputInterface(s.registry, ev.Name, bindVersion(ev.Version, 2)), Scale: 1}
		s.outputs = append(s.outputs, out)
		wlclient.OutputAddListener(out.WL, &outputEvents{sess: s, out: out})
	case "zwlr_layer_shell_v1":
		ctx, _ := wl.GetUserData[wl.Context](s.registry)
		shell := wlr.NewZwlrLayerShellV1(ctx)
		_ = s.registry.Bind(ev.Name, ev.Interface, 1, shell)
		s.layerShell = shell
	case "wl_seat":
		s.seat = wlclient.RegistryBindSeatInterface(s.registry, ev.Name, bindVersion(ev.Version, 7))
		wlclient.SeatAddListener(s.seat, s)
		s.ensureDataDevice()
	case "xdg_wm_base":
		ctx, _ := wl.GetUserData[wl.Context](s.registry)
		wmBase := xdg.NewShell(ctx)
		_ = s.registry.Bind(ev.Name, ev.Interface, bindVersion(ev.Version, 5), wmBase)
		xdg.WmBaseAddListener(wmBase, s)
		s.wmBase = wmBase
	case "wl_data_device_manager":
		ctx, _ := wl.GetUserData[wl.Context](s.registry)
		s.dataDeviceManager = wl.NewDataDeviceManager(ctx)
		_ = s.registry.Bind(ev.Name, ev.Interface, bindVersion(ev.Version, 3), s.dataDeviceManager)
		s.ensureDataDevice()
	case "zxdg_decoration_manager_v1":
		ctx, _ := wl.GetUserData[wl.Context](s.registry)
		s.decorationManager = deco.NewZxdgDecorationManagerV1(ctx)
		_ = s.registry.Bind(ev.Name, ev.Interface, bindVersion(ev.Version, 2), s.decorationManager)
	}
}

// ensureDataDevice creates the seat's data device once both the manager
// and the seat are bound, whichever arrives first.
func (s *Session) ensureDataDevice() {
	if s.dataDevice != nil || s.dataDeviceManager == nil || s.seat == nil {
		return
	}
	dev, err := s.dataDeviceManager.GetDataDevice(s.seat)
	if err != nil {
		return
	}
	s.dataDevice = dev
}

// HandleRegistryGlobalRemove implements wl.RegistryGlobalRemoveHandler.
func (s *Session) HandleRegistryGlobalRemove(ev wl.RegistryGlobalRemoveEvent) {
	iface, ok := s.ifaceNames[ev.Name]
	if !ok {
		return
	}
	delete(s.globals, iface)
	delete(s.ifaceNames, ev.Name)
}

// HandleShmFormat implements wl.ShmFormatHandler.
func (s *Session) HandleShmFormat(ev wl.ShmFormatEvent) {
	if ev.Format == wl.ShmFormatArgb8888 {
		s.hasArgb = true
	}
}

// outputEvents tracks one output's state; the wayland handlers carry no
// back-reference, so each output gets its own listener.
type outputEvents struct {
	sess *Session
	out  *Output
}

// HandleOutputScale implements wl.OutputScaleHandler.
func (e *outputEvents) HandleOutputScale(ev wl.OutputScaleEvent) {
	if ev.Factor > 0 {
		e.out.Scale = int(ev.Factor)
	}
}

// HandleOutputGeometry implements wl.OutputGeometryHandler.
func (e *outputEvents) HandleOutputGeometry(wl.OutputGeometryEvent) {}

// HandleOutputMode implements wl.OutputModeHandler.
func (e *outputEvents) HandleOutputMode(wl.OutputModeEvent) {}

// HandleOutputDone implements wl.OutputDoneHandler.
func (e *outputEvents) HandleOutputDone(wl.OutputDoneEvent) {}

// seat capabilities bits.
const (
	capPointer  = 1
	capKeyboard = 2
)

// HandleSeatCapabilities implements wl.SeatCapabilitiesHandler: the
// pointer and keyboard objects are created as the compositor offers them.
func (s *Session) HandleSeatCapabilities(ev wl.SeatCapabilitiesEvent) {
	if ev.Capabilities&capPointer != 0 && s.pointer == nil {
		p, err := s.seat.GetPointer()
		if err != nil {
			return
		}
		s.pointer = p
		wlclient.PointerAddListener(p, s)
	}
	if ev.Capabilities&capKeyboard != 0 && s.keyboard == nil {
		k, err := s.seat.GetKeyboard()
		if err != nil {
			return
		}
		s.keyboard = k
		wlclient.KeyboardAddListener(k, s)
	}
}

// HandleSeatName implements wl.SeatNameHandler.
func (s *Session) HandleSeatName(wl.SeatNameEvent) {}

// HandleWmBasePing implements xdg.WmBasePingHandler: the compositor is
// asking whether the client is alive.
func (s *Session) HandleWmBasePing(ev xdg.WmBasePingEvent) {
	if s.OnWmBasePing != nil {
		s.OnWmBasePing(ev.Serial)
	}
}

// HandlePointerEnter implements wl.PointerEnterHandler.
func (s *Session) HandlePointerEnter(ev wl.PointerEnterEvent) {
	if s.OnPointerMove != nil {
		s.OnPointerMove(float64(ev.SurfaceX), float64(ev.SurfaceY))
	}
}

// HandlePointerLeave implements wl.PointerLeaveHandler.
func (s *Session) HandlePointerLeave(wl.PointerLeaveEvent) {
	if s.OnPointerLeave != nil {
		s.OnPointerLeave()
	}
}

// HandlePointerMotion implements wl.PointerMotionHandler.
func (s *Session) HandlePointerMotion(ev wl.PointerMotionEvent) {
	if s.OnPointerMove != nil {
		s.OnPointerMove(float64(ev.SurfaceX), float64(ev.SurfaceY))
	}
}

// HandlePointerButton implements wl.PointerButtonHandler.
func (s *Session) HandlePointerButton(ev wl.PointerButtonEvent) {
	if s.OnPointerButton != nil {
		s.OnPointerButton(ev.Button, ev.State, ev.Serial)
	}
}

// HandlePointerAxis implements wl.PointerAxisHandler.
func (s *Session) HandlePointerAxis(ev wl.PointerAxisEvent) {
	if ev.Axis == 0 && s.OnPointerAxis != nil {
		s.OnPointerAxis(float64(ev.Value))
	}
}

// HandlePointerFrame implements wl.PointerFrameHandler.
func (s *Session) HandlePointerFrame(wl.PointerFrameEvent) {}

// HandlePointerAxisSource implements wl.PointerAxisSourceHandler.
func (s *Session) HandlePointerAxisSource(wl.PointerAxisSourceEvent) {}

// HandlePointerAxisStop implements wl.PointerAxisStopHandler.
func (s *Session) HandlePointerAxisStop(wl.PointerAxisStopEvent) {}

// HandlePointerAxisDiscrete implements wl.PointerAxisDiscreteHandler.
func (s *Session) HandlePointerAxisDiscrete(wl.PointerAxisDiscreteEvent) {}

// HandlePointerAxisValue120 implements wl.PointerAxisValue120Handler.
func (s *Session) HandlePointerAxisValue120(wl.PointerAxisValue120Event) {}

// HandleKeyboardKeymap implements wl.KeyboardKeymapHandler: the keymap fd
// is consumed and closed, gelm maps evdev keycodes with a built-in US
// layout instead of parsing xkb.
func (s *Session) HandleKeyboardKeymap(ev wl.KeyboardKeymapEvent) {
	if ev.FdError != nil || ev.Fd == 0 {
		return
	}
	f := os.NewFile(ev.Fd, "wayland-keymap")
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return
	}
	ctx := xkb.NewContext(context.Background(), xkb.ContextNoFlags)
	km, err := ctx.NewKeymapFromString(data, xkb.KeymapFormatTextV1)
	if err != nil {
		return
	}
	s.xkbKeymap = km
	s.xkbState = km.NewState()
}

// HandleKeyboardEnter implements wl.KeyboardEnterHandler.
func (s *Session) HandleKeyboardEnter(wl.KeyboardEnterEvent) {}

// HandleKeyboardLeave implements wl.KeyboardLeaveHandler.
func (s *Session) HandleKeyboardLeave(wl.KeyboardLeaveEvent) {}

// HandleKeyboardKey implements wl.KeyboardKeyHandler.
func (s *Session) HandleKeyboardKey(ev wl.KeyboardKeyEvent) {
	switch ev.State {
	case 1:
		if s.OnKey != nil {
			s.OnKey(ev.Key, s.Mods())
		}
	case 0:
		if s.OnKeyUp != nil {
			s.OnKeyUp(ev.Key)
		}
	}
}

// HandleKeyboardModifiers implements wl.KeyboardModifiersHandler.
func (s *Session) HandleKeyboardModifiers(ev wl.KeyboardModifiersEvent) {
	s.mods = ev.ModsDepressed
	if s.xkbState != nil {
		s.xkbState.UpdateMask(xkb.ModMask(ev.ModsDepressed), xkb.ModMask(ev.ModsLatched),
			xkb.ModMask(ev.ModsLocked), xkb.Group(ev.Group), xkb.Group(0), xkb.Group(0))
	}
}

// KeyUTF8 translates an evdev keycode through the compositor's keymap
// and returns the text it produces with the current modifiers (shift,
// AltGr, layout). Empty when no keymap arrived or the key types
// nothing. Keycodes need the +8 evdev-to-xkb offset.
func (s *Session) KeyUTF8(code uint32) string {
	if s.xkbState == nil {
		return ""
	}
	return s.xkbState.KeyGetUTF8(xkb.Keycode(code + 8))
}

// KeySym translates an evdev keycode to its primary keysym under the
// current state; KeyNoSymbol when there is no keymap.
func (s *Session) KeySym(code uint32) xkb.Keysym {
	if s.xkbState == nil {
		return xkb.KeyNoSymbol
	}
	return s.xkbState.KeyGetOneSym(xkb.Keycode(code + 8))
}

// Mods returns the currently held modifiers (shift, ctrl, alt).
func (s *Session) Mods() Mods {
	return Mods(s.mods) & (ModShift | ModCtrl | ModAlt)
}

// HandleKeyboardRepeatInfo implements wl.KeyboardRepeatInfoHandler: the
// compositor's rate (keys per second) and delay (milliseconds).
func (s *Session) HandleKeyboardRepeatInfo(ev wl.KeyboardRepeatInfoEvent) {
	s.repeatRate = uint32(ev.Rate)
	s.repeatDelay = uint32(ev.Delay)
}

// RepeatInfo returns the compositor's key repeat rate in keys per second
// and the initial delay in milliseconds. Zeros mean the compositor sent
// no repeat info; callers fall back to their own defaults.
func (s *Session) RepeatInfo() (rate, delayMs uint32) {
	return s.repeatRate, s.repeatDelay
}

// Compositor returns the bound wl_compositor.
func (s *Session) Compositor() *wl.Compositor { return s.compositor }

// Shm returns the bound wl_shm.
func (s *Session) Shm() *wl.Shm { return s.shm }

// LayerShell returns the bound zwlr_layer_shell_v1.
func (s *Session) LayerShell() *wlr.ZwlrLayerShellV1 { return s.layerShell }

// Outputs returns the bound outputs in registry order.
func (s *Session) Outputs() []*Output { return s.outputs }

// WmBase returns the bound xdg_wm_base, or nil when the compositor does
// not provide it; window support needs it.
func (s *Session) WmBase() *xdg.WmBase { return s.wmBase }

// Seat returns the bound wl_seat, or nil when the compositor has none.
func (s *Session) Seat() *wl.Seat { return s.seat }

// DataDeviceManager returns the bound wl_data_device_manager, or nil
// when the compositor does not provide it; clipboard support needs it.
func (s *Session) DataDeviceManager() *wl.DataDeviceManager { return s.dataDeviceManager }

// DataDevice returns the seat's data device, or nil before both the
// manager and the seat are bound.
func (s *Session) DataDevice() *wl.DataDevice { return s.dataDevice }

// KeyboardSerial returns the serial of the last keyboard enter, needed
// by selection requests.
func (s *Session) KeyboardSerial() uint32 { return s.keyboardSerial }

// DecorationManager returns the bound xdg-decoration manager, or nil
// when the compositor does not provide it; server-side window
// decorations need it.
func (s *Session) DecorationManager() *deco.ZxdgDecorationManagerV1 {
	return s.decorationManager
}

// Roundtrip issues a display sync and dispatches until it completes.
// Proxies destroyed mid-queue (a done frame callback, a dismissed
// popup) abort a dispatch pass with ErrContextRunProxyNil; retrying is
// safe and finishes the roundtrip.
func (s *Session) Roundtrip() error {
	cb, err := s.Display.Sync()
	if err != nil {
		return err
	}
	err = s.Display.Context().RunTill(cb)
	for errors.Is(err, wl.ErrContextRunProxyNil) {
		err = s.Display.Context().RunTill(cb)
	}
	return err
}

// Run dispatches events forever; it returns when the connection dies.
func (s *Session) Run() error {
	err := s.Display.Context().Run()
	for errors.Is(err, wl.ErrContextRunProxyNil) {
		err = s.Display.Context().Run()
	}
	return err
}

// Close disconnects from the display.
func (s *Session) Close() {
	if s.Display != nil {
		_ = s.Display.Context().Close()
	}
}
