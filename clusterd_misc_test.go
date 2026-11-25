package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

// 13.7: WHIP ingest — an encoder posts an SDP offer and gets an answer.
func TestClusterDWHIPIngestOfferAnswer(t *testing.T) {
	api := webrtc.NewAPI()
	var gotTrack bool
	h := newWHIPIngestHandler(api)
	h.onTrack = func(roomID string, track *webrtc.TrackRemote) {
		if roomID == "room-whip" {
			gotTrack = true
		}
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Encoder side: a Pion PeerConnection offering one video track.
	enc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264}, "video", "encoder-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := enc.AddTrack(track); err != nil {
		t.Fatal(err)
	}
	offer, err := enc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := enc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/whip/room-whip", strings.NewReader(offer.SDP))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/sdp")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("WHIP POST status %d, want 201", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/sdp" {
		t.Fatalf("WHIP answer Content-Type %q, want application/sdp", ct)
	}
	buf := make([]byte, 1<<20)
	n, _ := resp.Body.Read(buf)
	answerSDP := string(buf[:n])
	if !strings.Contains(answerSDP, "v=0") || !strings.Contains(answerSDP, "a=setup:") {
		t.Fatalf("WHIP answer is not SDP:\n%s", answerSDP)
	}
	if err := enc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answerSDP}); err != nil {
		t.Fatalf("encoder could not accept WHIP answer: %v", err)
	}
	_ = gotTrack // track arrival needs ICE connectivity (VM UDP-blocked); handshake is the gate here.
	_ = time.Second
}

// 13.7: WHIP rejects wrong content type and missing room.
func TestClusterDWHIPRejectsBadRequests(t *testing.T) {
	api := webrtc.NewAPI()
	h := newWHIPIngestHandler(api)
	srv := httptest.NewServer(h)
	defer srv.Close()
	// Missing room.
	resp, err := http.Post(srv.URL+"/whip/", "application/sdp", strings.NewReader("v=0"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing room: status %d, want 400", resp.StatusCode)
	}
	// Wrong content type.
	resp, err = http.Post(srv.URL+"/whip/room-1", "text/plain", strings.NewReader("v=0"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("wrong content type: status %d, want 415", resp.StatusCode)
	}
}

// 13.3: edge-simulator CDN measurement (real CDN: NEEDS-APPROVAL).
func TestClusterDEdgeSimulatorHitRatioAndStartTime(t *testing.T) {
	edge := newClusterDEdgeSimulator()
	edge.putOrigin("/media.m3u8", []byte("#EXTM3U\n#EXT-X-VERSION:7\n"))
	for i := 0; i < 5; i++ {
		edge.putOrigin("/seg-"+itoa(i)+".m4s", []byte{0x00, 0x00, 0x00, byte(i), 'm', 'o', 'o', 'f'})
	}
	segs := []string{"/seg-0.m4s", "/seg-1.m4s", "/seg-2.m4s", "/seg-3.m4s", "/seg-4.m4s"}
	m, err := edge.measureCDN("/media.m3u8", segs)
	if err != nil {
		t.Fatal(err)
	}
	// 6 cold requests (1 playlist + 5 segments) + 6 warm requests.
	if m.Requests != 12 {
		t.Fatalf("requests %d, want 12", m.Requests)
	}
	if m.Hits != 6 || m.Misses != 6 {
		t.Fatalf("hits %d misses %d, want 6/6", m.Hits, m.Misses)
	}
	if m.HitRatio != 0.5 {
		t.Fatalf("hit ratio %g, want 0.5", m.HitRatio)
	}
	// A third pass is all hits: ratio must rise.
	m2, err := edge.measureCDN("/media.m3u8", segs)
	if err != nil {
		t.Fatal(err)
	}
	if m2.HitRatio <= m.HitRatio {
		t.Fatalf("hit ratio did not rise on warm edge: %g -> %g", m.HitRatio, m2.HitRatio)
	}
	if m.StartTime < 0 {
		t.Fatal("negative start time")
	}
}

// 13.4: glass-to-glass + start-time method.
func TestClusterDLatencyMethod(t *testing.T) {
	g := &clusterDGlassToGlass{}
	base := time.Now()
	// Simulate 10 frames, 40 ms capture-to-render each.
	for i := 0; i < 10; i++ {
		cap := base.Add(time.Duration(i) * 33 * time.Millisecond)
		s := g.measureFrame(cap, cap.Add(40*time.Millisecond))
		if s.Duration != 40*time.Millisecond {
			t.Fatalf("frame latency %s, want 40ms", s.Duration)
		}
		if s.ClockNote != "single-clock" {
			t.Fatalf("clock note %q, want single-clock (VM)", s.ClockNote)
		}
	}
	var samples []time.Duration
	for _, s := range g.muSamples {
		samples = append(samples, s.Duration)
	}
	rep, err := summarizeLatency("webrtc-glass-to-glass", samples, "single-clock")
	if err != nil {
		t.Fatal(err)
	}
	if rep.P50 != 40*time.Millisecond || rep.P99 != 40*time.Millisecond {
		t.Fatalf("summary wrong: %s", formatLatencyReport(rep))
	}
	// HLS start time = playlist fetch + segment download + one segment.
	if got := hlsStartTime(50*time.Millisecond, 100*time.Millisecond, 2*time.Second); got != 2150*time.Millisecond {
		t.Fatalf("HLS start time %s, want 2.15s", got)
	}
	// HLS segment latency: packager entry -> playlist listing.
	if got := hlsSegmentLatency(base, base.Add(2100*time.Millisecond)); got != 2100*time.Millisecond {
		t.Fatalf("HLS segment latency %s, want 2.1s", got)
	}
}

// 10.6: client error reports pass the leak scan; bad ones are rejected.
func TestClusterDClientErrorReports(t *testing.T) {
	good := clusterDClientErrorReport{
		ErrorCode: "ICE_FAILED", SDKVersion: "0.1.0", Platform: "web",
		RoomID: "room-1", At: time.Now(), Detail: "ice connection failed after 5s",
	}
	if err := validateClientErrorReport(good); err != nil {
		t.Fatalf("good report rejected: %v", err)
	}
	bad := []clusterDClientErrorReport{
		{ErrorCode: "ice failed", SDKVersion: "0.1.0", Platform: "web", At: time.Now()},
		{ErrorCode: "ICE_FAILED", SDKVersion: "", Platform: "web", At: time.Now()},
		{ErrorCode: "ICE_FAILED", SDKVersion: "0.1.0", Platform: "windows", At: time.Now()},
		{ErrorCode: "ICE_FAILED", SDKVersion: "0.1.0", Platform: "web", At: time.Now(), Detail: "token bearer abc123 failed"},
		{ErrorCode: "ICE_FAILED", SDKVersion: "0.1.0", Platform: "web", At: time.Now(), Detail: "contact alice@example.invalid"},
		{ErrorCode: "ICE_FAILED", SDKVersion: "0.1.0", Platform: "web", At: time.Now(), Detail: "sdp: v=0 candidate:1 1 udp 123"},
	}
	for i, rep := range bad {
		if err := validateClientErrorReport(rep); err == nil {
			t.Fatalf("bad report %d accepted: %+v", i, rep)
		}
	}
	// Scrubber is a safety net, not a pass: scrubbed text must still
	// validate.
	scrubbed := scrubClientDetail("failed for alice@example.invalid bearer xyz /home/alice/x")
	if strings.Contains(scrubbed, "alice@example.invalid") || strings.Contains(scrubbed, "bearer xyz") || strings.Contains(scrubbed, "/home/alice") {
		t.Fatalf("scrubber missed personal data: %q", scrubbed)
	}
	if err := validateClientErrorReport(clusterDClientErrorReport{ErrorCode: "ICE_FAILED", SDKVersion: "0.1.0", Platform: "web", At: time.Now(), Detail: scrubbed}); err != nil {
		t.Fatalf("scrubbed report rejected: %v", err)
	}
}
