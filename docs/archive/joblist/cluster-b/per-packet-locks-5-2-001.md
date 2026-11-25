# Job 5.2 — Remove per-packet locks from the forward path — DONE (no removal)

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdict: DONE — no locks removed, by measurement and by inventory

5.1 does not implicate any per-packet lock, so nothing is removed.

## Per-packet lock inventory (verified by reading the code, 25 Nov 2026)

| site | lock per packet? |
|---|---|
| `forwardTrack` direct branch | NO. `r.mu.RLock` only on the version-gated snapshot rebuild (subscriber set changes) and on the ReadRTP-error path. Steady state: zero router locks per packet. |
| `enqueueRTP` | YES — `s.mu.RLock`, one per packet per viewer. The ONLY per-packet lock in the forward path. |
| `forwardQueuedRTP` | NO locks. |
| `shard.enqueue` / `shard.run` | NO locks on the per-packet path (shard.mu only on add/remove snapshot). |
| `handleNACK` | NO locks (`retransmittedAt` is nackWorker-owned). |

## Why the one remaining lock stays

`enqueueRTP`'s `s.mu.RLock` is shared-mode: 10,000 concurrent
readers do not block each other. It only serialises against a
writer — `stop()` (unsubscribe) taking `s.mu.Lock()` — which is
rare (once per viewer lifetime). Its purpose is safety, not
exclusion: without it, a send on `rtpQueue` could race `stop()`
closing that queue, and a send on a closed channel panics. Removing
it is not a deletion but a redesign (the close-based queue lifecycle
would have to go — e.g. a done-channel worker exit instead of
closing the queue), for an unmeasured gain. No mutex profile from a
media window implicates it (see the 5.1 note: the router-lock verdict
is still OPEN for a Mac run).

## Test (`perpacketlocks_test.go`, passing, `-race` clean)

`TestEnqueueRTPConcurrentStopDoesNotPanic`: 8 goroutines × 200
enqueues racing a `stop()` — no panic, no race, subscription
inactive afterwards. This guards the lock's purpose: if the RLock
were ever removed, this test (especially under `-race`) is the trip
wire for the send-on-closed-channel race.

## Reopen condition

A Mac media-window mutex profile (5.1 command) showing `s.mu` /
`sfuSubscription` in top contention reopens this job. Until then the
decision stands: keep the RLock.
