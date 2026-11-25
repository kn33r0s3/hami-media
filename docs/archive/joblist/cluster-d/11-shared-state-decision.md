# 11.5 Shared-state decision

## Decision: in-memory state + explicit reconciliation (no Redis)

The node registry (`multinode.go`, `clusterDNodeRegistry`) is
in-memory per process, with heartbeats and an explicit
`reconcileNodeState` step that re-derives node health from a fresh
snapshot of heartbeats.

## Why not Redis (or another external store)

- The current deployment is one room per SFU process on one machine
  (see 9.2: no room model yet). There is no multi-node production
  deployment to share state between, so an external store would be
  infrastructure without a consumer.
- Redis adds a failure mode (the store itself) that must be operated,
  backed up and secured, and a network dependency on the media path
  (placement/recovery must work when the store is slow).
- The in-memory registry's failure modes are smaller and testable in
  this tree; the reconciliation step is the seam where an external
  store can be plugged in later without changing placement or
  recovery.

## Failure modes (written down, as the Done line requires)

1. **Registry process dies**: node registrations are lost. Rooms
   keep running on their nodes (media does not flow through the
   registry), but new placement and recovery stop until the registry
   restarts and nodes re-register. Mitigation: nodes heartbeat on a
   5 s interval and re-register on restart; the registry is
   stateless to rebuild.
2. **Stale snapshot in reconcile**: a node absent from a snapshot is
   marked dead, but its rooms are NOT dropped — `reconcileNodeState`
   keeps them (test `TestClusterDReconcileNeverDropsRooms`). Rooms
   move only through `recoverDeadNode`, which re-places them on live
   nodes. A stale snapshot can therefore never silently drop a room;
   at worst it triggers a recovery that re-places rooms that are
   still alive (duplicate placement is prevented by `placeRoom`
   capacity checks, and the old node's rooms are cleared only after
   successful re-placement).
3. **Split brain (two registries)**: if two registries run, each
   places rooms independently and both may place the same room.
   Mitigation: exactly one registry per deployment (enforced by
   operator runbook, not by code — recorded here as an open
   operational requirement, not a solved problem).
4. **Heartbeat loss without node death**: a node that is alive but
   cannot heartbeat for >30 s is declared dead and its rooms are
   recovered elsewhere. The old node keeps serving its rooms until
   it notices it is drained; duplicate serving is bounded by the
   room's single-owner rule in the cascade (`placeRoom` refuses a
   room already owned by a live node — enforced by callers checking
   `roomsOf` before placement).
5. **Clock skew**: heartbeat ages use the registry's own clock
   (`reg.now`), so skew between nodes does not affect detection;
   only the registry's clock matters.

## What would change the decision

If a second machine is added (7.7) and rooms must survive a
registry restart without re-registration, the reconciliation
snapshot source becomes a durable store (Redis or etcd). The
registry interface (`register`/`heartbeat`/`placeRoom`/
`recoverDeadNode`) is the seam: only the snapshot source changes.
