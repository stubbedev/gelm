# gelm

A pure-Go (no cgo) widget toolkit for Wayland, built as a
feature-complete counterpart of [relm4](https://relm4.org): a retained
widget tree with GTK4/libadwaita-class widgets, xdg-shell and
layer-shell windows, a GTK-flavored CSS layer over a typed palette,
and an event loop that idles at 0% CPU.

It works end to end on real compositors. Sway is the required
headless gate (`just headless`). Hyprland runs the same suite in a
private NixOS VM (`just hyprland-vm`) and is allow-failure until it
has stayed green.

## Install

```sh
go get github.com/stubbedev/gelm@latest
```

Apps import the public packages: `component` (the component
framework, headless tests through `component/componenttest`), `ui`
(typed view builders), `app` (session, windows, loop, dialogs, desktop services),
`widget` (the widget tree, theme, CSS), `i18n` (gettext), `anim`,
`appearance`, `render`, `pdf` (PDF writing), `media` (video and sound playback), `dmabuf` (dma-buf descriptions),
`transfer`, `highlight`, `capture`, and `widget/css`.
Everything under `internal/` is implementation.

## A minimal app

```go
package main

import (
	"log"
	"strconv"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/component"
	"github.com/stubbedev/gelm/ui"
	"github.com/stubbedev/gelm/widget"
)

type Msg int

const (
	Increment Msg = iota
	Decrement
)

type Counter struct {
	env ui.Env
	n   int
}

func (c *Counter) Init(cx *component.Context[Msg, int]) widget.Widget {
	return ui.Mount(cx, c.env, ui.Row(
		ui.Button(ui.Label("-"), 8, 6).OnClick(func() { cx.Input(Decrement) }),
		ui.Expand(ui.Label("").WatchText(func() string { return strconv.Itoa(c.n) })),
		ui.Button(ui.Label("+"), 8, 6).OnClick(func() { cx.Input(Increment) }),
	).Spacing(6))
}

func (c *Counter) Update(cx *component.Context[Msg, int], msg Msg) {
	switch msg {
	case Increment:
		c.n++
	case Decrement:
		c.n--
	}
	cx.Output(c.n)
}

func main() {
	err := component.Run(app.WindowConfig{Title: "counter", AppID: "dev.example.counter"},
		func(a *app.Application) (*Counter, error) {
			face, err := app.Font("sans-serif", 15)
			if err != nil {
				return nil, err
			}
			if err := a.AddAccel("Escape", widget.NewAction("quit", a.Quit)); err != nil {
				return nil, err
			}
			return &Counter{env: ui.Env{Face: face, Size: 15}}, nil
		})
	if err != nil {
		log.Fatal(err)
	}
}
```

This program is kept compiling as `ExampleRun` in
`component/example_test.go`. [docs/components.md](docs/components.md)
walks through components, the typed builders, factories, workers and
actions. `app.NewApplication` and `NewWindow` remain the lower-level
path for apps that want the widget tree without components
(`ExampleApplication` in `app/example_test.go`).

## Development

```sh
devenv shell       # pinned Go, gopls, golangci-lint, just, sway; CGO_ENABLED=0
just demo          # the gelm-hello showcase
just check         # vet, lint, test, build: the release gates
just headless      # the compositor-in-the-loop suite on a private headless sway
just bench         # benchmarks (archived by CI, not a gate)
```

`nix build .#gelm` builds the module and every demo with the pinned
toolchain. Pushing a `v*` tag releases the demo binaries
(linux/amd64 and arm64) through goreleaser.

## Documentation

| Document | Contents |
| --- | --- |
| [docs/completeness.md](docs/completeness.md) | the capability map against relm4/GTK/libadwaita, and the known gaps |
| [docs/architecture.md](docs/architecture.md) | the layers, the invariants that keep them correct, theming, non-goals |
| [docs/application-model.md](docs/application-model.md) | windows, layer surfaces, disconnects, single instance, file dialogs, data transfer, chrome |
| [docs/components.md](docs/components.md) | the component framework: components, typed view builders, tracking, controllers, children, testing |
| [docs/threading.md](docs/threading.md) | the loop goroutine, `Invoke`/`Every`, the off-loop guard, the typed messaging layer |
| [docs/input-model.md](docs/input-model.md) | routing, implicit grab, click/drag, drag and drop, keyboard, IME, touch, tablets |
| [docs/css.md](docs/css.md) | the CSS layer: selectors, values, properties, cascade, lifecycle |
| [docs/appearance.md](docs/appearance.md) | following the desktop's color scheme, accent and contrast |
| [docs/icons.md](docs/icons.md) | icon-theme lookup and symbolic recoloring |
| [docs/a11y.md](docs/a11y.md) | semantic roles, the keyboard-first guarantee, the AT-SPI bridge |
| [docs/inspector.md](docs/inspector.md) | the live widget inspector, tree dump, and doctor block |

## Demos

| Command | What it shows |
| --- | --- |
| `cmd/gelm-hello` | the showcase as a component: core widgets, a context menu on typed actions, tooltips, Tab focus, drag-to-move |
| `cmd/gelm-bar` | a layer-shell top bar repainting only what changed each second |
| `cmd/gelm-panel` | a right-anchored layer-shell panel with live widgets |
| `cmd/gelm-multi` | one loop: per-output layer bars, on-demand windows, close-request veto |
| `cmd/gelm-invoke` | a lifetime-bound spawned command and `Every` feeding a component |
| `cmd/gelm-messages` | the typed messaging layer and single-instance forwarding |
| `cmd/gelm-settings` | a preferences app over typed persisted settings |
| `cmd/gelm-columns` | ten thousand sorted, filtered rows; the tree view; drag-to-reorder |
| `cmd/gelm-gpu` | a GPUArea paced by its own frame callbacks, with gelm drawing over it |
| `cmd/gelm-video` | a video with its controls: a file through ffmpeg, or a generated clip decoded in pure Go |
| `cmd/gelm-i18n` | a gettext .po catalog (plurals included) localizing built-in and app strings |
| `cmd/gelm-states`, `cmd/gelm-multilist`, `cmd/gelm-popover` | the clients the headless suite drives: window states, multi-select, popovers |
| `cmd/wlpointer`, `cmd/zz-vpclick` | synthetic input for the headless suite |

## Dependencies

All pure Go:
[neurlang/wayland](https://github.com/neurlang/wayland) (patched copy
in `third_party/`),
[godbus/dbus](https://github.com/godbus/dbus),
[go-text/typesetting](https://github.com/go-text/typesetting),
[golang.org/x/image](https://pkg.go.dev/golang.org/x/image),
[srwiley/oksvg](https://github.com/srwiley/oksvg) +
[rasterx](https://github.com/srwiley/rasterx),
[unxed/xkb-go](https://github.com/unxed/xkb-go), and
[jfreymuth/pulse](https://github.com/jfreymuth/pulse) (the PulseAudio
protocol, which pipewire-pulse also serves). `media.OpenFile` runs
`ffmpeg` and `ffprobe` from PATH when an app plays a file; nothing else
needs them.

## License

MIT
