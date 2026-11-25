# Job 5.7 — Reconcile goroutines + stacks — DONE (32 MB retracted; 75,184 confirmed)

Historical evidence note: raw log availability is tracked in
[`../../log-evidence-status.md`](../../log-evidence-status.md).

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdict: DONE — one figure retracted, one confirmed, sampler landed

## The reconciliation

### 75,184 goroutines — CONFIRMED

From the r12 cell-0 goroutine profile (20×10,000 ladder run,
2,500 viewers/cell → **30.07/viewer**), read on the VM with symbols.
It matches the runtime-sample counts exactly. No contradiction
remains on this figure.

### 32 MB stacks — RETRACTED as a stack total

The "32 MB stacks per cell at 20×5,000" figure came from gctrace
lines (`gc 25 ... 32 MB stacks`). It is NOT the stack total. Go
1.27.4 source (`runtime/mgc.go:1627`, `runtime/mgcpacer.go:213,920`):
the gctrace "stacks" field is `gcController.lastStackScan` — **the
bytes of stack SCANNED during one GC cycle**, not `StackInuse`.
Proof from the raw lines themselves
(`resource-pprof-midmedia-002.log`): cell-0's gc 26, one cycle after
gc 25's "32 MB stacks", reports **0 MB stacks** with the same live
goroutines — a stack total cannot be 0. The 32 MB bounds what one
cycle scanned; it says nothing about the total.

### The true stack total (derived from an empirical measurement)

`TestSampleRuntimeResourcesParkedGoroutines` (`resources_test.go`):
1,000 parked goroutines → StackInuse delta 2,621,440 bytes =
**2,621 B per goroutine** (RSS delta 11,001,856 bytes; no leak on
release). Parked goroutines have shallow stacks, so 2,621 B is a
FLOOR for the per-goroutine stack; live Pion goroutines (DTLS/SRTP/
ICE call chains) are deeper. Go stacks start at 2,048 B.

- 20×5,000, 4 cells (cell-0..3 in the mid-media log), 1,250
  viewers/cell, ~37,500 goroutines/cell: stacks ≥ 75 MB (2 KB
  minimum), ≈ **98 MB** at the measured floor — and higher live.
- 20×10,000, 75,184 goroutines/cell: stacks ≥ 150 MB, ≈ **197 MB**
  at the floor.

The 32 MB figure is ~3–6x below the floor. It is retracted.

### Consequence for the RSS gap

`stall-evidence-001.md` §7.7 concluded "the gap is NOT mostly
stacks" — that conclusion rested on the misread 32 MB. With stacks
at ~100 MB+ per cell (and plausibly 150–300 MB live), they are back
as the LEADING candidate for the ~150–270 MB/cell RSS-minus-
HeapInuse gap. The gap attribution is REOPENED, not closed; the
sampler below settles it on the next Mac run.

## The sampler (`resources.go`)

- `sampleRuntimeResources()`: NumGoroutine, `MemStats.StackInuse`,
  `StackSys`, `HeapAlloc`, RSS (`readProcessRSSBytes`:
  `/proc/self/statm` on Linux, `ps -o rss=` on Darwin, honest error
  on Windows). An RSS failure is recorded in the sample, not hidden.
- Wired into `startHeapBudgetMonitor`'s 1 s ticker beside the heap
  sample (`runtime_resources_sample={json}` per process), so each
  cell produces its own series. The next Mac scale run's stderr
  carries the per-cell StackInuse + RSS needed to close the gap
  attribution: compare sampled StackInuse against RSS − HeapAlloc
  per cell.

## Per-pid profile patch — LANDED (ported, not file-replaced)

The verified patch (`main-profilepid.go`, sha `e0e9bda9…`) is a
whole-file copy of an OLDER main.go; replacing tree-b's main.go with
it would have wiped all Cluster B work, so its substance was ported
instead: `profilePathForPID` is byte-IDENTICAL to the patch
(verified by diff), applied to the heap + CPU profile paths (the
patch's two call sites) and to the new mutex profile path (5.1).
Cells no longer truncate each other's profile file.

## OPEN — Mac run to close the gap

Run a scale proof on the Mac and grep the cell stderrs for
`runtime_resources_sample=`: per cell, compare StackInuse with
RSS − HeapAlloc. That settles whether stacks dominate the gap.
No VM command can substitute — the VM has no Mac cells.
