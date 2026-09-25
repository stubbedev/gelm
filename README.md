# gelm

A retained-mode, CSS-themed widget kit for Wayland, written in pure Go
(no cgo). Planned to carry a future Go wayle; see
[stubbedev/wayle#19](https://github.com/stubbedev/wayle/issues/19) for the
full plan and the completeness ladder.

## Status: M0 scaffold

What works today:

- wlr-layer-shell-unstable-v1 bindings generated with the pure-Go
  `neurlang/wayland` scanner from the vendored protocol XML
- layer surface with anchor, exclusive zone, keyboard mode, and the
  configure handshake (auto-size axes validated against the anchor rules)
- pooled `wl_shm` ARGB8888 buffers (premultiplied) with defined busy
  semantics; the shell never draws into a buffer the compositor holds
- damage-tracked frame loop driven by frame callbacks, integer HiDPI
  scaling from `wl_output.scale`

Demo (on a layer-shell compositor: Hyprland, niri, sway, river, ...):

```sh
go run ./cmd/gelm-bar
```

A 32px-tall top bar spanning the output, with a second notch moving along
it. Each second only the old and new notch regions are repainted.

## Roadmap

Milestones from the plan: M0 scaffold (here), M1 paint (text, shapes,
icons), M2 widgets, M3 CSS theming, M4 input, M5 apps (bar, OSD, launcher,
lock screen, settings). Long term: GTK-class completeness, tiers T1-T4 in
the issue.

## Dependencies

- [neurlang/wayland](https://github.com/neurlang/wayland) — pure-Go Wayland
  client, event loop, and protocol scanner

## License

MIT
