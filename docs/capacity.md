# Capacity

This page uses only measured numbers. Nothing here is guessed. If a number has no run behind it, it says so.

## Measured results

All runs are one logical room, H.264 video, 20 publishers, strict scoring (every frame must arrive, delivered-frame p99 latency at most 150 ms). Machine: the developer's MacBook Pro.

| Run | Result |
|---|---|
| 20 publishers x 5,000 viewers | PASS, strict |
| 20 publishers x 7,500 viewers | PASS, strict, three times |
| 20 publishers x 10,000 viewers | Systematic FAIL — at the macOS kernel loopback limit, not a code limit |

What "systematic fail at the loopback limit" means: the 20 x 10,000 run fails the same way every time, and the failure is in the macOS kernel's loopback path carrying 10,000 local viewers, not in this server's code. The code limit is higher than the machine's loopback path allows on one Mac. A real deployment with viewers on real networks does not hit this wall the same way.

## [PLACEHOLDER] 10 publishers x 6,000 viewers

> **Not yet measured.** A 10 x 6,000 run is in progress. This section will be filled in with the verified numbers when that run finishes. Do not quote a 10 x 6,000 result until this placeholder is replaced.
>
> Status 24 Nov 2026: BLOCKED — free swap on the Mac is 1,154 MiB, below the 2,048 MiB preflight floor. The run never started, so no numbers exist.

## Hard caps in the code

| Cap | Value | Where |
|---|---|---|
| Viewers per publisher | 500 | `sfuScaleMaxViewersPerPublisher` |
| Total viewers | 10,000 | `sfuScaleMaxViewers` |
| Delivered-frame p99 latency | 150 ms | `LatencyP99Max` |
| Heap | 4 GiB default | set when `GOMEMLIMIT` is unset |

These caps are the ceiling. The measured numbers above are what the machine actually did.

## The auto capacity detector

The detector's job: look at the machine, and set the biggest safe audience for that machine. It has two parts.

**Part 1 — host health check.** Before a scale run starts, the server reads the machine:

- macOS memory pressure level,
- free swap (needs 2 GiB free, or 8 GiB free memory when the machine has no swap pool),
- free disk (needs 10 GiB),
- memory and swap counters.

If the machine is not healthy, the run refuses to start. It prints why. While the run is going, a watchdog reads the same numbers with cheap system calls (no new processes, so it cannot stall the video) and stops the run if the machine runs out of room.

**Part 2 — automatic maximums (`capacity.go`, landed).** At server startup the detector reads the device — CPU model and core count, total RAM, swap, file-descriptor limits, UDP buffer sizes, network interfaces — and recommends the publisher and viewer limits from the measured anchors only. Nothing is guessed, and nothing is extrapolated past a measurement.

The rule is one line:

```
factor = min(cores / 10, RAM GiB / 24), clamped to [0, 1]
publishers = floor(20 * factor), at least 1
viewers    = floor(7,500 * factor), at least 1
```

The reference device is the Mac the anchors were measured on: Apple M5, 10 cores, 24 GiB RAM. On that machine the factor is 1.0, so the recommendation is 20 publishers and 7,500 viewers — the highest passing anchor. A stronger device still gets 20 / 7,500: the factor is clamped at 1, so the detector never recommends past a measurement. A weaker device scales down: a 5-core / 12 GiB machine gets factor 0.5, so 10 publishers and 3,750 viewers. The scaled-down number is a conservative rule of thumb, not a measurement — it is reported as such.

A pending or failing anchor is never used by the derivation. The 10 x 6,000 row is pending, so it changes nothing until its measurement lands. The 20 x 10,000 row failed, so nothing above 7,500 viewers is ever recommended.

If device detection fails, the server does not invent numbers. It falls back to the highest measured anchors (20 / 7,500) and logs a warning saying so.

See your own machine's answer with no server running:

```sh
./hami-media capacity
```

It prints the device, the recommended maximum publishers and viewers, the scale factor, and the anchors the recommendation came from. It takes no arguments.

## How to override limits

Your flags beat the detector, and they are never silently lowered:

```sh
# Set your own server limits (m7h-sfu server). An explicit number is
# kept as-is, even if it is above what the detector recommends.
./hami-media -mode m7h-sfu -max-publishers 10 -max-subscribers 6000

# Turn the detector off: limits come from the highest measured anchor.
./hami-media -mode m7h-sfu -auto-capacity=false

# Explicitly passing 0 means unlimited for that limit.
./hami-media -mode m7h-sfu -max-publishers 0 -max-subscribers 0
```

For scale proof runs, the same idea applies:

```sh
# Fewer viewers than the cap: fine, the run just does less.
./hami-media -mode sfu-single -video-file test.h264 \
  -sfu-publishers 10 -sfu-viewers-per-publisher 100 -duration 65s

# Skip the host health check. Only do this if you know the machine
# can take the run. A run that starts unhealthy usually fails loudly.
./hami-media -mode sfu-single -video-file test.h264 \
  -sfu-publishers 20 -sfu-viewers-per-publisher 500 \
  -allow-unsafe-host -duration 65s
```

You cannot override the hard caps (500 per publisher, 10,000 total) with a flag. Changing them means changing the code, and re-measuring.

## What "strict" means

Strict scoring: every frame must arrive, and the p99 delivered-frame latency must be at most 150 ms. The `dissolve` scoring mode allows a small fraction of late frames (default 1%, `-sfu-max-dissolved`) up to the same p99 cap. The measured results above are all strict.

## What is not measured yet

- Viewers on real networks instead of loopback. All big runs so far are loopback on one Mac.
- A second machine. The 20 x 10,000 wall is a one-machine loopback wall. It says nothing about two machines.
- The 10 x 6,000 run. See the placeholder above.
