// Package headlesstest is gelm's compositor-in-the-loop test harness:
// it attaches to a private headless compositor session (booted by the
// justfile's test-env recipe: sway on the wlroots headless backend
// with the pixman renderer, or Hyprland on its own DRM node inside
// the tests/hyprland-vm.nix VM - GELM_TEST_COMPOSITOR picks the
// driver), launches a gelm client under test inside it, drives a
// synthetic seat through zwlr_virtual_pointer_v1 and
// zwp_virtual_keyboard_v1, and asserts on the client's observable
// behavior through its GOELM_DEBUG trace log.
//
// The package has two kinds of users. The unit-testable pieces (trace
// parsing, keymap table, compositor drivers) are plain exported
// functions and run everywhere. The compositor-in-the-loop tests live
// in this package's _test files and are gated by the GELM_HEADLESS
// environment variable:
//
//   - GELM_HEADLESS unset or empty: the integration tests skip, so a
//     plain `go test ./...` on a machine with no compositor passes.
//   - GELM_HEADLESS set (CI, or `just headless`): the tests are
//     required. The harness attaches to the running compositor via
//     WAYLAND_DISPLAY/XDG_RUNTIME_DIR; if the env is not there, the
//     run fails - a skip under GELM_HEADLESS would be a silent hole.
//
// The session is isolated from the developer's desktop: the
// compositor runs against its own runtime dir, and the recipe's
// teardown removes it after the run.
package headlesstest

// AppID is the Wayland app_id of the showcase client under test. The
// test-env recipe's sway config pins it to a floating 640x470 window
// at the output's origin so widget coordinates traced by the client
// are compositor coordinates.
const AppID = "dev.stubbe.gelm.hello"

// showcaseW and showcaseH mirror the window size the showcase requests
// (cmd/gelm-hello); the sway rule in the recipe forces the same
// numbers, so a compositor that ignores the request fails the size
// assertion instead of silently shifting every click.
const (
	showcaseW = 640
	showcaseH = 470
)

// StatesAppID is the Wayland app_id of the window-state client
// (cmd/gelm-states). The recipe's sway rule pins it to a floating
// 420x280 window, so the confirmed maximize/fullscreen configure sizes
// are deterministic: the 1280x800 output mode for the state-sized
// configure, the pinned 420x280 for the restored one.
const StatesAppID = "dev.stubbe.gelm.states"

// statesW and statesH mirror the states client's requested (and
// recipe-pinned) floating size; outputW and outputH mirror the
// recipe's output mode.
const (
	statesW = 420
	statesH = 280
	outputW = 1280
	outputH = 800
)

// MultilistAppID is the Wayland app_id of the multi-select list
// client (cmd/gelm-multilist). The recipe's sway rule pins it to a
// floating window beside the showcase, so the row centers the client
// traces are compositor coordinates.
const MultilistAppID = "dev.stubbe.gelm.multilist"

// multilistW and multilistH mirror the multilist client's requested
// (and recipe-pinned) floating size; multilistRows mirrors its row
// count (cmd/gelm-multilist).
const (
	multilistW    = 300
	multilistH    = 300
	multilistRows = 10
)
