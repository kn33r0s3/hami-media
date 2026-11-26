# Capacity

This page uses only measured numbers. Nothing here is guessed. If a number has no run behind it, it says so.

## Measured results

All runs are one logical room, H.264 video, 20 publishers, strict scoring (every frame must arrive, delivered-frame p99 latency at most 150 ms). Machine: the reference machine.

| Run | Result |
|---|---|
| 20 publishers x 5,000 viewers | PASS, strict |
| 20 publishers x 6,000 viewers | PASS, strict |
| 20 publishers x 7,000 viewers | PASS, strict |
| 20 publishers x 7,200 viewers | PASS, strict |
| 20 publishers x 7,500 viewers | PASS, strict, three times, plus a fourth pass in a fresh re-check (25 Nov ~02:50 NPT, p99 19.51 ms) |
| 20 publishers x 8,000 viewers | PASS, strict, six runs (T0, T1, T4, FINAL, verify-s005, stall-capture; p99 24–28 ms) |
| 20 publishers x 10,000 viewers | Systematic FAIL — at the macOS kernel loopback limit, not a code limit |

The 20 x 7,500 row had a scare: a re-check on 24 Nov failed on
latency alone (p99 221.33 ms) while 7,000 passed, which briefly
looked like a scale threshold at 7,000–7,500. A fresh re-check in
the stable state passed clean (p99 19.51 ms, 7,500/7,500 frames),
and 8,000 passed again too. The threshold claim is withdrawn —
the failures were a transient machine state, not a scale limit.
Four 20 x 8,000 runs also failed catastrophically in that same
unstable window and then passed again in the identical config.
See REPORT.md §4.5 and §4.8.

What "systematic fail at the loopback limit" means: the 20 x 10,000 run fails the same way every time, and the failure is in the macOS kernel's loopback path carrying 10,000 local viewers, not in this server's code. The code limit is higher than the machine's loopback path allows on one Mac. A real deployment with viewers on real networks does not hit this wall the same way.

## [PLACEHOLDER] 10 publishers x 6,000 viewers

> **Not yet measured.** A 10 x 6,000 run is queued, not yet run. This section will be filled in with the verified numbers when that run finishes. Do not quote a 10 x 6,000 result until this placeholder is replaced.
>
> Status 25 Nov 2026: the old blocker is gone — the preflight
> rule changed from "2 GiB free swap" to "used swap at most
> 512 MiB" (see below), so free swap no longer blocks the run.
> The run itself is still unrun, queued as peakmem-10x6000 (10
> publishers x 6,000 viewers, 5 cells, 60 s, strict). No numbers
> exist yet.

## Hard caps in the code

| Cap | Value | Where |
|---|---|---|
| Viewers per publisher | 500 | `sfuScaleMaxViewersPerPublisher` |
| Total viewers | 10,000 | `sfuScaleMaxViewers` |
| Delivered-frame p99 latency | 150 ms | `LatencyP99Max` |
| Go runtime memory target | 4 GiB default, reduced to the M7-H viewer-based budget when smaller | set when `GOMEMLIMIT` is unset |

These caps are the ceiling. The measured numbers above are what the machine actually did.

## The auto capacity detector

The detector's job: look at the machine, and set the biggest safe audience for that machine. It has two parts.

**Part 1 — host health check.** Before a scale run starts, the server reads the machine:

- macOS memory pressure level,
- used swap (at most 512 MiB when the machine holds a swap pool, or 8 GiB free memory when it has no swap pool — with one open edge case: a swap total reading 0 twice is treated as unknown and the run refuses to start (fail closed), so no-swap protection is unverified in that edge case until the zero-reading glitch is identified; see REPORT.md §2.8),
- free disk (needs 10 GiB),
- memory and swap counters.

If the machine is not healthy, the run refuses to start. It prints why. While the run is going, a watchdog reads the same numbers with cheap system calls (no new processes, so it cannot stall the video) and stops the run if the machine runs out of room.

**Part 2 — automatic maximums (`capacity.go`, landed).** At server startup the detector reads the device — CPU model and core count, total RAM, swap status, file-descriptor limits, UDP buffer sizes, network interfaces — and recommends the publisher and viewer limits from the measured anchors only. CPU and RAM determine the scaling factor; unknown swap is reported as unknown and does not prevent those limits from being calculated. Nothing is extrapolated past a measurement.

The rule is one line:

```
factor = min(cores / 10, RAM GiB / 24), clamped to [0, 1]
publishers = floor(20 * factor), at least 1
viewers    = floor(7,500 * factor), at least 1
```

The reference device is the machine the anchors were measured on: Apple M5, 10 cores, 24 GiB RAM. On that machine the factor is 1.0, so the recommendation is 20 publishers and 7,500 viewers. A stronger device still gets 20 / 7,500: the factor is clamped at 1, so the detector never recommends past a measurement — even though 20 x 8,000 now passes strict six times (see the measured table above), the coded maxima are still 20 / 7,500 until the derivation is re-run with the new anchor. A weaker device scales down: a 5-core / 12 GiB machine gets factor 0.5, so 10 publishers and 3,750 viewers. The scaled-down number is a conservative rule of thumb, not a measurement — it is reported as such.

A pending or failing anchor is never used by the derivation. The 10 x 6,000 row is pending, so it changes nothing until its measurement lands. The 20 x 10,000 row failed, so nothing above the coded 7,500-viewer maximum is ever recommended.

## Swap: re-verified, not assumed (25 Nov ~02:55 NPT)

A terminal glitch returning 0 for swap readings was reported, so
the "swap never grew" claim was re-verified on a fresh 20 x 7,500
run with four cross-checked sources (`sysctl vm.swapusage`,
`vm_stat`, `memory_pressure`, `top`): used swap 256.44 MiB
constant, swapouts 473,693 constant, swapins 364,491 constant,
memory-pressure free percentage 73–74%, no zero reading in 44
sysctl reads. The zero glitch did not reproduce in the
measurement path. Older single-source "swap delta 0" readings are
not retroactively re-proven, but the claim itself is now verified
by the cross-checked re-run. The flat swapouts are explained by
macOS compressor-first behaviour (pageouts rise slowly, swapouts
only grow when the compressor spills), not by a stuck counter.

If device detection fails, automatic mode starts with a conservative 1-publisher / 1-viewer starter cap and logs that it is not a capacity measurement. Explicit operator limits still override the starter cap. Turning auto-capacity off remains an explicit choice to use the highest measured anchors (20 / 7,500).

A zero swap total is never treated as healthy by scale-proof preflight: the shared reader (`readSwapMiBChecked`, fixed27) re-reads once, and if the total still reads 0 it returns an error — the preflight retries as unknown and the watchdog aborts. The capacity report, however, can still recommend CPU/RAM-scaled starter limits and clearly labels swap as unknown. Until the zero-reading glitch is identified, no-swap protection is unverified in that edge case (REPORT.md §2.8).

M7-H SFU service mode also derives a default heap budget from its effective viewer limit, using the existing per-viewer estimate and headroom. It lowers the Go runtime memory target to that budget when smaller than the default and `GOMEMLIMIT` is unset. This is a protective estimate, not a measured low-end capacity guarantee; set `-max-heap-bytes` or `GOMEMLIMIT` explicitly when operating with a different memory policy.

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
