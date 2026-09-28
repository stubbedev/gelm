package headlesstest

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"

	"github.com/stubbedev/gelm/wlr"
)

// usKeymap is a flattened US xkb keymap (see testdata/us.xkb), the
// keymap uploaded to the virtual keyboard when the seat has none of
// its own - the protocol refuses key events before a keymap is set.
//
//go:embed testdata/us.xkb
var usKeymap string

// Pointer button codes (linux/input-event-codes.h), the values
// wl_pointer.button carries.
const (
	BTNLeft  = 0x110
	BTNRight = 0x111
)

// evdev key codes the suite drives (linux/input-event-codes.h). The
// compositor maps them through the seat's keymap when it has one; the
// client under test decodes keycodes with its built-in US fallback
// regardless, so codes behave as they would from a physical US
// keyboard.
const (
	KeyEscape = 1
	KeyTab    = 15
	KeyEnter  = 28
	KeyA      = 30
	KeyD      = 32
	KeyC      = 46
	KeyV      = 47
	KeySpace  = 57
	KeyHome   = 102
	KeyLeft   = 105
	KeyRight  = 106
	KeyEnd    = 107
	KeyDown   = 108
)

// settleTimes mirror wlpointer's pacing: after each action a roundtrip
// plus a short sleep lets the compositor deliver the event and the
// client react before the next step (assertions still wait on traces,
// so the pacing only orders actions, it does not carry correctness).
const (
	settleDelay = 80 * time.Millisecond
	pressDelay  = 60 * time.Millisecond
)

// VirtualInput is a connected synthetic-seat client: it creates a
// zwlr_virtual_pointer_v1 and a zwp_virtual_keyboard_v1 and drives
// them. One instance per test keeps focus, grab and keymap state from
// bleeding between tests.
type VirtualInput struct {
	d   *wl.Display
	ctx *wl.Context

	seat *wl.Seat
	out  *wl.Output

	vpMgr *wlr.ZwlrVirtualPointerManagerV1
	vp    *wlr.ZwlrVirtualPointerV1
	vkMgr *wlr.ZwpKeyboardManagerV1
	vk    *wlr.ZwpKeyboardV1

	modeW, modeH int32
	keymapBytes  uint32
	// keymapErr records a failed keymap arm: without a keymap the
	// compositor drops every key silently.
	keymapErr error
	// protoErr records the compositor's fatal protocol error, if any,
	// so Dial failures say what the compositor objected to.
	protoErr error
}

// Dial connects to the compositor socket named by WAYLAND_DISPLAY in
// the process's XDG_RUNTIME_DIR, binds the seat and the virtual-input
// managers, and creates a pointer and a keyboard. If the seat offers a
// keymap it is relayed to the virtual keyboard; when it does not (a
// virtual-only seat may never have compiled one), keys are injected
// raw and modifiers are pushed explicitly through
// zwp_virtual_keyboard_v1.modifiers - the client under test decodes
// keycodes with its own US fallback either way.
func Dial(display string) (*VirtualInput, error) {
	return dial(display, true)
}

// DialPointerOnly connects with a virtual pointer and no keyboard: the
// shape of a seat whose input devices are pointers only. Widgets that
// open surfaces on hover (tooltips, popups) must survive and behave
// there exactly as with a keyboard present.
func DialPointerOnly(display string) (*VirtualInput, error) {
	return dial(display, false)
}

// dial is the shared connection path; keyboard arms the virtual
// keyboard and waits for the seat's keyboard capability.
func dial(display string, keyboard bool) (*VirtualInput, error) {
	d, err := wl.Connect(display)
	if err != nil {
		return nil, err
	}
	v := &VirtualInput{d: d}
	reg, err := d.GetRegistry()
	if err != nil {
		_ = d.Context().Close()
		return nil, err
	}
	v.ctx, _ = wl.GetUserData[wl.Context](reg)
	d.AddErrorHandler(v)
	g := &regState{v: v}
	wlclient.RegistryAddListener(reg, g)
	if err := v.roundtrip(); err != nil {
		_ = d.Context().Close()
		return nil, fmt.Errorf("registry roundtrip: %w", err)
	}
	if g.seatName == 0 || g.vpName == 0 || g.vkmName == 0 || g.outName == 0 {
		_ = d.Context().Close()
		return nil, errors.New("headlesstest: compositor lacks wl_seat, wl_output, zwlr_virtual_pointer_manager_v1 or zwp_virtual_keyboard_manager_v1")
	}

	v.seat = wlclient.RegistryBindSeatInterface(reg, g.seatName, 5)
	wlclient.SeatAddListener(v.seat, g)
	v.out = wlclient.RegistryBindOutputInterface(reg, g.outName, 2)
	wlclient.OutputAddListener(v.out, g)
	v.vpMgr = wlr.NewZwlrVirtualPointerManagerV1(v.ctx)
	if err := reg.Bind(g.vpName, "zwlr_virtual_pointer_manager_v1", 1, v.vpMgr); err != nil {
		_ = d.Context().Close()
		return nil, err
	}
	v.vkMgr = nil
	if keyboard {
		v.vkMgr = wlr.NewZwpKeyboardManagerV1(v.ctx)
		if err := reg.Bind(g.vkmName, "zwp_virtual_keyboard_manager_v1", 1, v.vkMgr); err != nil {
			_ = d.Context().Close()
			return nil, err
		}
	}

	// The keyboard capability only exists once a keyboard device does,
	// so create both devices first, then bind wl_keyboard and relay the
	// keymap event to the virtual keyboard.
	vp, err := v.vpMgr.CreateVirtualPointer(v.seat)
	if err != nil {
		_ = d.Context().Close()
		return nil, err
	}
	v.vp = vp
	if !keyboard {
		return v, nil
	}
	vk, err := v.vkMgr.CreateKeyboard(v.seat)
	if err != nil {
		_ = d.Context().Close()
		return nil, err
	}
	v.vk = vk
	// The seat's keyboard capability propagates asynchronously after the
	// device is created; get_keyboard before the capabilities event
	// lands is a missing_capability protocol error that kills the
	// connection. Wait until the seat itself reports the capability.
	deadline := time.Now().Add(5 * time.Second)
	for g.capabilities&capKeyboard == 0 {
		if err := v.roundtrip(); err != nil {
			_ = d.Context().Close()
			return nil, fmt.Errorf("capability roundtrip: %w (protocol error: %w)", err, v.protoErr)
		}
		if time.Now().After(deadline) {
			_ = d.Context().Close()
			return nil, errors.New("headlesstest: seat never reported the keyboard capability after the virtual keyboard was created")
		}
		time.Sleep(20 * time.Millisecond)
	}

	kb, err := v.seat.GetKeyboard()
	if err != nil {
		_ = d.Context().Close()
		return nil, err
	}
	wlclient.KeyboardAddListener(kb, g)
	// Give the seat's keymap a few dispatches to arrive. A virtual-only
	// seat never has one and announces format 0 (skipped by the relay
	// handler), so fall back to uploading the embedded US keymap - the
	// protocol refuses key events before any keymap is set.
	for range 5 {
		if err := v.roundtrip(); err != nil {
			_ = d.Context().Close()
			return nil, fmt.Errorf("keymap roundtrip: %w (protocol error: %w)", err, v.drain())
		}
		if v.keymapBytes != 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if v.keymapBytes == 0 {
		if err := v.uploadKeymap([]byte(usKeymap)); err != nil {
			_ = d.Context().Close()
			return nil, fmt.Errorf("upload keymap: %w", err)
		}
		if err := v.roundtrip(); err != nil {
			_ = d.Context().Close()
			return nil, fmt.Errorf("keymap install roundtrip: %w (protocol error: %w)", err, v.drain())
		}
	}
	if v.keymapErr != nil {
		_ = d.Context().Close()
		return nil, fmt.Errorf("headlesstest: arm virtual keyboard keymap: %w", v.keymapErr)
	}
	return v, nil
}

// uploadKeymap arms the virtual keyboard with the given xkb text via
// a temp file descriptor (format 1, xkb_v1). An empty keymap skips the
// request entirely.
func (v *VirtualInput) uploadKeymap(keymap []byte) error {
	if len(keymap) == 0 {
		return nil
	}
	f, err := os.CreateTemp("", "gelm-headless-keymap-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(keymap); err != nil {
		_ = f.Close()
		return err
	}
	off, err := f.Seek(0, 0)
	if err != nil {
		_ = f.Close()
		return err
	}
	if off != 0 {
		_ = f.Close()
		return errors.New("headlesstest: temp keymap did not rewind to 0")
	}
	fd := f.Fd()
	defer func() { _ = f.Close() }()
	return v.vk.Keymap(1, fd, uint32(len(keymap)))
}

// regState collects the globals as they stream past and carries the
// listener plumbing for the bound seat, output and keyboard.
type regState struct {
	v *VirtualInput

	seatName, outName, vpName, vkmName uint32
	capabilities                       uint32
}

// capKeyboard is wl_seat.capabilities' keyboard bit.
const capKeyboard = 2

// HandleSeatCapabilities implements wl.SeatCapabilitiesHandler: the
// keyboard capability appears only after a keyboard device exists.
func (g *regState) HandleSeatCapabilities(ev wl.SeatCapabilitiesEvent) {
	g.capabilities = ev.Capabilities
}

func (g *regState) HandleSeatName(wl.SeatNameEvent) {}

func (g *regState) HandleRegistryGlobal(ev wl.RegistryGlobalEvent) {
	switch ev.Interface {
	case "wl_seat":
		g.seatName = ev.Name
	case "wl_output":
		g.outName = ev.Name
	case "zwlr_virtual_pointer_manager_v1":
		g.vpName = ev.Name
	case "zwp_virtual_keyboard_manager_v1":
		g.vkmName = ev.Name
	}
}

func (g *regState) HandleRegistryGlobalRemove(wl.RegistryGlobalRemoveEvent) {}

func (g *regState) HandleOutputGeometry(wl.OutputGeometryEvent) {}

// HandleOutputMode tracks the current mode (flag bit 0): absolute
// pointer motion is normalized against the output size.
func (g *regState) HandleOutputMode(ev wl.OutputModeEvent) {
	if ev.Flags&1 != 0 {
		g.v.modeW, g.v.modeH = ev.Width, ev.Height
	}
}

func (g *regState) HandleOutputDone(wl.OutputDoneEvent)   {}
func (g *regState) HandleOutputScale(wl.OutputScaleEvent) {}

// HandleKeyboardKeymap relays the seat's keymap to the virtual
// keyboard. The fd we received belongs to this connection's event
// loop, so dup it for the re-send (the wire layer closes what it
// sends). A format of 0 is NO_KEYMAP - a virtual-only seat announces
// one with a stub fd - and relaying that mmaps zero bytes compositor
// side, which kills the connection; skip it and drive keys raw.
func (g *regState) HandleKeyboardKeymap(ev wl.KeyboardKeymapEvent) {
	if ev.Format == 0 || ev.Size == 0 || ev.FdError != nil || ev.Fd == 0 || g.v.vk == nil {
		return
	}
	dup, err := syscall.Dup(int(ev.Fd))
	if err != nil {
		g.v.keymapErr = err
		return
	}
	if err := g.v.vk.Keymap(ev.Format, uintptr(dup), ev.Size); err != nil {
		// Without an armed keymap the compositor silently drops every
		// key, so a failed arm must not pass for success.
		g.v.keymapErr = err
		return
	}
	g.v.keymapBytes = ev.Size
}

func (g *regState) HandleKeyboardEnter(wl.KeyboardEnterEvent)           {}
func (g *regState) HandleKeyboardLeave(wl.KeyboardLeaveEvent)           {}
func (g *regState) HandleKeyboardKey(wl.KeyboardKeyEvent)               {}
func (g *regState) HandleKeyboardModifiers(wl.KeyboardModifiersEvent)   {}
func (g *regState) HandleKeyboardRepeatInfo(wl.KeyboardRepeatInfoEvent) {}

// HandleDisplayError implements wl.DisplayErrorHandler: remember the
// compositor's fatal protocol error so Dial failures name the cause.
func (v *VirtualInput) HandleDisplayError(ev wl.DisplayErrorEvent) {
	v.protoErr = fmt.Errorf("object %T: %s", ev.ObjectId, ev.Message)
}

// drain dispatches whatever is still buffered (the compositor's
// closing wl_display.error usually is) and returns the recorded
// protocol error for error messages.
func (v *VirtualInput) drain() error {
	for range 50 {
		if err := v.d.Context().Run(); err != nil {
			break
		}
	}
	return v.protoErr
}

// roundtrip dispatches until a sync callback completes, retrying past
// proxies destroyed mid-queue the way the session and wlpointer do.
func (v *VirtualInput) roundtrip() error {
	cb, err := v.d.Sync()
	if err != nil {
		return err
	}
	defer wlclient.CallbackDestroy(cb)
	for {
		err = v.d.Context().RunTill(cb)
		if err == nil {
			return nil
		}
		if err.Error() != "proxy nil" {
			return err
		}
	}
}

func (v *VirtualInput) now() uint32 { return uint32(time.Now().UnixNano() / 1e6) }

func (v *VirtualInput) settle() {
	_ = v.roundtrip()
	time.Sleep(settleDelay)
}

// MoveTo moves the pointer to compositor coordinates (x, y).
func (v *VirtualInput) MoveTo(x, y int) error {
	if err := v.vp.MotionAbsolute(v.now(), uint32(x), uint32(y), uint32(v.modeW), uint32(v.modeH)); err != nil {
		return err
	}
	if err := v.vp.Frame(); err != nil {
		return err
	}
	v.settle()
	return nil
}

// ClickAt presses and releases button at (x, y).
func (v *VirtualInput) ClickAt(x, y int, button uint32) error {
	if err := v.MoveTo(x, y); err != nil {
		return err
	}
	if err := v.vp.Button(v.now(), button, 1); err != nil {
		return err
	}
	_ = v.vp.Frame()
	if err := v.roundtrip(); err != nil {
		return err
	}
	time.Sleep(pressDelay)
	if err := v.vp.Button(v.now(), button, 0); err != nil {
		return err
	}
	_ = v.vp.Frame()
	if err := v.roundtrip(); err != nil {
		return err
	}
	time.Sleep(settleDelay)
	return nil
}

// DragTo presses at (x0, y0), moves in steps to (x1, y1), and
// releases there: the gesture drag-driven widgets need (a plain click
// neither moves a slider nor drags a scrollbar).
func (v *VirtualInput) DragTo(x0, y0, x1, y1 int, steps int) error {
	if err := v.MoveTo(x0, y0); err != nil {
		return err
	}
	if err := v.vp.Button(v.now(), BTNLeft, 1); err != nil {
		return err
	}
	_ = v.vp.Frame()
	if err := v.roundtrip(); err != nil {
		return err
	}
	time.Sleep(pressDelay)
	if steps < 1 {
		steps = 1
	}
	for i := 1; i <= steps; i++ {
		x := x0 + (x1-x0)*i/steps
		y := y0 + (y1-y0)*i/steps
		if err := v.MoveTo(x, y); err != nil {
			return err
		}
	}
	if err := v.vp.Button(v.now(), BTNLeft, 0); err != nil {
		return err
	}
	_ = v.vp.Frame()
	if err := v.roundtrip(); err != nil {
		return err
	}
	time.Sleep(settleDelay)
	return nil
}

// ScrollAt sends a wheel tick at (x, y); dy is in wayland axis units,
// positive down.
func (v *VirtualInput) ScrollAt(x, y int, dy float64) error {
	if err := v.MoveTo(x, y); err != nil {
		return err
	}
	if err := v.vp.Axis(v.now(), 0, float32(dy)); err != nil {
		return err
	}
	if err := v.vp.Frame(); err != nil {
		return err
	}
	v.settle()
	return nil
}

// Tap presses and releases one key.
func (v *VirtualInput) Tap(code uint32) error {
	if err := v.vk.Key(v.now(), code, 1); err != nil {
		return err
	}
	if err := v.roundtrip(); err != nil {
		return err
	}
	time.Sleep(pressDelay)
	if err := v.vk.Key(v.now(), code, 0); err != nil {
		return err
	}
	return v.roundtrip()
}

// TypeText types s: lowercase letters, digits and spaces. Anything
// else is a harness bug (a test asking for a key the table does not
// know), so it errors instead of guessing.
func (v *VirtualInput) TypeText(s string) error {
	for _, r := range s {
		code, shift, err := KeyFor(r)
		if err != nil {
			return err
		}
		if shift {
			if err := v.mods(ModShift); err != nil {
				return err
			}
		}
		if err := v.Tap(code); err != nil {
			return err
		}
		if shift {
			if err := v.mods(0); err != nil {
				return err
			}
		}
	}
	return v.roundtrip()
}

// xkb modifier masks, the values wl_keyboard.modifiers and
// zwp_virtual_keyboard_v1.modifiers exchange (Shift, Lock, Control,
// Alt from bit 0).
const (
	ModShift uint32 = 1 << 0
	ModCtrl  uint32 = 1 << 2
)

// mods pushes an explicit modifier state: on a seat without a keymap
// the compositor cannot derive modifiers from key presses, so the
// virtual keyboard protocol lets us declare them.
func (v *VirtualInput) mods(depressed uint32) error {
	if err := v.vk.Modifiers(depressed, 0, 0, 0); err != nil {
		return err
	}
	return v.roundtrip()
}

// Combo holds the ctrl modifier, taps key, releases it: ctrl+c and
// friends.
func (v *VirtualInput) Combo(code uint32) error {
	if err := v.mods(ModCtrl); err != nil {
		return err
	}
	if err := v.Tap(code); err != nil {
		return err
	}
	if err := v.mods(0); err != nil {
		return err
	}
	v.settle()
	return nil
}

// Close destroys the virtual devices and disconnects.
func (v *VirtualInput) Close() {
	if v.vk != nil {
		_ = v.vk.Destroy()
	}
	if v.vp != nil {
		_ = v.vp.Destroy()
	}
	if v.vpMgr != nil {
		_ = v.vpMgr.Destroy()
	}
	_ = v.d.Context().Close()
}
