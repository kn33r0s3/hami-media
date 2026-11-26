package main

import (
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// BenchmarkCopyRTPPacketWithSharedPayload measures the per-copy cost
// with the sync.Pool. Before the pool, each copy was a fresh heap alloc
// (&packetCopy); after, the struct is recycled.
func BenchmarkCopyRTPPacketWithSharedPayload(b *testing.B) {
	packet := &rtp.Packet{
		Header:  rtp.Header{SSRC: 11, SequenceNumber: 7, CSRC: []uint32{22}},
		Payload: []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copied := copyRTPPacketWithSharedPayload(packet)
		releaseRTPPacketCopy(copied)
	}
}

// BenchmarkCopyRTPPacketUnpooled (Job 5.4) is the before half of the
// pool comparison: the same packet as the pooled benchmark, with the
// pool bypassed, so allocs/op shows what the pool removes.
func BenchmarkCopyRTPPacketUnpooled(b *testing.B) {
	packet := &rtp.Packet{
		Header:  rtp.Header{SSRC: 11, SequenceNumber: 7, CSRC: []uint32{22}},
		Payload: []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
	disableRTPPacketCopyPool = true
	defer func() { disableRTPPacketCopyPool = false }()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copied := copyRTPPacketWithSharedPayload(packet)
		releaseRTPPacketCopy(copied)
	}
}

// videoShapePacket is the realistic forwarded packet (Job 5.3): a
// 1200-byte video payload with no CSRC and no header extensions —
// the shape the scale harness actually forwards.
func videoShapePacket() *rtp.Packet {
	return &rtp.Packet{
		Header:  rtp.Header{SSRC: 11, SequenceNumber: 7},
		Payload: make([]byte, 1200),
	}
}

// BenchmarkCopyRTPPacketVideoShape (Job 5.3) measures the copy cost
// for the realistic video packet shape, pooled.
func BenchmarkCopyRTPPacketVideoShape(b *testing.B) {
	packet := videoShapePacket()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copied := copyRTPPacketWithSharedPayload(packet)
		releaseRTPPacketCopy(copied)
	}
}

// BenchmarkCopyRTPPacketVideoShapeUnpooled (Job 5.4) is the before
// half for the realistic video shape: same packet, pool bypassed.
func BenchmarkCopyRTPPacketVideoShapeUnpooled(b *testing.B) {
	packet := videoShapePacket()
	disableRTPPacketCopyPool = true
	defer func() { disableRTPPacketCopyPool = false }()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copied := copyRTPPacketWithSharedPayload(packet)
		releaseRTPPacketCopy(copied)
	}
}

// BenchmarkEgressShardVsDirect measures the matched fan-out cost of
// the direct path against the egress-shard path (Job 5.9): same packet
// (1200-byte video payload, the dominant case), same viewer count,
// same drain semantics. Each iteration fans one packet out to every
// viewer and waits until every write lands, so ns/op is the true
// end-to-end fan-out cost, not enqueue-only. Direct: one
// copyRTPPacketWithSharedPayload per viewer (enqueueRTP) plus one
// nackWorker goroutine per subscription draining its own queue.
// Shard: one copy per shard (shard.enqueue) plus one run() goroutine
// per shard doing the fan-out serially. The downtrack is unbound, so
// WriteRTP is a no-op return — the benchmark measures the fan-out
// machinery (copies, channel hops, forwardQueuedRTP), not network
// writes. Run with -benchmem; the decision (keep or delete the shard
// path) is recorded in cluster-b/egress-shard-5-9-001.md.
func BenchmarkEgressShardVsDirect(b *testing.B) {
	newFanOut := func(b *testing.B, viewers, shards int) (subs []*sfuSubscription, send func(seq uint16), cleanup func()) {
		b.Helper()
		router := &sfuRouter{}
		codec := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}
		payload := make([]byte, 1200)
		for i := range payload {
			payload[i] = byte(i)
		}
		packet := &rtp.Packet{
			Header:  rtp.Header{SSRC: 11, SequenceNumber: 7},
			Payload: payload,
		}
		subs = make([]*sfuSubscription, 0, viewers)
		for i := 0; i < viewers; i++ {
			downTrack, err := webrtc.NewTrackLocalStaticRTP(codec, "video", "stream")
			if err != nil {
				b.Fatalf("NewTrackLocalStaticRTP: %v", err)
			}
			subs = append(subs, &sfuSubscription{
				viewerID:  "hami-sfu-viewer-" + strconv.Itoa(i),
				router:    router,
				h264Video: true,
				downTrack: downTrack,
				active:    true,
				rtpQueue:  make(chan queuedRTP, 1024),
				nackQueue: make(chan queuedNACK, nackQueueSize),
			})
		}
		if shards == 0 {
			for _, sub := range subs {
				go router.nackWorker(sub)
			}
			send = func(seq uint16) {
				packet.SequenceNumber = seq
				now := time.Now()
				for _, sub := range subs {
					if _, err := sub.enqueueRTP(packet, layerDelta, now); err != nil {
						b.Fatalf("enqueueRTP: %v", err)
					}
				}
			}
			cleanup = func() {
				for _, sub := range subs {
					close(sub.rtpQueue)
					close(sub.nackQueue)
				}
			}
			return subs, send, cleanup
		}
		shardList := make([]*sfuEgressShard, 0, shards)
		for i := 0; i < shards; i++ {
			shardList = append(shardList, &sfuEgressShard{
				subscriptions: make(map[string]*sfuSubscription),
				queue:         make(chan sfuEgressJob, 1024),
				nackQueue:     make(chan sfuEgressNACKJob, nackQueueSize),
				router:        router,
				closed:        make(chan struct{}),
			})
		}
		for _, sub := range subs {
			shardList[egressShardIndex(sub.viewerID, shards)].add(sub)
		}
		for _, shard := range shardList {
			go shard.run()
		}
		send = func(seq uint16) {
			packet.SequenceNumber = seq
			now := time.Now()
			for _, shard := range shardList {
				shard.enqueue(packet, layerDelta, now)
			}
		}
		cleanup = func() {
			for _, shard := range shardList {
				shard.stop()
			}
		}
		return subs, send, cleanup
	}
	waitForWrites := func(b *testing.B, subs []*sfuSubscription, target uint64) {
		b.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			var total uint64
			for _, sub := range subs {
				total += sub.rtpWriteSucceeded.Load()
			}
			if total >= target {
				return
			}
			if time.Now().After(deadline) {
				b.Fatalf("fan-out stalled: %d/%d writes landed", total, target)
			}
			runtime.Gosched()
		}
	}
	for _, tc := range []struct {
		name    string
		viewers int
		shards  int
	}{
		{"direct/100viewers", 100, 0},
		{"shard4/100viewers", 100, 4},
		{"direct/1000viewers", 1000, 0},
		{"shard4/1000viewers", 1000, 4},
		{"shard8/1000viewers", 1000, 8},
		{"shard16/1000viewers", 1000, 16},
		{"shard32/1000viewers", 1000, 32},
	} {
		b.Run(tc.name, func(b *testing.B) {
			subs, send, cleanup := newFanOut(b, tc.viewers, tc.shards)
			b.Cleanup(cleanup)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				send(uint16(1000 + i))
				waitForWrites(b, subs, uint64(i+1)*uint64(tc.viewers))
			}
			b.StopTimer()
			var dropped uint64
			for _, sub := range subs {
				dropped += sub.rtpQueueDropped.Load()
			}
			if dropped != 0 {
				b.Fatalf("fan-out dropped %d packets; queues must not fill in this benchmark", dropped)
			}
		})
	}
}
func BenchmarkEnqueueRTP(b *testing.B) {
	subscription := &sfuSubscription{active: true, rtpQueue: make(chan queuedRTP, 1024)}
	packet := &rtp.Packet{
		Header:  rtp.Header{SSRC: 11, SequenceNumber: 7},
		Payload: []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
	now := time.Now()
	// Drain the queue in the background so enqueue never blocks, and
	// release each copy so the pool is exercised end to end.
	done := make(chan struct{})
	go func() {
		for entry := range subscription.rtpQueue {
			releaseRTPPacketCopy(entry.packet)
		}
		close(done)
	}()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := subscription.enqueueRTP(packet, layerBase, now); err != nil {
			b.Fatalf("enqueueRTP: %v", err)
		}
	}
	b.StopTimer()
	close(subscription.rtpQueue)
	<-done
}

// BenchmarkEnqueueRTPUnpooled (Job 5.4) is the before half for the
// forward enqueue path: the same enqueue loop with the packet-copy
// pool bypassed, so allocs/op and ns/op show what the pool removes
// from the forward path.
func BenchmarkEnqueueRTPUnpooled(b *testing.B) {
	subscription := &sfuSubscription{active: true, rtpQueue: make(chan queuedRTP, 1024)}
	packet := &rtp.Packet{
		Header:  rtp.Header{SSRC: 11, SequenceNumber: 7},
		Payload: []byte{1, 2, 3, 4, 5, 6, 7, 8},
	}
	now := time.Now()
	// Drain the queue in the background so enqueue never blocks, and
	// release each copy so the pool is exercised end to end.
	done := make(chan struct{})
	go func() {
		for entry := range subscription.rtpQueue {
			releaseRTPPacketCopy(entry.packet)
		}
		close(done)
	}()
	disableRTPPacketCopyPool = true
	defer func() { disableRTPPacketCopyPool = false }()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := subscription.enqueueRTP(packet, layerBase, now); err != nil {
			b.Fatalf("enqueueRTP: %v", err)
		}
	}
	b.StopTimer()
	close(subscription.rtpQueue)
	<-done
}
