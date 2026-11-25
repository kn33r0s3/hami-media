# Known-results register entry — swap-zero reading (18.3)

> MERGED 25 Nov 2026 (job-list merge): the substance of this entry
> now lives in `known-results.md` (job 0.5 register) — the
> consequence line and the VM live-evidence note were appended
> there. This file is kept as the staged source; do not create a
> second 18.3 entry.

Job 18.3 is a [!] item: the mechanism identification needs owner
approval before investigation starts. This file is the register
entry that cluster A (owner of the known-results register, job 0.5)
can append verbatim. It records the item as OPEN, with the measured
facts — it does not identify the mechanism.

## Proposed register entry (for job 0.5)

**18.3 Swap-zero reading — OPEN, cause unknown.**

- Observation: the swap reader sometimes returns total 0 (used 0,
  free 0) for a live swap pool.
- Crosscheck (25 Nov 2026 swap redo): 15 consecutive reads found no
  zero glitch — sysctl total 1,024.00 MiB / used 256.44 MiB identical
  across all 15 reads.
- Guard in force: `readSwapMiBChecked()` (fixed27) re-reads once on
  a zero total and fails closed (returns an error) if it still reads
  zero — a zero total is treated as unknown, never healthy.
  Pinned by `TestReadSwapMiBCheckedZeroTotalFailsClosed`.
- Consequence: until the glitch is identified, the no-pool case is
  UNVERIFIED in the zero-total edge case; a freshly rebooted machine
  with no swap pool sits at `host_preflight=UNKNOWN` (retrying).
- Mechanism: **UNIDENTIFIED**. Investigation needs owner approval
  (job 18.3, [!]); do not start without it.

## What this cluster did

- Did NOT start mechanism identification (no approval).
- Wrote this entry text and verified the guard + test above against
  the current tree (`main.go:671-690`,
  `main_test.go: TestReadSwapMiBCheckedZeroTotalFailsClosed`).
- If the register file exists in the merge, cluster A appends the
  entry; if it does not, this file is the entry until the register
  is created.
