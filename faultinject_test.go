package main

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

func faultTestPacket(sequence uint16, timestamp uint32) *rtp.Packet {
	return &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: sequence, Timestamp: timestamp},
		Payload: []byte{0x41, 0x9a},
	}
}

// TestFaultInjectorDropBySequenceOnce (4.6): a sequence rule drops the
// packet once; its retransmit (same sequence) passes.
func TestFaultInjectorDropBySequenceOnce(t *testing.T) {
	injector := newFaultInjector().DropSequence(3)
	packets := []*rtp.Packet{
		faultTestPacket(1, 100), faultTestPacket(2, 100), faultTestPacket(3, 100),
		faultTestPacket(4, 100), faultTestPacket(5, 100),
	}
	dropped := 0
	for _, packet := range packets {
		if injector.shouldDrop(packet) {
			dropped++
		}
	}
	if dropped != 1 {
		t.Fatalf("dropped = %d, want 1 (sequence 3 only)", dropped)
	}
	if injector.shouldDrop(faultTestPacket(3, 100)) {
		t.Fatal("retransmit of sequence 3 must pass under drop-once")
	}
	stats := injector.Stats()
	if stats.DroppedTotal != 1 || stats.DroppedBySeq != 1 || stats.Allowed != 5 {
		t.Fatalf("stats = %+v, want dropped 1 by sequence, allowed 5", stats)
	}
}

// TestFaultInjectorDropByOrdinal (4.6): the ordinal counts writeRTP
// calls (originals and retransmits alike); the dropped ordinal is
// dropped once.
func TestFaultInjectorDropByOrdinal(t *testing.T) {
	injector := newFaultInjector().DropOrdinal(2)
	for sequence := uint16(1); sequence <= 5; sequence++ {
		dropped := injector.shouldDrop(faultTestPacket(sequence, 100))
		if sequence == 3 && !dropped {
			t.Fatalf("ordinal 2 (sequence 3) must drop")
		}
		if sequence != 3 && dropped {
			t.Fatalf("sequence %d must pass", sequence)
		}
	}
	if injector.shouldDrop(faultTestPacket(3, 100)) {
		t.Fatal("retransmit of the dropped ordinal must pass under drop-once")
	}
	stats := injector.Stats()
	if stats.DroppedTotal != 1 || stats.DroppedByOrdinal != 1 || stats.SeenPackets != 6 {
		t.Fatalf("stats = %+v, want dropped 1 by ordinal, seen 6", stats)
	}
}

// TestFaultInjectorDropByFrame (4.6): a frame rule drops every packet
// of that frame (distinct RTP timestamps in arrival order).
func TestFaultInjectorDropByFrame(t *testing.T) {
	injector := newFaultInjector().DropFrame(1)
	timestamps := []uint32{100, 100, 200, 200, 300}
	dropped := 0
	for i, timestamp := range timestamps {
		if injector.shouldDrop(faultTestPacket(uint16(i+1), timestamp)) {
			dropped++
		}
	}
	if dropped != 2 {
		t.Fatalf("dropped = %d, want 2 (both packets of frame 1, ts 200)", dropped)
	}
	stats := injector.Stats()
	if stats.DroppedByFrame != 2 || stats.SeenFrames != 3 {
		t.Fatalf("stats = %+v, want dropped-by-frame 2, seen frames 3", stats)
	}
}

// TestFaultInjectorDropRetransmitsAlways (4.6): with DropRetransmits,
// every matching attempt drops — NACK cannot repair it.
func TestFaultInjectorDropRetransmitsAlways(t *testing.T) {
	injector := newFaultInjector().DropSequence(3).DropRetransmits()
	for attempt := 0; attempt < 3; attempt++ {
		if !injector.shouldDrop(faultTestPacket(3, 100)) {
			t.Fatalf("attempt %d of sequence 3 must drop under DropRetransmits", attempt)
		}
	}
	if stats := injector.Stats(); stats.DroppedTotal != 3 || stats.Allowed != 0 {
		t.Fatalf("stats = %+v, want dropped 3, allowed 0", stats)
	}
}

// TestFaultInjectorRulePrecedence (4.6): a packet matching several
// rules counts once, under the first rule in sequence > ordinal >
// frame order.
func TestFaultInjectorRulePrecedence(t *testing.T) {
	injector := newFaultInjector().DropSequence(5).DropOrdinal(2).DropFrame(0)
	// seq 5, ordinal 0, frame 0: matches sequence and frame.
	if !injector.shouldDrop(faultTestPacket(5, 100)) {
		t.Fatal("sequence 5 must drop")
	}
	// seq 6, ordinal 1, frame 0: matches frame only.
	if !injector.shouldDrop(faultTestPacket(6, 100)) {
		t.Fatal("frame 0 packet must drop")
	}
	// seq 7, ordinal 2, frame 1: matches ordinal only.
	if !injector.shouldDrop(faultTestPacket(7, 200)) {
		t.Fatal("ordinal 2 packet must drop")
	}
	// seq 8, ordinal 3, frame 1: matches nothing.
	if injector.shouldDrop(faultTestPacket(8, 200)) {
		t.Fatal("sequence 8 must pass")
	}
	stats := injector.Stats()
	if stats.DroppedBySeq != 1 || stats.DroppedByFrame != 1 || stats.DroppedByOrdinal != 1 {
		t.Fatalf("stats = %+v, want one drop per rule", stats)
	}
}

// TestFaultInjectorNilIsNoop (4.6): a nil injector never drops, and
// Stats on nil is the zero value.
func TestFaultInjectorNilIsNoop(t *testing.T) {
	var injector *faultInjector
	if injector.shouldDrop(faultTestPacket(1, 100)) {
		t.Fatal("nil injector must not drop")
	}
	if stats := injector.Stats(); stats != (faultInjectorStats{}) {
		t.Fatalf("nil stats = %+v, want zero", stats)
	}
}

// TestFaultInjectorConcurrentUse (4.6): the injector is safe for
// concurrent writeRTP calls (run with -race).
func TestFaultInjectorConcurrentUse(t *testing.T) {
	injector := newFaultInjector().DropSequence(42)
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				injector.shouldDrop(faultTestPacket(uint16(worker*1000+i), uint32(i)))
			}
		}(worker)
	}
	wg.Wait()
	if stats := injector.Stats(); stats.SeenPackets != 8000 {
		t.Fatalf("seen packets = %d, want 8000", stats.SeenPackets)
	}
}

// newFaultInjectionRouter builds a router-level harness (4.1): one
// published track with a real packet cache, one subscription with a
// real PeerConnection sender (SSRC bound at AddTrack, no ICE/SDP),
// and the nackWorker drain running. The injector sits on the
// subscription's writeRTP seam. No UDP is involved.
func newFaultInjectionRouter(t *testing.T, injector *faultInjector) (*sfuRouter, *sfuSubscription, uint32, func()) {
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
	params := sender.GetParameters()
	if len(params.Encodings) == 0 || params.Encodings[0].SSRC == 0 {
		t.Fatal("viewer downtrack SSRC is not bound after AddTrack")
	}
	viewerSSRC := uint32(params.Encodings[0].SSRC)
	source := &sfuPublishedTrack{packetCache: make(map[uint16]*rtp.Packet)}
	router := &sfuRouter{}
	subscription := &sfuSubscription{
		viewerID:        "hami-sfu-viewer-1",
		source:          source,
		router:          router,
		h264Video:       true,
		downTrack:       downTrack,
		sender:          sender,
		active:          true,
		retransmittedAt: make(map[uint16]time.Time),
		rtpQueue:        make(chan queuedRTP, 1024),
		nackQueue:       make(chan queuedNACK, nackQueueSize),
		faultInjector:   injector,
	}
	go router.nackWorker(subscription)
	cleanup := func() {
		close(subscription.rtpQueue)
		close(subscription.nackQueue)
		_ = peer.Close()
	}
	return router, subscription, viewerSSRC, cleanup
}

func waitFaultInjectionWrites(t *testing.T, subscription *sfuSubscription, want uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := subscription.rtpWriteSucceeded.Load(); got >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("writes = %d, want %d within 5s", subscription.rtpWriteSucceeded.Load(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

// faultInjectionStream builds the 4.1 stream: frame 0 (ts 1000000,
// seqs 1000-1004, keyframe) and frame 1 (ts 1018000, seqs 1005-1009,
// delta), all cached on the source as the router read them.
func faultInjectionStream(source *sfuPublishedTrack) []*rtp.Packet {
	keyPayload := append([]byte{0x65, 0x88}, make([]byte, 1198)...)
	deltaPayload := append([]byte{0x41, 0x9a}, make([]byte, 1198)...)
	packets := make([]*rtp.Packet, 0, 10)
	for sequence := uint16(1000); sequence <= 1009; sequence++ {
		packet := &rtp.Packet{Header: rtp.Header{SequenceNumber: sequence}}
		if sequence < 1005 {
			packet.Timestamp = 1000000
			packet.Payload = keyPayload
			if sequence == 1004 {
				packet.Marker = true
			}
		} else {
			packet.Timestamp = 1018000
			packet.Payload = deltaPayload
			if sequence == 1009 {
				packet.Marker = true
			}
		}
		source.rememberPacket(packet, classifyPacketLayer(webrtc.MimeTypeH264, packet))
		packets = append(packets, packet)
	}
	return packets
}

// TestFaultInjectionWholeFrameDropDoesNotBlockLaterFrames (4.2,
// router-level): dropping a whole frame (frame 1, all 5 packets) at
// writeRTP must not stall the later frames — frames 0 and 2 still
// land — and the dropped frame is recoverable by NACK from the cache
// (FIX 31: delta packets are cached, so the P-frame's retransmits
// exist). Assembler-level proof is
// TestVideoFrameAssemblerLateCompletionRecoversMissedFrames plus the
// 3.5 property tests. The Mac e2e leg (real UDP pairs) is OPEN.
func TestFaultInjectionWholeFrameDropDoesNotBlockLaterFrames(t *testing.T) {
	injector := newFaultInjector().DropFrame(1)
	router, subscription, viewerSSRC, cleanup := newFaultInjectionRouter(t, injector)
	defer cleanup()
	keyPayload := append([]byte{0x65, 0x88}, make([]byte, 1198)...)
	deltaPayload := append([]byte{0x41, 0x9a}, make([]byte, 1198)...)
	timestamps := []uint32{1000000, 1018000, 1036000}
	packets := make([]*rtp.Packet, 0, 15)
	for frame, timestamp := range timestamps {
		for i := 0; i < 5; i++ {
			sequence := uint16(1000 + frame*5 + i)
			packet := &rtp.Packet{
				Header:  rtp.Header{SequenceNumber: sequence, Timestamp: timestamp},
				Payload: deltaPayload,
			}
			if frame == 0 {
				packet.Payload = keyPayload
			}
			if i == 4 {
				packet.Marker = true
			}
			layer := layerDelta
			if frame == 0 {
				layer = layerKey
			}
			subscription.source.rememberPacket(packet, layer)
			packets = append(packets, packet)
		}
	}
	now := time.Now()
	for _, packet := range packets {
		layer := layerDelta
		if packet.SequenceNumber < 1005 {
			layer = layerKey
		}
		if _, err := subscription.enqueueRTP(packet, layer, now); err != nil {
			t.Fatalf("enqueueRTP seq %d: %v", packet.SequenceNumber, err)
		}
	}
	// Frames 0 and 2 land (10 writes); frame 1's 5 packets are dropped.
	waitFaultInjectionWrites(t, subscription, 10)
	stats := injector.Stats()
	if stats.DroppedTotal != 5 || stats.DroppedByFrame != 5 {
		t.Fatalf("injector stats = %+v, want dropped 5 by frame", stats)
	}
	if got := subscription.rtpWriteSucceeded.Load(); got != 10 {
		t.Fatalf("writes = %d, want 10 (frames 0 and 2 must not be blocked by the missing frame 1)", got)
	}
	// NACK frame 1's sequences: all 5 retransmit from the cache and
	// land. (The retransmits carry frame 1's timestamp, so they map
	// to the same frame ordinal; drop-once passes them in any case.)
	feedback := &rtcp.TransportLayerNack{
		MediaSSRC: viewerSSRC,
		Nacks:     rtcp.NackPairsFromSequenceNumbers([]uint16{1005, 1006, 1007, 1008, 1009}),
	}
	if err := router.handleNACK(subscription, feedback); err != nil {
		t.Fatalf("handleNACK: %v", err)
	}
	if got := subscription.rtpWriteSucceeded.Load(); got != 15 {
		t.Fatalf("writes = %d, want 15 (5 retransmits must land)", got)
	}
	subscription.statsMu.Lock()
	retransmitted := subscription.feedback.NACKRetransmitted
	subscription.statsMu.Unlock()
	if retransmitted != 5 {
		t.Fatalf("NACKRetransmitted = %d, want 5", retransmitted)
	}
}

// TestFaultInjectionForcedDropRun (4.4): a forced-drop run in two
// phases, every drop accounted for.
//
// Phase A (drop-once): 3 single packets dropped on egress, all
// retransmit-recovered: injected == retransmitted, keyframe-recovered
// == 0.
// Phase B (drop-always + queue-full): a whole frame dropped including
// its retransmits (unrecoverable by NACK: injected 10 events, 0
// retransmitted, 5 unique packets lost), and separately a queue-full
// burst whose sheds set markNeedsKeyframe and whose next keyframe
// completes the recovery (rtpRecoverySamples == 1). The PLI network
// send leg (requestRecoveryKeyframe needs source.remote /
// publisherPeer) is Mac-only; the router-side flag and completion are
// what this test proves.
func TestFaultInjectionForcedDropRun(t *testing.T) {
	router := &sfuRouter{}
	keyPayload := append([]byte{0x65, 0x88}, make([]byte, 1198)...)
	deltaPayload := append([]byte{0x41, 0x9a}, make([]byte, 1198)...)
	newSub := func(t *testing.T, viewerID string, injector *faultInjector, queueSize int, drain bool) (*sfuSubscription, uint32, func()) {
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
		viewerSSRC := uint32(sender.GetParameters().Encodings[0].SSRC)
		sub := &sfuSubscription{
			viewerID:        viewerID,
			source:          &sfuPublishedTrack{packetCache: make(map[uint16]*rtp.Packet)},
			router:          router,
			h264Video:       true,
			downTrack:       downTrack,
			sender:          sender,
			active:          true,
			retransmittedAt: make(map[uint16]time.Time),
			rtpQueue:        make(chan queuedRTP, queueSize),
			nackQueue:       make(chan queuedNACK, nackQueueSize),
			faultInjector:   injector,
		}
		if drain {
			go router.nackWorker(sub)
		}
		return sub, viewerSSRC, func() {
			close(sub.rtpQueue)
			close(sub.nackQueue)
			_ = peer.Close()
		}
	}

	// ---- Phase A: 3 single-packet drops, all retransmit-recovered.
	{
		injector := newFaultInjector().DropSequence(3002).DropSequence(3007).DropSequence(3012)
		sub, viewerSSRC, cleanup := newSub(t, "hami-sfu-viewer-1", injector, 1024, true)
		defer cleanup()
		packets := make([]*rtp.Packet, 0, 15)
		for frame := 0; frame < 3; frame++ {
			for i := 0; i < 5; i++ {
				sequence := uint16(3000 + frame*5 + i)
				packet := &rtp.Packet{
					Header:  rtp.Header{SequenceNumber: sequence, Timestamp: uint32(1000000 + frame*18000)},
					Payload: deltaPayload,
				}
				if frame == 0 {
					packet.Payload = keyPayload
				}
				if i == 4 {
					packet.Marker = true
				}
				layer := layerDelta
				if frame == 0 {
					layer = layerKey
				}
				sub.source.rememberPacket(packet, layer)
				packets = append(packets, packet)
			}
		}
		now := time.Now()
		for _, packet := range packets {
			layer := layerDelta
			if packet.SequenceNumber < 3005 {
				layer = layerKey
			}
			if _, err := sub.enqueueRTP(packet, layer, now); err != nil {
				t.Fatalf("phase A enqueueRTP seq %d: %v", packet.SequenceNumber, err)
			}
		}
		waitFaultInjectionWrites(t, sub, 12)
		feedback := &rtcp.TransportLayerNack{
			MediaSSRC: viewerSSRC,
			Nacks:     rtcp.NackPairsFromSequenceNumbers([]uint16{3002, 3007, 3012}),
		}
		if err := router.handleNACK(sub, feedback); err != nil {
			t.Fatalf("phase A handleNACK: %v", err)
		}
		if got := sub.rtpWriteSucceeded.Load(); got != 15 {
			t.Fatalf("phase A writes = %d, want 15", got)
		}
		stats := injector.Stats()
		sub.statsMu.Lock()
		retransmitted := sub.feedback.NACKRetransmitted
		sub.statsMu.Unlock()
		if stats.DroppedTotal != 3 || retransmitted != 3 {
			t.Fatalf("phase A: injected = %d, retransmitted = %d, want 3 and 3", stats.DroppedTotal, retransmitted)
		}
		if got := router.rtpRecoverySamples.Load(); got != 0 {
			t.Fatalf("phase A: keyframe recoveries = %d, want 0 (no shed, no recovery)", got)
		}
		t.Logf("phase A accounting: injected=%d retransmitted=%d keyframeRecoveries=%d",
			stats.DroppedTotal, retransmitted, router.rtpRecoverySamples.Load())
	}

	// ---- Phase B, part 1: whole frame dropped including retransmits.
	{
		injector := newFaultInjector().DropFrame(1).DropRetransmits()
		sub, viewerSSRC, cleanup := newSub(t, "hami-sfu-viewer-2", injector, 1024, true)
		defer cleanup()
		packets := make([]*rtp.Packet, 0, 15)
		for frame := 0; frame < 3; frame++ {
			for i := 0; i < 5; i++ {
				sequence := uint16(4000 + frame*5 + i)
				packet := &rtp.Packet{
					Header:  rtp.Header{SequenceNumber: sequence, Timestamp: uint32(1000000 + frame*18000)},
					Payload: deltaPayload,
				}
				if frame == 0 {
					packet.Payload = keyPayload
				}
				if i == 4 {
					packet.Marker = true
				}
				layer := layerDelta
				if frame == 0 {
					layer = layerKey
				}
				sub.source.rememberPacket(packet, layer)
				packets = append(packets, packet)
			}
		}
		now := time.Now()
		for _, packet := range packets {
			layer := layerDelta
			if packet.SequenceNumber < 4005 {
				layer = layerKey
			}
			if _, err := sub.enqueueRTP(packet, layer, now); err != nil {
				t.Fatalf("phase B enqueueRTP seq %d: %v", packet.SequenceNumber, err)
			}
		}
		waitFaultInjectionWrites(t, sub, 10)
		feedback := &rtcp.TransportLayerNack{
			MediaSSRC: viewerSSRC,
			Nacks:     rtcp.NackPairsFromSequenceNumbers([]uint16{4005, 4006, 4007, 4008, 4009}),
		}
		if err := router.handleNACK(sub, feedback); err != nil {
			t.Fatalf("phase B handleNACK: %v", err)
		}
		if got := sub.rtpWriteSucceeded.Load(); got != 10 {
			t.Fatalf("phase B writes = %d, want 10 (retransmits must NOT land)", got)
		}
		stats := injector.Stats()
		sub.statsMu.Lock()
		retransmitted := sub.feedback.NACKRetransmitted
		sub.statsMu.Unlock()
		if stats.DroppedTotal != 10 || retransmitted != 0 {
			t.Fatalf("phase B: injected = %d, retransmitted = %d, want 10 and 0", stats.DroppedTotal, retransmitted)
		}
		t.Logf("phase B accounting: injected=%d (5 originals + 5 retransmit attempts) retransmitted=%d uniquePacketsLost=5",
			stats.DroppedTotal, retransmitted)
	}

	// ---- Phase B, part 2: queue-full burst sets markNeedsKeyframe;
	// the next keyframe completes the recovery.
	{
		sub, _, cleanup := newSub(t, "hami-sfu-viewer-3", nil, sfuRTPQueueSize, false)
		defer cleanup()
		// 20 frames x 5 packets = 100 packets into a 64-deep queue
		// with no drain: packets 0-63 fill it; packets 64-99 shed
		// (deltas) or evict the oldest delta (keyframes). The shed
		// counts below are exact for this layout: 11 delta sheds +
		// 5 keyframe evictions + 20 delta sheds = 36.
		enqueueDropped := 0
		now := time.Now()
		for frame := 0; frame < 20; frame++ {
			for i := 0; i < 5; i++ {
				sequence := uint16(2000 + frame*5 + i)
				packet := &rtp.Packet{
					Header:  rtp.Header{SequenceNumber: sequence, Timestamp: uint32(1000000 + frame*18000)},
					Payload: deltaPayload,
				}
				layer := layerDelta
				if frame%5 == 0 {
					packet.Payload = keyPayload
					layer = layerKey
				}
				if i == 4 {
					packet.Marker = true
				}
				sub.source.rememberPacket(packet, layer)
				dropped, err := sub.enqueueRTP(packet, layer, now)
				if err != nil {
					t.Fatalf("phase B2 enqueueRTP seq %d: %v", sequence, err)
				}
				if dropped {
					enqueueDropped++
				}
			}
		}
		if enqueueDropped != 36 {
			t.Fatalf("phase B2 enqueue sheds = %d, want 36", enqueueDropped)
		}
		if got := sub.rtpQueueDropped.Load(); got != 36 {
			t.Fatalf("phase B2 rtpQueueDropped = %d, want 36", got)
		}
		if got := sub.keyframeNeededAt.Load(); got == 0 {
			t.Fatal("phase B2: markNeedsKeyframe must be set after queue-full sheds")
		}
		go router.nackWorker(sub)
		waitFaultInjectionWrites(t, sub, 64)
		if got := router.rtpRecoverySamples.Load(); got != 1 {
			t.Fatalf("phase B2: keyframe recoveries = %d, want 1", got)
		}
		t.Logf("phase B2 accounting: enqueueSheds=%d keyframeRecoveries=%d writes=%d",
			enqueueDropped, router.rtpRecoverySamples.Load(), sub.rtpWriteSucceeded.Load())
	}
}

// TestFaultInjectionLostPFramePacketRetransmitted (4.1, router-level):
// the injector drops one P-frame packet (seq 1007) on egress; the
// viewer's gap tracker NACKs it; handleNACK retransmits it from the
// router cache and the write lands. The full-network leg (real UDP
// pairs, runLocalSFUProof with fault injection) is a Mac run — see
// cluster-b/fault-injection-4-1-001.md.
func TestFaultInjectionLostPFramePacketRetransmitted(t *testing.T) {
	injector := newFaultInjector().DropSequence(1007)
	router, subscription, viewerSSRC, cleanup := newFaultInjectionRouter(t, injector)
	defer cleanup()
	packets := faultInjectionStream(subscription.source)
	now := time.Now()
	for _, packet := range packets {
		layer := layerDelta
		if packet.SequenceNumber < 1005 {
			layer = layerKey
		}
		if _, err := subscription.enqueueRTP(packet, layer, now); err != nil {
			t.Fatalf("enqueueRTP seq %d: %v", packet.SequenceNumber, err)
		}
	}
	// 9 of 10 originals land; seq 1007 is dropped at writeRTP.
	waitFaultInjectionWrites(t, subscription, 9)
	if got := injector.Stats().DroppedTotal; got != 1 {
		t.Fatalf("injector dropped = %d, want 1 (sequence 1007)", got)
	}
	// The viewer's gap tracker reports exactly the lost sequence.
	tracker := newNACKGapTracker()
	var nacked []uint16
	for _, packet := range packets {
		if packet.SequenceNumber == 1007 {
			continue
		}
		nacked = append(nacked, tracker.observe(packet.SequenceNumber, now)...)
	}
	if len(nacked) != 1 || nacked[0] != 1007 {
		t.Fatalf("gap tracker NACKs = %v, want [1007]", nacked)
	}
	// The router serves the NACK from its cache; the retransmit passes
	// the injector (drop-once) and lands.
	feedback := &rtcp.TransportLayerNack{
		MediaSSRC: viewerSSRC,
		Nacks:     rtcp.NackPairsFromSequenceNumbers([]uint16{1007}),
	}
	if err := router.handleNACK(subscription, feedback); err != nil {
		t.Fatalf("handleNACK: %v", err)
	}
	if got := subscription.rtpWriteSucceeded.Load(); got != 10 {
		t.Fatalf("writes = %d, want 10 (9 originals + 1 retransmit)", got)
	}
	stats := injector.Stats()
	if stats.DroppedTotal != 1 || stats.DroppedBySeq != 1 || stats.Allowed != 10 || stats.SeenPackets != 11 {
		t.Fatalf("injector stats = %+v, want dropped 1, allowed 10, seen 11", stats)
	}
	subscription.statsMu.Lock()
	retransmitted := subscription.feedback.NACKRetransmitted
	subscription.statsMu.Unlock()
	if retransmitted != 1 {
		t.Fatalf("NACKRetransmitted = %d, want 1", retransmitted)
	}
}

// TestFaultInjectionDropRetransmitsControl (4.1 fail-before control):
// the same run with DropRetransmits must NOT recover — the retransmit
// is dropped too, writes stay at 9, and NACKRetransmitted stays 0.
// This proves the test above would catch a broken retransmit path.
func TestFaultInjectionDropRetransmitsControl(t *testing.T) {
	injector := newFaultInjector().DropSequence(1007).DropRetransmits()
	router, subscription, viewerSSRC, cleanup := newFaultInjectionRouter(t, injector)
	defer cleanup()
	packets := faultInjectionStream(subscription.source)
	now := time.Now()
	for _, packet := range packets {
		layer := layerDelta
		if packet.SequenceNumber < 1005 {
			layer = layerKey
		}
		if _, err := subscription.enqueueRTP(packet, layer, now); err != nil {
			t.Fatalf("enqueueRTP seq %d: %v", packet.SequenceNumber, err)
		}
	}
	waitFaultInjectionWrites(t, subscription, 9)
	feedback := &rtcp.TransportLayerNack{
		MediaSSRC: viewerSSRC,
		Nacks:     rtcp.NackPairsFromSequenceNumbers([]uint16{1007}),
	}
	if err := router.handleNACK(subscription, feedback); err != nil {
		t.Fatalf("handleNACK: %v", err)
	}
	if got := subscription.rtpWriteSucceeded.Load(); got != 9 {
		t.Fatalf("writes = %d, want 9 (retransmit must NOT land under DropRetransmits)", got)
	}
	stats := injector.Stats()
	if stats.DroppedTotal != 2 || stats.DroppedBySeq != 2 {
		t.Fatalf("injector stats = %+v, want dropped 2 (original + retransmit)", stats)
	}
	subscription.statsMu.Lock()
	retransmitted := subscription.feedback.NACKRetransmitted
	subscription.statsMu.Unlock()
	if retransmitted != 0 {
		t.Fatalf("NACKRetransmitted = %d, want 0", retransmitted)
	}
}
