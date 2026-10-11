# Allocation and syscall baseline (#77)

Measured 2026-09-28 on the worktree at `0f94768` (main). The only code
added for this baseline is the two missing render benchmarks
(`BenchmarkDrawAligned`, `BenchmarkCanvasPaint`) — no production code
changed, so every number below is that commit's behavior. Machine: Zen
2 desktop (i5-10400F-class, 12 threads), Go 1.27, Linux. Raw data:
`bench/data/before-{widget,render}.txt` (six-run benchstat files),
`bench/data/syscalls-by-window.txt` (strace entry counts per window).

Reproduce the numbers with:

    go test -run '^$' -bench . -benchmem -count 6 ./widget ./render
    go test -run TestAllocBudget -v ./app        # idle-tick allocs

## The rule this document serves

Baseline first, budgets derived from the baseline, then fixes with
benchstat before/after evidence, no behavior change: goldens stay
byte-identical (`git diff -- '*testdata/golden*'` empty) and every gate
(`just check`, `-race`, `just headless`) stays green.

## Widget gallery (buildShowcase: the gelm-hello window shape, 73 widgets)

| benchmark | sec/op | B/op | allocs/op |
|---|---|---|---|
| ShowcaseMeasure (cold measure, cache defeated) | 4.32µ | 176 | 44 |
| ShowcaseArrange (per-frame layout walk) | 1.67µ | 57 | 5 |
| ShowcasePaint (full frame: measure+arrange+damage+paint) | 1.83ms | 2069 | **24** |
| StaticTreeMeasure (warm, cache hits) | 3.2ns | 0 | 0 |
| ProgressOnlyFrame (real app frame, one bar animating) | 135µ | 1496 | 22 |
| EntryPaint (steady entry repaint) | 352µ | 167 | 3 |
| StyleGalleryFullRestyle (CSS match+compute, whole tree) | 11.4µ | 1250 | 9 |
| StyleGalleryClassToggle (single restyle) | 169ns | 0 | 0 |
| StyleGalleryPaint (styled steady frame) | 1.82ms | 2053 | 24 |

## Render path

| benchmark | sec/op | B/op | allocs/op |
|---|---|---|---|
| Shape200 (warm shaping cache) | 66ns | 0 | **0** |
| Shape200Cold (cache bypassed) | 26.3µ | 20.0Ki | 10 |
| ChainShape200 (warm) | 68ns | 0 | 0 |
| DrawText (warm atlas) | 22.0µ | 1 | 0 |
| DrawTextCold (atlas drained) | 115µ | 81.4Ki | 377 |
| DrawAligned (warm, added with this baseline) | 19.6µ | 1 | **0** |
| CanvasPaint (clear+fill+border+rounded+gradient, added here) | 203µ | 0 | **0** |

## App loop

- Idle tick over a wired window (the `TestAllocBudgetIdleLoopTick`
  harness): **0 allocs/tick** measured before any budget existed.
- Idle wake schedule: `TestIdleLoopDoesNotWake` already pins zero
  wakeups in a 200ms idle window.

## Steady-state syscalls (compositor in the loop, private env only)

Protocol: `just test-env /tmp/gelm-test-env-77`, `just --set test_dir
/tmp/gelm-test-env-77 test-build`, then the showcase client launched
under `strace -f -tt` inside that env (its own XDG_RUNTIME_DIR and
socket, never the desktop session). Pointer phases driven with the
sanctioned `just move` / `click` / `axis` / `sweep` / `dwell` recipes
against the same dir; syscall entries counted from the timestamped
trace, resume lines excluded. Full log: 640x470 client on headless sway
(pixman), 120/120 scale.

| phase | window | syscall entries | rate |
|---|---|---|---|
| idle (no timers armed, loop parked in epoll) | 2.9s (+16s and 55s windows in earlier runs) | **0** | **0 syscalls/sec** |
| pointer move into window (enter+hover+cursor load) | 2.0s | 236 | one-shot; ~750 of these are first-ever cursor-theme loads, cached after |
| button click + 600ms tween (~38 frames) | 0.8s (pure tween slice) | 3640 | ~96/frame ≈ 4550/s |
| two wheel axis ticks over the list | 5.0s | 1198 | per-tick burst |
| sweeps + tooltip dwell | 5.0s | 1675 | |

Tween per-frame breakdown (~38 frames): 28.4 `nanosleep` + 20.9
`epoll_pwait` + 14.5 `futex` + 11.4 `recvmsg` + 8.7 `read` + 6.4
`sendmsg` + 4.8 `write`.

Readings that bound the sweep:

- **Idle is already free.** Zero syscalls for minutes at a time: the
  parked loop holds when nothing is armed, and the `loopKicker`
  coalescing does its job. Any idle-cost work would be a regression
  hunt, not a fix hunt; the idle-tick allocation budget (below) gates it.
- **The wire path cannot batch below one syscall per request**: the
  binding (`neurlang/wayland` `Context.SendRequest` → `writeRequest`)
  does one `sendmsg` per protocol request, and one `recvmsg`+`read`
  pair per inbound message. gelm already sends near the minimum: a
  frame is attach + damage-per-rect + commit + frame-callback, and
  damage usually collapses to one or two rects. Batching further means
  buffering inside the binding — out of scope for this sweep;
  documented as bounded debt instead.
- **~28 of the ~96 syscalls/frame are `nanosleep` from the Go runtime's
  background monitor** spinning up around activity. Not gelm code; no
  gelm fix applies.
- The remaining per-frame passes are the event loop iterating between
  frame callbacks (`epoll_pwait` 21/frame during tweens). The loop
  wakes per delivered event; reducing requests and proxy churn
  (sync-callback per kick) is the only gelm-side lever, and the kicker
  already caps the timer side.

## Budgets (ceil of 1.2x baseline; gated in `just check` via `go test`)

| budget | baseline | budget | gate |
|---|---|---|---|
| one full frame measure+paint over the gallery | 24 allocs | **29 allocs** | `widget.TestAllocBudgetGalleryFrame` |
| one Shape of a stable line (warm cache) | 0 allocs | **0 allocs** | `render.TestAllocBudgetShape` |
| one DrawAligned over a fixed box (warm) | 0 allocs | **0 allocs** | `render.TestAllocBudgetDrawAligned` |
| one canvas primitive mix repaint | 0 allocs | **0 allocs** | `render.TestAllocBudgetCanvasPaint` |
| one idle loop tick (app) | 0 allocs | **0 allocs** | `app.TestAllocBudgetIdleLoopTick` |

## After the sweep

Six fixes landed, each behind a benchmark, none changing behavior
(`git diff -- '*testdata/golden*'` empty; `just check`, `-race`, `just
fmt-check`, `just lint`, `just headless` green; raw data:
`bench/data/after-*.txt`). Per-fix deltas with n=6 benchstat runs are
in each commit message; this is the cumulative table:

| benchmark | before allocs/op | after allocs/op | before B/op | after B/op | sec/op |
|---|---|---|---|---|---|
| ShowcaseMeasure (cold measure) | 44 | **0** | 176 | **0** | -26% |
| ShowcaseArrange | 5 | **1** | 57 | 24 | -10% |
| ShowcasePaint (full frame) | 24 | **9** | 2119 | **792** | ~ (pixel-bound) |
| EntryPaint | 3 | **0** | 167 | 23 | ~ |
| ProgressOnlyFrame (app frame) | 22 | **6.5** | 1532 | **192** | -1.2% |
| StyleGalleryFullRestyle | 9 | 9 | 1250 | 1250 | -2.4% |
| StyleGalleryPaint | 24 | **9** | 2102 | **756** | ~ |
| PointerMove (per event, new) | - | **0** | - | **0** | 88ns |
| WheelOverScroll (per event, new) | - | **0** | - | **0** | 95ns |
| DragHover (per event, new) | - | **0** | - | **0** | 80ns |
| render Shape200 warm | 0 | 0 | 0 | 0 | ~ |
| render DrawAligned warm | 0 | 0 | 0 | 0 | ~ |
| render CanvasPaint | 0 | 0 | 0 | 0 | ~ |
| app idle loop tick | 0 | 0 | 0 | 0 | ~ |

App draw path (real hostWindow.draw, fake wire): 3 allocs/frame —
down from 8 at the baseline.

### The fixes

1. **Rune-keyed shaping cache** (`Font.ShapeRune`, render): the text
   area's wrap walk probed one rune at a time and built a one-rune
   string per probe just to key the string cache — 58% of the measure
   profile. ShowcaseMeasure 44→0 allocs, -20% time.
2. **One conversion per line** (text area): panToCaret, caretX, and
   Paint's row loop converted the same line twice (shape key + RTL
   read); Paint now converts per line entered, not per row.
3. **Cached entry display string** (`Entry.setRunes`): the display
   text was re-converted from runes on each of 4+ reads per frame.
   EntryPaint 3→0 allocs.
4. **Per-depth damage-walk buffers** (`appendChildren` + walkStack):
   the frame's damage walk took a fresh `Children()` copy per
   container per frame. ShowcasePaint 18→9 allocs after the earlier
   fixes, ProgressOnlyFrame 1472→192 B.
5. **Bar fade covers repeated wheel ticks** (`Scroll.fadeTo`): the
   scroll event path restarted its tween every tick (7 allocs per
   event); a running fade to the same target now keeps running.
   WheelOverScroll 7→0 allocs, 370→91ns.
6. **Calendar day-number table**: 31 string conversions per calendar
   frame became a lookup.

## Budgets after the sweep

| budget line | baseline | budget | after | gate |
|---|---|---|---|---|
| gallery frame measure+paint | 24 | 29 | **1** | `widget.TestAllocBudgetGalleryFrame` |
| whole-tree cascade rebuild | 9 | 9 | **0** | `widget.TestAllocBudgetStyleFullRestyle` |
| sibling-selector restyle | - | 0 | **0** | `widget.TestAllocBudgetSiblingRestyle` |
| composing display reads | - | 0 | **0** | `widget.TestAllocBudgetComposingDisplay` |
| Shape of a stable line | 0 | 0 | 0 | `render.TestAllocBudgetShape` |
| ShapeRunes of a stable rune slice | - | 0 | **0** | `render.TestAllocBudgetShapeRunes` |
| DrawAligned over a fixed box | 0 | 0 | 0 | `render.TestAllocBudgetDrawAligned` |
| canvas primitive mix | 0 | 0 | 0 | `render.TestAllocBudgetCanvasPaint` |
| idle loop tick | 0 | 0 | 0 | `app.TestAllocBudgetIdleLoopTick` |

After the #120 sweep the gallery-frame ceiling tightened from 29 to
ceil(1×1.2) = 2: the fix that lowers the baseline tightens its budget
in the same commit. The others keep their baseline-derived values;
they are regression ceilings, not assertions of today's numbers.

## The #120 sweep: widget/render allocation residuals

Measured 2026-10-10 on the worktree branched off `1b0c118` (main),
same machine and method. Each fix is gated by a budget test; the
numbers below are `go test -run '^$' -bench ... -benchmem` over the
worktree before and after the sweep. No behavior change: goldens stay
byte-identical and every gate stays green.

| benchmark | before allocs/op | after allocs/op | before B/op | after B/op |
|---|---|---|---|---|
| ShowcasePaint (full frame) | 24 | **4** | 3366 | 694 |
| ShowcaseArrange | 4 | **0** | 144 | 0 |
| StyleGalleryFullRestyle | 9 | **0** | 1280 | 0 |
| StyleGalleryPaint | 12 | **6** | ~1900 | 1198 |
| ProgressOnlyFrame (app frame) | ~ | **1** | ~ | 34 |
| render ShapeRunes warm (new) | - | **0** | - | 0 |

### The fixes

1. **Rune-keyed line shaping cache** (`Font.ShapeRunes`/
   `ShapeDirRunes`, render): the rune-holding widgets (text areas,
   composing entries) converted their lines to a string per probe to
   key the string cache; a set-associative table keyed on
   (font, size, direction, runes) serves them without the conversion.
   `ShapedText.RTL()` carries the resolved base direction, so the
   RTL reads stopped converting too. Gated by
   `render.TestAllocBudgetShapeRunes` (0 allocs).
2. **Composing display memoized** (`Entry.displayRunes`,
   `TextArea.displayLine`): while a preedit is on show, the splice
   and its string form are built once per preedit change, not per
   read; every content mutation clears the preedit first, so the
   memo cannot outlive its text. Gated by
   `widget.TestAllocBudgetComposingDisplay` (0 allocs).
3. **Sibling list memoized per parent** (`visibleSiblings`): the
   filtered sibling list is cached on the parent keyed by the damage
   generation — a sibling-selector restyle marks the whole set in one
   pass, so the rebuild happens once per parent per generation.
   Membership and visibility changes all mark damage, so no mark can
   be missed. Gated by `widget.TestAllocBudgetSiblingRestyle`.
4. **Per-depth snapshot buffers for the style walks** (`styleKids` →
   `appendStyleKids`, `walkTree`, `Box.shrinkTakes`): the restyle
   pass, the generic tree walk, and the shrink distribution each took
   a fresh slice per container per pass. StyleGalleryFullRestyle
   9→0 allocs. Gated by `widget.TestAllocBudgetStyleFullRestyle`.
5. **Two-slot measure cache** (`node.measureHit`): the cache keeps
   the last two constraint sets, absorbing the containers that
   measure a child twice a frame under two sets (a row's natural and
   shrunk passes) instead of thrashing a single slot.
6. **Toast slide damage clipped to the delta** (`Toast.slideExtent`):
   an animated frame invalidates the shadow ring grown by the slide
   distance the card moved through — the union of its last and
   current positions — not the full entrance rise.
7. **Window-owned canvas** (`Canvas.Reset`, app): the frame canvas is
   re-readied per painted frame instead of allocated (`NewScaled`
   per frame leaves the bounded-debt list).

### Dispositioned

- **Leaf restyles recompute the ancestor cascade per leaf**: already
  amortized — `style()` serves a cached cascade per generation, so a
  leaf's climb is pointer compares, not recomputation; a batched
  restyle-from-root pass would add machinery for nothing measurable.
- **List `findCheckButton` walk per row creation**: already cached —
  `newRow` stores the checkbox on the row proxy (`r.check`) and the
  subtree walk runs once per row entering the viewport, not per
  frame.
- **`CollectDamage` rect growth without a capacity hint**: the rects
  slice is retained by callers (`hostWindow.lastDamage`), and the
  zero-alloc gates (`app.TestAllocBudgetToastDamageWalk`,
  `app.TestAllocBudgetWindowFrame`) forbid an unconditional
  pre-allocation on clean walks. Growth stays amortized and only on
  dirty frames; it remains bounded debt below. `render.Rect.Subtract`
  (the old render/damage.go site) has no production caller.

## Bounded debt

Allocation and syscall sites the profiles show that stay, each named
with its cost and why:

- **`CollectDamage`'s rect list and `hostWindow.draw`'s device rect
  mapping** (`widget.CollectDamage` growth + `app/window.go` `rects :=
  make(...)`): ~2 allocs per dirty frame. Both slices are RETAINED by
  callers (`hostWindow.lastDamage`, the harness) — reusing a buffer
  would alias previously returned results, an observable behavior
  change, and a capacity hint would spend an allocation on every
  walk, including the clean ones the zero-alloc gates pin.
- **`render.NewScaled` per non-window canvas** (`app/dragdrop.go`,
  `app/print.go`): one Canvas struct per dropped-image paint and per
  printed page — event-time, not frame-time; the frame path pools its
  canvas (`Canvas.Reset`).
- **One `sendmsg` per Wayland request, one `recvmsg`+`read` per
  message** (neurlang/wayland binding, `Context.SendRequest` →
  `writeRequest`): the wire cannot batch below its per-request
  syscall without buffering inside the binding. gelm already sends
  near the minimum (attach + damage-per-rect + commit + frame
  callback, damage usually one or two rects); ~6 `sendmsg` + ~20
  read-side syscalls per animated frame.
- **Go runtime background monitor** (`nanosleep` loop during
  activity): ~28 syscalls per animated frame during tweens, zero at
  idle. Not gelm code.
- **Damage-walk and style-walk snapshot buffers** (`widget.walkStack`,
  `styleStack`, `walkTreeStack`, the per-parent sibling lists, the
  rune-keyed shaping table): retained at the deepest tree the process
  has walked plus one filtered child list per parent with sibling
  selectors (a few hundred bytes to a few KB for a 48-row list); the
  rune table is fixed-size by construction (512 sets × 4 ways).
  Freed never; the bound is tree shape.

Syscalls after the sweep: unchanged by design — the fixes were
allocation-shaped, and the syscall profile's only gelm-side lever
(requests per frame) was already near minimal. Idle remains 0
syscalls/sec; the wheel tick's syscall burst shrinks with the tween
work it no longer restarts.

## The #159 CSS engine against the restyle budgets

Measured 2026-10-11 on the #159 worktree (the CSS engine completion:
the conformance suite and its fixes, background layers, border-image,
the animation timing model), same machine, Go 1.27, with the machine
under other load (load average 7–11): every number is the minimum of
six interleaved runs per tree — `b9dc18d` (main before #159), `599f68d`
(main with every #159 tranche but this one) and this change — so the
three columns share the load. Absolute times sit above the table at
the top of this file, which was measured on an idle machine; the
comparison is between the columns.

    go test -run '^$' -bench . -benchmem -count 1 ./widget   # x6, interleaved per tree
    go test -run '^$' -bench . -benchmem -count 1 ./render   # x6, interleaved (599f68d, this change)
    go test -run TestAllocBudget -v ./app ./widget ./render

| benchmark | before #159 (`b9dc18d`) | #159 tranches (`599f68d`) | this change |
|---|---|---|---|
| StyleGalleryFullRestyle | 62.7µs, 0 allocs | 130.6µs, 0 allocs | **55.8µs, 0 allocs** |
| StyleGalleryClassToggle | 990ns, 0 allocs | 1860ns, 0 allocs | **859ns, 0 allocs** |
| StyleGalleryPaint | 1.97ms, 4 allocs | 2.01ms, 4 allocs | 1.95ms, 4 allocs |
| ShowcasePaint | 2.11ms, 4 allocs | 2.01ms, 4 allocs | 2.01ms, 4 allocs |
| ShowcaseArrange | 13.7µs, 0 allocs | 12.5µs, 0 allocs | 12.1µs, 0 allocs |
| ShowcaseMeasure | 4ns, 0 allocs | 5ns, 0 allocs | 4ns, 0 allocs |
| StaticTreeMeasure | 3ns, 0 allocs | 3ns, 0 allocs | 4ns, 0 allocs |
| ProgressOnlyFrame | 171µs, 1 alloc | 168µs, 1 alloc | 177µs, 1 alloc |
| EntryPaint | 399µs, 0 allocs | 372µs, 0 allocs | 387µs, 0 allocs |
| PointerMove | 1022ns, 0 allocs | 251ns, 0 allocs | 263ns, 0 allocs |
| DragHover | 944ns, 0 allocs | 239ns, 0 allocs | 236ns, 0 allocs |
| WheelOverScroll | 245ns, 0 allocs | 254ns, 0 allocs | 253ns, 0 allocs |
| render CanvasPaint | - | 321µs, 0 allocs | 324µs, 0 allocs |
| render DrawAligned | - | 21.7µs, 0 allocs | 22.5µs, 0 allocs |
| render DrawText | - | 25.6µs, 0 allocs | 24.6µs, 0 allocs |
| render Shape200 | - | 71ns, 0 allocs | 71ns, 0 allocs |

The pointer rows rerun with eight interleaved rounds, as do the three
render rows whose first six-round minimums moved by more than the
noise (a second run put them within 4%).

Every allocation budget holds: `widget.TestAllocBudgetGalleryFrame`,
`TestAllocBudgetStyleFullRestyle` (0), `TestAllocBudgetSiblingRestyle`
(0), `TestAllocBudgetComposingDisplay` (0), the render budgets (0) and
the app budgets (`TestAllocBudgetIdleLoopTick` 0 and the window,
toast, tooltip and key-delivery walks) pass, as do the 0-alloc
matcher (`TestValuesNoAllocationPerMatch`), the effect fast path
(`TestEffectsFastPathAllocationFree`) and the steady CSS layout frames
(`TestCSSLayoutSteadyFrameAllocatesNothing`,
`TestCSSGridSteadyFrameAllocatesNothing`).

### What moved

The #159 tranches had doubled the restyle: the computed `Values`
grew to 4.6 KB with the new property groups, and every restyle copied
it four times (the old values, a zeroed result, the cache store, and
two by-value hand-offs to the transition and animation runners),
diffed it field by field, then built both the layout and the
inheritance keys even when nothing changed; each compute also derived
the theme's polarity through three `math.Pow` calls and cleared the
matcher's per-longhand tables in full. The fixes:

1. **Restyle into a shared buffer, compare, and stop.** A recompute
   lands in one event-loop buffer and is compared with the cached
   values through `style.Values.Equal` — the comparable values sit in
   an embedded `Scalars` struct compared with one `==`, the list- and
   pointer-valued groups (backgrounds, transforms, transitions,
   animations, the clip, mask and border image, the counters) through
   their own `Equal`. An unchanged result returns without a copy or
   the key projections; the diff is complete by construction, where
   the hand-written one had missed fields (a background url, the
   variation axes, the 3-D part of a transform, the accent color and
   caret shape never repainted when they alone changed).
2. **Pointers to the runners**: the transition and animation runners
   take the values by pointer; the transitions copy only on the path
   that starts one.
3. **The theme's polarity is memoized** by its background color.
4. **The matcher clears only what the last compute offered**: the
   per-longhand winner tables stay zero outside the previous compute's
   offered set.

The full restyle is 2.3x faster than before this change and 11% faster
than before #159; the class toggle 2.2x and 13%. Paint, layout and
the event paths are unchanged within the noise; `PointerMove` is 5%
slower than `599f68d` because a hit now walks the ancestors'
`clip-path`s (CSS Masking's clipped-out region takes no pointer
events), still allocation-free.
