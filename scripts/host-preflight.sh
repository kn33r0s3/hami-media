#!/bin/sh
# Job 18.1 — Uniform host preflight before every run.
# One call, one log line, every run mode. Checks memory pressure,
# free swap, disk space and other load, and logs the result as a
# single JSON line to stderr (and to $PREFLIGHT_LOG when set) before
# the wrapped command runs. Refuses to run the command when the host
# fails, unless --allow-unsafe-host is passed (logged as OVERRIDDEN).
# The binary's own scale-proof preflight (waitForSFUHostPreflight)
# still runs inside sfu-single/m9h modes; this wrapper is the uniform
# outer gate for every other mode (pair, sfu, m7h, audio-rung1, ...).
# Usage: scripts/host-preflight.sh [--allow-unsafe-host] -- <command...>
set -eu
ALLOW_UNSAFE=0
if [ "${1:-}" = "--allow-unsafe-host" ]; then ALLOW_UNSAFE=1; shift; fi
[ "${1:-}" = "--" ] && shift
[ $# -ge 1 ] || { echo "usage: host-preflight.sh [--allow-unsafe-host] -- <command...>" >&2; exit 2; }
MEM_FREE_MIB="$(awk '/^MemAvailable:/ {print int($2/1024)}' /proc/meminfo)"
SWAP_USED_MIB="$(awk '/^SwapTotal:/ {t=$2} /^SwapFree:/ {f=$2} END {print int((t-f)/1024)}' /proc/meminfo)"
SWAP_TOTAL_MIB="$(awk '/^SwapTotal:/ {print int($2/1024)}' /proc/meminfo)"
DISK_FREE_BYTES="$(df --output=avail -B1 . 2>/dev/null | tail -1 | tr -d ' ')"
LOAD="$(cut -d' ' -f1-3 /proc/loadavg)"
FAILURES=""
[ "$MEM_FREE_MIB" -lt 512 ] && FAILURES="${FAILURES}free-memory ${MEM_FREE_MIB}MiB < 512MiB; "
# Used-swap cap mirrors the code constant sfuHostMaxUsedSwapMiB = 512.
[ "$SWAP_USED_MIB" -gt 512 ] && FAILURES="${FAILURES}used-swap ${SWAP_USED_MIB}MiB > 512MiB; "
[ "$DISK_FREE_BYTES" -lt 10737418240 ] && FAILURES="${FAILURES}free-disk ${DISK_FREE_BYTES}B < 10GiB; "
LINE=$(python3 -c "
import json
print(json.dumps({'host_preflight': 'READY' if not '''$FAILURES''' else 'NOT_READY',
  'mem_available_mib': $MEM_FREE_MIB, 'swap_used_mib': $SWAP_USED_MIB,
  'swap_total_mib': $SWAP_TOTAL_MIB, 'free_disk_bytes': $DISK_FREE_BYTES,
  'loadavg': '''$LOAD''', 'failures': '''$FAILURES'''.strip()}))")
echo "host_preflight=$LINE" >&2
[ -n "${PREFLIGHT_LOG:-}" ] && printf '%s\n' "$LINE" >> "$PREFLIGHT_LOG"
if [ -n "$FAILURES" ] && [ "$ALLOW_UNSAFE" -eq 0 ]; then
  echo "host_preflight: refusing to run (use --allow-unsafe-host to override; override is logged)" >&2
  exit 3
fi
if [ -n "$FAILURES" ]; then echo "host_preflight=OVERRIDDEN failures=\"$FAILURES\"" >&2; fi
exec "$@"
