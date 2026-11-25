# Cluster E evidence report — sections 15, 16, 17, 18, 19

Worked 25 Nov 2026, VM-first, in `joblist-tree-e/` only.
`main.go` sha256
`9b253fafc2a73a251101fd4435c98ec2c6e17b0f07d94594b6d918b2294fd19a`
(unchanged by this cluster except `go.mod`/`go.sum` — see 17.1).
Baseline before this cluster's changes: full suite 150 pass, 6 fail
(the known VM sandbox UDP failures); after: same 6 fail, plus 5 new
tests pass (see below). `gofmt` clean, `go vet` clean, leak scan
passes.

## Verdicts

### Section 15 — Testing, CI, benchmarks

| Job | Verdict | Evidence |
|---|---|---|
| 15.1 fail-before/pass-after register | PARTIAL | `docs/fail-before-pass-after-register.md`. Records present for 4.1/4.2 (partial); 2.1/2.9 bugs recorded with no failing test yet; the other 19 records are owed to clusters A/B. |
| 15.2 deterministic seeds | DONE | `determinism_test.go` (3 tests) + `docs/archive/cluster-e-evidence-15.2.md`. Pass at `-count=2`. |
| 15.3 loopback integration in CI | PARTIAL | `docs/ci.md` + `docs/ci/loopback-job.yml` (job content). Wiring into `.github/workflows/ci.yml` is cluster A's; no CI run yet. |
| 15.4 nightly scale tests | OPEN (staged) | `scripts/nightly-scale.sh` + `docs/nightly-scale.md` (job + storage layout). No nightly has run; cron entry owed (reference machine). |
| 15.5 one benchmark runner | DONE | `scripts/benchmark-runner.sh`. Trial `bench-20261125-081224-9b253faf`: copy 270.8 ns/op, 1 alloc; enqueue 14,994 ns/op, 0 allocs. JSON + logs in `results/benchmarks/`; validates under 15.6. |
| 15.6 result schema validator | DONE | `scripts/validate-result.py`. Rejects a result missing sha/image/parameters/trial IDs/raw logs (tested); accepts a valid one (tested, incl. the 15.5 output). |
| 15.7 report from raw files | DONE (generator) | `scripts/generate-counts-table.py`. Regenerates REPORT.md §4.5 from raw JSONs: all 7 trials with raw files match exactly (FINAL 15.38/25.50 ms, verify-s005 13.94/24.46, gomem4g 896.72/1001.46, gomem8g 882.38/985.02, verify-t0-repeat 873.00/982.93, verify-t0-repeat2 871.77/980.98, stall-capture 16.39/28.30). T0/T1/T2/T3/T4 are RAW-MISSING (no raw JSON in the work dir) — reported, not invented. Output: `results/counts-table-20261125.txt`. |
| 15.8 24-hour soak | OPEN (staged) | `scripts/soak.sh` + `docs/soak-test-15.8.md` (limits: RSS <= +10% post-warm-up, threads +/-5%, zero ABORTED-BY-HOST). Pilot 60 s: harness proven (preflight READY, 2 samples, RSS 9,948 -> 17,048 KiB); process exit 1 on the VM UDP block (ICE sent=0/received=0). Not a soak pass. |

### Section 16 — Head-to-head and decision

| Job | Verdict | Evidence |
|---|---|---|
| 16.1 preregister matched benchmark | DONE | `docs/benchmark-preregistration.json` + `docs/preregistration-16.1-16.2.md`. Frozen 25 Nov 2026, before any comparison run. |
| 16.2 preregister win bar | DONE | Same files. 2x less CPU+RSS per sub per Mbps, or 2x more viewers at p99 <= 150 ms. Never changed after results. |
| 16.3 install LiveKit OSS, pinned | DONE | `docs/archive/livekit-install-16.3.md`. v1.13.11, sha `1e9ee90ca301…`, smoke test PASS (HTTP 200 `OK` on :7880). Binary in VM `/tmp`, not committed. |
| 16.4 run the ladder | BLOCKED | No second machine; VM sandbox blocks unconnected UDP and cannot be the load generator. |
| 16.5 reference hardware by script | DONE | `scripts/record-hardware.sh`. VM record: AMD EPYC 9D25, 4 cores, 7,934 MiB RAM, swap 1,023 MiB, Ubuntu 24.04.5 LTS, kernel 7.0.0-44-generic, Go 1.27.4. |
| 16.6 comparison from raw files | DONE (script) | `scripts/compare-livekit.py` + `docs/archive/comparison-16.6.md`. Tested with the LiveKit side MISSING -> UNDECIDED, exit 1. The table itself awaits 16.4. |
| 16.7 decision gate A | OPEN | `docs/archive/decision-gate-a-16.7.md` (gate + procedure). Decision not made; awaits 16.4/16.6. |
| 16.8 decision gate B | BLOCKED | Second hardware class + real network; no second machine. |

### Section 17 — Packaging, docs, licensing, release

| Job | Verdict | Evidence |
|---|---|---|
| 17.1 Docker image | PARTIAL | `Dockerfile` + `docs/archive/docker-image-17.1.md`. Trivy 0.76.2: config 0 findings (after HEALTHCHECK); fs 17 HIGH -> 0 HIGH, 0 CRITICAL after the x/crypto+x/net bump. No image built (no daemon in VM); no digest pin (registry lookup needed approval that was not answered in time; not retried). |
| 17.2 Helm or systemd | PARTIAL | `deploy/hami-media.{service,socket}` + `deploy/launch/main.go` + `docs/systemd-install-17.2.md`. `systemd-analyze verify` clean; server started via the Go launcher with fd 3/4 handoff: /health 401 without token, 200 `{"status":"ready"}` with it. Full `systemctl enable --now` run owed on a systemd host. Found and fixed in testing: shell launchers (dash AND bash) close fd 3, which would have broken socket activation. |
| 17.3 architecture/operator/API/SDK docs | PARTIAL | `docs/architecture.md`, `docs/operator.md`, `docs/api.md`, `docs/sdk.md` (SDKs: none exist; web SDK is cluster D's). Independent review owed for each. |
| 17.4 known limitations | DONE | `docs/known-limitations.md` (6 failing tests + all disabled features, for this release; update each release). |
| 17.5 license audit + NOTICE | DONE | `NOTICE` + `docs/archive/license-audit-17.5.md`. All 32 modules read from their LICENSE files: Pion MIT, x/* BSD 3-Clause, no copyleft. |
| 17.6 name + trademark check | DONE (preliminary) | `docs/archive/name-trademark-check-17.6.md`. No blocker found for "hami-media"; formal Nepal registry search owed before commercial use. |
| 17.7 versioning + changelog | DONE | `CHANGELOG.md` + `docs/archive/versioning-17.7.md`. Scheme set; first baseline v0.1.0 (unreleased as a tag). |
| 17.8 release checklist | PARTIAL | `docs/archive/release-checklist-17.8.md` + `scripts/release-check.sh`. Gates 1-8 enforced and passing; gate 9 (source manifest, job 1.2, cluster A) is recorded OPEN, not silently passed. |

### Section 18 — Host safety

| Job | Verdict | Evidence |
|---|---|---|
| 18.1 host preflight before every run | DONE | `scripts/host-preflight.sh` + `docs/host-preflight-18.1.md`. Verified READY in the VM (swap used 417 MiB at the time). |
| 18.2 one reachable swap rule | PARTIAL | `docs/swap-rule-18.2.md`. Three thresholds recorded (2,048 superseded, 2,500 superseded, 8 GiB no-pool intended-but-unproven); rule in force is used swap <= 512 MiB. **Owner's pick owed.** |
| 18.3 swap-zero reading | NEEDS-APPROVAL | `docs/known-results-swap-zero-entry.md` — register entry text for cluster A (job 0.5). Mechanism identification NOT started (no approval). |
| 18.4 ABORTED-BY-HOST logging | DONE | `TestHostWatchdogAbortIsAbortedByHostNotCapacityFailure` (`host_safety_test.go`): forced used-swap violation -> watchdog abort naming used swap -> `ABORTED-BY-HOST` error + JSON status. Passes. |
| 18.5 heap cap per run | DONE | `TestHeapCapPerRunFromMeasuredBudget` + `docs/heap-cap-18.5.md`. Budgets pinned (20x400 = 8 GB, 20x375 = 7.5 GB, floor 256 MiB); trigger is `>=` (found by the test failing on the first draft's `<` assumption). |
| 18.6 no power claims without a meter | DONE | Re-verified by this cluster: `grep -i "watt\|power.*consum\|milliwatt\|kWh" REPORT.md` -> zero matches. Left alone. |

### Section 19 — Upstream and strategy

| Job | Verdict | Evidence |
|---|---|---|
| 19.1 read LiveKit's forwarder | DONE | `docs/livekit-comparison-19.1.md` (v1.13.11 source, read-only, no code copied; Apache 2.0 attribution noted). Findings: no per-frame deadline in LiveKit (thresholds instead: resume 200 ms/2 s, switch 50/25 ms, acquire grace 1 s); drops are layer/deficient-state driven; NACK retransmit capped at 5,000 pending with RTX budget; codec set VP8/VP9/H.264/AV1/H265 + Opus; room model is first-class with MaxParticipants + EmptyTimeout. |
| 19.2 build/contribute/adopt decision | DONE (framework) | `docs/archive/build-contribute-adopt-framework-19.2.md`. Decision itself awaits 16.7/16.8 + 19.4. |
| 19.3 upstream proposal | OPEN | `docs/archive/upstream-proposal-outline-19.3.md` — outline only; proposal written only if 16.7/16.8 supports it. |
| 19.4 first target product + latency budget | NEEDS-APPROVAL | [!] Owner-written. Not started. |

## Files changed / added (diffstat)

New Go tests: `determinism_test.go`, `host_safety_test.go`.
New scripts (10): `scripts/record-hardware.sh`, `scripts/host-preflight.sh`,
`scripts/benchmark-runner.sh`, `scripts/validate-result.py`,
`scripts/generate-counts-table.py`, `scripts/compare-livekit.py`,
`scripts/leak-scan.sh`, `scripts/release-check.sh`, `scripts/soak.sh`,
`scripts/nightly-scale.sh`.
New docs (30): see the table above.
New packaging: `Dockerfile`, `deploy/hami-media.service`,
`deploy/hami-media.socket`, `deploy/launch/main.go`, `NOTICE`,
`CHANGELOG.md`.
Changed: `go.mod`, `go.sum` (x/crypto v0.59.0, x/net v0.63.0,
x/sys v0.50.0, x/term v0.48.0, x/text v0.44.0, go directive
1.24.0 -> 1.26.0 — required by the fixed deps; build/vet/tests
pass on Go 1.27.4).
Results (evidence, not source): `results/benchmarks/`,
`results/counts-table-20261125.txt`, `results/soak-pilot/`,
`results/scans/`.
No change to `main.go` or any other production source file.

## Notes for the coordinator

- **18.3 register entry**: the text is in
  `docs/known-results-swap-zero-entry.md` for cluster A to append to
  the 0.5 register. I did not add it to a register file myself —
  no register file exists in this copy.
- **17.8 manifest gate**: the release script enforces gates 1-8;
  gate 9 (source manifest, 1.2) is open until cluster A lands it.
- **15.3 CI wiring**: the loopback job content is in
  `docs/ci/loopback-job.yml` for cluster A to include in
  `.github/workflows/ci.yml`.
- **VM swap is close to the cap**: preflight reads showed swap used
  417-461 MiB against the 512 MiB cap during this session. A scale
  run on this VM would be near the refusal line.
- **Two of my own tests failed on first run and were corrected
  against the code** (heap trigger is `>=`, not `<`; the assembler
  needs priming to record a shuffled first frame). Both are
  documented in their test files — they are verifications, not
  defects.
