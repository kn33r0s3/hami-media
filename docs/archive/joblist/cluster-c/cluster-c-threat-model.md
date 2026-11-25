# Hami-media threat model (job 8.1)

Current implementation caveat: this historical threat model describes a
planned public signaling boundary. The implemented M7-H `/publisher` and
`/viewer` routes are internal loopback-only proof endpoints, and no
request-rate middleware is wired to them. See the current
[API boundary](../../../api.md) and
[known limitations](../../../known-limitations.md) before relying on
this draft.

Reviewed: not yet reviewed by a second person — review is owed before
this is treated as final (see §9, open items).

## Assets

| Asset | Why it matters |
|---|---|
| Live media (publisher video/audio) | The product. Leaked or hijacked streams are the primary harm. |
| Join tokens (HMAC) | Bearer-equivalent: whoever holds one can publish/subscribe as that identity until expiry. |
| HMAC key, TURN password, admin key | Minting/forging any credential. Compromise = full impersonation. |
| Participant metadata / identities | Who is in which room, display names. Privacy, not just secrecy. |
| Server capacity (CPU, bandwidth, UDP ports) | Exhaustion is a denial-of-service on every room on the node. |
| Webhook deliveries + signatures | Forged events could trigger downstream actions (recording, billing). |
| Recordings / outputs (when 13.x lands) | Stored media outlives the call. |

## Actors

- **Legitimate publisher/viewer**: holds a short-lived token for one room, one identity, named roles.
- **Unauthenticated network attacker**: can reach the signaling and TURN ports; cannot mint tokens without the key.
- **Authenticated but malicious participant**: holds a valid token; tries to exceed its grants (publish when subscribe-only, join another room, subscribe to another track, switch layers it is not on).
- **Compromised / stolen token holder**: replays a token until expiry.
- **Resource-exhaustion attacker**: floods joins, signaling, SDPs, candidates, tracks.
- **Webhook receiver attacker**: forges events or replays signed deliveries.
- **Insider / operator**: holds keys; mistakes (key in a log, world-readable key file) are in scope.

## Trust boundaries

1. **Public internet ↔ signaling server**: untrusted. All input validated (8.4 limits, 9.1 spec, 8.5 rate limits). Tokens verified before any PeerConnection.
2. **Signaling ↔ media (DTLS/SRTP)**: media keys come from the DTLS handshake, never from signaling. Signaling compromise alone does not decrypt media.
3. **SFU ↔ TURN**: TURN credentials are time-limited (7.2); relay allocations are bounded by port range.
4. **SFU ↔ webhook receiver**: signed with HMAC; receiver must verify (9.5) — an unsigned/unsigned-verified event is untrusted.
5. **Operator ↔ secrets**: secrets come from env/file only (8.8); the process refuses to start without them. Secrets never appear in logs, tokens, or repo files (leak scan, R2).
6. **Admin API ↔ rooms**: separate bearer key; admin can kick/mute/change permissions (9.4). Admin key compromise = room control, not media decryption.

## Abuse cases and mitigations (implemented in Cluster C unless noted)

| Abuse | Mitigation | Status |
|---|---|---|
| Forged / expired / wrong-room / wrong-role token | HMAC verify + expiry + room + role + subject (auth.go, 8.2 DONE) | Done |
| Subscribe to a track you are not allowed on | authorizeSubscription + authorizeLayerSwitch (8.3) | Done (tests) |
| Publish with a subscribe-only token | Role check on publish path (auth.go) + room.publishTrack CanPublish (9.2) | Done |
| Oversized SDP / candidate flood / track flood / room flood | 8.4 limits (64 KiB SDP, 64 candidates, 8 tracks/participant, 1024 rooms/node) | Done (tests) |
| Join / signaling flood | 8.5 rate limits (10/min/IP, 5/min/account joins; 60/30 signaling) | Done (tests) |
| Stolen token replay | Short TTLs (max 24 h, mint short in practice); no revocation — recorded as a limitation | Limitation |
| Malformed SDP/RTP/RTCP/JSON crashes the server | 8.6 fuzz targets + fixed-time run script | Done (targets + script; CI wiring owed) |
| Known-vulnerable dependency | 8.7 vuln scan script (govulncheck) | Done (script; CI wiring owed — cluster A owns workflow) |
| Secret in repo / log | 8.8 env/file only + fail-closed start; leak scan before every write (R2) | Done |
| Weak TLS on signaling | 8.9 TLS 1.2+ listener, TLS 1.1 refused | Done (listener); external TLS scan owed (reference machine) |
| SRTP/DTLS implementation flaw | 8.10 RFC 3711 vector check via pion/srtp | Done (SRTP part); DTLS relies on Pion's dtls/v3 + existing pair tests |
| Forged webhook event | HMAC signature; bad signatures rejected (9.5) | Done (tests) |
| Admin API abuse | Separate bearer key, constant-time compare (9.4) | Done |
| TURN credential theft | Time-limited usernames (7.2); expiry enforced | Done (check); production AuthHandler wiring owed |
| TURN open relay | Auth required; relay port range bounded; no anonymous allocation | Done (existing turn.go) |
| One stalled/slow viewer stalls others | Slow-viewer policy is job 4.7 (cluster A/B) — not in this threat model yet | Open (other cluster) |
| SRTP replay | Job 2.1 (cluster A/B): replay protection is OFF in three places in main.go | Open (other cluster) — KNOWN GAP, do not claim otherwise |

## Open items

1. Second-person review of this document.
2. External TLS scan (8.9) and external penetration test (8.11, NEEDS-APPROVAL — do not start).
3. Token revocation: currently none; if tokens must be revocable, add a revocation list or shorten TTLs further.
4. Rate-limit storage is in-memory per process; a multi-node deployment (11.x) needs shared counters.
