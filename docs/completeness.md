# gelm completeness

The audit map against relm4/GTK: what gelm ships per capability area, and
what it deliberately does not. Second pass, recorded in
[stubbedev/gelm#36](https://github.com/stubbedev/gelm/issues/36), which
filed the gaps as #27–#35 — all closed, all verified present in the tree
when this map was written. Sources: the issue text, the README feature
matrix, docs/*.md, and `git log`. The first-pass ladder lives in
[stubbedev/wayle#19](https://github.com/stubbedev/wayle/issues/19).

Third pass, with the maintainer's goal restated — a **feature-complete
replacement for relm4** — filed the gaps below and the non-goals that
parity overturns as #60–#74 under the epic
[stubbedev/gelm#59](https://github.com/stubbedev/gelm/issues/59), with
CSS support scoped first (#75 design, #76 engine) and the
allocation/syscall reduction sweep (#77) sequenced after feature
completeness. The third pass is complete: every ticket it filed is
closed and the map below reflects the tree as of that close.

## Widgets

| relm4/GTK area | gelm's answer | Where |
| --- | --- | --- |
| Label, styled text | `widget.Label` with `SetWrap` (UAX #14 word wrap) and `SetEllipsize` (start/middle/end) keyed to the offered width; markup runs with optional links in `widget.RichLabel` | widget/label.go, widget/richlabel.go |
| Button, toggle, check | `Button` (any child, Enter+Space), `Switch`, `CheckButton` | widget/button.go, widget/toggle.go |
| Range, progress | `Slider` (drag/arrows/Home/End), `ProgressBar` | widget/slider.go, widget/toggle.go |
| Entry | `Entry`: selection, clipboard, IME preedit; masked echo `EchoPassword`/`EchoNone` with app-driven reveal (#29) | widget/entry.go, widget/echo.go |
| Undo/redo | bounded coalescing stack behind `widget.Undoer`, shared by `Entry` and `TextArea`; ctrl+z / ctrl+shift+z / ctrl+y in `routeKey` (#29) | widget/undo.go, app/app.go |
| TextView | `TextArea`: soft wrap, logical-line editing, Tab trap | widget/textarea.go |
| ComboBox | `Dropdown` / `DropdownOf[T]` — face plus inline themed item list; prefix type-ahead on the open list, first-letter cycling on the closed face (#62) | widget/dropdown.go |
| Toast | `widget.Toast` + `Application.ShowToast`: stacking, action, hover-pause (#30) | widget/toast.go, app/toast.go |
| Expander | `Expander`: animated reveal, child visible only while open (#30) | widget/expander.go |
| Spinner | `Spinner`: anim-driven rotating arc (#30) | widget/spinner.go |
| Separator | `NewSeparator(orientation)` (#30) | widget/separator.go |
| Box, Stack, Overlay, ScrolledWindow | `Box`/`Stack`/`Overlay`/`Scroll` | widget/containers.go, widget/scroll.go |
| Paned | `Paned`: two panes, draggable themed divider clamped by MinSizer floors, keyboard nudges, GTK keep-child-one resize semantics (#71) | widget/paned.go |
| Calendar | `widget.Calendar` + `app.CalendarDialog`: month grid, month/year navigation, today marker, single selection, pluggable locale names (#73) | widget/calendar.go, app/dialog.go |
| Color picker | `widget.ColorChooser` + `app.ColorChooserDialog`: SV square (shader-less stacked gradients), hue/alpha strips, hex entry, theme presets + session palette (#72) | widget/colorchooser.go, app/dialog.go |
| Grid | `Grid`: cells, spans, per-axis spacing/homogeneous, child align (#34); deficit negotiation — under-sized rects squeeze tracks proportionally down to per-track MinSizer floors, overflowing only past them (#67) | widget/grid.go |
| Notebook | `Notebook`: tabs, close hook, ctrl+PageUp/PageDown | widget/notebook.go |
| ListView | virtualized `List`; single/browse/multiple selection modes with rubber-band drag and edge auto-scroll (#60) | widget/list.go |
| Menu | check/radio rows, submenus, keyboard nav; Alt-letter mnemonics with underlines, accelerators firing from an open menu through the app's table (#63) | widget/menu.go |
| Popover | `app.Popover`, widget-anchored, works on layer surfaces | app/popover.go |
| Dialog, MessageBox | `app.Dialog`, `app.MessageBox`; window-level modality through xdg-dialog-v1 with the application-level block as the floor (#61) | app/dialog.go, internal/wlsession/dialog.go |
| Tooltip | `SetTooltip` on any widget, 500ms dwell | widget/widget.go, app/tooltip.go |
| Drag and drop | `DragSource`/`DragEnterer` per widget, mime negotiation, cross-window | app/dragdrop.go, internal/dragdrop |
| Icon | raster/theme/file/embedded-SVG constructors, symbolic recoloring | widget/icon.go, [icons.md](icons.md) |
| Image | raster `Image`: `ImageFit`/`ImageCover`/`ImageNone`, async file/URL decode, LRU pixel cache (#35) | widget/image.go, internal/imgcache |
| Font selection | `sysfont.Sans`/`Monospace`/`Serif`/`Best(family, size)`, system-store fallback chains (#31); `Families()` enumeration + `app.FontChooserDialog` (#74) | internal/sysfont, app/dialog.go |
| Font fallback | `render.Chain`: per-rune coverage, bitmap/CBDT/sbix color-emoji strikes (#31) | render/text.go, internal/sysfont |
| RTL/bidirectional text | UAX #9 resolution shared by every text path (internal/text, cached by text + direction), visual-order shaping in `render`, a `Direction` (LTR/RTL/auto) on Label, RichLabel, Entry, TextArea, Menu, Dropdown, and Box; start/end alignment and Box row flow mirror, arrows and word motion move visually while cursor and selection stay logical (#68) | internal/text/bidi.go, render/text.go, widget/direction.go |
| Theming | typed `Theme` palette value with copying `With*` constructors, dark/light presets, WCAG contrast guard reporting through the injectable logger (#32, #55); a GTK-flavored CSS override layer above the palette — element/class/id/state selectors, cached computed styles, hot reload (#75 design, #76) | widget/theme.go, internal/style, widget/style.go, [css.md](css.md), [architecture.md](architecture.md) "Theming" |
| Library logging | injectable `*slog.Logger` (`wlsession.SetLogger`, `app.SetLogger`), silent by default (nil discards); Warn = degraded-but-running, Error = terminal, Debug = protocol chatter (#55) | internal/logutil, internal/wlsession/logger.go, [input-model.md](input-model.md) |
| State/threading (Components, Workers, Commands) | callbacks on one loop goroutine + `Application.Invoke`/`Every`; off-loop mutation trips the guard; relm4 concept mapping (#27) | app/invoke.go, widget/thread.go, [threading.md](threading.md) |

## Protocols and platform

| relm4/GTK area | gelm's answer | Where |
| --- | --- | --- |
| Windows | xdg-shell toplevels and zwlr-layer-shell surfaces, close veto, interactive resize, min/max | internal/window, internal/layersurface |
| Clipboard | wl_data_device + primary selection (copy-on-select, middle-click); image payloads - png claims and reads, jpeg reads, the widget.Image paste surface with off-loop decode (#69) | internal/clipboard, internal/wlsession/primary.go |
| Clipboard manager | ext-data-control-v1, falling back to wlr-data-control-unstable-v1: `Application.DataControl`, both selections watched with mime-ordered offers, bounded off-loop reads, claims from a `SelectionSource`, own-offer detection | internal/datacontrol, internal/wlsession/datacontrol.go |
| IME | zwp_text_input_v3 into Entry and TextArea | internal/wlsession/textinput.go |
| Fractional scale | wp_viewporter + wp_fractional_scale_v1, live rescale; integer fallback | internal/scale |
| Taskbar/window list | wlr-foreign-toplevel-management: `Toplevels()`, per-handle Activate/minimize/maximize/Close, `OnToplevel*` hooks (#33) | internal/wlsession/toplevel.go |
| Launcher focus | xdg-activation: `RequestActivationToken`/`Activate`/`OnActivationToken` (#33) | internal/wlsession/activation.go |
| Idle inhibit | zwp_idle_inhibit: `InhibitIdle`, nil-safe `IdleInhibitor` (#33) | internal/wlsession/idleinhibit.go |
| Shortcut grab (games, VMs) | keyboard-shortcuts-inhibit: `InhibitShortcuts`, focus-tracked `ShortcutsInhibitor` (#33) | internal/wlsession/shortinhibit.go |
| Output naming | xdg-output: logical-name/geometry lookup per output (#33) | internal/wlsession/xdgoutput.go |
| Dialog modality | xdg-dialog-v1: parented dialogs hint `set_modal`, compositor blocks the parent's input; silent degrade without the global (#61) | internal/wlsession/dialog.go, internal/window |
| Window icons | xdg-toplevel-icon-v1: `Application.SetIcon`/`SetWindowIcon`, buffers at the compositor's preferred sizes, silent degrade without the global (#70) | internal/wlsession/toplevelicon.go, app/windowicon.go |
| Wire bindings | generated pure-Go proxies for all of the above | wlr/ |
| Accessibility | semantic roles + `DescribeTree`, keyboard-first guarantee; in-process AT-SPI bridge behind the `atspi` build tag (#65) | widget/a11y.go, internal/atspi, [a11y.md](a11y.md) |
| Icon themes | freedesktop icon-theme spec in pure Go; live icon-theme setting followed through the portal monitor (#64) | internal/icons, [icons.md](icons.md) |

## Deliberate non-goals

Audited in #36 and revisited by the third pass: every non-goal that
relm4 parity required is now a shipped capability (RTL/bidi #68,
clipboard images #69, window icons #70, Paned #71, color chooser #72,
calendar #73, font chooser #74, the CSS override layer #75/#76, list
multi-select #60, dialog modality #61, type-ahead #62, menu
mnemonics #63, live icon themes #64, the AT-SPI bridge #65, Hyprland
in the gate #66, grid shrink #67). The standing non-goals — no
per-window goroutines, no window manager, no actor model, no
automatic theme switching, no live theme-change signal — and their
reasons live in [architecture.md](architecture.md) "Non-goals".

## Known gaps

Honest deferrals, each with its pointer — none of these block the
declared use cases:

- **The Hyprland gate starts allow-failure** — the
  compositor-in-the-loop suite runs unchanged on Hyprland in the VM
  gate (`just hyprland-vm`, tests/hyprland-vm.nix, #66: Hyprland gets
  its own DRM node inside a private NixOS VM, since aquamarine cannot
  boot without one and the developer's card belongs to their live
  session); it is non-blocking in CI until it has been green for a
  sustained window. Sway stays the primary gate.
