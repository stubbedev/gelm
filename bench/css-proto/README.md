# CSS prototype (docs/css.md, ticket #75)

A minimal match+compute engine over a synthetic widget gallery
(~200 nodes, 3 deep, 27 rules with element/class/id/state selectors
and descendant/child combinators), measuring the bar the real engine
(ticket #76) has to meet. Throwaway code: the design doc is the
artifact, these benches only produce its numbers.

Run:

    go test ./bench/css-proto -bench . -benchmem

Numbers on the recording machine (Intel i5-10400F, Go 1.27, Linux,
2026-09-28, commit with this file):

| Benchmark | Result |
| --- | --- |
| FullRestyle (200-node tree, all styles recomputed) | ~59 µs/op, 261 allocs/op |
| SingleRestyle (one widget, class toggle) | ~274 ns/op, 1 alloc/op |
| SteadyFrameReadsStyles (paint-path read of computed styles) | ~880 ns/op, 0 allocs/op |

Reading: a full restyle is three orders of magnitude under the 16.6 ms
frame budget; the steady-state paint path pays nothing (the bench is
the read-only walk the paint path performs, and it allocates nothing).
The prototype's allocations are in the naive candidate assembly and
the per-call map; the real engine reuses per-widget buffers and a
stack-allocated rank table, so #76's numbers should be at or under
these, not above them.
