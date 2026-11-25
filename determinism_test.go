package main

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/pion/rtp"
)

// Job 15.2 — Deterministic seeds.
//
// The tree has no seeded randomness in production code (grep for
// math/rand: no hits outside tests), so determinism here is a property
// of the test inputs, not of a seed flag in the binary. These tests pin
// the property down: the same seed must produce the same packet order,
// and the same packet order fed to the frame assembler must produce the
// same completed-frame record — twice, in the same process and across
// separate `go test -count=2` invocations (see
// docs/cluster-e-evidence-15.2.md for the run log).
//
// If a seeded path is ever added to production code (fault injection,
// jitter, loss), it must take its seed from an explicit parameter and
// gain a test of this shape: same seed in, byte-identical result out.

const determinismSeed = 20261125

// seededPacketOrder returns a fixed permutation of the packets of three
// frames (sequences 100-108, timestamps 10/20/30, marker on the last
// packet of each frame), shuffled with the given seed.
func seededPacketOrder(seed int64) []*rtp.Packet {
	packets := []*rtp.Packet{}
	for frame, ts := range []uint32{10, 20, 30} {
		for i := 0; i < 3; i++ {
			packets = append(packets, &rtp.Packet{Header: rtp.Header{
				SequenceNumber: uint16(100 + frame*3 + i),
				Timestamp:      ts,
				Marker:         i == 2,
			}})
		}
	}
	rng := rand.New(rand.NewSource(seed))
	rng.Shuffle(len(packets), func(i, j int) {
		packets[i], packets[j] = packets[j], packets[i]
	})
	return packets
}

func runSeededAssembler(seed int64) []uint64 {
	// Primed with the known first sequence (100), so every frame has
	// a known start boundary: without priming, the assembler
	// deliberately refuses to record the first frame it sees (no
	// known boundary), and a shuffled arrival order would lose
	// frames for that reason, not for nondeterminism.
	assembler := &videoFrameAssembler{
		frameBoundaryKnown:     true,
		nextFrameFirstSequence: 100,
	}
	completed := []uint64{}
	for _, packet := range seededPacketOrder(seed) {
		completed = append(completed, assembler.push(packet)...)
	}
	return completed
}

func TestSeededPacketOrderIsIdenticalAcrossRuns(t *testing.T) {
	first := seededPacketOrder(determinismSeed)
	second := seededPacketOrder(determinismSeed)
	if len(first) != len(second) {
		t.Fatalf("seeded order lengths differ: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Header.SequenceNumber != second[i].Header.SequenceNumber ||
			first[i].Header.Timestamp != second[i].Header.Timestamp ||
			first[i].Header.Marker != second[i].Header.Marker {
			t.Fatalf("seeded order differs at index %d: %+v vs %+v",
				i, first[i].Header, second[i].Header)
		}
	}
	// A different seed must give a different order, or the shuffle is a no-op.
	other := seededPacketOrder(determinismSeed + 1)
	same := true
	for i := range first {
		if first[i].Header.SequenceNumber != other[i].Header.SequenceNumber {
			same = false
			break
		}
	}
	if same {
		t.Fatal("different seeds gave the same packet order; the shuffle is not seed-dependent")
	}
}

func TestSeededAssemblerGivesIdenticalResults(t *testing.T) {
	first := runSeededAssembler(determinismSeed)
	second := runSeededAssembler(determinismSeed)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("seeded assembler results differ: %v vs %v", first, second)
	}
	// All three frames complete: the assembler must see every frame
	// exactly once regardless of arrival order.
	if !reflect.DeepEqual(first, []uint64{10, 20, 30}) {
		t.Fatalf("seeded assembler completed frames = %v, want [10 20 30]", first)
	}
}

// TestSeededLossPatternIsIdenticalAcrossRuns pins the shape a future
// deterministic fault-injection suite (4.6) must have: a loss set
// derived only from (seed, sequence range) is identical on every run.
func TestSeededLossPatternIsIdenticalAcrossRuns(t *testing.T) {
	lossSet := func(seed int64) map[uint16]bool {
		rng := rand.New(rand.NewSource(seed))
		lost := map[uint16]bool{}
		for seq := uint16(1000); seq < 1100; seq++ {
			if rng.Intn(100) < 5 { // 5% deterministic loss
				lost[seq] = true
			}
		}
		return lost
	}
	if !reflect.DeepEqual(lossSet(determinismSeed), lossSet(determinismSeed)) {
		t.Fatal("seeded loss pattern differs between identical runs")
	}
	if reflect.DeepEqual(lossSet(determinismSeed), lossSet(determinismSeed+1)) {
		t.Fatal("different seeds gave the same loss pattern")
	}
}
