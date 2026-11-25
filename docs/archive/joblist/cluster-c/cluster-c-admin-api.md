# Admin API (job 9.4)

Base: the SFU process. All routes require
`Authorization: Bearer <admin-key>` (separate secret from the HMAC
join key; constant-time compare; missing/wrong → 401). No admin
operation appears in logs with the key.

## Routes

| Method + path | Body | Success | Errors |
|---|---|---|---|
| `POST /admin/rooms` | `{"name": "room-1", "codec": "H264"}` | 201 `{"room": "room-1", "participants": 0}` | 400 duplicate room / unsupported codec / bad profile |
| `GET /admin/rooms` | — | 200 `{"rooms": ["room-1", ...]}` (sorted) | 401 |
| `DELETE /admin/rooms/{name}` | — | 200 `{"deleted": "room-1"}` + `room-ended` webhook | 404 unknown room |
| `POST /admin/rooms/{room}/participants/{id}/kick` | — | 200 `{"kicked": "viewer-1", "room": "room-1"}` + `left` webhook | 404 unknown room/participant |
| `POST /admin/rooms/{room}/participants/{id}/mute` | `{"muted": true}` | 200 `{"participant": "viewer-1", "muted": true}` | 404 unknown room/participant |
| `POST /admin/rooms/{room}/participants/{id}/permissions` | `{"can_publish": true}` | 200 `{"participant": "viewer-1", "can_publish": true}` | 404 unknown room/participant |

Notes:

- `codec` must be a supported codec (H264 today; VP8/VP9/AV1 are
  rejected — job 6.9).
- `permissions` with `can_publish: true` also sets the participant's
  role to `publish`; `false` revokes publish but keeps the participant
  in the room.
- `kick` removes the participant and all its subscriptions; a kicked
  participant must re-join with a fresh token.
- `mute` is server-side state; the client is expected to honour it —
  enforcement in the forwarding path is owed (the mute flag is stored
  on the participant; the router does not yet drop muted audio).
