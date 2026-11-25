# Cellular-like profile (job 7.8) — APPROXIMATION, NOT A FIELD TEST

This profile documents bursty loss and handoff patterns for lab use.
It is an approximation built from the in-process impairment pipe
(§7.5). It is NOT a field test: no real phone, no mobile data, no
drive test was run. Job 7.9 (field test on real phones) needs owner
approval and has not started.

## Patterns

| Pattern | Settings (impairment pipe) | What it approximates |
|---|---|---|
| Good LTE | loss 0%, delay 30 ms, jitter 5 ms | Stable cell, close tower |
| Congested LTE | loss 1%, delay 100 ms, jitter 20 ms | Busy cell, queueing |
| Poor LTE / edge | loss 3%, delay 150 ms, jitter 30 ms | Weak signal, retransmits |
| Burst loss | loss 3% with reorder on, 1 s bursts | Fading / interference burst — model as 10× loss for 1 s, then 0% |
| Handoff | 500 ms of 100% loss, then delay jumps 30→150 ms | Cell handover gap; the ICE-restart path (7.4, 5 s deadline) is the recovery mechanism |
| Duplication | duplicate every packet once | Carrier middlebox duplication |

## Measured (VM, in-process, seed 42, n=1000/cell)

From `TestClusterCImpairmentMatrix` (see `log (sha256 50cf5fc6724a, 137 lines; kept outside repo)`):

- loss 0% / delay 30/100/150 ms: 1000/1000 received, max delay = setting.
- loss 1%: 994/1000 received (6 lost).
- loss 3%: 975/1000 received (25 lost).
- jitter 20 ms on 100 ms delay: max delay 120 ms.
- reorder: 500/1000 out of order.
- duplication: 1000 duplicates on 1000 sent.

## What this does NOT prove

- Real radio behaviour (fading, scheduler, carrier NAT).
- Battery/thermal effects on encoding (job 10.5, BLOCKED).
- Behaviour on a real handoff (needs a moving device, job 7.9).

Use this profile to choose layer-switch thresholds (6.5) and the
ICE-restart deadline (7.4), and for nothing else.
