# Environment (job 1.4)

## Pinned versions

- Go: module `go 1.24` (`go.mod`); toolchain used in this VM:
  `go1.27.4 linux/amd64` (`~/go/bin/go`). Pion `v4.2.26`,
  pion/interceptor `v0.1.51`, pion/srtp `v3` (direct require
  since the Section-2 SRTP replay work). Full module versions:
  see `go.mod` / `go.sum`.
- Build: module mode (`GOFLAGS=-mod=mod`). Reproducible builds
  use `-trimpath`; see job 1.3 in
  `docs/archive/joblist/cluster-a/build-reproducibility-2026-11-25.md`.
- Test command: `sh scripts/test-all.sh` (job 1.8) — unit +
  race + integration + fuzz smoke, one summary.

## Honest limit: no scripted VM rebuild in this sandbox

There is **no scripted VM rebuild** in this tree. This sandbox
cannot rebuild its own VM image, so job 1.4 is PARTIAL: the
versions above are pinned and the test command is scripted, but a
reference-image setup script does not exist here. If one is
written later it must be marked UNTESTED until it has built a
fresh VM from scratch and run the suite on it.

## Known VM limits (this sandbox)

- Unconnected UDP sends are blocked (`sendto: operation not
  permitted`); exactly 6 tests fail for that reason, not code —
  see `docs/archive/joblist/cluster-a/baseline-vm-2026-11-25.md`.
- `/tmp` is a 512 MiB tmpfs. The race build's work directory does
  not fit in it — `scripts/test-all.sh` sets
  `GOTMPDIR=$HOME/.cache/go-tmp` for that reason. The first race
  attempt in this round failed to build for exactly this cause
  (see `log (sha256 04292a7ccc3b, 85 lines; kept outside repo)` header note).
