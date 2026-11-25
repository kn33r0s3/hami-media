// SRTP replay protection gate (job 2.1).
//
// Replay protection is ON by default: the three production call sites
// (viewer clients, publisher clients, router) must not disable it.
// The only way to turn it off is the hami_disable_srtp_replay build
// tag (srtp_replay_disable.go), which exists so a loss test can
// observe retransmitted duplicates. That tag is test-only: it must
// never be used in a production build, and the replay test
// (TestSRTPReplayProtectionDropsDuplicate) fails under it by design.
package main

import (
	"github.com/pion/srtp/v3"
)

// srtpRemoteReplayOptions returns the remote-context SRTP options
// matching this build's replay-protection setting, mirroring what
// Pion's DTLS transport does with SettingEngine
// (DisableSRTPReplayProtection -> SRTPNoReplayProtection). With
// protection on this returns nil and Pion's default 64-packet window
// applies.
func srtpRemoteReplayOptions() []srtp.ContextOption {
	if srtpReplayProtectionDisabled() {
		return []srtp.ContextOption{srtp.SRTPNoReplayProtection()}
	}
	return nil
}
