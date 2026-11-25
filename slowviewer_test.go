package main

import (
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// TestSlowViewerMaxLayerDefault (4.7): the zero value allows all
// layers, setMaxLayer caps the filter, and filterLayer counts only
// packets above the cap.
func TestSlowViewerMaxLayerDefault(t *testing.T) {
	subscription := &sfuSubscription{}
	if got := subscription.maxLayerValue(); got != layerDelta {
		t.Fatalf("zero-value maxLayer = %v, want layerDelta", got)
	}
	subscription.setMaxLayer(layerKey)
	if got := subscription.maxLayerValue(); got != layerKey {
		t.Fatalf("maxLayer after setMaxLayer(layerKey) = %v", got)
	}
	if !subscription.filterLayer(layerDelta) {
		t.Fatal("delta must be filtered at maxLayer layerKey")
	}
	if subscription.filterLayer(layerKey) {
		t.Fatal("keyframe must not be filtered at maxLayer layerKey")
	}
	if subscription.filterLayer(layerBase) {
		t.Fatal("base must not be filtered at maxLayer layerKey")
	}
	if got := subscription.layerFiltered.Load(); got != 1 {
		t.Fatalf("layerFiltered = %d, want 1", got)
	}
}

// TestSlowViewerPolicyStepsDownThenDrops (4.7): a viewer that never
// drains its queue steps down a layer, then another, then is
// dropped — each step counted. The driver loop is the forwardTrack
// dropped branch in miniature: filter, enqueue, strike on drop,
// reset on success. No drain goroutine runs, so the 8-packet queue
// fills and stays full.
func TestSlowViewerPolicyStepsDownThenDrops(t *testing.T) {
	subscription := &sfuSubscription{
		active:    true,
		h264Video: true,
		rtpQueue:  make(chan queuedRTP, 8),
		nackQueue: make(chan queuedNACK, nackQueueSize),
	}
	subscription.setMaxLayer(layerDelta)
	// One base (SPS/PPS), one keyframe, two deltas per cycle: every
	// layer keeps arriving, so each rung of the ladder is exercised.
	layers := []packetLayer{layerBase, layerKey, layerDelta, layerDelta}
	var steppedToKey, steppedToBase, droppedAt int
	for i := 0; i < 200 && subscription.isActive(); i++ {
		layer := layers[i%len(layers)]
		if subscription.filterLayer(layer) {
			continue
		}
		dropped, err := subscription.enqueueRTP(&rtp.Packet{
			Header:  rtp.Header{SequenceNumber: uint16(i)},
			Payload: []byte{0x41, 0x9a},
		}, layer, time.Now())
		if err != nil {
			t.Fatalf("enqueueRTP: %v", err)
		}
		if dropped {
			subscription.recordQueueFullDrop()
		} else {
			subscription.recordSuccessfulEnqueue()
		}
		switch subscription.maxLayerValue() {
		case layerKey:
			if steppedToKey == 0 {
				steppedToKey = i + 1
			}
		case layerBase:
			if steppedToBase == 0 {
				steppedToBase = i + 1
			}
		}
	}
	if subscription.isActive() {
		t.Fatal("viewer was never dropped after 200 packets against a full queue")
	}
	droppedAt = 200
	if steppedToKey == 0 || steppedToBase == 0 {
		t.Fatalf("ladder not traversed: steppedToKey=%d steppedToBase=%d (want both > 0)",
			steppedToKey, steppedToBase)
	}
	if !(steppedToKey < steppedToBase && steppedToBase < droppedAt) {
		t.Fatalf("ladder out of order: key=%d base=%d drop<=%d", steppedToKey, steppedToBase, droppedAt)
	}
	if got := subscription.layerStepDowns.Load(); got != 2 {
		t.Fatalf("layerStepDowns = %d, want 2", got)
	}
	if got := subscription.slowViewerDrops.Load(); got != 1 {
		t.Fatalf("slowViewerDrops = %d, want 1", got)
	}
	if got := subscription.layerFiltered.Load(); got == 0 {
		t.Fatal("layerFiltered = 0, want > 0 (deltas filtered after the first step-down)")
	}
	if got := subscription.rtpQueueDropped.Load(); got == 0 {
		t.Fatal("rtpQueueDropped = 0, want > 0 (the queue filled and shed)")
	}
}

// TestSlowViewerShardLayerFilter (4.7): on the shard path the layer
// filter runs in run()'s fan-out loop — a stepped-down viewer (max
// layer keyframe) does not receive a delta job but does receive a
// keyframe job. No UDP; forwardQueuedRTP writes to an unbound
// downtrack.
func TestSlowViewerShardLayerFilter(t *testing.T) {
	router := &sfuRouter{}
	codec := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}
	downTrack, err := webrtc.NewTrackLocalStaticRTP(codec, "video", "stream")
	if err != nil {
		t.Fatalf("NewTrackLocalStaticRTP: %v", err)
	}
	subscription := &sfuSubscription{
		viewerID:  "hami-sfu-viewer-1",
		active:    true,
		h264Video: true,
		downTrack: downTrack,
	}
	subscription.setMaxLayer(layerKey)
	shard := &sfuEgressShard{
		subscriptions: map[string]*sfuSubscription{subscription.viewerID: subscription},
		queue:         make(chan sfuEgressJob, 16),
		nackQueue:     make(chan sfuEgressNACKJob, 16),
		router:        router,
		closed:        make(chan struct{}),
	}
	go shard.run()
	defer close(shard.closed)
	now := time.Now()
	shard.queue <- sfuEgressJob{
		packet:     &rtp.Packet{Header: rtp.Header{SequenceNumber: 100}, Payload: []byte{0x41, 0x9a}},
		enqueuedAt: now,
		layer:      layerDelta,
	}
	shard.queue <- sfuEgressJob{
		packet: &rtp.Packet{
			Header:  rtp.Header{SequenceNumber: 101, Marker: true},
			Payload: []byte{0x65, 0x88},
		},
		enqueuedAt: now,
		layer:      layerKey,
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if subscription.layerFiltered.Load() == 1 && subscription.rtpWriteSucceeded.Load() == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("shard filter: layerFiltered=%d writes=%d, want 1 and 1 within 5s",
				subscription.layerFiltered.Load(), subscription.rtpWriteSucceeded.Load())
		}
		time.Sleep(time.Millisecond)
	}
}
