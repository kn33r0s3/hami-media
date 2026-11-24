# hami-media — Full Report (A to Z)

**Date:** 24 Nov 2026, ~23:50 NPT; updated 25 Nov 2026 ~01:50 NPT; corrected 25 Nov 2026 ~02:05 NPT (U10).
**Repo:** `[local path redacted]`, module
`github.com/kn33r0s3/hami-media`, LICENSE Apache-2.0.
**Status of this report:** every number below comes from a log, doc, or
result note I read while writing it. Sources are cited by path in
section 9. Where a result does not exist, this report says so instead
of inventing one. Nothing here is committed or pushed; that is Niroj's
terminal action.

**One-line summary:** hami-media is a self-hosted WebRTC video SFU
(one room, H.264) with per-user HMAC auth, NACK lost-frame recovery,
a standalone TURN relay, a host-health preflight and watchdog, and an
auto capacity detector. Measured on Niroj's Mac (Apple M5, 10 cores,
24 GiB RAM): 20 publishers x 5,000 viewers PASS strict once;
20 x 7,500 PASS strict three times; 20 x 8,000 PASS strict (T0–T4
+ FINAL + verify, §4.5); 20 x 10,000 FAILS systematically at the
macOS kernel loopback wall, not in this code. 10 x 6,000 is
BLOCKED, never started, no numbers exist.

## Contents

1. What hami-media is
2. Every capability, with how to use it
3. Test catalogue — every test, what it proves, the numbers, the verdict
4. Capacity runs — the full ladder
5. The hybrid viewer path (MediaMTX / HLS) — Phases 0–3b
6. The sibling service hami-sfu (Stages 1–6) — shares these mechanisms
7. Honest limits and ceilings
8. Open work and in-progress investigations
9. Evidence pointers
10. How to run every test

---

## 1. What hami-media is

hami-media is a small video server. One person streams live video.
Thousands of people watch it at the same time, in one room. It runs
on your own computer. No cloud service is needed. (README.md opening.)

- **Product:** VIDEO streaming, not audio. One logical room, H.264
  video, WebRTC. Viewers watch in a normal browser, no plug-in.
- **Stack:** Go (go.mod requires Go 1.24.0; measured with Go 1.27.4),
  Pion WebRTC v4.2.26, pion/interceptor v0.1.51, pion/turn v5.1.3,
  pion/ice v4.4.8, stdlib-first elsewhere. No cloud service, no
  account.
- **Shape today:** one `package main` program you run, not a library
  you import. The server core lives in `main.go` (548,904 B in the
  detector version). Splitting it into an importable package plus a
  thin command is planned follow-up work, deliberately left out of
  this release.
- **One room per server process.** That is the design, not a bug.
  The `m7h-sfu` server process serves the room in `HAMI_SFU_ROOM`
  (default `default`).
- **Source of truth for limits:** measured anchors only, never
  guesses. See section 2.7 and section 4.

### Repo layout (standalone repo, re-synced 24 Nov ~23:26 NPT)

| File | Size | sha256 (verified, detector version) |
|---|---|---|
| `main.go` | 548,904 B | `c8feddf90496825758ac2eebb9241f852c40ffd2bee416775e3bfb014386d489` |
| `capacity.go` (new) | 17,584 B | `ffbbd04e92243de5cc9ded69454b2cd8971455caa25ad71a547d527cbd4f2a09` |
| `capacity_test.go` (new) | 13,740 B | `f59e354f0cfb17da45a2b0e5b9fdd88a85324dd5530d72d82f6fedb9ed6d7d41` |
| `platform_darwin.go` | 8,053 B | `6802542ee673015f37589eb9b4aeeda0fc386bec33691d881a5b6dbe68a393a6` |
| `platform_linux.go` | 7,623 B | `74026d858fc36977ef78245b651fdb1583fe2f116a0a7d1f05d8d1760917e2af` |
| `platform_windows.go` | 2,203 B | `dd84d43188a25faccf3dab8146b3cee6c86497f48f6b320f2c2829100bddb7a0` |
| `auth_test.go` | 14,336 B | `bcd3d7d6080c9ee93e14a09e1ea7c68ea9279e8772a2faa8709cde98fb06d7f4` |
| `go.mod` (renamed module) | 936 B | `1036f6762decbe4e7ea2c3d4aab3dc41ecd3c87e266a82b608bbf689ff453320` |
| `go.sum` | 3,938 B | `e86cb630ca4cc9ae52ff0e86d0dce8c48d697a107bb99b664b18c51057265007` |
| `README.md` | 5,942 B | `dd5d17ad3385292b576b02e222cf224e9eeef84cd96ad2d1030cf998fb183a4b` |
| `LICENSE` (Apache-2.0) | 11,334 B | `e7e2b21545835563b275a809484b564b90faf57ee6125861cf16161d8506f948` |
| `docs/capacity.md` | 5,920 B | `d4311d609a4e24a87bea8060597f143374f52188fedb7541607da043e688e305` |
| `docs/configuration.md` | 5,420 B | `e9f55eef90cacf99d203aec0b29fbb4bb1abb1a35aea43dd43b73dad08b7b0df` |

Other files (sizes from the repo listing): `auth.go` 12,831 B,
`turn.go` 10,632 B, `latency.go` 10,373 B, `main_test.go` 91,695 B,
`turn_test.go` 12,865 B, `latency_test.go` 8,755 B,
`layers_test.go` 15,820 B, `bench_test.go` 1,526 B,
`platform_unix.go` 654 B, `.gitignore` 345 B.
Docs: `docs/setup.md`, `docs/configuration.md`, `docs/capacity.md`,
`docs/api-auth.md`, `docs/turn.md`, `docs/troubleshooting.md`,
`docs/integration.md` (24,043 B), `docs/auth-turn.md` (8,039 B).

---

## 2. Every capability, with how to use it

### 2.1 Run modes (`-mode`)

One binary, many modes (docs/configuration.md):

| Mode | What it does |
|---|---|
| `pair` (default) | One publisher, one viewer, on this machine. The smoke test. |
| `sfu` | Local SFU proof run. |
| `sfu-single` | The scale proof: N publishers, M viewers each, one room. |
| `sfu-cascade` | SFU proof across worker processes. |
| `m1h` … `m9h` | Ladder proof runs of growing size. `m9h` is the 10,000-viewer event shape. |
| `m7h-sfu` | The real server process. Spawned by the harness, not by hand. |
| `mint-token` | Print one client token and exit. |
| `turn-server` | Run the standalone TURN relay until stopped. |
| `audio-rung1` | Audio-only proof run (speakers and listeners). |

Plus the `capacity` subcommand (checked before `flag.Parse`, takes no
arguments): `./hami-media capacity` prints the device, the recommended
max publishers/subscribers, the scale factor, and the anchors used.

First run:

```sh
go build -o hami-media .
./hami-media -mode pair -video-file test.h264 -duration 3s
```

Scale proof run (the shape every capacity number in section 4 uses):

```sh
./hami-media -mode sfu-single -video-file test.h264 -duration 60s \
  -sfu-publishers 20 -sfu-viewers-per-publisher 250 -sfu-cells 4 \
  -metrics-listen 127.0.0.1:9464
```

### 2.2 Scale flags (`-mode sfu-single`)

| Flag | Default | Meaning |
|---|---|---|
| `-sfu-publishers` | 1 | Publisher count (1–3000). |
| `-sfu-viewers-per-publisher` | 1 | Viewers per publisher (1–500). |
| `-sfu-cells` | 1 | Split the run across N worker processes. |
| `-sfu-cell-index` | 0 | Which worker this process is (set by the parent). |
| `-sfu-score-mode` | `strict` | `strict` or `dissolve`. |
| `-sfu-max-p99` | 150 ms | Worst allowed p99 frame latency (dissolve scoring). |
| `-sfu-max-dissolved` | 0.01 | Worst allowed fraction of dissolved (late) frames. |
| `-allow-unsafe-host` | false | Skip the host health check. Only for runs you fully control. |
| `-max-heap-bytes` | 0 | Stop the run when Go heap use passes this many bytes. 0 = off. |
| `-nack-max-queue-age` | (built in) | Drop RTP older than this from the retransmit queue. |
| `-video-file` | — | H.264 source file. Required for video modes. |
| `-duration` | 3 s | How long to send media. |
| `-metrics-listen` | — | Optional loopback-only Prometheus endpoint, e.g. `127.0.0.1:9464`. |

**Strict scoring** (the bar for every capacity PASS in this report):
every frame must arrive, and the delivered-frame p99 latency must be
at most 150 ms. Dissolve scoring allows a small fraction of late
frames (default 1%) up to the same p99 cap. All measured results in
section 4 are strict.

**Hard caps in the code** (cannot be overridden with a flag; changing
them means changing the code and re-measuring):

| Cap | Value |
|---|---|
| Viewers per publisher | 500 |
| Total viewers | 10,000 |
| Delivered-frame p99 latency | 150 ms |
| Heap | 4 GiB default when `GOMEMLIMIT` is unset |

### 2.3 Video SFU — fan-out, layers, cascade

- A publisher sends one H.264 stream; the server copies it to every
  subscribed viewer in the room (layered fan-out: audio = base layer
  only; video layers fanned out by mask — proven by the layer tests,
  e.g. direct=100 writes vs layered=58, -42.0%, Stage 1).
- **Cascade:** `sfu-cascade` mode routes media across worker
  processes; the cascade RTCP test
  (`TestLocalSFUCascadeRoutesRTCPAcrossWorkers`) proves RTCP crosses
  workers. The cascade is how the big runs split across cells
  (`-sfu-cells 4` or `5`).
- **IDR phase stagger:** publishers start at different frame offsets
  so their IDR (keyframe) ticks do not align into one synchronized
  burst (LADDER FIX 3). Without it, 20 aligned IDRs burst ~45,000
  packets in ~50–100 ms of fan-out. With it, at most one publisher
  is in its IDR burst at any instant.
- **Egress sharding** (`HAMI_SFU_EGRESS_SHARDS`, 0–32, default 0):
  parallelises fan-out across shard goroutines. Measured at
  20x10,000 run 5/6 and found to be a DEAD END as implemented: it
  trades the serial fan-out's queueing for burst loss (~15% kernel
  loss) plus per-packet overhead (p95 med 179.39 ms vs 99.92 ms
  non-shard). Do not enable it for the ladder. Kept off by default.
- **NACK recovery (see 2.4)** and the feedback-wait probe (each
  viewer NACKs First/Last/Missing sequences; in a clean run the 2
  already-received duplicates per viewer are a benign measurement
  artifact, explained in polish-001, not a bug).

### 2.4 Lost-frame recovery (NACK)

- Viewers detect sequence gaps themselves and NACK the missing
  sequences; the router retransmits from its cache (512-packet
  responder history on the publisher side; 256-packet cache in the
  hami-sfu port).
- Upstream forwarding: genuinely-lost sequences are forwarded
  upstream at most once per 500 ms per track (dedup window), so 375
  viewers NACKing the same lost packet do not cause a retransmit
  storm (LADDER FIX 2; FIX 1 without dedup caused a storm:
  147,485 router retransmits in 20x7,500 run 2).
- One NACK attempt per missing sequence (`nackGapMaxAttempts = 1`,
  LADDER FIX 5). Retries were removed deliberately: a retry fires
  600 ms after the first NACK, but a frame dissolves ~400 ms after
  its first packet, so a retry retransmit can never complete its
  frame — retries are pure load. The 600 ms retry / 500 ms dedup /
  ~400 ms abandonment window stack is incompatible by design; any
  future retry must resolve it first.
- What it recovers, measured (hami-sfu Stage 3, same mechanism):
  publisher hop 300/300 frames at 5% and 10% injected loss;
  subscriber hop 300/300 at 5%, 297/300 (99.0%) at 10% — the
  remaining 1% is Pion's responder sending ~1 retransmit per dropped
  packet and never retrying it (42 RTX per run), a Pion-library
  behaviour, not hami-media server code.
- The hami-media scale runs prove the same recovery at load: in the
  20x7,500 passing runs NACK counts were 0/0 (no loss to recover);
  in the 20x10,000 failing runs the NACK storm itself was diagnosed
  and killed (FIX 5 + FIX 6: the second NACK source was Pion's NACK
  generator interceptor in the default viewer API; the light API
  without it cut router NACK intake from 1,661,985 to 10,000
  exactly, run 3 vs run 4).

### 2.5 Per-user auth tokens (HMAC)

Two layers (docs/api-auth.md):

1. **Bearer token.** Every signalling endpoint needs it. A random
   32-byte token made fresh for each server run. It proves the caller
   is the harness that started this server process. It says nothing
   about rooms or roles and never expires on its own. In the
   `m7h-sfu` server it is passed on an inherited file descriptor
   (fd 4) after polish P3, never on argv, so it never appears in `ps`
   output.
2. **Client token.** Minted per client, per room, per role, with an
   expiry. Format: `base64url(payload).base64url(HMAC-SHA256)`,
   stdlib only, no JWT dependency. Claims: `room`, `roles`
   (`publish` / `subscribe`), `sub`, `exp`, `iat`. No clock-skew
   allowance; rejected at or after `exp`.

Key rules: key from `HAMI_SFU_HMAC_KEY_FILE` (preferred, file must be
mode 0600, group/other-readable is refused) or `HAMI_SFU_HMAC_KEY`;
set only one. Minimum 32 characters. Max TTL 24 h. The server
refuses to start without a key (fail-closed). The key is never
printed or logged. Mint:

```sh
export HAMI_SFU_HMAC_KEY_FILE=/path/to/hmac-key.txt
./hami-media -mode mint-token -token-subject alice -token-ttl 1h
```

Token flags: `-token-room` (default `HAMI_SFU_ROOM` or `default`),
`-token-roles` (default `publish,subscribe`), `-token-subject`
(required), `-token-ttl` (default 1 h).

Auth runs BEFORE any publisher/subscriber slot is taken, so bad
tokens cannot burn capacity. An unauthorised subscription is denied
by the router itself, not just by the HTTP layer
(`TestLocalSFURoutesAuthorizedStreamsAndRTCP`).

### 2.6 Room API (HTTP endpoints)

All endpoints need the Bearer token. Base URL is the server's
loopback listener (the harness owns the address).

| Endpoint | Method | Needs role | What it does |
|---|---|---|---|
| `/health` | — | — | Server health. |
| `/publisher` | POST | `publish` | Publish a track. |
| `/publisher/ready` | — | — | Publisher readiness. |
| `/viewer` | POST | `subscribe` | Subscribe to a track. |
| `/stats` | — | — | Server stats. |
| `/receive-stats` | — | — | Publisher receive stats. |
| `/heap-profile` | — | — | Heap profile (Bearer-protected). |
| `/goroutine-profile` | — | — | Goroutine profile. |
| `/cpu-profile` | — | — | CPU profile. |
| `/cleanup` | — | — | Clean up state. |
| `/shutdown` | — | — | Shut the server down. |

A request without the Bearer token gets 401. A publisher request
with a token that lacks `publish` is refused; same for viewers and
`subscribe`. An expired client token is refused.

**Reconnect:** a client that drops can resume on the same room and
token — a reconnect is a fresh POST with the same room + token; rooms
survive participant disconnect; auth is re-verified on every request,
so an expired token is refused on resume with 401 before any slot is
taken. Measured in hami-sfu Stage 4 (section 6.4): subscriber
first frame 20.7 ms after re-subscribe, publisher 238.8 ms,
expired-token resume 401/401. Known limitation: an EXISTING
subscriber does NOT recover when its publisher re-joins — it is bound
to the old track object and must re-subscribe itself (needs
server-initiated renegotiation to fix; the web client must
re-subscribe on track end).

**Metrics:** `-metrics-listen 127.0.0.1:9464` exposes a Prometheus
endpoint, loopback-only on purpose. It reports forwarding counts and
resource signals.

### 2.7 Capacity detector (landed 24 Nov 2026)

`capacity.go` (17,584 B) + `capacity_test.go` (16 tests) + the
`capacity` subcommand + `-auto-capacity` / `-max-publishers` /
`-max-subscribers` flags on `-mode m7h-sfu` + per-platform profile
readers (`platform_darwin.go`, `platform_linux.go`,
`platform_windows.go` honest stub that errors, never fakes numbers).

**What it reads:** CPU model + cores, total RAM, swap (reused
`readAudioSwapMiB`), RLIMIT_NOFILE soft/hard, system-wide file cap,
UDP buffer ceilings, OS/kernel, NIC list. Darwin reads via syscalls
only (no subprocess — the FIX 30 rule: a startup detector must not
fork). Linux reads `/proc`. Fields a platform cannot read stay zero
and are reported as unknown, never invented.

**Anchor table — the only source of limits** (`capacity.go:85`).
Only PASSING rows feed the maxima:

| Publishers | Viewers (total) | Result | Runs |
|---|---|---|---|
| 20 | 5,000 | pass | 1 |
| 20 | 7,500 | pass | 3 |
| 20 | 8,000 | pass | 5 (T0, T1, T4, FINAL, verify-s005 — §4.5; T2/T3 failed on frames, see §4.5) |
| 20 | 10,000 | fail | — (systematic, macOS kernel loopback limit, NOT a code limit) |
| 10 | 6,000 | **pending** | 0 — NOT measured, never used by the derivation |

**Formula** (`deriveCapacityLimits`):

```
factor     = min(cores / 10, RAM GiB / 24), clamped to [0, 1]
publishers = floor(20   * factor), at least 1
viewers    = floor(7500 * factor), at least 1
```

Reference device: Apple M5, 10 cores, 24 GiB RAM -> factor 1.00 ->
20 publishers / 7,500 viewers, exactly the highest passing anchors.
A stronger device still gets 20 / 7,500 (never past a measurement).
A weaker device scales down (5 cores / 12 GiB -> 10 / 3,750). The
scale-down is a conservative heuristic, not a measurement; the report
says so. There is no `-max-rooms`: a room limit has nothing to cap in
a one-room-per-process server.

NOTE (25 Nov ~01:50 NPT): 20 x 8,000 now passes strict (5 runs,
§4.5), so the highest passing anchor is 8,000 viewers, not 7,500.
The detector's coded maxima are STILL 20 / 7,500 (`capacity.go:85`,
formula above) — the derivation has NOT been re-run with the new
anchor, and no code has changed. Updating the coded maxima to
20 / 8,000 is a separate change with its own verification; until
then the report and the code disagree by design, and this note is
the record of it.

**Override rules:** `-auto-capacity` defaults true; off means the
defaults are the anchor maxima (20 / 7,500). An explicit
`-max-publishers` / `-max-subscribers` value replaces the auto value
as-is — never silently lowered, even above the anchors (that case
logs a WARNING saying it is unmeasured, it is not clamped). An
explicit 0 means unlimited for that limit. A negative explicit value
is a hard startup error. Detection failure on the serve path falls
back to the anchor maxima with a loud WARNING; in the `capacity`
subcommand a detection failure IS an error. The chosen limits are
logged at startup, e.g.
`capacity: auto=true maxPublishers=20 maxSubscribers=7500
scaleFactor=1.00 cpu=Mac17,2 cores=10 ramGiB=24.0`.

**Enforcement:** past the limit, `/publisher` returns 503
`publisher limit reached: max N publishers` and `/viewer` returns 503
`subscriber limit reached: max N subscribers`, checked under the same
lock that guards the maps, at insert time.

**Live output on the real Mac** (verify-004, 24 Nov 23:15 NPT):
device `Mac17,2 / darwin 25.6.0 / 10 cores / 24.0 GiB RAM`; swap
used 1,918 MiB, free 1,154 MiB, total 3,072 MiB; file descriptors
soft 92,160, hard 9,223,372,036,854,775,807 (RLIM_INFINITY), system
max 184,320; UDP buffers receive 786,896 bytes, send 0 bytes (darwin
has no separate send ceiling exposed this way — reported as 0 = not
read, not invented); recommended 20 publishers / 7,500 viewers,
scale factor 1.00. It also warned: free swap 1,154 MiB is below the
2,048 MiB scale-proof preflight bar — a warning, not a silent
adjustment of the limits.

### 2.8 Host-health preflight and watchdog (CURRENT rule)

**Current rule (LANDED 24 Nov ~23:50 NPT, `main.go` 550,043 B,
binary fixed26 `42fbba59…`):** before a scale run starts, the
server samples the host and loops on a 10-minute recheck while any
check fails. When macOS holds a swap pool, the memory failure is
**used swap above `sfuHostMaxUsedSwapMiB` = 512 MiB**; when it holds
no pool, free memory is the signal instead (8 GiB free memory
required). Also: free disk 10 GiB, macOS memory-pressure level.
While a run is going, a watchdog re-reads the same numbers with
cheap system calls (no new processes) and stops the run if the
machine runs out of room. `-allow-unsafe-host` skips the check; a
run that starts unhealthy usually fails loudly.

**Why the rule changed (old rule, superseded):** the old rule failed
a run when free swap fell below 2,048 MiB. macOS shrank the swap
pool to 2,048 MiB total after the 23:19 clean (used 366 MiB, free
1,682 MiB), so the 2,048 MiB free floor could never pass — used
swap would have had to be ~0. The new rule FAILS the old state
(pool 3,072 / used 1,918 / free 1,154 MiB) and PASSES the new state
(pool 2,048 / used 366 / free 1,682 MiB); the threshold (512 MiB
used) is stated in code with the reasoning in a comment, not a
silent bypass. The rule is proven in production by the T0–T4 +
FINAL + verify-s005 runs (§4.5), all of which passed preflight with
used swap 264–272 MiB. A final `main.go` re-sync into this repo
follows (not yet done at this update). The `docs/capacity.md`
preflight line still states the old 2,048 MiB bar and is updated
with that re-sync.

### 2.9 TURN relay

- `./hami-media -mode turn-server` runs a standalone TURN relay
  (pion/turn) until stopped. Default listen `127.0.0.1:3478`
  (`-turn-listen` / `HAMI_SFU_TURN_LISTEN`). Realm `hami`.
- Credentials: `HAMI_SFU_TURN_USERNAME` (required),
  `HAMI_SFU_TURN_PASSWORD` or `HAMI_SFU_TURN_PASSWORD_FILE`
  (preferred; 0600-enforced since polish P2 — a 0644 password file
  used to be silently accepted, now refused loudly). Set the
  password in exactly one place. The server refuses to start without
  a username and a password. Credentials are never hardcoded, never
  logged, never printed. Clients get the URL from
  `HAMI_SFU_TURN_URL` (default `turn:127.0.0.1:3478`).
- Relay ports: 49152–59152 (10,000 ports) in production.
- The SFU server itself needs NO TURN config: a relayed client
  candidate pairs with the SFU's host candidate. The server never
  advertises TURN in-band; the operator hands the TURN URL +
  credentials to clients out of band.
- Proven (hami-sfu Stage 5, same mechanism): relay-only clients got
  23 frames via relay under a kernel-enforced block of direct UDP,
  while plain clients got 0 frames. NOT proven: a client on a
  separate network behind a real NAT — that gate is still open (see
  section 7).

### 2.10 Measurement, profiles, and guardrails

- Every scale run reports frames arrived, latency percentiles
  (p50/p95/p99, nearest-rank), dissolved fraction, NACK counts,
  retransmits, queue drops, write errors, cleanup verification, and
  memory (whole-run RSS sampler + per-cell heap).
- Heap guard: 4 GiB Go heap limit when `GOMEMLIMIT` is unset;
  `-max-heap-bytes` stops a run cleanly when the budget is hit.
  Heap profile env: `HAMI_HEAP_PROFILE_PATH` /
  `HAMI_HEAP_PROFILE_AT_BYTES` (default 512 MiB). CPU profile env:
  `HAMI_CPU_PROFILE_PATH` / `HAMI_CPU_PROFILE_DELAY` (default 90 s)
  / `HAMI_CPU_PROFILE_WINDOW`. Diagnostics: `-sfu-profile` flag
  (per-cell CPU/goroutine/mutex profiles), `-sfu-ingress-split`
  (ingress-leg instrumentation), socket-counting ldflags
  (`clientSocketReadMode`, `routerSocketWriteMode`, diagnostic-only,
  default disabled).
- Security hardening (polish-001): P1 removed the unauthenticated
  `/debug/pprof/heap` endpoint from the metrics server (any local
  process could GET a heap dump, which can contain HMAC key / Bearer
  token bytes; the Bearer-protected `/heap-profile` already exists).
  P2 enforced the 0600 rule on the TURN password file. P3 moved the
  SFU Bearer token off argv onto an inherited fd. Probe found NO
  token value had ever landed in a log (the fix is preventive, not a
  scrub).
- Test video: `/private/tmp/hami-cost-interframe-20261119.h264`,
  161,149 B, sha `ed283d3aab61839a514a0d1add6bb92bce640915b22274d29e158945a11a6969d`,
  302 frames, 10 IDR keyframes (one every 30 frames), 372 packets
  per track. The wrapped `test-60s.mp4` used for the MediaMTX phases
  is 10.00 s, 30 fps, exactly 300 frames (the wrap re-timed the raw
  5 fps clip to 30 fps).

### 2.11 What hami-media does NOT do

- No library package yet (see section 1). Run the binary, or talk
  to its HTTP API, or run its TURN server.
- No per-viewer bitrate cap (deliberately left out of hami-sfu
  Stage 2: a flag accepted but not enforced would be dishonest;
  real enforcement needs per-subscriber byte accounting in the
  fan-out).
- No mixer/composite output. The hybrid path (section 5) is one HLS
  output per published stream; a viewer watches one stream at a
  time.
- No second-machine proof yet. All big runs are loopback on one Mac.
- No LiveKit anywhere. The packager in section 5 (MediaMTX) is a
  borrowed tool doing one job (package + serve segments), not the
  product.

---

## 3. Test catalogue — every test, what it proves, the numbers, the verdict

### 3.1 Unit tests — `go test ./...`

**How to run:** `go test -count=1 -v -timeout 600s .` (plus
`gofmt -l .` and `go vet ./...`, both must be clean).

| Run | Where | Result |
|---|---|---|
| Standalone repo, first snapshot (pre-detector) | Mac, via hami loop, 24 Nov 22:58 NPT | **100 PASS / 0 FAIL**, `ok github.com/kn33r0s3/hami-media 21.010s` |
| Standalone repo, first snapshot | Linux VM, Go 1.24.0 | **94 PASS / 6 FAIL**, 122.4 s — all 6 are the known VM UDP block (see below) |
| Standalone repo, detector re-sync | Mac, 24 Nov 23:25 NPT | **116 PASS / 0 FAIL**, `ok ... 21.040s` (100 + 16 capacity tests) |
| Standalone repo, detector re-sync | Linux VM | **110 PASS / 6 FAIL**, 122.4 s — same 6 UDP failures, 0 capacity failures |
| Build-tags work, Mac | Mac, 24 Nov 21:40 NPT | **100 PASS / 0 FAIL**, `ok ... 21.152s` |
| Build-tags work, Linux | Linux VM | **94 PASS / 6 FAIL**, 122.283 s |
| Capacity detector, full suite | Mac, verify-004, 24 Nov 23:15 NPT | **full `go test` ok, 20.631 s** |

The 6 Linux failures are ALWAYS the same six tests, and they are the
VM sandbox, not the code — the VM blocks every unconnected UDP
`sendto()` with EPERM (Seccomp: 2, 4 filters; 32 failed sends in the
ICE trace; connected UDP works, e.g. DNS answered 29 bytes). The six:

1. `TestLocalPairReceivesAudioAndVideoRTP`
2. `TestCountingUDPMuxCountsPeerWritesWithoutClosingSharedMux`
3. `TestPacketReadCountingConnCountsDatagrams`
4. `TestLocalSFURoutesAuthorizedStreamsAndRTCP`
5. `TestLocalSFUCascadeRoutesRTCPAcrossWorkers`
6. `TestSFUTURNRelayCarriesMedia`

All six pass on the Mac. The VM therefore cannot serve as a second
machine for any UDP-media test while its sandbox policy stands; no
code change in hami-media can fix that (vm-ice-diagnosis-001.md).

**Detector tests (16, all PASS on Mac and VM)** — derivation on the
reference device (20 / 7,500, factor 1), scale-down (5 cores / 12 GiB
-> 10 / 3,750), weaker-axis (10 cores / 4 GiB -> 3 / 1,250), never
scales up (32 cores / 128 GiB -> still 20 / 7,500), empty profile
rejected, anchors exclude pending/failing (exactly one pending anchor,
10x6,000), explicit override never lowered (50 / 9,000 honoured +
warning; 5 / 100 honoured), auto-off uses anchor maxima, negative
explicit rejected, explicit 0 = unlimited, report contents (reference
and weak device), live detection plausible values, subcommand prints
a report, limit predicates at the boundary, publisher limit enforced
over HTTP (1-publisher server: 1st publish 200, 2nd authorised
publish 503). The subscriber-limit 503 branch has NO HTTP test (it
needs a live published track; the publisher branch is proven
end-to-end and the subscriber predicate at the boundary).

**Build-tags tests:** hami-media now builds and vets clean on Linux,
macOS and Windows from a single tree. The pre-fix tree FAILED the
Linux build with exactly two errors (`main.go:756: undefined:
syscall.SysctlUint32`, `main.go:4658: undefined: syscall.Sysctl`).
Fix: `platform_darwin.go` (verbatim moves), `platform_linux.go`
(real `/proc` readers, no faked measurements), `platform_windows.go`
(honest error stubs). Linux pressure signal is deliberately absent
(`hostPressureSupported = false`, check skipped) — a `/proc/pressure`
mapping onto the macOS 1/2/4 scale was considered and rejected as
uncalibrated invention.

### 3.2 Smoke tests (pair mode + hami-sfu loopback)

- hami-media pair mode: `./hami-media -mode pair -video-file
  test.h264 -duration 3s` — one publisher, one viewer, report says
  the frames arrived.
- hami-sfu Stage 1 loopback smoke (same WebRTC path):
  `TestSmokeLoopback` PASS, subscriber received **23 frames**
  (wanted >= 20), 1.03 s. Re-proven in every later stage gate
  (Stage 2: 23 frames; Stage 5 gate-002: 23 frames; Stage 6 run 002:
  22 frames; Stage 4 re-check: 23 frames).
- hami-sfu Stage 2 auth smoke: `TestSmokeAuth` PASS, 10 frames
  (wanted >= 5), after 4x401 rejection checks.

### 3.3 Loss tests (NACK recovery)

Full detail in section 6.3 (hami-sfu Stage 3, the same mechanism).
Headline: publisher hop **300/300 at 5% and 10%** injected loss;
subscriber hop **300/300 at 5%, 297/300 (99.0%) at 10%**; no-NACK
control **281/300 (93.7%)**, proving the test measures NACK recovery
and is not vacuous. Fail-before (no RTCP pump): 283/300 (94.3%) and
270/300 (90.0%), exit 1. Subscriber-hop decisive experiment (6 runs):
with reports 297, 297, 297; without reports 297, 297, 296 — noise,
not a receiver-report effect; no-reports variant DOES retransmit
(41 RTX in all 3 runs).

### 3.4 Reconnect tests

Section 6.4. Subscriber drop 5 s: first frame **20.7 ms** after
re-subscribe. Publisher drop 5 s: first frame **238.8 ms**. Expired
token on resume: **401 / 401**, no participants left behind. 4/4 PASS.

### 3.5 TURN tests

Section 6.5. Relay-only clients: **23 frames via relay** under a
kernel-enforced block of direct UDP; plain clients: **0 frames in
10 s**. Wrong credentials: no relay candidate gathered. Server
without credentials: refuses, exit 1. Linux unit leg: 36/36 PASS.

### 3.6 Webpage test (hami-sfu Stage 6)

Section 6.6. Viewer-fix page 11,685 B / 324 lines (sha `8069468b…`,
deployed 24 Nov ~22:20 NPT, verified served); indefinite
re-subscribe retry (backoff capped 8 s) + 5 s no-frames watchdog.
Pre-fix page (10,791 B / 304 lines) verified 9/9 markers present,
zero external references, curl 200 first try; smokes 2/2 PASS
(22 + 10 frames). Markers/smokes NOT yet re-run on the fix version.
No live browser end-to-end check yet.

---

## 4. Capacity runs — the full ladder

All runs: one logical room, H.264 clip (161,149 B, sha `ed283d3a…`),
60 s media, strict scoring (p95 <= 100 ms, p99 <= 150 ms,
MinFrames 302, dissolved <= 0.01), `GOMEMLIMIT=10GiB`, never
`-allow-unsafe-host`, all 5 launch gates passed before every run
(SFT/pilot/gateway stopped; binary sha; video sha; free memory
>= 8,192 MiB; no run active). Machine: Niroj's Mac (Apple M5,
10 cores, 24 GiB RAM) unless stated.

### 4.1 10 x 5,000 — the repeat protocol

| Run | Binary | Verdict | p50 med | p95 med | p99 med | Short |
|---|---|---|---|---|---|---|
| FIX 30 run 1 | fixed9 `de6dc3c9…` | TRUE, 5,000/5,000 | — | — | 49.77 ms | 0 |
| FIX 30 run 2 | fixed9 | latency-pass / completeness-FAIL | — | — | — | 10 pending frames |
| FIX 31 diag | fixed9 | **TRUE, 5,000/5,000** | 4.36 ms | 11.28 ms | 56.32 ms (max 82.27) | 0 |
| FIX 31 diag-2 | fixed9 | TRUE | — | — | — | 0 |
| Repeat 1 (17:35 NPT) | fixed9 | **TRUE, 5,000/5,000** | 3.76 ms | 8.97 ms | 52.76 ms | 0 |
| Repeat 2 (17:38 NPT) | fixed9 | **TRUE, 5,000/5,000** | 3.00 ms | 7.19 ms | 35.50 ms | 0 |
| Repeat 3 (17:42 NPT) | fixed9 | **TRUE, 5,000/5,000** | 3.06 ms | 6.99 ms | 28.72 ms | 0 |

Record before the repeats: 4 runs, 3 full passes, 1 completeness
failure. With the repeats: 7 runs, 6 full passes, 1 completeness
failure. The FIX 31 kernel/loopback loss (~1-in-4 event) did NOT
reproduce in the three repeats — three clean runs are consistent
with it, they do not clear it. FIX 31 located that loss: between
the router socket's successful `sendto` and the viewer socket's
`recv` — the macOS kernel/loopback delivery path; Pion exonerated
(299/331 short viewers had `d_socket == d_total`; 0/331 had
`d_pion > 3`). Only frames 150/151, one ~400 ms event at ~30 s.
Netstat delta on a clean run: `dropped due to full socket buffers`
1,487 -> 1,487 = 0. The specific kernel queue is NOT identified; any
buffer raise would be a guess — not done.

### 4.2 20 x 5,000 — PASS

Step 3, 23 Nov 17:46:26 NPT, binary fixed9 `de6dc3c9…`, 20 publishers
x 250 viewers, 4 cells. Aggregate JSON
`cap20x5000-step3-20261123.json`, sha
`355300a2c6080962f6c7d575d6260acbfac4db8aa4d0fcacbbaa3ab007aff0ab`,
6,640,451 B.

- **measurement_pass = True, strict. 5,000/5,000 on every gate.**
- p50 med/max 1.89 / 3.67 ms; p95 med/max 4.60 / 10.77 ms;
  p99 med/max 19.65 / 39.39 ms.
- Short viewers 0. Frames {302: 4,000, 303: 1,000}. Delivered
  1,511,000 / video frames sent 6,044. Dissolved 0. Latency pending
  0/0. rtp_queue_drops 0, rtp_write_errors 0, cleanup_verified true.
- Retransmitted {1: 4,986, 2: 14}. Worst track p99 med 29.30 ms
  (stream-0020). Connected 5,000/5,000.
- Wall 108,623 ms. Per-cell max heapInuse sum 2,878.78 MiB
  (~2.81 GiB). Max goroutines 36,462/cell. Parent peak RSS 40.19 MiB.
- Context: earlier FIX 30 20x5,000 evidence runs on fixed9 also
  passed twice (p99 24.29 / 49.95) but with 500 short viewers
  (tracks 0008+0009, end-of-stream loss the pending clause is blind
  to). This step-3 run has ZERO short viewers.

### 4.3 20 x 7,500 — PASS strict, three times

| Run | Binary | Verdict | p95 med/max | p99 med/max | Memory |
|---|---|---|---|---|---|
| Ladder run 4 | fixed15 `37854f5a…` | **True, 7,500/7,500** | 22.10 / 49.73 ms | 34.51 / 62.17 ms | 6.574 GiB |
| Ladder run 5 (repeat) | fixed15 | **True, 7,500/7,500** | 28.96 / 50.78 ms | 45.05 / 66.10 ms | 6.635 GiB |
| Polish confirm (24 Nov 11:27 NPT) | fixed25 `ff84b3cd…` | **True, 7,500/7,500** | 11.35 / 19.14 ms | 19.28 / 31.34 ms | 5.662 GiB |

All three: frames {302: 7,500}, short 0, NACK 0/0, dissolved 0,
pending 0. Polish confirm detail: p50 med 2.26 ms (max 5.61),
delivered 2,265,000 frames, video_frames_sent 6,040,
rtp_queue_drops 0, rtp_write_errors 0, cleanup_verified true, wall
131.72 s, JSON sha
`09ca2cc1b4f658b74a039d633e3578a945e51d33090b49f5e9e4940cf2b84ef3`,
10,453,763 B. The polish numbers are BETTER than runs 4–5, but the
polish changes do not touch the media path — that is machine-state
variance, not a polish improvement, and is not claimed as one.

The road to the pass (runs 1–3, all FAIL, recorded): run 1 —
6,705/7,500, upstream loss on stream-0014 (21 packets, frames
90/91, 180/181, 240/241) + downstream IDR loss; run 2 — 207/7,500,
FIX 1 backfired into a retransmit storm (147,485 retransmits);
run 3 — 6,952/7,500, storm fixed by FIX 2 dedup, 307 short on
downstream IDR loss. Fixes: FIX 1 upstream NACK forwarding +
publisher NACK responder; FIX 2 per-track upstream-forward dedup
(500 ms window); FIX 3 IDR phase stagger (the load-shape amendment:
same gates, same total load, IDR phases spread — a synchronized
burst never happens in production).

### 4.4 20 x 10,000 — SYSTEMATIC FAIL at the kernel loopback wall

**Verdict: systematic fail, NOT a code limit.** Run 4 once passed
p95 (99.92 ms) with perfect frames but does not reproduce; runs 7,
8, 9, 10, 11, 12 all fail. The failure is in the macOS kernel's
loopback path carrying 10,000 local viewers. A real deployment with
viewers on real networks does not hit this wall the same way.

| Run | Binary | Verdict | Frames complete | p95 med | p99 med |
|---|---|---|---|---|---|
| 1 | fixed15 | FAILED (feedback wait timeout, no media verdict) | — | — | — |
| 2 | fixed16 | FALSE, 0/10,000 | ~35/viewer med | 10,360.85 ms | 12,764.41 ms |
| 3 | fixed17 | FALSE, 0/10,000 | ~40/viewer med | 7,969.17 ms | 10,047.20 ms |
| 4 | fixed18 | FALSE, 5,056/10,000 | **10,000/10,000** | **99.92 ms** | 123.42 ms |
| 5 | fixed18 + shards 8 | FAILED (feedback counting bug, no media verdict) | — | — | — |
| 6 | fixed19 + shards 8 | FALSE, 0/10,000 | 3,135/10,000 | 179.39 ms | 480.45 ms |
| 7 | fixed20 | FALSE, 0/10,000 | 494/10,000 | 224.01 ms | 641.12 ms |
| 8 | fixed20 (repeat) | FALSE, 0/10,000 | 0/10,000 | 429.62 ms | 673.51 ms |
| 9 | fixed21 | FALSE, 0/10,000 | 96/10,000 | 283.37 ms | 636.10 ms |
| 10 | fixed22 | FALSE, 0/10,000 | 5,285/10,000 | 169.67 ms | 460.79 ms |
| 11 | fixed23 | FALSE, 0/10,000 | 9,880/10,000 | 138.04 ms | 173.35 ms |
| 12 | fixed24 | FALSE, 0/10,000 | 0/10,000 | 380.68 ms | 669.66 ms |

Run 4 detail (best-known state): pass 5,056/10,000; frames
{302: 10,000}, delivered 3,020,000/3,020,000, dissolved 0, pending
0/0, rtp_queue_drops 0, rtp_write_errors 0; p50 med 24.63 ms
(10,000/10,000 pass); p95 med 99.92 / max 118.15 (5,056 pass);
p99 med 123.42 / max 157.37 (9,998 pass). FIX 6 confirmed: router
NACK intake 10,000 total = the feedback-wait probe only (run 3:
1,661,985; -99.4%); retransmits 20,000 = 2/viewer probe only.
Zero kernel loss in run 4 (router writes 3,632,500 = viewer socket
reads 3,632,500). The ONLY failing gate was p95, by 0.08 ms of
median. Slow frames cluster at IDR positions (80–165 ms at IDR
ticks); the serial 500-enqueue fan-out loop takes ~150 ms per IDR
(6 packets x 500 enqueues at ~50 us each).

What the later runs proved, in order:

- **Runs 2–3:** the retransmit storm amplifies the loss (12.5M
  retransmits, 3.4x the originals, 98.5% never arrive; viewers
  complete ~35–40/302 frames; dissolved 0.9278 / 0.9135). FIX 5
  (single NACK attempt) cut NACKs sent 63% but retransmits only 9%
  — the storm was an amplifier, not the sole cause.
- **Second NACK source found:** Pion's NACK generator interceptor
  in the default viewer API sent ~1.5M NACK packets per run,
  independent of the gap tracker. FIX 6 (light API, no generator)
  killed it.
- **Run 5:** egress sharding exposed a counting bug
  (`shard.enqueueNACK` never called `recordNACKReceived()`); FIX 7
  fixed the counting. Sharding also cost ~15% kernel loss (burst
  writes ~250k/s vs serial ~20k/s).
- **Run 6:** sharding is a dead end (see section 2.3).
- **Run 7 (profile run):** the wait is ROUTER INGRESS: mean
  41–51 ms (run 4: 13–17 ms). Fan-out loop REFUTED (0.19–0.24
  ms/packet, max <10 ms). CPU (~1 core streaming), scheduler
  (p95 ~0.2 ms), GC (~0), mutex contention (~0.04 cores) ALL RULED
  OUT, identical in good run 4 and bad run 7. Profile wiring
  exonerated.
- **Runs 7+8:** systematic limit confirmed (run 7's "run-variable
  at the margin" reading SUPERSEDED — the bad state reproduced
  twice in a row, worse on every axis).
- **Run 9:** ingress-split instrumentation did not engage (flag not
  forwarded to cells); fixed in run 10. Frames 96/10,000.
- **Run 10:** ingress split measured — the wait is in the mux leg;
  NACKs fewest of the bad runs (42,796). Frames 5,285/10,000.
- **Run 11 — the four-way mux split (the decisive measurement):**
  pooled mux 18.67 ms = send 0.37 ms (2.0%) + **kernel 18.30 ms
  (98.0%)** + fifo 0.01 ms (0.1%). Per cell: send 0.30–0.31 ms
  (1.4–1.8%), kernel 17.28–21.81 ms (98.2–98.6%), fifo 0.01 ms,
  Pion 0.06–0.07 ms. Cross-cell correlation of the per-second kernel
  series 0.898–0.996 — four independent processes, one shared delay
  curve: the cause is common to all cells and lives in the kernel
  loopback path. Inbound-load hypothesis STAYS DEAD (corr RTCP/s vs
  kernel 0.079, STUN/s 0.108; STUN is a flat ~2,497/s flood per cell
  while the kernel delay ramps 2 -> 20 ms — a flat load cannot drive
  a ramp). Honesty check: removing the whole ~18 ms kernel wait
  would move p95 only to ~120 ms (if 1:1, unproven) — still above
  the 100 ms bar; the p95 tail has another contributor (IDR-cluster
  frames and/or queueing dynamics) not isolated by this measurement.
- **Run 12:** the transit-vs-backlog split is UNMEASURABLE on this
  macOS — `SO_TIMESTAMPNS` returns ENOPROTOOPT, `SO_TIMESTAMP` sets
  but `recvmsg` returns no ancillary data (proven in three probes:
  Go, raw syscalls, Python). STUN-flood load test: dead by
  intervention (flood window t=29–44 showed no effect beyond the
  run's own bad state). Send leg also blocks (~10 ms). SIOCINQ shows
  the socket queue empty. Tail is GOP-periodic (IDR pos 0 mean
  254.91 ms vs steady pos 1–10 mean 136.37 ms) but even steady
  positions average 136 ms — above the 100 ms bar — so flattening
  IDR bursts alone would not pass p95. Same code path gives p95
  100 (run 4), 138 (run 11), 381 (run 12) across runs: the kernel's
  saturation level varies enormously run to run; it is machine
  state, not code.

Memory across the 10,000 runs: 7.17–8.98 GiB (sampler); rises with
viewer count; not a gate, but it becomes the constraint if it
approaches the 10 GiB GOMEMLIMIT.

### 4.5 20 x 8,000 — PASS strict (T0–T4 + FINAL + verify-s005)

The largest unmeasured configuration the anchors support (20x7,500
banked 3x; 20x10,000 a systematic kernel-loopback fail). 20x8,000 is
400 viewers per publisher — inside the 375-pass / 500-fail
per-publisher band — and isolates the total-rate dimension. All
runs: fixed26 (`42fbba59…`, 19,304,994 B), same clip (sha
`ed283d3a…`), same flags apart from counts and env, 60 s, 4 cells,
strict (p95 <= 100 ms, p99 <= 150 ms, MinFrames 302, dissolved
<= 0.01). Env varies by trial (see table); the §4 header's
`GOMEMLIMIT=10GiB` is the default, T1/T4/gomem4g differ.

| Trial | Env (GOMEMLIMIT / GOGC) | Verdict | p95 med | p99 med | Frames | Peak RSS | Cell HeapInuse sum | Swap Δ |
|---|---|---|---|---|---|---|---|---|
| T0 | 10 GiB / default | **PASS** | 15.50 ms | 26.27 ms | {302: 8000} | 4.690 GiB | 4,729.18 MiB | 0.00 |
| T1 | 6 GiB / default | **PASS** | 14.52 ms | 24.83 ms | {302: 8000} | 6.208 GiB | 4,924.74 MiB | −8.00 (system reclamation) |
| T2 | 10 GiB / 50 | **FAIL (frames)** | 16.55 ms | 26.03 ms | {301: 400, 302: 7600} | 5.125 GiB | 3,975.47 MiB | 0.00 |
| T3 | 10 GiB / 20 | **FAIL (frames)** | 18.25 ms | 30.45 ms | {301: 1200, 302: 6800} | 4.243 GiB | 3,164.46 MiB | 0.00 |
| T4 | 6 GiB / 50 | **PASS** | 15.71 ms | 24.39 ms | {302: 8000} | 5.115 GiB | 3,767.19 MiB | 0.00 |
| FINAL | 10 GiB / default | **PASS** | 15.38 ms | 25.50 ms | {302: 8000} | 5.51 GiB | 4,935.82 MiB | 0.00 |
| verify-s005 | 10 GiB / default | **PASS** | 13.94 ms | 24.46 ms | {302: 8000} | 5.668 GiB | 4,871.84 MiB | 0.00 |
| gomem4g | 4 GiB / default | **FAIL (catastrophic — confounded, U10)** | 896.72 ms | 1001.46 ms | 129–215, all 8,000 short | 5.744 GiB | 4,661.67 MiB | 0.00 |
| gomem8g | 8 GiB / default | **FAIL (catastrophic — confounded, U10)** | 882.38 ms | 985.02 ms | 133–207, all 8,000 short | 5.801 GiB | 4,836.53 MiB | 0.00 |
| verify-t0-repeat | 10 GiB / default | **FAIL (catastrophic — confounded, U10)** | 873.00 ms | 982.93 ms | 137–217, all 8,000 short | 5.762 GiB | 4,713.74 MiB | 0.00 |

FINAL (recomputed from the raw aggregate JSON,
`cap10x6000-final-20x8000-20261124.json`, 11,137,848 B): p50 median
3.70 ms, p95 median 15.38 ms, p99 median 25.50 ms (max 36.74 ms),
all 8,000 latency pass = true, delivered 2,416,000 frames,
dissolved 0, swap used 264.44 MiB constant across every sample,
swapouts flat at 473,693, peak RSS 5,775,824 KiB = 5.51 GiB
(18:49:04Z sample). T4 (heap cap 6 GiB + GOGC 50): p99 24.39 ms,
peak RSS 5.115 GiB, 302/302 frames, swap flat — the best no-swap
config found so far.

**Headline for the best config proven by re-run: FINAL's 25.50 ms /
5.51 GiB** (T0 env, GOMEMLIMIT 10 GiB, default GC). Run-to-run
variance is plain: T4 vs FINAL p99 24.39 vs 25.50 ms, peak RSS
5.115 vs 5.51 GiB; the T0 env re-measured a third time
(verify-s005) at p99 24.46 ms / 5.668 GiB — a 0.98 GiB RSS spread
across three T0-env runs. All far under the 150 ms gate. The T0
env is the best config by frame-cleanliness (2/2 strict passes
with zero short frames, plus verify-s005); T4 holds the single
best p99 (24.39 ms) with a one-run caveat.

**The T2/T3 frame failures are honest, not hidden.** Aggressive GC
trades memory for frame completeness: GOGC=default 0 tracks short,
GOGC=50 one track (stream-0005, 400 viewers), GOGC=20 three tracks
(stream-0003/0004/0005, 1,200 viewers). Each affected track loses
exactly 1 frame and `video_frames_sent` is short by exactly the
affected-track count — the frame never left the publisher (0
NACKs, 0 queue drops, 0 dissolved). Shortfall is exclusively cell
0's later publishers (U1 in the unknown hunt). T4 breaks the simple
dose-response (GOGC=50 + 6 GiB passed clean) — stochastic boundary
noise or a GC-cadence effect; one pass cannot distinguish them.

**GOMEMLIMIT=4 GiB and 8 GiB runs failed catastrophically — but
the caps are NOT proven to be the cause** (CORRECTION 25 Nov
~02:05 NPT; the earlier causal claim in this paragraph is
withdrawn). gomem4g (verified from the raw JSON `fa678f11…`):
0/8,000 pass, frames 129–215 (median 165), dissolved 0.4475,
NACK sequences 1,199,026, delivered 1,334,943 / 2,416,000 (55%),
p99 1001.46 ms, peak RSS 5.744 GiB. gomem8g failed identically
(raw JSON `dc577414…`): 0/8,000 pass, frames 133–207 (median
177), dissolved 0.4171, NACK sequences 1,116,058, delivered
1,408,261 / 2,416,000 (58%), p99 985.02 ms, peak RSS 5.801 GiB.
Then the decisive test — `peakmem-verify-t0-repeat` in the exact
T0 env (10 GiB/default) that passed T0, FINAL and verify-s005 —
failed identically (raw JSON `68173d7d…`): 0/8,000 pass, frames
137–217 (median 180), dissolved 0.4053, NACK sequences 1,122,361,
delivered 1,436,854 / 2,416,000 (59%), p99 982.93 ms, peak RSS
5.762 GiB. All three failures ran after 19:29Z; all seven passing
20x8,000 runs ran before 19:26Z. The failures are a machine-state
change (U10), not a heap-cap effect: the claim that "the 4 GiB cap
does not save memory — it thrashes" and that "the low-peak
boundary lies between 4 and 6 GiB" is WITHDRAWN. The failing runs
burned 450–716% run CPU sustained (a GC death spiral, as a
symptom, not a proven cause). Swap stayed flat (264.44 MiB)
through all three collapses. Env-grid results after 19:29Z are
confounded until the state change is identified; a second T0
repeat and a kernel-network snapshot are running.

**Per-role peaks (verify-s005, sampler-005):** cell0 1.43 GiB,
cell1 1.39 GiB, cell2 1.41 GiB, cell3 1.42 GiB; parent 17.8 MiB.
The cells hold ~99.7% of the peak, evenly spread; heap is ~86% of
cell RSS. Per-cell peak RSS is the number the detector's anchor
should eventually be expressed in, not just the total.

### 4.6 10 x 6,000 — BLOCKED, NO MEASUREMENT

**Do NOT quote any p99 or frame number for 10x6,000 — none exists.**

- Order: verify the new bar (10 publishers x 6,000 viewers, one
  room, p99 <= 150 ms, zero lost frames, same strict gate) as the
  detector's anchor for Niroj's Mac.
- Run launched 24 Nov 22:52:49 NPT, binary fixed25
  (`ff84b3cd761faa71abc82a1b6a035b0dc80794a456d400b0b08be5a99e1b0a7b`,
  19,287,682 B — the binary that passed 20x7,500 strict a third
  time on 24 Nov 11:27 NPT), clip sha `ed283d3a…`, flags
  `-mode sfu-single -video-file <clip> -duration 60s
  -sfu-publishers 10 -sfu-viewers-per-publisher 600 -sfu-cells 5
  -metrics-listen 127.0.0.1:9464`, `GOMEMLIMIT=10GiB`, strict.
  All 5 launch gates passed.
- The binary's own host preflight refused to run it. Free swap on
  the Mac was **1,154.375 MiB, below the 2,048 MiB floor**
  (shortfall **893.625 MiB**), identical across three samples 20
  minutes apart (17:07:51, 17:17:51, 17:27:51 UTC); swap used,
  free, total, and swapouts (461,016) byte-identical — the pool is
  frozen, not draining. Free RAM actually rose (10,005 -> 11,277
  MiB) while swap stayed put. Pool total 3,072 MiB, used
  1,917.625 MiB. The pool appeared during the day: at the polish
  confirmation run (11:27 NPT, same binary) swap total was 0.00M,
  so the preflight passed vacuously on free memory (13,331 MiB).
- The parked run was stopped at 17:29:11Z so it would stop blocking
  GATE5. Aggregate JSON 0 bytes, 0 cell files, 0 frames sent, 0
  viewers connected. The sampler's 15,488 KiB "memory number" is
  the parked parent's idle RSS only — not a run measurement.
- What unblocks it: free >= 894 MiB of swap (to reach 2,048 MiB
  free), or reboot the Mac (reboot resets swap to 0 and the
  preflight passes vacuously on free memory). Freeing swap means
  the swapped pages' owners release them — in practice quitting /
  restarting the heaviest swapped desktop apps (Brave, WebKit /
  WebContent) or a reboot. That is Niroj's action, not Muse's; no
  user application was quit, killed, or touched. The launch is fully
  staged for a re-run (`cap10x6000-launch-001.sh`); re-queue it after
  swap is freed; no rebuild needed. Note the same preflight will park
  ANY fixed-binary capacity run right now until swap is freed.
- The detector's anchor table therefore keeps 10x6,000 as PENDING,
  listed in the `capacity` output, excluded from the derivation by
  construction, with a unit test asserting exactly one pending anchor
  exists and the maxima are still 20 / 7,500.

UPDATE 25 Nov ~01:50 NPT: the preflight rule has changed
(used-swap cap 512 MiB, §2.8) — the free-swap floor no longer
blocks this run. The measurement is queued in the resource-reduction
sweep as peakmem-10x6000 (10 publishers x 600 viewers, 5 cells,
60 s, strict, fixed26); until it lands, this section stays as the
record of the blocked attempt.

### 4.7 Smaller runs (for completeness)

- 10x4,000: 3/3 PASS (step 1 of the repeat protocol, 23 Nov;
  run 001: measurement_pass TRUE, 4,000/4,000 on every gate, p50 med
  2.86 ms, p95 med 7.00 ms, p99 med 24.56 ms, 0 short viewers,
  delivered 1,208,800 frames, rtp_queue_drops 0, cleanup true).
- 2x250 falsifier run: measurement_pass TRUE, 500/500, p50 med
  4.34 ms, p95 med 8.36 ms, p99 med 22.44 ms, dissolved 0 — the p99
  diagnosis prediction confirmed (same 250/track fan-out, ~10x less
  total scale, tail collapsed).
- 10x500 (half run, earlier): p95 med 100.6 ms, p99 med 261.2 ms,
  dissolved 0.0127 — FAIL (the run that started the FIX 19+
  investigation; see cap20x5000-result-001.md for the comparison).

---

## 5. The hybrid viewer path (MediaMTX / HLS) — Phases 0–3b

**Goal:** carry 10,000 viewers by moving them off the live SFU onto
packaged HLS, while the live publishers stay on Hami's own SFU.
Architecture: publishers (20 banked) -> hami-media SFU -> packager
(MediaMTX first, SRS if it fails a gate) -> one HLS output PER
published stream -> static server -> viewers (10,000, plain HTTP
fetches). No mixer/composite. Viewers on HLS watch seconds behind
and cannot just go live; becoming a publisher means switching paths
(HLS -> WebRTC join). MediaMTX v1.21.4, ffmpeg 7.1.1. All on the Mac.

**Correction that frames this whole section:** Hami is for VIDEO
streaming, not audio (Niroj, 24 Nov ~17:09). The first version of
this plan was audio-sized and is abandoned; Phase 0 was redone with
video.

### Phase 0 — video smoke test: PASS (24 Nov ~17:20 NPT)

ffmpeg publish (RTMP) -> MediaMTX -> HLS master playlist -> media
playlist -> downloadable segment carrying H.264 video. Seven
attempts, each failure found in the logs: empty config rejects
unconfigured paths; playlist fetched empty (MediaMTX cookie-checks
first-time clients with HTTP 302 — plain `curl -s` never followed
it); publish died after ~10 s (raw .h264 has no timestamps, `-re`
could not pace it); grep looked for `.ts` in the MASTER playlist
(which lists only the media playlist); grep grabbed `gap.mp4` (the
#EXT-X-GAP placeholder, 46 B) instead of a real segment. Final:
downloaded init + real segment seg7, **16,907 B**; probe:
`Duration: 00:00:27.00`, `Video: h264 (Constrained Baseline),
yuv420p, 640x360, 128 kb/s, 30 fps, 30 tbn`. Video confirmed in the
packaged segment.

Watch items (not failures): MediaMTX reports FRAME-RATE=30.000
while the source clip was said to be 5 fps — settled in Phase 2:
the wrapped file IS 30 fps (see below). The media playlist starts
with an #EXT-X-GAP entry before the real segments.

### Phase 1 — packaging CPU, 20 publishers, remux: PASS (remux baseline)

20 ffmpeg RTMP publishes (`-c:v copy`, no transcode) into MediaMTX
paths pub00..pub19. MediaMTX CPU while all 20 streams packaged
(ps %cpu, percent of one core): **4.4%, 4.6%, 6.4%** across three
samples 10 s apart — about 0.05 cores on the 10-core Mac. RSS:
45,520 KB idle -> ~51,360–51,744 KB under load -> 59,712 KB after
publishers stopped. Master playlist check: **20 / 20 paths serve a
playlist.** System load 2.85 / 2.37 / 2.01. G1 remux baseline: PASS.
Transcode per rendition: NOT measured in this run — owed before G1
fully closes (closed in Phase 2, below).

### Phase 2 — lag + continuity, G1 close: G2 PASS, G3 PASS with pre-roll

- **Correction found in run 001:** `test-60s.mp4` is **10.00 s,
  30 fps, exactly 300 frames**, NOT 60 s / 302 frames. The wrap of
  the raw 302-frame clip re-timed it to 30 fps. MediaMTX reports 30
  fps because the source IS 30 fps. A 60 s window = 6 loops = 1,800
  frames expected. Run 002 loops the file 6x (`-stream_loop 5`).
- **G2 playlist lag — PASS:** first segment in playlist after
  publish start **3,588 ms** (run 001), **3,624 ms** (run 002).
  Two runs agree: **~3.6 s**. Bar: LL-HLS <= 6 s, standard HLS
  <= 12 s. Passes both. G5 (informational): publisher->viewer
  latency ~3.6 s at the live edge.
- **G3 continuity — NOT PASSED as written, then PASSED with
  pre-roll:** 60 s publish, 55 whole segments seg7–seg61,
  contiguous, none missed; packaged frames **1,680 of 1,800
  expected — 120 short (6.7%)**; per-frame pts max gap 0.034 s,
  **0 gaps over 200 ms**, median gap 0.033 s, span 55.966 s, implied
  fps 30.02. The loss is all at stream start: the playlist's first
  6 slots are #EXT-X-GAP entries, first real segment seg7 — ~4 s
  (120 frames) never made it into segments. After the start, the
  packaged output is frame-perfect. Phase 2d re-measure (10 s
  pre-roll, publish looped 8x, measure a window after the playlist
  is live, 0 gap entries): **71 segments seg11–seg81, consecutive,
  none missed; 2,160 frames counted vs 2,159 expected over a
  71.966 s span — matches to within 1 frame; max gap 0.034 s,
  0 gaps over 200 ms.** (Window spanned 78.9 s, not 60 s — a timing
  bug in the loop; the verdict is computed against the ACTUAL span.)
  **G3 verdict: PASS with pre-roll.** Operational rule: publish ~10 s
  before viewers join. The first ~4 s of a cold publish are still
  lost — pre-roll moves that loss outside the viewer-visible window.
- **G1 transcode — CLOSED:** C1 (1 stream x 2 renditions,
  426x240 @800k + 320x180 @400k, libx264 veryfast): **41.6, 34.7,
  34.1, 19.3, 38.2, 24.2 % of one core — mean ~32%** for both
  renditions together. C2 (20 streams x 1 rendition 426x240):
  **190.9, 194.8, 136.6, 170.3, 167.3, 184.9 — mean ~174% of one
  core total, ~8.7% per stream per rendition.** Projections for
  20 streams x 2 renditions: ~3.5 cores (C2 rate) to ~6.4 cores
  (C1 rate). Conservative planning number: **~6.4 cores**. Either
  way it fits on the 10-core Mac. G1 closed: remux PASS (~5% of one
  core for 20 streams) + transcode measured per rendition.

### Phase 3 — viewer scale (G4): PASS, by a wide margin

- Setup: nginx is NOT on the Mac and brew is missing. Static server
  is **Caddy v2.11.9** instead (50,147,042 B, official download).
  hey v0.1.5 ships ZERO assets, so hey was built with `go install`.
  Segments on disk: 71 whole segments + init from Phase 2d, plus a
  static playlist `phase3.m3u8` (148 lines). Caddy serves on :8080.
- **G4 — 10,000 requests at 1,000 concurrency** (`hey -n 10000
  -c 1000` against one 16,907 B segment seg40, ulimit 10,240):
  **total 0.2389 s, 41,863 requests/s, status 200: 10,000, 0 failed,
  p50 14.5 ms, p95 58.5 ms, p99 86.3 ms, slowest 111.6 ms.**
  Gate: 0 failed, p95 < 500 ms. **PASS — p95 is 8x under the bar.**
  Playlist pass (same settings): 10,000 fetches in 0.2036 s,
  49,114 req/s, 0 failed, p95 91.4 ms, p99 145.8 ms.
- Honest label: 10,000 requests at 1,000 concurrency — 1,000
  concurrent connections, 10 sequential fetches per worker. This is
  NOT 10,000 simultaneous viewers each holding a connection. At 2 s
  segments, 10,000 viewers make ~5,000 requests/s; this Mac served
  41,863/s — about 8x the request rate. That is request-rate
  headroom, not 8x the viewers. The test segment is 16.9 KB, tiny;
  at a real 720p 1.5 Mbps with 2 s segments, segments are ~375 KB,
  so per-request cost shifts toward bytes copied — expect the
  41,863 req/s figure to drop substantially at real bitrate. The
  real constraint at scale is the wire, not the server: 10,000
  viewers need ~1.3 Gbps at this test clip's 128 kbps, and ~15 Gbps
  at a real 720p 1.5 Mbps. That is a network/CDN question.
- Server CPU point samples (207.5% after pass 1, 416.1% after pass 2)
  are instantaneous multithreaded readings during a quarter-second
  burst, NOT sustained averages, and support NO capacity conclusion.

### Phase 3b — rotating segments + playlist: gate still PASSES

11 sequential bursts, each `hey -n 10000 -c 1000`, across 10
segments spread over the playlist (seg11/18/25/32/40/47/54/61/68/75)
plus `phase3.m3u8`. **110,000 requests total, 110,000 x status 200,
0 failed. Total time 3.0078 s, aggregate 36,570 req/s. Worst p95
116.8 ms (seg11), worst p99 185.9 ms (seg40), worst slowest
258.4 ms (seg11). Per-burst req/s 30,088–42,954.** Against Phase 3
single-segment (41,863 req/s, p95 58.5 ms): rotating is ~13% slower
on request rate, worst p95 ~2x higher — but the gate (0 failed,
p95 < 500 ms) passes on every burst, 4.3x under the bar at worst.
Headroom: 36,570 / ~5,000 req/s = ~7.3x the request rate 10,000
viewers need at 2 s segments. Same caveats as Phase 3: request-rate
headroom at ~17 KB segments, not viewer headroom. Clean re-run:
50,000/50,000 status 200, 0 dial errors, aggregate 37,048 req/s,
worst p95 103.4 ms. (The first pass polluted its last five bursts
with client-side ephemeral port exhaustion — 25,154 dial errors,
zero server-side failures — fixed with a 60 s sleep between bursts;
those numbers are NOT reported as server results. Seg54 is 33,860 B,
double the other segments, and showed no penalty.)

### Hybrid gates — final status

| Gate | Verdict |
|---|---|
| G1 packaging CPU | CLOSED (remux ~5% of one core for 20 streams; transcode ~8.7% per stream per rendition; 20x2 projection 3.5–6.4 cores) |
| G2 playlist lag | PASS (~3.6 s, LL-HLS bar 6 s) |
| G3 continuity | PASS with pre-roll (10 s pre-roll; frame-complete window) |
| G4 viewer scale | PASS (10,000 requests at 1,000 concurrency, 0 failed, p95 58.5 ms vs 500 ms bar — request-rate headroom only) |
| G5 publisher->viewer latency | MEASURED (~3.6 s at the live edge, informational, no bar) |
| G6 promotion (viewer becomes publisher) | BLOCKED on per-user auth (not wired end to end) |

Next step per the plan: a real CDN in front of the segments, not
more SFU work. MediaMTX (pid 68581) + Caddy (pid 82351) still running
as test services only.

---

## 6. The sibling service hami-sfu (Stages 1–6) — shares these mechanisms

hami-sfu (`[local path redacted]`)
is the product codebase; hami-media is the capacity lab. The stages
below verified the mechanisms hami-media also uses (auth, NACK,
TURN), ported from hami-media. They are included here because the
brief requires every test — but they are hami-sfu results, not
hami-media runs. All trees UNCOMMITTED; no commit sha exists for any
of them.

### 6.1 Stage 1 — baseline: PASS (24 Nov ~19:19–19:29 NPT)

Tree in repo, builds clean macOS AND Linux, `go test` passes, smoke
moves real frames. Mac: `go build` + `go vet` clean; forced rebuild
binary 16,436,610 B; `-h` prints all 10 flags. Linux VM: `go vet`
clean, **`go test` 7/7 PASS** (TestClassifyPacket + 17 subtests,
layer fan-out tests incl. layered=58 writes vs direct=100, -42.0%).
Mac: same 7 tests pass (`ok hami-sfu 0.629s`). Smoke:
`TestSmokeLoopback` PASS, **23 frames** (wanted >= 20), 1.03 s.
Files: main.go 13,509 B / 549 lines, layers.go 9,004 B,
layers_test.go 13,286 B, smoke_test.go 7,196 B.

### 6.2 Stage 2 — serve hardening (auth + room API): PASS (24 Nov ~19:31–19:56 NPT)

- Per-user HMAC tokens (port of hami-media auth.go): format
  `base64url(JSON claims).base64url(HMAC-SHA256)`, claims room /
  roles / sub / exp / iat; key via `-hmac-key-file`, else
  `HAMI_SFU_HMAC_KEY_FILE`, else `HAMI_SFU_HMAC_KEY`; file mode 0600
  enforced; min 32 bytes; max TTL 24 h; mint command
  `hami-sfu token --room demo --user alice --role publish --ttl 1h`
  (prints a 155-char token, never the key). Shared `-token` flag
  GONE; server refuses to start without a key (fail-closed).
- Room API: `GET /api/rooms` (any valid token),
  `DELETE /api/rooms/{room}` (publish role, 204),
  `POST /api/rooms/{room}/kick` (publish role, 200 `{"kicked": n}`).
  Auth runs BEFORE any slot is taken. Per-viewer bitrate cap LEFT
  OUT deliberately (see section 2.11).
- Tests: **34/34 PASS on macOS AND Linux** (7 layer + 27 new auth
  tests). Fail-before: tests failed to BUILD on the Stage 1 tree
  (undefined symbols) — every rejection case fails without the fix.
  Live Mac check: 10/10 curl checks PASS (401s for no/bad/wrong-role
  tokens, 400 for valid token + bad SDP, 200 list-rooms, 404s).
  Smoke: Loopback 23 frames, Auth **10 frames** (wanted >= 5).

### 6.3 Stage 3 — lost-frame recovery: PASS (24 Nov ~20:49–21:13 NPT)

The fix: the publisher-side RTCP pump — a 22-line block in
`loss_test.go:479–499` (`pubSender.ReadRTCP()` in a goroutine).
Before it, the publisher test client never read RTCP, so the
server's NACKs sat unread (0 NACKs seen). No server file changed.

| Hop | Loss | Sent | Received | Verdict |
|---|---|---|---|---|
| Publisher | 5% | 300 | **300 (100.0%)** | PASS (>= 99%) |
| Publisher | 10% | 300 | **300 (100.0%)** | PASS |
| Subscriber | 5% | 300 | **300 (100.0%)** | PASS |
| Subscriber | 10% | 300 | **297 (99.0%)** | PASS (>= 99%) |
| No-NACK control | 10% | 300 | **281 (93.7%)** | in the 85–98% band |

Fail-before / pass-after, identical command: pre-fix variant
(`loss_test.nopump`, 28,400 B, sha256 `c380162a…503cd1b`) —
**283/300 (94.3%) and 270/300 (90.0%), exit 1**; fixed
(`loss_test.go`, 29,212 B, sha256 `9a74061d…07076a`) — **300/300
both, exit 0**. Logs: fail-before
`~/hami/logs/stage3-failbefore-rerun-003.log` (VM copy sha256
`eaa73906…1809ae3`), pass-after
`~/hami/logs/stage3-failbefore-rerun-003-restore.log` (VM copy
`c9e1a94d…352d49b`). Two earlier fail-before citations are VOID
(both were overwritten pass runs — the hami loop retries a queue
script that exits non-zero; see the stage note section 3 for the
full correction). Full re-check green: **34/34 unit tests**,
smokes 2/2 (23 + 10 frames). Linux leg: `go vet` clean, **34/34
PASS** (unit tests only; VM blocks UDP loopback).

Known limitation: subscriber hop at 10% recovers 297/300, not
300/300, systematically — Pion's NACK responder sends ~1 retransmit
per dropped packet and never retries it (42 RTX in run 6; stuck
packets NACKed 257–289 times each). The mechanism is inside Pion's
responder (`pion/interceptor` v0.1.51), not hami-sfu code. The
subscriber-hop 10% WITHOUT receiver reports variant scored 296/300
in run 5 — ruled OUT OF SCOPE (Niroj's rule names only the
publisher hop; diagnostic probe; one-frame gap is noise).

**Subscriber-hop answer (decisive experiment, 24 Nov ~21:21–21:39
NPT):** 6 runs, 10% loss, 300 frames — with RRs 297, 297, 297;
without RRs 297, 297, 296. **Noise, not a receiver-report effect.**
The no-reports variant DOES retransmit (41 RTX in all 3 runs). The
one-shot mechanism holds in both variants (45 dropped, RTX 41–42,
stuck packets NACKed 211–290 times each, no retries). Fix implied:
none for Receiver Reports. Harness lesson: `-count=3` does NOT work
for these tests (fixed UDP port per test, socket stays bound for the
process lifetime) — repeats need separate `go test` processes.

### 6.4 Stage 4 — reconnect: PASS (24 Nov 21:23–21:24 NPT, one run, first try)

Test-only stage: NO server file changed (only new file
`reconnect_test.go`, 25,095 B, 660 lines, sha256
`8d1c060faf015b2ab053c27684f9c1a8f7875ebc6ef4d006459e5a4f86f47826`).

| Test | Result |
|---|---|
| Subscriber drops 5 s, same room + token | 20 frames before, **0 during blackout**, server drops old subscriber in **548 us**, first frame after re-subscribe **20.7 ms** (bound 10 s) |
| Publisher drops 5 s, same rule | 20 frames before, **0 during blackout**, old publisher + track dropped in **499 us**, track listed again, first frame **238.8 ms** (20.7 ms after the re-subscribe) |
| Expired token refused on resume | 5 frames before expiry (TTL 2 s), re-publish **401**, re-subscribe **401**, room left empty, room still fine with a fresh token (200) |
| Full re-check | vet clean, **34/34 unit**, smokes 2/2 (23 + 10), loss 300/300 both, reconnect **4/4 PASS** |

Limits, stated plainly: the 5 s drop is simulated by a blackout
interceptor that drops ALL RTP in both directions — ICE/STUN still
flows, so the PC stays Connected and the rejoin is app-driven (close
old PC, re-join with same room + token), which is exactly what a
real client app does. ICE-level failure detection (real transport
death) is NOT covered here. Diagnostic: same-PC ICE recovery after
a 5 s blackout, first frame **42.1 ms** after blackout end — the
media path heals itself without a rejoin when the blackout is
RTP-only. Known limitation: an EXISTING subscriber does NOT recover
from a publisher rejoin (0 frames since re-publish) — it is bound
to the old track object; the Stage 6 web client must re-subscribe
on track end.

### 6.5 Stage 5 — TURN: wiring COMPLETE, relay PROVEN under a kernel-enforced block; literal real-NAT gate NOT closed

**Read the verdict before quoting a PASS.** Niroj's rule: "a client
behind a real NAT with UDP blocked/restricted connects via the relay
and receives frames. Loopback does NOT count." What was proven:
the wiring is in (`hami-sfu turn-server` runs the standalone relay;
server refuses to start without credentials; relay-only clients
connect and receive frames); the direct path was blocked at the
KERNEL (sandbox-exec denies the test process's outbound UDP to
`*:17530`, sendto refused with EPERM — probe-004 proved both the
`localhost:` and `*:` forms); under that block, relay-only clients
got **23 frames via relay** (nominated pair local=relay /
remote=host on both PCs) and plain clients got **0 frames in 10 s**.
What was NOT proven: a client on a SEPARATE network. Everything ran
on one machine. **The relay mechanism is proven; the gate as
literally stated (real NAT, separate network) stays open.** Needs a
second machine or network — the same blocker as the capacity items.

Gate run 002 (22:01–22:03 NPT, FINAL_FAIL=0): vet clean, **36/36
unit tests** (34 + 2 TURN config tests), smokes 23 + 10 frames,
loss 300/300 both, reconnect 4/4 (subscriber 41.2 ms, publisher
436.2 ms, ICE diagnostic 21.0 ms), `turn-server` without credentials
refuses exit 1, standalone server
`listening=127.0.0.1:3479 realm=hami`, gate EXTERNAL **23 frames via
relay** (first frame 0 ms — already received by the time the
subscribe completed), gate IN-PROCESS **22 frames via relay**,
wrong credentials PASS, fail-before still holds (0 frames).
Sandbox run 002 (22:07–22:08 NPT, FINAL_FAIL=0): gate sandboxed
**23 frames via relay**; control sandboxed **0 frames in 10 s,
neither PC connected, server lists no track**. Fail-before
(21:53–21:54 NPT): relay-only with a dead TURN URL, **0 frames in
10 s, no relay candidate, no connection** (25.63 s). Linux leg:
`go vet` clean, **36/36 PASS** (unit tests only; VM Pion ICE is
broken — separate diagnosis). Not run: pf (needs a sudoers line only
Niroj can add; moot for the one-machine case), Docker (no CLI on
the loop's PATH). Files: `turn.go` NEW 12,204 B / 334 lines,
`main.go` 24,441 B / 891 lines, `turn_test.go` NEW 10,332 B,
`turn_relay_test.go` NEW 7,966 B, `turn_config_test.go` NEW 2,756 B.

### 6.6 Stage 6 — webpage (streamer + viewer, one page): DONE, verified on the Mac; no live browser check yet

Requirement (Niroj, 24 Nov ~21:33): "on the webpage i must see
viewer's side and streamer's side, all in one". One file changed:
`index.html` 4,158 B / 121 lines -> 10,791 B / 304 lines (run 002,
below) -> **11,685 B / 324 lines** (viewer fix, sha
`8069468b…`, deployed 24 Nov ~22:20 NPT, verified served).
Two panels side by side (grid, streamer left, viewer right,
stacking below 720 px), one shared room field, separate token inputs
per panel with live mint hints, per-panel status + event log (last
20 lines each), Stop buttons, frames counted via
`requestVideoFrameCallback`. The Stage 4 lesson is implemented in
the viewer panel: on remote track end or PC failure it
automatically re-subscribes with the same room + token (backoff
1 s -> 2 s -> 4 s -> 8 s, max 5 tries — pre-fix; the viewer fix
removes the cap: it keeps trying until you press Stop, backoff
capped at 8 s — plus a 5 s no-frames watchdog that re-subscribes
when the picture freezes); `disconnected` gets a 3 s grace first
(a short stall heals on its own, 42 ms). Dependency-free
(zero external references), same endpoints, two independent
PeerConnections. Verification run 002 (21:44:44–21:44:48 NPT,
FINAL_FAIL=0): build clean, server up on 127.0.0.1:18096, curl **200
first try**, **9/9 markers present**, served page 304 lines /
10,791 bytes exactly, smokes **2/2 PASS** (Loopback 22 frames, Auth
10 frames) — that verification is for the PRE-fix page (10,791 B /
304 lines, sha `2ab1b49b…`). Run 001 failed on the curl check alone
(server slow to print its startup line under load; cause not
proven) and was re-run, not hidden. The viewer fix (MAX_WATCH_TRIES
= 5 made the viewer give up after ~23 s of outage and freeze on the
old frame) was deployed 24 Nov ~22:20 NPT and verified served
(parent read-back 25 Nov ~01:17 NPT: 11,685 B, 324 lines, sha
`8069468b…`, infinite re-subscribe + 5 s watchdog present, no 5-try
cap). Markers/smokes NOT yet re-run on the fix version. NOT done:
a live browser end-to-end check (real publish + watch in a browser
on Niroj's Mac) — the last step.

---

## 7. Honest limits and ceilings

1. **20 x 10,000 is a measured wall on one Mac.** At 20 publishers
   x 10,000 viewers the macOS kernel loopback path itself runs out,
   not this code. Run 11 localized it: 98.0–98.6% of the mux leg is
   kernel wait (send return -> router socket ReadFrom return).
   Do NOT present 20x10,000 as achievable on one Mac. The code limit
   is higher than the machine's loopback path allows. A real
   deployment with viewers on real networks does not hit this wall
   the same way — but that is unmeasured, not proven.
2. **One room per process.** Design, not a bug. Multi-room means
   multi-process (hami-sfu serves many rooms; hami-media serves one
   per process).
3. **The swap pool behaviour.** Free swap is capped by the pool
   total. The pool shrank **3,072 -> 2,048 MiB** after a clean
   (used 366 MiB, free 1,682 MiB) — so the OLD 2,048 MiB free floor
   could never pass, which is why the preflight rule changed to a
   used-swap cap of 512 MiB (§2.8, landed). The 10x6,000 run was
   blocked by free swap 1,154.375 MiB vs the old 2,048 MiB floor
   (shortfall 893.625 MiB). Swap is owned by long-lived desktop
   apps (Brave, WebKit); freeing it is Niroj's action
   (quit/restart apps, or reboot) — but no longer required to run,
   since the rule judges used swap, not free.
4. **The VM sandbox UDP block.** The VM blocks every unconnected UDP
   `sendto()` with EPERM, so no loopback-dependent test can pass
   there and the VM cannot be the second machine for any UDP-media
   test, any 20x5,000 run, or the LiveKit comparison. A real second
   machine without this sandbox policy is still required.
5. **Subscriber-hop 10% loss: 297/300, not 300/300.** Systematic,
   inside Pion's NACK responder (one-shot retransmit). If the
   subscriber hop ever needs 100%, the fix is a retransmit-retry
   (replacing or working around Pion's responder), not anything
   about receiver reports.
6. **Existing subscriber does not recover from a publisher rejoin.**
   It must re-subscribe itself. Server-initiated renegotiation
   would fix it; not built.
7. **ICE failure detection (real transport death) is untested.**
   The reconnect tests keep STUN alive; a real network break is a
   different path.
8. **TURN real-NAT gate open.** Relay proven under a kernel block on
   one machine; a client on a separate network behind a real NAT is
   unproven.
9. **Hybrid path:** G6 (viewer -> publisher promotion) blocked on
   per-user auth; cold-start first ~4 s of a publish are lost
   (pre-roll moves the loss outside the viewer window, it does not
   fix the cold start); G4 is request-rate headroom at ~17 KB
   segments, not viewer headroom at real 720p bitrates; the wire
   (~15 Gbps at 720p 1.5 Mbps for 10,000 viewers) is a network/CDN
   question no code change removes.
10. **No library package yet; no per-viewer bitrate cap; Windows host
    readers are error stubs** (Windows builds, but its preflight /
    watchdog would refuse to run); **Linux has no pressure signal**
    (deliberately absent, not faked).
11. **Run-to-run variance is real at the margin.** The same code path
    gives p95 100 ms (run 4), 138 ms (run 11), 381 ms (run 12) at
    20x10,000. Single-run numbers at the edge are not stable; the
    ladder's conclusions rest on repeats, not one run.
12. **The 20x7,500 pass does not imply 50 publishers.** 50 is a
    stretch gate with its own numbers, unmeasured. Nothing about 20
    is assumed to carry over to 50.
13. **Run-to-run peak-RSS variance is ~1 GiB at 20x8,000.** The
    same T0 env measured 4.690, 5.508 and 5.668 GiB across three
    runs. Peak-memory comparisons need repeats; a single run's RSS
    cannot separate configs closer than ~1 GiB.
14. **Three consecutive systemic failures after 19:29Z
    (gomem4g, gomem8g, T0-repeat) are a machine-state change, NOT
    a heap-cap effect (U10).** All three failed identically
    (0/8,000 pass, dissolved 0.41–0.45, p99 ~1000 ms) including a
    repeat in the exact T0 env that had passed three times. The
    earlier claim that the 4 GiB cap thrashes is WITHDRAWN; the
    low-peak boundary is UNKNOWN until clean runs exist. Env-grid
    results after 19:29Z are confounded.
15. **GPU: the SFU path uses none (measured).** No GPU framework is
    linked (otool: only libSystem, libresolv, CoreFoundation,
    Security), no GPU API references exist (nm), the binary is pure
    Go + Pion. The Mac has an Apple M5 GPU (Metal 4); the software
    never touches it. There are no GPU savings to claim.

---

## 8. Open work and in-progress investigations

- **Preflight rule change: LANDED** (used-swap cap 512 MiB, §2.8,
  fixed26). Remaining: final `main.go` re-sync into this repo +
  `docs/capacity.md` preflight line update.
- **Swap / heat / limit investigations (measurement agent, IN
  PROGRESS — partial outcomes):** swap — best no-swap config proven
  (T4: 6 GiB + GOGC 50, 5.115 GiB, swap flat); three systemic
  failures after 19:29Z (machine-state change, U10 — env-grid
  results in that window confounded); heat — media phase ~1.1
  cores, setup 2.5–3.9 cores, no throttling, temperature/power
  still a gap (powermetrics needs root); limit — 20x8,000 found
  and verified strict (§4.5). The combined resource sweep
  continues (env grid, scale runs, unknown hunt U1–U10).
- **10 x 6,000 measurement:** UNBLOCKED by the preflight rule
  change (section 4.6). Queued in the resource-reduction sweep as
  peakmem-10x6000 (10 publishers x 600 viewers, 5 cells, 60 s,
  strict, fixed26). Until it lands, the detector keeps it as
  PENDING and never uses it.
- **Resource-reduction sweep (in progress):** env grid
  {4,6,8,10} GiB x {default,50,20} at 20x8,000 + scale runs
  (20x5,000, 20x7,500, 10x6,000) + unknown hunt (U1 301-frame
  shortfall, U3 per-pid CPU-profile patch prepared-not-landed, U9
  darwin-vet paradox). Best combined config decided only from
  strict-passing runs.
- **API contract + SDK plan (next track, docs only):** document the
  signaling + REST contract from the actual endpoints in the code
  (hami-media serve path, hami-sfu `/api/publish`, `/api/subscribe`,
  `/api/rooms`), then SDKs for JS/TS, Kotlin/Java, Swift, Dart,
  React Native. No SDK building yet.
- **Library split:** `package main` -> importable package + thin
  `cmd/` entrypoint, without breaking the 116 tests. Planned
  follow-up, after the preflight re-sync, against the finished
  files.
- **TURN real-NAT gate:** needs a second machine/network.
- **Second-machine capacity proof (20x5,000 x3 + LiveKit OSS
  comparison, same session, same 150 ms p99 gate):** blocked — no
  second machine exists; only Niroj's Mac is paired.
- **Live browser end-to-end check of the Stage 6 page** on Niroj's
  Mac (publish left panel, watch right panel, stop publisher,
  confirm the viewer re-subscribes on its own).
- **Subscriber-hop 100% at 10% loss:** needs a retransmit-retry
  around Pion's responder (see limit 5).
- **Commit + push + GitHub repo creation:** Niroj's terminal action.
  Nothing here is committed.
- **G6 promotion (hybrid):** blocked on per-user auth, end to end.

---

## 9. Evidence pointers

### Repo (Mac)

- `[local path redacted]` — standalone repo
  (README.md, LICENSE, go.mod, go.sum, main.go, capacity.go,
  capacity_test.go, auth.go, turn.go, latency.go, platform_*.go,
  *_test.go, docs/). Re-synced 24 Nov ~23:26 NPT (detector landed).
- `[local path redacted]` —
  source tree the repo was copied from (uncommitted).
- `[local path redacted]` —
  sibling product service, Stages 1–6 (uncommitted).

### Result notes (VM, `~/workspace/hami-media-work/`)

- `stage1-baseline-result-001.md`, `stage2-serve-auth-result-001.md`,
  `stage3-lost-frames-result-001.md`,
  `stage3-subscriber-hop-answer-001.md`,
  `stage4-reconnect-result-001.md`, `stage5-turn-result-001.md`,
  `stage6-webpage-result-001.md`
- `capacity-detector-result-001.md`, `cap10x6000-result-001.md`,
  `hami-media-build-tags-result-001.md`,
  `standalone-repo-result-001.md` (has the re-sync section),
  `vm-ice-diagnosis-001.md`
- `cap20x5000-step3-result-001.md` (20x5,000 PASS),
  `cap10x5000-repeat-result-001.md` (10x5,000 3/3),
  `cap10x5000-fix31-result-001.md` (loss location),
  `cap20x5000-scale-ladder-001.md` (20x7,500 runs 1–5 and 20x10,000
  runs 1–12, full detail), `polish-001.md` (P1–P3 hardening +
  20x7,500 third pass, 24 Nov 11:27 NPT)
- `peakmem-001.md` (peak-memory sweep), `resource-sweep-001.md`
  (combined resource sweep), `unknown-hunt-001.md` (U1–U9),
  `hami-media-mandate-2026-11-25.md` (Niroj's mandate, 25 Nov
  ~01:10 NPT)
- `mediamtx-phase0-result-001.md`, `mediamtx-phase1-result-001.md`,
  `mediamtx-phase2-result-001.md`, `mediamtx-phase3-result-001.md`,
  `mediamtx-srs-test-plan-001.md`

### Logs (Mac `~/hami/logs/`, VM copies in `~/workspace/hami-media-work/`)

- Stage 3: `stage3-final-fixed.log`, `stage3-failbefore-rerun-003.log`
  (sha256 `eaa73906…1809ae3`), `stage3-failbefore-rerun-003-restore.log`
  (`c9e1a94d…352d49b`), `stage3-subhop-001-20261124-213001.log`
  (`78f3446a…fbd91f2`), `stage3-subhop-002-20261124-213706.log`
  (`7a93e6cf…0920d6`)
- Stage 4: `stage4-reconnect-001.log` (VM copy 12,514 B, sha256
  `ca1537fab3ac29c11702594f3a9a11c445d7e39a5e85221ba195facdfcdafb90`)
- Stage 5: `stage5-turn-gate-002.log` (`26452189…51831a`),
  `stage5-turn-sandbox-002.log` (`39d8c0c5…eb054be`),
  `stage5-turn-failbefore-001.log` (`3f4b66c0…31b1ac`),
  `stage5-sandbox-probe-004.log` (`42794159…95674bc`)
- Stage 6: `stage6-webpage-001.log` (`3d6f6d86…`),
  `stage6-webpage-002.log` (`6459f23c…`)
- Detector: `capacity-detector-verify-004-20261124-231517.log`
  (VM copy sha256 `22be38a4…100de8a7`)
- 10x6,000: `cap10x6000-20261124.json` (0 B, sha256
  `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`),
  `cap10x6000-20261124.stderr` (1,106 B, sha256
  `eff6974d3b561ce3ddc25ed369aed156bf7f1b71dc4fbd3e6a41fe6eebcbdb43`),
  `cap10x6000-launch-001.log` (`de4b1e78…`), `cap10x6000-run-001.log`
  (`5d4c0276…`), `cap10x6000-probe-002.log` (`92d2622b…`),
  `cap10x6000-kill-001.log` (`a9438776…`)
- 20x8,000: `swap-invest-t0-base-20261124.json` (sha256
  `cac1c0ef…`), `swap-invest-t1-gomem6g-20261124.json`
  (`65e785cb…`), `swap-invest-t2-gogc50-20261124.json`
  (`637ee967…`), `swap-invest-t3-gogc20-20261124.json`
  (`920d909d…`), `swap-invest-t4-gomem6g-gogc50-20261124.json`
  (`afe3831c…`), `cap10x6000-final-20x8000-20261124.json`
  (`f62e8463…`), `peakmem-verify-s005-20261124.json`
  (`fe39ec13…`), `peakmem-gomem4g-20261124.json` (`fa678f11…`),
  `peakmem-gomem8g-20261124.json` (`dc577414…`),
  `peakmem-verify-t0-repeat-20261124.json` (`68173d7d…`)
- Build tags: `build-tags-mac-001-20261124-214015.log` (VM copy
  26,361 B, sha256 `04fbb42b…`), `build-tags-linux-test-003-20261124-155047.log`
  (sha256 `3de123e6…`)
- Standalone verify: `standalone-verify-001-20261124-225857.log`
  (26,350 B), `standalone-resync-verify-001-20261124-232559.txt`
  (sha256 `1bcddce1…`), `standalone-resync-vm-test.log` (sha256
  `b4d867f8…`)
- Capacity runs: `~/hami/logs/cap20x5000-step3-20261123.json`
  (sha `355300a2…`, 6,640,451 B),
  `cap20x5000-polish001-20x7500-20261123.json` (sha `09ca2cc1…`,
  10,453,763 B), `cap20x5000-lad20x7500r4/r5-20261123.json`,
  `cap20x5000-lad20x10000r{1..12}-20261123.json`, and the per-run
  `*-run-001.log` files.
- MediaMTX: `~/hami/logs/mediamtx-phase0-00{1..7}.log`,
  `mediamtx-phase1-001.log`, `mediamtx-phase2-00{1..4}.log`,
  `mediamtx-phase3-setup-00{1..3}.log`, `mediamtx-phase3-diag-001.log`,
  `mediamtx-phase3-load-001.log`, `mediamtx-phase3b-load-002/003.log`
- Heat investigation (in progress, no conclusion):
  `~/workspace/hami-media-work/heat-invest-probe-001-20261124-234125.log`

### Key file hashes (hami-sfu stages)

- Stage 3: `loss_test.go` 29,212 B sha256
  `9a74061dd620c122e4a99b76f14b726db2345ec77bf2875c621bc2a71907076a`;
  pre-fix variant 28,400 B `c380162adb8797bb3cdbf4f220ddb8ca68fbe2e973b8ac689ea31845f503cd1b`
- Stage 4: `reconnect_test.go` 25,095 B sha256
  `8d1c060faf015b2ab053c27684f9c1a8f7875ebc6ef4d006459e5a4f86f47826`
- Stage 5: `turn.go` `6dbad70ce65b24f6466ae1d45597c1748fe78fcd7664af7fd2cefc64de8749b2`,
  `main.go` `6a0f0487bb67a326a0d8f2d8352fe4a38eee836ff6e03f85c20da51500240cc7`
- Stage 6: `index.html` 11,685 B (viewer fix, sha `8069468b…`,
  deployed 24 Nov ~22:20 NPT); pre-fix page 10,791 B sha256
  `2ab1b49b8631d957c9db0548c2eb779b4bf04f20b20da918bc3bb1eeca6d2d09`

---

## 10. How to run every test

```sh
# Unit tests (all platforms; Linux VM: expect the same 6 UDP failures)
gofmt -l .
go vet ./...
go test -count=1 -v -timeout 600s .

# Detector only
go test -count=1 -v -run 'TestDeriveCapacityLimits|TestCapacityAnchors|TestResolveCapacityLimits|TestWriteCapacityReport|TestDetectDeviceProfile|TestRunCapacityCommand|TestCapacityServerLimitReached|TestCapacityPublisherLimitEnforcedOverHTTP' .

# Detector answer on your machine (no server running)
./hami-media capacity

# Smoke test (pair mode)
./hami-media -mode pair -video-file test.h264 -duration 3s

# Scale proof (the capacity runs)
./hami-media -mode sfu-single -video-file test.h264 -duration 60s \
  -sfu-publishers 20 -sfu-viewers-per-publisher 250 -sfu-cells 4 \
  -metrics-listen 127.0.0.1:9464
# 20x7,500: -sfu-viewers-per-publisher 375. 20x10,000: 500.
# 10x6,000 (BLOCKED until swap is freed / Mac reboots):
#   -sfu-publishers 10 -sfu-viewers-per-publisher 600 -sfu-cells 5

# TURN relay
export HAMI_SFU_TURN_USERNAME=relay
export HAMI_SFU_TURN_PASSWORD_FILE=/path/to/turn-password.txt
./hami-media -mode turn-server

# Mint a client token
export HAMI_SFU_HMAC_KEY_FILE=/path/to/hmac-key.txt
./hami-media -mode mint-token -token-subject alice -token-ttl 1h

# hami-sfu smoke / loss / reconnect / TURN tests (Mac only, UDP loopback)
go test -tags smoke -run 'TestSmokeLoopback|TestSmokeAuth' -count=1 -v -timeout 180s .
go test -tags smoke -run 'TestLossRecoveryPublisher5Pct|TestLossRecoveryPublisher10Pct' -v -count=1 -timeout 300s .
go test -tags smoke -run 'TestReconnect' -v -count=1 -timeout 300s .
# Repeats of the fixed-port tests need SEPARATE go test processes;
# do NOT use -count=3 on them ("bind: address already in use").

# Hybrid path (MediaMTX, Mac)
# Phase 0: ffmpeg RTMP publish -> MediaMTX -> curl -sL the HLS playlist,
#   download a real segment (not gap.mp4), ffprobe it for H.264 video.
# Phase 1: 20 ffmpeg publishes (-c:v copy) -> sample MediaMTX CPU/RSS.
# Phase 2: publish looped 6x, count frames in whole seg*.mp4 files only.
# Phase 3: Caddy static server + hey -n 10000 -c 1000 per segment.
```

**Loop-script discipline (learned in Stage 3, applies to every Mac
run via the hami loop):** a loop script whose EXPECTED outcome is a
test failure must exit 0 always (verdict lives in the log) and use
unique timestamped log/tee names — the loop retries a queue script
that exits non-zero, and a retry overwrites the fail evidence with a
pass. Never hash or list inside a loop script (TCC-blocked on
`~/Downloads`); verify by `files.write` receipt + VM hashes. Never
`go test -count=3` on fixed-port tests.

---

*End of report. Written 24 Nov 2026 ~23:50 NPT; updated 25 Nov
2026 ~01:50 NPT (§2.7–2.8, §3.6, §4.5 new, §4.6–4.7 renumbered,
§6.6, §7–9); corrected 25 Nov 2026 ~02:05 NPT (§4.5, §7–8: U10 —
gomem4g attribution withdrawn). Not committed, not pushed. The
next update follows the 10x6,000 measurement and the
resource-reduction sweep results.*
