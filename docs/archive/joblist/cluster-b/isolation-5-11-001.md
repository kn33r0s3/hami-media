# Job 5.11 — Isolation test — DONE

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdict: DONE (router-level; Mac scale leg OPEN)

## Stated tolerance

Every other participant's forward p99 (enqueue→write) must stay
within **2× the clean-run p99, with a 1 ms floor** for scheduler
noise, and within the **50 ms stall-gate bound** in absolute terms.
The floor is a noise allowance, not a performance bound.

## Harness (`isolation_test.go`)

`isolationViewer` builds a viewer the way the router does (real
PeerConnection + downtrack + `sfuSubscription` with production
queues) and drains its RTP queue with a recording goroutine that
does exactly what nackWorker does for the queue
(`forwardQueuedRTP` + release the copy), recording enqueue→write
latency per packet. `isolationDrive` is the test's forwardTrack:
`rememberPacket` once, then per subscription the exact
forwardTrack direct-branch sequence (layer filter, `enqueueRTP`,
drop bookkeeping + `recordQueueFullDrop`, success reset). The
stream is 60 frames × 5 packets = 300 packets at 1 ms pacing,
frame 0 keyframe, the rest delta, 1,200-byte payloads. No UDP is
involved, so it runs in the VM. Each test runs a clean phase
first (its own baseline p99), then the stall phase on fresh
viewers, and compares.

## Stalled viewer (`TestIsolationStalledViewer`)

Viewer 0's drain sleeps 200 ms per packet. The stall is real:
its queue fills, the 4.7 policy steps it down once
(`layerStepDowns=1`; it stops at layerKey because the stream's 5
keyframe packets all precede the stall, so no further strikes
accumulate), `queueDropped=8`, and it writes **1 of 300 packets**.
The four healthy viewers: clean p99 50–62 µs, stall-run p99
93–95 µs — within the 1 ms tolerance. (Under `-race`: clean
382–834 µs, stall-run 432–728 µs, tolerance 1.67 ms — passes.)

## Stalled publisher (`TestIsolationStalledPublisher`)

Two publishers, 3 viewers each. Publisher A's driver sends 10
packets, then blocks forever (the forwardTrack-blocked-in-ReadRTP
shape). The stall is real: A's viewers write exactly 10 each.
Publisher B's viewers: clean p99 46–49 µs, stall-run p99 60–61 µs
— within the 1 ms tolerance. (Under `-race`: clean 141–182 µs,
stall-run 98–116 µs on repeats; one first-run outlier at 803 µs
vs the 1 ms floor — scheduler/instrumentation noise, not a
coupling: the repeats sit at or below the clean p99.)

Raw log: `log (sha256 fe359bb65125, 26 lines; kept outside repo)` (race run).

## What this does NOT prove (Mac leg OPEN)

The forward path here is the direct branch with an unbound
downtrack (`WriteRTP` is a no-op). It proves the router's fan-out,
queues, and policy do not couple participants. It does not prove
isolation through a real network write (a slow socket blocking a
shared shard `run()` loop — the 5.9 head-of-line note — or a
shared ICE/DTLS path) or at scale. Mac leg: a scale proof with
one throttled viewer (a viewer that never drains) and one stalled
publisher, asserting the same tolerance on the measured
per-viewer p99s from the result JSON.
