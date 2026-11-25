# CLUSTER D REPORT — job list sections 10, 11, 12, 13, 14

Current implementation caveat: the archived 10.1 prototype tests use
mocked networking and do not establish compatibility with the server's
internal loopback API. See the current [SDK status](../../../sdk.md).

Working copy: `joblist-tree-d/`
Base `main.go` sha256: `9b253fafc2a73a251101fd4435c98ec2c6e17b0f07d94594b6d918b2294fd19a`
After Cluster D `main.go` sha256: `da36017a562bee7813c6fb0b377f8cffe5c1baf783e05d7a36cd2c5b8760394d`
(only change: 7-line 14.4 readiness fix in `handleHealth`, lines 7750-7755)
`outputs/packager.go` sha256: `51a82ec8cd62d7192f3f3a9bcd7f59e4783d65edd5717004c1556aadffc175ca`

Baseline (before Cluster D): full suite fails exactly the 6 known
VM sandbox UDP tests, 122 s (`log (sha256 f6fc97b25b7c, 71 lines; kept outside repo)`).
After Cluster D: full suite fails exactly the same 6, nothing else
(`log (sha256 885dbda15423, 73 lines; kept outside repo)`, 122.8 s). `gofmt` clean,
`go vet ./...` clean. Leak scan: 0 replacements, 0 surviving
(`repo-scrub-20261125.py` over the tree). 32 Cluster D Go tests pass.

## Per-job verdicts

### Section 10 — Clients
| Job | Verdict | Evidence |
|---|---|---|
| 10.1 Web SDK | PARTIAL | `sdk/web/hami.js` + `test/web-sdk-test.mjs` PASS (join, publish, subscribe/ontrack, 3 layer switches, scrubbed error report, leave) — `log (sha256 1fb61a149820, 2 lines; kept outside repo)`. Gap: harness stubs RTCPeerConnection/fetch; no browser in VM, so the headless-browser test is owed on a machine with a browser. |
| 10.2 iOS SDK | BLOCKED | No physical iOS device. |
| 10.3 Android SDK | BLOCKED | No physical Android device. |
| 10.4 Playout tests | PARTIAL | Web DONE: `test/playout-test.mjs` — clean 0 stalls, impaired 2 stall events / 20 late frames (`log (sha256 24ae0d12414e, 2 lines; kept outside repo)`). iOS/Android BLOCKED with 10.2/10.3. |
| 10.5 Device matrix | BLOCKED | No physical devices. |
| 10.6 Client error reports | DONE | `client_errors.go` + `TestClusterDClientErrorReports`: good reports pass, token/email/SDP reports rejected; SDK `reportError()` scrubs before sending. |
| 10.7 Sample app | PARTIAL | Web DONE: `sample-apps/web/` served from a fresh copy — index.html 200, sdk 200 (`log (sha256 617b11162909, 4 lines; kept outside repo)`). iOS/Android BLOCKED. |

### Section 11 — Multi-node
| Job | Verdict | Evidence |
|---|---|---|
| 11.1 Node registry | DONE | `multinode.go` registry + `TestClusterDRegistryDrainClosesNoRooms`: drained node gets no new rooms, keeps its 3 rooms. |
| 11.2 Room placement | DONE | `TestClusterDPlacementSpreadsUnderLoad`: 30 rooms spread 10/10/10 across 3 nodes. |
| 11.3 Cascading | PARTIAL | Multi-worker in-process cascade DONE (`TestClusterDCascadeMultiWorkerDelivers`: 20 packets, in order, payloads unchanged, unknown room errors). Cross-machine cascade at target size: gap recorded — needs a second machine. |
| 11.4 Failure detection + recovery | DONE | `TestClusterDFailureDetectionAndRecovery`: node dead after 31 s without heartbeat; both rooms re-placed on the live node well inside the agreed 30 s. |
| 11.5 Shared-state decision | DONE | `docs/cluster-d/11-shared-state-decision.md` — in-memory + reconciliation, 5 failure modes written down. |
| 11.6 Multi-region decision | DONE | `docs/cluster-d/11-multi-region-decision.md` — single region; no multi-region claim. |

### Section 12 — Audio
| Job | Verdict | Evidence |
|---|---|---|
| 12.1 Opus 1x100 | DONE (in-process) | `TestClusterDOpusOneSpeakerHundredListeners`: 50 frames to 100 listeners, payload bytes identical (no transcoding). |
| 12.2 Audio level + active speaker | DONE | `TestClusterDActiveSpeakerFiresWithinStatedTime`: speaker change fires within the stated 500 ms. |
| 12.3 DTX + silence | DONE | `TestClusterDDTXSilentPeriodSendsOnlyKeepalives`: silent period sends only DTX keepalives (2 grace speech frames, then drops). |
| 12.4 Server mixing decision | DONE | `docs/cluster-d/12-mixing-decision.md` — mixing stays in-process, with the condition that would move it out. |
| 12.5 Audio rung benchmark | PARTIAL | In-process shape done (12.1). Real-pair VM attempt FAILS on the known VM UDP block (`log (sha256 b56ebb28d20f, 12 lines; kept outside repo)`: `read udp 127.0.0.1:39317: i/o timeout`, ffmpeg `Operation not permitted`). Mac run with same gates owed — command recorded in `docs/cluster-d/12-audio-rung-benchmark.md`. |
| 12.6 A/V sync per layer | DONE | `TestClusterDAudioVideoSyncPerLayer`: 50 ms passes, 120 ms fails the stated 80 ms tolerance, both directions. |

### Section 13 — Outputs
| Job | Verdict | Evidence |
|---|---|---|
| 13.1 HLS/CMAF packager | PARTIAL | OWN packager built (`outputs/packager.go`, `cmd/hami-packager`): 10 s clip -> 5 segments, ffprobe reads playlist (h264 640x360, 10.0 s), ffmpeg decodes all 300 frames with 0 errors (`log (sha256 e5f89b6f0442, 48 lines; kept outside repo)`). Gap: Safari + hls.js playback cannot be verified in this VM (no browser). |
| 13.2 Segments start on keyframes | DONE | `MediaSegment` refuses non-keyframe starts; `Packager.Close` drops leading non-keyframes; tests pass. |
| 13.3 CDN test | PARTIAL / NEEDS-APPROVAL (real CDN) | Edge-simulator only: hit ratio 0.5 cold -> rises on warm pass, start time measured (`TestClusterDEdgeSimulatorHitRatioAndStartTime`). Real CDN NOT started — needs owner approval. |
| 13.4 Glass-to-glass + start time | DONE (method + VM check) | `latency_measure.go`, `docs/cluster-d/13-latency-method.md`: WebRTC p50=p99=40 ms synthetic; HLS start 2.15 s (segment-dominated, inherent). Single-clock labelled; two-machine run needs NTP-synced clocks. |
| 13.5 Recording MP4/segments | DONE | `TestClusterDRecordingRoundTripAndDeletion`: ffmpeg MP4 remux plays back (ffprobe), retention expiry listed, deletion removes files + dir. |
| 13.6 Recording consent | DONE | Recording refuses without prior consent; consent record visible before start. |
| 13.7 RTMP/WHIP ingest | DONE (WHIP) | WHIP endpoint + Pion offer/answer test + ffmpeg WHIP encoder handshake (offers=1; media path stalls on VM UDP, exit 234 — recorded). OBS not installed; ffmpeg used as encoder, stated in `docs/cluster-d/13-ingest.md`. RTMP not implemented (decision recorded). |
| 13.8 Outputs as separate processes | DONE | Forwarder deps import `outputs` 0 times; forwarder and `cmd/hami-packager` both build separately (`log (sha256 8b86f4d7376c, 6 lines; kept outside repo)`). |

### Section 14 — Observability and operations
| Job | Verdict | Evidence |
|---|---|---|
| 14.1 Metrics per room/participant/track | DONE | `clusterDMetrics` + `hami_media_room_packets_total{room,participant,track,kind}`; metric set documented in `docs/cluster-d/14-metrics.md`. |
| 14.2 Structured logs | DONE | JSON logger + `TestClusterDStructuredLogsPassLeakScan` over 200+ lines; a log line with token+email+home path is caught. |
| 14.3 Dashboards + alerts | DONE | `observability/alerts.yaml` + `observability/dashboard.json`; test alert fires (`TestClusterDAlertFires`: below threshold no fire, before `for` window no fire, above threshold for full window fires). |
| 14.4 Health + readiness | DONE | **Fix with fail-before/pass-after**: `handleHealth` returned ready unconditionally; now 503 `not_ready` naming the failing dep. Fail-before log shows 200 `ready` with a failing dep; pass-after shows 503. (`log (sha256 1393b13fcee0, 8 lines; kept outside repo)`, `log (sha256 9df919ca9c74, 9 lines; kept outside repo)`). |
| 14.5 Graceful shutdown | DONE | `TestClusterDGracefulShutdownDrainsRooms`: 3 rooms drained, 0 leaked goroutines; above-baseline finish reported. |
| 14.6 Runbook per alert | DONE | `docs/cluster-d/14-runbook.md` — all 4 alerts link to entries; test asserts every rule has a runbook. |
| 14.7 Config validation | DONE | `validateClusterDServerConfig` names the field in every error; 6 bad-config cases tested. |
| 14.8 OS tuning guide | PARTIAL | Guide (`docs/cluster-d/14-os-tuning.md`) + verify script (`scripts/verify-os-tuning.sh`) done. VM cannot apply settings (`sysctl -w` / `ulimit -n` return Operation not permitted; `netdev_max_backlog` absent) — script FAILs honestly with live values (`log (sha256 45b67c85b295, 10 lines; kept outside repo)`). Apply+verify owed on the reference image. |
| 14.9 Crash restart | DONE | `TestClusterDCrashRestartRejoinsRoom`: client rejoins the same room on attempt 3 after 2 down attempts; a never-back server fails after the budget (exactly 3 attempts). |

## Files changed / added (vs `joblist-tree/`)

Changed: `main.go` (+7 lines, 14.4 only).
Added Go: `multinode.go`, `multinode_test.go`, `audio_ext.go`,
`audio_ext_test.go`, `observability.go`, `observability_test.go`,
`recording.go`, `recording_test.go`, `whip.go`, `cdn_sim.go`,
`latency_measure.go`, `client_errors.go`, `session_resume.go`,
`session_resume_test.go`, `clusterd_misc_test.go`,
`clusterd_test_helpers.go`, `outputs/packager.go`,
`outputs/packager_test.go`, `cmd/hami-packager/main.go` (2,371 lines
across the new Go files, incl. tests).
Added JS: `sdk/web/hami.js`, `test/web-sdk-test.mjs`,
`test/playout-test.mjs`, `sample-apps/web/index.html`,
`sample-apps/web/README.md`.
Added docs/ops: `docs/cluster-d/` (10 docs), `observability/alerts.yaml`,
`observability/dashboard.json`, `scripts/verify-os-tuning.sh`.
Evidence: `evidence/cluster-d/` (15 logs + this report).

## Open / owed (for the coordinator)
- 10.1 headless-browser test (needs a browser machine); 10.2/10.3/10.5
  (physical devices); 11.3 cross-machine cascade (second machine);
  12.5 Mac audio-rung run (reference machine); 13.1 Safari + hls.js
  playback (browser machine); 13.3 real CDN (owner approval);
  14.8 apply + verify on the reference Linux image (privilege).
- Nothing in this cluster was faked: every BLOCKED/PARTIAL gap above
  names the missing resource and the evidence that stops there.
