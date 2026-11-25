# 14.8 OS tuning guide — reference Linux image

Tested on this VM (Ubuntu 24.04, kernel 6.x) with
`scripts/verify-os-tuning.sh`. Apply, then verify with the script;
do not claim tuning without the script's output.

**VM limit (recorded, not hidden):** this VM cannot apply the
settings — `sysctl -w` and `ulimit -n` both return `Operation not
permitted`, and `net.core.netdev_max_backlog` does not exist in
this VM's `/proc`. The script therefore FAILs here with the live
values (rmem/wmem max 4 MiB, defaults 212992, ulimit 4096).
Applying + verifying the settings is owed on the reference Linux
image, where the operator has privilege. The guide and the script
are the deliverable that can be done here; 14.8 stays PARTIAL
until the reference-image run passes.

## UDP buffers (the media path is UDP)

```sh
# Receive + send buffers: 16 MiB each (from the 212992 default).
sysctl -w net.core.rmem_max=16777216
sysctl -w net.core.wmem_max=16777216
sysctl -w net.core.rmem_default=16777216
sysctl -w net.core.wmem_default=16777216
# Backlog for bursty arrival.
sysctl -w net.core.netdev_max_backlog=4096
```

Persist in `/etc/sysctl.d/99-hami-media.conf` (same keys, no `sysctl -w`).

## File descriptors

```sh
ulimit -n 65536
# Persist for the service user in /etc/security/limits.conf:
# hami soft nofile 65536
# hami hard nofile 65536
```

Each viewer holds sockets + files; the 1024 default is exhausted
long before the viewer limit (see capacity limits, `capacity.go`).

## CPU

```sh
# Keep the forwarder on performance governors where the operator
# controls the machine; do not set powersave on a media node.
# Verify: cat /sys/devices/system/cpu/cpu0/cpufreq/scaling_governor
# Pin the process if the machine is shared: taskset -c 0-7 <cmd>
```

No CPU frequency changes are made by the verification script — it
only reads and reports.

## Verification

```sh
scripts/verify-os-tuning.sh
```

The script reads the live values (rmem/wmem max+default,
netdev_max_backlog, `ulimit -n`, governor, CPU count, memory) and
reports PASS/FAIL per setting with the measured value. It does not
change anything: applying the settings is the operator's step
(sysctl needs privilege the VM does not grant).
