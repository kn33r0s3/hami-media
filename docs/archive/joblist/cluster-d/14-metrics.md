# 14.1 Metrics — agreed metric set

## Existing (from the forwarder, `/metrics`)

| Metric | Type | Meaning |
|---|---|---|
| `hami_sfu_tracks` | gauge | published tracks |
| `hami_sfu_viewers` | gauge | connected viewers |
| `hami_sfu_subscriptions` | gauge | viewer subscriptions |
| `hami_sfu_nack_cache_packets` | gauge | packets in NACK cache |
| `hami_sfu_nack_cache_payload_bytes` | gauge | cache payload bytes |
| `hami_sfu_rtp_forwarded_packets_total` | counter | packets forwarded |
| `hami_sfu_rtp_forward_errors_total` | counter | forward errors |
| `hami_sfu_rtp_queue_dropped_total` | counter | queue drops |
| `hami_sfu_rtp_intentional_shed_total` | counter | intentional sheds |
| `hami_sfu_rtp_keyframe_recoveries_total` | counter | keyframe recoveries |
| `hami_sfu_rtp_keyframe_recovery_seconds_{sum,max}` | counter/gauge | recovery time |
| `hami_sfu_rtp_forward_seconds` | histogram | forward latency |
| `hami_sfu_rtp_queue_wait_seconds` | histogram | queue wait |
| `hami_sfu_write_sample_seconds` | histogram | write latency |

## New (Cluster D, per room / participant / track)

`hami_media_room_packets_total{room,participant,track,kind}` —
counter, kinds: `packets_forwarded`, `packets_dropped`,
`nack_received`, `keyframes_requested`. Implemented in
`observability.go` (`clusterDMetrics`); rendered by
`prometheusText()`. Test: `TestClusterDMetricsPerRoomParticipantTrack`.

## Rules

- No per-packet logs: only counters. A metric value never contains
  a packet payload, an SDP, a token or a personal identifier.
- Labels are room / participant / track IDs (opaque); cardinality
  is bounded by the room and participant limits (see 8.4 input
  limits). If cardinality ever grows unbounded, the per-participant
  labels drop first (aggregate to room level).
