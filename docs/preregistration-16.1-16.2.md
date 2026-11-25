# Preregistration: matched benchmark (16.1) and win bar (16.2)

Status: **PREREGISTERED 25 Nov 2026** — committed before the first
comparison run (rule R6). The machine-readable form is
`docs/benchmark-preregistration.json`; this file is the human record.
**The win bar is never changed after results come in.** If the ladder
cannot be run as specified, the run is recorded as NOT RUN, not
redefined to fit what was possible.

## 16.1 — Matched benchmark

- Top layer: **720p30 at about 1.2 Mbps**.
- Keyframes: **every 2 s**, plus one sensitivity run at LiveKit's
  about **3 s** (LiveKit's keyframe interval is the sensitivity
  dimension; the 2 s run is the primary comparison).
- Ramp: **5 subscribers per second**.
- Load generator: **on a separate machine** from the server under test.
- Both servers as **native processes** (no containers).
- Ladder: **100, 500, 1,000, 2,000, 5,000 subscribers, three runs per
  step**, continuing upward until the first failed step. Raw files for
  every run, failures included.
- Gates per run: **p99 <= 150 ms, zero lost frames**.

## 16.2 — Win bar

hami-media wins if **either** leg holds, measured the same way for
both servers (same hardware class, same network, same load generator,
same clip, same gates):

1. at least **2x less CPU and RSS per subscriber per Mbps**, or
2. at least **2x more viewers at p99 <= 150 ms**.

## Pins (frozen with this preregistration)

- LiveKit: **v1.13.11** (binary sha256
  `1e9ee90ca301e1842b2887fc77cb122759756a9c8aeb4334ee91de3325ec43fb`,
  linux amd64 tarball; smoke test in `docs/archive/livekit-install-16.3.md`).
- hami-media: `main.go` sha256
  `9b253fafc2a73a251101fd4435c98ec2c6e17b0f07d94594b6d918b2294fd19a`,
  upstream Pion v4.2.26.

## Status of the ladder itself

- **16.4 BLOCKED**: no second machine exists for the load generator,
  and the VM sandbox blocks unconnected UDP, so the VM cannot be the
  load generator either. Recorded, not faked.
- **16.8 BLOCKED**: second hardware class + real network; no second
  machine.
- The comparison generator (16.6, `scripts/compare-livekit.py`) is
  written and tested against existing hami-media raw files; with the
  LiveKit side missing it reports UNDECIDED, never a win.
