# Signaling spec v1 (job 9.1)

Transport: HTTP(S) POST for offer/answer (existing `/publisher` and
`/viewer` endpoints) plus the JSON message format below for the
in-band signaling channel. All messages are JSON objects. The
implementation is `parseSignalingMessage` in `signaling.go`; the
tests in `cluster_c_section9_test.go` run the implementation against
this spec.

## Versioning

- `version` is required and must be `1`. Version 0, a missing version,
  and any future version are rejected with an error — never silently
  accepted or downgraded.
- A future v2 must add a new `version` value and keep v1 accepted
  during migration; the version check is the single gate.

## Message set

| type | required fields | payload |
|---|---|---|
| `join` | room, from | — |
| `leave` | room, from | — |
| `offer` | from, to, payload | SDP offer |
| `answer` | from, to, payload | SDP answer |
| `candidate` | from, to, payload | ICE candidate |
| `publish` | room, from | — |
| `subscribe` | room, from | — |
| `layer-switch` | room, from, payload | `{"layer": "top"\|"middle"\|"base"}` |
| `metadata` | room, from, payload | key/value metadata |
| `message` | room, from, payload | `{"text": "..."}` (data channel, 9.8) |
| `resume` | room, from | — |
| `error` | to, payload | `{"code": "...", "detail": "..."}` |

Unknown types are rejected. Unknown JSON fields are rejected
(`DisallowUnknownFields`). Messages over 64 KiB are rejected (8.4).

## States

- A participant is `joining` → `joined` (after token verification) →
  `publishing`/`subscribing` → `leaving` → `left`.
- A `resume` within 60 s of a drop restores the `joined` state with
  its tracks and metadata (9.6); after 60 s it is an error.
- A `candidate` arriving after the offer/answer is buffered and
  applied in arrival order (trickle ICE, 9.3) — it is never dropped
  for being late.

## Errors

Error messages have type `error` and payload `{"code", "detail"}`.
Codes: `unauthorized`, `bad-version`, `unknown-type`, `bad-message`,
`not-in-room`, `rate-limited`, `room-full`. Every rejection uses the
same generic client-facing message for auth failures (no oracle).

## Authentication

Every `join`/`publish`/`subscribe` carries a signed token (8.2) in
the request, not in the signaling message body. The token names the
room, identity and roles; the server verifies it before any
PeerConnection is created.
