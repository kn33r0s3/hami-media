# Job 4.5 — NACK feedback-age profiles (interactive / broadcast / recording) — DONE

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdict: DONE

## Design

`sfuNACKProfile` (string type; zero value is broadcast). Ages:

| profile | feedback age |
|---|---|
| interactive | 100 ms |
| broadcast | 500 ms (default) |
| recording | 2000 ms |

- `parseNACKProfile` parses `HAMI_SFU_NACK_PROFILE` (empty = broadcast;
  unknown = error, returned by `newSFURouter`). The router stores the
  default in `sfuRouter.nackProfile`; `subscribe()` copies it onto each
  subscription (`sfuSubscription.nackProfile`).
- `sfuSubscription.nackFeedbackMaxAge()` returns the subscription's
  profile age. Both stale checks read it: the direct `nackWorker`
  and the shard `run()` NACK-job branch.
- The old `nackFeedbackMaxQueueAge = 10s` const is DELETED (its comment
  claimed the feedback queue had no age limit, which was false — both
  paths enforced 10 s). Nothing else referenced it.
- These are technical presets, not the product latency decision (job
  19.4, owner-written).

## Tests (`nackprofile_test.go`, all passing, `-race` clean)

- `TestParseNACKProfile`: all three names parse, empty = broadcast,
  "bogus" errors.
- `TestNACKProfileFeedbackMaxAge`: 100 / 500 / 2000 ms, zero value =
  500 ms.
- `TestNACKProfileEnvWiring`: `newSFURouter` defaults to broadcast,
  reads "interactive" from the env, rejects "bogus".
- `TestNACKProfileStaleBoundary` (router-level, real nackWorker +
  handleNACK, no UDP): for EACH profile, a NACK aged age−1 ms is
  retransmitted (writes == 1, stale == 0) and a NACK aged age+1 ms is
  dropped as stale (stale == 1, writes == 0).
