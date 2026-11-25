package main

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// isolationRecorder records enqueue->write latencies for one viewer.
type isolationRecorder struct {
	mu        sync.Mutex
	latencies []time.Duration
}

func (r *isolationRecorder) record(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latencies = append(r.latencies, d)
}

func (r *isolationRecorder) p99() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.latencies) == 0 {
		return 0
	}
	return Percentile(append([]time.Duration(nil), r.latencies...), 99)
}

func (r *isolationRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.latencies)
}

// isolationViewer is one viewer with a recording drain goroutine
// that does exactly what nackWorker does for the RTP queue
// (forwardQueuedRTP + release the copy), plus an optional per-packet
// drain delay that models a slow network. drainDelay > 0 stalls the
// viewer: its queue fills and the forward path sheds for it alone.
type isolationViewer struct {
	subscription *sfuSubscription
	recorder     *isolationRecorder
	done         chan struct{}
	stopping     atomic.Bool
	cleanup      func()
}

func newIsolationViewer(t *testing.T, router *sfuRouter, source *sfuPublishedTrack, viewerID string, drainDelay time.Duration) *isolationViewer {
	t.Helper()
	api := webrtc.NewAPI()
	peer, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("NewPeerConnection: %v", err)
	}
	codec := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}
	downTrack, err := webrtc.NewTrackLocalStaticRTP(codec, "video", "stream")
	if err != nil {
		t.Fatalf("NewTrackLocalStaticRTP: %v", err)
	}
	sender, err := peer.AddTrack(downTrack)
	if err != nil {
		t.Fatalf("AddTrack: %v", err)
	}
	subscription := &sfuSubscription{
		viewerID:        viewerID,
		source:          source,
		router:          router,
		key:             sfuTrackKey{PublisherID: source.key.PublisherID, TrackID: "video"},
		h264Video:       true,
		downTrack:       downTrack,
		sender:          sender,
		active:          true,
		retransmittedAt: make(map[uint16]time.Time),
		rtpQueue:        make(chan queuedRTP, sfuRTPQueueSize),
		nackQueue:       make(chan queuedNACK, nackQueueSize),
	}
	viewer := &isolationViewer{
		subscription: subscription,
		recorder:     &isolationRecorder{},
		done:         make(chan struct{}),
	}
	go func() {
		defer close(viewer.done)
		for queued := range subscription.rtpQueue {
			// Cleanup sets stopping so the drain exits promptly
			// instead of chewing through a full stalled queue at
			// 200 ms per packet (~13 s for 64 packets).
			if viewer.stopping.Load() {
				return
			}
			if !subscription.isActive() {
				return
			}
			if drainDelay > 0 {
				time.Sleep(drainDelay)
			}
			router.forwardQueuedRTP(subscription, queued)
			releaseRTPPacketCopy(queued.packet)
			viewer.recorder.record(time.Since(queued.enqueuedAt))
		}
	}()
	viewer.cleanup = func() {
		viewer.stopping.Store(true)
		close(subscription.rtpQueue)
		close(subscription.nackQueue)
		<-viewer.done
		_ = peer.Close()
	}
	return viewer
}

// isolationDrive is the test's forwardTrack: for one packet, remember
// it on the source and fan it out to every subscription with the
// exact forwardTrack direct-branch sequence (layer filter,
// enqueueRTP, drop bookkeeping, success reset).
func isolationDrive(router *sfuRouter, source *sfuPublishedTrack, viewers []*isolationViewer, packet *rtp.Packet, layer packetLayer, now time.Time) {
	source.rememberPacket(packet, layer)
	for _, viewer := range viewers {
		subscription := viewer.subscription
		if subscription.filterLayer(layer) {
			continue
		}
		dropped, err := subscription.enqueueRTP(packet, layer, now)
		if dropped {
			router.rtpQueueDropped.Add(1)
			router.rtpQueueDropShed.Add(1)
			router.recordRTPIntentionalShed(subscription)
			router.requestRecoveryKeyframe(subscription)
			subscription.recordQueueFullDrop()
		} else {
			subscription.recordSuccessfulEnqueue()
		}
		if err != nil && subscription.isActive() {
			router.rtpDropped.Add(1)
			router.recordError(err)
		}
	}
}

// isolationStream builds a 60-frame stream: frame 0 keyframe (5
// packets), frames 1-59 delta (5 packets each), 300 packets total,
// 1200-byte payloads, one timestamp per frame (+18000 ticks).
func isolationStream() ([]*rtp.Packet, []packetLayer) {
	keyPayload := append([]byte{0x65, 0x88}, make([]byte, 1198)...)
	deltaPayload := append([]byte{0x41, 0x9a}, make([]byte, 1198)...)
	packets := make([]*rtp.Packet, 0, 300)
	layers := make([]packetLayer, 0, 300)
	for frame := 0; frame < 60; frame++ {
		for i := 0; i < 5; i++ {
			sequence := uint16(1000 + frame*5 + i)
			packet := &rtp.Packet{
				Header:  rtp.Header{SequenceNumber: sequence, Timestamp: 1000000 + uint32(frame)*18000},
				Payload: deltaPayload,
			}
			layer := layerDelta
			if frame == 0 {
				packet.Payload = keyPayload
				layer = layerKey
			}
			if i == 4 {
				packet.Marker = true
			}
			packets = append(packets, packet)
			layers = append(layers, layer)
		}
	}
	return packets, layers
}

func waitIsolationWrites(t *testing.T, viewer *isolationViewer, want uint64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if viewer.subscription.rtpWriteSucceeded.Load() >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("viewer %s writes = %d, want %d within 10s", viewer.subscription.viewerID, viewer.subscription.rtpWriteSucceeded.Load(), want)
}

// isolationTolerance is the stated tolerance for 5.11: every other
// participant's forward p99 must stay within 2x the clean-run p99
// (with a 1 ms floor for scheduler noise) and within the 50 ms
// stall-gate bound in absolute terms.
func isolationTolerance(cleanP99 time.Duration) time.Duration {
	tolerance := 2 * cleanP99
	if tolerance < time.Millisecond {
		tolerance = time.Millisecond
	}
	return tolerance
}

func assertIsolated(t *testing.T, name string, cleanP99, stalledP99 time.Duration) {
	t.Helper()
	tolerance := isolationTolerance(cleanP99)
	t.Logf("%s: clean p99=%v stalled-run p99=%v tolerance=%v", name, cleanP99, stalledP99, tolerance)
	if stalledP99 > tolerance {
		t.Fatalf("%s: p99 %v exceeds tolerance %v (clean p99 %v)", name, stalledP99, tolerance, cleanP99)
	}
	if stalledP99 > 50*time.Millisecond {
		t.Fatalf("%s: p99 %v exceeds the 50 ms stall-gate bound", name, stalledP99)
	}
}

// TestIsolationStalledViewer (5.11): one viewer whose drain sleeps
// 200 ms per packet (a stalled network) must not slow the other
// viewers. The stalled viewer's queue fills, the 4.7 policy steps it
// down, and it receives a fraction of the stream — while every other
// viewer's forward p99 stays within tolerance of the clean run.
func TestIsolationStalledViewer(t *testing.T) {
	packets, layers := isolationStream()

	// Phase 1 (clean): 5 healthy viewers, 300 packets at 1 ms pacing.
	router := &sfuRouter{}
	source := &sfuPublishedTrack{packetCache: make(map[uint16]*rtp.Packet), key: sfuTrackKey{PublisherID: "pub-1", TrackID: "video"}}
	viewers := make([]*isolationViewer, 0, 5)
	for i := 0; i < 5; i++ {
		viewers = append(viewers, newIsolationViewer(t, router, source, "viewer-clean-"+strconv.Itoa(i), 0))
	}
	for i, packet := range packets {
		isolationDrive(router, source, viewers, packet, layers[i], time.Now())
		time.Sleep(time.Millisecond)
	}
	for _, viewer := range viewers {
		waitIsolationWrites(t, viewer, 300)
	}
	var cleanP99 time.Duration
	for _, viewer := range viewers {
		p99 := viewer.recorder.p99()
		t.Logf("clean viewer %s: p99=%v writes=%d", viewer.subscription.viewerID, p99, viewer.subscription.rtpWriteSucceeded.Load())
		if p99 > cleanP99 {
			cleanP99 = p99
		}
	}
	for _, viewer := range viewers {
		viewer.cleanup()
	}

	// Phase 2 (stall): viewer 0 drains at 200 ms/packet; the rest
	// are healthy. Same router shape, fresh viewers.
	router = &sfuRouter{}
	source = &sfuPublishedTrack{packetCache: make(map[uint16]*rtp.Packet), key: sfuTrackKey{PublisherID: "pub-1", TrackID: "video"}}
	viewers = make([]*isolationViewer, 0, 5)
	viewers = append(viewers, newIsolationViewer(t, router, source, "viewer-stalled", 200*time.Millisecond))
	for i := 1; i < 5; i++ {
		viewers = append(viewers, newIsolationViewer(t, router, source, "viewer-healthy-"+strconv.Itoa(i), 0))
	}
	for i, packet := range packets {
		isolationDrive(router, source, viewers, packet, layers[i], time.Now())
		time.Sleep(time.Millisecond)
	}
	for _, viewer := range viewers[1:] {
		waitIsolationWrites(t, viewer, 300)
	}
	// The stall must be real: the stalled viewer steps down a layer
	// (4.7 policy) and receives a fraction of the stream.
	stalled := viewers[0]
	if stalled.subscription.layerStepDowns.Load() < 1 {
		t.Fatalf("stalled viewer layerStepDowns = %d, want >= 1 (the stall did not bite)", stalled.subscription.layerStepDowns.Load())
	}
	stalledWrites := stalled.subscription.rtpWriteSucceeded.Load()
	t.Logf("stalled viewer: layerStepDowns=%d writes=%d queueDropped=%d", stalled.subscription.layerStepDowns.Load(), stalledWrites, stalled.subscription.rtpQueueDropped.Load())
	if stalledWrites >= 300 {
		t.Fatalf("stalled viewer writes = %d, want < 300 (it received the whole stream, so it was not stalled)", stalledWrites)
	}
	for _, viewer := range viewers[1:] {
		assertIsolated(t, "healthy viewer "+viewer.subscription.viewerID, cleanP99, viewer.recorder.p99())
	}
	for _, viewer := range viewers {
		viewer.cleanup()
	}
}

// TestIsolationStalledPublisher (5.11): one publisher that stops
// sending (its driver blocks forever after 10 packets, the
// forwardTrack-blocked-in-ReadRTP shape) must not slow the other
// publisher's viewers.
func TestIsolationStalledPublisher(t *testing.T) {
	packets, layers := isolationStream()

	build := func() (*sfuRouter, *sfuPublishedTrack, []*isolationViewer, *sfuPublishedTrack, []*isolationViewer) {
		router := &sfuRouter{}
		sourceA := &sfuPublishedTrack{packetCache: make(map[uint16]*rtp.Packet), key: sfuTrackKey{PublisherID: "pub-A", TrackID: "video"}}
		sourceB := &sfuPublishedTrack{packetCache: make(map[uint16]*rtp.Packet), key: sfuTrackKey{PublisherID: "pub-B", TrackID: "video"}}
		viewersA := make([]*isolationViewer, 0, 3)
		viewersB := make([]*isolationViewer, 0, 3)
		for i := 0; i < 3; i++ {
			viewersA = append(viewersA, newIsolationViewer(t, router, sourceA, "viewer-A-"+strconv.Itoa(i), 0))
			viewersB = append(viewersB, newIsolationViewer(t, router, sourceB, "viewer-B-"+strconv.Itoa(i), 0))
		}
		return router, sourceA, viewersA, sourceB, viewersB
	}
	// Phase 1 (clean): both publishers send all 300 packets.
	router, sourceA, viewersA, sourceB, viewersB := build()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i, packet := range packets {
			isolationDrive(router, sourceA, viewersA, packet, layers[i], time.Now())
			time.Sleep(time.Millisecond)
		}
	}()
	for i, packet := range packets {
		isolationDrive(router, sourceB, viewersB, packet, layers[i], time.Now())
		time.Sleep(time.Millisecond)
	}
	<-done
	for _, viewer := range append(append([]*isolationViewer{}, viewersA...), viewersB...) {
		waitIsolationWrites(t, viewer, 300)
	}
	var cleanP99 time.Duration
	for _, viewer := range viewersB {
		p99 := viewer.recorder.p99()
		t.Logf("clean publisher B viewer %s: p99=%v", viewer.subscription.viewerID, p99)
		if p99 > cleanP99 {
			cleanP99 = p99
		}
	}
	for _, viewer := range append(append([]*isolationViewer{}, viewersA...), viewersB...) {
		viewer.cleanup()
	}

	// Phase 2 (stall): publisher A sends 10 packets, then its
	// driver blocks forever; publisher B sends all 300.
	router, sourceA, viewersA, sourceB, viewersB = build()
	stall := make(chan struct{})
	driverA := make(chan struct{})
	go func() {
		defer close(driverA)
		for i := 0; i < 10; i++ {
			isolationDrive(router, sourceA, viewersA, packets[i], layers[i], time.Now())
			time.Sleep(time.Millisecond)
		}
		<-stall // parked: the forwardTrack-blocked-in-ReadRTP shape
	}()
	for i, packet := range packets {
		isolationDrive(router, sourceB, viewersB, packet, layers[i], time.Now())
		time.Sleep(time.Millisecond)
	}
	for _, viewer := range viewersB {
		waitIsolationWrites(t, viewer, 300)
	}
	// The stall must be real: A's viewers got ~10 packets, not 300.
	for _, viewer := range viewersA {
		writes := viewer.subscription.rtpWriteSucceeded.Load()
		t.Logf("stalled publisher A viewer %s: writes=%d", viewer.subscription.viewerID, writes)
		if writes > 12 {
			t.Fatalf("publisher A viewer writes = %d, want <= 12 (publisher A did not stall)", writes)
		}
	}
	for _, viewer := range viewersB {
		assertIsolated(t, "publisher B viewer "+viewer.subscription.viewerID, cleanP99, viewer.recorder.p99())
	}
	close(stall)
	<-driverA
	for _, viewer := range append(append([]*isolationViewer{}, viewersA...), viewersB...) {
		viewer.cleanup()
	}
}
