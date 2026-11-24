//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// hostPressureSupported reports whether this platform exposes the macOS
// VM pressure level (kern.memorystatus_vm_pressure_level). Linux has no
// equivalent: /proc/pressure/memory exists, but mapping its stall
// percentages onto the macOS 1/2/4 scale would be an uncalibrated
// invention, so Linux reports no pressure level (PressureLevel stays 0)
// and the preflight/watchdog gates skip the pressure check here (see
// sfuHostPreflightFailures / sfuHostWatchdogFailures). Swap and disk
// still guard the run on Linux — both are read for real below.
const hostPressureSupported = false

// readAudioSwapMiB reads swap usage from /proc/meminfo (SwapTotal and
// SwapFree, in KiB). Same (used, free) MiB contract as the darwin
// sysctl version.
func readAudioSwapMiB() (used, free float64, returnErr error) {
	totalKiB, err := readMeminfoKiB("SwapTotal")
	if err != nil {
		return 0, 0, fmt.Errorf("read Linux swap usage: %w", err)
	}
	freeKiB, err := readMeminfoKiB("SwapFree")
	if err != nil {
		return 0, 0, fmt.Errorf("read Linux swap usage: %w", err)
	}
	used = float64(totalKiB-freeKiB) / 1024
	free = float64(freeKiB) / 1024
	return used, free, nil
}

// readSFUHostSnapshot is the Linux preflight reader. It mirrors the
// darwin reader's contract with Linux sources: swap via /proc/meminfo,
// reclaimable memory via MemAvailable (the kernel's own reclaimable
// estimate — the closest equivalent of darwin free+inactive+
// speculative), swap/page counters via /proc/vmstat (pswpout/pgpgout),
// disk via `df -k` and the shared parseDFAvailableBytes (the Available
// column is fields[3] on both platforms). PressureLevel stays 0: no
// macOS-style pressure level exists on Linux.
// readDeviceProfilePlatform fills the Linux-specific fields of a
// device profile: CPU model, total RAM, kernel release, fd limits
// and UDP buffer ceilings, all from /proc and getrlimit(2). No
// subprocess is spawned.
func readDeviceProfilePlatform(profile *deviceProfile) error {
	cpuinfo, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return fmt.Errorf("read /proc/cpuinfo: %w", err)
	}
	for _, line := range strings.Split(string(cpuinfo), "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(name) != "model name" {
			continue
		}
		profile.CPUModel = strings.TrimSpace(rest)
		break
	}
	if profile.CPUModel == "" {
		return fmt.Errorf("/proc/cpuinfo has no model name key")
	}
	release, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return fmt.Errorf("read /proc/sys/kernel/osrelease: %w", err)
	}
	profile.Kernel = strings.TrimSpace(string(release))
	memTotalKiB, err := readMeminfoKiB("MemTotal")
	if err != nil {
		return fmt.Errorf("read Linux total memory: %w", err)
	}
	profile.RAMTotalMiB = memTotalKiB / 1024
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		return fmt.Errorf("read Linux file descriptor limit: %w", err)
	}
	profile.FDSoftLimit = rlimit.Cur
	profile.FDHardLimit = rlimit.Max
	fileMax, err := readProcSysUint64("/proc/sys/fs/file-max")
	if err != nil {
		return fmt.Errorf("read Linux system file cap (/proc/sys/fs/file-max): %w", err)
	}
	profile.FDMaxFiles = fileMax
	rmemMax, err := readProcSysUint64("/proc/sys/net/core/rmem_max")
	if err != nil {
		return fmt.Errorf("read Linux UDP receive buffer ceiling (/proc/sys/net/core/rmem_max): %w", err)
	}
	profile.UDPRecvBufferBytes = int(rmemMax)
	wmemMax, err := readProcSysUint64("/proc/sys/net/core/wmem_max")
	if err != nil {
		return fmt.Errorf("read Linux UDP send buffer ceiling (/proc/sys/net/core/wmem_max): %w", err)
	}
	profile.UDPSendBufferBytes = int(wmemMax)
	return nil
}

// readProcSysUint64 reads one /proc/sys value as an unsigned integer.
func readProcSysUint64(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s %q: %w", path, strings.TrimSpace(string(data)), err)
	}
	return value, nil
}

func readSFUHostSnapshot() (sfuHostSnapshot, error) {
	snapshot := sfuHostSnapshot{ObservedAt: time.Now().UTC()}
	var err error
	snapshot.SwapUsedMiB, snapshot.SwapFreeMiB, err = readAudioSwapMiB()
	if err != nil {
		return snapshot, err
	}
	snapshot.SwapTotalMiB = snapshot.SwapUsedMiB + snapshot.SwapFreeMiB
	memAvailableKiB, err := readMeminfoKiB("MemAvailable")
	if err != nil {
		return snapshot, fmt.Errorf("read Linux available memory: %w", err)
	}
	snapshot.MemFreeMiB = float64(memAvailableKiB) / 1024
	snapshot.Swapouts, err = readVMStatCounter("pswpout")
	if err != nil {
		return snapshot, err
	}
	snapshot.Pageouts, err = readVMStatCounter("pgpgout")
	if err != nil {
		return snapshot, err
	}
	diskOutput, err := exec.Command("df", "-k", ".").Output()
	if err != nil {
		return snapshot, fmt.Errorf("read free disk space: %w", err)
	}
	snapshot.FreeDiskBytes, err = parseDFAvailableBytes(string(diskOutput))
	if err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

// readSFUHostSnapshotLive is the Linux in-run host watchdog reader. It
// mirrors the darwin live reader: MemFreeMiB/Swapouts/Pageouts are
// carried from the preflight baseline (the watchdog's abort decision
// uses only pressure, swap and disk — all read live here), while swap
// and disk are refreshed live via /proc/meminfo and statfs. No
// subprocess is spawned, so a watchdog tick costs microseconds.
func readSFUHostSnapshotLive(baseline sfuHostSnapshot) (sfuHostSnapshot, error) {
	snapshot := baseline
	snapshot.ObservedAt = time.Now().UTC()
	var err error
	snapshot.SwapUsedMiB, snapshot.SwapFreeMiB, err = readAudioSwapMiB()
	if err != nil {
		return snapshot, err
	}
	snapshot.SwapTotalMiB = snapshot.SwapUsedMiB + snapshot.SwapFreeMiB
	var stat syscall.Statfs_t
	if err := syscall.Statfs(".", &stat); err != nil {
		return snapshot, fmt.Errorf("read free disk space: %w", err)
	}
	snapshot.FreeDiskBytes = stat.Bavail * uint64(stat.Bsize)
	return snapshot, nil
}

// readMeminfoKiB returns the KiB value of a /proc/meminfo key
// (e.g. "SwapTotal", "SwapFree", "MemAvailable").
func readMeminfoKiB(key string) (uint64, error) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("read /proc/meminfo: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found || strings.TrimSpace(name) != key {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return 0, fmt.Errorf("/proc/meminfo %s has no value", key)
		}
		value, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse /proc/meminfo %s %q: %w", key, fields[0], err)
		}
		return value, nil
	}
	return 0, fmt.Errorf("/proc/meminfo has no %s key", key)
}

// readVMStatCounter returns a /proc/vmstat counter by key
// (e.g. "pswpout", "pgpgout").
func readVMStatCounter(key string) (uint64, error) {
	data, err := os.ReadFile("/proc/vmstat")
	if err != nil {
		return 0, fmt.Errorf("read /proc/vmstat: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] != key {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse /proc/vmstat %s %q: %w", key, fields[1], err)
		}
		return value, nil
	}
	return 0, fmt.Errorf("/proc/vmstat has no %s key", key)
}
