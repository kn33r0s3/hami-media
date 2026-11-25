# Build, contribute or adopt decision — FRAMEWORK (19.2)

Job 19.2: the decision is recorded.

Status: **FRAMEWORK DONE 25 Nov 2026; the decision itself awaits
16.7/16.8.** Nothing in this file decides build vs contribute vs
adopt. It states the criteria, the evidence each option needs, and
who decides.

## The three options

1. **Build** — keep hami-media as the product forwarder; LiveKit is
   a benchmark reference only.
2. **Contribute** — keep hami-media, but upstream the mechanisms
   that win (e.g. a deadline policy, if 19.3 supports it) to
   LiveKit, and adopt LiveKit's mechanisms where they are better.
3. **Adopt** — stop product work on hami-media's forwarder and make
   LiveKit OSS the platform (the 16.7 stop rule).

## Criteria (all decided from measured evidence, not preference)

| Criterion | Evidence needed | Source |
|---|---|---|
| Head-to-head win bar | 16.6 comparison table from the 16.4 ladder | `scripts/compare-livekit.py` output |
| Second hardware class + real network | 16.8 repeat, same gates | 16.8 raw files |
| Feature gaps that block the product | 19.1 comparison (room model, codec set, SDKs) vs the job list §§6, 9, 10 | `docs/livekit-comparison-19.1.md` |
| Cost of closing the gaps in hami-media | Per-gap estimates with the tests that would prove closure | Job list §§6, 9, 10 verdicts |
| Upstream fit | 19.3 proposal assessment (only if a deadline policy wins) | `docs/archive/upstream-proposal-outline-19.3.md` |

## Decision rules

- **16.7 MISS -> Adopt.** No further debate: the stop rule fires.
- **16.7 WIN + 16.8 confirmed -> Build**, unless a feature gap
  (room model, codec set, SDKs) is judged product-blocking and
  cheaper to get from LiveKit than to build — in which case the
  decision is **Adopt with a hami-media contribution path**, and the
  reasoning is written here with the gap list attached.
- **16.7 WIN + 16.8 fails ->** no claim is made; the decision is
  deferred and the 16.8 failure is investigated, not reinterpreted.
- **A deadline policy wins (19.3) -> Contribute** the policy
  upstream in parallel with the Build decision; contribution does
  not change the Build/Adopt outcome by itself.

## Who decides

The owner decides, with this framework's table in front of him.
The decision is recorded in this file with its numbers and date. A
decision recorded anywhere else (chat, summary) is not the decision.

## Blockers on the decision

- 19.4 (first target product + latency budget, [!], owner-written):
  the product the forwarder serves is not yet written down, so the
  "product-blocking gap" criterion cannot be applied until 19.4
  exists. This framework is complete; the decision waits for both
  16.7/16.8 and 19.4.
