// Cluster C §6 adaptation — keyframes, simulcast tracking, bandwidth
// estimation, dynacast, keyframe-request handling.
//
// Jobs: 6.3 keyframe every 2 s per layer with SPS/PPS at each keyframe;
// 6.4 simulcast ingest (three-layer publisher accepted, router tracks
// each layer separately — keyed on sfuTrackKey, job 2.6's layer key
// (PublisherID+TrackID+Layer), re-keyed at merge from the tracker's
// own simulcastLayerKey; the layer names stay this file's own domain
// ("top" | "middle" | "base"), validated on register); 6.7 bandwidth
// estimate from
// TWCC/REMB; 6.8 dynacast; 6.11 encoder honours PLI/FIR.
//
// All of it is in-process and deterministic: no real encoder, no UDP,
// so the VM sandbox does not block it. The device-matrix part of 6.11
// (job 10.5, BLOCKED — no devices) is recorded, not faked.
package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// --- 6.3 keyframe interval + parameter sets ---------------------------------

const keyframeInterval = 2 * time.Second // job 6.3: every layer, every 2 s

// keyframeDue reports whether a keyframe is due for a layer whose last
// keyframe was at last, evaluated at now.
func keyframeDue(last, now time.Time) bool {
	return !now.Before(last.Add(keyframeInterval))
}

// h264NALType returns the NAL type of the first byte of a single-NAL
// payload (type = byte & 0x1f). 7=SPS, 8=PPS, 5=IDR.
func h264NALType(payload []byte) (int, bool) {
	if len(payload) == 0 {
		return 0, false
	}
	return int(payload[0] & 0x1f), true
}

// ensureParameterSets reports whether an IDR frame payload carries SPS
// (7) and PPS (8) with it — either as single NALs at the head or inside
// a STAP-A (type 24) aggregation. Job 6.3 requires SPS+PPS with EACH
// keyframe; a keyframe without them is a defect, not a pass.
func ensureParameterSets(payload []byte) (hasSPS, hasPPS bool) {
	if len(payload) == 0 {
		return false, false
	}
	nalType, _ := h264NALType(payload)
	switch nalType {
	case 7:
		hasSPS = true
		// A second NAL may follow in the same packet only in STAP-A;
		// single-NAL SPS alone does not carry PPS — caller must check
		// the following packets of the same frame.
	case 8:
		hasPPS = true
	case 24: // STAP-A: [type][len][nal][len][nal]...
		off := 1
		for off+2 <= len(payload) {
			size := int(payload[off])<<8 | int(payload[off+1])
			off += 2
			if size <= 0 || off+size > len(payload) {
				break
			}
			switch int(payload[off] & 0x1f) {
			case 7:
				hasSPS = true
			case 8:
				hasPPS = true
			}
			off += size
		}
	}
	return hasSPS, hasPPS
}

// frameHasParameterSets checks a whole frame (ordered NAL payloads):
// SPS and PPS must appear in the frame's packets before/at the IDR.
func frameHasParameterSets(nals [][]byte) bool {
	var sps, pps bool
	for _, nal := range nals {
		a, b := ensureParameterSets(nal)
		sps = sps || a
		pps = pps || b
	}
	return sps && pps
}

// --- 6.4 simulcast ingest tracking ------------------------------------------

// simulcastLayerState is the router-side per-layer state for one layer.
type simulcastLayerState struct {
	Key       sfuTrackKey
	SSRC      uint32
	Packets   uint64
	LastSeq   uint16
	LastSeen  time.Time
	Keyframes uint64
}

// simulcastTracker tracks each layer of each three-layer publisher
// separately (job 6.4 Done line). Register is called on ingest; a
// publisher that offers only one layer registers one entry, a
// three-layer publisher registers three.
type simulcastTracker struct {
	mu     sync.Mutex
	layers map[sfuTrackKey]*simulcastLayerState
}

func newSimulcastTracker() *simulcastTracker {
	return &simulcastTracker{layers: map[sfuTrackKey]*simulcastLayerState{}}
}

// registerLayer accepts one simulcast layer. Duplicate registration of
// the same key with a different SSRC is an error (layer collision).
func (t *simulcastTracker) registerLayer(key sfuTrackKey, ssrc uint32, now time.Time) error {
	if key.PublisherID == "" || key.TrackID == "" {
		return errors.New("simulcast layer needs publisher and track IDs")
	}
	if key.Layer != "top" && key.Layer != "middle" && key.Layer != "base" {
		return fmt.Errorf("unknown simulcast layer %q", key.Layer)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if existing, ok := t.layers[key]; ok {
		if existing.SSRC != ssrc {
			return fmt.Errorf("simulcast layer %v already registered with SSRC %d, got %d", key, existing.SSRC, ssrc)
		}
		return nil
	}
	t.layers[key] = &simulcastLayerState{Key: key, SSRC: ssrc, LastSeen: now}
	return nil
}

// recordPacket attributes one packet to its layer (per-layer SSRC
// mapping — the feedback for a layer maps to that layer's own SSRC).
func (t *simulcastTracker) recordPacket(key sfuTrackKey, seq uint16, isKeyframe bool, now time.Time) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.layers[key]
	if !ok {
		return fmt.Errorf("simulcast layer %v is not registered", key)
	}
	st.Packets++
	st.LastSeq = seq
	st.LastSeen = now
	if isKeyframe {
		st.Keyframes++
	}
	return nil
}

// layerCount returns how many layers are tracked for one publisher/track.
func (t *simulcastTracker) layerCount(publisherID, trackID string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for k := range t.layers {
		if k.PublisherID == publisherID && k.TrackID == trackID {
			n++
		}
	}
	return n
}

// state returns a copy of one layer's state.
func (t *simulcastTracker) state(key sfuTrackKey) (simulcastLayerState, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	st, ok := t.layers[key]
	if !ok {
		return simulcastLayerState{}, false
	}
	return *st, true
}

// --- 6.7 bandwidth estimate from TWCC/REMB ----------------------------------
//
// bandwidthEstimator is a deliberately simple, stated estimator: it
// tracks loss fraction and RTT from RTCP feedback (TWCC/REMB reports
// feed it; the transport here is the estimator input, not the wire).
// Estimate = last good throughput sample, multiplicatively decreased
// by loss (each 1% loss above 2% cuts 5%) and by RTT growth. The
// tolerance in the test is ±20% on a link whose true capacity is
// known — the Done line's "stated tolerance".
type bandwidthEstimator struct {
	mu           sync.Mutex
	estimateKbps int // current estimate
	lastGoodKbps int
	samples      int
}

func newBandwidthEstimator(initialKbps int) *bandwidthEstimator {
	if initialKbps <= 0 {
		initialKbps = 200
	}
	return &bandwidthEstimator{estimateKbps: initialKbps, lastGoodKbps: initialKbps}
}

// update feeds one feedback sample: throughputKbps observed on the
// link, lossFraction 0..1, rttMs. Returns the new estimate.
func (e *bandwidthEstimator) update(throughputKbps int, lossFraction float64, rttMs int) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	if throughputKbps <= 0 {
		return e.estimateKbps
	}
	est := throughputKbps
	if lossFraction > 0.02 {
		// 5% cut per 1% loss above 2%.
		cut := 1.0 - 0.05*((lossFraction-0.02)*100)
		if cut < 0.2 {
			cut = 0.2
		}
		est = int(float64(est) * cut)
	}
	if rttMs > 150 {
		est = est * 3 / 4
	}
	if est < 50 {
		est = 50
	}
	// Smooth: 70% new, 30% old, so one noisy sample cannot swing the
	// layer selection.
	e.estimateKbps = (est*7 + e.estimateKbps*3) / 10
	e.lastGoodKbps = e.estimateKbps
	e.samples++
	return e.estimateKbps
}

func (e *bandwidthEstimator) estimate() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.estimateKbps
}

// --- 6.8 dynacast ------------------------------------------------------------
//
// dynacastLayerGate: a layer sends packets only while at least one
// subscriber is on it. Zero subscribers => zero packets (job 6.8).
type dynacastGate struct {
	mu          sync.Mutex
	subscribers map[string]int // layer name -> subscriber count
	sent        map[string]uint64
	suppressed  map[string]uint64
}

func newDynacastGate() *dynacastGate {
	return &dynacastGate{subscribers: map[string]int{}, sent: map[string]uint64{}, suppressed: map[string]uint64{}}
}

func (g *dynacastGate) subscribe(layer string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.subscribers[layer]++
}

func (g *dynacastGate) unsubscribe(layer string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.subscribers[layer] > 0 {
		g.subscribers[layer]--
	}
}

// send reports whether a packet for layer may be sent. When nobody
// subscribes, the packet is suppressed and counted, never sent.
func (g *dynacastGate) send(layer string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.subscribers[layer] == 0 {
		g.suppressed[layer]++
		return false
	}
	g.sent[layer]++
	return true
}

func (g *dynacastGate) counts(layer string) (sent, suppressed uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.sent[layer], g.suppressed[layer]
}

// --- 6.11 PLI/FIR -> keyframe -----------------------------------------------
//
// keyframeRequestTracker: on a PLI or FIR the encoder must produce a
// keyframe within the stated time. The stated time here is 500 ms
// (one GOP is 2 s; a request answered after a full GOP is a stall, so
// the bar is a quarter GOP). The VM-testable part is the request ->
// keyframe handshake in-process; the device-matrix part (job 10.5,
// BLOCKED) is the same handshake against a real encoder on each
// device, and is recorded as owed in docs/cluster-c-results.md.
const keyframeRequestDeadline = 500 * time.Millisecond

type keyframeRequestTracker struct {
	mu             sync.Mutex
	requested      time.Time
	fulfilled      time.Time
	requests       int
	fulfilledCount int
	late           int
}

// request records a PLI/FIR at now.
func (k *keyframeRequestTracker) request(now time.Time) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.requested = now
	k.requests++
}

// keyframe records a keyframe produced at now. It fulfils the latest
// outstanding request if it arrives within the deadline.
func (k *keyframeRequestTracker) keyframe(now time.Time) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.requested.IsZero() {
		return false
	}
	if now.Sub(k.requested) <= keyframeRequestDeadline {
		k.fulfilledCount++
	} else {
		k.late++
	}
	k.fulfilled = now
	k.requested = time.Time{}
	return true
}

// stats returns (requests, fulfilled-in-time, late).
func (k *keyframeRequestTracker) stats() (int, int, int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.requests, k.fulfilledCount, k.late
}
