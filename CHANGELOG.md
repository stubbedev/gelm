# Changelog

## Unreleased

### Component framework

- New `component` package, relm4's component model:
  `Component[In, Out]` (`Init`, `Update`, optional `UpdateView` and
  `Shutdown`), `Context` (`Input`, `Output`, `Sender`, `Watch`,
  `OnShutdown`, child `Launch`), and `Controller` (`Widget`, `Send`,
  `Forward`, `ForwardTo`, `Detach`, `Shutdown`). `component.Window` and
  `Layer` launch a component as a window's root.
- Children launched from a context shut down with their parent;
  top-level components shut down when the loop stops.
- `component/componenttest.Loop` runs components headless in tests.
- `Application.OnStop(fn)`: hooks run when `Run` returns, in reverse
  order. The loop's stop set used to iterate a map.
- Removed `app.Component[Msg]` and `NewComponent`; `component`
  replaces them.

### Public system appearance

- `Application.ColorScheme`, `Accent`, `Contrast` and their
  `OnColorSchemeChange`, `OnAccentChange`, `OnContrastChange` hooks,
  delivered on the loop goroutine (each returns `off`). The value types
  live in the new public `appearance` package; `Accent.Color` converts
  to a `render.Color`.
- The portal monitor moved to `internal/portalsettings`, and
  `internal/appearance` is gone. Apps no longer construct a second
  monitor or bridge its goroutine themselves.

### Public animation API

- `anim` is public: `Animate`, `Tween.Easing`, the easing curves
  (`CubicBezier`, `Spring`, ...), `Sequence`, `Parallel`, `Delay`,
  `Play`, `Start`, `Cancel`, and reduced motion (`Instant`,
  `SetInstant`, `GELM_NO_ANIM=1`). Apps outside the module can
  animate widgets, e.g. a `ProgressBar` value.
- The loop-driving scheduler (`Tick`, `Next`, `Active`, `SetClock`,
  `Reset`) moved to `internal/animclock`, out of the public surface.
  `internal/anim` is gone.

### Scrolling by frame: wheel steps, touchpad pixels

- The session routes a pointer frame's scroll once: wheel notches
  (axis_discrete on seats 5-7, axis_value120 on 8+) as steps, even with
  the smooth value the compositor sends beside them; finger and
  continuous scrolling as exact pixels to a `SurfacePreciseScroller`.
  Touchpads used to round every small finger delta up to a 40px step.
- `Router.AxisPixels` routes pixel scrolling; `Scroll` and `List`
  implement `PixelScroller` (fractions carried), any other scroller
  gets a step per 40 pixels.
- Fix: axis_value120 is signed as the axis is (positive down), per the
  protocol; it was negated.

### SpinButton and a focus hook

- `widget.SpinButton`: a numeric entry over `[min, max]` with a step
  and shown decimals. Up/Down step, PageUp/PageDown step ten times;
  Enter or focus leaving commits the typed text, clamped and rounded,
  and unparsable text reverts. `OnValueChanged` hears user changes
  only; `SetValue`/`SetRange` are silent. Its CSS element is
  `spinbutton`.
  `SetStep` and `SetDigits` change the step and the shown decimals.
- `SetOnFocusChanged(fn)` on every widget hears it gain or lose
  keyboard focus, however focus moved.
- `Overlay.AppendAligned(w, h, v)`: a GtkOverlay-style child at its
  natural size pinned per axis (start, center, end, or fill), hitting
  only inside its own rect.
- `DialogConfig.Bare` (with `Background`): the content is the whole
  dialog window, no toolkit card or button row; it answers through
  `Dialog.Respond`, Esc and Enter still mapping to its responses.
- `Paned.SetMaxPosition(px)`: caps the divider (drags, keys,
  SetPosition) above the start pane's floor; zero lifts the cap.
- Entries and text areas paint their caret only while focused (GTK).
- Containers parent their children as they measure, so the first
  layout is styled (it used to come out unstyled and settle a frame
  later).
- `NewDropdownRows` with `DropdownRow{Label, Icon, Header}`: per-row
  icons on the list and the face, group headers that are never
  selected, stepped onto or typed to (GTK's DropDown with a row
  factory). Menus gain `ItemHeader` caption rows.
- `List.SetSingleClickActivate`: a click activates the row too (GTK's
  single-click-activate).
- `List.SetMaxHeight(px)`: caps the natural height; the rest scrolls,
  still virtualized.
- A List with an auto row height (0) sizes rows by the styled first
  row (its padding and fonts), re-measured as styles change.
- Every widget a container parents is in its style walk (a subtree
  restyle reaches a List's rows, a Menu's icons, a Switch's knob),
  pinned by a test over every container.
- Fix: a list row takes the stylesheets above the list (rows were
  arranged before they were parented, styling them without the
  sheet).
- Fix: popovers open on toplevel windows (`OpenPopover` with an
  `*app.Window` host was refused).
- `app.FontFamilies()`: the installed families by display name, sorted
  and deduped (resolved once per process).
- A dropdown's list claims no menu mnemonics (no underlines; type-ahead
  is its keyboard model).
- Fix: faded colors (disabled text, accents) scale each premultiplied
  channel; scaling the packed value garbled them (a disabled entry's
  text came out green).
- The window's focus ring marks keyboard focus only (GTK's
  `:focus-visible`): a click focuses without a ring.
  `widget.FocusVisible(w)` reports it.
- `Box.AppendAligned(w, expand, cross)`: GTK's valign (in a row) or
  halign (in a column) per child; AlignFill is the plain stretch.
- `Switch` takes the stylesheet the GTK way: `switch` styles the track
  (background, radius, min size, box layers, `:checked` while on) and
  `switch slider` the knob (min size, margin, background, radius,
  shadow); unstyled it is the theme pill, unchanged.
- Fix: Paned paints its panes (it drew only the divider).
- Fix: SVG icons carrying GTK's symbolic markup (`gpa:fill='foreground'`)
  parse: attributes in foreign namespaces drop before oksvg reads them.
- Fix: a subtree restyle reaches a button's content, so its label
  follows the button's inherited color (hover, class changes) and the
  first-parent restyle.
- Fix: a widget gaining its first parent restyles its subtree, so the
  panes under a Paned (arranged before the Paned is parented) take
  the stylesheets above it.

### Data control: the clipboard-manager protocols

- `Application.DataControl` binds the seat's data-control device over
  ext-data-control-v1, or wlr-data-control-unstable-v1 where the
  compositor predates it. No surface or keyboard focus is needed.
- `DataControl.OnSelection` reports every change of the regular and
  the primary selection as a `SelectionOffer` carrying the owner's
  mime types in advertised order; `Own` marks our own claim coming
  back.
- `SelectionOffer.Read(mime, limit, done)` drains the pipe off the
  loop, bounded in size (`ErrTransferTooLarge`, refused whole) and
  time (`ErrTransferTimeout`), and delivers on the loop;
  `Receive` hands out the raw pipe.
- `DataControl.SetSelection(sel, SelectionSource)` claims either
  selection with validated mimes (none, empty, or duplicate are
  errors), answered by a `Data` payload written off the loop under a
  deadline or by `Send`, which hands the receiver's pipe over for
  deferred answers; `SelectionClaim` reports `Live`, `OnCancelled`, and
  `Release`. `ClearSelection` empties a selection.

### Menus from live models: row icons, disabled rows, sliding submenus

- `widget.MenuItem.Icon`: an `*Icon` (theme, file, SVG, or static)
  painted at the row's leading edge; once one row has an icon every
  label shifts by the same slot.
- `widget.MenuItem.Disabled`: greys a row out without dropping its
  action - no hover, keyboard motion, mnemonic, or activation, and its
  submenu does not open.
- `widget.MenuStack`: a Menu tree with sliding submenus (GTK's default
  PopoverMenu presentation). A submenu replaces the visible level,
  headed by a back row; Left goes back. It measures to the largest
  level, so one popover holds every level. `SetItems` swaps the tree
  for a live model update.
- `render.IconFromImage` and `render.IconFromARGB32`: icons from pixels
  that did not come from a file - any decoded image, or a network-order
  ARGB32 raster (the StatusNotifierItem IconPixmap format).
- `widget.ThemeIconExists`: whether a theme icon name resolves, for
  picking between candidate names before building a `NewThemeIcon`.

### Image: stretch and scale-down policies

- `ImageStretch` resamples the whole image to exactly the box,
  ignoring the aspect ratio.
- `ImageScaleDown` draws the natural size (one source pixel per device
  pixel, like `ImageNone`) when the image fits and shrinks it to fit
  like `ImageFit` when it does not; it never enlarges (GTK's
  `ContentFit.SCALE_DOWN`).
- An `Image` whose resolved size equals its source rect now blits the
  pixels directly instead of resampling through the cache - a 1:1 fit
  as well as `ImageNone`.

### Launcher-shaped surfaces: key capture, focus, resizing, clipboard

- `WindowConfig.KeyCapture` / `LayerConfig.KeyCapture`: a capture-phase
  hook that sees every press (repeats included) before built-in widget
  handling, accelerators, text routing, and `OnKey`, and consumes it by
  returning true. It receives an `Accel` normalized like accelerators
  (letters folded, caps lock masked), so it compares equal to
  `ParseAccel` of a binding. A launcher binds Return/Tab/Up to its list
  while every other key still types into the focused entry.
- `widget.Router.SetFocus`, `Window.SetFocus`, `LayerWindow.SetFocus`:
  programmatic keyboard focus. Widgets that take no keys, are disabled,
  or sit outside the tree are ignored; nil clears.
- `LayerWindow.SetSize`: resize a layer surface after creation, with
  the same auto-axis rule as the initial size enforced before the wire.
- `app.Clipboard`, `app.NewClipboard`, `app.ErrClipboardUnavailable`:
  external consumers can now construct the clipboard they hand to
  `SetClipboard`, and write to it from application code.

### Screen capture

- New `capture` package: a private capture connection (`Connect`)
  that feature-detects and binds wlr-screencopy, ext-image-copy-capture
  with its output and foreign-toplevel sources, ext-foreign-toplevel-
  list, Hyprland's toplevel-export, and linux-dmabuf. `CaptureOutput`
  and `CaptureOutputRegion` (screencopy, optional cursor, damage, one
  reused shm slot), `Toplevels` + `CaptureToplevel` (single windows),
  `CaptureOutputOnce`, `CaptureHyprlandWindow`, and the zero-copy trio
  `DmabufFormat` / `ImportDmabuf` / `CaptureOutputDmabuf`.
  `OpenOutputStream` is continuous, damage-driven output capture on its
  own goroutine (a static screen costs no copies). `Frame.Image`
  converts the 8-bit, 10-bit, and 24-bit formats compositors hand out
  to `image.RGBA`, honoring y-invert. Missing protocols fail with
  `ErrUnsupported`. The headless gate (`just headless`) now runs the
  package's compositor-in-the-loop tests.
- `wlr`: bindings for the six capture protocols.
- Cursor shape `"none"` (`wlsession.CursorHidden`) hides the pointer
  over a surface - a widget's `CursorName` can return it.
- `Application.Clipboard` returns the installed clipboard, so code that
  did not construct it (a screenshot copy) can still write to it.
- `app.Mods` and the `Mod*` bits alias the modifier mask the `OnKey`
  hooks receive.
- `Canvas.DrawImageDevice` paints `*image.RGBA` rasters straight from
  their pixel bytes (opaque pixels without a blend), so full-screen
  rasters repaint at memory speed.
- Fix: a `WakeAfter` timer firing after `Session.Close` no longer
  panics on the closed connection.

### Session lock: ext-session-lock-v1

- `Application.LockSession(SessionLockConfig)` locks the session: a
  lock surface per output built by `Surface(out)`, created right away
  for every output and again for each output plugged in while the lock
  holds; an unplugged output's surface is destroyed. `OnLocked` fires
  once the compositor confirmed the lock, `OnFinished` when it refused
  or ended it.
- `SessionLock.Unlock` (only after locked: `ErrNotLocked` before) and
  `SessionLock.Cancel` (only before locked: `ErrLocked` after) release
  the lock with the one request the protocol allows in that state;
  `ErrSessionLockActive` refuses a second lock, and
  `ErrSessionLockUnavailable` a compositor without the global.
- Lock surfaces make no initial commit and stay undrawable until their
  first configure is acked, and are sized exactly as configured.
- `LockSurface.Focus` focuses a widget as the surface is created;
  `SessionLock.SetFocus` moves focus later (a re-enabled password
  entry).
- `widget.Entry.OnActivate` fires with the contents on Enter, GTK's
  `activate`: the submit hook a password field needs.
- While a lock is held, `Run` keeps going with no window mapped.
- `Session.WatchOutputs` subscribes to output hotplug next to the
  single-slot `OnOutputAdded`/`OnOutputRemoved` hooks.

### Button: explicit transparent backgrounds

- `widget.Button.BgExplicit`: with it set, `Bg`, `BgHover`, and
  `BgPressed` are literal, so a zero color is transparent instead of
  "unset, use the stylesheet or the theme surface". A button can rest
  on whatever is behind it and fill only on hover or press. Without the
  flag nothing changes.
- A transparent fill under a stylesheet border now strokes the
  outline (`Canvas.BorderRect`) instead of painting the whole button
  in the border color.

### API consistency wave (breaks; #52)

One deliberate consistency pass before wayle and the 1.0 freeze. No
deprecation shims: wayle is the only consumer, and everything below
breaks on purpose. The conventions are also written down in the
`widget` package doc.

#### Constructors: one positional order

Every constructor is positional, in one fixed order — **face, then
sizePx (logical pixels), then the content/initial state, then
colors**. The majority already followed it; the outliers were
converted:

- `widget.NewLabel(face, text, sizePx, color)` →
  `NewLabel(face, sizePx, text, color)`
- `widget.NewRichLabel(face *render.Typeface, markup, sizePx, color)` →
  `NewRichLabel(face render.Font, sizePx, markup, color)` — the face
  parameter also widens from `*render.Typeface` to the `render.Font`
  interface every other text widget takes, so a `render.Chain` works
  here too.
- `widget.NewDropdown(items, selected)` →
  `NewDropdown(face, sizePx, items, selected)`. The two-phase
  `SetFace` is removed: a dropdown without a face used to paint a
  fixed placeholder and silently never open. `NewDropdownOf` follows
  the same order.

Everything else already conforms (Entry, TextArea, Menu, Toast,
Expander, Notebook, Button, Box, Grid, Scroll, List, Slider, Switch,
CheckButton, Spinner, Separator, Icon family, Image family, Stack,
Overlay, Fader, Elevation, Spacer, ProgressBar, app.Config-shaped
constructors).

#### Nil-face contract: panic at construction, naming the argument

Rule (b) of the two options: a constructor that takes a font either
receives a usable face or panics right there with a message naming the
constructor and the argument (`widget.NewLabel: nil face`). It never
fails later on the first `Shape` deep in shaping. Typed nils —
`(*render.Typeface)(nil)`, `(*render.Chain)(nil)` inside the interface
— count as nil; they used to slip past a `face == nil` check and
panic in glyph lookup.

- Applies to NewLabel, NewRichLabel, NewEntry, NewTextArea, NewMenu,
  NewToast, NewExpander, NewNotebook, NewDropdown, NewDropdownOf.
- `render.NewChain` enforces the same contract on its `primary`
  (`render.NewChain: nil primary`).
- App level keeps documented fallbacks instead: `ToastConfig.Face` nil
  falls back to the tooltip face, then the default sans face
  (`app.Application.resolveFace`); `app.Config.TooltipFace` nil still
  disables tooltips. If no face can be resolved at all, dialogs and
  message boxes return an error and toasts are dropped with a debug
  log — a nil face never reaches a widget constructor.

#### Events: exported fields

Decided once: **event hooks are exported function fields** set after
construction — `OnClick`, `OnChanged`, `OnSelect`, `OnToggled`,
`OnDismissed`, `OnClosed`, `OnResponse`, ... No `Set*`-style hook
setters exist or will be added. The Router and the handler interfaces
it dispatches to (`Clicker`, `KeyActionHandler`, `HoverSetter`,
`PressSetter`, `DragMover`, ...) are input plumbing, not app hooks,
and keep their `Set*`-shaped method names (`SetHovered`, `SetPressed`).

#### Getter/setter pairs

Every `Set*` whose state is app-meaningful now has a bare-name getter
(no `Get` prefix). Added where missing:

- `Label.Alignment`, `RichLabel.Alignment`
- `Entry.Placeholder`
- `TextArea.Placeholder`, `TextArea.Wrap`, `TextArea.Indent`
- `Icon.Tint`
- `Image.PlaceholderColor`
- `Grid.ColumnSpacing`, `Grid.RowSpacing`, `Grid.ColumnHomogeneous`,
  `Grid.RowHomogeneous`

`Bounds()` is defined before the first `Arrange`: it returns the zero
rect — never a panic, never stale geometry.

#### Units

Every coordinate, size, and pixel parameter in the API is logical
pixels; the device scale is applied exactly once, at the buffer
boundary. `sizePx` stays the name everywhere. The `Width`/`Height`
fields of `app.WindowConfig`, `app.LayerConfig`, and `app.DialogConfig`
are now documented as logical (surface) pixels, matching the internal
`layersurface.Config` and `popup.Config` docs.

#### Docs

`revive`'s `exported` rule is enabled in `.golangci.yml`: every
exported symbol must carry a doc comment starting with its name, and
`widget`, `app`, and `render` keep package docs (the `widget` one now
spells out the conventions above). 25 missing/misplaced doc comments
fixed across `widget`, `app`, `render`, `internal/*`, and `wlr`.

#### Migration notes for wayle

- Reorder `NewLabel`/`NewRichLabel` arguments; pass the face where
  `NewDropdown` used to take items and drop `SetFace` calls.
- Never pass a nil face: load one first (`sysfont.Sans` or
  `render.LoadFont`); nil now panics in the constructor instead of
  failing at first paint.
- The docs quickstart program and the godoc examples reflect the new
  signatures.
