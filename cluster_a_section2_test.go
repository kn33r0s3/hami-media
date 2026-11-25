// Cluster A section-2 tests (job list 2026-11-25, jobs 2.1-2.10).
// Written failing-first: each test names the job it covers and the
// condition from that job's Done line. Tests that need a new production
// helper fail to compile until the helper exists; that compile failure
// is itself the fail-before record for those jobs (see
// docs/archive/joblist/cluster-a/fail-before-2.1-2.10.log).
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/srtp/v3"
	"github.com/pion/webrtc/v4"
)

// goroutineCountSoon polls until the goroutine count is at most want,
// or fails. Writers and shard runners exit asynchronously after their
// queue is closed, so a single immediate read would be flaky.
func goroutineCountSoon(t *testing.T, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if got := runtime.NumGoroutine(); got <= want {
			return
		} else if time.Now().After(deadline) {
			t.Fatalf("goroutine count = %d, want at most %d after %s", got, want, timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// testClusterAServer builds a signalling server like
// testSFUHTTPServerWithLimits but returns the server and router too,
// so tests can close the router, plant synthetic tracks, and drive
// /cleanup. It registers /publisher, /viewer and /cleanup.
func testClusterAServer(t *testing.T, limits capacityLimits) (*httptest.Server, *m7hRemoteSFUServer, *sfuRouter, *sfuAuthVerifier) {
	t.Helper()
	verifier := testSFUAuthVerifier(t)
	settings, mux, err := loopbackSettings(true, 0)
	if err != nil {
		t.Fatalf("loopback settings: %v", err)
	}
	t.Cleanup(func() {
		if mux != nil {
			_ = mux.Close()
		}
	})
	api := webrtc.NewAPI(webrtc.WithSettingEngine(settings))
	router, err := newSFURouter(api)
	if err != nil {
		t.Fatalf("newSFURouter: %v", err)
	}
	router.m7hPublisherReceiveStats = make(map[string]*m7hPublisherReceiveStats)
	t.Cleanup(func() { _ = router.close() })
	server := &m7hRemoteSFUServer{
		apis:            []*webrtc.API{api},
		muxByShard:      []ice.UDPMux{mux},
		settingsByShard: []webrtc.SettingEngine{settings},
		publishers:      make(map[string]m7hRemotePublisherState),
		viewers:         make(map[string]viewerEndpoint),
		auth:            verifier,
		maxPublishers:   limits.MaxPublishers,
		maxSubscribers:  limits.MaxSubscribers,
	}
	server.router = router
	m := http.NewServeMux()
	m.HandleFunc("/publisher", server.handlePublisher("test-bearer"))
	m.HandleFunc("/viewer", server.handleViewer("test-bearer"))
	m.HandleFunc("/cleanup", server.handleCleanup("test-bearer"))
	httpServer := httptest.NewServer(m)
	t.Cleanup(httpServer.Close)
	return httpServer, server, router, verifier
}

// testClusterAPublish posts one /publisher request and returns status/body.
func testClusterAPublish(t *testing.T, httpServer *httptest.Server, verifier *sfuAuthVerifier, publisherID, offer string) (int, string) {
	t.Helper()
	token, err := verifier.mintSFUToken("default", publisherID, []string{sfuTokenRolePublish}, time.Hour)
	if err != nil {
		t.Fatalf("mint publish token for %s: %v", publisherID, err)
	}
	return testSFUPostJSON(t, httpServer.URL+"/publisher", "test-bearer", m7hRemoteSDPRequest{
		PublisherID: publisherID,
		Offer:       offer,
		Token:       token,
	})
}

// --- 2.1 SRTP replay protection ------------------------------------------

// TestSRTPReplayProtectionDropsDuplicate (job 2.1): a duplicate RTP
// packet must reach the receiver exactly once when replay protection
// is on. The receiver's remote options come from the same gate the
// three production call sites use (srtpReplayProtectionDisabled),
// so this test FAILS on the hami_disable_srtp_replay build (protection
// off: the duplicate is delivered) and PASSES on the default build.
// It runs over net.Pipe, so it works in the VM sandbox (no UDP).
func TestSRTPReplayProtectionDropsDuplicate(t *testing.T) {
	masterKey := []byte{0xE1, 0xF9, 0x7A, 0x0D, 0x3E, 0x01, 0x8B, 0xE0, 0xD6, 0x4F, 0xA3, 0x2C, 0x06, 0xDE, 0x41, 0x39}
	masterSalt := []byte{0x0E, 0xC6, 0x75, 0xAD, 0x49, 0x8A, 0xFE, 0xEB, 0xB6, 0x96, 0x0B, 0x3A, 0xAB, 0xE6}
	keys := srtp.SessionKeys{
		LocalMasterKey:   masterKey,
		LocalMasterSalt:  masterSalt,
		RemoteMasterKey:  masterKey,
		RemoteMasterSalt: masterSalt,
	}
	aPipe, bPipe := net.Pipe()
	sender, err := srtp.NewSessionSRTP(aPipe, &srtp.Config{
		Profile: srtp.ProtectionProfileAes128CmHmacSha1_80,
		Keys:    keys,
	})
	if err != nil {
		t.Fatalf("sender SRTP session: %v", err)
	}
	defer sender.Close()
	receiver, err := srtp.NewSessionSRTP(bPipe, &srtp.Config{
		Profile:       srtp.ProtectionProfileAes128CmHmacSha1_80,
		Keys:          keys,
		RemoteOptions: srtpRemoteReplayOptions(),
	})
	if err != nil {
		t.Fatalf("receiver SRTP session: %v", err)
	}
	defer receiver.Close()

	writeStream, err := sender.OpenWriteStream()
	if err != nil {
		t.Fatalf("open SRTP write stream: %v", err)
	}
	header := &rtp.Header{SSRC: 5000, SequenceNumber: 100, Timestamp: 90000, PayloadType: 102}
	payload := []byte{0x00, 0x01, 0x03, 0x04}
	// Writes and reads are interleaved: the sessions run over a
	// synchronous net.Pipe, so two writes before any read can
	// deadlock (the receiver's read loop blocks delivering the
	// first packet while the sender blocks writing the second).
	writeOnce := func() {
		t.Helper()
		if _, err := writeStream.WriteRTP(header, append([]byte(nil), payload...)); err != nil {
			t.Fatalf("write RTP: %v", err)
		}
	}
	writeOnce()

	readStream, ssrc, err := receiver.AcceptStream()
	if err != nil {
		t.Fatalf("accept SRTP read stream: %v", err)
	}
	if ssrc != 5000 {
		t.Fatalf("read stream SSRC = %d, want 5000", ssrc)
	}
	buf := make([]byte, 1500)
	if err := readStream.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	n, err := readStream.Read(buf)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if n < 12 || !bytes.Equal(buf[12:n], payload) {
		t.Fatalf("first read = %d bytes %x, want payload %x", n, buf[:n], payload)
	}
	// Write the identical packet again: this is the replay.
	writeOnce()
	// The replay must never arrive: a second read must time out.
	if err := readStream.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("set second read deadline: %v", err)
	}
	if n, err := readStream.Read(buf); err == nil {
		t.Fatalf("replay packet delivered (%d bytes); replay protection is off", n)
	}
}

// --- 2.2 upstreamQueue leak ----------------------------------------------

// testUpstreamTrack builds a synthetic published track with no live
// remote: the upstream writer only needs the queue and a nil
// publisherPeer (writeUpstreamBatch returns early on a nil peer).
func testUpstreamTrack(publisherID, trackID string) *sfuPublishedTrack {
	return &sfuPublishedTrack{
		key:             sfuTrackKey{PublisherID: publisherID, TrackID: trackID},
		subscribers:     make(map[string]*sfuSubscription),
		firstFrameReady: make(chan struct{}),
		upstreamQueue:   make(chan upstreamFeedback, 1024),
		packetCache:     make(map[uint16]*rtp.Packet),
	}
}

// TestUpstreamQueueClosesOnUnregister (job 2.2): after 100
// register/unregister cycles the goroutine count returns to baseline
// (the per-track upstream writer exits when its queue closes).
func TestUpstreamQueueClosesOnUnregister(t *testing.T) {
	api := webrtc.NewAPI()
	router, err := newSFURouter(api)
	if err != nil {
		t.Fatalf("newSFURouter: %v", err)
	}
	defer router.close()
	sub := &sfuSubscription{viewerID: "viewer-1"}
	packet := &rtcp.PictureLossIndication{SenderSSRC: 1, MediaSSRC: 2}

	// Warm up: one cycle first, so lazy runtime goroutines are not
	// counted against the baseline.
	track := testUpstreamTrack("pub-warm", "track-warm")
	router.mu.Lock()
	router.tracks[track.key] = track
	router.mu.Unlock()
	router.enqueueUpstreamFeedback(track, packet, "pli", sub)
	if err := router.unregisterTrack(track.key); err != nil {
		t.Fatalf("unregister warm-up track: %v", err)
	}
	baseline := runtime.NumGoroutine()

	for i := 0; i < 100; i++ {
		key := sfuTrackKey{PublisherID: "pub-cycle", TrackID: "track-cycle"}
		track := testUpstreamTrack(key.PublisherID, key.TrackID)
		router.mu.Lock()
		router.tracks[key] = track
		router.mu.Unlock()
		router.enqueueUpstreamFeedback(track, packet, "pli", sub)
		if err := router.unregisterTrack(key); err != nil {
			t.Fatalf("unregister cycle %d: %v", i, err)
		}
	}
	goroutineCountSoon(t, baseline, 10*time.Second)
}

// TestUpstreamQueueSendAfterCloseDoesNotPanic (job 2.2): feedback
// enqueued after a track is unregistered must not panic on a closed
// channel (the Done line's no-panic condition).
func TestUpstreamQueueSendAfterCloseDoesNotPanic(t *testing.T) {
	api := webrtc.NewAPI()
	router, err := newSFURouter(api)
	if err != nil {
		t.Fatalf("newSFURouter: %v", err)
	}
	defer router.close()
	sub := &sfuSubscription{viewerID: "viewer-1"}
	packet := &rtcp.PictureLossIndication{SenderSSRC: 1, MediaSSRC: 2}
	track := testUpstreamTrack("pub-1", "track-1")
	router.mu.Lock()
	router.tracks[track.key] = track
	router.mu.Unlock()
	router.enqueueUpstreamFeedback(track, packet, "pli", sub)
	if err := router.unregisterTrack(track.key); err != nil {
		t.Fatalf("unregister track: %v", err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("enqueue after unregister panicked: %v", r)
		}
	}()
	for i := 0; i < 10; i++ {
		router.enqueueUpstreamFeedback(track, packet, "pli", sub)
	}
}

// --- 2.3 pcLifetimes ------------------------------------------------------

// pcLifetimeCountForID counts pcLifetimes entries for one id across
// all kinds (test helper; the map itself is package-private).
func pcLifetimeCountForID(id string) int {
	n := 0
	pcLifetimes.Range(func(key, _ any) bool {
		if k, ok := key.(string); ok && strings.HasSuffix(k, ":"+id) {
			n++
		}
		return true
	})
	return n
}

// TestPCLifetimesDeletedOnRouterClose (job 2.3): after a router
// teardown, no pcLifetimes entry remains for its publishers/viewers.
func TestPCLifetimesDeletedOnRouterClose(t *testing.T) {
	settings, _, err := loopbackSettings(false, 0)
	if err != nil {
		t.Fatalf("loopback settings: %v", err)
	}
	api := webrtc.NewAPI(webrtc.WithSettingEngine(settings))
	router, err := newSFURouter(api)
	if err != nil {
		t.Fatalf("newSFURouter: %v", err)
	}
	if _, err := router.registerPublisher(api, "pub-1"); err != nil {
		t.Fatalf("register publisher: %v", err)
	}
	viewerPeer, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("viewer peer: %v", err)
	}
	if err := router.registerViewer("viewer-1", viewerPeer); err != nil {
		t.Fatalf("register viewer: %v", err)
	}
	for _, kind := range []string{"publisher-client", "router-publisherPeer", "viewer-client", "router-serverPeer"} {
		pcLifetimeFor(kind, "pub-1")
		pcLifetimeFor(kind, "viewer-1")
	}
	if got := pcLifetimeCountForID("pub-1"); got == 0 {
		t.Fatal("no pcLifetimes entries were created for pub-1")
	}
	if err := router.close(); err != nil {
		t.Fatalf("router close: %v", err)
	}
	if got := pcLifetimeCountForID("pub-1"); got != 0 {
		t.Fatalf("pcLifetimes entries for pub-1 after close = %d, want 0", got)
	}
	if got := pcLifetimeCountForID("viewer-1"); got != 0 {
		t.Fatalf("pcLifetimes entries for viewer-1 after close = %d, want 0", got)
	}
}

// TestPCLifetimesBoundedThroughChurn (job 2.3, boundedness half): 2000
// create/delete cycles leave the map at its baseline size. The
// literal 30-minute churn run in the Done line is NOT run here; it is
// owed on the reference machine (see the cluster-A report).
func TestPCLifetimesBoundedThroughChurn(t *testing.T) {
	before := 0
	pcLifetimes.Range(func(_, _ any) bool { before++; return true })
	for i := 0; i < 2000; i++ {
		id := "churn-pc"
		pcLifetimeFor("viewer-client", id)
		pcLifetimeDelete("viewer-client", id)
	}
	after := 0
	pcLifetimes.Range(func(_, _ any) bool { after++; return true })
	if after != before {
		t.Fatalf("pcLifetimes size = %d after 2000 create/delete cycles, baseline %d", after, before)
	}
}

// --- 2.4 stored track keys ------------------------------------------------

// TestRouterTrackKeysForPublisher (job 2.4): the router returns the
// track keys it stored at registration, for any publisher name.
func TestRouterTrackKeysForPublisher(t *testing.T) {
	api := webrtc.NewAPI()
	router, err := newSFURouter(api)
	if err != nil {
		t.Fatalf("newSFURouter: %v", err)
	}
	defer router.close()
	want := sfuTrackKey{PublisherID: "alice", TrackID: "weird-track-name"}
	router.mu.Lock()
	router.tracks[want] = &sfuPublishedTrack{key: want}
	router.mu.Unlock()
	keys := router.trackKeysForPublisher("alice")
	if len(keys) != 1 || keys[0] != want {
		t.Fatalf("trackKeysForPublisher(alice) = %v, want [%v]", keys, want)
	}
	if got := router.trackKeysForPublisher("nobody"); len(got) != 0 {
		t.Fatalf("trackKeysForPublisher(nobody) = %v, want empty", got)
	}
}

// TestCleanupUsesStoredTrackKeysForNonPatternPublisher (job 2.4):
// /cleanup must remove the track of a publisher whose name does NOT
// match the old hami-m7h-publisher-%04d pattern (the old code rebuilt
// the track ID as stream-%04d from the name and missed it).
func TestCleanupUsesStoredTrackKeysForNonPatternPublisher(t *testing.T) {
	httpServer, server, router, _ := testClusterAServer(t, capacityLimits{})
	key := sfuTrackKey{PublisherID: "alice", TrackID: "weird-track-name"}
	track := testUpstreamTrack(key.PublisherID, key.TrackID)
	router.mu.Lock()
	router.tracks[key] = track
	router.mu.Unlock()
	server.mu.Lock()
	server.publishers["alice"] = m7hRemotePublisherState{shard: 0}
	server.mu.Unlock()

	status, body := testSFUPostJSON(t, httpServer.URL+"/cleanup", "test-bearer", nil)
	if status != http.StatusOK {
		t.Fatalf("cleanup status = %d, body = %s, want 200", status, body)
	}
	var response m7hRemoteCleanupResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode cleanup response: %v", err)
	}
	if !response.CleanupVerified || response.TracksRemaining != 0 {
		t.Fatalf("cleanup response = %+v, want verified with 0 tracks remaining", response)
	}
}

// --- 2.5 stale publisher entry --------------------------------------------

// TestFailedPublisherRegistrationRetrySucceeds (job 2.5): a failed
// registration must not leave a stale entry, so a retry with the same
// publisher ID does not return 409.
func TestFailedPublisherRegistrationRetrySucceeds(t *testing.T) {
	t.Run("closed router", func(t *testing.T) {
		httpServer, _, router, verifier := testClusterAServer(t, capacityLimits{})
		if err := router.close(); err != nil {
			t.Fatalf("close router: %v", err)
		}
		offer := testSFUClientOffer(t)
		status, _ := testClusterAPublish(t, httpServer, verifier, "pub-1", offer)
		if status == http.StatusConflict {
			t.Fatalf("first publish on closed router: status = 409, want a non-409 failure")
		}
		status, body := testClusterAPublish(t, httpServer, verifier, "pub-1", offer)
		if status == http.StatusConflict {
			t.Fatalf("retry after failed registration: status = 409, body = %s; stale entry was left behind", body)
		}
	})

	t.Run("bad offer then valid offer", func(t *testing.T) {
		httpServer, _, _, verifier := testClusterAServer(t, capacityLimits{})
		status, _ := testClusterAPublish(t, httpServer, verifier, "pub-1", "not-a-valid-sdp")
		if status != http.StatusBadRequest {
			t.Fatalf("publish with bad offer: status = %d, want 400", status)
		}
		status, body := testClusterAPublish(t, httpServer, verifier, "pub-1", "not-a-valid-sdp")
		if status == http.StatusConflict {
			t.Fatalf("retry after answer failure: status = 409, body = %s; stale entry was left behind", body)
		}
		// And a retry with a good offer must now succeed outright.
		offer := testSFUClientOffer(t)
		status, body = testClusterAPublish(t, httpServer, verifier, "pub-1", offer)
		if status != http.StatusOK {
			t.Fatalf("retry with valid offer: status = %d, body = %s, want 200", status, body)
		}
	})
}

// --- 2.6 key by publisher, track, layer -----------------------------------

// TestSimulcastTrackKeysDistinctByLayer (job 2.6): two layers of one
// publisher track (same publisher, same track ID, different layer)
// must be distinct router keys, so the second layer no longer
// overwrites — or is rejected as a duplicate of — the first.
func TestSimulcastTrackKeysDistinctByLayer(t *testing.T) {
	api := webrtc.NewAPI()
	router, err := newSFURouter(api)
	if err != nil {
		t.Fatalf("newSFURouter: %v", err)
	}
	defer router.close()
	low := sfuTrackKey{PublisherID: "pub-1", TrackID: "stream-1", Layer: "0"}
	high := sfuTrackKey{PublisherID: "pub-1", TrackID: "stream-1", Layer: "1"}
	if low == high {
		t.Fatal("track keys differing only by layer compare equal")
	}
	router.mu.Lock()
	router.tracks[low] = &sfuPublishedTrack{key: low}
	router.tracks[high] = &sfuPublishedTrack{key: high}
	router.mu.Unlock()
	if got := len(router.tracks); got != 2 {
		t.Fatalf("router holds %d tracks for a two-layer publisher, want 2", got)
	}
}

// TestResolveTrackKey (job 2.6): lookups by publisher+track resolve to
// the stored layer key when unambiguous, match exactly when a layer
// is given, and fail loudly when two layers make the lookup ambiguous.
func TestResolveTrackKey(t *testing.T) {
	api := webrtc.NewAPI()
	router, err := newSFURouter(api)
	if err != nil {
		t.Fatalf("newSFURouter: %v", err)
	}
	defer router.close()
	low := sfuTrackKey{PublisherID: "pub-1", TrackID: "stream-1", Layer: "0"}
	high := sfuTrackKey{PublisherID: "pub-1", TrackID: "stream-1", Layer: "1"}
	router.mu.Lock()
	router.tracks[low] = &sfuPublishedTrack{key: low}
	router.mu.Unlock()

	if got, err := router.resolveTrackKey(sfuTrackKey{PublisherID: "pub-1", TrackID: "stream-1"}); err != nil || got != low {
		t.Fatalf("resolveTrackKey without layer, one layer stored = %v, err = %v, want %v", got, err, low)
	}
	if got, err := router.resolveTrackKey(low); err != nil || got != low {
		t.Fatalf("resolveTrackKey exact layer = %v, err = %v, want %v", got, err, low)
	}
	if _, err := router.resolveTrackKey(sfuTrackKey{PublisherID: "pub-1", TrackID: "missing"}); err == nil {
		t.Fatal("resolveTrackKey for an unknown track succeeded, want an error")
	}
	router.mu.Lock()
	router.tracks[high] = &sfuPublishedTrack{key: high}
	router.mu.Unlock()
	if _, err := router.resolveTrackKey(sfuTrackKey{PublisherID: "pub-1", TrackID: "stream-1"}); err == nil {
		t.Fatal("resolveTrackKey without layer, two layers stored, succeeded; want an ambiguous-key error")
	}
	if got, err := router.resolveTrackKey(high); err != nil || got != high {
		t.Fatalf("resolveTrackKey exact high layer = %v, err = %v, want %v", got, err, high)
	}
}

// --- 2.7 hardcoded track IDs ----------------------------------------------

// TestNoHardcodedTrackIDInFeedbackDebug (job 2.7, debug-output half):
// the feedback-wait diagnostic must not gate its logging on one
// historical track ID (stream-0253 / stream-1885). The full
// repo-wide search output is saved as evidence; the remaining
// literals it finds are harness fixture inputs, listed there.
func TestNoHardcodedTrackIDInFeedbackDebug(t *testing.T) {
	content, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	for _, literal := range []string{`"stream-0253"`, `"stream-1885"`} {
		if strings.Contains(string(content), literal) {
			t.Fatalf("main.go still contains hardcoded track ID %s", literal)
		}
	}
}

// --- 2.8 platform guard for the swap check --------------------------------

// TestAudioSwapCheckRunsOnLinux (job 2.8, decision record): on Linux
// the audio swap check reads real swap from /proc/meminfo — it does
// not call a macOS-only sysctl, and it does not need skipping. This
// test pins that reader against /proc/meminfo itself. The Done line
// as written (skip on Linux) is NOT implemented: skipping would drop
// a working safety guard. See docs/archive/joblist/cluster-a/job-2.8-decision.md.
func TestAudioSwapCheckRunsOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux /proc/meminfo reader test runs on Linux only")
	}
	totalKiB, err := readMeminfoKiB("SwapTotal")
	if err != nil {
		t.Fatalf("read /proc/meminfo SwapTotal: %v", err)
	}
	freeKiB, err := readMeminfoKiB("SwapFree")
	if err != nil {
		t.Fatalf("read /proc/meminfo SwapFree: %v", err)
	}
	used, free, err := readAudioSwapMiB()
	if err != nil {
		t.Fatalf("readAudioSwapMiB: %v", err)
	}
	wantUsed := float64(totalKiB-freeKiB) / 1024
	wantFree := float64(freeKiB) / 1024
	if used != wantUsed || free != wantFree {
		t.Fatalf("readAudioSwapMiB = %.2f/%.2f MiB, /proc/meminfo says %.2f/%.2f MiB",
			used, free, wantUsed, wantFree)
	}
}

// --- 2.9 H.264 frames by picture ------------------------------------------

// annexBSplit splits Annex-B bytes into NAL payloads (without the
// start code). Test helper for building multi-slice fixtures.
func annexBSplit(t *testing.T, data []byte) [][]byte {
	t.Helper()
	var nals [][]byte
	start := -1
	for i := 0; i+4 <= len(data); i++ {
		if data[i] == 0 && data[i+1] == 0 && data[i+2] == 0 && data[i+3] == 1 {
			if start >= 0 {
				nals = append(nals, data[start:i])
			}
			start = i + 4
			i += 3
		}
	}
	if start >= 0 {
		nals = append(nals, data[start:])
	}
	if len(nals) == 0 {
		t.Fatal("fixture has no Annex-B NALs")
	}
	return nals
}

func annexBJoin(nals ...[]byte) []byte {
	var out []byte
	for _, nal := range nals {
		out = append(out, 0, 0, 0, 1)
		out = append(out, nal...)
	}
	return out
}

// TestReadH264FramesGroupsMultiSlicePictures (job 2.9): a stream with
// two pictures — an IDR picture in two slices and a P picture in two
// slices — must parse as two Samples, not four.
func TestReadH264FramesGroupsMultiSlicePictures(t *testing.T) {
	fixture, err := base64.StdEncoding.DecodeString(testH264AccessUnit)
	if err != nil {
		t.Fatal(err)
	}
	nals := annexBSplit(t, fixture)
	var sps, pps, idr []byte
	for _, nal := range nals {
		switch nal[0] & 0x1f {
		case 7:
			sps = nal
		case 8:
			pps = nal
		case 5:
			idr = nal
		}
	}
	if sps == nil || pps == nil || idr == nil {
		t.Fatalf("fixture NALs: sps=%v pps=%v idr=%v", sps != nil, pps != nil, idr != nil)
	}
	// Second IDR slice: first_mb_in_slice = 2 (0x40 -> ue(v) 2), so it
	// continues the IDR picture. Second P slice likewise continues
	// its picture; the first P slice has first_mb_in_slice = 0
	// (0x80 -> ue(v) 0), so it starts the second picture.
	idrSlice2 := []byte{0x65, 0x40, 0x11, 0x22, 0x33}
	pSlice1 := []byte{0x41, 0x80, 0x11, 0x22, 0x33}
	pSlice2 := []byte{0x41, 0x40, 0x44, 0x55, 0x66}
	stream := annexBJoin(sps, pps, idr, idrSlice2, pSlice1, pSlice2)
	frames, err := readH264Frames(bytes.NewReader(stream))
	if err != nil {
		t.Fatalf("read multi-slice stream: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("parsed %d frames from a 2-picture multi-slice stream, want 2", len(frames))
	}
	if !bytes.Contains(frames[0].Data, idrSlice2) {
		t.Fatal("first frame does not contain the second IDR slice")
	}
	if !bytes.Contains(frames[1].Data, pSlice1) || !bytes.Contains(frames[1].Data, pSlice2) {
		t.Fatal("second frame does not contain both P slices")
	}
	if bytes.Contains(frames[1].Data, sps) {
		t.Fatal("non-IDR frame must not carry SPS/PPS")
	}
}

// --- 2.10 egress-shard path ------------------------------------------------

// TestEgressShardStopsWhenStopped (job 2.10): a shard's run goroutine
// exits after stop() is called.
func TestEgressShardStopsWhenStopped(t *testing.T) {
	shard := &sfuEgressShard{
		subscriptions: make(map[string]*sfuSubscription),
		queue:         make(chan sfuEgressJob, 4),
		nackQueue:     make(chan sfuEgressNACKJob, 4),
		closed:        make(chan struct{}),
	}
	baseline := runtime.NumGoroutine()
	go shard.run()
	// Poll for the run goroutine to appear: a fixed sleep is not
	// enough under load (this test failed once in a full-suite run
	// on a loaded machine when 100 ms passed before the goroutine
	// was scheduled).
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := runtime.NumGoroutine(); got > baseline {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("shard run goroutine did not start (count %d, baseline %d)", got, baseline)
		}
		time.Sleep(10 * time.Millisecond)
	}
	shard.stop()
	goroutineCountSoon(t, baseline, 5*time.Second)
}

// TestRegisterTrackErrorStartsNoShardGoroutines (job 2.10): a failed
// track registration must not leave shard goroutines behind. The two
// error paths reachable without a live remote are exercised here
// (nil remote, empty track ID); the valid-remote paths (closed
// router, duplicate track, unregistered publisher) are covered by
// construction — shard creation sits after every validation return
// in registerTrack — and by inspection (see the 2.10 evidence file).
func TestRegisterTrackErrorStartsNoShardGoroutines(t *testing.T) {
	t.Setenv("HAMI_SFU_EGRESS_SHARDS", "2")
	api := webrtc.NewAPI()
	router, err := newSFURouter(api)
	if err != nil {
		t.Fatalf("newSFURouter: %v", err)
	}
	defer router.close()
	baseline := runtime.NumGoroutine()
	if err := router.registerTrack("pub-x", nil); err == nil {
		t.Fatal("registerTrack with nil remote succeeded, want an error")
	}
	if err := router.registerTrack("pub-x", &webrtc.TrackRemote{}); err == nil {
		t.Fatal("registerTrack with an empty-ID remote succeeded, want an error")
	}
	time.Sleep(200 * time.Millisecond)
	if got := runtime.NumGoroutine(); got > baseline+1 {
		t.Fatalf("goroutine count = %d after failed registrations, baseline %d (shard goroutines leaked)", got, baseline)
	}
}

// TestShardEnqueueFullQueueCountsDropAndRequestsKeyframe (job 2.10):
// a keyframe arriving at a full shard queue evicts the oldest delta,
// and that eviction is counted as a drop and requests a keyframe.
func TestShardEnqueueFullQueueCountsDropAndRequestsKeyframe(t *testing.T) {
	router := &sfuRouter{}
	subscription := &sfuSubscription{viewerID: "viewer-1", active: true, h264Video: true}
	shard := &sfuEgressShard{
		subscriptions: map[string]*sfuSubscription{subscription.viewerID: subscription},
		queue:         make(chan sfuEgressJob, 2),
		router:        router,
	}
	delta := []byte{0x41, 0x9a}
	key := []byte{0x65, 0x88}
	for _, sequence := range []uint16{1, 2} {
		shard.enqueue(layerTestPacket(sequence, delta), layerDelta, time.Now())
	}
	before := router.rtpQueueDropped.Load()
	shard.enqueue(layerTestPacket(3, key), layerKey, time.Now())
	if got := router.rtpQueueDropped.Load(); got != before+1 {
		t.Fatalf("router drop count = %d after keyframe eviction, want %d", got, before+1)
	}
	if got := subscription.rtpQueueDropped.Load(); got != 1 {
		t.Fatalf("subscription drop count = %d, want 1 (evicted delta counted)", got)
	}
	if subscription.keyframeNeededAt.Load() == 0 {
		t.Fatal("keyframe-eviction drop did not request keyframe recovery")
	}
}

// TestClusterAContext keeps the context import used by future tests in
// this file (httptest servers run with a background context).
var _ = context.Background
