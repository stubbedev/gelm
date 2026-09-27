# gelm

A retained-mode widget kit for Wayland, written in pure Go (no cgo).
Logical-coordinate widget space, damage-tracked repaint, a parked event
loop that idles at 0% CPU, and a renderer built on premultiplied-alpha
ARGB — designed to carry a future Go wayle; see
[stubbedev/wayle#19](https://github.com/stubbedev/wayle/issues/19) for
the full plan and the completeness ladder.

## Status

The toolkit core works end to end on real compositors (verified on
Hyprland and headless sway): toplevel and layer-shell windows, the full
widget set below, keyboard and pointer input with a defined
[input model](docs/input-model.md), the clipboard, drag and drop, IME
composition, fractional scaling with live rescale, and an animation
clock. Theming is a composable palette value (`widget.SetTheme`,
`widget.DarkTheme().WithAccentHex("#a6e3a1").WithPadding(8)`-style
chaining, dark and light presets); state shades derive from the
palette and there is no CSS engine and deliberately no per-widget
theme overrides (see [docs/architecture.md](docs/architecture.md),
"Theming").

## Feature matrix

A capability-by-capability audit against relm4/GTK — including the
deliberate non-goals and the honest, deferred gaps — lives in
[docs/completeness.md](docs/completeness.md).

### Widgets

| Widget | Status | Notes and caveats |
| --- | --- | --- |
| Label | done | shaped text, alignment, `SetText`; wrap and ellipsize helpers live on the typeface |
| Button | done | any child widget; hover/pressed states; Enter + Space activate |
| Slider | done | drag or arrows/Home/End, clamped steps |
| Switch, CheckButton | done | keyboard toggles on Enter and Space |
| ProgressBar | done | animated by the app through `anim` tweens |
| Entry | done | single-line; selection (shift motion, drag, double-click word, select-all); clipboard; IME preedit display |
| TextArea | done | multi-line; soft wrap on by default (logical-line editing model); plain Tab indents, ctrl/shift+Tab traverse |
| Scroll | done | both axes; draggable bars, gutter paging, auto-hide fade, fill/center stretch |
| List | done | virtualized model rows (a viewport's worth of widgets); **single-select only** |
| Notebook | done | tabs with close hook; ctrl+PageUp/PageDown cycles; hidden pages skipped by focus |
| Menu | done | check/radio rows, separators, nested submenus; accelerators are display-only labels (no mnemonics) |
| Dropdown, `DropdownOf[T]` | done | combobox: face plus an inline themed item list (a `Children` child only while open, not an `app.Popover`); Enter/Space/Down opens, arrows navigate, Esc cancels; no type-ahead |
| Popover (`app.Popover`) | done | anchored to any widget, flips inside the host; works on layer surfaces too |
| Dialog, MessageBox (`app`) | done | parented toplevels; **application-level** modality (xdg_shell has no modal bit); Esc/Enter responses |
| Icon | done | raster, theme-name, file, and embedded-SVG constructors; symbolic sources follow the accent or a pinned tint |
| Box, Stack, Overlay, Scroll | done | the containers; row/column layout with expanding children |
| Tooltips | done | `SetTooltip` on any widget, 500ms dwell; toplevel hosts only |
| Drag source / drop target | done | `widget.DragSource` + `DragEnterer` per widget; mime negotiation, highlight, cross-window drops |

### Protocols and input

| Piece | Status | Notes and caveats |
| --- | --- | --- |
| xdg-shell | done | toplevels: title/app-id, min/max, close-request veto, ping/pong, interactive resize edges (6 logical px), xdg_popup menus and popovers |
| wlr-layer-shell | done | layer, anchors, margins, exclusive zone, keyboard interactivity; popups via `get_popup` |
| xdg-decoration | optional | server-side decorations when the compositor decorates; silently skipped otherwise (decorated windows opt out of client resize edges) |
| wl_data_device | done | clipboard (ctrl+c/x/v) and drag and drop, including cross-window; `set_actions`/`finish` gated on data-device v3 |
| xkb keyboard | done | the compositor's keymap (alt layouts, AltGr, dead keys); synthesized key repeat at the compositor's rate/delay |
| zwp_text_input_v3 | optional | IME composition into Entry and TextArea; no-op when the compositor lacks the global |
| wp_viewporter + wp_fractional_scale_v1 | optional | fractional scale (1.25 and friends) with **live rescale**; integer `set_buffer_scale` fallback |
| xdg_toplevel resize/min/max | done | interactive edges, published limits, configure relayout in the same frame |
| Cursors | done | xcursor theme loading, animated cursors, per-widget shapes, resize aliases, leave restoration |
| wl_shm buffers | done | one arena per session: a single fd and mapping, buffers as sub-allocations; busy buffers are never overwritten |

### Engine

| Piece | Status | Notes and caveats |
| --- | --- | --- |
| Parked event loop | done | blocks in one dispatch; wakes only for compositor events, frame callbacks, and timer deadlines — idle CPU 0%, even with an occluded surface |
| Damage-tracked repaint | done | per-widget invalidation, damage-union clipping, per-buffer staleness so rotated buffers stay correct; idle frames paint nothing |
| Measure cache | done | memoized per widget against its constraints; edits drop exactly the affected branch |
| Animation | done | timer-paced clock; easing curves, damped springs, Sequence/Parallel timelines, mid-flight cancel |
| Fractional scale | done | 120-based rational scales end to end; logical coordinates for input, layout, and carets; text rasterizes at device scale |
| Rendering | done | premultiplied-alpha ARGB8888, signed-distance AA (coverage scales all channels), shaped text, SVG/PNG icons |
| Accessibility | decision | semantic roles + `DescribeTree`, keyboard-first guarantee pinned by tests; **no in-process AT-SPI** — see [docs/a11y.md](docs/a11y.md) |
| Icon themes | done | freedesktop icon-theme spec lookup in pure Go; explicit theme switches (no live xsettings signal) — see [docs/icons.md](docs/icons.md) |

The rules that keep all of this correct — the parked loop, the
resize-before-acquire ordering, buffer staleness, the shared line
height, the implicit-grab input model — are written down with file
pointers in [docs/architecture.md](docs/architecture.md).

## Quickstart

```sh
nix develop   # pinned go 1.27, gopls, golangci-lint, gofumpt, delve, just, sway; CGO_ENABLED=0
just demo     # the gelm-hello showcase: widgets, drag, tooltips, menu, Tab focus
just panel    # the gelm-panel layer-shell demo
```

Other recipes: `just bar` (layer-shell bar), `just multi` (many windows
on one loop), `just check` (the release gates: vet, lint, test,
build), `just fmt` (gofumpt in place; `just fmt-check` is the gate CI
runs), `just bench` (benchmarks, see below). For headless runs without
a desktop: `just test-env` starts a private sway on wlroots' headless
backend, and `just demo-headless`, `move`, `click`, `sweep`, `axis`
drive the demo through the synthetic `wlpointer` client.

### Packaging and releases

The flake exposes the module and the demo binaries as packages, built
with the same pinned Go toolchain the dev shell uses (no cgo, no
network at build time — modules are vendored via `buildGoModule` with
`vendorHash` pinning `go.sum`):

```sh
nix build .#gelm-hello   # the widget showcase binary
nix build .#gelm-bar     # the layer-shell bar
nix build .#gelm-panel   # the layer-shell panel
nix build .#gelm         # the whole module: every demo binary + wlpointer
```

`packages.gelm.goModules` carries the vendored dependency tree for
other nix Go builds. Plain Go consumers don't go through nix: the
toolkit ships as an ordinary Go module, and apps pin a version by
semver tag — `go get github.com/stubbedev/gelm@v0.1.0`.

Releases are tag-driven: pushing `v*` runs
`.github/workflows/release.yml`, which builds the three demo binaries
(linux/amd64 + arm64, CGO off) with goreleaser and attaches archives
to the GitHub release. goreleaser was chosen over a nix-based release
path because release artifacts must be usable without nix, while the
flake remains the primary build path; the two share the same source
and the same CGO-off constraint.

### Benchmarks

`widget/benchmark_test.go` benchmarks the three frame passes on the
showcase tree: `BenchmarkShowcaseMeasure` (cold measure, cache
defeated by alternating constraints), `BenchmarkStaticTreeMeasure`
(warm measure: zero recursion), `BenchmarkShowcaseArrange` (steady
layout walk), `BenchmarkShowcasePaint` (full-window repaint), plus the
damage-tracker benchmarks `BenchmarkProgressOnlyFrame` /
`BenchmarkProgressOnlyFrames` (incremental repaint). Fixed sizes, no
time-dependent content. `just bench` runs them; CI archives the
numbers as workflow artifacts for trend watching — deliberately not a
gate, shared runners are too noisy.

### The minimal app

gelm's session plumbing is `internal/`, so an app is a command inside
the module (all the demos are). Add `cmd/hello/main.go`:

```go
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/internal/sysfont"
	"github.com/stubbedev/gelm/internal/window"
	"github.com/stubbedev/gelm/internal/wlsession"
	"github.com/stubbedev/gelm/widget"
)

func main() {
	sess := must(wlsession.Connect())
	defer sess.Close()
	tf := must(sysfont.Sans())
	surf := must(sess.Compositor().CreateSurface())
	win := must(window.New(sess.WmBase(), surf, window.Config{
		Title: "hello", AppID: "dev.stubbe.gelm.hello", Width: 320, Height: 120,
	}))
	die(surf.Commit())
	for range 20 { // complete the xdg configure handshake
		if win.EnsureUsable() == nil {
			break
		}
		die(sess.Roundtrip())
	}

	clicks := 0
	count := widget.NewLabel(tf, "clicked 0 times", 15, widget.Current().Text)
	button := widget.NewButton(count, 10, 8)
	button.OnClick = func() {
		clicks++
		count.SetText(fmt.Sprintf("clicked %d times", clicks))
	}
	sess.OnWmBasePing = win.Pong

	err := app.Run(app.Config{Session: sess, Host: win, Root: button, Background: widget.Current().Bg})
	if err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

// must unwraps a (value, error) pair or dies; die checks an error.
func must[T any](v T, err error) T {
	if err != nil {
		log.Fatal(err)
	}
	return v
}

func die(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
```

`go run ./cmd/hello` maps the window; the compositor's close button
ends the loop. This exact program is kept compiling (and honest) as
`ExampleRun` in `app/example_test.go`, and the widget-level examples —
`ExampleEntry`, `ExampleTextArea`, `ExampleMenu` — run under
`go test ./...` with their output checked.

### Documentation

| Document | Contents |
| --- | --- |
| [docs/architecture.md](docs/architecture.md) | session → surfaces → app loop → router → widgets, and the invariants |
| [docs/input-model.md](docs/input-model.md) | the input contract: routing, implicit grab, click/drag, dnd, keyboard, IME |
| [docs/application-model.md](docs/application-model.md) | many windows on one loop; relm4/GTK concept mapping |
| [docs/threading.md](docs/threading.md) | the threading contract: `app.Invoke`, `app.Every`, goroutine rules, and the relm4 Component/Worker/Command/Factory mapping |
| [docs/a11y.md](docs/a11y.md) | the accessibility decision and the recorded AT-SPI path |
| [docs/icons.md](docs/icons.md) | icon theme lookup and symbolic recoloring |
| [docs/completeness.md](docs/completeness.md) | the relm4/GTK coverage map: shipped, deliberate non-goals, known gaps |

## Demos

| Command | What it shows |
| --- | --- |
| `cmd/gelm-hello` | the showcase: every widget, drag-to-move, context menu, tooltips, Tab focus |
| `cmd/gelm-multi` | one process, one loop: per-output layer bars, on-demand windows, close-request veto |
| `cmd/gelm-invoke` | the threading model: a goroutine updates a label via `app.Invoke`, `app.Every` drives a poller, the loop parks between ticks |
| `cmd/gelm-panel` | a right-anchored layer-shell panel with live widgets |
| `cmd/gelm-bar` | the M0 bar: a 32px top bar; each second only the old and new notch regions repaint |
| `cmd/wlpointer` | synthetic pointer for the headless test env |

## Roadmap

Milestones from the plan: M1 paint, M2 widgets, and M4 input are in;
M3 theming is palette-only (CSS engine ahead); M5 apps (bar, OSD,
launcher, lock screen, settings) is where the demos point. Long term:
GTK-class completeness, tiers T1-T4 in the issue.

## Dependencies

All pure Go, no cgo:

- [neurlang/wayland](https://github.com/neurlang/wayland) — Wayland
  client, event loop, and protocol scanner
- [go-text/typesetting](https://github.com/go-text/typesetting) —
  Harfbuzz-grade shaping and glyph outlines
- [golang.org/x/image](https://pkg.go.dev/golang.org/x/image) — vector
  rasterization (and embedded test fonts)
- [srwiley/oksvg](https://github.com/srwiley/oksvg) +
  [srwiley/rasterx](https://github.com/srwiley/rasterx) — SVG icons
- [unxed/xkb-go](https://github.com/unxed/xkb-go) — the compositor's
  xkb keymap, self-contained (no system xkb data at runtime)

## License

MIT
