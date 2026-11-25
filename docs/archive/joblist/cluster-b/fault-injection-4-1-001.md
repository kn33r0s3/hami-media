# Jobs 4.6 + 4.1 — Deterministic fault injection + lost P-frame retransmit — DONE (router-level; Mac e2e OPEN)

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdicts

- **4.6 DONE** (suite written, CI-ready: pure `go test`, no network,
  no CI exists yet — cluster A owns CI).
- **4.1 DONE at router level; OPEN for the full-network leg on the Mac.**

## What was built

- `faultinject.go` (new): `faultInjector` — drop by RTP sequence
  number, by per-subscription write ordinal, by per-subscription frame
  ordinal; drop-once by default (retransmit passes), `DropRetransmits`
  for drop-always. Counters: dropped total / by-sequence / by-ordinal /
  by-frame, allowed, seen packets/frames (`Stats()`).
- Seam: `sfuSubscription.writeRTP` — the single seam for originals
  (via `forwardQueuedRTP`) and retransmits (via `handleNACK`). A dropped
  packet reports unsent with no error, exactly as a network loss would.
- Wiring: `sfuSubscription.faultInjector` field;
  `sfuRouter.faultInjectors map[string]*faultInjector` +
  `SetFaultInjector(viewerID, injector)`; `subscribe()` attaches the
  viewer's injector; `sfuProofConfig.faultInjection
  map[string]*faultInjector`, installed on the router in
  `runLocalSFUProofWithConfig` before viewers subscribe (viewer IDs are
  `hami-sfu-viewer-%d`). Nil everywhere means no injection — existing
  proofs are unaffected (no proof config sets it).

## Tests (`faultinject_test.go`, all passing, race-clean)

4.6 unit tests: drop-by-sequence-once (retransmit passes),
drop-by-ordinal, drop-by-frame (both packets of the frame),
drop-retransmits-always (3 attempts all drop), rule precedence
(sequence > ordinal > frame, one count), nil injector is a no-op,
concurrent use (8 goroutines x 1000 calls, `-race` clean).

4.1 router-level test `TestFaultInjectionLostPFramePacketRetransmitted`:
10-packet stream (frame 0 keyframe seqs 1000–1004, frame 1 delta seqs
1005–1009), all cached on the source via `rememberPacket`; injector
drops seq 1007 on egress through the real path
(`enqueueRTP` → `nackWorker` → `forwardQueuedRTP` → `writeRTP`):
9 writes land, injector dropped 1; the viewer's `nackGapTracker`
reports exactly `[1007]`; `handleNACK` retransmits from cache and the
write lands (10 writes total; injector allowed 10, seen 11;
`NACKRetransmitted == 1`). The harness uses a real PeerConnection +
`AddTrack` (SSRC bound at AddTrack, no ICE/SDP/UDP — verified by probe).

Fail-before control `TestFaultInjectionDropRetransmitsControl`: same run
with `DropRetransmits` — the retransmit is dropped too, writes stay 9,
`NACKRetransmitted == 0`. This proves the 4.1 test would catch a broken
retransmit path.

## OPEN — full-network leg (Mac)

`runLocalSFUProof` with `sfuProofConfig.faultInjection` set (one viewer
dropping a P-frame packet) needs real UDP pairs, which the VM sandbox
blocks. Exact Mac command (from the repo root on the Mac, after the
coordinator merges tree-b):

```
go run . --video-file test-video.h264 --frame-rate 5 --proof local-sfu ...
```

— with a proof-config caller that sets `faultInjection` for
`hami-sfu-viewer-1` to `newFaultInjector().DropSequence(<a P-frame seq>)`.
No such caller exists yet: wiring a `--fault-inject` flag (or a
proof-config entry point) into the Mac harness is part of the Mac leg,
not done here. The router-level proof above is the evidence that the
seam, the injector, and the retransmit path work; the Mac leg proves
the same through real ICE/DTLS/SRTP.

## Evidence

- `faultinject.go`, `faultinject_test.go` (new); `main.go` (seam + wiring).
- `go test -run 'TestFaultInjector|TestFaultInjection' -v` — 9/9 pass.
- `go test -race` on the same set — clean.
