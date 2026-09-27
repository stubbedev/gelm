# gelm completeness

The audit map against relm4/GTK: what gelm ships per capability area, and
what it deliberately does not. Second pass, recorded in
[stubbedev/gelm#36](https://github.com/stubbedev/gelm/issues/36), which
filed the gaps as #27–#35 — all closed, all verified present in the tree
when this map was written. Sources: the issue text, the README feature
matrix, docs/*.md, and `git log`. The first-pass ladder lives in
[stubbedev/wayle#19](https://github.com/stubbedev/wayle/issues/19).

## Widgets

| relm4/GTK area | gelm's answer | Where |
| --- | --- | --- |
| Label, styled text | `widget.Label` with `SetWrap` (UAX #14 word wrap) and `SetEllipsize` (start/middle/end) keyed to the offered width; markup runs with optional links in `widget.RichLabel` | widget/label.go, widget/richlabel.go |
| Button, toggle, check | `Button` (any child, Enter+Space), `Switch`, `CheckButton` | widget/button.go, widget/toggle.go |
| Range, progress | `Slider` (drag/arrows/Home/End), `ProgressBar` | widget/slider.go, widget/toggle.go |
| Entry | `Entry`: selection, clipboard, IME preedit; masked echo `EchoPassword`/`EchoNone` with app-driven reveal (#29) | widget/entry.go, widget/echo.go |
| Undo/redo | bounded coalescing stack behind `widget.Undoer`, shared by `Entry` and `TextArea`; ctrl+z / ctrl+shift+z / ctrl+y in `routeKey` (#29) | widget/undo.go, app/app.go |
| TextView | `TextArea`: soft wrap, logical-line editing, Tab trap | widget/textarea.go |
| ComboBox | `Dropdown` / `DropdownOf[T]` — face plus inline themed item list; type-ahead decided against and documented on the type (#28) | widget/dropdown.go |
| Toast | `widget.Toast` + `Application.ShowToast`: stacking, action, hover-pause (#30) | widget/toast.go, app/toast.go |
| Expander | `Expander`: animated reveal, child visible only while open (#30) | widget/expander.go |
| Spinner | `Spinner`: anim-driven rotating arc (#30) | widget/spinner.go |
| Separator | `NewSeparator(orientation)` (#30) | widget/separator.go |
| Box, Stack, Overlay, ScrolledWindow | `Box`/`Stack`/`Overlay`/`Scroll` | widget/containers.go, widget/scroll.go |
| Grid | `Grid`: cells, spans, per-axis spacing/homogeneous, child align (#34) | widget/grid.go |
| Notebook | `Notebook`: tabs, close hook, ctrl+PageUp/PageDown | widget/notebook.go |
| ListView | virtualized `List`; **single-select only** (a documented limit, see Known gaps) | widget/list.go |
| Menu | check/radio rows, submenus, keyboard nav; accelerators display-only | widget/menu.go |
| Popover | `app.Popover`, widget-anchored, works on layer surfaces | app/popover.go |
| Dialog, MessageBox | `app.Dialog`, `app.MessageBox`; application-level modality (xdg_shell has no modal bit) | app/dialog.go |
| Tooltip | `SetTooltip` on any widget, 500ms dwell | widget/widget.go, app/tooltip.go |
| Drag and drop | `DragSource`/`DragEnterer` per widget, mime negotiation, cross-window | app/dragdrop.go, internal/dragdrop |
| Icon | raster/theme/file/embedded-SVG constructors, symbolic recoloring | widget/icon.go, [icons.md](icons.md) |
| Image | raster `Image`: `ImageFit`/`ImageCover`/`ImageNone`, async file/URL decode, LRU pixel cache (#35) | widget/image.go, internal/imgcache |
| Font selection | `sysfont.Sans`/`Monospace`/`Serif`/`Best(family, size)`, system-store fallback chains (#31) | internal/sysfont |
| Font fallback | `render.Chain`: per-rune coverage, bitmap/CBDT/sbix color-emoji strikes (#31) | render/text.go, internal/sysfont |
| Theming | typed `Theme` palette value with copying `With*` constructors, dark/light presets, debug WCAG guard (#32) — no CSS engine (a non-goal, see below) | widget/theme.go, [architecture.md](architecture.md) "Theming" |
| State/threading (Components, Workers, Commands) | callbacks on one loop goroutine + `Application.Invoke`/`Every`; off-loop mutation trips the guard; relm4 concept mapping (#27) | app/invoke.go, widget/thread.go, [threading.md](threading.md) |

## Protocols and platform

| relm4/GTK area | gelm's answer | Where |
| --- | --- | --- |
| Windows | xdg-shell toplevels and zwlr-layer-shell surfaces, close veto, interactive resize, min/max | internal/window, internal/layersurface |
| Clipboard | wl_data_device + primary selection (copy-on-select, middle-click) | internal/clipboard, internal/wlsession/primary.go |
| IME | zwp_text_input_v3 into Entry and TextArea | internal/wlsession/textinput.go |
| Fractional scale | wp_viewporter + wp_fractional_scale_v1, live rescale; integer fallback | internal/scale |
| Taskbar/window list | wlr-foreign-toplevel-management: `Toplevels()`, per-handle Activate/minimize/maximize/Close, `OnToplevel*` hooks (#33) | internal/wlsession/toplevel.go |
| Launcher focus | xdg-activation: `RequestActivationToken`/`Activate`/`OnActivationToken` (#33) | internal/wlsession/activation.go |
| Idle inhibit | zwp_idle_inhibit: `InhibitIdle`, nil-safe `IdleInhibitor` (#33) | internal/wlsession/idleinhibit.go |
| Shortcut grab (games, VMs) | keyboard-shortcuts-inhibit: `InhibitShortcuts`, focus-tracked `ShortcutsInhibitor` (#33) | internal/wlsession/shortinhibit.go |
| Output naming | xdg-output: logical-name/geometry lookup per output (#33) | internal/wlsession/xdgoutput.go |
| Wire bindings | generated pure-Go proxies for all of the above | wlr/ |
| Accessibility | semantic roles + `DescribeTree`, keyboard-first guarantee; **no in-process AT-SPI** (a non-goal, see below) | widget/a11y.go, [a11y.md](a11y.md) |
| Icon themes | freedesktop icon-theme spec in pure Go; theme switches are explicit | internal/icons, [icons.md](icons.md) |

## Deliberate non-goals

Audited in #36 and **not** filed — with the maintainer's reasons, now
also recorded in [architecture.md](architecture.md) "Non-goals":

- **RTL/bidirectional text** — no wayle use case; revisit if ever needed.
- **Clipboard images** — wayle is text-only for clipboard.
- **Window icons** — Wayland has no client window icons.
- **GtkCss analog** — rejected on cost; #32 scopes styling to the typed
  `Theme` struct instead.
- **Paned (draggable splitter)** — no wayle layout needs it; file later
  if the settings UI wants it.
- **Color picker, calendar, font chooser** — application-dialog
  territory gelm does not aim at.

The standing non-goals (no per-window goroutines, no window manager, no
actor model, no per-widget theme overrides, no live theme-change
signal) are in [architecture.md](architecture.md) "Non-goals".

## Known gaps

Honest deferrals, each with its pointer — none of these block the
declared use cases:

- **List is single-select**; no multi-select model yet (README matrix).
- **Dialog modality is application-level only** — xdg_shell has no
  modal bit (README matrix, app/dialog.go).
- **No Dropdown type-ahead** — decided against in #28: menus have no
  mnemonics either, and a typed character collides with Space-opens in
  the text router (documented on the type, widget/dropdown.go).
- **Menu accelerators are display-only**; no mnemonics (README matrix).
- **Icon themes do not follow live setting changes** — the lookup is
  explicit ([icons.md](icons.md), widget/theme.go); the system
  dark/light preference itself is observable since #53
  ([appearance.md](appearance.md)) but wiring it to a palette swap is
  the app's call, never the toolkit's.
- **No in-process AT-SPI** — the integration path is recorded, not
  built ([a11y.md](a11y.md)).
- **Compositor-in-the-loop tests run on headless sway only** — the
  automated gate (`just headless`, internal/headlesstest) boots wlroots'
  headless backend; Hyprland is verified manually (README "Status"),
  not in CI.
- **Grid tracks never shrink below their maxima** — under-sized rects
  overflow and let the painter's clip decide visibility, matching Box
  (widget/grid.go).
