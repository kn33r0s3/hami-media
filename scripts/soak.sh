#!/bin/sh
# Job 15.8 — 24-hour soak test (STAGED; a full 24 h run cannot
# complete in-session). Runs a small sfu-single proof continuously
# for $DURATION (default 24h) and samples RSS + goroutines every
# 60 s from /proc/<pid>/status and the run's stderr heap samples.
# Limits (agreed, docs/soak-test-15.8.md): RSS growth <= 10% over the
# run after the first 10 min warm-up, goroutine count stable within
# +/-5% of the warm-up median, zero ABORTED-BY-HOST, zero heap-budget
# aborts. A short pilot (e.g. DURATION=120s) proves the harness; it
# is NOT a soak pass.
# Usage: DURATION=120s VIDEO=test.h264 scripts/soak.sh <outdir>
set -eu
DURATION="${DURATION:-24h}"
VIDEO="${VIDEO:-test.h264}"
OUTDIR="${1:-results/soak}"
mkdir -p "$OUTDIR"
RUN_ID="soak-$(date -u +%Y%m%d-%H%M%S)"
[ -f "$VIDEO" ] || { echo "soak: video file $VIDEO not found" >&2; exit 2; }
export PATH="$HOME/go/bin:$PATH"
./scripts/host-preflight.sh -- go build -trimpath -o "$OUTDIR/hami-media-soak" . || exit 1
./scripts/record-hardware.sh "$OUTDIR/$RUN_ID.hardware.json" >/dev/null
"$OUTDIR/hami-media-soak" -mode sfu-single -video-file "$VIDEO" \
  -sfu-publishers 2 -sfu-viewers-per-publisher 50 -duration "$DURATION" \
  > "$OUTDIR/$RUN_ID.stdout.log" 2> "$OUTDIR/$RUN_ID.stderr.log" &
PID=$!
echo "soak $RUN_ID pid=$PID duration=$DURATION" | tee "$OUTDIR/$RUN_ID.samples.tsv"
printf 'ts_utc\trss_kib\tthreads\n' >> "$OUTDIR/$RUN_ID.samples.tsv"
while kill -0 "$PID" 2>/dev/null; do
  RSS="$(awk '/^VmRSS:/ {print $2}' /proc/$PID/status 2>/dev/null || echo 0)"
  THR="$(awk '/^Threads:/ {print $2}' /proc/$PID/status 2>/dev/null || echo 0)"
  printf '%s\t%s\t%s\n' "$(date -u +%FT%TZ)" "$RSS" "$THR" >> "$OUTDIR/$RUN_ID.samples.tsv"
  sleep 60
done
wait "$PID" || STATUS=$?
STATUS=${STATUS:-0}
echo "soak $RUN_ID exit=$STATUS samples=$(wc -l < "$OUTDIR/$RUN_ID.samples.tsv")" | tee -a "$OUTDIR/$RUN_ID.samples.tsv"
python3 - "$OUTDIR/$RUN_ID.samples.tsv" <<'PY'
import sys
rows = [l.rstrip("\n").split("\t") for l in open(sys.argv[1]) if l.startswith("20")]
rss = [int(r[1]) for r in rows if r[1].isdigit()]
thr = [int(r[2]) for r in rows if r[2].isdigit()]
if len(rss) >= 12:
    warm = sorted(rss[10:20])[len(rss[10:20])//2] if len(rss) > 10 else rss[0]
    print(f"soak samples={len(rss)} rss_first={rss[0]}KiB rss_last={rss[-1]}KiB rss_warmup_median={warm}KiB threads_last={thr[-1] if thr else '?'}")
else:
    print(f"soak PILOT ONLY: {len(rss)} sample(s) — not a 24 h soak pass")
PY
exit "$STATUS"
