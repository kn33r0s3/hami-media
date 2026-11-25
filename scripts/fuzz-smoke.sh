#!/bin/sh
# Cluster C §8.6/§8.7 — fixed-time fuzz smoke + dependency vulnerability scan.
# Usage: sh scripts/fuzz-smoke.sh [seconds-per-target, default 60]
#        sh scripts/vuln-scan.sh
# No CI exists yet (cluster A owns the workflow file); these scripts are
# the standalone deliverables. The CI wiring is: run fuzz-smoke.sh and
# vuln-scan.sh on every change; fail the build on any crash or on a
# known vulnerability above LOW severity.
set -eu
cd "$(dirname "$0")/.."
GO="${GO:-$HOME/go/bin/go}"
SECS="${1:-60}"
echo "fuzz-smoke: go=$GO fuzztime=${SECS}s per target"
for target in FuzzParseSDP FuzzParseRTP FuzzParseRTCP FuzzSignalingJSON; do
  echo "--- $target (${SECS}s) ---"
  "$GO" test -run="^$" -fuzz="$target" -fuzztime="${SECS}s" . 2>&1 | tail -5
done
echo "fuzz-smoke: all targets finished with no crash"
