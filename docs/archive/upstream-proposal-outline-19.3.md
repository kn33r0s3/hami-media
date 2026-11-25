# Upstream proposal — outline, awaiting the gates (19.3)

Job 19.3: a proposal is written with its benchmark, and only if
16.7 or 16.8 supports it.

Status: **AWAITING THE GATES — draft outline only, 25 Nov 2026.**
No proposal has been written to LiveKit, and none will be until a
deadline policy is shown to win under 16.7/16.8.

## Draft outline (what the proposal would contain)

1. **The policy**: hami-media's per-track millisecond deadline
   (job 4.3 — one ms value driving queue age, NACK window and cache
   horizon; job 4.2 — abandon a frame at its deadline and advance
   the cursor), stated as a concrete algorithm with its parameters.
2. **The benchmark**: the matched 16.1 ladder numbers showing the
   policy's effect (frames recovered, p99, dissolved fraction) on
   hami-media, and the same workload against LiveKit's forwarder
   (whose drop policy is layer/state-driven, not deadline-driven —
   see `docs/livekit-comparison-19.1.md`).
3. **The gap it closes in LiveKit**: what LiveKit's forwarder does
   today when a frame is late (deficient-state drops + PLI/NACK),
   and where a deadline would change the outcome.
4. **The change, scoped**: the smallest LiveKit change that carries
   the policy (forwarder + downtrack retransmit worker), with tests
   in LiveKit's suite, under Apache 2.0 with attribution.
5. **Non-goals**: no hami-media code is proposed for copying; the
   proposal is a policy + measurements, written for LiveKit's
   maintainers to accept, modify or refuse.

## Preconditions (all must hold before the proposal is written)

- [ ] 16.7 WIN (the deadline policy is part of what won), or 16.8
      confirms the win on the second hardware class.
- [ ] Job 4.3 implemented in hami-media with its unit tests at 6
      and 187 packets/s, and job 4.4's forced-drop run showing every
      drop accounted for.
- [ ] The benchmark in (2) reproduced from raw files by the 15.7
      generator, not from a summary.

If any precondition fails, this file stays an outline and the job
stays open. Writing the proposal early would be a claim without the
measurement that justifies it.
