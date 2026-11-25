package main

import (
	"runtime"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// newMemoryBudgetViewer builds one idle viewer the way the router
// does: a real Pion PeerConnection with a downtrack, a
// sfuSubscription with production queue sizes, and a nackWorker.
// No ICE/SDP/UDP is involved, so this runs in the VM.
func newMemoryBudgetViewer(t *testing.T, viewerID string) (*sfuSubscription, func()) {
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
	source := &sfuPublishedTrack{packetCache: make(map[uint16]*rtp.Packet)}
	router := &sfuRouter{}
	subscription := &sfuSubscription{
		viewerID:        viewerID,
		source:          source,
		router:          router,
		h264Video:       true,
		downTrack:       downTrack,
		sender:          sender,
		active:          true,
		retransmittedAt: make(map[uint16]time.Time),
		rtpQueue:        make(chan queuedRTP, sfuRTPQueueSize),
		nackQueue:       make(chan queuedNACK, nackQueueSize),
	}
	go router.nackWorker(subscription)
	cleanup := func() {
		close(subscription.rtpQueue)
		close(subscription.nackQueue)
		_ = peer.Close()
	}
	return subscription, cleanup
}

// heapAllocAfterGC returns HeapAlloc after a full GC, so the reading
// is the live heap, not garbage awaiting collection.
func heapAllocAfterGC() uint64 {
	runtime.GC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	return memory.HeapAlloc
}

func medianUint64(values []uint64) uint64 {
	sorted := append([]uint64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	return sorted[len(sorted)/2]
}

// leastSquaresSlope returns the slope of y against x (bytes per
// viewer when x is a viewer count).
func leastSquaresSlope(xs, ys []float64) float64 {
	var meanX, meanY float64
	for i := range xs {
		meanX += xs[i]
		meanY += ys[i]
	}
	meanX /= float64(len(xs))
	meanY /= float64(len(ys))
	var numerator, denominator float64
	for i := range xs {
		numerator += (xs[i] - meanX) * (ys[i] - meanY)
		denominator += (xs[i] - meanX) * (xs[i] - meanX)
	}
	return numerator / denominator
}

// TestPerViewerMemoryBudgetMeasured (5.10): measures HeapAlloc per
// viewer at object level — one idle viewer is a Pion PeerConnection
// plus a sfuSubscription with production queues — and enforces that
// the measured cost fits inside the budget formula's per-viewer
// estimate (500,000 B), so the formula's x2 multiplier is genuine
// headroom, not cover for an underestimate. The in-flight variant
// measures the same viewers with full 64-packet queues.
func TestPerViewerMemoryBudgetMeasured(t *testing.T) {
	steps := []int{0, 10, 20, 40}
	heaps := make([]uint64, len(steps))
	var subscriptions []*sfuSubscription
	var cleanups []func()
	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()
	for i, want := range steps {
		for len(subscriptions) < want {
			id := len(subscriptions)
			subscription, cleanup := newMemoryBudgetViewer(t, "hami-sfu-viewer-mem-"+strconv.Itoa(id))
			subscriptions = append(subscriptions, subscription)
			cleanups = append(cleanups, cleanup)
		}
		samples := make([]uint64, 0, 3)
		for s := 0; s < 3; s++ {
			samples = append(samples, heapAllocAfterGC())
		}
		heaps[i] = medianUint64(samples)
		t.Logf("viewers=%d heapAlloc=%d bytes (median of 3 GC readings)", want, heaps[i])
	}
	xs := make([]float64, len(steps))
	ys := make([]float64, len(steps))
	for i := range steps {
		xs[i] = float64(steps[i])
		ys[i] = float64(heaps[i])
	}
	perViewer := leastSquaresSlope(xs, ys)
	t.Logf("measured per-viewer heap: %.0f bytes/viewer (least-squares slope over %v)", perViewer, steps)
	if perViewer > 500_000 {
		t.Fatalf("measured per-viewer heap %.0f bytes exceeds the 500,000 B estimate in defaultSFUHeapBudgetForViewers", perViewer)
	}
	measuredTotal := heaps[len(heaps)-1] - heaps[0]
	budget := defaultSFUHeapBudgetForViewers(steps[len(steps)-1])
	t.Logf("measured total for %d viewers: %d bytes; budget formula: %d bytes", steps[len(steps)-1], measuredTotal, budget)
	if uint64(measuredTotal) > budget {
		t.Fatalf("measured total heap %d bytes for %d viewers exceeds the budget formula %d bytes", measuredTotal, steps[len(steps)-1], budget)
	}

	// In-flight variant: fill every viewer's 64-packet queue with
	// 1200-byte video packets (shared payload, one copy per slot)
	// and measure the extra heap per viewer.
	payload := append([]byte{0x41, 0x9a}, make([]byte, 1198)...)
	now := time.Now()
	for _, subscription := range subscriptions {
		for i := 0; i < sfuRTPQueueSize; i++ {
			packet := &rtp.Packet{
				Header:  rtp.Header{SequenceNumber: uint16(2000 + i), Timestamp: 1000000},
				Payload: payload,
			}
			if _, err := subscription.enqueueRTP(packet, layerDelta, now); err != nil {
				t.Fatalf("enqueueRTP: %v", err)
			}
		}
	}
	samples := make([]uint64, 0, 3)
	for s := 0; s < 3; s++ {
		samples = append(samples, heapAllocAfterGC())
	}
	fullHeap := medianUint64(samples)
	inFlightPerViewer := float64(fullHeap-heaps[len(heaps)-1]) / float64(len(subscriptions))
	t.Logf("in-flight: full queues add %.0f bytes/viewer (heap %d -> %d)", inFlightPerViewer, heaps[len(heaps)-1], fullHeap)
	if inFlightPerViewer > 500_000 {
		t.Fatalf("in-flight queue cost %.0f bytes/viewer exceeds the 500,000 B estimate", inFlightPerViewer)
	}
}
