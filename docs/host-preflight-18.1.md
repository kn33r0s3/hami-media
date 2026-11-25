# Host preflight before every run (18.1)

Job 18.1: memory pressure, free swap, disk space and other load are
checked and logged before every run.

Status: **DONE 25 Nov 2026** — one uniform outer gate plus the
in-binary gates that already existed.

## The uniform call

`scripts/host-preflight.sh [--allow-unsafe-host] -- <command...>`
is the single preflight call for every run mode. It checks and logs
one JSON line before the wrapped command runs:

```json
{"host_preflight": "READY", "mem_available_mib": 1195, "swap_used_mib": 417,
 "swap_total_mib": 1023, "free_disk_bytes": 109485211648,
 "loadavg": "4.71 4.36 2.34", "failures": ""}
```

Checks (failures refuse the run unless `--allow-unsafe-host` is
passed, and the override is itself logged):

- free memory (MemAvailable) below 512 MiB;
- used swap above **512 MiB** (mirrors the code constant
  `sfuHostMaxUsedSwapMiB`);
- free disk below 10 GiB (mirrors `sfuHostMinFreeDiskBytes`);
- load average is recorded on every line (other load is checked and
  logged; it does not by itself refuse a run — the binary's own
  preflight is the refusing gate for scale runs).

The line goes to stderr and, when `PREFLIGHT_LOG` is set, is
appended to that file. The benchmark runner (15.5), the soak script
(15.8) and the nightly job (15.4) all call this wrapper, so their
preflight logs are uniform.

## The in-binary gates (already present, wired per mode)

- Scale proofs (`sfu-single`, `m9h`): `waitForSFUHostPreflight()`
  blocks until `sfuHostPreflightFailures` is empty, logging
  `host_preflight_sample=` and `host_preflight=NOT_READY` lines;
  `readSFUHostSnapshot` / `readSFUHostSnapshotLive` supply memory
  pressure, swap and disk; the in-run watchdog
  (`startSFUHostWatchdog`, 1 s tick) aborts on a violation.
- Audio rung: its own guard via `readSwapMiBChecked` + snapshot.
- Capacity detector: `detectDeviceProfile` via the same readers.

Verified in the VM, 25 Nov 2026:

```
$ ./scripts/host-preflight.sh -- echo PREFLIGHT-OK
host_preflight={"host_preflight": "READY", ... "swap_used_mib": 417, ...}
PREFLIGHT-OK
```

Note the VM's swap used (417 MiB at the time) sits close to the
512 MiB cap — a scale run on this VM would be near the refusal line.
That is a property of the VM, not of the gate.
