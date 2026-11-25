# Operator guide

Status: written 25 Nov 2026 for job 17.3. Independent review (by
someone who did not write it) is still owed.

## Install

- From source: `go build -trimpath -o hami-media .` (Go 1.24+).
  No binary is tracked in the repo.
- As a service: see `docs/systemd-install-17.2.md` (socket + service
  + Go launcher; token on fd 4, HMAC key from file).
- As a container: see `docs/archive/docker-image-17.1.md` (proof runs only;
  no image build has been done yet — no Docker daemon in the VM).

## Before every run

Run the uniform preflight: `scripts/host-preflight.sh -- <command>`
(job 18.1). It refuses on low free memory (< 512 MiB), used swap
above 512 MiB, or free disk below 10 GiB, and logs one JSON line.
Scale proofs run the in-binary preflight as well
(`waitForSFUHostPreflight`).

## Running a scale proof

```sh
./hami-media -mode sfu-single -video-file test.h264 \
  -sfu-publishers 20 -sfu-viewers-per-publisher 400 -duration 60s
```

- Duration must be at least 60 s for scale proofs.
- The heap cap is automatic: `defaultSFUHeapBudget` (500,000 bytes
  per viewer x2, 256 MiB floor; job 18.5). Override with
  `-max-heap-bytes` only if you know why.
- Do not set `GOMEMLIMIT` below the run's needs without evidence:
  the 4 GiB / 8 GiB catastrophic runs were later shown to be a
  machine-state change (U10), not a heap-cap effect (REPORT.md §4.5).
- The run prints a JSON result to stdout and diagnostics to stderr.
  Keep both: every result records code sha, parameters, trial IDs
  and raw logs (rule R4).

## Capacity

`./hami-media capacity` prints the detected device and the
recommended limits. Flags beat the detector. The detector never
recommends past a measurement. See `docs/capacity.md`.

## Monitoring

- `/health` returns `{"status":"ready"}` (bearer-token protected;
  it does not check dependencies — job 14.4 is partial).
- `/metrics` (loopback-only, `-metrics-listen 127.0.0.1:9464`)
  exposes Prometheus `hami_sfu_rtp_*` metrics.
- Heap budget samples are logged to stderr (`heap budget sample:`).
- If the host degrades mid-run the watchdog aborts with
  `ABORTED-BY-HOST`, not a capacity failure (job 18.4).

## If a run fails

1. Read the JSON result's `error` / `status` first.
2. `ABORTED-BY-HOST` means the host failed (swap, pressure, disk) —
   fix the host, do not re-tune the code.
3. Frame-short failures with 0 NACKs / 0 drops mean the frame never
   left the publisher — see REPORT.md §4.5 (T2/T3) before changing
   GC settings.
4. Record the failure in the known-results register (job 0.5) with
   its conditions; never delete a failed raw file.
