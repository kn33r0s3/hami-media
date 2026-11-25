# Job 4.4 — Forced-drop run: drops + keyframe recoveries, every drop accounted — DONE (router-level; PLI network leg Mac-only)

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdict: DONE at router level.

Test `TestFaultInjectionForcedDropRun` (`faultinject_test.go`), two
phases, one router, three subscriptions. All numbers from the test's
own log lines (run output):

## Phase A — single-packet drops, all retransmit-recovered

- 3 single packets dropped on egress (seqs 3002, 3007, 3012, drop-once).
- Accounting: `injected=3 retransmitted=3 keyframeRecoveries=0`.
- So: injected == retransmitted + keyframe-recovered (3 == 3 + 0).

## Phase B, part 1 — whole-frame drop including retransmits

- Frame 1 (5 packets) dropped with `DropFrame(1).DropRetransmits()`.
- Accounting: `injected=10 (5 originals + 5 retransmit attempts)
  retransmitted=0 uniquePacketsLost=5`.
- The frame is unrecoverable by NACK: 10 drop events, 0 retransmitted,
  5 unique packets lost forever. Writes stay at 10 (frames 0 and 2).

## Phase B, part 2 — keyframe recovery via queue-full drops

- A second subscription, production queue size (64), no drain: 100
  packets (20 frames x 5) enqueued. Exact layout counts: 11 delta
  sheds + 5 keyframe evictions (oldest delta evicted) + 20 delta
  sheds = **36 enqueue sheds** (`rtpQueueDropped == 36`,
  `markNeedsKeyframe` set).
- Drain started: 64 packets written; the first keyframe's marker
  completes the recovery: **`rtpRecoverySamples == 1`**.
- Accounting: `enqueueSheds=36 keyframeRecoveries=1 writes=64`.

## Injector fix found by this test (worth recording)

`DropFrame` originally counted frames by timestamp CHANGES in arrival
order, so a retransmit arriving after a later frame counted as a NEW
frame ordinal and did NOT re-match the frame rule — phase B's
retransmits landed despite `DropRetransmits`. Fixed: frames are
identified by RTP timestamp (`frameOrdinals map[uint32]uint64`,
first-seen order), so a retransmit maps to its frame's ordinal and
drops again under `DropRetransmits`. (If originals arrive out of
timestamp order, the ordinal follows arrival order — documented in
`DropFrame`.) The 4.2 test comment was corrected to match.

## OPEN — PLI network send leg (Mac)

`requestRecoveryKeyframe` sends the PLI upstream via
`source.publisherPeer.WriteRTCP`, which needs `source.remote` /
`source.publisherPeer` (real peers) — the VM harness has neither, so
the function returns early. The router-side flag
(`markNeedsKeyframe`) and completion (`completeKeyframeRecovery` →
`rtpRecoverySamples`) are proven here; the PLI actually reaching the
publisher over the network is a Mac-run leg (`runLocalSFUProof` with
fault injection), same as 4.1.

## Evidence

- `faultinject.go` (frame-identity fix), `faultinject_test.go`
  (`TestFaultInjectionForcedDropRun`).
- `go test -run TestFaultInjectionForcedDropRun -v` — PASS with the
  accounting lines above.
