# 14.6 Runbook — one entry per alert

Each alert in `observability/alerts.yaml` links here. An alert
without a runbook entry is a bug (the test
`TestClusterDAlertFires` asserts every rule has a runbook).

## high-queue-drops (warning)

- **Fires when**: `packets_dropped` > 100 for 60 s.
- **Meaning**: viewers' queues are overflowing; the forwarder is
  dropping packets faster than viewers drain them.
- **Check**: `/metrics` — which room/track has the drops
  (`hami_media_room_packets_total{kind="packets_dropped"}`); check
  whether one viewer or all viewers are affected.
- **Actions**: (1) check the slow-viewer policy (4.7) — a stuck
  viewer should step down a layer, then be dropped; (2) check CPU
  on the node (a saturated node drops for everyone); (3) if one
  room only, check that room's publisher bitrate against its
  profile cap.
- **Escalate**: if drops persist after the viewer is dropped, the
  node is saturated — drain the node (11.1) and move rooms.

## no-forwarding (critical)

- **Fires when**: `packets_forwarded` < 1 (no packets forwarded at
  all while rooms exist).
- **Meaning**: the media path is dead; viewers see frozen video.
- **Check**: `/health` (readiness), process liveness, then
  `/metrics` for `hami_sfu_rtp_forward_errors_total`.
- **Actions**: (1) restart the forwarder process (14.9 — clients
  reconnect and rejoin); (2) if the restart does not restore
  forwarding, check the publisher side (is anyone publishing?) and
  the TURN/ICE path.
- **Escalate**: page the on-call; this is a service outage.

## room-count-high (warning)

- **Fires when**: rooms > 1000 for 5 min.
- **Meaning**: approaching the room limit; placement may start
  failing (11.2).
- **Check**: node registry capacity (`/metrics` rooms gauge per node).
- **Actions**: add a node to the registry; check for leaked rooms
  (rooms with no participants that were never closed).

## keyframe-recovery-slow (warning)

- **Fires when**: max keyframe recovery time > 2 s for 60 s.
- **Meaning**: after a loss, viewers wait too long for a keyframe;
  they see frozen or corrupted video.
- **Check**: `hami_sfu_rtp_keyframe_recovery_seconds_max` and the
  publisher's PLI/FIR handling (6.11).
- **Actions**: check the publisher is honouring keyframe requests;
  check loss on the publisher uplink (7.5 impairment matrix).
