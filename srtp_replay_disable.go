//go:build hami_disable_srtp_replay

// Loss-test-only build of the SRTP replay gate (job 2.1). Building
// with -tags hami_disable_srtp_replay turns SRTP replay protection
// OFF so a loss test can observe retransmitted duplicates. This file
// must never be used in a production build. The replay test
// (TestSRTPReplayProtectionDropsDuplicate) fails under this tag by
// design: a duplicate packet is delivered instead of dropped.
package main

// srtpReplayProtectionDisabled reports whether SRTP replay protection
// is disabled for this build. Tagged build: true (protection off).
func srtpReplayProtectionDisabled() bool {
	return true
}
