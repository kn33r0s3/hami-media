# API

Status: written 25 Nov 2026 for job 17.3. Covers the m7h-sfu server's
HTTP API as implemented in `main.go` (`runM7HSFUServer`). Independent
review (by someone who did not write it) is still owed. The server
implements an internal proof API, not a public signaling service; the
archived signaling document is a design record, not an implemented
contract.

## Deployment boundary

The listener is inherited by the M7-H child process from its local
harness and is loopback-only. Its process Bearer token authenticates that
parent process and must not be exposed to browsers or other clients. The
routes below are not a public client API. The Web prototype under
`sdk/web/` does not match these routes or request contracts; see
[`sdk.md`](sdk.md).

## Authentication

Every endpoint takes the server's bearer token:
`Authorization: Bearer <token>` (the token the parent passes on fd
4). Without it: **401**. Join tokens for publishers/viewers are
HMAC-SHA256 (room, identity, roles, expiry) — see `docs/api-auth.md`.

## Endpoints (all on the fd-3 listener, loopback by deployment)

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
- No request rate-limit middleware is wired to these handlers. The
  `limits.go` helpers have unit tests only; the per-IP policy cannot be
  applied meaningfully to this inherited loopback listener because all
  simulated peers share its loopback source address. Do not treat the
  tests as protection for a public endpoint.
- No admin API for rooms/participants yet (job 9.4, open); no
  webhooks (9.5), no data channel (9.8).

## Errors

Errors are JSON (`{"error": "..."}`), with `status` set to
`ABORTED-BY-HOST` when the host watchdog aborts a run (job 18.4).
