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

	text "github.com/neurlang/wayland/unstable/text-input-v3"
	deco "github.com/neurlang/wayland/unstable/xdg-decoration-v1"
	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"
	"github.com/neurlang/wayland/xdg"
	"github.com/unxed/xkb-go"

	"github.com/stubbedev/gelm/internal/buffer"
	"github.com/stubbedev/gelm/internal/debug"
	"github.com/stubbedev/gelm/wlr"
)

// requiredGlobals are the interfaces gelm cannot run without, in the order
// they are reported as missing. The fractional-scale protocols
// (wp_viewporter, wp_fractional_scale_manager_v1) are deliberately not
// here: they are feature-detected and optional, and compositors without
// them keep the exact integer-scale behavior.
var requiredGlobals = []string{"wl_compositor", "wl_shm", "wl_output", "zwlr_layer_shell_v1"}

// minCompositorVersion is the wl_surface version SetBufferScale needs
// (set_buffer_scale is version 3).
const minCompositorVersion = 3

// minDataDeviceVersion is the wl_data_device_manager version the dnd
// action requests and events need (set_actions, finish, dnd_finished).
const minDataDeviceVersion = 3

// Output is one wl_output and its current integer scale.
type Output struct {
	WL    *wl.Output
	Scale int

	// ModeW and ModeH are the current mode's pixel size.
	ModeW, ModeH int

	// Transform is the output's rotation/flip as wl_output.geometry
	// reports it (a wl_output.transform enum); surfaces pinned to the
	// output publish it with wl_surface.set_buffer_transform.
	Transform int32

	// Name and Description are the output's stable xdg-output identity
	// (DP-1, HDMI-A-1) — what compositor configs are written against.
	// Empty when the compositor lacks xdg_output or advertises it
	// below version 2, in which case outputs are only identifiable by
	// registry order.
	Name, Description string

	// LogicalX/Y and LogicalW/H are the output's position and size in
	// the global compositor space, as xdg_output.logical_position and
	// logical_size report them.
	LogicalX, LogicalY, LogicalW, LogicalH int32

	// name is the registry global name, for hotplug removal.
	name uint32

	// xdg is the output's xdg_output object (see xdgoutput.go), nil
	// until the manager is bound or the object is created.
	xdg xdgOutputAPI
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

	registry            *wl.Registry
	compositor          *wl.Compositor
	shm                 *wl.Shm
	layerShell          *wlr.ZwlrLayerShellV1
	viewporter          *wlr.WpViewporter
	fracScaleManager    *wlr.WpScaleManagerV1
	seat                *wl.Seat
	pointer             *wl.Pointer
	keyboard            *wl.Keyboard
	wmBase              *xdg.WmBase
	compositorVersion   uint32
	outputs             []*Output
	hasArgb             bool
	globals             map[string]bool
	globalVersions      map[string]uint32
	ifaceNames          map[uint32]string
	mods                uint32
	repeatRate          uint32
	repeatDelay         uint32
	xkbKeymap           *xkb.Keymap
	xkbState            *xkb.State
	dataDeviceManager   *wl.DataDeviceManager
	dataDeviceVersion   uint32
	dataDevice          *wl.DataDevice
	keyboardSerial      uint32
	decorationManager   *deco.ZxdgDecorationManagerV1
	textInputMgr        *text.ZwpInputManagerV3
	textInput           *text.ZwpInputV3
	tiSerial            uint32 // commits sent on textInput; done events compare against it
	tiPending           tiPending
	primarySelectionMgr *wlr.ZwpPrimarySelectionDeviceManagerV1
	primarySelectionDev *wlr.ZwpPrimarySelectionDeviceV1

	// Shell-integration protocols (toplevel.go, activation.go,
	// idleinhibit.go, shortinhibit.go, xdgoutput.go), each seen
	// through a narrow interface so tests can substitute recorders;
	// nil on the no-protocol path.
	foreignToplevelMgr  foreignToplevelManagerAPI
	toplevels           []*Toplevel
	activation          activationAPI
	activationTokens    []*activationToken
	idleInhibitMgr      idleInhibitAPI
	shortcutsInhibitMgr shortcutsInhibitAPI
	xdgOutputMgr        xdgOutputMaker

	pointerEnterSerial uint32

	// crs is the cursor state (shape, animation timing, frame
	// timer); see cursor.go.
	crs cursorState

	// OnOutputAdded fires when a wl_output global appears, including
	// after Connect for hotplug; OnOutputRemoved fires when its global
	// goes away. Set them after Connect to drive per-output windows.
	OnOutputAdded   func(*Output)
	OnOutputRemoved func(*Output)

	// Pointer routing state. surfaceHandlers maps a wl_surface to the
	// handler that receives pointer events targeting it; pointerFocus
	// is the surface the compositor says the pointer is over, and
	// grabSurface holds implicit-grab routing while a button is down.
	surfaceHandlers map[*wl.Surface]SurfacePointerHandler
	dropHandlers    map[*wl.Surface]SurfaceDropHandler
	pointerFocus    *wl.Surface
	grabSurface     *wl.Surface

	// keyboardFocus is the surface the compositor gives keyboard input
	// to; keyboard events route through KeyboardFocus().
	keyboardFocus *wl.Surface

	// wheel120 accumulates axis_value120 units per axis (vertical,
	// horizontal) between pointer frames; the frame flushes them as
	// whole-notch scroll deltas.
	wheel120 [2]int32

	// protoErr carries the compositor's fatal wl_display.error, if one
	// arrived, so dispatch failures name the compositor's verdict
	// instead of a bare connection reset.
	protoErr error

	// OnKey fires on key presses (never releases) with the evdev
	// keycode and the held modifiers. Keyboard focus is seat-wide,
	// unlike pointer events which route per surface.
	OnKey func(keycode uint32, mods Mods)
	// OnKeyUp fires on key releases with the evdev keycode.
	OnKeyUp func(keycode uint32)
	// OnWmBasePing fires when the compositor pings liveness; reply
	// through Window.Pong.
	OnWmBasePing func(serial uint32)
	// OnIME fires when the input method applies a batch of changes (a
	// done event) — commit, preedit, and surrounding-text deletion in
	// protocol order. Nil without the text-input protocol, or when no
	// host consumes the events.
	OnIME func(IMEEvent)
	// OnIMEFocus fires when text-input focus enters or leaves a
	// surface; committed state is invalidated and must be re-pushed
	// for whichever surface the input method now targets.
	OnIMEFocus func()

	// OnToplevelAdded fires when the compositor advertises a foreign
	// toplevel — including our own windows; OnToplevelRemoved fires
	// when one closes, OnToplevelUpdated when a batch (title, app-id,
	// state) applied. Nil without the foreign-toplevel protocol or
	// when no host consumes the events.
	OnToplevelAdded   func(*Toplevel)
	OnToplevelRemoved func(*Toplevel)
	OnToplevelUpdated func(*Toplevel)

	// OnActivationToken fires when a token requested through
	// RequestActivationToken is issued; the string is what a spawned
	// app expects in XDG_ACTIVATION_TOKEN.
	OnActivationToken func(token string)

	// OnOutputIdentity fires when an output's stable xdg-output name
	// arrives or changes — the point where an `output "DP-1"` config
	// section can be matched to a hotplugged monitor.
	OnOutputIdentity func(*Output)
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
	// HandlePointerAxis reports scroll motion: dx from the horizontal
	// axis (tilt wheels, trackpads), dy from the vertical, both
	// positive right/down.
	HandlePointerAxis(dx, dy float64)
	// HandlePointerLeave reports the pointer leaving the surface.
	HandlePointerLeave()
}

// SurfaceDropHandler receives the wl_data_device drag-and-drop events
// targeting one registered surface, with coordinates in that surface's
// logical space. The offer travels with the enter event; its advertised
// mime types are collected by whoever tracks the offer object
// (internal/dragdrop), leaving the selection bookkeeping of
// internal/clipboard untouched.
type SurfaceDropHandler interface {
	// HandleDragEnter reports a drag entering the surface: position in
	// surface coordinates, the enter serial (the one accept replies
	// with), and the offered data; a nil offer means no transfer.
	HandleDragEnter(x, y float64, serial uint32, offer *wl.DataOffer)
	// HandleDragMotion reports drag movement within the surface.
	HandleDragMotion(x, y float64)
	// HandleDragLeave reports the drag leaving without a drop.
	HandleDragLeave()
	// HandleDrop reports the drop; the payload transfers on request
	// through the offer afterward.
	HandleDrop()
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
		globalVersions:  make(map[string]uint32),
		ifaceNames:      make(map[uint32]string),
		surfaceHandlers: make(map[*wl.Surface]SurfacePointerHandler),
		dropHandlers:    make(map[*wl.Surface]SurfaceDropHandler),
	}

	reg, err := d.GetRegistry()
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("wlsession: registry: %w", err)
	}
	s.registry = reg
	// Record the compositor's fatal protocol errors, so a killed
	// connection reports what the compositor objected to.
	d.AddErrorHandler(s)
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
	s.globalVersions[ev.Interface] = ev.Version
	s.ifaceNames[ev.Name] = ev.Interface

	switch ev.Interface {
	case "wl_compositor":
		s.compositorVersion = ev.Version
		s.compositor = wlclient.RegistryBindCompositorInterface(s.registry, ev.Name, bindVersion(ev.Version, 4))
	case "wl_shm":
		s.shm = wlclient.RegistryBindShmInterface(s.registry, ev.Name, 1)
		wlclient.ShmAddListener(s.shm, s)
	case "wl_output":
		wlo := wlclient.RegistryBindOutputInterface(s.registry, ev.Name, bindVersion(ev.Version, 2))
		out := &Output{WL: wlo, Scale: 1, name: ev.Name}
		wlclient.OutputAddListener(wlo, &outputEvents{sess: s, out: out})
		s.trackOutput(out)
		s.ensureXdgOutputs()
	case "zwlr_layer_shell_v1":
		ctx, _ := wl.GetUserData[wl.Context](s.registry)
		shell := wlr.NewZwlrLayerShellV1(ctx)
		_ = s.registry.Bind(ev.Name, ev.Interface, 1, shell)
		s.layerShell = shell
	case "wp_viewporter":
		ctx, _ := wl.GetUserData[wl.Context](s.registry)
		vp := wlr.NewWpViewporter(ctx)
		_ = s.registry.Bind(ev.Name, ev.Interface, bindVersion(ev.Version, 1), vp)
		s.viewporter = vp
	case "wp_fractional_scale_manager_v1":
		ctx, _ := wl.GetUserData[wl.Context](s.registry)
		mgr := wlr.NewWpScaleManagerV1(ctx)
		_ = s.registry.Bind(ev.Name, ev.Interface, bindVersion(ev.Version, 1), mgr)
		s.fracScaleManager = mgr
	case "wl_seat":
		s.seat = wlclient.RegistryBindSeatInterface(s.registry, ev.Name, bindVersion(ev.Version, 7))
		wlclient.SeatAddListener(s.seat, s)
		s.ensureDataDevice()
		s.ensureTextInput()
		s.ensurePrimarySelectionDevice()
	case "xdg_wm_base":
		ctx, _ := wl.GetUserData[wl.Context](s.registry)
		wmBase := xdg.NewShell(ctx)
		_ = s.registry.Bind(ev.Name, ev.Interface, bindVersion(ev.Version, 5), wmBase)
		xdg.WmBaseAddListener(wmBase, s)
		s.wmBase = wmBase
	case "wl_data_device_manager":
		ctx, _ := wl.GetUserData[wl.Context](s.registry)
		s.dataDeviceManager = wl.NewDataDeviceManager(ctx)
		s.dataDeviceVersion = bindVersion(ev.Version, minDataDeviceVersion)
		_ = s.registry.Bind(ev.Name, ev.Interface, s.dataDeviceVersion, s.dataDeviceManager)
		s.ensureDataDevice()
	case "zxdg_decoration_manager_v1":
		ctx, _ := wl.GetUserData[wl.Context](s.registry)
		s.decorationManager = deco.NewZxdgDecorationManagerV1(ctx)
		_ = s.registry.Bind(ev.Name, ev.Interface, bindVersion(ev.Version, 2), s.decorationManager)
	case "zwp_text_input_manager_v3":
		s.bindTextInputManager(ev)
	case "zwp_primary_selection_device_manager_v1":
		s.bindPrimarySelectionManager(ev)
	case "zwlr_foreign_toplevel_manager_v1":
		s.bindForeignToplevelManager(ev)
	case "xdg_activation_v1":
		s.bindActivation(ev)
	case "zwp_idle_inhibit_manager_v1":
		s.bindIdleInhibitManager(ev)
	case "zwp_keyboard_shortcuts_inhibit_manager_v1":
		s.bindShortcutsInhibitManager(ev)
	case "zxdg_output_manager_v1":
		s.bindXdgOutputManager(ev)
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
	// The session routes the drag-and-drop events per surface; the
	// selection and data_offer events go to internal/clipboard and
	// internal/dragdrop, which register their own handlers.
	dev.AddEnterHandler(s)
	dev.AddLeaveHandler(s)
	dev.AddMotionHandler(s)
	dev.AddDropHandler(s)
	s.dataDevice = dev
}

// HandleRegistryGlobalRemove implements wl.RegistryGlobalRemoveHandler:
// drop the global and, for outputs, run the hotplug hook so hosts on
// that output can tear themselves down.
func (s *Session) HandleRegistryGlobalRemove(ev wl.RegistryGlobalRemoveEvent) {
	iface, ok := s.ifaceNames[ev.Name]
	if !ok {
		return
	}
	if iface == "wl_output" {
		for i, out := range s.outputs {
			if out.name != ev.Name {
				continue
			}
			s.outputs = append(s.outputs[:i], s.outputs[i+1:]...)
			delete(s.ifaceNames, ev.Name)
			if s.OnOutputRemoved != nil {
				s.OnOutputRemoved(out)
			}
			return
		}
	}
	delete(s.globals, iface)
	delete(s.globalVersions, iface)
	delete(s.ifaceNames, ev.Name)
}

// trackOutput records a discovered output in arrival order and fires
// the hotplug hook. Separated from the registry bind so the
// bookkeeping is testable without a live connection.
func (s *Session) trackOutput(out *Output) {
	s.outputs = append(s.outputs, out)
	if s.OnOutputAdded != nil {
		s.OnOutputAdded(out)
	}
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

// HandleOutputGeometry implements wl.OutputGeometryHandler: the event's
// transform is what rotated outputs need surfaces to publish through
// wl_surface.set_buffer_transform.
func (e *outputEvents) HandleOutputGeometry(ev wl.OutputGeometryEvent) {
	e.out.Transform = ev.Transform
}

// HandleOutputMode implements wl.OutputModeHandler: the current mode
// (flag bit 0) records the output's pixel size, the fallback for layer
// surfaces with automatic axes.
func (e *outputEvents) HandleOutputMode(ev wl.OutputModeEvent) {
	if ev.Flags&1 != 0 {
		e.out.ModeW = int(ev.Width)
		e.out.ModeH = int(ev.Height)
	}
}

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
	s.crs.mu.Lock()
	s.pointerEnterSerial = 0
	s.crs.mu.Unlock()
	s.stopCursorAnim()
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

// SetSurfaceDrop registers h as the receiver of drag-and-drop events
// targeting surf; h == nil unregisters. It mirrors SetSurfaceInput for
// the wl_data_device half of the input model.
func (s *Session) SetSurfaceDrop(surf *wl.Surface, h SurfaceDropHandler) {
	if h == nil {
		delete(s.dropHandlers, surf)
		return
	}
	if s.dropHandlers == nil {
		s.dropHandlers = make(map[*wl.Surface]SurfaceDropHandler)
	}
	s.dropHandlers[surf] = h
}

// HandleDataDeviceEnter implements wl.DataDeviceEnterHandler: a drag
// entered a surface; route it to that surface's drop handler.
func (s *Session) HandleDataDeviceEnter(ev wl.DataDeviceEnterEvent) {
	if ev.Surface != nil {
		debug.Log("input", "wire dnd enter surf=%d (%.1f,%.1f) serial=%d",
			ev.Surface.Id(), ev.X, ev.Y, ev.Serial)
	}
	if h := s.dropHandlers[ev.Surface]; h != nil {
		h.HandleDragEnter(float64(ev.X), float64(ev.Y), ev.Serial, ev.Id)
	}
}

// HandleDataDeviceLeave implements wl.DataDeviceLeaveHandler: the drag
// ended over every surface. The event names no surface, so every
// registered handler hears it; only the one holding a drag acts.
func (s *Session) HandleDataDeviceLeave(wl.DataDeviceLeaveEvent) {
	debug.Log("input", "wire dnd leave")
	for _, h := range s.dropHandlers {
		h.HandleDragLeave()
	}
}

// HandleDataDeviceMotion implements wl.DataDeviceMotionHandler.
func (s *Session) HandleDataDeviceMotion(ev wl.DataDeviceMotionEvent) {
	for _, h := range s.dropHandlers {
		h.HandleDragMotion(float64(ev.X), float64(ev.Y))
	}
	debug.Log("input", "wire dnd motion (%.1f,%.1f)", ev.X, ev.Y)
}

// HandleDataDeviceDrop implements wl.DataDeviceDropHandler.
func (s *Session) HandleDataDeviceDrop(wl.DataDeviceDropEvent) {
	debug.Log("input", "wire dnd drop")
	for _, h := range s.dropHandlers {
		h.HandleDrop()
	}
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
	s.crs.mu.Lock()
	s.pointerEnterSerial = ev.Serial
	s.crs.mu.Unlock()
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
	grabbed := s.grabSurface != nil
	s.grabSurface = nil
	if s.pointerFocus == ev.Surface {
		s.pointerFocus = nil
	}
	// Restore the default shape unless an implicit grab keeps the
	// pointer: a drag (slider, resize) must not see its cursor flip
	// to the arrow just because the pointer crossed the surface edge.
	// Shapes stick across surfaces of one client, so without the
	// restore the next surface would inherit whatever the old one
	// showed.
	if !grabbed {
		_ = s.restoreCursor()
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

// Axis orientation enum values on the wire.
const (
	axisVertical   = 0
	axisHorizontal = 1
)

// wheelPerStep converts axis_value120 units to the smooth-axis scale
// the app treats as one scroll step (10): one wheel click is 120 units.
const wheelPerStep = 12.0

// HandlePointerAxis implements wl.PointerAxisHandler: smooth scroll
// motion routed like motion, so scrolling follows the grab while
// dragging. Discrete wheel clicks arrive separately through
// axis_value120 and are flushed at the frame.
func (s *Session) HandlePointerAxis(ev wl.PointerAxisEvent) {
	dx, dy := 0.0, 0.0
	if ev.Axis == axisVertical {
		dy = float64(ev.Value)
	} else {
		dx = float64(ev.Value)
	}
	if h := s.pointerTarget(); h != nil {
		h.HandlePointerAxis(dx, dy)
	}
	debug.Log("input", "wire axis dx=%.1f dy=%.1f", dx, dy)
}

// HandlePointerFrame implements wl.PointerFrameHandler: flushes
// accumulated axis_value120 wheel clicks to the pointer target as
// whole-notch scroll deltas. Multiple clicks inside one frame batch
// here, which is the wheel acceleration multiplicity.
func (s *Session) HandlePointerFrame(wl.PointerFrameEvent) {
	dv, dh := s.wheel120[0], s.wheel120[1]
	s.wheel120[0], s.wheel120[1] = 0, 0
	if dv == 0 && dh == 0 {
		return
	}
	if h := s.pointerTarget(); h != nil {
		h.HandlePointerAxis(float64(dh)/wheelPerStep, float64(-dv)/wheelPerStep)
	}
}

// HandlePointerAxisSource implements wl.PointerAxisSourceHandler.
func (s *Session) HandlePointerAxisSource(wl.PointerAxisSourceEvent) {}

// HandlePointerAxisStop implements wl.PointerAxisStopHandler.
func (s *Session) HandlePointerAxisStop(wl.PointerAxisStopEvent) {}

// HandlePointerAxisDiscrete implements wl.PointerAxisDiscreteHandler.
func (s *Session) HandlePointerAxisDiscrete(wl.PointerAxisDiscreteEvent) {}

// HandlePointerAxisValue120 implements wl.PointerAxisValue120Handler:
// accumulates wheel click counts per axis; one click is 120 units.
func (s *Session) HandlePointerAxisValue120(ev wl.PointerAxisValue120Event) {
	s.wheel120[min(ev.Axis, 1)] += ev.Value120
}

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

// HandleKeyboardEnter implements wl.KeyboardEnterHandler: the named
// surface receives keyboard input until a leave or another enter.
func (s *Session) HandleKeyboardEnter(ev wl.KeyboardEnterEvent) {
	s.keyboardFocus = ev.Surface
	s.keyboardSerial = ev.Serial
}

// HandleKeyboardLeave implements wl.KeyboardLeaveHandler.
func (s *Session) HandleKeyboardLeave(wl.KeyboardLeaveEvent) {
	s.keyboardFocus = nil
}

// KeyboardFocus returns the surface currently holding keyboard input,
// or nil when the compositor gives the seat no keyboard focus.
func (s *Session) KeyboardFocus() *wl.Surface { return s.keyboardFocus }

// HandleKeyboardKey implements wl.KeyboardKeyHandler.
func (s *Session) HandleKeyboardKey(ev wl.KeyboardKeyEvent) {
	debug.Log("input", "wire key code=%d state=%d", ev.Key, ev.State)
	s.keyboardSerial = ev.Serial
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
	s.keyboardSerial = ev.Serial
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

// Viewporter returns the bound wp_viewporter, or nil when the
// compositor does not provide it; fractional scaling degrades to
// integer set_buffer_scale without it.
func (s *Session) Viewporter() *wlr.WpViewporter { return s.viewporter }

// FractionalScaleManager returns the bound
// wp_fractional_scale_manager_v1, or nil when the compositor does not
// provide it; without it there are no preferred_scale events and
// surfaces keep their integer scale.
func (s *Session) FractionalScaleManager() *wlr.WpScaleManagerV1 { return s.fracScaleManager }

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

// DataDeviceVersion returns the negotiated wl_data_device_manager
// version: three and above have the dnd action requests and events
// (set_actions, finish), below that drags run in the v1 subset.
func (s *Session) DataDeviceVersion() uint32 { return s.dataDeviceVersion }

// PrimarySelectionManager returns the bound primary selection device
// manager, or nil when the compositor does not provide it; primary
// selection support needs it.
func (s *Session) PrimarySelectionManager() *wlr.ZwpPrimarySelectionDeviceManagerV1 {
	return s.primarySelectionMgr
}

// KeyboardSerial returns the serial of the most recent keyboard event
// (enter, key or modifiers), needed by selection requests.
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
	// done is a destructor event: once it lands (or the wait fails) the
	// callback is dead on both sides, so its id must rejoin the
	// client's pool. Skipping this leaks a proxy per roundtrip - and
	// some loops roundtrip per frame.
	defer cb.Unregister()
	err = s.Display.Context().RunTill(cb)
	for errors.Is(err, wl.ErrContextRunProxyNil) {
		err = s.Display.Context().RunTill(cb)
	}
	return s.withProtoErr(err)
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
	return s.withProtoErr(err)
}

// HandleDisplayError implements wl.DisplayErrorHandler: record the
// compositor's fatal protocol error so the dispatch failure that ends
// the run names the compositor's verdict, and trace it. Registered in
// Connect; a closed connection without an error event leaves protoErr
// nil, which is itself diagnostic (the compositor dropped us without
// saying why).
func (s *Session) HandleDisplayError(ev wl.DisplayErrorEvent) {
	s.protoErr = fmt.Errorf("object %T: %s", ev.ObjectId, ev.Message)
	debug.Log("wire", "compositor fatal: %v", s.protoErr)
}

// withProtoErr decorates a dispatch failure with the recorded protocol
// error, if any.
func (s *Session) withProtoErr(err error) error {
	if err != nil && s.protoErr != nil {
		return fmt.Errorf("%w (compositor: %w)", err, s.protoErr)
	}
	return err
}

// kickHandler unregisters its sync callback once the wakeup fired, so
// parked-loop kicks do not leak proxies. (wlclient.CallbackDestroy is a
// no-op in the binding; Unregister is the real teardown - done is a
// destructor event, so no wire request is needed.)
type kickHandler struct{ cb *wl.Callback }

// HandleCallbackDone implements wl.CallbackDoneHandler.
func (k kickHandler) HandleCallbackDone(wl.CallbackDoneEvent) {
	k.cb.Unregister()
}

// WakeAfter arranges for a loop parked in Step to return by waiting d:
// a timer sends a wl_display.sync, and its done event ends the blocking
// read. If the event is dispatched before the listener attaches, the
// callback's registration leaks - the wake itself is unaffected because
// dispatching the event is what ends Step.
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

// Close disconnects from the display and releases the session's shared
// buffer arena: the pool proxy, mapping, and the session's one fd.
func (s *Session) Close() {
	s.stopCursorAnim()
	buffer.CloseArenas(s.shm)
	if s.Display != nil {
		_ = s.Display.Context().Close()
	}
}
