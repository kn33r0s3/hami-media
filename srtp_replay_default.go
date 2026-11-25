//go:build !hami_disable_srtp_replay

// Default build of the SRTP replay gate (job 2.1): replay protection
// is ON.
package main

// srtpReplayProtectionDisabled reports whether SRTP replay protection
// is disabled for this build. Default build: false (protection on).
func srtpReplayProtectionDisabled() bool {
	return false
}
