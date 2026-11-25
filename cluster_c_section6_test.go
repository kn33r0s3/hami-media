package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 6.1: one profile config file, read by SDP/queues/caches.
func TestClusterCProfileConfigFile(t *testing.T) {
	cfg, err := loadProfileConfig("")
	if err != nil {
		t.Fatalf("default profile config: %v", err)
	}
	if len(cfg.Profiles) != 3 {
		t.Fatalf("profiles = %d, want 3", len(cfg.Profiles))
	}
	byName := cfg.byName()
	top, mid, base := byName["top"], byName["middle"], byName["base"]
	if top.Width != 1280 || top.Height != 720 || top.FPS != 30 || top.MaxKbps != 1200 {
		t.Fatalf("top = %+v, want 1280x720@30 max 1200 kbps", top)
	}
	if mid.Width != 640 || mid.Height != 360 || mid.FPS != 30 {
		t.Fatalf("middle = %+v, want 640x360@30", mid)
	}
	if base.Width != 320 || base.Height != 180 || base.FPS != 15 {
		t.Fatalf("base = %+v, want 320x180@15", base)
	}
	for _, p := range cfg.Profiles {
		if p.KeyframeS != 2 {
			t.Fatalf("profile %s keyframe = %ds, want 2s", p.Name, p.KeyframeS)
		}
	}
	// File round-trip: write the default ladder to a file, read it back.
	dir := t.TempDir()
	path := filepath.Join(dir, "profiles.json")
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := loadProfileConfig(path)
	if err != nil {
		t.Fatalf("load profile file: %v", err)
	}
	if got.byName()["top"].MaxKbps != 1200 {
		t.Fatalf("file top max_kbps = %d, want 1200", got.byName()["top"].MaxKbps)
	}
	// Bad file fails closed.
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"version":1,"profiles":[{"name":"top"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadProfileConfig(bad); err == nil {
		t.Fatal("invalid profile file accepted")
	}
	if _, err := loadProfileConfig(filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing profile file accepted")
	}
	t.Logf("6.1 profiles: top 720p30/1200kbps middle 360p30/600kbps base 180p15/200kbps, keyframe 2s")
}

// 6.2: profile-level-id negotiation. Check each target client's value
// BEFORE fixing ours: 42001f (Baseline, no constraints) vs 42e01f
// (Constrained Baseline) are the same profile+level — they must
// negotiate, not reject. 4d001f (Main) must NOT negotiate with ours.
func TestClusterCProfileLevelIDNegotiation(t *testing.T) {
	cases := []struct {
		name    string
		offered []string
		want    string
		wantOK  bool
	}{
		{"same value 42001f", []string{"42001f"}, "42001f", true},
		{"constrained baseline 42e01f negotiates with 42001f", []string{"42e01f"}, "42001f", true},
		{"both offered, constrained first", []string{"42e01f", "42001f"}, "42001f", true},
		{"main profile 4d001f does not match baseline", []string{"4d001f"}, "", false},
		{"high profile 64001f does not match baseline", []string{"64001f"}, "", false},
		{"lower level 42001e negotiates down", []string{"42001e"}, "42001e", true},
		{"higher level 420028 exceeds ours", []string{"420028"}, "", false},
		{"garbage rejected", []string{"zzzzzz", "42"}, "", false},
		{"empty offer rejected", nil, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := negotiateProfileLevelID("42001f", c.offered)
			if ok != c.wantOK || got != c.want {
				t.Fatalf("negotiate(42001f, %v) = (%q,%v), want (%q,%v)", c.offered, got, ok, c.want, c.wantOK)
			}
		})
	}
	t.Logf("6.2: 42001f and 42e01f are the same Baseline profile at level 3.1; constraint flags are ignored in negotiation")
}

// 6.5 + 7.6: layer selection from bandwidth estimate, incl. caps.
func TestClusterCLayerSelectionFromBandwidth(t *testing.T) {
	cfg, _ := loadProfileConfig("")
	cases := []struct {
		kbps int
		want string
	}{
		{2000, "top"}, {1200, "top"}, {1000, "top"}, {700, "top"},
		{500, "middle"}, // 7.6: 500 kbps cap -> middle (min 300), not top
		{300, "middle"}, {250, "base"}, {100, "base"}, {0, "base"}, {-5, "base"},
	}
	for _, c := range cases {
		if got := selectLayerForBandwidth(cfg, c.kbps); got.Name != c.want {
			t.Fatalf("selectLayer(%d kbps) = %s, want %s", c.kbps, got.Name, c.want)
		}
	}
	// 7.6 explicitly: 500 kbps, 1 Mbps, 2 Mbps caps each select a layer
	// whose minimum the cap sustains (top at 1/2 Mbps, middle at 500).
	for _, tc := range []struct {
		cap, wantMin int
		want         string
	}{{500, 300, "middle"}, {1000, 700, "top"}, {2000, 700, "top"}} {
		layer := selectLayerForBandwidth(cfg, tc.cap)
		if layer.Name != tc.want || layer.MinKbps != tc.wantMin {
			t.Fatalf("cap %d kbps -> %s, want %s", tc.cap, layer.Name, tc.want)
		}
		t.Logf("7.6 cap %d kbps -> layer %s (min %d kbps, max %d kbps)", tc.cap, layer.Name, layer.MinKbps, layer.MaxKbps)
	}
}

// 6.6: keyframe-aligned switching, no gap > 1 GOP (2 s).
func TestClusterCKeyframeAlignedSwitching(t *testing.T) {
	now := time.Now()
	sw := requestLayerSwitch("middle", "top", now)
	if sw.applyAtKeyframe(false, now.Add(100*time.Millisecond)) {
		t.Fatal("switch took effect on a non-keyframe packet")
	}
	if !sw.applyAtKeyframe(true, now.Add(300*time.Millisecond)) {
		t.Fatal("switch did not take effect at a keyframe")
	}
	if !sw.switchGapOK(keyframeInterval) {
		t.Fatal("switch gap exceeded one GOP")
	}
	// A switch that waits past one GOP fails the Done line.
	late := requestLayerSwitch("top", "middle", now)
	late.applyAtKeyframe(true, now.Add(3*time.Second))
	if late.switchGapOK(keyframeInterval) {
		t.Fatal("switch with a 3 s gap passed the 1-GOP check")
	}
	t.Logf("6.6 switch gap 300ms <= 1 GOP (2s); 3s gap correctly fails")
}

// 6.12: publisher bitrate control at a 1 Mbps cap.
func TestClusterCPublisherBitrateControl(t *testing.T) {
	cfg, _ := loadProfileConfig("")
	top, _ := cfg.profile("top")
	// 1 Mbps cap: use a profile capped at 1000 kbps.
	capped := top
	capped.MaxKbps = 1000
	capped.MinKbps = 700
	now := time.Now()
	ctrl := newBitrateController(capped, now)
	if ctrl.targetKbps() != 1000 {
		t.Fatalf("initial target = %d, want 1000", ctrl.targetKbps())
	}
	// Sending 1000 kbps for a second is allowed; one byte more is not.
	if !ctrl.allow(125000, now) {
		t.Fatal("1 Mbps for one second should be allowed at a 1 Mbps cap")
	}
	ctrl.sent(125000, now)
	if ctrl.allow(1, now) {
		t.Fatal("byte over the 1 Mbps cap was allowed in the same second")
	}
	// Congestion feedback reduces the target; it never drops below min.
	for i := 0; i < 10; i++ {
		ctrl.onCongestion()
	}
	if got := ctrl.targetKbps(); got != capped.MinKbps {
		t.Fatalf("target after sustained congestion = %d, want floor %d", got, capped.MinKbps)
	}
	// Clean intervals recover to the cap.
	for i := 0; i < 30; i++ {
		ctrl.onCleanInterval()
	}
	if got := ctrl.targetKbps(); got != 1000 {
		t.Fatalf("target after recovery = %d, want 1000", got)
	}
	t.Logf("6.12: 1 Mbps cap enforced, congestion reduces to 700 kbps floor, clean intervals recover to 1000")
}

// 6.3: keyframe every 2 s + SPS/PPS at each keyframe.
func TestClusterCKeyframeIntervalAndParameterSets(t *testing.T) {
	last := time.Now()
	if keyframeDue(last, last.Add(1500*time.Millisecond)) {
		t.Fatal("keyframe due at 1.5s with a 2s interval")
	}
	if !keyframeDue(last, last.Add(2*time.Second)) {
		t.Fatal("keyframe not due at 2s")
	}
	// STAP-A carrying SPS+PPS+IDR.
	stap := []byte{0x78, 0, 3, 0x67, 0x42, 0x00, 0, 4, 0x68, 0xce, 0x38, 0x80, 0, 2, 0x65, 0x88}
	if sps, pps := ensureParameterSets(stap); !sps || !pps {
		t.Fatalf("STAP-A SPS=%v PPS=%v, want both true", sps, pps)
	}
	// A bare IDR without SPS/PPS fails the parameter-set check.
	if frameHasParameterSets([][]byte{{0x65, 0x88, 0x84}}) {
		t.Fatal("bare IDR passed the SPS/PPS check")
	}
	// SPS+PPS in earlier packets of the same frame + IDR passes.
	if !frameHasParameterSets([][]byte{{0x67, 0x42, 0x00, 0x1e}, {0x68, 0xce, 0x38, 0x80}, {0x65, 0x88}}) {
		t.Fatal("SPS+PPS+IDR frame failed the parameter-set check")
	}
	t.Logf("6.3: keyframe interval 2s; SPS+PPS required with each keyframe (STAP-A and split-NAL forms)")
}

// 6.4: simulcast ingest — three-layer publisher accepted, each layer
// tracked separately with its own SSRC.
func TestClusterCSimulcastIngest(t *testing.T) {
	tr := newSimulcastTracker()
	now := time.Now()
	for _, layer := range []string{"top", "middle", "base"} {
		key := sfuTrackKey{PublisherID: "pub-1", TrackID: "track-1", Layer: layer}
		if err := tr.registerLayer(key, uint32(1000+len(layer)), now); err != nil {
			t.Fatalf("register %s: %v", layer, err)
		}
	}
	if got := tr.layerCount("pub-1", "track-1"); got != 3 {
		t.Fatalf("tracked layers = %d, want 3", got)
	}
	// Per-layer packet accounting is separate.
	for i, layer := range []string{"top", "middle", "base"} {
		key := sfuTrackKey{PublisherID: "pub-1", TrackID: "track-1", Layer: layer}
		for seq := 1; seq <= 5; seq++ {
			if err := tr.recordPacket(key, uint16(seq+i*10), seq == 1, now); err != nil {
				t.Fatalf("record %s: %v", layer, err)
			}
		}
		st, ok := tr.state(key)
		if !ok || st.Packets != 5 || st.Keyframes != 1 {
			t.Fatalf("layer %s state = %+v, want 5 packets 1 keyframe", layer, st)
		}
	}
	// Same layer re-registered with a different SSRC is a collision.
	key := sfuTrackKey{PublisherID: "pub-1", TrackID: "track-1", Layer: "top"}
	if err := tr.registerLayer(key, 99999, now); err == nil {
		t.Fatal("SSRC collision on a registered layer was accepted")
	}
	// Unknown layer rejected; unregistered layer cannot receive packets.
	if err := tr.registerLayer(sfuTrackKey{PublisherID: "pub-1", TrackID: "track-1", Layer: "ultra"}, 1, now); err == nil {
		t.Fatal("unknown layer accepted")
	}
	if err := tr.recordPacket(sfuTrackKey{PublisherID: "pub-1", TrackID: "track-1", Layer: "base"}, 99, false, now); err != nil {
		t.Fatalf("base record after top collision check: %v", err)
	}
	if err := tr.recordPacket(sfuTrackKey{PublisherID: "pub-9", TrackID: "track-9", Layer: "base"}, 1, false, now); err == nil {
		t.Fatal("packet for an unregistered layer was accepted")
	}
	t.Logf("6.4: three layers registered and tracked separately (own SSRC each); overlap with 2.6 flagged — this tracker does not touch sfuTrackKey")
}

// 6.7: bandwidth estimate tracks an impaired link within ±20%.
func TestClusterCBandwidthEstimateTracksImpairedLink(t *testing.T) {
	est := newBandwidthEstimator(1200)
	// Clean 1.2 Mbps link: estimate stays near 1200.
	for i := 0; i < 5; i++ {
		est.update(1200, 0.0, 40)
	}
	if got := est.estimate(); got < 960 || got > 1440 {
		t.Fatalf("clean-link estimate = %d kbps, want within ±20%% of 1200", got)
	}
	// Impaired link: true capacity 600 kbps, 3% loss, 120 ms RTT.
	// Loss cut: 3% -> 5% cut; smoothed estimate must land within ±20% of 600.
	for i := 0; i < 10; i++ {
		est.update(600, 0.03, 120)
	}
	if got := est.estimate(); got < 480 || got > 720 {
		t.Fatalf("impaired-link estimate = %d kbps, want within ±20%% of 600", got)
	}
	// Severe loss collapses the estimate but never below the 50 kbps floor.
	for i := 0; i < 10; i++ {
		est.update(100, 0.20, 300)
	}
	if got := est.estimate(); got < 50 {
		t.Fatalf("severe-loss estimate = %d kbps, want >= 50 floor", got)
	}
	t.Logf("6.7: estimate tracks clean 1200 and impaired 600 kbps links within ±20%% (stated tolerance)")
}

// 6.8: dynacast — zero-subscriber layer sends no packets.
func TestClusterCDynacastStopsIdleLayers(t *testing.T) {
	g := newDynacastGate()
	// Nobody subscribes to top: 100 packets suppressed, 0 sent.
	for i := 0; i < 100; i++ {
		if g.send("top") {
			t.Fatal("packet sent on a layer with zero subscribers")
		}
	}
	if sent, sup := g.counts("top"); sent != 0 || sup != 100 {
		t.Fatalf("top sent=%d suppressed=%d, want 0/100", sent, sup)
	}
	g.subscribe("top")
	for i := 0; i < 10; i++ {
		if !g.send("top") {
			t.Fatal("packet suppressed on a subscribed layer")
		}
	}
	g.unsubscribe("top")
	if g.send("top") {
		t.Fatal("packet sent after the last subscriber left")
	}
	t.Logf("6.8: zero-subscriber layer sent 0/100 packets; subscribed layer sent 10/10")
}

// 6.11: PLI/FIR -> keyframe within the stated 500 ms (VM-testable part).
func TestClusterCKeyframeRequestHonoured(t *testing.T) {
	tr := &keyframeRequestTracker{}
	now := time.Now()
	tr.request(now)
	if !tr.keyframe(now.Add(200 * time.Millisecond)) {
		t.Fatal("keyframe did not fulfil the request")
	}
	if req, ok, late := tr.stats(); req != 1 || ok != 1 || late != 0 {
		t.Fatalf("stats = %d/%d/%d, want 1/1/0", req, ok, late)
	}
	// A keyframe after the deadline counts as late, not fulfilled.
	tr.request(now.Add(time.Second))
	tr.keyframe(now.Add(2 * time.Second))
	if req, ok, late := tr.stats(); req != 2 || ok != 1 || late != 1 {
		t.Fatalf("stats = %d/%d/%d, want 2/1/1", req, ok, late)
	}
	t.Logf("6.11 VM part: PLI/FIR -> keyframe in 200ms (deadline 500ms); 1s-late keyframe counted late. Device-matrix part BLOCKED (no devices, job 10.5)")
}

// 6.9: codec set per room.
func TestClusterCCodecSetPerRoom(t *testing.T) {
	if got, err := codecForRoom("room-1", "H264"); err != nil || got != "H264" {
		t.Fatalf("H264 for room-1: (%q, %v), want (H264, nil)", got, err)
	}
	for _, codec := range []string{"VP8", "VP9", "AV1"} {
		if _, err := codecForRoom("room-1", codec); err == nil {
			t.Fatalf("codec %s accepted, want unsupported error", codec)
		}
		if isSupportedCodec(codec) {
			t.Fatalf("codec %s reported supported", codec)
		}
	}
	if _, err := codecForRoom("", "H264"); err == nil {
		t.Fatal("empty room accepted")
	}
	t.Logf("6.9: H264 supported end to end; VP8/VP9/AV1 listed unsupported (docs/cluster-c-codecs.md)")
}
