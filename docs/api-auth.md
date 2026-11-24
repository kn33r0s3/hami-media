# API and auth

## Two layers of auth

1. **Bearer token.** Every signalling endpoint needs it. It is a random 32-byte token made fresh for each server run. It proves the caller is the harness that started this server process. It says nothing about rooms or roles, and it never expires on its own.
2. **Client token.** Minted per client, per room, per role, with an expiry. It proves who the client is and what they may do.

Both are needed. The Bearer token alone cannot publish or subscribe.

## Client tokens

Format: `base64url(payload).base64url(signature)`, HMAC-SHA256, standard library only. No JWT dependency.

The payload is JSON:

```json
{"room": "default", "roles": ["publish", "subscribe"], "sub": "alice", "exp": 1790000000, "iat": 1789996400}
```

- `room`: the one room this server process serves.
- `roles`: a subset of `["publish", "subscribe"]`.
- `sub`: the client identity. The request ID must match it.
- `exp`: expiry as unix seconds. The token is rejected at or after `exp`. No clock-skew allowance.
- `iat`: minted-at unix seconds.

Mint one:

```sh
export HAMI_SFU_HMAC_KEY_FILE=/path/to/hmac-key.txt
./hami-media -mode mint-token -token-subject alice -token-ttl 1h
```

The key comes from `HAMI_SFU_HMAC_KEY_FILE` (preferred, file `0600`) or `HAMI_SFU_HMAC_KEY`. Set only one. Minimum 32 characters. The key is never printed or logged. The room comes from `HAMI_SFU_ROOM` (default `default`).

## Endpoints

All endpoints need the Bearer token. The base URL is the server's loopback listener (the harness owns the address).

| Endpoint | Method | Needs role | What it does |
|---|---|---|---|
| `/health` | — | — | Server health. |
| `/publisher` | POST | `publish` | Publish a track. |
| `/publisher/ready` | — | — | Publisher readiness. |
| `/viewer` | POST | `subscribe` | Subscribe to a track. |
| `/stats` | — | — | Server stats. |
| `/receive-stats` | — | — | Publisher receive stats. |
| `/heap-profile` | — | — | Heap profile. |
| `/goroutine-profile` | — | — | Goroutine profile. |
| `/cpu-profile` | — | — | CPU profile. |
| `/cleanup` | — | — | Clean up state. |
| `/shutdown` | — | — | Shut the server down. |

A request without the Bearer token gets 401. A publisher request with a token that lacks `publish` is refused. Same for viewers and `subscribe`. An expired client token is refused. An unauthorised subscription is denied by the router itself, not just by the HTTP layer: `TestLocalSFURoutesAuthorizedStreamsAndRTCP` proves it.

## How a client connects (short version)

1. Mint a token for the client, with the right room and roles.
2. The client calls `POST /publisher` (or `/viewer`) with the Bearer token and its client token.
3. The server answers with the WebRTC offer/answer exchange.
4. Media flows over WebRTC. The publisher's video is copied to every subscribed viewer.

For the full design notes, see [auth-turn.md](auth-turn.md).

## Reconnects

A client that drops can resume on the same room and token. Frames flow again after reconnect. An expired token is refused on resume. This is a stated gate, not an assumption: it is part of the reconnect checks.

## Metrics

Pass `-metrics-listen 127.0.0.1:9464` to expose a Prometheus endpoint. It is loopback-only on purpose. It reports forwarding counts and resource signals.
