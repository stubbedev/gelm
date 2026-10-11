# Allocation and syscall baseline

What the hot paths cost, how the numbers are measured, and the
allocation budgets the test suite enforces. The budgets are hard
ceilings: a regression past one fails `just check`.

## Method

Machine: Intel i5-10400F (6 cores, 12 threads), Go 1.27, Linux. Each
benchmark runs six times; the tables give the median.

    go test -run '^$' -bench . -benchmem -count 6 ./widget ./render ./app
    go test -run TestAllocBudget -v ./app ./widget ./render

`just bench` runs every benchmark once; CI archives that output for
trend watching and does not gate on it, since shared runners are too
noisy for timing. Allocation counts are deterministic, so the budget
tests gate those instead.

A change to a hot path is measured with benchstat over six runs
before and after, and leaves the goldens byte-identical
(`git diff -- '*testdata/golden*'` empty).

## Widget gallery

`buildShowcase` is the gelm-hello window shape: 73 widgets. The style
benchmarks load a 25-rule stylesheet over the same tree.

| benchmark | sec/op | B/op | allocs/op |
|---|---|---|---|
| ShowcaseMeasure (cold measure, cache defeated) | 4.6ns | 0 | 0 |
| StaticTreeMeasure (warm, cache hits) | 4.0ns | 0 | 0 |
| ShowcaseArrange (per-frame layout walk) | 13.6µs | 0 | 0 |
| ShowcasePaint (full frame: measure+arrange+damage+paint) | 2.28ms | 802 | 4 |
| ProgressOnlyFrame (one bar animating, damage-clipped) | 198µs | 33 | 1 |
| EntryPaint (steady entry repaint) | 410µs | 27 | 0 |
| StyleGalleryFullRestyle (match+compute, whole tree) | 64.3µs | 0 | 0 |
| StyleGalleryClassToggle (single restyle) | 940ns | 0 | 0 |
| StyleGalleryPaint (styled steady frame) | 2.23ms | 794 | 4 |

A styled frame paints in the unstyled frame's time: the paint path
reads the computed values and never restyles.

## Input events

Per event, over the gallery. A gesture delivers tens of events per
frame, so each allocation here would multiply.

| benchmark | sec/op | B/op | allocs/op |
|---|---|---|---|
| PointerMove (hover hit walk) | 307ns | 0 | 0 |
| WheelOverScroll (one wheel tick into a scrolled list) | 265ns | 0 | 0 |
| DragHover (drag-hover walk) | 274ns | 0 | 0 |

A hit walks the ancestors' `clip-path`s, since a clipped-out region
takes no pointer events.

## Render path

| benchmark | sec/op | B/op | allocs/op |
|---|---|---|---|
| Shape200 (warm shaping cache) | 74ns | 0 | 0 |
| Shape200Cold (cache bypassed) | 83.2µs | 20.0Ki | 10 |
| ChainShape200 (fallback chain, warm) | 79ns | 0 | 0 |
| DrawText (warm atlas) | 26.3µs | 1 | 0 |
| DrawTextCold (atlas drained) | 461µs | 84.2Ki | 388 |
| DrawAligned (warm) | 23.2µs | 2 | 0 |
| CanvasPaint (clear+fill+border+rounded+gradient) | 323µs | 0 | 0 |

## App loop

| benchmark | sec/op | B/op | allocs/op |
|---|---|---|---|
| ProgressOnlyFrames (real `hostWindow` draw, fake wire) | 120µs | 336 | 1 |

The idle tick over a wired window allocates nothing
(`app.TestAllocBudgetIdleLoopTick`), and an idle loop schedules no
wakeup at all (`app.TestIdleLoopDoesNotWake` pins zero wakeups across
a 200ms idle window).

## Steady-state syscalls

Measured with the compositor in the loop, in a private test
environment only: `just test-env <dir>` and
`just --set test_dir <dir> test-build`, then the showcase client
launched under `strace -f -tt` inside that environment (its own
`XDG_RUNTIME_DIR` and socket, never the desktop session). The pointer
phases are driven with the `just move`, `click`, `axis`, `sweep` and
`dwell` recipes against the same directory, and syscall entries are
counted from the timestamped trace, resume lines excluded. The client
is the 640x470 showcase on headless sway (pixman) at scale 120/120.
Raw counts per phase: `bench/data/syscalls-by-window.txt`.

| phase | window | syscall entries | rate |
|---|---|---|---|
| idle (no timers armed, loop parked in epoll) | 2.9s | **0** | **0 syscalls/sec** |
| pointer move into window (enter+hover+cursor load) | 2.0s | 236 | one-shot; the first cursor-theme load is cached after |
| button click + 600ms tween (~38 frames) | 0.8s | 3640 | ~96/frame ≈ 4550/s |
| two wheel axis ticks over the list | 5.0s | 1198 | per-tick burst |
| sweeps + tooltip dwell | 5.0s | 1675 | |

Per animated frame during the tween: 28.4 `nanosleep`, 20.9
`epoll_pwait`, 14.5 `futex`, 11.4 `recvmsg`, 8.7 `read`, 6.4 `sendmsg`
and 4.8 `write`.

Idle costs nothing: the parked loop holds when nothing is armed, and
`loopKicker` coalesces the timer kicks. The animated-frame cost is
dominated by the wire and the Go runtime; see the known costs below.

## Budgets

Each budget is the ceiling of 1.2x the measured count, and a change
that lowers a count tightens its budget in the same commit. They run
in `just check` through `go test`.

| budget | measured | ceiling | gate |
|---|---|---|---|
| gallery frame: measure+arrange+damage+paint | 1 (the harness's rect append) | 2 | `widget.TestAllocBudgetGalleryFrame` |
| whole-tree cascade rebuild | 0 | 0 | `widget.TestAllocBudgetStyleFullRestyle` |
| sibling-selector restyle of a wide sibling set | 0 | 0 | `widget.TestAllocBudgetSiblingRestyle` |
| input-method composing display reads | 0 | 0 | `widget.TestAllocBudgetComposingDisplay` |
| Shape of a stable line (warm cache) | 0 | 0 | `render.TestAllocBudgetShape` |
| ShapeRunes of a stable rune slice | 0 | 0 | `render.TestAllocBudgetShapeRunes` |
| DrawAligned over a fixed box (warm) | 0 | 0 | `render.TestAllocBudgetDrawAligned` |
| canvas primitive mix repaint | 0 | 0 | `render.TestAllocBudgetCanvasPaint` |
| idle loop tick | 0 | 0 | `app.TestAllocBudgetIdleLoopTick` |
| key delivery through window and app hooks | 0 | 0 | `app.TestAllocBudgetKeyDelivery` |
| icon reap over many windows with no icons | 0 | 0 | `app.TestAllocBudgetIconReap` |
| file watch drain | 0 | 0 | `app.TestAllocBudgetFileWatchDrain` |
| damage walk under a visible toast | 0 | 0 | `app.TestAllocBudgetToastDamageWalk` |
| damage-restricted window frame | 0 | 0 | `app.TestAllocBudgetWindowFrame` |

Two app gates count work instead of allocations:
`app.TestAllocBudgetPumpScopedDamageWalk` (a drained `Invoke` that
touched no tree runs no damage walk) and
`app.TestAllocBudgetTooltipPainterPass` (a settled tooltip runs no
paint pass, an owed frame exactly one).

The CSS engine has its own allocation-free gates:
`internal/style`'s `TestComputeVarFreeAllocatesNothing` and
`TestSelectorFormsAllocateNothingPerMatch` (the matcher and cascade),
`widget.TestEffectsFastPathAllocationFree` (a frame with no rendering
effect), and `widget.TestCSSLayoutSteadyFrameAllocatesNothing` and
`widget.TestCSSGridSteadyFrameAllocatesNothing` (static flex, flow and
grid trees).

## Known costs

Allocation and syscall sites the profiles show, each with its cost and
why it stays:

- **`widget.CollectDamage`'s rect list**: about one allocation per
  dirty frame. The returned slice belongs to the caller; reusing a
  shared buffer would alias earlier results, and a capacity hint
  would spend an allocation on every walk, including the clean ones
  the zero-allocation gates pin. Clean frames allocate nothing.
- **`render.NewScaled` per non-window canvas** (`app/dragdrop.go`,
  `app/print.go`): one Canvas per dropped-image paint and per printed
  page. That is event-time work; the frame path reuses the window's
  canvas (`Canvas.Reset`).
- **One `sendmsg` per Wayland request, one `recvmsg` and `read` per
  inbound message** (the neurlang/wayland binding,
  `Context.SendRequest` → `writeRequest`): the wire cannot batch below
  one syscall per request without buffering inside the binding. A
  frame is attach, damage per rect, commit and frame callback, and the
  damage is usually one or two rects: about 6 `sendmsg` and 20
  read-side syscalls per animated frame.
- **The Go runtime's background monitor**: about 28 `nanosleep` calls
  per animated frame during tweens, none at idle. It is not gelm code.
- **Walk snapshot buffers** (`widget.walkStack`, `styleStack`,
  `walkTreeStack`, the per-parent sibling lists) and **the rune-keyed
  shaping table**: the buffers are retained at the deepest tree the
  process has walked, plus one filtered child list per parent with
  sibling selectors (a few hundred bytes to a few KB for a 48-row
  list); the rune table is fixed-size (512 sets × 4 ways). They are
  never freed; the bound is the tree's shape.
