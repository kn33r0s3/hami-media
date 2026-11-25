// Cluster C §7 network + §9.3 trickle ICE.
//
// 7.1 STUN + server-reflexive candidates: pion/stun is already a
// dependency and STUN packets are counted in pair diagnostics
// (main.go). What was missing is a configured STUN server and a test
// that connects through server-reflexive candidates. The VM sandbox
// blocks unconnected UDP, so a live STUN exchange cannot run here;
// this file therefore (a) provides the STUN configuration + candidate
// classification used on the reference machine, (b) proves candidate
// parsing/classification in-process, and (c) provides the
// NAT-simulated test that runs on the reference machine and SKIPS
// honestly in the VM (see TestClusterCSTUNServerReflexive).
//
// 7.3 TURN over TLS on 443 + TCP fallback: pion/turn v5.1.3 DOES
// support TCP listeners (ListenerConfigs) and TLS is handled by the
// TLS handshake in readListener (server.go:179-185) — a TLS listener
// is a net.Listener wrapped with crypto/tls. Implemented here:
// startSFUTURNTCPServer / startSFUTURNTLSServer. The VM limitation is
// that a real TLS-on-443 test needs a certificate + port 443; the
// test uses an ephemeral port with a test certificate and proves the
// server accepts a TCP/TLS allocation handshake path at the listener
// level (the full relay-media path over TCP is a reference-machine
// run, recorded in docs/cluster-c-results.md).
//
// 7.4 ICE restart on network change: iceRestartTracker models the
// restart state machine (gather -> check -> connected) with a stated
// recovery time; the VM-testable part is the state machine + timing,
// the live interface-switch part needs a second interface and is
// recorded as owed.
//
// 9.3 Trickle ICE: lateCandidateBuffer holds candidates that arrive
// after the offer; they are applied in order and still connect.
package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pion/stun/v4"
	"github.com/pion/turn/v5"
	"github.com/pion/webrtc/v4"
)

// --- 7.1 STUN ---------------------------------------------------------------

// stunServerURL is the STUN server configured for clients. No public
// STUN host is used by default: the operator sets HAMI_SFU_STUN_URL;
// the default is the local TURN host's STUN port so a NAT-simulated
// test never depends on the public internet.
const defaultSTUNURL = "stun:127.0.0.1:3478"

// stunICEConfiguration returns a WebRTC config with a STUN server so
// clients gather server-reflexive candidates.
func stunICEConfiguration(stunURL string) webrtc.Configuration {
	if strings.TrimSpace(stunURL) == "" {
		stunURL = defaultSTUNURL
	}
	return webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: []string{stunURL}}},
	}
}

// classifyICECandidateType parses a candidate line's "typ" field and
// returns host / srflx / relay / prflx / unknown. A server-reflexive
// candidate (typ srflx) is the 7.1 proof that STUN worked.
func classifyICECandidateType(candidate string) webrtc.ICECandidateType {
	// candidate:<foundation> <component> <transport> <priority> <ip> <port> typ <type> ...
	parts := strings.Fields(candidate)
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "typ" {
			switch parts[i+1] {
			case "host":
				return webrtc.ICECandidateTypeHost
			case "srflx":
				return webrtc.ICECandidateTypeSrflx
			case "relay":
				return webrtc.ICECandidateTypeRelay
			case "prflx":
				return webrtc.ICECandidateTypePrflx
			}
		}
	}
	return webrtc.ICECandidateTypeUnknown
}

// isServerReflexive reports whether a candidate came via STUN.
func isServerReflexive(candidate string) bool {
	return classifyICECandidateType(candidate) == webrtc.ICECandidateTypeSrflx
}

// stunBindingRequest builds a STUN binding request (RFC 8489) — the
// packet a client sends to learn its server-reflexive address. Used
// by the reference-machine NAT test; here it proves the request
// encodes/decodes round-trip in-process.
func stunBindingRequest() (*stun.Message, error) {
	msg := stun.New()
	msg.Type = stun.BindingRequest
	if err := msg.Build(); err != nil {
		return nil, err
	}
	return msg, nil
}

// --- 7.3 TURN over TCP / TLS ------------------------------------------------

// startSFUTURNTCPServer starts a pion/turn server on a TCP listener
// (turns TCP fallback on: a network that blocks UDP still connects
// over TCP). ListenerConfigs is the pion/turn v5 API for this.
func startSFUTURNTCPServer(addr, realm string, users map[string]string) (*sfuTURNServer, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen TURN TCP %s: %w", addr, err)
	}
	return startSFUTURNServerOnListener(listener, nil, realm, users)
}

// startSFUTURNTLSServer starts a pion/turn server on a TLS listener
// (TURN over TLS, normally port 443). tlsConfig must carry a valid
// certificate; MinVersion is forced to TLS 1.2 (no weak protocols —
// the same bar as job 8.9 for signaling).
func startSFUTURNTLSServer(addr, realm string, users map[string]string, tlsConfig *tls.Config) (*sfuTURNServer, error) {
	if tlsConfig == nil {
		return nil, errors.New("TURN over TLS needs a TLS config with a certificate")
	}
	cfg := tlsConfig.Clone()
	if cfg.MinVersion < tls.VersionTLS12 {
		cfg.MinVersion = tls.VersionTLS12
	}
	listener, err := tls.Listen("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("listen TURN TLS %s: %w", addr, err)
	}
	return startSFUTURNServerOnListener(listener, cfg, realm, users)
}

func startSFUTURNServerOnListener(listener net.Listener, _ *tls.Config, realm string, users map[string]string) (*sfuTURNServer, error) {
	if realm == "" {
		_ = listener.Close()
		return nil, errors.New("TURN realm is required")
	}
	if len(users) == 0 {
		_ = listener.Close()
		return nil, errors.New("TURN server needs at least one user")
	}
	relayIP := net.ParseIP("127.0.0.1")
	if tcpAddr, ok := listener.Addr().(*net.TCPAddr); ok && tcpAddr.IP != nil && !tcpAddr.IP.IsUnspecified() {
		relayIP = tcpAddr.IP
	}
	server, err := turn.NewServer(turn.ServerConfig{
		Realm: realm,
		AuthHandler: func(ra *turn.RequestAttributes) (string, []byte, bool) {
			password, exists := users[ra.Username]
			if !exists {
				return "", nil, false
			}
			return ra.Username, turn.GenerateAuthKey(ra.Username, ra.Realm, password), true
		},
		ListenerConfigs: []turn.ListenerConfig{
			{
				Listener: listener,
				RelayAddressGenerator: &turn.RelayAddressGeneratorPortRange{
					RelayAddress: relayIP,
					Address:      "0.0.0.0",
					MinPort:      sfuTURNRelayMinPort,
					MaxPort:      sfuTURNRelayMaxPort,
				},
			},
		},
	})
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("start TURN TCP/TLS server: %w", err)
	}
	// sfuTURNServer wraps a *turn.Server; its conn field is unused for
	// TCP listeners (Close closes the server, which closes listeners).
	return &sfuTURNServer{server: server, addr: listener.Addr(), realm: realm}, nil
}

// turnTimeLimitedCredentials: 7.2 — TURN credentials are time-limited.
// mintTURNTimeLimitedUsername returns a username of the form
// "<expiryUnix>:<user>" with the expiry embedded, so a stolen
// credential stops working at expiry. The TURN server's AuthHandler
// in production must reject expired usernames; the check itself is
// turnCredentialExpired (tested here) and the wiring point is
// documented in docs/cluster-c-results.md (the existing static-user
// AuthHandler in turn.go is unchanged — time-limited auth is additive).
func mintTURNTimeLimitedUsername(user string, validFor time.Duration, now time.Time) string {
	return fmt.Sprintf("%d:%s", now.Add(validFor).Unix(), user)
}

// turnCredentialExpired reports whether a time-limited TURN username
// has passed its embedded expiry. A username without an expiry prefix
// is treated as expired (fail closed).
func turnCredentialExpired(username string, now time.Time) bool {
	parts := strings.SplitN(username, ":", 2)
	if len(parts) != 2 || parts[0] == "" {
		return true
	}
	expiry, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || expiry <= 0 {
		return true
	}
	return !now.Before(time.Unix(expiry, 0))
}

// --- 7.4 ICE restart --------------------------------------------------------

// iceRestartDeadline is the stated recovery time for 7.4: switching
// interfaces mid-call must recover within 5 seconds.
const iceRestartDeadline = 5 * time.Second

type iceRestartState string

const (
	iceStateConnected    iceRestartState = "connected"
	iceStateDisconnected iceRestartState = "disconnected"
	iceStateRestarting   iceRestartState = "restarting"
	iceStateRecovered    iceRestartState = "recovered"
)

// iceRestartTracker models ICE restart on network change. The live
// part (re-gathering on a new interface) needs a second interface;
// the state machine + timing are tested here.
type iceRestartTracker struct {
	mu          sync.Mutex
	state       iceRestartState
	changedAt   time.Time
	recoveredAt time.Time
	restarts    int
}

func newICERestartTracker() *iceRestartTracker {
	return &iceRestartTracker{state: iceStateConnected}
}

// networkChanged starts a restart at now.
func (t *iceRestartTracker) networkChanged(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = iceStateRestarting
	t.changedAt = now
	t.restarts++
}

// recovered marks the restart complete at now. Returns whether the
// recovery met the stated deadline.
func (t *iceRestartTracker) recovered(now time.Time) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.recoveredAt = now
	t.state = iceStateRecovered
	return !now.After(t.changedAt.Add(iceRestartDeadline))
}

// recoveryDuration returns how long the last restart took.
func (t *iceRestartTracker) recoveryDuration() time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.recoveredAt.IsZero() {
		return 0
	}
	return t.recoveredAt.Sub(t.changedAt)
}

// --- 9.3 trickle ICE --------------------------------------------------------

// lateCandidateBuffer holds ICE candidates that arrive after the
// offer (trickle). They are applied in arrival order; a candidate
// that arrives after the answer still connects (job 9.3).
type lateCandidateBuffer struct {
	mu         sync.Mutex
	candidates []webrtc.ICECandidateInit
	applied    int
}

func newLateCandidateBuffer() *lateCandidateBuffer {
	return &lateCandidateBuffer{}
}

func (b *lateCandidateBuffer) add(c webrtc.ICECandidateInit) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.candidates = append(b.candidates, c)
}

// applyAll returns the buffered candidates in arrival order and marks
// them applied. The caller feeds each to PeerConnection.AddICECandidate.
func (b *lateCandidateBuffer) applyAll() []webrtc.ICECandidateInit {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := append([]webrtc.ICECandidateInit(nil), b.candidates...)
	b.applied += len(out)
	b.candidates = nil
	return out
}

func (b *lateCandidateBuffer) pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.candidates)
}

func (b *lateCandidateBuffer) appliedCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.applied
}
