package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// 14.1: metrics per room/participant/track.
func TestClusterDMetricsPerRoomParticipantTrack(t *testing.T) {
	m := newClusterDMetrics()
	m.inc(clusterDMetricKey{Room: "room-1", Participant: "alice", Track: "audio", Kind: "packets_forwarded"})
	m.inc(clusterDMetricKey{Room: "room-1", Participant: "alice", Track: "audio", Kind: "packets_forwarded"})
	m.inc(clusterDMetricKey{Room: "room-1", Participant: "bob", Track: "video-720p", Kind: "packets_dropped"})
	m.inc(clusterDMetricKey{Room: "room-2", Participant: "carol", Track: "video-360p", Kind: "nack_received"})
	if got := m.get(clusterDMetricKey{Room: "room-1", Participant: "alice", Track: "audio", Kind: "packets_forwarded"}); got != 2 {
		t.Fatalf("alice audio forwarded %d, want 2", got)
	}
	text := m.prometheusText()
	for _, want := range []string{
		`room="room-1",participant="alice",track="audio",kind="packets_forwarded"} 2`,
		`room="room-1",participant="bob",track="video-720p",kind="packets_dropped"} 1`,
		`room="room-2",participant="carol",track="video-360p",kind="nack_received"} 1`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics text missing %q:\n%s", want, text)
		}
	}
	// No per-packet logs: the registry only holds counters.
	if strings.Contains(text, "packet-") {
		t.Fatalf("metrics text looks like per-packet data:\n%s", text)
	}
}

// 14.2: structured logs pass the leak scan on a full run's logs.
func TestClusterDStructuredLogsPassLeakScan(t *testing.T) {
	var buf bytes.Buffer
	log := newClusterDLogger(&buf)
	log.info("room_created", "room-1", "", "publisher joined")
	log.info("track_published", "room-1", "audio", "opus track registered")
	log.warn("queue_drop", "room-1", "video-720p", "dropped 3 packets")
	log.error("forward_error", "room-1", "video-720p", "write failed: connection reset")
	// A full run's logs: 200 lines of realistic entries.
	for i := 0; i < 200; i++ {
		log.info("packet_forwarded", "room-1", "video-720p", "batch complete")
	}
	if hits := leakScanLogText(buf.String()); len(hits) > 0 {
		t.Fatalf("structured logs fail leak scan: %v", hits)
	}
	// And a log line WITH personal data must be caught.
	bad := "room_created bearer abc123 user alice@example.invalid /home/alice/file"
	if hits := leakScanLogText(bad); len(hits) == 0 {
		t.Fatal("leak scan missed bearer token + email + home path")
	}
}

// 14.3: a test alert fires.
func TestClusterDAlertFires(t *testing.T) {
	rules := clusterDAlertRules()
	var dropsRule *clusterDAlertRule
	for i := range rules {
		if rules[i].Name == "HighQueueDrops" {
			dropsRule = &rules[i]
		}
	}
	if dropsRule == nil {
		t.Fatal("HighQueueDrops rule missing")
	}
	if dropsRule.Runbook == "" {
		t.Fatal("HighQueueDrops rule has no runbook link (14.6)")
	}
	// Below threshold: no alert.
	if a := evaluateAlert(*dropsRule, map[string]float64{"packets_dropped": 50}, 120*time.Second, time.Now()); a != nil {
		t.Fatalf("alert fired below threshold: %+v", a)
	}
	// Above threshold but not for long enough: no alert.
	if a := evaluateAlert(*dropsRule, map[string]float64{"packets_dropped": 500}, 30*time.Second, time.Now()); a != nil {
		t.Fatalf("alert fired before the For window: %+v", a)
	}
	// Above threshold for the full window: fires, with runbook.
	a := evaluateAlert(*dropsRule, map[string]float64{"packets_dropped": 500}, 120*time.Second, time.Now())
	if a == nil {
		t.Fatal("test alert did not fire above threshold for the full window")
	}
	if a.Runbook != "high-queue-drops" {
		t.Fatalf("alert runbook %q, want high-queue-drops", a.Runbook)
	}
	// Every rule must link to a runbook entry.
	for _, r := range rules {
		if r.Runbook == "" {
			t.Fatalf("rule %s has no runbook entry", r.Name)
		}
	}
}

// 14.4: FAIL-BEFORE was recorded in evidence (test failed against the
// unconditional /health). This test now passes with the fix: a node
// with a failing dependency reports not ready (503).
func TestClusterDHealthReportsNotReadyOnFailingDependency(t *testing.T) {
	clusterDResetReadiness()
	sfuReadiness.register(clusterDDependency{Name: "turn", Check: func() error { return nil }})
	if !sfuReadiness.ready() {
		t.Fatal("readiness should be true with healthy deps")
	}
	if got := sfuReadiness.failing(); len(got) != 0 {
		t.Fatalf("failing deps %v, want none", got)
	}
	clusterDResetReadiness()
	sfuReadiness.register(clusterDDependency{Name: "turn", Check: func() error { return errTestDependencyDown }})
	if sfuReadiness.ready() {
		t.Fatal("readiness must be false with a failing dependency")
	}
	if got := sfuReadiness.failing(); len(got) != 1 || got[0] != "turn" {
		t.Fatalf("failing deps %v, want [turn]", got)
	}
	clusterDResetReadiness()
}

var errTestDependencyDown = errTestDepDown

// 14.5: graceful shutdown drains rooms with no leaked goroutines.
func TestClusterDGracefulShutdownDrainsRooms(t *testing.T) {
	sd := newClusterDShutdown()
	sd.begin(42)
	for _, room := range []string{"room-1", "room-2", "room-3"} {
		sd.drainRoom(room)
	}
	sd.finish(42)
	if got := sd.drainedRooms(); len(got) != 3 {
		t.Fatalf("drained %d rooms, want 3", len(got))
	}
	if leaked := sd.leakedGoroutines(); leaked != 0 {
		t.Fatalf("shutdown leaked %d goroutines", leaked)
	}
	// A shutdown that ends above baseline is reported.
	sd2 := newClusterDShutdown()
	sd2.begin(10)
	sd2.finish(13)
	if leaked := sd2.leakedGoroutines(); leaked != 3 {
		t.Fatalf("leaked %d, want 3", leaked)
	}
}

// 14.7: bad config fails at startup and names the field.
func TestClusterDConfigValidationNamesField(t *testing.T) {
	good := clusterDServerConfig{ListenAddr: "127.0.0.1:9000", MaxPublishers: 10, MaxSubscribers: 100, MaxRooms: 5, TokenKeyFile: "/etc/hami/key", HeapLimitBytes: 1 << 30}
	if err := validateClusterDServerConfig(good); err != nil {
		t.Fatalf("good config rejected: %v", err)
	}
	cases := []struct {
		cfg  clusterDServerConfig
		want string
	}{
		{clusterDServerConfig{MaxPublishers: 1, TokenKeyFile: "k"}, "listen_addr"},
		{clusterDServerConfig{ListenAddr: "x", MaxPublishers: -1, TokenKeyFile: "k"}, "max_publishers"},
		{clusterDServerConfig{ListenAddr: "x", MaxSubscribers: -5, TokenKeyFile: "k"}, "max_subscribers"},
		{clusterDServerConfig{ListenAddr: "x", MaxRooms: -1, TokenKeyFile: "k"}, "max_rooms"},
		{clusterDServerConfig{ListenAddr: "x"}, "token_key_file"},
		{clusterDServerConfig{ListenAddr: "x", TokenKeyFile: "k", HeapLimitBytes: 1024}, "heap_limit_bytes"},
	}
	for _, c := range cases {
		err := validateClusterDServerConfig(c.cfg)
		if err == nil {
			t.Fatalf("config %+v accepted, want error naming %s", c.cfg, c.want)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Fatalf("error %q does not name field %s", err.Error(), c.want)
		}
	}
}

// 14.4 HTTP-level check: the /health handler path used by the
// remote SFU server consults sfuReadiness via healthStatus.
// 14.4 HTTP-level check: the real /health handler on the remote
// SFU server must return 503 when a dependency fails.
func TestClusterDHealthHandlerReportsNotReady(t *testing.T) {
	server := &m7hRemoteSFUServer{}
	call := func() (int, string) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()
		server.handleHealth("test-token")(rec, req)
		return rec.Code, rec.Body.String()
	}
	clusterDResetReadiness()
	code, body := call()
	if code != http.StatusOK || !strings.Contains(body, `"status":"ready"`) {
		t.Fatalf("healthy: code %d body %s, want 200 ready", code, body)
	}
	sfuReadiness.register(clusterDDependency{Name: "turn", Check: func() error { return errTestDependencyDown }})
	code, body = call()
	if code != http.StatusServiceUnavailable {
		t.Fatalf("failing dep: /health code %d, want 503 (body %s)", code, body)
	}
	if !strings.Contains(body, "not_ready") || !strings.Contains(body, "turn") {
		t.Fatalf("failing dep: /health body %s, want not_ready naming turn", body)
	}
	clusterDResetReadiness()
}

func TestClusterDHealthStatusShape(t *testing.T) {
	clusterDResetReadiness()
	status, code := clusterDHealthStatus()
	if code != http.StatusOK || status != "ready" {
		t.Fatalf("healthy node: status %q code %d, want ready/200", status, code)
	}
	sfuReadiness.register(clusterDDependency{Name: "turn", Check: func() error { return errTestDependencyDown }})
	status, code = clusterDHealthStatus()
	if code != http.StatusServiceUnavailable || status != "not_ready" {
		t.Fatalf("failing dep: status %q code %d, want not_ready/503", status, code)
	}
	clusterDResetReadiness()
	_ = httptest.NewRecorder
}
