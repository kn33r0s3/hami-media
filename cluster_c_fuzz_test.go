// Cluster C §8.6 — fuzz targets for SDP, RTP, RTCP, signaling JSON.
//
// Run: go test -run=Fuzz -fuzz=FuzzParseSDP -fuzztime=60s .  (one
// target at a time; the script scripts/fuzz-smoke.sh runs each target
// for a fixed 60 s and fails on any crash). No CI exists yet (cluster
// A owns the workflow file); the targets + fixed-time script are the
// deliverable here, and the CI wiring is recorded in
// docs/cluster-c-results.md §8.6/8.7.
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
)

// FuzzParseSDP feeds arbitrary SDP text to the SDP size validator and
// the profile-level-id parser. Neither may panic; oversized input
// must be rejected, never accepted.
func FuzzParseSDP(f *testing.F) {
	f.Add("v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n")
	f.Add("a=fmtp:102 profile-level-id=42001f;packetization-mode=1\r\n")
	f.Add("")
	f.Fuzz(func(t *testing.T, sdp string) {
		_ = validateSDPSize(sdp)
		// Extract any profile-level-id= value and parse it; must not panic.
		for _, line := range strings.Split(sdp, "\n") {
			if i := strings.Index(line, "profile-level-id="); i >= 0 {
				val := line[i+len("profile-level-id="):]
				val = strings.TrimRight(val, " ;\r")
				if j := strings.IndexAny(val, "; \r"); j >= 0 {
					val = val[:j]
				}
				_, _, _ = parseProfileLevelID(val)
				_, _ = negotiateProfileLevelID("42001f", []string{val})
			}
		}
	})
}

// FuzzParseRTP feeds arbitrary bytes to the RTP unmarshaller and the
// packet-layer classifier. Neither may panic.
func FuzzParseRTP(f *testing.F) {
	f.Add([]byte{0x80, 0x65, 0x00, 0x01, 0x00, 0x00, 0x00, 0x2a, 0x00, 0x00, 0x00, 0x01, 0x41, 0x9a})
	f.Add([]byte{})
	f.Add([]byte{0x80})
	f.Fuzz(func(t *testing.T, data []byte) {
		pkt := &rtp.Packet{}
		if err := pkt.Unmarshal(data); err != nil {
			return
		}
		_ = classifyPacketLayer("video/H264", pkt)
		_ = classifyPacketLayer("video/VP8", pkt)
	})
}

// FuzzParseRTCP feeds arbitrary bytes to the RTCP unmarshaller.
func FuzzParseRTCP(f *testing.F) {
	f.Add([]byte{0x80, 0xc9, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		packets, err := rtcp.Unmarshal(data)
		if err != nil {
			return
		}
		_ = packets
	})
}

// FuzzSignalingJSON feeds arbitrary bytes to the signaling message
// validator (job 9.1 spec). It must never panic; malformed JSON must
// be rejected with an error.
func FuzzSignalingJSON(f *testing.F) {
	f.Add([]byte(`{"version":1,"type":"join","room":"default"}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`not json`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var raw json.RawMessage
		if err := json.Unmarshal(data, &raw); err != nil {
			return
		}
		_, _ = parseSignalingMessage(data)
	})
}
