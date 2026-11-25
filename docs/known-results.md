# Known-results register (job 0.5)

Append-only. Every past failure is recorded here with its
conditions and its measured cause, or marked "cause unknown".
Never edit an entry in place: a correction is a new entry that
cites the old one. Sources are the working notes in the
hami-media-work directory; file names below are those notes.

## 2026-11-25 — 9×100 720p30 heap stop (user-supplied summary)

- Conditions: reportedly the original 1280×720, 30 fps H.264 stream,
  unchanged; 9 publishers × 100 viewers; 60-second run.
- Outcome: **FAILED**. The supplied summary says Go heap reached 942 MB
  against a 900 MB budget at about 8 seconds, triggering the safety stop.
  No viewer count, frame count, or p99 verdict was produced.
- Evidence limitation: the exact command, raw stderr, source path, and
  source hash were not supplied. This entry records the summary only; the
  run was not replayed or independently verified.
- Interpretation: this does not establish a 900-viewer limit or a
  capacity verdict. The default budget scales with viewer count, not
  resolution or frame rate. Do not change the cap to relabel this run as
  passing. See [the heap budget analysis](heap-cap-18.5.md).

## 2026-11-25 — 20×5,000 latency failure, fixed2 binary (cap20x5000-result-001.md)

- Conditions: 20 publishers × 250 viewers, 60 s, fixed2 binary
  (sha `fd73952f…`), strict gates p95 ≤ 100 ms, p99 ≤ 150 ms.
- Outcome: FAIL. p99 median 196.22 ms, p95 median 141.27 ms,
  overall pass 493/5,000. Frames complete (5,000/5,000 survived,
  feedback 5,000 each, zero drops, cleanup verified).
- Measured cause: latency, not loss. The run completed with no
  feedback-deadline error (FIX 19 held) and zero queue drops;
  latency alone failed the gates. The mechanism that made p99
  196 ms while p50 stayed 1.49 ms is "cause unknown" — recorded
  as such, not guessed.

## 2026-11-25 — 10×6,000 blocked by host preflight (cap10x6000-result-001.md)

- Conditions: 10 publishers × 600 viewers, fixed25 binary
  (sha `ff84b3cd…`), strict gate, ordered 24 Nov 22:49 NPT.
- Outcome: BLOCKED — no measurement. The binary's own host
  preflight refused to start: free swap 1,154.375 MiB, below the
  binary's 2,048 MiB floor, stable across three samples 20 minutes
  apart.
- Measured cause: the free-swap floor could not be met because the
  swap pool had shrunk; free swap is pool accounting, not a
  distress signal. This failure led to the used-swap rule
  (sfuHostMaxUsedSwapMiB = 512) replacing the free-swap floor.
  No gate was lowered to force the run.

## 2026-11-25 — Swap zero-total readings (swap-zero-001.md, swap-redo-001.md)

- Conditions: swap readings on the reference machine, Nov 2026.
  Zero totals appeared in terminal output and in the harness
  (used 0, free 0).
- Outcome: the zero readings were a glitch, not a healthy state.
  Cross-check (sysctl `vm.swapusage` ×3, `vm_stat`,
  `memory_pressure`, Activity Monitor, harness sampler) found no
  zero-glitch in 15 repeated reads: total 1,024.00 MiB, used
  256.44 MiB, free 767.56 MiB, identical across reads.
- Measured cause of the original zeros: "cause unknown" — the
  glitch mechanism itself was never identified. Fix landed
  (fixed27): `readSwapMiBChecked()` re-reads once on a zero total
  and fails closed if it still reads 0; a zero total is never
  treated as healthy. Mechanism identification is job 18.3 [!]
  and stays open here.
- Consequence (from cluster E's staged 18.3 entry, merged 25 Nov
  2026 — no second entry created): until the glitch is identified,
  the no-pool case is UNVERIFIED in the zero-total edge case; a
  freshly rebooted machine with no swap pool sits at
  `host_preflight=UNKNOWN` (retrying).
- Live evidence for the re-read path (cluster-C full-suite log,
  25 Nov 2026 08:17 UTC): two zero-total reads occurred in the VM.
  The first re-read still returned 0.00/0.00/0.00 (the twice-zero
  fail-closed path exercised live); the second re-read recovered
  128.00/896.00/1024.00 MiB. The re-read path now has live evidence
  in both directions; the glitch mechanism itself is still
  unidentified.

## 2026-11-25 — Swap thresholds unreachable (job 18.2; swap-zero-001.md, cap10x6000-result-001.md)

- Conditions: thresholds 2,048 / 2,500 MiB free swap and 8 GiB
  (job-list record); reference pool 1,024–3,072 MiB and shrinking.
- Outcome: the 2,500 MiB free-swap rule is superseded — after a
  swap clean the pool shrank 3,072 → 2,048 MiB and free swap was
  1,682 MiB, so free ≥ 2,048 would need used ≈ 0. The 2,048 MiB
  floor blocked the 10×6,000 run above.
- Measured cause: the OS resizes the pool, so free swap is capped
  by the live pool size. Current rule judges by used swap /
  memory pressure instead. The owner's pick of one reachable
  threshold is still owed.

## 2026-11-25 — p50 stall gate passes a deliberately stalled run (stallgate-ctrlA-001.md)

- Conditions: 20×8,000, fixed27 binary (sha `0fe2c109…`),
  GOMAXPROCS=1 (verified live in parent and all 4 cells),
  p50-median > 10 ms gate under test.
- Outcome: gate PASSED a stalled run. p50 median 4.76 ms (0 of
  8,000 viewers above 10 ms); p99 median 104.91 ms; strict gate
  FAILED (7,797/8,000, 202 viewers p99 > 150 ms).
- Measured cause: the p50 median is insensitive to this failure
  mode — no p50 threshold separates the stalled run from passing
  runs (passing p50 medians 1.66–3.70 ms; control A 4.76 ms;
  worst single-viewer p50 7.52 ms, below the highest passing
  7.90 ms). The p50 gate is dead; replaced by the p99-median
  > 50 ms gate (stallgate-new-001.md).

## 2026-11-25 — 7,500 latency failure, then two passing re-runs (stallgate-recheck-20x7500-001.md)

- Conditions: 20×7,500, 20:11Z run failed with p99 median
  221.33 ms and an unstable window.
- Outcome: overturned twice — recheck-2 (03:00Z, p99 median
  19.51 ms) and recheck (03:50Z, p99 median 19.03 ms, 7,500/
  7,500 strict pass) both pass. There is no 7,500 cliff.
- Measured cause of the original 20:11Z failure: "cause
  unknown" — attributed in the note to run conditions, not a
  ceiling, on the strength of the two passing re-runs, not on a
  diagnosed mechanism.

## 2026-11-25 — CPU profile straddles the media window (stall-evidence-001.md)

- Conditions: 20×8,000 stall run, fixed26 binary; CPU profile
  taken with a 60 s delay from parent-process start.
- Outcome: the profile (20:46:01–20:46:11Z) covers only 3.6 s of
  the 61 s send window (20:45:04.6–20:46:04.6Z) and 6.5 s after
  it. It does NOT cleanly cover the media window.
- Measured cause: the 60 s delay from parent start lands ~57 s
  after cell start, i.e. at the tail of the window. No failing
  run was ever profiled in-window — stated, not hidden.

## 2026-11-25 — Variant A frame shortfall at 20×8,000 (stallgate-varA-20x8000-001.md)

- Conditions: 20×8,000, varA binary (no-op Pion logger factory,
  sha `53fa6837…`), strict gate.
- Outcome: FAIL strict on frames only — 7,600/8,000; 400 viewers
  on stream-0015 got 301 frames; video_frames_sent 6,039 of
  6,040. Latency passed (p99 median 24.46 ms).
- Measured cause: publisher-side — one publisher sent 301
  frames. Same shape as the T2/T3 frame failures in the
  12-trial reconciliation (301-frame shortfalls, publisher-side,
  on unmodified binaries). Why one publisher sent one frame
  short: "cause unknown".

## 2026-11-25 — 20×8,000 twelve-trial reconciliation (REPORT.md §4.5)

- Conditions: 20×8,000, twelve trials (T0/T1/T4/FINAL/
  verify-s005/stall-capture and others).
- Outcome: six strict passes (T0/T1/T4/FINAL/verify-s005/
  stall-capture); T2/T3 frame-fail (301-frame shortfalls,
  publisher-side); 4 catastrophic runs.
- Measured cause: T2/T3 shortfalls are publisher-side and their
  mechanism is "cause unknown". The 4 catastrophic runs are
  recorded in REPORT.md §4.5; their causes are as stated there
  and are not re-decided here.

## 2026-11-24 — Goroutine and stack figures disagree (stall-evidence-001.md, goroutine-inventory-001.md)

- Conditions: ladder runs at 20×5,000. One cell showed 75,184
  goroutines (30.07/viewer); stacks measured 32 MB/cell.
- Outcome: the two figures are NOT reconciled — they disagree
  and no fresh same-binary run has closed the gap. A per-pid
  profile patch (sha `e0e9bda9…`) is verified but NOT landed.
- Measured cause: "cause unknown" until the fresh run lands.
  Small-scale inventory (2×10 viewers) measured ~35 goroutines
  per viewer (~16 per PeerConnection) and is not a substitute
  for the at-scale reconciliation.

## 2026-11-25 — VM baseline: 6 sandbox UDP failures (this round, job 1.5)

- Conditions: VM Linux suite, `go test ./...`, pre-change tree
  (`main.go` sha `9b253faf…`), Go 1.27.4.
- Outcome: 150 pass / 6 fail. The 6 failures are the sandbox
  blocking unconnected UDP sends (`sendto: operation not
  permitted`, ICE pairs failed): TestLocalPairReceivesAudioAnd-
  VideoRTP, TestCountingUDPMuxCountsPeerWritesWithoutClosing-
  SharedMux, TestPacketReadCountingConnCountsDatagrams,
  TestLocalSFURoutesAuthorizedStreamsAndRTCP,
  TestLocalSFUCascadeRoutesRTCPAcrossWorkers,
  TestSFUTURNRelayCarriesMedia.
- Measured cause: the sandbox, not the code — all six fail at
  the UDP send / ICE-connect step with no application error.
  Full per-test table: `docs/archive/joblist/cluster-a/baseline-vm-2026-11-25.md`.
