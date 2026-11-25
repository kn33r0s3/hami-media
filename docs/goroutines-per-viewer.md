# Goroutines per viewer — design decision

Date: 25 Nov 2026. Job 5.5.

## Decision: KEEP the per-PC model, and KEEP cell-sharding

Hami-media keeps one Pion PeerConnection per viewer (two PCs per
viewer in the full path: the viewer client + the router's
serverPeer), and scales by sharding viewers across worker processes
(`-sfu-cells`, FIX 25). No attempt is made to reduce goroutines by
sharing a PC across viewers or by disabling Pion's per-PC machinery.
The numbers below are why: the per-PC cost is dominated by Pion's
transport machinery, the configurable savings are small, and the
remaining floor still exceeds a single process's budget at 5,000
viewers — so the scaling answer is processes, not fewer goroutines
per PC.

## Measured numbers

| figure | value | source |
|---|---|---|
| Goroutines in one ladder cell | **75,184** | `stall-evidence-001.md` (20×5,000 cell) |
| Per viewer in that cell | **30.07** | same |
| Per-viewer scaling law | **~35** | `goroutine-inventory-001.md`: 2×250 → 20×250 delta (175,767 − 17,583 = 158,184 for 4,500 viewers; base ≈ 8) |
| Per PC, live connection | **~16** | inventory: 44 PCs = 40 viewer PCs + 2 publisher-client + 2 router publisherPeer PCs |
| Per PC, bare (VM lower bound) | **4.00** | `TestGoroutinesPerPeerConnectionLowerBound` (`goroutines_test.go`): 20 PCs with AddTrack, never negotiated — baseline 2 → 82, delta 80. No ICE/DTLS/SRTP, no media flowing, so this is a LOWER BOUND; the live ~16/PC adds the transport goroutines. No leak: count returns to 2 after Close. |
| Per-PC floor after config cuts | **~11** | inventory: disabling RTCP reports + TWCC saves 3/PC (16 → 13); floor ~11/PC → ~25/viewer → ~125,000 at 5,000 viewers single-process |
| Harness per viewer | 3 | inventory: `main.subscribe` 2 + viewer ReadRTP loop 1 |

## Per-PC breakdown (live, from the inventory)

Per PC: RTCP report writers 2, `undeclaredMediaProcessor` 2,
DTLS handshake 2, SRTP start 2, TWCC writer 1, ICE start 1,
`taskloop` 1, `context.propagateCancel` 1, connectivity checks 1,
mux 1, `netctx.ReadFromContext` 1, `ice.GetConn` ~0.5. NACK writer
only on generator PCs (22 of 44). No SCTP goroutines (no data
channels). Per publisher: `forwardTrack` 1 + `sendPublisherVideo` 1.

## Why config cuts cannot reach the target

Disabling RTCP reports + TWCC saves 3/PC → 13/PC → 29/viewer →
~145,000 at 5,000 viewers. The per-PC floor (~11/PC) gives
~25/viewer → ~125,000 single-process. The cell budget is ~60,000
goroutines per process. A 2-process split (router | all viewers)
gives the router 80,000 (5,000 serverPeers × 16) — the router alone
must also split, or the viewers must shard AND the router must
shard. FIX 25 (`-sfu-cells N`: the parent spawns N worker
subprocesses, each a complete single-process room for a slice of
tracks; 4 cells × 5 publishers × 250 viewers = 1,250 viewers/cell,
~44,000 goroutines/cell, under 60,000) is the design that fits the
measured numbers. The 75,184 / 30.07-per-viewer cell figure is
consistent with it.

## What would change this decision

- A Pion (or transport) change that removes per-PC goroutines at the
  source — not a config flag, a different transport architecture.
  Out of scope for this job; the decision is revisited only if such a
  change lands.
- A Mac media-window mutex profile (Job 5.1) showing goroutine
  *contention* (not count) as the bottleneck — that is a 5.2 question,
  not a count question, and does not change the per-PC model.
