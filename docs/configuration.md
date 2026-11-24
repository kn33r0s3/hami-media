# Configuration

Run `./hami-media -h` for the full flag list. This page covers the flags and settings you will actually touch.

## Modes

One binary, many modes. `-mode` picks the job.

| Mode | What it does |
|---|---|
| `pair` (default) | One publisher, one viewer, on this machine. The smoke test. |
| `sfu` | Local SFU proof run. |
| `sfu-single` | The scale proof: N publishers, M viewers each, one room. |
| `sfu-cascade` | SFU proof across worker processes. |
| `m1h` … `m9h` | Ladder proof runs of growing size. `m9h` is the 10,000-viewer event shape. |
| `m7h-sfu` | The real server process. Spawned by the harness; see below. |
| `mint-token` | Print one client token and exit. |
| `turn-server` | Run the standalone TURN relay until stopped. |
| `audio-rung1` | Audio-only proof run (speakers and listeners). |

## Scale flags (`-mode sfu-single`)

| Flag | Default | Meaning |
|---|---|---|
| `-sfu-publishers` | 1 | Publisher count (1–3000). |
| `-sfu-viewers-per-publisher` | 1 | Viewers per publisher (1–500). |
| `-sfu-cells` | 1 | Split the run across N worker processes. 1 is one process. |
| `-sfu-cell-index` | 0 | Which worker this process is (set by the parent). |
| `-sfu-score-mode` | `strict` | `strict` or `dissolve`. Dissolve allows a small fraction of late frames. |
| `-sfu-max-p99` | 150 ms | Worst allowed p99 frame latency for dissolve scoring. |
| `-sfu-max-dissolved` | 0.01 | Worst allowed fraction of dissolved (late) frames. |
| `-allow-unsafe-host` | false | Skip the host health check. Only for runs you fully control. |
| `-max-heap-bytes` | 0 | Stop the run when Go heap use passes this many bytes. 0 turns it off. |
| `-nack-max-queue-age` | (built in) | Drop RTP older than this from the retransmit queue. |
| `-video-file` | — | H.264 source file. Required for video modes. |
| `-duration` | 3 s | How long to send media. |
| `-metrics-listen` | — | Optional loopback-only Prometheus endpoint, e.g. `127.0.0.1:9464`. |

Hard caps in the code: 500 viewers per publisher, 10,000 viewers total. A run above those is refused.

## Server flags (`-mode m7h-sfu`)

The server process is started by the harness, not by hand. It inherits its listening socket on file descriptor 3 and takes its Bearer token from `-m7h-sfu-token` (or from file descriptor 4). Do not put the token on the command line in shared logs: any process on the same machine can read it from `ps`.

### Capacity flags (`-mode m7h-sfu`)

| Flag | Default | Meaning |
|---|---|---|
| `-auto-capacity` | true | Detect the device at startup and use it for the default limits. `false` uses the highest measured anchor instead. |
| `-max-publishers` | 0 | Maximum concurrent publishers. 0 means use the auto-detected limit. An explicit number is never lowered. An explicit 0 means unlimited. |
| `-max-subscribers` | 0 | Maximum concurrent viewers. Same rules as `-max-publishers`. |

Check the detector's answer without starting a server: `./hami-media capacity`. It prints the device, the recommended limits, the scale factor, and the anchors. It takes no arguments.

## Token flags (`-mode mint-token`)

| Flag | Default | Meaning |
|---|---|---|
| `-token-room` | `HAMI_SFU_ROOM` or `default` | The room the token is for. |
| `-token-roles` | `publish,subscribe` | Roles in the token. |
| `-token-subject` | — | Who the token is for. Required. |
| `-token-ttl` | 1 h | How long the token lives. |

## TURN flags (`-mode turn-server`)

| Flag | Default | Meaning |
|---|---|---|
| `-turn-listen` | `HAMI_SFU_TURN_LISTEN` or `127.0.0.1:3478` | Where the TURN server listens. |

## Environment variables

| Variable | Meaning |
|---|---|
| `HAMI_SFU_HMAC_KEY_FILE` | Path to a file holding the HMAC key. Preferred. File should be `0600`. |
| `HAMI_SFU_HMAC_KEY` | The HMAC key itself. Set only one of this and the file. |
| `HAMI_SFU_ROOM` | The room this server process serves. Default `default`. |
| `HAMI_SFU_TURN_URL` | TURN URL clients use. Default `turn:127.0.0.1:3478`. |
| `HAMI_SFU_TURN_USERNAME` | TURN username. |
| `HAMI_SFU_TURN_PASSWORD` | TURN password. |
| `HAMI_SFU_TURN_PASSWORD_FILE` | Path to a file holding the TURN password. Preferred. |
| `HAMI_SFU_TURN_REALM` | TURN realm. Default `hami`. |
| `HAMI_SFU_TURN_LISTEN` | Where `-mode turn-server` listens. Default `127.0.0.1:3478`. |
| `HAMI_SFU_EGRESS_SHARDS` | Egress shard count, 0–32. 0 is the default. |
| `GOMEMLIMIT` | Go heap limit. If unset, the binary sets a 4 GiB default. |
| `HAMI_HEAP_PROFILE_PATH` | Write a heap profile when heap use passes `HAMI_HEAP_PROFILE_AT_BYTES` (default 512 MiB). |
| `HAMI_CPU_PROFILE_PATH` | Write a CPU profile, starting after `HAMI_CPU_PROFILE_DELAY` (default 90 s) for `HAMI_CPU_PROFILE_WINDOW`. |
| `HAMI_READQUEUE_SAMPLE` | Set to `1` to sample the read queue (diagnostics only). |

The HMAC key must be at least 32 characters. The server refuses to start without it. The key is never printed or logged.

## Heap and memory

- The binary sets a 4 GiB Go heap limit when `GOMEMLIMIT` is unset. A bare run cannot grow without limit.
- `-mode sfu-single` and `-mode m9h` pick a heap budget from the viewer count unless you pass `-max-heap-bytes`. The run stops cleanly when the budget is hit.
- The host health check wants 2 GiB of free swap, or 8 GiB of free memory when the machine has no swap pool at all (as after a reboot), and 10 GiB of free disk. See [capacity.md](capacity.md).
