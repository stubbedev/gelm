// Package wlsession owns the wl_display connection: registry discovery,
// globals binding, output scale tracking, and the blocking event dispatch.
package wlsession

import (
	"errors"
	"fmt"

	"github.com/neurlang/wayland/wl"
	"github.com/neurlang/wayland/wlclient"
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

// Session is a connected display with the globals gelm needs bound.
type Session struct {
	Display *wl.Display

	registry          *wl.Registry
	compositor        *wl.Compositor
	shm               *wl.Shm
	layerShell        *wlr.ZwlrLayerShellV1
	compositorVersion uint32
	outputs           []*Output
	hasArgb           bool
	globals           map[string]bool
	ifaceNames        map[uint32]string
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
		return nil, fmt.Errorf("wlsession: compositor lacks ARGB8888 wl_shm support")
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
	}
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

// Compositor returns the bound wl_compositor.
func (s *Session) Compositor() *wl.Compositor { return s.compositor }

// Shm returns the bound wl_shm.
func (s *Session) Shm() *wl.Shm { return s.shm }

// LayerShell returns the bound zwlr_layer_shell_v1.
func (s *Session) LayerShell() *wlr.ZwlrLayerShellV1 { return s.layerShell }

// Outputs returns the bound outputs in registry order.
func (s *Session) Outputs() []*Output { return s.outputs }

// Roundtrip issues a display sync and dispatches until it completes.
func (s *Session) Roundtrip() error {
	cb, err := s.Display.Sync()
	if err != nil {
		return err
	}
	return s.Display.Context().RunTill(cb)
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
		s.Display.Context().Close()
	}
}
