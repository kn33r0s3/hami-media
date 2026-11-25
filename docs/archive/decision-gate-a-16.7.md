# Decision gate A (16.7)

Job 16.7: if hami-media misses the win bar at matched load, product
work stops and LiveKit OSS is the platform. The decision is written
with its numbers.

Status: **OPEN — the gate and its procedure are recorded here; the
decision itself awaits 16.4/16.6.** No decision is made in this file.

## The gate (from the job list, preregistered 25 Nov 2026)

- Win bar (16.2): at least 2x less CPU and RSS per subscriber per
  Mbps, OR at least 2x more viewers at p99 <= 150 ms, measured the
  same way for both servers.
- If hami-media misses the bar at matched load: **product work stops
  and LiveKit OSS is the platform.**
- If it clears the bar: decision gate B (16.8) applies — repeat on a
  second hardware class and a real network before any claim is made.

## Decision procedure

1. Run the ladder (16.4): 100–5,000 subscribers, 3 runs per step,
   both servers native, load generator on a separate machine.
2. Generate the table with `scripts/compare-livekit.py` from the raw
   result JSONs (16.6). Failures appear as failures.
3. Apply the win bar to the table. The outcome is one of:
   - **WIN** (either leg of the bar holds on all three runs per
     step, no failed trials hidden) -> proceed to gate B (16.8);
   - **MISS** -> product work stops; LiveKit OSS is the platform;
     the decision is written here with the table attached;
   - **UNDECIDED** (a side missing or a run failed for a host
     reason, e.g. ABORTED-BY-HOST) -> no decision; the run is
     repeated, not reinterpreted.
4. The decision, once made, is written in this file with the
   comparison table, the trial IDs and the raw-file paths. It is not
   made in chat, and it is not made from a summary.

## Current state

- Preregistration: DONE (`docs/benchmark-preregistration.json`).
- LiveKit pinned + smoke-tested: DONE (16.3, v1.13.11).
- Ladder: BLOCKED (16.4 — no second machine).
- Decision: **not made**.
