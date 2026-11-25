//go:build darwin

package main

import (
	"encoding/binary"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// hostPressureSupported reports whether this platform exposes the macOS
// VM pressure level (kern.memorystatus_vm_pressure_level). darwin is the
// only platform that does; the Linux and Windows readers report no
// pressure level, and the preflight/watchdog gates skip the pressure
// check there (see sfuHostPreflightFailures / sfuHostWatchdogFailures).
const hostPressureSupported = true

// readDeviceProfilePlatform fills the darwin-specific fields of a
// device profile: CPU model, total RAM, kernel release, fd limits
// and UDP buffer ceilings, all from sysctl(2) / getrlimit(2). No
// subprocess is spawned: the capacity detector runs at startup and
// must not fork (see the FIX 30 note on readSFUHostSnapshotLive).
func readDeviceProfilePlatform(profile *deviceProfile) error {
	model, err := syscall.Sysctl("hw.model")
	if err != nil {
		return fmt.Errorf("read darwin CPU model (hw.model): %w", err)
	}
	profile.CPUModel = strings.TrimSpace(strings.TrimRight(model, "\x00"))
	kernel, err := syscall.Sysctl("kern.osrelease")
	if err != nil {
		return fmt.Errorf("read darwin kernel release (kern.osrelease): %w", err)
	}
	profile.Kernel = strings.TrimSpace(strings.TrimRight(kernel, "\x00"))
	memsize, err := parseSysctlUint64("hw.memsize")
	if err != nil {
		return fmt.Errorf("read darwin total RAM (hw.memsize): %w", err)
	}
	profile.RAMTotalMiB = memsize / (1024 * 1024)
	var rlimit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlimit); err != nil {
		return fmt.Errorf("read darwin file descriptor limit: %w", err)
	}
	profile.FDSoftLimit = rlimit.Cur
	profile.FDHardLimit = rlimit.Max
	maxfiles, err := parseSysctlUint64("kern.maxfiles")
	if err != nil {
		return fmt.Errorf("read darwin system file cap (kern.maxfiles): %w", err)
	}
	profile.FDMaxFiles = maxfiles
	recvspace, err := parseSysctlUint64("net.inet.udp.recvspace")
	if err != nil {
		return fmt.Errorf("read darwin UDP receive buffer (net.inet.udp.recvspace): %w", err)
	}
	profile.UDPRecvBufferBytes = int(recvspace)
	return nil
}

// parseSysctlUint64 reads one integer sysctl(2) value.
// syscall.Sysctl returns the raw bytes, not a decimal string, and
// it trims one trailing NUL byte: an 8-byte uint64 (hw.memsize)
// arrives as 7 bytes and a 4-byte int (kern.maxfiles,
// net.inet.udp.recvspace) as 3 bytes whenever its top byte is zero.
// The value is little-endian, so the missing bytes are the zero
// high bytes: pad on the right to 8 bytes and decode. All Macs are
// little-endian — the same note as readAudioSwapMiB's xsw_usage
// decode. String sysctls (hw.model, kern.osrelease) come back as
// NUL-terminated text and must NOT go through here.
func parseSysctlUint64(name string) (uint64, error) {
	raw, err := syscall.Sysctl(name)
	if err != nil {
		return 0, err
	}
	if len(raw) == 0 || len(raw) > 8 {
		return 0, fmt.Errorf("parse sysctl %s: unexpected %d-byte value %q", name, len(raw), raw)
	}
	padded := make([]byte, 8)
	copy(padded, raw)
	return binary.LittleEndian.Uint64(padded), nil
}

func readSFUHostSnapshot() (sfuHostSnapshot, error) {
	snapshot := sfuHostSnapshot{ObservedAt: time.Now().UTC()}
	pressureOutput, err := exec.Command("sysctl", "-n", "kern.memorystatus_vm_pressure_level").Output()
	if err != nil {
		return snapshot, fmt.Errorf("read macOS VM pressure: %w", err)
	}
	snapshot.PressureLevel, err = strconv.Atoi(strings.TrimSpace(string(pressureOutput)))
	if err != nil {
		return snapshot, fmt.Errorf("parse macOS VM pressure %q: %w", strings.TrimSpace(string(pressureOutput)), err)
	}
	snapshot.SwapUsedMiB, snapshot.SwapFreeMiB, err = readSwapMiBChecked()
	if err != nil {
		return snapshot, err
	}
	snapshot.SwapTotalMiB = snapshot.SwapUsedMiB + snapshot.SwapFreeMiB
	vmStatOutput, err := exec.Command("vm_stat").Output()
	if err != nil {
		return snapshot, fmt.Errorf("read macOS VM counters: %w", err)
	}
	snapshot.MemFreeMiB, err = parseVMStatFreeMiB(string(vmStatOutput))
	if err != nil {
		return snapshot, err
	}
	snapshot.Swapouts, err = parseVMStatCounter(string(vmStatOutput), "Swapouts")
	if err != nil {
		return snapshot, err
	}
	snapshot.Pageouts, err = parseVMStatCounter(string(vmStatOutput), "Pageouts")
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

// FIX 30: readSFUHostSnapshotLive is the in-run host watchdog reader.
// The preflight reader above spawns four subprocesses per call (sysctl
// pressure, sysctl vm.swapusage, vm_stat, df); fired once a second
// inside a streaming cell, those forks stalled the whole process
// 50-120 ms per tick (FIX 30 measurement: heartbeat late 50-120 ms at
// exactly the watchdog tick phase in every cell, both runs; frames
// whose transit overlapped the stall went 100+ ms slow; run-to-run
// variance is the tick phase vs the 200 ms frame grid). This reader
// uses only direct syscalls — pressure via sysctlbyname, swap via
// syscall.Sysctl("vm.swapusage"), disk via statfs — so a watchdog tick
// costs microseconds and cannot stall the pipeline.
// MemFreeMiB/Swapouts/Pageouts are carried from the preflight baseline:
// macOS exposes them only via Mach host_statistics (vm_stat), which has
// no fork-free equivalent. That is safe: the watchdog's abort decision
// (sfuHostWatchdogFailures) uses only pressure, swap and disk — all read
// live here — and the parent aggregates the preflight snapshot, not the
// live samples. The live sample JSON keeps its shape.
func readSFUHostSnapshotLive(baseline sfuHostSnapshot) (sfuHostSnapshot, error) {
	// Start from the preflight baseline: MemFreeMiB/Swapouts/Pageouts
	// have no fork-free source on macOS (see the FIX 30 note above)
	// and are carried forward unchanged. ObservedAt, pressure, swap
	// and disk are all refreshed live below.
	snapshot := baseline
	snapshot.ObservedAt = time.Now().UTC()
	pressure, err := syscall.SysctlUint32("kern.memorystatus_vm_pressure_level")
	if err != nil {
		return snapshot, fmt.Errorf("read macOS VM pressure: %w", err)
	}
	snapshot.PressureLevel = int(pressure)
	snapshot.SwapUsedMiB, snapshot.SwapFreeMiB, err = readSwapMiBChecked()
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

// FIX 30: readAudioSwapMiB reads swap usage via the sysctl(2) syscall
// instead of spawning `sysctl vm.swapusage`. The old version forked a
// subprocess on every call; called once a second by the host watchdog
// inside a ~700 MB / 36k-goroutine cell, that fork stalled the whole
// process 50-120 ms (FIX 30 measurement: the 10 ms heartbeat goroutine
// went late by 50-120 ms at exactly the watchdog tick phase, and every
// frame whose transit overlapped the stall went 100+ ms slow). The raw
// sysctl returns struct xsw_usage: total, avail, used as uint64 bytes
// at offsets 0/8/16, then pagesize + encrypted. All Macs are
// little-endian. Same (used, free) MiB contract as before.
func readAudioSwapMiB() (used, free float64, returnErr error) {
	raw, err := syscall.Sysctl("vm.swapusage")
	if err != nil {
		return 0, 0, fmt.Errorf("read macOS swap usage: %w", err)
	}
	rawBytes := []byte(raw)
	if len(rawBytes) < 24 {
		return 0, 0, fmt.Errorf("read macOS swap usage: short xsw_usage (%d bytes)", len(rawBytes))
	}
	freeBytes := binary.LittleEndian.Uint64(rawBytes[8:16])
	usedBytes := binary.LittleEndian.Uint64(rawBytes[16:24])
	used = float64(usedBytes) / (1024 * 1024)
	free = float64(freeBytes) / (1024 * 1024)
	return used, free, nil
}

// readProcessRSSBytes returns the current process's resident set size
// in bytes, from `ps -o rss=` (KiB). Darwin has no /proc; the sampler
// runs once per second in a background goroutine, so one ps fork per
// sample is acceptable for a diagnostic (the darwin preflight already
// shells out to df/vm_stat the same way).
func readProcessRSSBytes() (uint64, error) {
	output, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(syscall.Getpid())).Output()
	if err != nil {
		return 0, fmt.Errorf("read process RSS via ps: %w", err)
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return 0, fmt.Errorf("ps RSS output is empty")
	}
	residentKiB, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse ps RSS %q: %w", fields[0], err)
	}
	return residentKiB * 1024, nil
}

// readMeminfoKiB reads one /proc/meminfo value. Linux-only: the reader
// lives in platform_linux.go. On this platform there is no /proc, so
// the stub errors instead of fake numbers. The only caller is
// TestAudioSwapCheckRunsOnLinux, which skips at runtime off Linux —
// the stub exists so the test file still compiles on every platform
// (merge fix: vet failed on darwin/windows with undefined
// readMeminfoKiB).
func readMeminfoKiB(key string) (uint64, error) {
	return 0, fmt.Errorf("readMeminfoKiB is Linux-only (/proc/meminfo)")
}
