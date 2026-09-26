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
	"time"

	deco "github.com/neurlang/wayland/unstable/xdg-decoration-v1"
	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"
	"github.com/neurlang/wayland/xdg"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/internal/debug"
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

	desiredCursor      string
	pointerEnterSerial uint32
	cursorSurface      *wl.Surface

	// Pointer routing state. surfaceHandlers maps a wl_surface to the
	// handler that receives pointer events targeting it; pointerFocus
	// is the surface the compositor says the pointer is over, and
	// grabSurface holds implicit-grab routing while a button is down.
	surfaceHandlers map[*wl.Surface]SurfacePointerHandler
	pointerFocus    *wl.Surface
	grabSurface     *wl.Surface

	// OnKey fires on key presses (never releases) with the evdev
	// keycode and the held modifiers. Keyboard focus is seat-wide,
	// unlike pointer events which route per surface.
	OnKey func(keycode uint32, mods Mods)
	// OnKeyUp fires on key releases with the evdev keycode.
	OnKeyUp func(keycode uint32)
	// OnWmBasePing fires when the compositor pings liveness; reply
	// through Window.Pong.
	OnWmBasePing func(serial uint32)
}

// SurfacePointerHandler receives the pointer events whose compositor
// focus belongs to one registered surface, with coordinates in that
// surface's logical space. Hosts and popup surfaces register one
// handler each, so multiple surfaces never fight over one callback.
type SurfacePointerHandler interface {
	// HandlePointerEnter reports the pointer entering the surface.
	HandlePointerEnter(x, y float64)
	// HandlePointerMotion reports movement within the surface; while
	// a grab is active it may carry coordinates outside the surface.
	HandlePointerMotion(x, y float64)
	// HandlePointerButton reports a press or release: the wayland
	// button code, 1 pressed / 0 released, and the event serial.
	HandlePointerButton(button, state, serial uint32)
	// HandlePointerAxis reports vertical scroll, positive down.
	HandlePointerAxis(dy float64)
	// HandlePointerLeave reports the pointer leaving the surface.
	HandlePointerLeave()
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
		Display:         d,
		globals:         make(map[string]bool),
		ifaceNames:      make(map[uint32]string),
		surfaceHandlers: make(map[*wl.Surface]SurfacePointerHandler),
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

// HandleSeatCapabilities implements wl.SeatCapabilitiesHandler. The
// pointer and keyboard objects track the advertised capabilities: when
// a capability disappears the compositor destroys the matching object,
// so the stale proxy must be dropped and a fresh one created on the
// next gain, or input goes silent after a unplug-replug.
func (s *Session) HandleSeatCapabilities(ev wl.SeatCapabilitiesEvent) {
	hasPointer := ev.Capabilities&capPointer != 0
	hasKeyboard := ev.Capabilities&capKeyboard != 0

	if !hasPointer && s.pointer != nil {
		debug.Log("seat", "pointer capability lost")
		s.pointerLost()
	}
	if hasPointer && s.pointer == nil {
		p, err := s.seat.GetPointer()
		if err != nil {
			return
		}
		s.pointer = p
		wlclient.PointerAddListener(p, s)
		debug.Log("seat", "pointer capability gained")
	}
	if !hasKeyboard && s.keyboard != nil {
		s.keyboard = nil
		s.mods = 0
		debug.Log("seat", "keyboard capability lost")
	}
	if hasKeyboard && s.keyboard == nil {
		k, err := s.seat.GetKeyboard()
		if err != nil {
			return
		}
		s.keyboard = k
		wlclient.KeyboardAddListener(k, s)
		debug.Log("seat", "keyboard capability gained")
	}
}

// pointerLost drops the pointer object and every routing state that
// depends on it, notifying the focused surface's handler first.
func (s *Session) pointerLost() {
	s.pointer = nil
	s.pointerEnterSerial = 0
	s.grabSurface = nil
	if s.pointerFocus != nil {
		if h := s.surfaceHandlers[s.pointerFocus]; h != nil {
			h.HandlePointerLeave()
		}
		s.pointerFocus = nil
	}
}

// SetSurfaceInput registers h as the receiver of pointer events
// targeting surf; h == nil unregisters. Handlers must be registered
// before the surface can receive input and removed when the surface
// is destroyed.
func (s *Session) SetSurfaceInput(surf *wl.Surface, h SurfacePointerHandler) {
	if h == nil {
		delete(s.surfaceHandlers, surf)
		return
	}
	s.surfaceHandlers[surf] = h
}

// pointerTarget returns the handler pointer events route to right
// now: the grabbed surface while a button is held, else the surface
// under the pointer. Nil when neither applies or nothing is
// registered.
func (s *Session) pointerTarget() SurfacePointerHandler {
	surf := s.grabSurface
	if surf == nil {
		surf = s.pointerFocus
	}
	if surf == nil {
		return nil
	}
	return s.surfaceHandlers[surf]
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

// HandlePointerEnter implements wl.PointerEnterHandler: the pointer
// gained focus on ev.Surface. The enter is routed to that surface's
// handler and becomes the motion target until a leave or grab says
// otherwise.
func (s *Session) HandlePointerEnter(ev wl.PointerEnterEvent) {
	s.pointerEnterSerial = ev.Serial
	_ = s.applyCursor()
	s.pointerFocus = ev.Surface
	debug.Log("input", "wire enter surf=%d (%.1f,%.1f)",
		ev.Surface.Id(), ev.SurfaceX, ev.SurfaceY)
	if h := s.surfaceHandlers[ev.Surface]; h != nil {
		h.HandlePointerEnter(float64(ev.SurfaceX), float64(ev.SurfaceY))
	}
}

// HandlePointerLeave implements wl.PointerLeaveHandler: pointer focus
// left ev.Surface. A leave always ends the client-side grab: the
// compositor only strips focus when the pointer truly left or the
// grabbing input device went away.
func (s *Session) HandlePointerLeave(ev wl.PointerLeaveEvent) {
	debug.Log("input", "wire leave surf=%d", ev.Surface.Id())
	s.grabSurface = nil
	if s.pointerFocus == ev.Surface {
		s.pointerFocus = nil
	}
	if h := s.surfaceHandlers[ev.Surface]; h != nil {
		h.HandlePointerLeave()
	}
}

// HandlePointerMotion implements wl.PointerMotionHandler: routed to
// the grabbed surface during a drag, else the focused surface.
func (s *Session) HandlePointerMotion(ev wl.PointerMotionEvent) {
	if h := s.pointerTarget(); h != nil {
		h.HandlePointerMotion(float64(ev.SurfaceX), float64(ev.SurfaceY))
	}
	debug.Log("input", "wire motion (%.1f,%.1f)", ev.SurfaceX, ev.SurfaceY)
}

// HandlePointerButton implements wl.PointerButtonHandler: routed like
// motion, and a press opens the client-side implicit grab so drags
// keep feeding the pressed surface after the pointer leaves it. A
// release ends the grab.
func (s *Session) HandlePointerButton(ev wl.PointerButtonEvent) {
	debug.Log("input", "wire button %d state=%d serial=%d", ev.Button, ev.State, ev.Serial)
	if h := s.pointerTarget(); h != nil {
		h.HandlePointerButton(ev.Button, ev.State, ev.Serial)
	}
	switch ev.State {
	case 1:
		if s.grabSurface == nil {
			s.grabSurface = s.pointerFocus
		}
	case 0:
		s.grabSurface = nil
	}
}

// HandlePointerAxis implements wl.PointerAxisHandler: routed like
// motion, so wheel scrolling follows the grab while dragging.
func (s *Session) HandlePointerAxis(ev wl.PointerAxisEvent) {
	if ev.Axis != 0 {
		return
	}
	if h := s.pointerTarget(); h != nil {
		h.HandlePointerAxis(float64(ev.Value))
	}
	debug.Log("input", "wire axis %.1f", ev.Value)
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

// Step dispatches exactly one event, blocking until one arrives. It is
// the park point of an event-driven loop: with nothing to do, a loop
// calling Step holds no CPU and wakes only when the compositor sends
// something or a WakeAfter kick fires.
func (s *Session) Step() error {
	err := s.Display.Context().Run()
	for errors.Is(err, wl.ErrContextRunProxyNil) {
		err = s.Display.Context().Run()
	}
	return err
}

// kickHandler unregisters its sync callback once the wakeup fired, so
// parked-loop kicks do not leak proxies.
type kickHandler struct{ cb *wl.Callback }

// HandleCallbackDone implements wl.CallbackDoneHandler.
func (k kickHandler) HandleCallbackDone(wl.CallbackDoneEvent) {
	wlclient.CallbackDestroy(k.cb)
}

// WakeAfter arranges for a loop parked in Step to return by waiting d:
// a timer sends a wl_display.sync, and its done event ends the blocking
// read. The sync callback may occasionally leak its registration if the
// event is dispatched before the listener attaches; the wake itself is
// unaffected because dispatching the event is what ends Step.
func (s *Session) WakeAfter(d time.Duration) {
	if d < 0 {
		d = 0
	}
	time.AfterFunc(d, func() {
		cb, err := s.Display.Sync()
		if err != nil {
			return
		}
		wlclient.CallbackAddListener(cb, kickHandler{cb: cb})
	})
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
