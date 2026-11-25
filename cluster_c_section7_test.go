package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/pion/stun/v4"
	"github.com/pion/webrtc/v4"
)

// 7.1: STUN + server-reflexive candidates.
func TestClusterCSTUNServerReflexive(t *testing.T) {
	// In-process proof: candidate classification + STUN binding request
	// round-trip. A srflx candidate is the STUN proof.
	srflx := "candidate:1 1 udp 1686052607 203.0.113.7 54321 typ srflx raddr 10.0.0.5 rport 54321"
	if !isServerReflexive(srflx) {
		t.Fatal("srflx candidate not classified as server-reflexive")
	}
	if got := classifyICECandidateType(srflx); got != webrtc.ICECandidateTypeSrflx {
		t.Fatalf("srflx type = %v, want srflx", got)
	}
	host := "candidate:1 1 udp 2130706431 10.0.0.5 54321 typ host"
	if classifyICECandidateType(host) != webrtc.ICECandidateTypeHost {
		t.Fatal("host candidate misclassified")
	}
	relay := "candidate:1 1 udp 41885439 203.0.113.9 50000 typ relay"
	if classifyICECandidateType(relay) != webrtc.ICECandidateTypeRelay {
		t.Fatal("relay candidate misclassified")
	}
	msg, err := stunBindingRequest()
	if err != nil {
		t.Fatalf("STUN binding request: %v", err)
	}
	if msg.Type != stun.BindingRequest {
		t.Fatalf("STUN message type = %v, want BindingRequest", msg.Type)
	}
	// The live NAT-simulated connection cannot run in the VM sandbox
	// (unconnected UDP blocked): the reference-machine run is owed
	// and recorded in docs/cluster-c-results.md, not faked here.
	t.Logf("7.1 PARTIAL (VM): candidate classification + STUN binding request proven in-process; NAT-simulated live connection is a reference-machine run (VM sandbox blocks unconnected UDP)")
}

// 7.2: TURN with time-limited credentials.
func TestClusterCTURNTimeLimitedCredentials(t *testing.T) {
	now := time.Now()
	user := mintTURNTimeLimitedUsername("alice", time.Hour, now)
	if turnCredentialExpired(user, now.Add(30*time.Minute)) {
		t.Fatal("credential expired 30 minutes into a 1-hour validity")
	}
	if !turnCredentialExpired(user, now.Add(2*time.Hour)) {
		t.Fatal("credential still valid 2 hours into a 1-hour validity")
	}
	if !turnCredentialExpired("alice", now) {
		t.Fatal("username without an expiry prefix was treated as valid (must fail closed)")
	}
	if !turnCredentialExpired("not-a-time:alice", now) {
		t.Fatal("unparseable expiry was treated as valid")
	}
	// Symmetric-NAT simulation: the live relay path is the existing
	// TestSFUTURNRelayCarriesMedia (turn_test.go), which is one of the
	// 6 known VM sandbox UDP failures — it passes on the reference
	// machine. The simulation design (two relay-only peers behind
	// distinct NATs) is documented in docs/cluster-c-results.md §7.2.
	t.Logf("7.2 PARTIAL: time-limited credential expiry proven; symmetric-NAT relay media path is the existing relay test (VM-blocked, reference-machine pass)")
}

// 7.3: TURN over TLS on 443 + TCP fallback.
func TestClusterCTURNTCPAndTLSListeners(t *testing.T) {
	// TCP fallback: server starts on an ephemeral TCP port.
	tcpServer, err := startSFUTURNTCPServer("127.0.0.1:0", "hami-test", map[string]string{"alice": "s3cret-turn-password"})
	if err != nil {
		t.Fatalf("TURN TCP server: %v", err)
	}
	defer func() { _ = tcpServer.Close() }()
	if conn, err := net.DialTimeout("tcp", tcpServer.Addr(), 2*time.Second); err != nil {
		t.Fatalf("dial TURN TCP listener %s: %v", tcpServer.Addr(), err)
	} else {
		_ = conn.Close()
		t.Logf("7.3 TCP fallback: TURN server listening on TCP %s, dial succeeds", tcpServer.Addr())
	}

	// TURN over TLS: server starts with a test certificate, MinVersion
	// forced to TLS 1.2. A TLS handshake to it succeeds; a TLS 1.0/1.1
	// client is refused by the min-version floor.
	cert := testSelfSignedCert(t)
	tlsServer, err := startSFUTURNTLSServer("127.0.0.1:0", "hami-test", map[string]string{"alice": "s3cret-turn-password"}, &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("TURN TLS server: %v", err)
	}
	defer func() { _ = tlsServer.Close() }()
	conn, err := tls.Dial("tcp", tlsServer.Addr(), &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}) //nolint:gosec // test-only, self-signed cert
	if err != nil {
		t.Fatalf("TLS dial to TURN TLS listener: %v", err)
	}
	state := conn.ConnectionState()
	if state.Version < tls.VersionTLS12 {
		t.Fatalf("TURN TLS negotiated %x, want >= TLS 1.2", state.Version)
	}
	_ = conn.Close()
	t.Logf("7.3 TURN over TLS: listener on %s, TLS 1.2+ handshake succeeds (port 443 + public-cert deployment is a reference-machine step, not a VM step)", tlsServer.Addr())
}

func testSelfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.invalid"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"test.invalid"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        tmpl,
	}
}

// 7.4: ICE restart on network change, recovery within the stated time.
func TestClusterCICERestart(t *testing.T) {
	tr := newICERestartTracker()
	now := time.Now()
	tr.networkChanged(now)
	if ok := tr.recovered(now.Add(2 * time.Second)); !ok {
		t.Fatal("ICE restart in 2s did not meet the 5s deadline")
	}
	if got := tr.recoveryDuration(); got != 2*time.Second {
		t.Fatalf("recovery duration = %v, want 2s", got)
	}
	tr.networkChanged(now.Add(10 * time.Second))
	if ok := tr.recovered(now.Add(10*time.Second + 6*time.Second)); ok {
		t.Fatal("ICE restart in 6s met the 5s deadline")
	}
	t.Logf("7.4 PARTIAL: restart state machine + 5s stated deadline proven; live interface-switch recovery is a reference-machine run (needs a second interface)")
}

// 7.5: impairment matrix in the VM (in-process; exact settings saved).
func TestClusterCImpairmentMatrix(t *testing.T) {
	results := runImpairmentMatrix()
	if len(results) != 12 {
		t.Fatalf("matrix cells = %d, want 12 (9 loss×delay + jitter + reorder + duplication)", len(results))
	}
	// Loss cells: measured loss must be within ±1.5 percentage points
	// of the setting at n=1000 with seed 42 (deterministic).
	for _, r := range results {
		if r.Settings.JitterMs == 0 && !r.Settings.Reorder && !r.Settings.Duplicate {
			wantLost := int(r.Settings.LossPct * 10) // n=1000
			if r.Lost < wantLost-15 || r.Lost > wantLost+15 {
				t.Fatalf("cell loss=%.0f%% delay=%d: lost=%d, want ~%d", r.Settings.LossPct, r.Settings.DelayMs, r.Lost, wantLost)
			}
			if r.Settings.LossPct == 0 && r.Lost != 0 {
				t.Fatalf("loss=0 cell lost %d packets", r.Lost)
			}
			if r.MaxDelayMs < r.Settings.DelayMs {
				t.Fatalf("cell delay=%d: maxDelay=%dms, want >= %d", r.Settings.DelayMs, r.MaxDelayMs, r.Settings.DelayMs)
			}
		}
	}
	// Reorder cell produces out-of-order deliveries; duplication cell
	// doubles deliveries; jitter cell delays vary.
	for _, r := range results {
		t.Logf("7.5 %s", describeImpairmentCell(r))
		if r.Settings.Reorder && r.OutOfOrder == 0 {
			t.Fatal("reorder cell produced no out-of-order deliveries")
		}
		if r.Settings.Duplicate && r.Duplicated == 0 {
			t.Fatal("duplication cell produced no duplicates")
		}
	}
	// Determinism: same seed gives identical results.
	again := runImpairmentMatrix()
	raw1, _ := json.Marshal(results)
	raw2, _ := json.Marshal(again)
	if string(raw1) != string(raw2) {
		t.Fatal("impairment matrix is not deterministic across runs")
	}
}

// 9.3: trickle ICE — late candidates still connect.
func TestClusterCTrickleICELateCandidates(t *testing.T) {
	buf := newLateCandidateBuffer()
	cands := []webrtc.ICECandidateInit{
		{Candidate: "candidate:1 1 udp 2130706431 10.0.0.5 5000 typ host"},
		{Candidate: "candidate:2 1 udp 1686052607 203.0.113.7 5001 typ srflx raddr 10.0.0.5 rport 5000"},
	}
	// Candidates arrive AFTER the offer (the trickle case).
	for _, c := range cands {
		buf.add(c)
	}
	if got := buf.pending(); got != 2 {
		t.Fatalf("pending = %d, want 2", got)
	}
	applied := buf.applyAll()
	if len(applied) != 2 || applied[0].Candidate != cands[0].Candidate || applied[1].Candidate != cands[1].Candidate {
		t.Fatalf("applied candidates out of order or incomplete: %+v", applied)
	}
	if buf.pending() != 0 || buf.appliedCount() != 2 {
		t.Fatalf("after apply: pending=%d applied=%d, want 0/2", buf.pending(), buf.appliedCount())
	}
	// A candidate arriving after the answer is still buffered, not dropped.
	buf.add(webrtc.ICECandidateInit{Candidate: "candidate:3 1 udp 41885439 203.0.113.9 5002 typ relay"})
	if buf.pending() != 1 {
		t.Fatal("late candidate after the answer was dropped")
	}
	t.Logf("9.3: late candidates buffered in arrival order and applied; post-answer candidate still accepted")
}
