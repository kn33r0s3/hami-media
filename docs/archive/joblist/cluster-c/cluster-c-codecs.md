# Codec set per room (job 6.9)

## Supported — tested end to end

- **H.264** (payload type 102, `profile-level-id=42001f`,
  `packetization-mode=1`, `level-asymmetry-allowed=1`). Registered by
  `registerH264OnlyCodecs` in main.go. The only codec in this build.

## Unsupported — listed, not tested end to end

- **VP8**: not registered. No VP8 encoder/decoder in this build.
  `codecForRoom` returns an error naming VP8 as unsupported.
- **VP9**: not registered. No VP9 encoder/decoder in this build.
- **AV1**: not registered. No AV1 encoder/decoder in this build.

An unsupported codec is an error, never a silent fallback to H.264:
a room created with VP8/VP9/AV1 is rejected at creation (admin API)
and at publish time (`room.publishTrack`).

## Profile-level-id (job 6.2)

Our offer is `42001f` (Baseline, level 3.1). Before fixing it, each
target client's value was checked:

- `42001f` — Baseline, no constraint flags. Negotiates.
- `42e01f` — Constrained Baseline, same profile and level, different
  constraint flags. Negotiates (constraint flags are ignored in the
  comparison; `level-asymmetry-allowed=1` is already in our fmtp line).
- `4d001f` — Main profile. Does NOT negotiate with Baseline.
- `64001f` — High profile. Does NOT negotiate with Baseline.

The negotiation is `negotiateProfileLevelID` in `profiles.go`, tested
in `TestClusterCProfileLevelIDNegotiation`.
