# Heap cap per run from the measured budget (18.5)

Job 18.5: the cap is in the run config and triggers in a test.

Status: **DONE 25 Nov 2026.**

## The cap in the run config

For `sfu-single` / `m9h` runs, `main()` sets

```
maxHeapBytes = defaultSFUHeapBudget(publishers, viewersPerPublisher)
```

unless `-max-heap-bytes` was given explicitly (in which case the
explicit value wins and is logged). The budget formula is measured:

- **500,000 estimated heap bytes per viewer x2 headroom**, with a
  **256 MiB floor** (`defaultSFUHeapBudgetForViewers`, `main.go`).
- 20 publishers x 400 viewers (the 20x8,000 anchor):
  8,000 x 500,000 x 2 = **8,000,000,000 bytes**.
- 20 x 375 (the 20x7,500 anchor): **7,500,000,000 bytes**.
- 1 x 1: the 256 MiB floor.

Separately, when `GOMEMLIMIT` is unset the binary hardens the
runtime limit to 4 GiB (`debug.SetMemoryLimit(4_000_000_000)`,
`main.go:1093-1095`); the per-run cap above is the tighter,
measurement-derived budget for scale runs.

## The trigger

- `goHeapBudgetExceeded(heapAlloc, maxHeapBytes)`: triggers when
  `HeapAlloc >= maxHeapBytes` (at the cap triggers, one byte below
  does not — the `>=` is pinned by test, after the first draft of
  this test assumed `<` and failed, which is itself the verification
  that the test reads the code rather than the comment).
- `startHeapBudgetMonitor` polls every 100 ms and cancels the run
  on trigger; the startup check `checkGoHeapBudget` refuses to
  start over budget.

## Tests

- `TestHeapCapPerRunFromMeasuredBudget` (`host_safety_test.go`):
  pins the 20x400 and 20x375 budgets, the 256 MiB floor, and the
  trigger predicate (below/at/above the cap, and disabled at 0).
- `TestHeapBudgetMonitorCancelsProofWhenLimitIsReached` and
  `TestHeapBudgetCanBeDisabled` (`main_test.go`): the monitor's live
  cancellation and the disabled case.
- `TestDefaultSFUHeapBudgetUsesViewerEstimateWithSmallRunFloor`
  (`main_test.go`): the formula at 20x50.

All pass (`go test -run 'TestHeapCapPerRun|TestHeapBudget' -count=1`).
