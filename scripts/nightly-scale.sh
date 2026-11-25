#!/bin/sh
# Job 15.4 — Nightly scale tests with the section 16 gates.
# Staged for cron (see docs/nightly-scale.md). A 24 h-scale run cannot
# complete in-session; this script is the nightly job itself: it runs
# the benchmark runner, then the small scale ladder that fits in a
# night window, stores every raw file under
# results/nightly/<YYYY-MM-DD>/, and validates each result with the
# 15.6 schema validator. It never overwrites a previous night.
# Usage: scripts/nightly-scale.sh [results-root]  (default results/nightly)
set -eu
ROOT="${1:-results/nightly}"
NIGHT="$(date -u +%Y-%m-%d)"
OUT="$ROOT/$NIGHT"
[ -e "$OUT" ] && { echo "nightly: $OUT already exists; refusing to overwrite" >&2; exit 2; }
mkdir -p "$OUT"
export PATH="$HOME/go/bin:$PATH"
./scripts/benchmark-runner.sh "$OUT/benchmarks" || echo "nightly: benchmark leg failed (raw logs kept in $OUT/benchmarks)"
# Small ladder leg: 2 publishers x 50 viewers, 60 s, strict gates.
# The full ladder (16.4) needs a second machine and is NOT run here.
if [ -f test.h264 ]; then
  ./scripts/host-preflight.sh -- ./hami-media -mode sfu-single -video-file test.h264 \
    -sfu-publishers 2 -sfu-viewers-per-publisher 50 -duration 60s \
    > "$OUT/ladder-2x50.json" 2> "$OUT/ladder-2x50.stderr.log" || echo "nightly: ladder leg failed (raw logs kept)"
fi
./scripts/record-hardware.sh "$OUT/hardware.json" >/dev/null
ls -l "$OUT" | tee "$OUT/MANIFEST.txt"
echo "nightly: raw files stored in $OUT (never overwritten; see docs/nightly-scale.md)"
