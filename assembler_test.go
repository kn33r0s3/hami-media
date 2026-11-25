package main

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/pion/rtp"
)

// --- Job 3.4: sequence wraparound ---------------------------------------

// The 16-bit sequence number wraps mid-stream: frames span 65534,
// 65535, 0, 1, 2, ... and every frame still completes exactly once,
// in timestamp order.
func TestVideoFrameAssemblerSequenceWraparound(t *testing.T) {
	push := func(a *videoFrameAssembler, sequence uint16, timestamp uint32, marker bool) []uint64 {
		t.Helper()
		return a.push(&rtp.Packet{Header: rtp.Header{
			SequenceNumber: sequence,
			Timestamp:      timestamp,
			Marker:         marker,
		}})
	}
	assembler := &videoFrameAssembler{
		frameBoundaryKnown:     true,
		nextFrameFirstSequence: 65534,
	}
	// Frame ts=100: seqs 65534, 65535(marker). Frame ts=110: seqs
	// 0, 1(marker). Frame ts=120: seqs 2, 3, 4(marker).
	if ids := push(assembler, 65534, 100, false); len(ids) != 0 {
		t.Fatalf("frame 100 early completion: %v", ids)
	}
	if ids := push(assembler, 65535, 100, true); len(ids) != 1 || ids[0] != 100 {
		t.Fatalf("frame 100 = %v, want [100]", ids)
	}
	if ids := push(assembler, 0, 110, false); len(ids) != 0 {
		t.Fatalf("frame 110 early completion: %v", ids)
	}
	if ids := push(assembler, 1, 110, true); len(ids) != 1 || ids[0] != 110 {
		t.Fatalf("frame 110 = %v, want [110]", ids)
	}
	if ids := push(assembler, 2, 120, false); len(ids) != 0 {
		t.Fatalf("frame 120 early completion: %v", ids)
	}
	if ids := push(assembler, 3, 120, false); len(ids) != 0 {
		t.Fatalf("frame 120 early completion: %v", ids)
	}
	if ids := push(assembler, 4, 120, true); len(ids) != 1 || ids[0] != 120 {
		t.Fatalf("frame 120 = %v, want [120]", ids)
	}
	// A duplicate of the wrapped marker (65535) after the wrap must
	// not complete anything again.
	if ids := push(assembler, 65535, 100, true); len(ids) != 0 {
		t.Fatalf("duplicate wrapped marker completed: %v", ids)
	}
}

// The same wrap, but the marker of the post-wrap frame arrives
// BEFORE its first fragment: the frame must wait, then complete.
func TestVideoFrameAssemblerSequenceWraparoundReordered(t *testing.T) {
	push := func(a *videoFrameAssembler, sequence uint16, timestamp uint32, marker bool) []uint64 {
		t.Helper()
		return a.push(&rtp.Packet{Header: rtp.Header{
			SequenceNumber: sequence,
			Timestamp:      timestamp,
			Marker:         marker,
		}})
	}
	assembler := &videoFrameAssembler{
		frameBoundaryKnown:     true,
		nextFrameFirstSequence: 65534,
	}
	if ids := push(assembler, 65534, 100, false); len(ids) != 0 {
		t.Fatalf("frame 100 early completion: %v", ids)
	}
	if ids := push(assembler, 65535, 100, true); len(ids) != 1 || ids[0] != 100 {
		t.Fatalf("frame 100 = %v, want [100]", ids)
	}
	// Marker of frame 110 (seq 1) arrives before its fragment (seq 0).
	if ids := push(assembler, 1, 110, true); len(ids) != 0 {
		t.Fatalf("post-wrap marker completed early: %v", ids)
	}
	if ids := push(assembler, 0, 110, false); len(ids) != 1 || ids[0] != 110 {
		t.Fatalf("post-wrap reordered frame = %v, want [110]", ids)
	}
}

// --- Job 3.5: randomized reorder / duplicate property tests ------------

type assemblerPacket struct {
	sequence  uint16
	timestamp uint32
	marker    bool
}

// buildAssemblerStream builds framesCount frames of 1..4 packets each,
// sequential sequence numbers from firstSeq, timestamps advancing
// 18,000 ticks (5 fps) per frame from a nonzero base: a frame stamped
// 0 could never enter the late span (its low bound is exclusive and
// zero-valued), and real streams start at a random nonzero RTP
// timestamp, so 0 is not a realistic first timestamp.
func buildAssemblerStream(rng *rand.Rand, framesCount int, firstSeq uint16) []assemblerPacket {
	var stream []assemblerPacket
	seq := firstSeq
	for frame := 0; frame < framesCount; frame++ {
		packets := 1 + rng.Intn(4)
		ts := uint32(1000000 + frame*18000)
		for p := 0; p < packets; p++ {
			stream = append(stream, assemblerPacket{
				sequence:  seq,
				timestamp: ts,
				marker:    p == packets-1,
			})
			seq++
		}
	}
	return stream
}

// feedAssemblerStream pushes packets in order and returns the
// completed frame timestamps in completion order. firstSeq is the
// stream's true first sequence (the priming point): it must come
// from the unshuffled stream, not from the first packet to arrive.
func feedAssemblerStream(t *testing.T, stream []assemblerPacket, firstSeq uint16) []uint64 {
	t.Helper()
	assembler := &videoFrameAssembler{
		frameBoundaryKnown:     true,
		nextFrameFirstSequence: firstSeq,
	}
	var completed []uint64
	for _, p := range stream {
		ids := assembler.push(&rtp.Packet{Header: rtp.Header{
			SequenceNumber: p.sequence,
			Timestamp:      p.timestamp,
			Marker:         p.marker,
		}})
		completed = append(completed, ids...)
	}
	return completed
}

// Reordering in real networks is bounded: packets arrive out of order
// within a jitter window, not arbitrarily late. The property test
// shuffles within a sliding window of 8 consecutive packets and
// duplicates ~20% of packets at random positions, over 25 fixed
// seeds (deterministic: same seeds, same streams, every run).
// Every frame must complete exactly once. Completion ORDER is not
// asserted: a later frame may complete before an earlier one whose
// packets are still in flight (late completion is by design — see
// the late-retransmit tests), and downstream matching is by RTP
// timestamp, not by arrival order.
func TestVideoFrameAssemblerPropertyReorderAndDuplicate(t *testing.T) {
	const seeds = 25
	for seed := int64(1); seed <= seeds; seed++ {
		rng := rand.New(rand.NewSource(seed))
		framesCount := 8 + rng.Intn(13) // 8..20 frames
		// Half the seeds start the stream near the sequence wrap.
		firstSeq := uint16(rng.Intn(60000))
		if seed%2 == 0 {
			firstSeq = 65530 - uint16(rng.Intn(10))
		}
		stream := buildAssemblerStream(rng, framesCount, firstSeq)

		// Shuffle within windows of 8 consecutive packets.
		shuffled := make([]assemblerPacket, len(stream))
		copy(shuffled, stream)
		for start := 0; start < len(shuffled); start += 8 {
			end := start + 8
			if end > len(shuffled) {
				end = len(shuffled)
			}
			rng.Shuffle(end-start, func(i, j int) {
				shuffled[start+i], shuffled[start+j] = shuffled[start+j], shuffled[start+i]
			})
		}
		// Duplicate ~20% of packets at random positions.
		duplicates := len(shuffled) / 5
		for i := 0; i < duplicates; i++ {
			at := rng.Intn(len(shuffled) + 1)
			pick := shuffled[rng.Intn(len(shuffled))]
			shuffled = append(shuffled, assemblerPacket{})
			copy(shuffled[at+1:], shuffled[at:])
			shuffled[at] = pick
		}

		completed := feedAssemblerStream(t, shuffled, firstSeq)
		want := make([]uint64, framesCount)
		for frame := 0; frame < framesCount; frame++ {
			want[frame] = uint64(1000000 + frame*18000)
		}
		// Set equality: every frame exactly once, in any order.
		sorted := append([]uint64(nil), completed...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		if len(sorted) != len(want) {
			t.Fatalf("seed %d: completed %d frames %v, want %d frames %v (stream %d packets, %d dups)",
				seed, len(sorted), sorted, len(want), want, len(stream), duplicates)
		}
		for i := range want {
			if sorted[i] != want[i] {
				t.Fatalf("seed %d: completed set %v, want %v",
					seed, sorted, want)
			}
		}
	}
}

// The same property with duplicates only (no reordering): every frame
// completes exactly once.
func TestVideoFrameAssemblerPropertyDuplicatesOnly(t *testing.T) {
	for seed := int64(101); seed <= 110; seed++ {
		rng := rand.New(rand.NewSource(seed))
		stream := buildAssemblerStream(rng, 12, uint16(rng.Intn(60000)))
		doubled := make([]assemblerPacket, 0, 2*len(stream))
		for _, p := range stream {
			doubled = append(doubled, p)
			if rng.Intn(2) == 0 {
				doubled = append(doubled, p)
			}
		}
		completed := feedAssemblerStream(t, doubled, stream[0].sequence)
		want := make([]uint64, 12)
		for frame := range want {
			want[frame] = uint64(1000000 + frame*18000)
		}
		if len(completed) != len(want) {
			t.Fatalf("seed %d: completed %v, want %v", seed, completed, want)
		}
		for i := range want {
			if completed[i] != want[i] {
				t.Fatalf("seed %d: completed[%d] = %d, want %d", seed, i, completed[i], want[i])
			}
		}
	}
}

// A full shuffle (no window bound) still completes every frame
// exactly once — the assembler holds no ordering guarantee, so the
// assertion is set equality, not completion order.
func TestVideoFrameAssemblerPropertyFullShuffleCompletesEveryFrame(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	stream := buildAssemblerStream(rng, 10, 1000)
	firstSeq := stream[0].sequence
	rng.Shuffle(len(stream), func(i, j int) { stream[i], stream[j] = stream[j], stream[i] })
	completed := feedAssemblerStream(t, stream, firstSeq)
	want := make([]uint64, 10)
	for frame := range want {
		want[frame] = uint64(1000000 + frame*18000)
	}
	sorted := append([]uint64(nil), completed...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	if len(sorted) != len(want) {
		t.Fatalf("full shuffle: completed %d frames %v, want %d %v", len(sorted), sorted, len(want), want)
	}
	for i := range want {
		if sorted[i] != want[i] {
			t.Fatalf("full shuffle: completed set %v, want %v", sorted, want)
		}
	}
}
