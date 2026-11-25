# Cluster A evidence — job-list sections 0, 1, 2 (2026-11-25)

Working copy `joblist-tree-a`. VM-first: every change below ran in
this VM before anything was recorded as done. Raw `.log` files in
this directory are test evidence; they may contain sandbox paths
from stack traces by design and are excluded from the leak scan
(see `scripts/leak-scan.py`).

## Section 2 — fail-before / pass-after

- `log (sha256 37d73d2ca637, 15 lines; kept outside repo)` — first test run, compile
  failure (missing helpers).
- `log (sha256 7667c9b09ebd, 31 lines; kept outside repo)` — pre-fix
  `unregisterTrack` panicked on nil remote.
- `log (sha256 46de68e8d2a8, 62 lines; kept outside repo)` — clean pre-fix run: FAIL 2.2 (goroutine
  leak), 2.3 (router close), 2.4 (cleanup), 2.5 (retry), 2.7
  (literals), 2.9 (multi-slice); PASS 2.1 default, 2.6 helpers, 2.8,
  2.10 guards.
- `log (sha256 eb85e7f06364, 8 lines; kept outside repo)` — tagged build
  (`-tags hami_disable_srtp_replay`) fails as designed: replay
  packet delivered, replay protection off.
- `log (sha256 f4f87c68b338, 52 lines; kept outside repo)` — post-fix: all 17 new tests PASS
  (0.773 s on the refresh; 0.865 s on the first run). Refreshed
  after the `TestEgressShardStopsWhenStopped` hardening (poll for
  the goroutine instead of a fixed 100 ms sleep — it had failed
  once under full-suite load).
- `log (sha256 0e26522b1759, 31 lines; kept outside repo)` — full `scripts/test-all.sh` run
  (leak, gofmt, vet, unit, race, fuzz smoke + summary).

## Section 1 — baseline, race, builds, diff

- `baseline-vm-2026-11-25.md` — full suite baseline, per-test
  table: 150 PASS / 6 FAIL (the known sandbox UDP blocks).
- `log (sha256 7b19b53445e9, 82 lines; kept outside repo)` — post-fix full suite: same six
  failures, no new ones.
- `log (sha256 ae6328a5f260, 83 lines; kept outside repo)` — second post-fix full suite:
  known 6 + the 2 profile flakes (see the race section in
  `baseline-vm-2026-11-25.md`).
- `log (sha256 04292a7ccc3b, 85 lines; kept outside repo)` — `go test -race` output: zero `DATA RACE`
  lines (no data races); 9 failures = the known 6 UDP blocks + 3
  timing-sensitive flakes (see the race section in
  `baseline-vm-2026-11-25.md`). First attempt failed to BUILD only
  because /tmp (512 MiB tmpfs) ran out of space; re-run with
  GOTMPDIR on the home volume.
- `build-reproducibility-2026-11-25.md` — two `-trimpath`
  builds, binary shas compared (job 1.3).
- `source-manifest-2026-11-25.sha256` — sha256 of every source
  file in the final tree (job 1.2).
- `canonical-vs-mirror.diff` — ForgeOS copy vs standalone copy,
  5 hunks, 26 changed lines (job 1.1).

## Section 0 / Section 2 decisions

- `log (sha256 6b7ebb042dea, 1 lines; kept outside repo)` — leak scan: 41 files, 0 leaks
  (jobs 0.3, 1.7 current-tree scan).
- `literal-track-ids-2026-11-25.txt` — repo-wide literal track-ID
  search (job 2.7).
- `job-2.8-decision.md` — Linux swap-check decision (job 2.8,
  PARTIAL: Done-line reword owed).
