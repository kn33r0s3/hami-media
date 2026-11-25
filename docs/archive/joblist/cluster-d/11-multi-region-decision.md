# 11.6 Multi-region decision

## Decision: single region. No multi-region claim is made.

There is no multi-region deployment, no multi-region test, and no
multi-region measurement in this tree. Any claim of multi-region
support before tests exist would be false, so none is made.

## What multi-region would require (recorded, not built)

1. **Region-aware placement**: the registry must know each node's
   region and prefer the region closest to the room's participants
   (11.2 placement today is capacity-only, region-blind).
2. **Cross-region cascade**: cascade relay across regions adds
   inter-region latency to the media path; the cascade gates (same
   p99 <= 150 ms as single-node runs, 11.3) must be re-run across
   regions, not assumed.
3. **Shared state across regions**: the 11.5 in-memory +
   reconciliation decision does not span regions; a cross-region
   store (or per-region registries with a federation protocol) is a
   separate decision with its own failure modes.
4. **Failure detection across regions**: heartbeat timeouts must
   distinguish "region partitioned" from "node dead", or a partition
   triggers mass room recovery to the wrong region.
5. **Data residency / legal**: regions imply data crossing borders;
   that is a legal question (owner approval), not an engineering one.

## Gate

Multi-region work starts only after: (a) the single-node and
multi-node gates in 11.1–11.4 pass on one region, (b) a second
region exists to test against, (c) the owner approves the spend.
Until then, 11.6 stays a recorded decision, not a capability.
