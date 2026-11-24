//go:build windows

package main

import (
	"errors"
	"fmt"
	"syscall"
	"time"
)

// hostPressureSupported reports whether this platform exposes the macOS
// VM pressure level. Windows does not; the preflight/watchdog gates skip
// the pressure check there (see sfuHostPreflightFailures /
// sfuHostWatchdogFailures). The host readers below are honest stubs:
// Windows has no equivalent snapshot source wired in, so they return an
// error instead of fake numbers.
const hostPressureSupported = false

// readDeviceProfilePlatform is not supported on Windows: there is
// no equivalent profile source wired in, so it returns an error
// instead of fake numbers (same honesty rule as the host snapshot
// stubs above). The `capacity` subcommand therefore fails on
// Windows until a real reader lands.
func readDeviceProfilePlatform(profile *deviceProfile) error {
	return errors.New("device capability detection not supported on windows")
}

// readSFUHostSnapshot is not supported on Windows.
func readSFUHostSnapshot() (sfuHostSnapshot, error) {
	return sfuHostSnapshot{}, errors.New("host preflight snapshot not supported on windows")
}

// readSFUHostSnapshotLive is not supported on Windows.
func readSFUHostSnapshotLive(baseline sfuHostSnapshot) (sfuHostSnapshot, error) {
	return sfuHostSnapshot{}, errors.New("host watchdog snapshot not supported on windows")
}

// readAudioSwapMiB is not supported on Windows.
func readAudioSwapMiB() (used, free float64, returnErr error) {
	return 0, 0, errors.New("swap usage reading not supported on windows")
}

func readSocketReceiveBuffer(fd uintptr) (int, error) {
	return syscall.GetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF)
}

func m7hProcessCPUSeconds() (float64, error) {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0, fmt.Errorf("get current process handle: %w", err)
	}
	var creation, exit, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return 0, fmt.Errorf("read process CPU usage: %w", err)
	}
	return float64(user.Nanoseconds())/float64(time.Second) +
		float64(kernel.Nanoseconds())/float64(time.Second), nil
}
