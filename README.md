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

Apps import the public packages: `app` (session, windows, loop,
dialogs, desktop services), `widget` (the widget tree, theme, CSS),
`render`, `transfer`, `highlight`, `capture`, and `widget/css`.
Everything under `internal/` is implementation.

## A minimal app

```go
package main

import (
	"errors"
	"fmt"
	"log"

	"github.com/stubbedev/gelm/app"
	"github.com/stubbedev/gelm/widget"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, app.ErrClosed) {
		log.Fatal(err)
	}
}

func run() error {
	sess, err := app.Connect()
	if err != nil {
		return err
	}
	defer sess.Close()
	face, err := app.Font("sans", 15)
	if err != nil {
		return err
	}

	clicks := 0
	count := widget.NewLabel(face, 15, "clicked 0 times", widget.Current().Text)
	button := widget.NewButton(count, 10, 8)
	button.OnClick = func() {
		clicks++
		count.SetText(fmt.Sprintf("clicked %d times", clicks))
	}

	application := app.NewApplication(sess)
	if _, err := application.NewWindow(app.WindowConfig{
		Title: "hello", AppID: "dev.stubbe.gelm.hello",
		Width: 320, Height: 120,
		Root: button, Background: widget.Current().Bg,
	}); err != nil {
		return err
	}
	return application.Run()
}
```

This program is kept compiling as `ExampleApplication` in
`app/example_test.go`.

## Development

```sh
devenv shell       # pinned Go, gopls, golangci-lint, gofumpt, just, sway; CGO_ENABLED=0
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
| `cmd/gelm-hello` | the showcase: every core widget, drag and drop, context menu, tooltips, Tab focus |
| `cmd/gelm-bar` | a layer-shell top bar repainting only what changed each second |
| `cmd/gelm-panel` | a right-anchored layer-shell panel with live widgets |
| `cmd/gelm-multi` | one loop: per-output layer bars, on-demand windows, close-request veto |
| `cmd/gelm-invoke` | goroutines reaching the loop through `Invoke` and `Every` |
| `cmd/gelm-messages` | the typed messaging layer and single-instance forwarding |
| `cmd/gelm-settings` | a preferences app over typed persisted settings |
| `cmd/gelm-columns` | ten thousand sorted, filtered rows; the tree view; drag-to-reorder |
| `cmd/gelm-i18n` | the message catalog localizing every built-in string |
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
[rasterx](https://github.com/srwiley/rasterx), and
[unxed/xkb-go](https://github.com/unxed/xkb-go).

## License

MIT
