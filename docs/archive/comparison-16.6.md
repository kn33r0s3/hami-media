# Comparison generated from raw files (16.6)

Job 16.6: the table is produced by the report script, and failures
appear as failures.

Status: **DONE (script) / OPEN (table) — the generator is written and
tested; the comparison table itself awaits the ladder (16.4).**

## The generator

`scripts/compare-livekit.py <hami-result.json> <livekit-result.json>`
reads both sides in the R4 schema (validated by
`scripts/validate-result.py`) and prints a table against the
preregistered win bar (`docs/benchmark-preregistration.json`).

Rules the script enforces:

- A missing side prints as **MISSING** and the verdict is
  **UNDECIDED** — never a win, never a silent drop.
- A side whose trials failed prints its `failed_trials` list, and
  the verdict is **FAILURES PRESENT** — failures are failures; the
  win bar is not met on a failed leg.
- Either path may be the literal word `MISSING` (side not run yet).

## Test run (25 Nov 2026)

Run against the benchmark result produced by the 15.5 runner
(`results/benchmarks/bench-20261125-081224-9b253faf.json`) with the
LiveKit side missing:

```
$ python3 scripts/compare-livekit.py results/benchmarks/bench-20261125-081224-9b253faf.json MISSING
| Metric | hami-media | LiveKit | Win bar (16.2) |
| trial_ids | ['bench-20261125-081224-9b253faf'] | MISSING | 2x (either leg) |
| viewers at p99<=150ms | MISSING | MISSING | 2x (either leg) |
| CPU per subscriber per Mbps | MISSING | MISSING | 2x (either leg) |
| RSS per subscriber per Mbps | MISSING | MISSING | 2x (either leg) |
| p99 median (ms) | MISSING | MISSING | 2x (either leg) |
| failed trials | MISSING | MISSING | 2x (either leg) |
NOTE: livekit: MISSING (not run yet)
VERDICT: UNDECIDED — a side is missing; no win/loss claim is made.
```

Exit code 1 (a side is missing). The hami-media benchmark JSON
itself carries no ladder metrics (it is a micro-benchmark result:
copy 270.8 ns/op, enqueue 14,994 ns/op) — the ladder metrics
(`max_viewers_at_p99_150ms`, per-subscriber CPU/RSS per Mbps) only
exist once 16.4 runs. The script correctly reports them MISSING
rather than inventing them from the micro-benchmarks.

## Owed

- The ladder result JSONs for both sides (16.4), after which this
  script produces the real table and 16.7 is decided from it.
