# One reachable swap rule (18.2)

Job 18.2: the record uses three swap thresholds (2,048 MiB, 2,500 MiB
and 8 GiB). The owner picks one the reference machine can meet, and
the preflight applies it.

Status: **PARTIAL — the three thresholds and the current state are
recorded; the owner's pick is owed (open decision below).** This file
does not pick for the owner.

## The three thresholds (where each comes from)

| Threshold | Meaning | Status |
|---|---|---|
| **2,048 MiB free swap** (`sfuHostMinFreeSwapMiB`, old rule) | The original floor: free swap >= 2,048 MiB | **Unreachable, superseded.** The OS resizes the pool to the live pressure, so free swap is capped by the pool size — after the 24 Nov 2026 swap clean the pool shrank 3,072 -> 2,048 MiB and free swap is 1,682 MiB, so free >= 2,048 would need used ~= 0. Code comment at `main.go:68-81` states this. |
| **2,500 MiB** (the 2,500 MiB rule) | The rule the list originally named as the bar | **Superseded.** The reference pool is 1,024 MiB total; a 2,500 MiB free-swap (or total) threshold can never be met by a 1,024 MiB pool. Judge by used swap / memory pressure instead. |
| **8 GiB** (`sfuHostMinFreeMemNoSwapMiB = 8192`) | Free-memory headroom required when the machine holds **no** swap pool (total swap 0, as after a reboot) | **Current, but unproven in the edge case.** The no-pool branch of `sfuHostMemoryFailures` is the intended rule; the live path cannot reach it with a zero total because `readSwapMiBChecked()` fails closed on a twice-zero total first (see 18.3). A freshly rebooted machine with no pool therefore sits at `host_preflight=UNKNOWN` (retrying) until the pool appears or the glitch is identified. |

## The rule in force today (code, not a pick)

- Swap pool present: refuse while **used swap > 512 MiB**
  (`sfuHostMaxUsedSwapMiB`), plus memory-pressure level must be
  NORMAL, plus free disk >= 10 GiB. This rule passes the real
  post-clean state (pool 2,048, used 365.88 MiB) and fails the
  pre-clean state (pool 3,072, used 1,917.625 MiB) — pinned by
  `TestSFUHostPreflightParsesCountersAndEnforcesStops`.
- No pool (total 0): free memory >= 8,192 MiB (intended rule, see
  above).

## OPEN DECISION — owner's pick (owed)

The owner picks ONE reachable threshold for the reference machine
from: (a) used swap <= 512 MiB (the rule in force), (b) a different
used-swap cap, or (c) the 8 GiB no-pool free-memory rule as the
primary rule. Until the pick is recorded here with a date, (a)
stands as the enforced rule and this job stays PARTIAL.
