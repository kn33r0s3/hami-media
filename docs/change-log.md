# VM-first change log (job 0.2)

Rule: every synced change lists the VM run IDs that passed before
the sync. A change with no passing VM run listed here is not synced.
Run IDs resolve through the external J7 log manifest; raw logs are kept
outside the repository and are not part of a clone.
or the working notes they cite. No host names appear in this log.
Historical log references that could not be recovered are listed in
[the archive evidence register](archive/log-evidence-status.md).

## 2026-11-25 — Cluster A: job-list sections 0, 1, 2

Working copy: `joblist-tree-a`, `main.go` sha256
`9b253fafc2a73a251101fd4435c98ec2c6e17b0f07d94594b6d918b2294fd19a`
at start. Go 1.27.4 (linux/amd64), module Go 1.24, Pion v4.2.26.

| Change | VM runs that passed before sync |
|---|---|
| Baseline suite (pre-change): full `go test ./...`, 150 pass / 6 fail — the 6 are the known sandbox UDP blocks, not code | `docs/archive/joblist/cluster-a/baseline-vm-2026-11-25.md` (raw: `/tmp/clusterA-baseline-verbose.log`, package time 122.267 s) |
| Section-2 fixes 2.1–2.10 (SRTP replay restored, upstreamQueue close, pcLifetimes delete, stored track keys, stale-publisher discard, layer keys, debug track-ID removal, Linux swap-reader decision, H.264 picture grouping, egress-shard start-after-validation) | Fail-before: `log (sha256 46de68e8d2a8, 62 lines; kept outside repo)`, `log (sha256 eb85e7f06364, 8 lines; kept outside repo)`, `log (sha256 7667c9b09ebd, 31 lines; kept outside repo)`. Pass-after: `log (sha256 f4f87c68b338, 52 lines; kept outside repo)` (17/17 new tests pass) |
| Post-fix full suite + race | `log (sha256 7b19b53445e9, 82 lines; kept outside repo)`, `log (sha256 04292a7ccc3b, 85 lines; kept outside repo)` (see 1.5/1.6 entries below) |
| Governance docs 0.1–0.7, repo docs 1.1–1.8 | Leak scan + `gofmt` + `go vet` clean; see `log (sha256 6b7ebb042dea, 1 lines; kept outside repo)` |
| Follow-up fixes found by the `test-all.sh` smoke run (same day) | `TestEgressShardStopsWhenStopped` hardened (poll instead of fixed 100 ms sleep — it failed once under full-suite load); `docs/canonical-repo.md` reworded so the leak scan passes (it quoted the old names); re-verified by `log (sha256 0e26522b1759, 31 lines; kept outside repo)` and a refreshed `log (sha256 f4f87c68b338, 52 lines; kept outside repo)` (17/17 PASS) |

Sync status: **not synced**. This tree is a VM working copy; the
coordinator merges it and the owner syncs it. Nothing in this log
claims a Mac run.

## Format for later entries

```
## YYYY-MM-DD — <short title>
Working copy / code sha / Go + Pion versions.
| Change | VM runs that passed before sync |
| ... | <evidence file + run ID> |
Sync status: not synced / synced <where, when>.
```
