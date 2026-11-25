# Known limitations

Status: written 25 Nov 2026 for job 17.4. Updated each release.
Every failing test and every disabled feature is listed here. If a
limitation is not in this file, it is either fixed or it was missed
— report the miss, do not assume it away.

## Failing tests (VM, 25 Nov 2026)

Full suite: **150 pass, 6 fail** (`go test ./...`,
`main.go` sha `9b253faf…`). All six fail for one reason: the VM
sandbox blocks unconnected UDP sends, and Pion only ever sends
unconnected, so no ICE packet can leave either side (sent=0,
received=0). They pass on the reference machine and on real CI
runners. They are sandbox failures, not code failures.

1. `TestCountingUDPMuxCountsPeerWritesWithoutClosingSharedMux`
2. `TestLocalPairReceivesAudioAndVideoRTP`
3. `TestLocalSFUCascadeRoutesRTCPAcrossWorkers`
4. `TestLocalSFURoutesAuthorizedStreamsAndRTCP`
5. `TestPacketReadCountingConnCountsDatagrams`
6. `TestSFUTURNRelayCarriesMedia`

## Disabled / missing features

- **SRTP replay protection OFF** in three places (viewer clients,
  publisher clients, router) — job 2.1, confirmed bug, fix owed.
- **H.264 multi-slice pictures overcounted as frames**
  (`readH264Frames`) — job 2.9, confirmed bug, fix owed.
- **H.264 only** — VP8/VP9/AV1 neither tested nor listed as
  supported (job 6.9). Simulcast layers collide on track keys
  (job 2.6).
- **No slow-viewer policy** — no step-down, no throttle, no
  isolation test (jobs 4.7, 5.11).
- **No room model** — one global router, one room per process
  (job 9.2). No admin API, no webhooks, no data channel.
- **No public signaling service or wired rate limits.** The current
  `/publisher` and `/viewer` API is private to the loopback M7-H proof
  harness. `limits.go` contains tested helpers but no endpoint middleware;
  per-IP limits on this listener would group every simulated client as
  `127.0.0.1`. Do not expose these routes as a public service. The fuzz
  targets are tracked separately (job 8.6).
- **No packager** — no HLS/CMAF output (jobs 13.1/13.2). No
  recording, no RTMP/WHIP ingest.
- **No multi-node** (§11) and no device matrix (§10.5, blocked on
  physical devices).
- **Swap-zero mechanism unidentified** (job 18.3, needs owner
  approval); the no-pool rule is unproven in the zero-total edge
  case. Swap thresholds 2,048/2,500 MiB are superseded (job 18.2);
  the owner's pick of one reachable rule is owed.
- **Stall gate proven vs CPU starvation only** (job 5.6); GC-pause
  and network-stall controls not run.
- **20x10,000 fails systematically** at the macOS kernel loopback
  limit (~98% ingress wait) — not a code limit, but the ceiling on
  the reference machine (REPORT.md §4.5).
- **`/health` returns ready unconditionally** — no failing-
  dependency -> not-ready behavior (job 14.4 partial).
- **Egress shards default off** (count 0); keep-or-delete decision
  owed under a matched benchmark (jobs 2.10, 5.9).
- **No power figures anywhere** — by rule (job 18.6, done): no
  result contains a power figure without a meter measurement.
- **No Docker image built yet** (no daemon in the VM) and no
  digest pin on the base images (job 17.1, partial).
- **No supported SDKs.** `sdk/web/hami.js` is an experimental prototype
  that calls routes the server does not implement; see `docs/sdk.md`.
- **No CI run yet** (job 15.3, job content written, wiring owed to
  cluster A). No nightly run yet (15.4, staged). No 24 h soak yet
  (15.8, staged; pilot blocked by the VM UDP sandbox).
