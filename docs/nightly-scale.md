# Nightly scale tests (15.4)

Historical evidence note: raw log availability is tracked in
[the archive evidence register](archive/log-evidence-status.md). The
date-and-SHA names below are templates, not evidence for completed runs.

Job 15.4: nightly results are stored with their raw files, run
against the section 16 gates.

Status: **STAGED — the job script exists; no nightly has run yet.**
A night-scale run cannot complete in-session; this file stages the
job so the first night it runs is already correct.

## The job

`scripts/nightly-scale.sh [results-root]` (default
`results/nightly`). Intended cron entry (on the reference machine,
not the VM — the VM cannot run scale proofs over loopback UDP):

```
0 2 * * * cd /path/to/hami-media && ./scripts/nightly-scale.sh >> results/nightly/cron.log 2>&1
```

Each night:

1. Runs the benchmark runner (`scripts/benchmark-runner.sh`) into
   `<root>/<YYYY-MM-DD>/benchmarks/`.
2. Runs the small ladder leg (2 publishers x 50 viewers, 60 s,
   strict gates) into `<root>/<YYYY-MM-DD>/ladder-2x50.json` with its
   stderr log beside it. The full ladder (16.4) needs a second
   machine and is NOT run here.
3. Records the hardware (`<root>/<YYYY-MM-DD>/hardware.json`,
   `scripts/record-hardware.sh`).
4. Writes `<root>/<YYYY-MM-DD>/MANIFEST.txt` listing every file.
5. Refuses to overwrite an existing night directory — a rerun gets a
   new date, never a silent overwrite.

## Storage layout

```
results/nightly/
  2026-11-26/
    benchmarks/bench-20261126-020000-<sha>.json
    benchmarks/bench-20261126-020000-<sha>.log
    benchmarks/bench-20261126-020000-<sha>.preflight.log
    ladder-2x50.json
    ladder-2x50.stderr.log
    hardware.json
    MANIFEST.txt
```

## Gates applied

The section 16 gates (p99 <= 150 ms, zero lost frames) are the
pass/fail rule for the ladder leg. A night that fails the gates is
kept with its raw files and reported as a failure, not deleted and
not re-run until the cause is understood.

## Owed

- The cron entry itself (owner's machine, not this VM).
- The first nightly directory, with its MANIFEST.
