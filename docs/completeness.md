# gelm completeness

The capability map against relm4, GTK4 and libadwaita: what gelm ships
per area and where it lives. Known gaps are listed at the end; anything
not listed there is shipped. The standing non-goals and their reasons
live in [architecture.md](architecture.md#non-goals).

## Framework

| relm4 / GTK | gelm | Where |
| --- | --- | --- |
| `relm4::Application`, `ApplicationWindow` | `app.Application` (one loop for every window), `app.Window`, `app.LayerWindow` | app/application.go, [application-model.md](application-model.md) |
| main-thread contract, `glib::timeout_add` | one loop goroutine; `Application.Invoke`, `Every`; the off-loop mutation guard | app/invoke.go, widget/thread.go, [threading.md](threading.md) |
| `Component`, `Controller`, `RelmApp::run` | `component.Component[In, Out]`, `Context`, `Controller` (`Forward`, `ForwardTo`, `Detach`), `component.Window`/`Layer`; headless tests with `componenttest.Loop` | component/, [components.md](components.md) |
| `MessageBroker`, `SharedState` | `app.Stream[Msg]`, `app.SharedState[T]` | app/message.go |
| relm4 `binding` module | `widget.Binding[T]` + the widget `Bind*` connectors | widget/binding.go |
| GApplication single instance, `command-line`, `open` | `app.ClaimInstance` + `InstanceConfig` hooks | app/instance.go |
| GSettings | `app.NewSettings` + typed `Key[T]`, persisted atomically | app/settings.go |
| gettext for built-in strings | `widget.SetMessageCatalog`, `widget.Tr` | widget/message.go |
| `gtk::Application` accels, `gio::SimpleAction` | `Application.AddAction`, `AddAccel` (chords), `AddWidgetAccel` | app/accel.go |
| library logging | injectable `*slog.Logger` (`app.SetLogger`), silent by default | internal/logutil |
| GTK inspector | `GELM_INSPECT=1`, `Application.SetInspect`, `widget.DumpTree` | app/inspect.go, [inspector.md](inspector.md) |

## Widgets

| relm4 / GTK / adw | gelm | Where |
| --- | --- | --- |
| Label, styled text | `Label` (UAX #14 wrap, start/middle/end ellipsize), `RichLabel` (markup, links) | widget/label.go, widget/richlabel.go |
| Button, ToggleButton, CheckButton, Switch, radio groups | `Button` (any child), `ToggleButton`, `CheckButton`, `Switch`; toggles and checks join radio groups | widget/button.go, widget/toggle.go, widget/radio.go |
| Scale, ProgressBar, LevelBar, SpinButton | `Slider`, `ProgressBar`, `LevelBar`, `SpinButton` | widget/slider.go, widget/progressbar.go, widget/levelbar.go, widget/spin.go |
| Entry, PasswordEntry, SearchEntry, EditableLabel | `Entry` (selection, clipboard, IME, `EchoPassword`/`EchoNone`), `SearchEntry`, `SearchBar`, `ComboEntry` + completion, `EditableLabel` | widget/entry.go, widget/search.go, widget/completion.go, widget/editablelabel.go |
| undo/redo | bounded coalescing stack shared by `Entry` and `TextArea` | widget/undo.go |
| TextView, SourceView | `TextArea` (soft wrap, Tab trap), code view (line numbers, highlighters, auto-indent); `highlight`: TOML, Go, JSON, YAML, Markdown, Adwaita schemes | widget/textarea.go, widget/codeview.go, highlight/ |
| DropDown, ComboBox | `Dropdown`, `DropdownOf[T]`, `NewDropdownRows` (icons, headers); type-ahead | widget/dropdown.go |
| Box, CenterBox, Stack, Overlay, ScrolledWindow | `Box`, `CenterBox`, `Stack` (transitions), `Overlay`, `Scroll` (kinetic, `RevealRect`), `Scrollbar` | widget/box.go, widget/centerbox.go, widget/containers.go |
| Grid, FlowBox, adw WrapBox | `Grid` (spans, squeeze to MinSizer floors), `FlowBox`, `NewWrapBox` | widget/grid.go, widget/flowbox.go |
| Paned, Frame, AspectFrame, Revealer, Expander, SizeGroup | the same names | widget/paned.go, widget/frame.go, widget/revealer.go, widget/expander.go, widget/shrink.go |
| Notebook | `Notebook`: close hook, reorder by drag, overflow scrolling | widget/notebook.go |
| ListView, GridView, ColumnView, TreeExpander | virtualized `List` (none/single/browse/multiple selection, rubber band), `ColumnView[T]` (sort, filter), `FlatTree[T]` | widget/list.go, widget/columnview.go, widget/tree.go |
| PopoverMenu, PopoverMenuBar | `Menu` (check/radio, submenus, mnemonics, accels), `MenuBar` | widget/menu.go, widget/menubar.go |
| HeaderBar, WindowControls, ActionBar | the same names; `Application.AttachHeader`, `AttachMenuBar` | widget/chrome.go, app/chrome.go |
| Popover | `app.Popover`, anchored to any widget, on toplevels and layer surfaces | app/popover.go |
| Dialog, MessageDialog, AlertDialog | `app.Dialog`, `MessageBox`, `PromptDialog`, `ProgressDialog`; modality through xdg-dialog-v1 | app/dialog.go, app/prompt.go |
| FileDialog | `app.OpenFileDialog`, `OpenFilesDialog`, `OpenFolderDialog`, `SaveFileDialog` (pure-Go picker, XDG recents) | app/filedialog.go, widget/filechooser.go |
| ColorDialog, FontDialog, Calendar, EmojiChooser | `ColorChooserDialog`, `FontChooserDialog`, `CalendarDialog`, `EmojiChooser` | app/dialog.go, app/emojichooser.go |
| adw AboutDialog | `Application.AboutDialog` | app/about.go |
| ShortcutsWindow | `ShortcutsView` + `app.ShortcutsDialog` from the accelerator registry | widget/shortcuts.go, app/accel.go |
| adw Toast | `Toast` + `Application.ShowToast` (stacking, action, hover pause) | widget/toast.go, app/toast.go |
| Tooltip | `SetTooltip`, `SetTooltipMarkup`, `SetTooltipOptions` on any widget | widget/widget.go, app/tooltip.go |
| Spinner, Separator, Image, Icon | `Spinner`, `NewSeparator`, `Image` (fit/cover, async decode, PNG/JPEG/GIF/WebP, animated GIF/APNG), `Icon` (theme, file, SVG, symbolic tint, bundled Lucide fallback) | widget/spinner.go, widget/image.go, widget/icon.go |
| DrawingArea, GestureStylus | `DrawingArea` (`OnDraw`, `OnStylus`) | widget/drawingarea.go |
| adw PreferencesPage/Group, ActionRow family | `PreferencesPage`, `PreferencesGroup`, `ActionRow`, `SwitchRow`, `SpinRow`, `ComboRow`, `EntryRow`, `ButtonRow`, `ExpanderRow` | widget/prefs.go, widget/actionrow.go, widget/controlrows.go |
| adw NavigationView, NavigationSplitView, OverlaySplitView, Carousel, ViewSwitcher, StackSwitcher, Clamp, BreakpointBin | the same names; one breakpoint engine | widget/navigation.go, widget/adaptive.go |
| adw Banner, BottomSheet, StatusPage, Avatar, SplitButton, ButtonContent, ToggleGroup | the same names | widget/adw.go, widget/bottomsheet.go |
| drag and drop | `DragSource`/`DragEnterer` per widget, action negotiation, cross-window | widget/input.go, app/dragdrop.go |
| RTL/bidi | UAX #9 in every text path; `Direction` on text widgets, menus, dropdowns and Box | internal/text, widget/direction.go |

## Text, rendering and theming

| Area | gelm | Where |
| --- | --- | --- |
| rendering | premultiplied ARGB8888, signed-distance AA, damage-clipped repaint, gradients (linear, radial, conic, repeating) | render/ |
| fonts | system lookup (`app.Font`, `FontWeighted`, `FontFamilies`), per-rune fallback chains, color emoji (CBDT/sbix, COLRv0/v1, OT-SVG), variable fonts | app/fonts.go, render/, internal/sysfont |
| theming | typed `Theme` palette with copying `With*`, dark/light/high-contrast presets, WCAG guard | widget/theme.go |
| CSS | GTK-flavored stylesheets above the palette: selectors, `@define-color`, transitions, `@keyframes`, hot reload | internal/style, widget/style.go, [css.md](css.md) |
| icon themes | freedesktop spec in pure Go, live icon-theme following | internal/icons, [icons.md](icons.md) |
| accessibility | semantic roles, keyboard-first guarantee, in-process AT-SPI bridge | widget/a11y.go, internal/atspi, [a11y.md](a11y.md) |
| animation (`adw::TimedAnimation`, `SpringAnimation`) | `anim`: tweens, easing curves, springs, Sequence/Parallel/Delay timelines, reduced motion | anim/, internal/animclock |

## Protocols and platform

| Area | gelm | Where |
| --- | --- | --- |
| windows | xdg-shell toplevels (close veto, resize edges, min/max, maximize, fullscreen per output, transient parents, move/resize grabs), wlr-layer-shell surfaces | internal/window, internal/layersurface, app/windowctl.go |
| decorations | xdg-decoration when the compositor decorates; `HeaderBar` is the explicit client-side opt-in | internal/window, [application-model.md](application-model.md#window-chrome) |
| window icons | xdg-toplevel-icon-v1 | app/windowicon.go |
| surface hints | content type, live opacity (alpha-modifier), error bell, `wm_capabilities` | app/surfacehints.go |
| keyboard | the compositor's xkb keymap, synthesized repeat | internal/wlsession |
| IME | zwp_text_input_v3 into Entry and TextArea, popovers included | internal/wlsession/textinput.go |
| pointer, cursors | cursor-shape-v1 with xcursor fallback, pointer constraints, relative motion | internal/wlsession, app/constraints.go |
| touch, gestures, tablets | wl_touch with pointer emulation, pointer-gestures, tablet-v2 stylus samples | internal/wlsession, widget/touch.go |
| fractional scale | wp_viewporter + wp_fractional_scale_v1, live rescale; integer fallback | internal/scale |
| clipboard | wl_data_device + primary selection; text, images, URIs, HTML | app/clipboard.go, transfer/ |
| clipboard managers | ext-data-control-v1, wlr-data-control fallback | app/datacontrol.go |
| taskbars, launchers | wlr-foreign-toplevel-management, xdg-activation, `OpenURL`/`OpenPath` (portal, xdg-open fallback) | internal/wlsession, app/launcher.go |
| idle | idle inhibit, ext-idle-notify-v1 (`OnIdle`) | app/application.go |
| shortcuts | keyboard-shortcuts-inhibit, GlobalShortcuts portal | internal/wlsession, app/globalshortcuts.go |
| lock screens | ext-session-lock-v1 (`LockSession`, every output, hotplug) | app/sessionlock.go |
| notifications | `Application.Notify` with actions | app/notification.go |
| screen capture | `capture`: wlr-screencopy, ext-image-copy-capture, Hyprland toplevel export, dmabuf import | capture/ |
| file and fd watching | `WatchFiles`, `WatchFD` on the loop | app/watch.go |
| compositor restarts | typed disconnect, clean exit code 75, optional reconnect with rebuild | app/disconnect.go, app/reconnect.go |
| system appearance (`adw::StyleManager`) | `Application.ColorScheme`, `Accent`, `Contrast` with loop-delivered `On*Change` hooks; types in `appearance` | app/appearance.go, appearance/, [appearance.md](appearance.md) |

## Known gaps

The component framework relm4 is built around, tracked in
[#139](https://github.com/stubbedev/gelm/issues/139):

- no declarative view: there are no typed public builders, and no
  watch/track refresh;
- no change tracking over a model;
- no factory of per-item components with in-place edits;
- no commands bound to a component's lifetime, and no async
  components;
- no typed workers;
- actions are string-keyed, with no state, parameters or groups;
- the demos use `internal/` packages.

The Hyprland gate is allow-failure until it has been green for a
sustained window; sway is the required gate.
