#!/bin/sh
# Job 16.5 — Record the reference hardware by script.
# Writes one JSON object to stdout (or to $1 if given) with CPU, RAM,
# OS, kernel, network interfaces, Go version and the hami-media code
# sha. Attach the output to every result (R4). No host names, no user
# names, no home paths are recorded (R2).
# Usage: scripts/record-hardware.sh [output.json]
set -eu
OUT="${1:-/dev/stdout}"
CPU_MODEL="$(grep -m1 'model name' /proc/cpuinfo 2>/dev/null | cut -d: -f2- | sed 's/^ *//;s/ *$//')"
CPU_CORES="$(nproc 2>/dev/null || grep -c '^processor' /proc/cpuinfo)"
MEM_TOTAL_KB="$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)"
SWAP_TOTAL_KB="$(awk '/^SwapTotal:/ {print $2}' /proc/meminfo)"
KERNEL="$(uname -r)"
OS="$( (lsb_release -ds 2>/dev/null || grep '^PRETTY_NAME=' /etc/os-release 2>/dev/null | cut -d= -f2- | tr -d '"') || echo unknown)"
GO_VERSION="$(go version 2>/dev/null || echo unknown)"
CODE_SHA="$(sha256sum main.go 2>/dev/null | cut -d' ' -f1 || echo unknown)"
NET="$(ip -brief addr 2>/dev/null | awk '{print $1":"$3}' | paste -sd, - || echo unknown)"
python3 - "$OUT" <<PY
import json, sys
doc = {
  "cpu_model": """$CPU_MODEL""",
  "cpu_cores": int("""$CPU_CORES"""),
  "mem_total_mib": int("""$MEM_TOTAL_KB""") // 1024,
  "swap_total_mib": int("""$SWAP_TOTAL_KB""") // 1024,
  "kernel": """$KERNEL""",
  "os": """$OS""",
  "go_version": """$GO_VERSION""",
  "main_go_sha256": """$CODE_SHA""",
  "network_interfaces": """$NET""",
}
out = sys.argv[1]
if out == "/dev/stdout":
    print(json.dumps(doc, indent=2, sort_keys=True))
else:
    with open(out, "w") as f:
        json.dump(doc, f, indent=2, sort_keys=True)
        f.write("\n")
PY
