package main

import (
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// TestRetransmitWindowDrivesCacheHorizon (4.3): the packet-cache
// horizon is the single retransmit window (time), not the packet
// count. The same age boundary holds at 6 packets/s and at 187
// packets/s — only the cached COUNT differs — and the boundary moves
// when the single value moves.
func TestRetransmitWindowDrivesCacheHorizon(t *testing.T) {
	origWindow := sfuRetransmitWindow
	defer func() { sfuRetransmitWindow = origWindow }()
	sfuRetransmitWindow = 500 * time.Millisecond

	// 6 packets/s: one packet every ~166 ms.
	now := time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	track := &sfuPublishedTrack{
		packetCache: make(map[uint16]*rtp.Packet),
		packetClock: func() time.Time { return now },
	}
	remember := func(sequence uint16) {
		track.rememberPacket(&rtp.Packet{
			Header:  rtp.Header{SequenceNumber: sequence},
			Payload: []byte{0x41, 0x9a},
		}, layerDelta)
	}
	remember(1) // T+0
	now = now.Add(499 * time.Millisecond)
	remember(2) // T+499ms: seq 1 is 499 ms old, inside the window.
	if _, missing := track.cachedPackets([]uint16{1}); len(missing) != 0 {
		t.Fatalf("6 pkt/s: sequence 1 evicted at age 499 ms, inside the 500 ms window")
	}
	now = now.Add(2 * time.Millisecond)
	remember(3) // T+501ms: seq 1 is 501 ms old, past the window.
	if _, missing := track.cachedPackets([]uint16{1}); len(missing) != 1 {
		t.Fatal("6 pkt/s: sequence 1 must be evicted at age 501 ms, past the 500 ms window")
	}
	if packets, missing := track.cachedPackets([]uint16{2, 3}); len(packets) != 2 || len(missing) != 0 {
		t.Fatalf("6 pkt/s: sequences 2,3 = %d packets, missing %v, want both cached", len(packets), missing)
	}

	// 187 packets/s: one packet every ~5.35 ms. 100 packets span
	// ~535 ms, so the first 7 (older than 500 ms at the end) are
	// evicted and the remaining 93 stay — same age boundary, very
	// different count.
	now = time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	fast := &sfuPublishedTrack{
		packetCache: make(map[uint16]*rtp.Packet),
		packetClock: func() time.Time { return now },
	}
	interval := 5350 * time.Microsecond // ~187 packets/s
	for sequence := uint16(1); sequence <= 100; sequence++ {
		fast.rememberPacket(&rtp.Packet{
			Header:  rtp.Header{SequenceNumber: sequence},
			Payload: []byte{0x41, 0x9a},
		}, layerDelta)
		now = now.Add(interval)
	}
	if _, missing := fast.cachedPackets([]uint16{7}); len(missing) != 1 {
		t.Fatal("187 pkt/s: sequence 7 (age ~503 ms) must be evicted past the window")
	}
	if packets, missing := fast.cachedPackets([]uint16{8}); len(packets) != 1 || len(missing) != 0 {
		t.Fatal("187 pkt/s: sequence 8 (age ~498 ms) must still be cached inside the window")
	}
	if got := fast.packetCachePackets.Load(); got != 93 {
		t.Fatalf("187 pkt/s: cached packets = %d, want 93 (100 remembered, 7 expired)", got)
	}

	// The boundary moves with the single value: at 200 ms, a packet
	// is kept at 199 ms and gone at 201 ms.
	sfuRetransmitWindow = 200 * time.Millisecond
	now = time.Date(2026, 11, 25, 0, 0, 0, 0, time.UTC)
	short := &sfuPublishedTrack{
		packetCache: make(map[uint16]*rtp.Packet),
		packetClock: func() time.Time { return now },
	}
	short.rememberPacket(&rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 1},
		Payload: []byte{0x41, 0x9a},
	}, layerDelta)
	now = now.Add(199 * time.Millisecond)
	short.rememberPacket(&rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 2},
		Payload: []byte{0x41, 0x9a},
	}, layerDelta)
	if _, missing := short.cachedPackets([]uint16{1}); len(missing) != 0 {
		t.Fatal("200 ms window: sequence 1 evicted at age 199 ms")
	}
	now = now.Add(2 * time.Millisecond)
	short.rememberPacket(&rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 3},
		Payload: []byte{0x41, 0x9a},
	}, layerDelta)
	if _, missing := short.cachedPackets([]uint16{1}); len(missing) != 1 {
		t.Fatal("200 ms window: sequence 1 must be evicted at age 201 ms")
	}
}

// TestRetransmitWindowDrivesNACKAggregation (4.3): the NACK dedup
// window is the same single value — a retransmit inside the window is
// skipped as a duplicate, past it the sequence is fresh again.
func TestRetransmitWindowDrivesNACKAggregation(t *testing.T) {
	origWindow := sfuRetransmitWindow
	defer func() { sfuRetransmitWindow = origWindow }()
	sfuRetransmitWindow = 200 * time.Millisecond
	now := time.Now()
	retransmittedAt := map[uint16]time.Time{100: now}
	pairs := rtcp.NackPairsFromSequenceNumbers([]uint16{100})
	fresh, duplicates, _ := selectNACKSequences(pairs, 200, retransmittedAt, now.Add(199*time.Millisecond))
	if len(fresh) != 0 || duplicates != 1 {
		t.Fatalf("inside window: fresh=%v duplicates=%d, want none and 1", fresh, duplicates)
	}
	fresh, _, _ = selectNACKSequences(pairs, 200, retransmittedAt, now.Add(201*time.Millisecond))
	if len(fresh) != 1 || fresh[0] != 100 {
		t.Fatalf("past window: fresh=%v, want [100]", fresh)
	}
}

// TestRetransmitWindowDrivesQueueAge (4.3): the queue-age drop is the
// same single value — a packet aged inside the window forwards, past
// it the packet is shed as stale and marks keyframe recovery.
func TestRetransmitWindowDrivesQueueAge(t *testing.T) {
	origWindow := sfuRetransmitWindow
	defer func() { sfuRetransmitWindow = origWindow }()
	sfuRetransmitWindow = 200 * time.Millisecond
	router := &sfuRouter{}
	codec := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}
	downTrack, err := webrtc.NewTrackLocalStaticRTP(codec, "video", "stream")
	if err != nil {
		t.Fatalf("NewTrackLocalStaticRTP: %v", err)
	}
	subscription := &sfuSubscription{active: true, h264Video: true, downTrack: downTrack}
	now := time.Now()
	router.forwardQueuedRTP(subscription, queuedRTP{
		packet:     &rtp.Packet{Payload: []byte{0x41, 0x80}},
		enqueuedAt: now.Add(-199 * time.Millisecond),
		layer:      layerDelta,
	})
	if got := subscription.rtpWriteSucceeded.Load(); got != 1 {
		t.Fatalf("inside window: writes = %d, want 1 (packet must forward)", got)
	}
	router.forwardQueuedRTP(subscription, queuedRTP{
		packet:     &rtp.Packet{Payload: []byte{0x41, 0x80}},
		enqueuedAt: now.Add(-201 * time.Millisecond),
		layer:      layerDelta,
	})
	if got := subscription.rtpWriteSucceeded.Load(); got != 1 {
		t.Fatalf("past window: writes = %d, want 1 (stale packet must not forward)", got)
	}
	if subscription.keyframeNeededAt.Load() == 0 {
		t.Fatal("past window: stale packet must mark keyframe recovery")
	}
	if got := router.rtpQueueDropped.Load(); got != 1 {
		t.Fatalf("past window: queue drops = %d, want 1", got)
	}
}
