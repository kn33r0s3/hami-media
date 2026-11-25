# Job 4.3 — One millisecond deadline driving queue age, NACK window, cache horizon — DONE

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdict: DONE

## The single value

`sfuRetransmitWindow` (`time.Duration` var, default 500 ms), set by the
existing `-nack-max-queue-age` flag (flag name kept for compatibility;
help text now says it is the single retransmit window). It drives all
three horizons:

1. **Queue age** — `forwardQueuedRTP` drops packets older than the
   window (`if queueWait > sfuRetransmitWindow`).
2. **NACK window** — the `nackAggregationWindow` const (500 ms) is
   DELETED; `selectNACKSequences` and `filterUpstreamForwardable` now
   read `sfuRetransmitWindow` directly. No sync point, no second knob.
3. **Cache horizon** — `rememberPacket` evicts packets older than the
   window (`evictExpiredPackets`, head-of-ring loop); `cachedPackets`
   prunes the same way, so a NACK is never served a packet older than
   the window. New fields on `sfuPublishedTrack`: `packetReceivedAt
   map[uint16]time.Time` + `packetClock func() time.Time` (nil means
   `time.Now`; tests inject a fake clock). The 512-packet ring stays as
   a hard cap, not a horizon: at 5 fps it would otherwise hold ~34 s
   of packets.

Renames (mechanical, all references updated): `nackMaxQueueAge` →
`sfuRetransmitWindow`, `defaultNACKMaxQueueAge` →
`defaultRetransmitWindow`. The `NACKMaxQueueAge` JSON field name in
the diagnostics struct is kept (output compatibility); its value is
now the single window. Two existing tests updated to the new name
(`TestStaleRTPIsIntentionallyShedAndMarksKeyframeRecovery`,
`TestFilterUpstreamForwardableDedupsWithinWindow`).

What was NOT changed: `maxSequencesPerNACK` (512) and
`nackGapMaxJump` (1024) stay — they are per-NACK / per-gap bounds, not
horizons. The job's "packet-count limits" is the cache horizon (the
ring as the only horizon), which is now time-based.

## Safety note (recorded in the code)

The window is safe with `nackGapMaxAttempts = 1` (no NACK retries):
detection is ~one frame interval (~200 ms at 5 fps), inside the
window. A retry at 600 ms would find its packet evicted, so
re-enabling retries needs a window above the retry interval.

## Tests (`retransmit_test.go`, all passing)

- `TestRetransmitWindowDrivesCacheHorizon`: fake clock. At 6 pkt/s a
  packet is kept at age 499 ms and evicted at 501 ms. At 187 pkt/s
  (100 packets over ~535 ms) the same boundary holds: packets 1–7
  evicted (oldest ~503 ms), packet 8 kept (~498 ms), 93 cached.
  Same age boundary, very different counts — proving time, not count,
  drives the horizon. Then the window is moved to 200 ms and the
  boundary moves with it (kept 199 ms, gone 201 ms).
- `TestRetransmitWindowDrivesNACKAggregation`: window set to 200 ms;
  a retransmit at +199 ms is deduped, at +201 ms it is fresh.
- `TestRetransmitWindowDrivesQueueAge`: window set to 200 ms; a
  packet aged 199 ms forwards, aged 201 ms is shed as stale and marks
  keyframe recovery.

## Full suite

`log (sha256 ea25bd02e417, 71 lines; kept outside repo)` — exit 1, 124.565 s, **exactly the known
six VM UDP failures** (`TestLocalPairReceivesAudioAndVideoRTP`,
`TestCountingUDPMuxCountsPeerWritesWithoutClosingSharedMux`,
`TestPacketReadCountingConnCountsDatagrams`,
`TestLocalSFURoutesAuthorizedStreamsAndRTCP`,
`TestLocalSFUCascadeRoutesRTCPAcrossWorkers`,
`TestSFUTURNRelayCarriesMedia`). No regressions from the window
unification or the cache eviction.
