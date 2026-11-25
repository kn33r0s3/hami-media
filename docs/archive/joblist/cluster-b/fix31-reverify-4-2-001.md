# Job 4.2 — Re-verify FIX 31 result note against the current tree — DONE (code-level; Mac runs not re-verifiable in VM)

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Filename correction (carried from the brief)

The briefed note `cap20x5000-fix31-result-001.md` does NOT exist. The
FIX 31 record is `cap20x5000-f31-fix-001.md`
(fix + validation, commit `61e3480`); the 10x5000 result is
`cap10x5000-fix31-result-001.md`. This verification is against the
f31-fix note's code claims.

## Code claims — all verified in tree-b

1. **Cache the delta.** `rememberPacket` has no
   `layerDeltaCacheSkipped` early return (grep: zero matches for
   `layerDeltaCacheSkipped` in `main.go`/`layers_test.go`). The two
   remaining `if l == layerDelta` sites (`main.go:5599` shard enqueue,
   `main.go:9216` `enqueueRTP`) are queue-shed logic, not caching.
   Test `TestRememberPacketCachesDeltaLayer` passes: 100 packets /
   109,000 bytes cached, NACK for delta seq 3 served from cache.
2. **Re-scan the span on merge.** `a.lateComplete()` is called at
   three sites in `processPacket` (`main.go:14258` late branch,
   `14301` frame-change finalize, `14352` unknown-boundary merge) —
   the note's two finalize paths plus the pre-existing late-branch
   call. Verified by grep.
3. **Rescue whole frames.** `lateMarkerFrameStart` exists
   (`main.go:14659`) and is called from `lateCompleteOne` (`14581`).
   **CHANGE since the note (job 3.5, this session):** the note's guard
   "a lone marker never rescues on its own" (`frameStart < marker`)
   is SUPERSEDED in tree-b — a lone marker now rescues when its frame
   is proven single-packet (preceding sequence in the span with a
   different timestamp, or preceding sequence recorded in
   `completedMarkers`). Rationale, proof, and the 25-seed property
   tests are in `cluster-b/job35-property-tests-001.md`. The guard's
   purpose — never complete on incomplete evidence — is preserved by
   the proof requirement.
4. **Gate alignment.** `latency.go:37`: `MinFrames = 302`. Verified.
5. **Unit tests.** All three late-retransmit tests exist and pass in
   tree-b: `TestVideoFrameAssemblerLateRetransmitAfterNewerFramesStarted`,
   `TestVideoFrameAssemblerLateRetransmitBeforeNextFrameMerges`,
   `TestVideoFrameAssemblerLateRetransmitCompletesOnRetransmitArrival`,
   plus `TestVideoFrameAssemblerLateCompletionRecoversMarkerLoss` and
   `TestVideoFrameAssemblerLateCompletionRecoversMissedFrames`.

## What is NOT re-verified here

- The note's file shas are for fix commit `61e3480` (main.go
  `82389e28…`, 527,563 B). Tree-b is a later tree (main.go sha
  `9b253faf…` at session start, plus this session's 3.1–3.5, 4.x, 5.9
  changes), so shas do not match. Verification is by grep + tests on
  the current code, not by sha.
- The Mac runs in the note (f31fixi11 / f31fixrep injection runs:
  500/500 affected viewers at 302 frames; r1–r3 no-regression runs
  5000/5000; the latency bind — recovered viewers at p95 ~202 ms /
  p99 ~205 ms failing the strict gates) are Mac-scale measurements
  that cannot be re-run in this VM (no UDP, no scale harness). Their
  numbers are taken as recorded; nothing in tree-b contradicts them.
  A repeat on the Mac after the merge is the coordinator's call.

## Router-level e2e proof (new test, this session)

`TestFaultInjectionWholeFrameDropDoesNotBlockLaterFrames`
(`faultinject_test.go`): a 3-frame stream (15 packets) with frame 1
wholly dropped at `writeRTP` via `DropFrame(1)` — frames 0 and 2
still land (10 writes, injector dropped 5 by frame), so the missing
frame does not block later frames; NACKing frame 1's 5 sequences
retransmits all 5 from the cache (15 writes,
`NACKRetransmitted == 5`). PASS.

## Verdict: 4.2 DONE (code-level re-verification + router-level proof;
Mac-scale repeat OPEN as above).
