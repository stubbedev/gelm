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
| gallery frame measure+paint | 24 | 29 | **9** | `widget.TestAllocBudgetGalleryFrame` |
| Shape of a stable line | 0 | 0 | 0 | `render.TestAllocBudgetShape` |
| DrawAligned over a fixed box | 0 | 0 | 0 | `render.TestAllocBudgetDrawAligned` |
| canvas primitive mix | 0 | 0 | 0 | `render.TestAllocBudgetCanvasPaint` |
| idle loop tick | 0 | 0 | 0 | `app.TestAllocBudgetIdleLoopTick` |

The budgets stay at their baseline-derived values: they are regression
ceilings, not assertions of today's numbers.

## Bounded debt

Allocation and syscall sites the profiles show that stay, each named
with its cost and why:

- **`[]rune`→`string` conversions that key the shaping cache**
  (`TextArea.shapeLine`, `caretX`, `panToCaret`, `richlabel`): ~5-6
  allocs per text-widget frame (each ~32-64B). The cache is
  string-keyed; removing the conversions means rune-slice keys (Go
  slices are not comparable, so a map key is out) or hashing runes
  (a hash per probe per frame to save one small alloc). Not worth the
  complexity.
- **`CollectDamage`'s rect list and `hostWindow.draw`'s device rect
  mapping** (`widget.CollectDamage` growth + `app/window.go` `rects :=
  make(...)`): ~3 allocs per frame. Both slices are RETAINED by
  callers (`hostWindow.lastDamage`, the harness) — reusing a buffer
  would alias previously returned results, an observable behavior
  change.
- **`render.NewScaled` per frame** (`app/window.go` draw): one Canvas
  struct (~150B) per painted frame. Pooling means either a sync.Pool
  in render or a window-owned canvas with a reset method — API churn
  for one small allocation.
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
- **Damage-walk snapshot buffers** (`widget.walkStack`): retained at
  the deepest tree the process has walked (a few hundred bytes to a
  few KB for a 48-row list). Freed never; the bound is tree depth.
- **IME composing display** (`Entry.displayText` composing branch,
  `TextArea.displayLine`): converts per read while composing —
  keystroke-time, not frame-time; the steady state never hits it.

Syscalls after the sweep: unchanged by design — the fixes were
allocation-shaped, and the syscall profile's only gelm-side lever
(requests per frame) was already near minimal. Idle remains 0
syscalls/sec; the wheel tick's syscall burst shrinks with the tween
work it no longer restarts.
