# Job 5.9 — Egress-shard keep-or-delete — KEEP (decision recorded)

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdict: KEEP the shard path. No deletion.

Matched benchmark `BenchmarkEgressShardVsDirect` (new in `bench_test.go`):
same 1200-byte video packet, same viewer count, same drain semantics;
each iteration fans one packet out to every viewer and waits until every
write lands, so ns/op is the true end-to-end fan-out cost, not
enqueue-only. Direct: one `copyRTPPacketWithSharedPayload` per viewer
(`enqueueRTP`) + one `nackWorker` goroutine per subscription. Shard:
one copy per shard (`shard.enqueue`) + one `run()` goroutine per shard
doing the fan-out serially. Downtrack is unbound, so `WriteRTP` is a
no-op return — the benchmark measures the fan-out machinery (copies,
channel hops, `forwardQueuedRTP`), not network writes.

## Numbers (VM, `go test -bench`, `-benchtime 1s`)

Run 1 (`log (sha256 88007c0f6ebb, 11 lines; kept outside repo)`):

| arm | ns/op | B/op | allocs/op |
|---|---|---|---|
| direct / 100 viewers | 223,506 | 5 | 0 |
| shard4 / 100 viewers | 61,213 | 0 | 0 |
| direct / 1000 viewers | 2,035,413 | 529 | 1 |
| shard4 / 1000 viewers | 680,831 | 3 | 0 |

Repeat (`-count=3`, `log (sha256 49f23f5f561c, 19 lines; kept outside repo)`):

| arm | ns/op (3 runs) |
|---|---|
| direct / 100 viewers | 215,972 / 313,953 / 234,564 (mean ~255k) |
| shard4 / 100 viewers | 71,458 / 57,167 / 70,635 (mean ~66k) |
| direct / 1000 viewers | 1,569,503 / 1,237,028 / 1,209,476 (mean ~1.34M) |
| shard4 / 1000 viewers | 959,662 / 914,452 / 975,334 (mean ~0.95M) |

Shards win in all 6 comparisons: ~3.9x at 100 viewers, ~1.4x at 1000
viewers (4 shards), and 0 vs 143–529 B/op allocs at 1000 viewers. The
pool keeps both arms' allocs low; the ns/op gap is copies (1 per shard
vs 1 per viewer) plus per-viewer goroutine wakeups (100/1000 nackWorkers
waking per packet vs 4 shard goroutines).

## Why the margin narrows with N (honest caveat)

At fixed shard count 4, the shard path serialises each shard's fan-out
through one goroutine (250 sequential `forwardQueuedRTP` calls at 1000
viewers), while the direct path parallelises across per-subscription
goroutines. The win shrinks from 3.9x (100 viewers) to 1.4x (1000).
At 5000 viewers with 4 shards the serial chain (1250 writes) may lose
to direct — shard-count tuning (e.g. 16–32 shards) at Mac scale is
OPEN and is the right next measurement, not deletion.

## Coupling caveat (affects 4.7 / 5.11)

The shard path couples viewers: one slow viewer stalls its whole
shard's `run()` loop (head-of-line blocking), where the direct path
isolates slow viewers behind per-subscription queues. This benchmark
has no slow viewer, so it does not measure that cost. The keep
decision stands, but 4.7's slow-viewer policy must apply at the shard
level too, and 5.11's isolation test must cover the shard path.

## What this does NOT claim

- Not a Mac-scale result: 20x5000 verification on the Mac is still
  owed (exact command for the coordinator's Mac run is in the final
  report). VM numbers only.
- Not a real-network result: unbound downtrack, no ICE/DTLS/SRTP cost.
- NACK handling is not in the benchmark (both paths handle NACKs
  differently: `shard.enqueueNACK` vs per-subscription `nackWorker`).

## Decision

- KEEP `sfuEgressShard` (type, methods, env `HAMI_SFU_EGRESS_SHARDS`
  parsing, wiring in `newSFURouter`/`registerTrack`/`subscribe`/
  `forwardTrack`, shard tests). Job 2.10 is NOT closed by deletion;
  it stands as answered by this measurement instead.
- Default stays 0 (off, operator opt-in). Changing the default needs
  Mac-scale evidence + the shard-count tuning above; that is a separate
  decision for the coordinator, not taken here.

## Evidence

- `bench_test.go`: `BenchmarkEgressShardVsDirect` (new).
- `log (sha256 88007c0f6ebb, 11 lines; kept outside repo)`
- `log (sha256 49f23f5f561c, 19 lines; kept outside repo)`
