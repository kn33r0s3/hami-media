# Per-viewer memory budget

Measured in the VM, 25 Nov 2026 (Job 5.10). Test:
`TestPerViewerMemoryBudgetMeasured` (`memorybudget_test.go`).
Raw log: `log (sha256 5aa3206a6cdc, 30 lines; kept outside repo)` (working notes).

## What was measured

HeapAlloc per viewer at object level: one idle viewer is a Pion
PeerConnection with a downtrack plus a `sfuSubscription` with
production queue sizes (64-packet RTP queue, 64-entry NACK queue)
and a nackWorker. No ICE/SDP/UDP — the VM sandbox blocks it — so
these are **unnegotiated** PeerConnections. HeapAlloc is read after
a full GC, median of 3 readings per step.

| Viewers | HeapAlloc (bytes) |
|--------:|------------------:|
| 0 | 339,224 |
| 10 | 810,488 |
| 20 | 1,277,096 |
| 40 | 2,213,672 |

Least-squares slope: **46,840 bytes/viewer** (run 1). Three
repeat runs: 46,840 / 43,994 / 43,567 — **≈ 44–47 KB per idle
viewer**. The steps are linear (per-step deltas 47,126 / 46,661 /
46,829), so the slope is a real per-viewer cost, not baseline
noise.

In-flight variant: the same 40 viewers with every 64-packet queue
filled with 1,200-byte video packets (shared payload, one copy per
slot) adds **4,425–7,108 bytes/viewer** (≈ 64 packets × ~100 B of
struct/header per copy; the payload bytes are shared, not copied).

## The budget formula

`defaultSFUHeapBudgetForViewers(viewers)` =
`max(viewers × 500,000 × 2, 256 MiB)`.

This is a viewer-count estimate, not a frame-size, bitrate, or frame-rate
model. It has not been validated as a safe budget for 720p30. A supplied
9×100 720p30 run summary reports the 900 MB cap stopping the run at 942 MB
after about 8 seconds, but the command, raw stderr, and source hash were
unavailable, so those values are not independently verified. Do not treat
the summary as a capacity verdict or tune the cap to pass it.

- The **500,000 B per-viewer estimate is retained**, not lowered to
  the measured 44–47 KB. The VM number is a floor: live Mac viewers
  carry negotiated transport state (ICE/DTLS/SRTP) that the VM
  harness has no way to build, and the live cost is unmeasured.
  The estimate has ~10× headroom over the VM floor; the ×2
  multiplier is genuine headroom on top of that, not cover for an
  underestimate. Tightening the estimate needs the Mac
  re-measurement (see below), not a guess from the VM floor.
- The **256 MiB floor** dominates below 269 viewers
  (256 MiB / 1,000,000 B per viewer). At 40 viewers the formula
  gives 268,435,456 B against a measured total of ~1.9 MB.
- The formula's budget exceeds the default 4 GB memory limit
  (`debug.SetMemoryLimit`) at **4,000 viewers** (4,000 × 1,000,000
  = 4 GB; at 5,000 viewers the budget is 5 GB). The budget is an
  abort threshold for the heap monitor, not an allocation target,
  and the memory limit binds first at that scale — the monitor
  will not see a 5 GB HeapAlloc in a process capped at 4 GB. This
  is a property of the formula, not a measured failure.

## Scope: what the budget does NOT cover

The budget covers HeapAlloc only. It does not cover:

- **Goroutine stacks** (StackInuse, not HeapAlloc): Job 5.7
  measured a 2,621 B floor per parked goroutine; at ~16 live
  goroutines per PeerConnection that is ~42 KB per viewer of
  stack at the floor, and live stacks are deeper — roughly the
  same order as the heap cost, outside this budget.
- **Transport buffers** (ICE/DTLS/SRTP state, socket buffers):
  unmeasured in the VM, Mac-only.
- **The publisher's packet cache**: 512 packets × ~1,200 B ≈
  600 KB per publisher, shared across that publisher's viewers —
  a per-publisher cost, not a per-viewer cost.

## Enforcement

`TestPerViewerMemoryBudgetMeasured` enforces two things:

1. The measured per-viewer slope stays ≤ 500,000 B (the
   estimate). A code change that adds a per-viewer heap cost
   above the estimate fails the test, and the estimate must be
   revisited then — not silently absorbed.
2. The measured total heap for the largest step stays ≤ the
   formula's budget for that viewer count.

## OPEN — Mac re-measurement

Re-measure with negotiated PeerConnections on the Mac: HeapAlloc
per viewer at 5,000 viewers in a scale run (read the
`runtime_resources_sample` series from the cell stderrs — the 5.7
sampler logs HeapAlloc per second — and divide the HeapAlloc rise
by the viewer count). If the live per-viewer cost is far below
500,000 B, the estimate may be tightened then; if it is near or
above it, the estimate stays and the multiplier is doing its job.
