# 24-hour soak test (15.8)

Job 15.8: memory and goroutine trends stay inside the agreed limit
over 24 hours.

Status: **OPEN — staged, not passed.** The script exists and the
pilot harness is proven; a full 24 h run cannot complete in-session
and cannot run in this VM at all (see the pilot result below).

## The script

`scripts/soak.sh <outdir>` (`DURATION`, default `24h`; `VIDEO`,
default `test.h264`). It runs a small sfu-single proof (2 publishers
x 50 viewers) for the duration and samples RSS + thread count from
`/proc/<pid>/status` every 60 s into `<run-id>.samples.tsv`, with the
run's stdout/stderr logs beside it.

## Agreed limits

- RSS growth **<= 10%** over the run, measured after the first
  10-minute warm-up.
- Goroutine/thread count stable within **+/-5%** of the warm-up
  median. (Threads from `/proc` are the available proxy; goroutine
  counts come from the run's stderr heap samples.)
- Zero `ABORTED-BY-HOST`, zero heap-budget aborts.

## Pilot run (25 Nov 2026, VM)

- Command: `DURATION=60s VIDEO=/tmp/test-e.h264 ./scripts/soak.sh
  results/soak-pilot`
- Preflight: READY (mem available 953 MiB, swap used 350 MiB,
  free disk 109 GB, load 1.74).
- Samples: RSS 9,948 KiB -> 17,048 KiB, threads 6 -> 7 (2 samples).
- Outcome: process exit 1 after the 60 s window — the publisher
  pairs never connected: ICE candidates failed, sent=0 received=0.
  That is the known VM sandbox UDP block, not a soak finding.
- Verdict: **pilot proves the harness (preflight, sampling, exit
  capture); it is NOT a soak pass.** Raw files:
  `results/soak-pilot/soak-20261125-083047.samples.tsv`,
  external stdout log (sha256 `0682b15fc048`, 1 line), external
  stderr log (sha256 `a01c92e5a366`, 156 lines), and
  `results/soak-pilot/soak-20261125-083047.hardware.json`.

## Owed

- A 24 h run on the reference machine (or any host where loopback
  UDP works), with its samples file and a limits check against the
  numbers above.
