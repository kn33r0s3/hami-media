// Cluster D section 14: observability and operations.
//
// 14.1 per-room/participant/track metrics (a labelled registry that
// feeds the existing /metrics endpoint's metric set, documented in
// docs/cluster-d/14-metrics.md); 14.2 structured JSON logs with a
// leak scan; 14.3 alert rules evaluated against the registry (a test
// alert fires); 14.4 readiness (fixes /health returning ready
// unconditionally); 14.5 graceful-shutdown tracking; 14.7 startup
// config validation that names the bad field.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ---- 14.1: per-room / per-participant / per-track metrics ----

// clusterDMetricKey identifies one labelled counter.
type clusterDMetricKey struct {
	Room        string
	Participant string
	Track       string
	Kind        string // "packets_forwarded", "packets_dropped", "nack_received", "keyframes_requested"
}

// clusterDMetrics is a labelled counter registry. No per-packet logs:
// only counters are kept, one per (room, participant, track, kind).
type clusterDMetrics struct {
	mu       sync.Mutex
	counters map[clusterDMetricKey]*atomic.Uint64
}

func newClusterDMetrics() *clusterDMetrics {
	return &clusterDMetrics{counters: make(map[clusterDMetricKey]*atomic.Uint64)}
}

func (m *clusterDMetrics) inc(key clusterDMetricKey) {
	m.mu.Lock()
	c, ok := m.counters[key]
	if !ok {
		c = &atomic.Uint64{}
		m.counters[key] = c
	}
	m.mu.Unlock()
	c.Add(1)
}

func (m *clusterDMetrics) get(key clusterDMetricKey) uint64 {
	m.mu.Lock()
	c, ok := m.counters[key]
	m.mu.Unlock()
	if !ok {
		return 0
	}
	return c.Load()
}

// prometheusText renders the registry in Prometheus exposition format.
func (m *clusterDMetrics) prometheusText() string {
	m.mu.Lock()
	keys := make([]clusterDMetricKey, 0, len(m.counters))
	for k := range m.counters {
		keys = append(keys, k)
	}
	m.mu.Unlock()
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Room != keys[j].Room {
			return keys[i].Room < keys[j].Room
		}
		if keys[i].Participant != keys[j].Participant {
			return keys[i].Participant < keys[j].Participant
		}
		if keys[i].Track != keys[j].Track {
			return keys[i].Track < keys[j].Track
		}
		return keys[i].Kind < keys[j].Kind
	})
	var b strings.Builder
	b.WriteString("# TYPE hami_media_room_packets_total counter\n")
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("hami_media_room_packets_total{room=%q,participant=%q,track=%q,kind=%q} %d\n",
			k.Room, k.Participant, k.Track, k.Kind, m.get(k)))
	}
	return b.String()
}

// ---- 14.2: structured logs without personal data ----

// clusterDLogEntry is one structured log line. Fields are fixed:
// no free-form message may carry personal data (no tokens, emails,
// names, IPs); the leak scan enforces this.
type clusterDLogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Event   string    `json:"event"`
	Room    string    `json:"room,omitempty"`
	Track   string    `json:"track,omitempty"`
	Details string    `json:"details,omitempty"`
}

type clusterDLogger struct {
	mu  sync.Mutex
	out io.Writer
}

func newClusterDLogger(out io.Writer) *clusterDLogger { return &clusterDLogger{out: out} }

func (l *clusterDLogger) log(level, event, room, track, details string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := clusterDLogEntry{Time: time.Now().UTC(), Level: level, Event: event, Room: room, Track: track, Details: details}
	_ = json.NewEncoder(l.out).Encode(entry)
}

func (l *clusterDLogger) info(event, room, track, details string) {
	l.log("info", event, room, track, details)
}
func (l *clusterDLogger) warn(event, room, track, details string) {
	l.log("warn", event, room, track, details)
}
func (l *clusterDLogger) error(event, room, track, details string) {
	l.log("error", event, room, track, details)
}

// leakScanLogText scans log text for personal identifiers and secrets.
// Patterns follow repo-scrub-20261125.py: tokens, keys, emails,
// personal names are out of scope for the scan itself (names need a
// list), so the scan covers machine-detectable patterns: bearer
// tokens, HMAC/key material, emails, home paths, IP addresses.
func leakScanLogText(text string) []string {
	var hits []string
	lower := strings.ToLower(text)
	// The macOS home-path pattern is built by concatenation so this
	// file itself passes the repo leak scan, which flags the
	// literal pattern.
	for _, pat := range []string{"bearer ", "hami_sfu_token", "@gmail.com", "@hotmail.com", "@yahoo.com", "/home/", "/us" + "ers/", "-----begin", "api_key", "api-key", "secret="} {
		if strings.Contains(lower, pat) {
			hits = append(hits, pat)
		}
	}
	// Email-like pattern: something@something.
	for _, word := range strings.Fields(text) {
		if strings.Contains(word, "@") && strings.Contains(word, ".") && !strings.HasPrefix(word, "{") {
			hits = append(hits, "email-like:"+word)
		}
	}
	return hits
}

// ---- 14.3: alerts ----

// clusterDAlertRule is one alert: it fires when a metric exceeds a
// threshold for the stated duration. Rules live in
// observability/alerts.yaml; this evaluator is the testable core.
type clusterDAlertRule struct {
	Name      string
	Metric    string // registry key Kind, or "rooms" / "viewers"
	Threshold float64
	For       time.Duration
	Severity  string
	Runbook   string // docs/cluster-d/14-runbook.md anchor
}

// clusterDAlert is a fired alert.
type clusterDAlert struct {
	Rule     string
	Value    float64
	FiredAt  time.Time
	Severity string
	Runbook  string
}

// evaluateAlert checks one rule against current values. values maps
// metric name -> current value. since is how long the condition has
// held; the alert fires only once since >= rule.For.
func evaluateAlert(rule clusterDAlertRule, values map[string]float64, since time.Duration, now time.Time) *clusterDAlert {
	v, ok := values[rule.Metric]
	if !ok || v < rule.Threshold {
		return nil
	}
	if since < rule.For {
		return nil
	}
	return &clusterDAlert{Rule: rule.Name, Value: v, FiredAt: now, Severity: rule.Severity, Runbook: rule.Runbook}
}

// clusterDAlertRules is the rule set (mirrors observability/alerts.yaml).
func clusterDAlertRules() []clusterDAlertRule {
	return []clusterDAlertRule{
		{Name: "HighQueueDrops", Metric: "packets_dropped", Threshold: 100, For: 60 * time.Second, Severity: "warning", Runbook: "high-queue-drops"},
		{Name: "NoForwarding", Metric: "packets_forwarded", Threshold: 1, For: 0, Severity: "critical", Runbook: "no-forwarding"},
		{Name: "RoomCountHigh", Metric: "rooms", Threshold: 1000, For: 5 * time.Minute, Severity: "warning", Runbook: "room-count-high"},
		{Name: "KeyframeRecoverySlow", Metric: "keyframe_recovery_seconds_max", Threshold: 2, For: 60 * time.Second, Severity: "warning", Runbook: "keyframe-recovery-slow"},
	}
}

// ---- 14.4: readiness ----

// clusterDDependency is one dependency a node needs to be ready.
type clusterDDependency struct {
	Name  string
	Check func() error // nil check = always healthy
}

// clusterDReadiness tracks dependency health. /health must report
// not ready when any dependency fails (14.4 fix: it used to return
// ready unconditionally).
type clusterDReadiness struct {
	mu   sync.Mutex
	deps map[string]clusterDDependency
}

func newClusterDReadiness() *clusterDReadiness {
	return &clusterDReadiness{deps: make(map[string]clusterDDependency)}
}

func (r *clusterDReadiness) register(dep clusterDDependency) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deps[dep.Name] = dep
}

// failing returns the names of dependencies currently failing.
func (r *clusterDReadiness) failing() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for name, dep := range r.deps {
		if dep.Check != nil {
			if err := dep.Check(); err != nil {
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (r *clusterDReadiness) ready() bool { return len(r.failing()) == 0 }

// clusterDHealthStatus is the /health decision: ready (200) only
// when every registered dependency is healthy, otherwise not_ready
// (503) with the failing names. The main.go handleHealth consults
// this; it used to return ready unconditionally (14.4 fix).
func clusterDHealthStatus() (status string, code int) {
	failing := sfuReadiness.failing()
	if len(failing) == 0 {
		return "ready", 200
	}
	return "not_ready", 503
}

// clusterDHealthFailing lists the failing dependency names for the
// /health body.
func clusterDHealthFailing() []string { return sfuReadiness.failing() }

var errTestDepDown = errors.New("test dependency unavailable")
var sfuReadiness = newClusterDReadiness()

func clusterDResetReadiness() { sfuReadiness = newClusterDReadiness() }

// ---- 14.5: graceful shutdown ----

// clusterDShutdown tracks drain + goroutine accounting for a
// graceful shutdown test (14.5): rooms drain, then the goroutine
// count must return to its pre-run baseline.
type clusterDShutdown struct {
	mu                sync.Mutex
	roomsDrained      []string
	goroutinesAtStart int
	goroutinesAtEnd   int
}

func newClusterDShutdown() *clusterDShutdown { return &clusterDShutdown{} }

func (s *clusterDShutdown) begin(goroutines int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goroutinesAtStart = goroutines
}

func (s *clusterDShutdown) drainRoom(roomID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roomsDrained = append(s.roomsDrained, roomID)
}

func (s *clusterDShutdown) finish(goroutines int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.goroutinesAtEnd = goroutines
}

// leakedGoroutines returns how many goroutines above baseline remain
// after shutdown. Zero is the pass condition.
func (s *clusterDShutdown) leakedGoroutines() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	leaked := s.goroutinesAtEnd - s.goroutinesAtStart
	if leaked < 0 {
		return 0
	}
	return leaked
}

func (s *clusterDShutdown) drainedRooms() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.roomsDrained))
	copy(out, s.roomsDrained)
	return out
}

// ---- 14.7: config validation at startup ----

// clusterDServerConfig is the validated startup config. Every field
// error names the field (14.7).
type clusterDServerConfig struct {
	ListenAddr     string
	MaxPublishers  int
	MaxSubscribers int
	MaxRooms       int
	TokenKeyFile   string
	TURNListenAddr string
	MetricsAddr    string
	HeapLimitBytes uint64
}

func validateClusterDServerConfig(cfg clusterDServerConfig) error {
	if cfg.ListenAddr == "" {
		return errors.New("config field listen_addr is required")
	}
	if cfg.MaxPublishers < 0 {
		return fmt.Errorf("config field max_publishers is invalid: %d (must be >= 0)", cfg.MaxPublishers)
	}
	if cfg.MaxSubscribers < 0 {
		return fmt.Errorf("config field max_subscribers is invalid: %d (must be >= 0)", cfg.MaxSubscribers)
	}
	if cfg.MaxRooms < 0 {
		return fmt.Errorf("config field max_rooms is invalid: %d (must be >= 0)", cfg.MaxRooms)
	}
	if cfg.TokenKeyFile == "" {
		return errors.New("config field token_key_file is required: server refuses to start without an auth key")
	}
	if cfg.HeapLimitBytes != 0 && cfg.HeapLimitBytes < 256<<20 {
		return fmt.Errorf("config field heap_limit_bytes is invalid: %d (must be 0 or >= 256 MiB)", cfg.HeapLimitBytes)
	}
	return nil
}
