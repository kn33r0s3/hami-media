# Counts-table rule (job 0.6)

Rule (R5): no headline count without its trial list. Every reported
count — viewers served, frames delivered, pass/fail tallies, latency
medians — must be accompanied by a table of its trial IDs, the
outcome of each trial, and the failure cause for each failed trial
(or "cause unknown" where the cause is unestablished). A count
reported without its table is not a result; it is a claim.

## What a counts table contains

| Trial ID | Conditions (code sha, parameters) | Outcome | Failure cause |
|---|---|---|---|
| ... | ... | pass / fail / blocked | measured cause, or "cause unknown" |

- Trial IDs are stable across notes: the same run keeps the same ID
  in the launch script, the raw JSON, and the table.
- Conditions name the code sha and the parameters that matter
  (publishers, viewers per publisher, duration, gates).
- A re-run is a new trial ID, never an overwrite of the old row.
- Raw files (JSON, stderr, logs) are kept beside the table; the
  table is a summary of them, not a replacement.

## Exemplar

REPORT.md §4.5 reconciles all 12 trials of 20×8,000 (T0/T1/T4/
FINAL/verify-s005/stall-capture pass; T2/T3 frame-fail; 4
catastrophic) with outcomes and causes. New counts follow that
shape. The VM baseline in `docs/archive/joblist/cluster-a/` follows it too:
`baseline-vm-2026-11-25.md` lists every test with its outcome, and
the 6 failures name their cause (sandbox UDP block).

## Checks

- A result note that states a count without a trial table fails
  review until the table is added.
- The report generator (job 15.7, not yet built) must produce the
  table from the raw files; until then the table is maintained by
  hand and checked against the raw files before any sync.
