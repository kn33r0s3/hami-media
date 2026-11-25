# LiveKit forwarder comparison (19.1) — READ ONLY

Job 19.1: a written comparison of LiveKit's forwarder (deadline
model, drop policy, codec set, room model) citing files at the pinned
version. **Read only: no LiveKit code is copied.** All observations
below are from reading the source; nothing was copied into this repo.

Status: **DONE 25 Nov 2026.**

## Pinned version

LiveKit **v1.13.11** (source checked out at tag `v1.13.11` from
`github.com/livekit/livekit`, sparse checkout of `pkg/rtc`,
`pkg/sfu`, `pkg/config`; the binary pin is the same version, see
`docs/archive/livekit-install-16.3.md`). License: **Apache License 2.0**
(`pkg/sfu/forwarder.go` header; repo `LICENSE`); LiveKit, Inc.
copyright. Any future reuse of LiveKit code requires the Apache 2.0
notice — this document is a comparison, not a reuse.

LiveKit's Pion is its own warp fork: `go.mod` replaces
`github.com/pion/webrtc/v4` with `github.com/livekit/webrtc-pion/v4
v4.2.18-warp.7` (upstream Pion v4.2.18 + warp patches). hami-media
uses upstream Pion v4.2.26.

## Deadline model

LiveKit's forwarder has **no per-frame deadline** of the kind
hami-media is building (job 4.2: abandon a frame at its deadline and
advance the cursor). What it has instead is a set of
time-thresholds around layer switching and resumption, all in
`pkg/sfu/forwarder.go`:

- `ResumeBehindThresholdSeconds = 0.2` (200 ms) and
  `ResumeBehindHighThresholdSeconds = 2.0` (2 s): how far behind a
  switch point the forwarder may resume.
- `LayerSwitchBehindThresholdSeconds = 0.05` (50 ms),
  `SwitchAheadThresholdSeconds = 0.025` (25 ms).
- `initialLayerAcquisitionGrace = time.Second`: while a subscriber
  is acquiring its first layer, aim straight for the requested layer
  for 1 s instead of latching onto a lower layer seen first
  (`withinAcquireGraceLocked`, `MaybeExpireAcquireGrace`).

Deadlines in LiveKit are therefore about *where* to (re)join a
stream, not about *when* to abandon a frame. hami-media's 4.2
deadline (abandon the frame, advance the cursor) is a different
mechanism with no direct LiveKit counterpart in the forwarder.

## Drop policy

Drops in `pkg/sfu/forwarder.go` (`getTranslationParamsVideo`,
`translateCodecHeader`) are layer- and state-driven, not
time-driven:

- When the subscriber is **DEFICIENT** (bandwidth-constrained) and
  the target layer is lower than the current layer, higher-layer
  packets are dropped (`shouldDrop = true`) — "continuing to send
  higher layer will only exacerbate the situation" (comment at
  ~line 2332). In the other direction (switching up), forwarding
  continues until the switch point for a smoother stream.
- The codec munger drops filtered temporal layers and out-of-order
  VP8 picture-ID cache misses (`ErrFilteredTemporalLayer`,
  `ErrOutOfOrderVP8PictureIdCacheMiss`); a dropped packet's sequence
  slot is either registered as dropped with the RTP munger
  (`PacketDropped`) or padded (`padDroppedSlot`) so downstream
  sequence numbers stay contiguous.
- NACK-driven retransmission lives in `pkg/sfu/downtrack.go`: a
  retransmit worker serves NACKed sequences, capped at
  `maxPendingNACKs = 5000` buffered between runs, with an RTX budget
  (`rtxbudget.go`) and NACK throttling (`isNACKThrottled`; a PLI is
  sent at most once per loss episode). There is no retransmit-window
  cache horizon expressed in milliseconds (contrast hami-media job
  4.3, which wants one millisecond value driving queue age, NACK
  window and cache horizon).

hami-media's drop accounting (queue drops counted once, drops
requesting a keyframe, job 4.4) has no direct LiveKit analogue in
the forwarder: LiveKit drops by layer/bandwidth state and relies on
PLI + NACK retransmit for recovery.

## Codec set

`pkg/rtc/mediaengine.go` (`registerCodecs`) registers, from
`livekit/protocol` `VideoCodecsParameters` (read from
`livekit/protocol` main branch `codecs/codecs.go`, the dependency
the pinned server builds against):

- **VP8, VP9 (profile 0 and 1), H.264 (42e01f + high profile),
  AV1, H265**, plus Opus audio and RTX/FlexFEC.
- Extra strict-fmtp H.264 registrations: `42001f` and `4d001f`,
  packetization modes 0 and 1.
- Default fallback codec when no alternative is selected: **VP8**
  (`selectAlternativeVideoCodec`).

hami-media registers **H.264 only** (`registerH264OnlyCodecs`,
`main.go:11767`). The gap is job 6.9 (each of H.264, VP8, VP9, AV1
either tested end to end or listed as unsupported). LiveKit's H.264
profile-level-id set (42e01f, 42001f, 4d001f) is the reference for
job 6.2.

## Room model

`pkg/rtc/room.go`: `Room` holds a `participants` map (keyed by
participant identity), a `RoomTrackManager`, per-room config
(`WebRTCConfig`, `config.RoomConfig`), telemetry, egress launcher
and agent dispatches. `Room.Join` enforces `MaxParticipants`
(`ErrMaxParticipantsExceeded`) and `EmptyTimeout` (the room's empty
timeout, defaulted from config). Rooms are first-class: many rooms
per node, each with its own participants, tracks and config.

hami-media has a **single global router / one room per process**
(the token names the room; job 9.2, room model, is open). The gap
is structural, not a tuning difference.

## Other verified LiveKit facts (prior verification, restated)

Standalone server (binary/Docker); per-user JWT tokens;
room/participant API; reconnect/resume (protocol reconnect flag,
SyncState, ReconnectResponse); TURN incl. TURN/TLS; simulcast;
NACK recovery; many SDKs; Prometheus `:6789/metrics`; `lk`
load-test tool; ~3,000 users/room documented ceiling.

## What this comparison does not claim

- No performance claim: the head-to-head is 16.4/16.6/16.7, not
  this document.
- No line of LiveKit code was copied into this repo. The only
  LiveKit artifact here is this prose comparison and the pinned
  binary's version/config record (16.3).
