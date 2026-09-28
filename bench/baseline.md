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

The after table, the fixes' benchstat deltas, and the bounded-debt list
land at the bottom of this file as the sweep closes.
