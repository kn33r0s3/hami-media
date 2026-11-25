package main

import (
	"sync"
	"sync/atomic"

	"github.com/pion/rtp"
)

// faultInjector is the deterministic fault-injection seam for the SFU
// forward path (Job 4.6). It sits in sfuSubscription.writeRTP — the
// single seam through which both originals (via forwardQueuedRTP) and
// retransmits (via handleNACK) leave the router — and drops selected
// packets before they reach the downtrack, exactly as a network loss
// would. A dropped packet is silent: writeRTP reports it unsent with
// no error, so the router's counters see a loss, not a failure.
//
// Selection is per viewer (one injector per subscription):
//   - by RTP sequence number (DropSequence),
//   - by per-subscription write ordinal (DropOrdinal; 0-based, counting
//     every writeRTP call — originals and retransmits alike),
//   - by per-subscription frame ordinal (DropFrame; 0-based, counting
//     distinct RTP timestamps in arrival order).
//
// By default a matching packet is dropped ONCE: its retransmit passes,
// which is the drop-once loss model (4.1). DropRetransmits makes the
// rule match every time, so the packet never arrives even after NACK
// (4.4 phase B, and the 4.1 fail-before control). Counters record every
// decision, so a forced-drop run can account for every drop (4.4).
type faultInjector struct {
	mu sync.Mutex
	// Rules. A packet matches when any one fires; sequence wins over
	// ordinal, ordinal over frame, for the by-rule counters.
	dropSequences map[uint16]bool
	dropOrdinals  map[uint64]bool
	dropFrames    map[uint64]bool
	// dropRetransmits: when false, a sequence dropped once is allowed
	// on its next attempt (the retransmit); when true, every matching
	// attempt is dropped.
	dropRetransmits bool
	// droppedOnce records sequences already dropped once (drop-once
	// semantics). Keyed by sequence, not ordinal: a retransmit carries
	// the same sequence as the original.
	droppedOnce map[uint16]bool
	// Arrival state for the ordinal/frame rules. Frames are
	// identified by RTP timestamp: a retransmit carries its frame's
	// timestamp, so it maps to the same frame ordinal as the
	// original. One map entry per distinct timestamp seen.
	seenPackets      uint64
	frameOrdinals    map[uint32]uint64
	nextFrameOrdinal uint64

	droppedTotal     atomic.Uint64
	droppedBySeq     atomic.Uint64
	droppedByOrdinal atomic.Uint64
	droppedByFrame   atomic.Uint64
	allowed          atomic.Uint64
}

func newFaultInjector() *faultInjector {
	return &faultInjector{
		dropSequences: make(map[uint16]bool),
		dropOrdinals:  make(map[uint64]bool),
		dropFrames:    make(map[uint64]bool),
		droppedOnce:   make(map[uint16]bool),
		frameOrdinals: make(map[uint32]uint64),
	}
}

// DropSequence drops the packet with this RTP sequence number (once,
// unless DropRetransmits is set).
func (f *faultInjector) DropSequence(sequence uint16) *faultInjector {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dropSequences[sequence] = true
	return f
}

// DropOrdinal drops the nth writeRTP call on this subscription
// (0-based; originals and retransmits both count).
func (f *faultInjector) DropOrdinal(ordinal uint64) *faultInjector {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dropOrdinals[ordinal] = true
	return f
}

// DropFrame drops every packet of the nth frame seen on this
// subscription (0-based, by RTP timestamp in first-seen order). A
// frame is identified by its RTP timestamp, so a retransmit of a
// dropped frame maps to the same frame ordinal and drops again under
// DropRetransmits. (If originals arrive out of timestamp order, the
// ordinal follows arrival order — the injector cannot know the true
// frame order in advance.)
func (f *faultInjector) DropFrame(frame uint64) *faultInjector {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dropFrames[frame] = true
	return f
}

// DropRetransmits makes every matching packet drop on every attempt,
// so NACK retransmission cannot repair it.
func (f *faultInjector) DropRetransmits() *faultInjector {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dropRetransmits = true
	return f
}

// shouldDrop reports whether this packet must be dropped. It is the
// only method the forward path calls, from writeRTP.
func (f *faultInjector) shouldDrop(packet *rtp.Packet) bool {
	if f == nil || packet == nil {
		return false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	ordinal := f.seenPackets
	f.seenPackets++
	frame, ok := f.frameOrdinals[packet.Timestamp]
	if !ok {
		frame = f.nextFrameOrdinal
		f.nextFrameOrdinal++
		f.frameOrdinals[packet.Timestamp] = frame
	}
	matched := ""
	switch {
	case f.dropSequences[packet.SequenceNumber]:
		matched = "sequence"
	case f.dropOrdinals[ordinal]:
		matched = "ordinal"
	case f.dropFrames[frame]:
		matched = "frame"
	}
	if matched == "" {
		f.allowed.Add(1)
		return false
	}
	if !f.dropRetransmits {
		if f.droppedOnce[packet.SequenceNumber] {
			f.allowed.Add(1)
			return false
		}
		f.droppedOnce[packet.SequenceNumber] = true
	}
	f.droppedTotal.Add(1)
	switch matched {
	case "sequence":
		f.droppedBySeq.Add(1)
	case "ordinal":
		f.droppedByOrdinal.Add(1)
	case "frame":
		f.droppedByFrame.Add(1)
	}
	return true
}

// faultInjectorStats is a point-in-time snapshot of an injector's
// counters, for tests and forced-drop accounting (4.4).
type faultInjectorStats struct {
	DroppedTotal     uint64
	DroppedBySeq     uint64
	DroppedByOrdinal uint64
	DroppedByFrame   uint64
	Allowed          uint64
	SeenPackets      uint64
	SeenFrames       uint64
}

func (f *faultInjector) Stats() faultInjectorStats {
	if f == nil {
		return faultInjectorStats{}
	}
	f.mu.Lock()
	seenPackets := f.seenPackets
	seenFrames := uint64(len(f.frameOrdinals))
	f.mu.Unlock()
	return faultInjectorStats{
		DroppedTotal:     f.droppedTotal.Load(),
		DroppedBySeq:     f.droppedBySeq.Load(),
		DroppedByOrdinal: f.droppedByOrdinal.Load(),
		DroppedByFrame:   f.droppedByFrame.Load(),
		Allowed:          f.allowed.Load(),
		SeenPackets:      seenPackets,
		SeenFrames:       seenFrames,
	}
}
