# Architecture

Status: written 25 Nov 2026 for job 17.3. Reviewed against the
current tree (`main.go` sha
`9b253fafc2a73a251101fd4435c98ec2c6e17b0f07d94594b6d918b2294fd19a`).
Independent review (by someone who did not write it) is still owed.

## What hami-media is

A small WebRTC video forwarding server: one publisher streams live
video; the server copies it to every viewer in one room. It runs as
one process (or one process per cell in sharded scale proofs). No
cloud service is needed. H.264 only; Opus audio on the audio rung.

## Components (all in this repo)

| Component | File | Role |
|---|---|---|
| Proof harness / CLI | `main.go` (15,604 lines) | All run modes (`pair`, `sfu`, `sfu-single`, `sfu-cascade`, `m7h`, `m9h`, `audio-rung1`, `m7h-sfu`, `turn-server`, `mint-token`, `capacity`); flag parsing; host preflight + watchdog; heap budget |
| Router | `main.go` (`sfuRouter`, track registry) | Holds publishers/tracks; fans a publisher's packets out to every subscriber's queue. Keyed by publisher + track (`sfuTrackKey`); layer keying is open (job 2.6) |
| Publisher / viewer clients | `main.go` (Pion peer connections) | WebRTC peer connections to the router; SRTP replay protection is currently OFF in three places (job 2.1, open) |
| Frame assembler | `main.go` (`videoFrameAssembler`) | Reassembles RTP packets into frames; completes late frames on retransmit arrival (FIX 31); requires a known start boundary |
| Retransmit / NACK | `main.go` (`rememberPacket`, packet cache, NACK workers) | Caches every layer incl. delta packets in the retransmit window (FIX 31); serves NACKs with aggregation (500 ms) and a per-NACK budget |
| Subscriber queues | `main.go` (`sfuSubscription`, `enqueueRTP`) | Per-viewer bounded queues with shared-payload packet copies from `rtpPacketCopyPool`; drops oldest on full, counts drops once |
| Audio mixer | `main.go` (`startAudioRTPMixer`) | In-process Opus mixing for the audio rung |
| Auth | `auth.go` | HMAC-SHA256 join tokens (room, identity, roles, expiry); key >= 32 bytes from file (0600) or env; never logged. DONE (job 8.2) |
| TURN | `turn.go` | Built-in TURN server with time-limited credentials; password file 0600-checked |
| Capacity detector | `capacity.go` + `platform_*.go` | Reads the device (cores, RAM, swap, disk, NICs) and recommends max publishers/viewers from measured anchors, scaled down for weaker machines |
| Metrics | `main.go` (`/metrics`) | Prometheus `hami_sfu_rtp_*` metrics; loopback-only endpoint |
| Latency / scoring | `latency.go` | Per-viewer latency stats, strict vs dissolve scoring, stall gate (p99-median > 50 ms, proven vs CPU starvation only) |

## Data flow (video)

Publisher -> router (one copy per packet into the packet cache) ->
per-viewer queue (shared payload, copied mutable header) ->
viewer peer connection. RTCP feedback (NACK/PLI/FIR) flows back
per viewer; the router maps it to the publisher (per-layer SSRC
mapping is open, job 2.6).

## Run modes

- **Proof modes** (`pair`, `sfu`, `sfu-single`, `sfu-cascade`,
  `m7h`, `m9h`, `audio-rung1`): one-shot measurements that print a
  JSON result and exit. `sfu-single` is the scale proof; `-sfu-cells`
  shards it across worker subprocesses.
- **Serve mode** (`m7h-sfu`): the long-lived server. Adopts an
  inherited TCP listener on fd 3 (systemd socket activation) and
  reads its bearer token on fd 4. See `docs/systemd-install-17.2.md`.
- **Helpers**: `capacity` (print detected limits),
  `mint-token` (mint a join token), `turn-server` (standalone TURN).

## What is NOT in this architecture (open jobs)

- No room model (single global router; job 9.2), no multi-node
  (§11), no packager (§13.1), no supported SDKs (§10), no public
  signaling API or wired request-rate middleware (8.5), no fuzz
  targets (8.6). The M7-H signaling routes are internal loopback
  endpoints only. See `docs/known-limitations.md`.
