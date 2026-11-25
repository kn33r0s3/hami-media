#!/bin/sh
# Job 15.5 — One benchmark runner.
# One command runs the benchmarks with host preflight and writes a
# JSON result with trial IDs, code sha, hardware record and raw logs.
# Usage: scripts/benchmark-runner.sh [--benchtime 1x] <outdir>
# Output: <outdir>/benchmark-<trial-id>.json, <outdir>/benchmark-<trial-id>.log,
#         <outdir>/hardware.json. Trial ID: bench-YYYYMMDD-HHMMSS-<short-sha>.
set -eu
BENCHTIME="${BENCHTIME:-1s}"
OUTDIR="${1:-results/benchmarks}"
mkdir -p "$OUTDIR"
SHA="$(sha256sum main.go | cut -d' ' -f1)"
SHORT_SHA="$(printf '%s' "$SHA" | cut -c1-8)"
TRIAL_ID="bench-$(date -u +%Y%m%d-%H%M%S)-$SHORT_SHA"
export PREFLIGHT_LOG="$OUTDIR/$TRIAL_ID.preflight.log"
# Host preflight first (job 18.1); it refuses on a failing host.
./scripts/host-preflight.sh -- go test -bench . -benchtime "$BENCHTIME" -run '^$' -count=1 -v . > "$OUTDIR/$TRIAL_ID.log" 2>&1
STATUS=$?
./scripts/record-hardware.sh "$OUTDIR/hardware.json" >/dev/null
python3 - "$OUTDIR" "$TRIAL_ID" "$SHA" "$STATUS" <<'PY'
import json, os, re, sys
outdir, trial, sha, status = sys.argv[1], sys.argv[2], sys.argv[3], int(sys.argv[4])
log = open(f"{outdir}/{trial}.log").read()
benches = {}
for m in re.finditer(r"^(Benchmark\w+)-\d+\s+(\d+)\s+([\d.]+) ns/op\s+([\d.]+) B/op\s+([\d.]+) allocs/op", log, re.M):
    benches[m.group(1)] = {"iterations": int(m.group(2)), "ns_per_op": float(m.group(3)),
                           "bytes_per_op": float(m.group(4)), "allocs_per_op": float(m.group(5))}
hardware = json.load(open(f"{outdir}/hardware.json"))
doc = {"trial_id": trial, "trial_ids": [trial],
       "code_sha256_main_go": sha, "image_hash": "n/a-native",
       "parameters": {"benchtime": os.environ.get("BENCHTIME", "1s"), "count": 1},
       "status": "pass" if status == 0 else "fail",
       "exit_code": status, "benchmarks": benches, "hardware": hardware,
       "raw_log": f"{trial}.log", "raw_logs": [f"{trial}.log", f"{trial}.preflight.log"],
       "preflight_log": f"{trial}.preflight.log",
       "schema": "hami-media-benchmark/1"}
with open(f"{outdir}/{trial}.json", "w") as f:
    json.dump(doc, f, indent=2, sort_keys=True)
print(f"{trial}: status={doc['status']} benchmarks={sorted(benches)}")
PY
exit "$STATUS"
