# Cluster C results — sections 6, 7, 8, 9 (25 Nov 2026)

Current status supersedes historical completion labels: the §8.5 rate
limiter helpers have unit tests but are not wired into the internal
loopback endpoints; no public signaling API exists. See the current
[API boundary](../../../api.md) and
[known limitations](../../../known-limitations.md).

Working copy: `joblist-tree-c/`. `main.go` sha256
`9b253fafc2a73a251101fd4435c98ec2c6e17b0f07d94594b6d918b2294fd19a`
(UNCHANGED — no edits to main.go, auth.go or turn.go in this session).
Go 1.27.4, Pion webrtc v4.2.26, turn v5.1.3. `go.mod` changed ONLY
for the 8.7 vulnerability fix (x/net 0.50.0 → 0.56.0, x/crypto
0.48.0 → 0.53.0, x/sys 0.41.0 → 0.46.0, go directive 1.24.0 → 1.25.0).

## Test totals

- Baseline (before Cluster C): 117 test funcs; full suite fails with
  exactly the 6 known VM sandbox UDP failures
  (`log (sha256 1a00e43a065b, 71 lines; kept outside repo)`, 122.3 s).
- After Cluster C: 148 test funcs (117 + 31 new `TestClusterC*`).
  Full suite (`log (sha256 7a7d23b9c616, 72 lines; kept outside repo)`, 122.5 s) fails with
  EXACTLY the same six — no regression, no new failure:
  TestLocalPairReceivesAudioAndVideoRTP, TestCountingUDPMuxCountsPeerWritesWithoutClosingSharedMux,
  TestPacketReadCountingConnCountsDatagrams, TestLocalSFURoutesAuthorizedStreamsAndRTCP,
  TestLocalSFUCascadeRoutesRTCPAcrossWorkers, TestSFUTURNRelayCarriesMedia.
- Cluster C tests alone: 31/31 pass
  (`log (sha256 50cf5fc6724a, 137 lines; kept outside repo)`).
- `go vet`: clean. `gofmt`: clean on all new .go files.
- Leak scan (R2): grep over all new files for personal names,
  emails, home paths, real secrets — clean. Only test-only dummy
  values appear (`s3cret-turn-password`, already in turn_test.go;
  `test-secret-value`, `admin-secret`, `webhook-secret` — clearly
  fake placeholders, never real credentials).

## Per-job verdicts

### Section 6 — Codecs, layers, adaptation

| Job | Verdict | Evidence |
|---|---|---|
| 6.1 profile config file | DONE | `profiles.go` + `profiles.json` (top 720p30/1200 kbps, middle 360p30/600, base 180p15/200, keyframe 2 s); file round-trip + invalid-file rejection tested. |
| 6.2 profile-level-id | DONE | `negotiateProfileLevelID`: 42001f vs 42e01f negotiate (same Baseline profile/level, constraint flags ignored); 4d001f/64001f rejected; level downgrade works. Checked BEFORE fixing ours (see docs/cluster-c-codecs.md). |
| 6.3 keyframe 2 s + SPS/PPS | DONE | `keyframeDue` (2 s), `ensureParameterSets`/`frameHasParameterSets` (STAP-A + split NALs); bare IDR without SPS/PPS fails. |
| 6.4 simulcast ingest | DONE | `simulcastTracker`: 3 layers registered, tracked separately with own SSRCs; SSRC collision + unknown layer + unregistered packet rejected. OVERLAP FLAG: job 2.6 layer keying is cluster A's job; this tracker uses its OWN key type and does NOT touch `sfuTrackKey`. If 2.6's key lands, re-key this tracker onto it and re-run the test. |
| 6.5 layer selection | DONE | `selectLayerForBandwidth` from estimate; capped subscriber drops a layer instead of stalling. |
| 6.6 keyframe-aligned switching | DONE | Switch takes effect only at a keyframe; gap 300 ms ≤ 1 GOP passes, 3 s gap fails. |
| 6.7 bandwidth estimate | DONE | `bandwidthEstimator` tracks clean 1200 kbps and impaired 600 kbps links within ±20% (stated tolerance); floor 50 kbps. |
| 6.8 dynacast | DONE | Zero-subscriber layer: 0/100 packets sent, 100 suppressed; subscribed layer 10/10 sent. |
| 6.9 codec set per room | DONE | H.264 only, tested; VP8/VP9/AV1 rejected with named errors; doc `docs/cluster-c-codecs.md`. |
| 6.10 codec licensing review | NEEDS-APPROVAL | [!] Not started. |
| 6.11 PLI/FIR → keyframe | PARTIAL | VM part DONE: request → keyframe in 200 ms (deadline 500 ms); late keyframe counted late. Device-matrix part BLOCKED (no devices; job 10.5). |
| 6.12 bitrate control | DONE | 1 Mbps cap enforced (125000 B/s allowed, 1 B more refused); congestion reduces to 700 kbps floor; clean intervals recover to 1000. |

### Section 7 — Network

| Job | Verdict | Evidence |
|---|---|---|
| 7.1 STUN + srflx | PARTIAL | Candidate classification (host/srflx/relay) + STUN binding request proven in-process. Live NAT-simulated connection NOT run — VM sandbox blocks unconnected UDP. Reference-machine run owed. |
| 7.2 TURN time-limited creds | PARTIAL | `mintTURNTimeLimitedUsername` + expiry check proven (valid 30 min in, expired 2 h in, no-prefix fails closed). Symmetric-NAT relay media path = existing `TestSFUTURNRelayCarriesMedia` (one of the 6 VM failures; passes on reference machine). Production AuthHandler wiring for time-limited usernames owed (static-user handler in turn.go unchanged). |
| 7.3 TURN TLS/TCP | DONE (VM part) | pion/turn v5.1.3 DOES support TCP listeners (`ListenerConfigs`) + TLS via `tls.Listener` (readListener does the handshake). Implemented `startSFUTURNTCPServer`/`startSFUTURNTLSServer`; TCP dial succeeds; TLS 1.2+ handshake succeeds. Port-443 + public-cert deployment is a reference-machine step. |
| 7.4 ICE restart | PARTIAL | State machine + 5 s stated deadline proven (2 s recovery passes, 6 s fails). Live interface-switch recovery owed (needs a second interface). |
| 7.5 impairment matrix | DONE | In-process pipe, 12 cells (loss 0/1/3% × delay 30/100/150 ms + jitter/reorder/duplication), n=1000, seed 42, deterministic across runs. Measured: loss 1% → 6 lost, 3% → 25 lost; reorder → 500 OOO; duplication → 1000 dups; jitter 20 ms → max delay 120 ms. Exact settings saved per result. |
| 7.6 per-subscriber caps | DONE | Layer selection passes at 500 kbps → middle, 1 Mbps → top, 2 Mbps → top (min-kbps rule; the rung runs below its max under the 6.12 controller). |
| 7.7 two-machine LAN | BLOCKED | No second machine; VM sandbox blocks unconnected UDP. Recorded, not faked. |
| 7.8 cellular profile | DONE | `docs/cluster-c-cellular-profile.md` — bursty loss + handoff, labelled approximation, not a field test. |
| 7.9 field test | NEEDS-APPROVAL | [!] Not started. |

### Section 8 — Security and access

| Job | Verdict | Evidence |
|---|---|---|
| 8.1 threat model | DONE | `docs/cluster-c-threat-model.md` (assets, actors, trust boundaries, abuse cases). Open: second-person review owed. |
| 8.2 signed join tokens | DONE | Verified in this copy (auth tests pass; code untouched). |
| 8.3 auth on subscribe + layer switch | DONE | Per-path tests: publish-only token rejected on subscribe; layer switch requires subscribe role + allowed track; forged token rejected. |
| 8.4 input limits | DONE | SDP 64 KiB, candidates 64, tracks/participant 8, rooms/node 1024 — each with an exceed-the-limit test. |
| 8.5 rate limits | DONE | Joins 10/min/IP, 5/min/account; signaling 60/min/IP, 30/min/account; window rollover tested. |
| 8.6 fuzzing | DONE (VM part) | 4 targets (SDP, RTP, RTCP, signaling JSON) + `scripts/fuzz-smoke.sh` (fixed 60 s per target). Runs: SDP 47,554 execs, RTP 14,798 execs, RTCP + JSON 10 s each — no crashes. CI wiring owed (no CI exists; cluster A owns the workflow file). First SDP run failed on a FULL /tmp (512 MB tmpfs), not a crash — re-run with TMPDIR in home passed. |
| 8.7 vuln scan in CI | PARTIAL | `scripts/vuln-scan.sh` + govulncheck run. FOUND + FIXED: GO-2026-5942 (x/net v0.50.0 dnsmessage panic, reachable via webrtc NewPeerConnection) — `log (sha256 80093ecb77f3, 17 lines; kept outside repo)` (1 affecting) → bumped x/net to v0.56.0 → `log (sha256 bc96d586d1b7, 9 lines; kept outside repo)` (0 affecting). 12 vulnerabilities remain in required modules that the code does not call. CI wiring owed (cluster A owns the workflow file). |
| 8.8 secrets fail-closed | DONE | `requireSecret`: missing secret refuses to start; env + 0600-file sources work; HMAC loader same behaviour verified. |
| 8.9 TLS for signaling | PARTIAL | TLS 1.2+ signaling listener implemented + tested (TLS 1.1 client refused; MinVersion < 1.2 refused at construction). External TLS scan NOT run — no public endpoint in the VM; reference-machine step. |
| 8.10 DTLS/SRTP check | PARTIAL | SRTP: RFC 3711 Appendix B vectors (master key E1F97A0D3E018BE0D64FA32C06DE4139, salt 0EC675AD498AFEEBB6960B3AABE6 — corrected from a wrong salt I first wrote; see fail-before below) via pion/srtp v3.1.3: protect/unprotect round-trip byte-identical + tampered packet rejected. DTLS: relies on Pion dtls/v3 + existing local-pair tests; no separate DTLS vector run. The authoritative library vector suite command: `go test` in the pion/srtp module cache (not part of this repo's suite). |
| 8.11 penetration test | NEEDS-APPROVAL | [!] Not started. |

### Section 9 — Signaling, rooms, API

| Job | Verdict | Evidence |
|---|---|---|
| 9.1 signaling spec | DONE | `docs/cluster-c-signaling-spec.md` (v1) + `parseSignalingMessage`: 7 valid forms pass, 10 invalid forms rejected, oversize rejected. |
| 9.2 room model | PARTIAL | `roomManager`/`room`: create/list/delete, join/leave, publish/subscribe tracks, per-room profile, independent rooms — all tested. Implemented BEHIND the existing single-room behaviour; `main.go`'s global router is untouched. Owed: migrating the live router's publish/subscribe paths onto RoomManager (hot-path refactor, not smuggled in here). |
| 9.3 trickle ICE | DONE | Late candidates buffered in arrival order and applied; post-answer candidate accepted, not dropped. |
| 9.4 admin API | DONE | create/list/delete/kick/mute/permission-change, each tested; documented schema `docs/cluster-c-admin-api.md`; unauthenticated calls 401. Note: `mute` is stored state; the router does not yet drop muted audio (owed). |
| 9.5 webhooks | DONE | joined/left/track-published/room-ended deliver; failed deliveries retry (3 attempts, 1 s/2 s/4 s backoff — tested with injected no-sleep); HMAC signatures verified, bad/wrong-secret/wrong-body signatures rejected. |
| 9.6 session resume | DONE | 60 s window (matches audioReconnectWindow): resume inside restores state, after 61 s expires, single-use, unknown participant rejected. |
| 9.7 metadata propagation | DONE | Metadata change stored + fan-out audience = every participant in the room (tested, 3/3). |
| 9.8 data channel | PARTIAL | In-process message bus: message reaches every participant in the room, in order, no media involved, no cross-room leak. Real Pion DataChannel wiring on the PeerConnection is owed. |

## Fail-before / pass-after records (this session)

1. **Impairment pipe (7.5)**: first run `log (sha256 af9996a08619, 138 lines; kept outside repo)` —
   reorder cell produced 0 out-of-order deliveries and maxDelay was
   ~20010 ms (I sorted deliveries by DeliverAt, hiding reorder, and
   measured delay from run start instead of send time). Fixed the pipe
   (emission order preserved, per-packet SentAt); `log (sha256 50cf5fc6724a, 137 lines; kept outside repo)`
   — reorder 500 OOO, maxDelay = setting exactly.
2. **SRTP vectors (8.10)**: first run failed `encoding/hex: odd length
   hex string` — I had written the RFC 3711 master key/salt from
   memory and both were wrong. Corrected against pion/srtp's own
   `key_derivation_test.go` (TestValidSessionKeys_AesCm128); pass in
   `log (sha256 50cf5fc6724a, 137 lines; kept outside repo)`.
3. **Vulnerability (8.7)**: `log (sha256 80093ecb77f3, 17 lines; kept outside repo)` GO-2026-5942 affecting
   → `go get golang.org/x/net@v0.56.0` → `log (sha256 bc96d586d1b7, 9 lines; kept outside repo)` 0 affecting.
   Full suite after the bump: same 6 known failures only.

## Files changed / added (joblist-tree-c only)

New Go files: `profiles.go`, `adaptation.go`, `codecs.go`,
`impairment.go`, `ice.go`, `limits.go`, `security.go`, `rooms.go`,
`signaling.go`, `admin.go` (~3,645 lines incl. tests below).
New tests: `cluster_c_section6_test.go`, `cluster_c_section7_test.go`,
`cluster_c_section8_test.go`, `cluster_c_section9_test.go`,
`cluster_c_fuzz_test.go` (31 new test funcs).
New data/docs/scripts: `profiles.json`, `docs/cluster-c-threat-model.md`,
`docs/cluster-c-signaling-spec.md`, `docs/cluster-c-admin-api.md`,
`docs/cluster-c-codecs.md`, `docs/cluster-c-cellular-profile.md`,
`docs/cluster-c-results.md` (this file), `scripts/fuzz-smoke.sh`,
`scripts/vuln-scan.sh`.
Modified: `go.mod`, `go.sum` (vuln fix only — see above).
Untouched: `main.go`, `auth.go`, `turn.go`, all other existing files.

## Logs (in logs/ in this directory)

- `log (sha256 1a00e43a065b, 71 lines; kept outside repo)` — pre-change full suite (6 known failures).
- `log (sha256 af9996a08619, 138 lines; kept outside repo)` — Cluster C tests, first run (2 fail-before).
- `log (sha256 50cf5fc6724a, 137 lines; kept outside repo)` — Cluster C tests, 31/31 pass.
- `log (sha256 7a7d23b9c616, 72 lines; kept outside repo)` — post-change full suite (same 6 failures).
- `log (sha256 a692f7aa01cd, 233 lines; kept outside repo)` — first SDP fuzz, failed on full /tmp (infra, not a crash).
- `log (sha256 12e1f48d859f, 9 lines; kept outside repo)`, `log (sha256 c0f0cff95839, 9 lines; kept outside repo)`, `log (sha256 77b328addce2, 9 lines; kept outside repo)`, `log (sha256 12b1b3e366a1, 9 lines; kept outside repo)` — all 4 fuzz targets, 10 s each, no crashes.
- `log (sha256 80093ecb77f3, 17 lines; kept outside repo)` / `log (sha256 bc96d586d1b7, 9 lines; kept outside repo)` — before/after the x/net fix.
- `log (sha256 ef797d909a03, 6 lines; kept outside repo)`, `log (sha256 6fb0db69912b, 8 lines; kept outside repo)`.

## What remains (owed, not faked)

- 6.11 device matrix; 7.1 live NAT test; 7.2 AuthHandler wiring;
  7.3 port-443 deployment; 7.4 live interface switch; 8.6/8.7 CI
  wiring (cluster A owns the workflow); 8.9 external TLS scan;
  8.10 separate DTLS vector run; 9.2 live-router migration;
  9.4 mute enforcement in the forwarding path; 9.8 real DataChannel
  wiring; second-person review of the threat model (8.1).
- BLOCKED: 7.7 (no second machine). NEEDS-APPROVAL: 6.10, 7.9, 8.11.
