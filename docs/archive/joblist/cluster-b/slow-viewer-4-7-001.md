# Job 4.7 — Slow-viewer policy: step down a layer, then drop — DONE

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdict: DONE (router-level; Mac end-to-end leg OPEN — see below)

## Design

State on `sfuSubscription` (all atomics; one writer goroutine per
subscription at a time — forwardTrack on the direct path, shard.run
on the shard path):

- `maxLayer` — highest layer still forwarded, PLUS ONE (0 = unset =
  all layers, the default for test-built subscriptions; 1 = layerBase
  only, 2 = layerKey, 3 = layerDelta). `maxLayerValue()` /
  `setMaxLayer()` hide the encoding.
- `queueFullStrikes` — consecutive queue-full drops (8 =
  `slowViewerStepDownStrikes` triggers a step). A successful enqueue
  resets it, so a viewer that recovers stays at its layer.
- Counters: `layerStepDowns`, `layerFiltered`, `slowViewerDrops`.

Policy (`recordQueueFullDrop`, called from forwardTrack's dropped
branch with no locks held): 8 consecutive queue-full drops → step
down one layer (delta → keyframe → base); a viewer already at
layerBase is dropped — `stop()` (active=false, queues closed) and
`slowViewerDrops` counted. The drop is a media-path drop: the viewer
receives nothing further and its nackWorker exits. The subscription
stays in the router maps until an explicit `unsubscribe` (no map
mutation from the hot forward path; recorded limitation, not hidden).

Layer filter (`filterLayer`, counted): forwardTrack checks it before
`enqueueRTP`; shard.run checks it in its fan-out loop before
`forwardQueuedRTP`. A filtered packet is intentional degradation, not
queue loss — no keyframe-recovery request for it (the viewer still
receives keyframes, so the P-chain restarts cleanly).

`subscribe()` starts every viewer at `layerDelta`.

## Shard-path limitation (recorded, not hidden)

The shard queue is shared across a shard's subscriptions, so there is
no per-subscription queue-full signal on the shard path — the strike
hook lives on the direct path only. The shard path gets the layer
filter (a stepped-down viewer stays stepped-down when shards are on).
Per-subscription queue accounting in the shard path would be needed
to drive strikes there; that is follow-up work, not part of this job.

## What "step down" means without simulcast (6.4)

There is one H.264 stream, classified per packet: SPS/PPS =
layerBase, keyframe = layerKey, delta = layerDelta. Stepping down to
layerKey means keyframes only (the viewer freezes between keyframes
but stays alive); layerBase means parameter sets only (essentially
nothing viewable). A true multi-layer step-down (simulcast renditions)
needs 6.4 — recorded honestly, not claimed here.

## Tests (`slowviewer_test.go`, all passing, `-race` clean)

- `TestSlowViewerMaxLayerDefault`: zero value = all layers;
  `setMaxLayer(layerKey)` caps the filter; only packets above the cap
  are filtered and counted.
- `TestSlowViewerPolicyStepsDownThenDrops`: a subscription with an
  8-packet queue and no drain, driven by the forwardTrack dropped
  branch in miniature (filter → enqueue → strike on drop, reset on
  success), stream cycling base/key/delta/delta. The ladder is
  traversed in order (step to layerKey, then layerBase, then dropped),
  with `layerStepDowns == 2`, `slowViewerDrops == 1`,
  `layerFiltered > 0`, `rtpQueueDropped > 0`.
- `TestSlowViewerShardLayerFilter`: real `shard.run()` fan-out — a
  subscription capped at layerKey does not receive a delta job
  (`layerFiltered == 1`) but does receive a keyframe job
  (`rtpWriteSucceeded == 1`).

## OPEN — Mac end-to-end leg

forwardTrack itself (real publisher → real ReadRTP → throttled viewer)
cannot be driven in this VM (no UDP; WriteRTP on an unbound track is
a no-op, so forwardTrack never receives a packet). The wiring in
forwardTrack's dropped branch is verified by code inspection and by
the tests above calling the same methods in the same order. Mac run
owed: a scale-proof run with one throttled viewer (e.g. a viewer that
never drains), asserting the same ladder in the router diagnosis
(step-downs, drops, filtered counts). Suggested shape — run the
existing local SFU proof with a slow viewer and read the counters;
exact command to be set by the Mac harness owner (no Mac access from
this VM).
