# Guidebook execution progress

Status date: 2026-11-27

## Section gates

- Section 0: executable governance checks pass, but this work does not
  rewrite history or grant any `[!]` approval.
- Section 1: PARTIAL. The source manifest, reproducible builds, test
  summary, and Mac test runner pass. Docker Linux is a development
  execution surface, not the guidebook reference VM.
- Section 2: existing focused fixes pass 1,000 Linux race repetitions.
  No job is marked closed until VM fail-before/pass-after evidence exists.
- Section 3: fast timing tests pass 1,000 Linux race repetitions.
  The deliberate 5 fps timing run remains a reference-run/VM item.
- Section 4: short fault-injection, retransmit, cache, and slow-viewer
  tests pass 1,000 Linux race repetitions. The isolation controls pass once
  under race and are not repeated 1,000 times because they are long-window
  controls.
- Memory measurement: fixed unsigned underflow in
  `TestPerViewerMemoryBudgetMeasured`; it passes 1,000 repetitions.

## Executable evidence

- Local test-all: [20261127T124955Z-X1TjjB](../results/test-all/20261127T124955Z-X1TjjB/summary.json)
- Linux Docker test-all: [20261127T125709Z-be6Ls8](../results/test-all/20261127T125709Z-be6Ls8/summary.json)
- Final local test-all: [20261127T134525Z-rgbswD](../results/test-all/20261127T134525Z-rgbswD/summary.json)
- Linux baseline table: [table.md](../results/baseline-linux/table.md)
- Reproducible build hashes:
  - hami-media: `8da8fa9e71e219f9c860fd5f78b449ef705064f48e24b2bf94e52f538a9fe36d`
  - cmd/hami-packager: `4831b85e4306323d17dcc8f3179ac0a18b90efa5ade4405faef6cfddfb396712`

The Linux Docker baseline has 347 passing test names and
`TestClusterDWHIPFFmpegEncoderAttempt` failing because the installed
Debian FFmpeg lacks a WHIP muxer and reports `Unknown format 'whip'`.
It is not a code verdict or reference-VM result.

## Blocked

- Reference Linux VM image/build, image digest, VM run IDs, and VM evidence.
- Job 1.1 final canonical/mirror owner decision.
- Job 1.7 history rewrite, because `[!]` approval is not recorded.
- Jobs 6.10, 7.9, 8.11, 13.3, 18.3, and 19.4 remain approval-gated.
- Head-to-head, real-network, LAN, CDN, mobile, and second-hardware runs
  cannot be claimed without the required hosts and approvals.
