#!/bin/sh
# 14.8: verify OS tuning on the reference Linux image.
# Reads live values only; it does not change anything.
# Exit 0 only if every check passes; prints measured values either way.
set -u
fail=0
check() {
  # check <name> <got> <want-min>
  if [ "$2" -ge "$3" ] 2>/dev/null; then
    echo "PASS $1: $2 (>= $3)"
  else
    echo "FAIL $1: $2 (want >= $3)"
    fail=1
  fi
}
rmem_max=$(sysctl -n net.core.rmem_max 2>/dev/null || echo 0)
wmem_max=$(sysctl -n net.core.wmem_max 2>/dev/null || echo 0)
rmem_def=$(sysctl -n net.core.rmem_default 2>/dev/null || echo 0)
wmem_def=$(sysctl -n net.core.wmem_default 2>/dev/null || echo 0)
backlog=$(sysctl -n net.core.netdev_max_backlog 2>/dev/null || echo MISSING)
check net.core.rmem_max "$rmem_max" 16777216
check net.core.wmem_max "$wmem_max" 16777216
check net.core.rmem_default "$rmem_def" 16777216
check net.core.wmem_default "$wmem_def" 16777216
if [ "$backlog" = "MISSING" ]; then
  echo "UNKNOWN net.core.netdev_max_backlog: key not present in this VM (reference image must have it)"
  fail=1
else
  check net.core.netdev_max_backlog "$backlog" 4096
fi
nofile=$(ulimit -n)
check "ulimit -n" "$nofile" 65536
cpus=$(nproc)
echo "INFO cpus: $cpus"
mem_kb=$(awk '/MemTotal/ {print $2}' /proc/meminfo)
echo "INFO MemTotal_kB: $mem_kb"
gov=$(cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor 2>/dev/null || echo "unknown (not readable in this VM)")
echo "INFO cpu governor: $gov"
if [ "$fail" -eq 0 ]; then echo "OS-TUNING PASS"; else echo "OS-TUNING FAIL (apply docs/cluster-d/14-os-tuning.md, then re-run)"; fi
exit $fail
