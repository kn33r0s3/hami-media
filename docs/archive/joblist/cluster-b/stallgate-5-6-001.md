# Job 5.6 — Validate the stall gate (p99 median > 50 ms = FAIL) — DONE (with one honest Mac leg)

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdict: DONE for CPU + network + clean runs; GC leg is OPEN (Mac) — see below

## The gate (`stallgate.go`)

- `stallGateP99MedianMax = 50 ms`; `evaluateStallGate(p99Median)`
  passes iff median ≤ 50 ms; `stallGateP99Median` (median of
  per-viewer p99s; even counts average the two middles — the same
  median the Mac JSONs were recomputed with); `sfuProofP99Median`
  extracts it from a proof result.
- Wired into BOTH verdict functions (`sfuMeasurementPass` and
  `sfuDissolveMeasurementPass`): a run whose median viewer p99
  exceeds 50 ms fails even when every other check passes.
  `TestStallGateWiredIntoMeasurementPass` proves the wiring with
  fabricated results (median 20 ms passes, 60 ms fails, both modes).

## Recomputed Mac medians (from the saved raw JSONs — exact match)

Log: `log (sha256 5bd728983c17, 5 lines; kept outside repo)` (python recomputation from
`viewers[].latency.p99_ns`).

| run | recomputed p99 median | recorded | gate | measurement_pass |
|---|---|---|---|---|
| control A (GOMAXPROCS=1) | 104.91 ms | 104.91 | FAIL | False |
| baseline fixed27 20×8,000 | 23.49 ms | 23.49 | PASS | True |
| recheck 20×7,500 | 19.03 ms | 19.03 | PASS | True |
| varA run 1 20×8,000 | 24.46 ms | 24.46 | PASS | False (frames, not latency) |
| varA retry 20×8,000 | 23.76 ms | 23.76 | PASS | True |

## In-process controls (`stallgate_test.go`)

Harness: one publisher floods packets into 3 subscriptions
(production 64-packet queue); one drain goroutine per subscription
dequeues, records enqueue→dequeue latency, and forwards through the
real `forwardQueuedRTP`. Per-viewer p99 via `Percentile(..., 99)`;
median across viewers; gate evaluated. Each control's mechanism is
recorded exactly; the VM cannot do real UDP network stalls, so the
network control is an in-process packet delay.

| control | what it does | measured median | gate |
|---|---|---|---|
| CPU starvation | GOMAXPROCS=1 + 8 spinning goroutines (the Mac control-A analog: the forward goroutines are starved of scheduling slots) | **167.10 ms** (p99s 169/167/121 ms) | **FAIL** ✓ |
| Network stall | fixed 200 ms sleep in the drain before each forward (a stalled network backing the queue up: 64-deep queue × 200 ms) | **12.83 s** | **FAIL** ✓ |
| GC pressure | GOMEMLIMIT 64 MiB over a retained 40 MiB live set, GOGC=1, 2× 1 MiB churn goroutines | **9.85 ms** (this run; 8.01 / 0.40 / 0.38 ms in earlier drafts) | PASS — see below |
| Clean ×3 | no stall, no delay | 45.6 / 50.3 / 83.5 µs | **PASS** ✓✓✓ |

Logs: `log (sha256 000dd0be68e9, 6 lines; kept outside repo)`, `log (sha256 52c2cc4ddd71, 6 lines; kept outside repo)`,
`log (sha256 edf2d7d681ce, 8 lines; kept outside repo)` (final draft), `log (sha256 9803c9dd0be8, 11 lines; kept outside repo)`
(GC + 3 clean runs).

## The GC leg — measured, not faked (OPEN for a Mac run)

Three GC-control drafts all failed to cross the gate in the VM:
GOGC=1 + churn alone (8.01 ms), + a 256 MiB retained flat-byte heap
(0.40 ms), + GOMEMLIMIT 64 MiB over a retained 40 MiB heap
(0.38–9.85 ms across runs). The pressure is real — the final run
recorded **NumGC = 35 collections in 0.17 s** — but Go's concurrent
collector bounds stop-the-world pauses to low-ms even under extreme
churn, and the drain (microseconds per packet) recovers between
pauses, so the queue never builds a 50 ms backlog. A >50 ms GC pause
needs a multi-GB pointer-heavy heap — the scale runs' actual
condition, not reproducible in-process here. Faking it (e.g. a sleep
in the drain labelled "GC") would be a network control in disguise,
so it was not done.

`TestStallGateGCStallMeasured` records this honestly: it asserts the
pressure is real (NumGC rises) and the median stays below the gate,
and it is the trip wire — if a future Go version, heap shape, or
drain change ever lets GC pressure cross 50 ms in the VM, it fails
and the GC leg is reproduced in-process.

**OPEN — Mac GC leg:** run a scale proof with a multi-GB heap and
heavy allocation (the actual scale-run condition) and read the
median viewer p99 from the result JSON; the gate fails the run iff
it exceeds 50 ms. No VM command can substitute — the VM's collector
pauses are too short by construction.

## CORRECTION — the gate is strict-mode only (25 Nov 2026, found by the final full suite)

The first draft of this job wired the gate into BOTH verdict
functions (`sfuMeasurementPass` and `sfuDissolveMeasurementPass`).
That was wrong and it broke a pre-existing test: the final full
suite (`log (sha256 20e3ffd8057c, 78 lines; kept outside repo)`) showed **7 failures** — the
known six UDP failures plus
`TestSFUDissolveScoringUsesSyntheticDeliveredLatencyAndConfiguredLimits`,
which records the dissolve mode's design: dissolve scoring accepts
a 120 ms p99 at a configured 150 ms limit, a latency the strict
recorder rejects (P95 120 > 100). The gate was proven only for the
strict pass (control A + the in-process controls, all
strict-mode); no evidence was presented that dissolve scoring
should be gated at a 50 ms median, and wiring it in kills the
dissolve design for the median viewer.

Fix: the gate block was REMOVED from
`sfuDissolveMeasurementPass` (with a comment recording why), and
`TestStallGateWiredIntoMeasurementPass` was corrected to assert
the gate applies to the strict verdict and does NOT apply to
dissolve scoring (a 60 ms median passes dissolve scoring at
maxP99=150 ms). Fail-before: 7 failures in
`log (sha256 20e3ffd8057c, 78 lines; kept outside repo)`. Pass-after: the dissolve test and all
stall-gate tests pass; the re-run full suite must show exactly the
known six (see `log (sha256 d324d9cdfb3b, 76 lines; kept outside repo)`).

**OPEN — design question for the coordinator:** should dissolve
mode also carry the 50 ms median gate? That is a design decision,
not a unilateral change — it would need the dissolve test's
premise (strict-rejects but dissolve-accepts) to be re-decided
first.
