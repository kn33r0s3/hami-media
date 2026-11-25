# Job 5.10 — Per-viewer memory budget — DONE

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdict: DONE (measured, documented, enforced; Mac re-measurement OPEN)

## Measured (VM, `memorybudget_test.go`, `-count=3`)

One idle viewer = a Pion PeerConnection + downtrack + a
`sfuSubscription` with production queues (64-packet RTP, 64-entry
NACK) + nackWorker. HeapAlloc after a full GC, median of 3 per step.

| Viewers | HeapAlloc |
|--------:|----------:|
| 0 | 339,224 B |
| 10 | 810,488 B |
| 20 | 1,277,096 B |
| 40 | 2,213,672 B |

Least-squares slope over the 4 steps: **46,840 B/viewer** (run 1);
repeats 43,994 / 43,567 — **≈ 44–47 KB per idle viewer**. The
steps are linear, so the slope is a real per-viewer cost.
In-flight variant (every queue filled with 64 × 1,200 B packets,
shared payload): **+4,425–7,108 B/viewer**.

Raw log: `log (sha256 5aa3206a6cdc, 30 lines; kept outside repo)`.

## Decision: the 500,000 B estimate is RETAINED

The VM number is a floor — the harness builds unnegotiated PCs
(no ICE/DTLS/SRTP state), and the live Mac cost is unmeasured.
Lowering the estimate to the VM floor would be a guess in the
other direction. The estimate has ~10× headroom over the floor,
and the formula's ×2 multiplier is genuine headroom on top.
`docs/per-viewer-memory-budget.md` records the numbers, the
formula (`max(viewers × 500,000 × 2, 256 MiB)`), the 269-viewer
floor crossover, the 4,000-viewer point where the budget exceeds
the 4 GB default memory limit (the limit binds first; the budget
is an abort threshold, not a target), and the scope: HeapAlloc
only — stacks (~42 KB/viewer floor, 5.7) and transport buffers
are outside it.

## Enforcement

`TestPerViewerMemoryBudgetMeasured` fails if the measured slope
exceeds 500,000 B or the measured total exceeds the formula's
budget for that viewer count. A code change that adds per-viewer
heap above the estimate must revisit the estimate, not silently
absorb it.

## OPEN — Mac re-measurement

Read the `runtime_resources_sample` HeapAlloc series from the cell
stderrs of a Mac scale run (5.7 sampler) and divide the HeapAlloc
rise by the viewer count. Tighten the estimate only on that
number.
