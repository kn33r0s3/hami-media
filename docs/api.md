# API

Status: written 25 Nov 2026 for job 17.3, updated 26 Nov 2026 for the
browser streamer (Part 4). Covers the m7h-sfu server's HTTP API as
implemented in `main.go` (`runM7HSFUServer`) and the browser-facing
serve mode in `serve.go` (`runServeMode`). Independent review (by
someone who did not write it) is still owed.

## Deployment boundary

The M7-H listener is inherited by the child process from its local
harness and is loopback-only. Its process Bearer token authenticates that
parent process and must not be exposed to browsers.

The serve mode (`-mode=serve`) opens its own listener on `--serve-listen`
(default `127.0.0.1:8080`). Browser clients authenticate with HMAC
tokens (see `docs/api-auth.md`), not the process bearer token.

## Authentication

**M7-H mode:** Every endpoint takes the server's bearer token:
`Authorization: Bearer <token>` (passed on fd 4). Without it: **401**.

**Serve mode:** Browser endpoints take an HMAC token in the JSON request
body (`"token"` field) or query parameter (`?token=`). The token is
minted with `-mode=mint-token`. Join tokens for publishers/viewers are
HMAC-SHA256 (room, identity, roles, expiry) — see `docs/api-auth.md`.

## Browser signaling contract (serve mode)

### POST /publisher

Publish media tracks. Requires a publish token.

**Request:**
```json
{
  "publisher_id": "streamer-1",
  "offer": "<SDP offer string>",
  "token": "<HMAC publish token>",
  "display_name": "Alice"
}
```

**Response (200):**
```json
{
  "answer": "<SDP answer string>"
}
```

**Errors:** 400 (bad request), 401 (invalid token), 409 (already exists),
503 (publisher limit reached).

### POST /publisher/leave

Stop publishing without leaving the room. Removes the publisher's
tracks from every viewer. Requires a publish token.

**Request:**
```json
{
  "publisher_id": "streamer-1",
  "token": "<HMAC publish token>"
}
```

**Response (200):**
```json
{
  "left": "streamer-1"
}
```

### POST /viewer

Subscribe to a publisher's tracks. Requires a subscribe token.

**Request:**
```json
{
  "viewer_id": "viewer-1",
  "subscriber_id": "viewer-1-streamer-1",
  "key": {
    "publisher_id": "streamer-1",
    "track_id": "<video track id from GET /rooms/{room}/members>"
  },
  "offer": "<SDP offer string>",
  "token": "<HMAC subscribe token>"
}
```
`viewer_id` must match the subscribe token subject. `subscriber_id` is an
optional unique connection ID used when one viewer subscribes to multiple
publishers; older clients may omit it.

**Response (200):**
```json
{
  "answer": "<SDP answer string>"
}
```

### GET /rooms/{room}/members?token=\<subscribe-token\>

Returns the list of live publishers and their registered media tracks.
Track IDs are assigned by WebRTC at runtime; clients should select the
publisher's track whose `kind` is `video` rather than assuming a fixed ID.
Immediately after a publisher joins, its `tracks` list may still be empty
while its media tracks are being registered.

**Response (200):**
```json
{
  "members": [
    {
      "id": "streamer-1",
      "name": "Alice",
      "room": "default",
      "joined_at": "2026-11-26T12:00:00Z",
      "tracks": [
        {"id": "runtime-track-id", "kind": "video"},
        {"id": "runtime-audio-id", "kind": "audio"}
      ]
    }
  ]
}
```

### GET /rooms/{room}/events?token=\<subscribe-token\>

Server-Sent Events stream. Events:

- `publisher-joined` — data: `{"id":"...","name":"...","room":"...","joined_at":"...","tracks":[]}`. Fetch the member list to discover tracks after registration.
- `publisher-left` — data: `{"id":"..."}`
- `chat` — data: `{"from":"...","text":"...","at":"..."}`

On connect, the last 50 chat messages are replayed.

### POST /rooms/{room}/chat

Send a chat message. Either publish or subscribe token accepted.

**Request:**
```json
{
  "token": "<HMAC token>",
  "text": "Hello everyone!",
  "from": "Alice"
}
```

**Response (200):** `{"sent":"ok"}`

**Errors:** 400 (text empty or > 300 chars), 401 (bad token), 429 (rate limited).

### GET /health

**Response (200):** `{"status":"ready"}`

### GET /stats

**Response (200):**
```json
{
  "publishers": 1,
  "viewers": 2
}
```

## M7-H Endpoints (all on the fd-3 listener, loopback by deployment)

| Method + path | Purpose |
|---|---|
| `GET /health` | `{"status":"ready"}`. Returns ready unconditionally — it does not check dependencies (job 14.4 partial). |
| `POST /publisher` | Publish a track (offer/answer exchange). |
| `GET /publisher/ready` | Publisher readiness. |
| `POST /viewer` | Subscribe a viewer to a track. |
| `GET /stats` | Server stats (rooms, publishers, viewers, forwarding counts). |
| `GET /receive-stats` | Publisher receive statistics. |
| `GET /heap-profile` | Go heap profile (bounded, local file). |
| `GET /goroutine-profile` | Goroutine profile (local file only). |
| `GET /cpu-profile` | CPU profile, bounded duration (max enforced, rejects over-limit requests). |
| `POST /cleanup` | Remove stale publishers/tracks; returns cleanup verification. |
| `POST /shutdown` | Graceful shutdown. |

## Metrics endpoint (separate)

`-metrics-listen 127.0.0.1:9464` enables `GET /metrics` (Prometheus
`hami_sfu_rtp_*`). Loopback-only; rejected otherwise (pinned by
`TestMetricsEndpointIsLoopbackOnlyAndServesPrometheus`).

## Request limits

- JSON bodies are capped at 4 MB (`decodeM7HJSON`).
- Chat messages are capped at 300 characters. Chat is rate-limited
  at 10 messages/minute per IP.
- No admin API for rooms/participants yet (job 9.4, open); no
  webhooks (9.5), no data channel (9.8).

## Errors

Errors are JSON (`{"error": "..."}`), with `status` set to
`ABORTED-BY-HOST` when the host watchdog aborts a run (job 18.4).
