package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

// TestEvaluateStallGate (5.6): the gate itself — the p99 median must
// stay at or below 50 ms.
func TestEvaluateStallGate(t *testing.T) {
	cases := []struct {
		median time.Duration
		pass   bool
	}{
		{0, true},
		{49 * time.Millisecond, true},
		{50 * time.Millisecond, true},
		{51 * time.Millisecond, false},
		{104910 * time.Microsecond, false}, // control A, 104.91 ms
		{221330 * time.Microsecond, false}, // 7,500 latency failure
	}
	for _, tc := range cases {
		if got := evaluateStallGate(tc.median); got != tc.pass {
			t.Fatalf("evaluateStallGate(%v) = %v, want %v", tc.median, got, tc.pass)
		}
	}
}

// TestStallGateP99Median (5.6): the median helper — odd, even, empty.
func TestStallGateP99Median(t *testing.T) {
	if got := stallGateP99Median(nil); got != 0 {
		t.Fatalf("median of empty = %v, want 0", got)
	}
	if got := stallGateP99Median([]time.Duration{30 * time.Millisecond}); got != 30*time.Millisecond {
		t.Fatalf("median of one = %v", got)
	}
	if got := stallGateP99Median([]time.Duration{10 * time.Millisecond, 30 * time.Millisecond, 20 * time.Millisecond}); got != 20*time.Millisecond {
		t.Fatalf("median of [10,30,20] = %v, want 20ms", got)
	}
	if got := stallGateP99Median([]time.Duration{10 * time.Millisecond, 30 * time.Millisecond}); got != 20*time.Millisecond {
		t.Fatalf("median of [10,30] = %v, want 20ms", got)
	}
}

// TestStallGateWiredIntoMeasurementPass (5.6): the gate is part of
// the STRICT proof verdict — a result whose per-viewer p99 median
// exceeds 50 ms fails even when every other check passes. It does
// NOT apply to dissolve scoring: dissolve mode is a separate regime
// whose latency bound is the configured maxP99 per viewer, and the
// pre-existing dissolve test records that it accepts a 120 ms p99
// at maxP99=150 ms. (An earlier draft wired the gate into dissolve
// scoring too; it broke that test and was reverted — see the 5.6
// note. Whether dissolve mode should also carry the median gate is
// an open design question.)
func TestStallGateWiredIntoMeasurementPass(t *testing.T) {
	buildResult := func(p99 time.Duration) sfuProofResult {
		viewers := make([]sfuViewerResult, 3)
		for i := range viewers {
			viewers[i] = sfuViewerResult{
				Latency: &Summary{
					Frames: 302, P99: p99, Pass: true,
				},
				OnlyRequested:  true,
				PeerConnection: peerConnectionConnected,
			}
		}
		return sfuProofResult{
			PublisherState:                 peerConnectionConnected,
			UnauthorizedSubscriptionDenied: true,
			CleanupVerified:                true,
			Viewers:                        viewers,
		}
	}
	if !sfuMeasurementPass(buildResult(20*time.Millisecond), 3, false) {
		t.Fatal("median 20 ms must pass the strict verdict")
	}
	if sfuMeasurementPass(buildResult(60*time.Millisecond), 3, false) {
		t.Fatal("median 60 ms must fail the strict verdict")
	}
	// Dissolve: the median gate does NOT apply. Per-viewer P99
	// (60 ms) is under maxP99 (150 ms) and the dissolved fraction
	// is 0, so dissolve scoring passes it — that is the recorded
	// dissolve design (a latency the strict verdict rejects can
	// still pass dissolve scoring at a configured limit).
	dissolvePass := sfuDissolveMeasurementPass(buildResult(20*time.Millisecond), 3, false, 150*time.Millisecond, 0.01)
	if !dissolvePass {
		t.Fatal("median 20 ms must pass the dissolve verdict")
	}
	if !sfuDissolveMeasurementPass(buildResult(60*time.Millisecond), 3, false, 150*time.Millisecond, 0.01) {
		t.Fatal("median 60 ms must pass the dissolve verdict at maxP99=150ms (the median gate does not apply to dissolve scoring)")
	}
}

// stallDrain is one viewer's drain in the 5.6 harness: it reads the
// subscription queue, records the enqueue→dequeue latency, and
// forwards through the real forwardQueuedRTP.
type stallDrain struct {
	subscription *sfuSubscription
	mu           sync.Mutex
	latencies    []time.Duration
	done         chan struct{}
}

// runStallHarness runs one in-process forward "run" and returns the
// per-viewer p99 latencies. One publisher floods packets into each
// subscription (production 64-packet queue); one drain goroutine per
// subscription dequeues, records the latency, and forwards via the
// real forwardQueuedRTP. The publisher stops when every viewer has
// `samples` latencies or the 30 s deadline fires. drainDelay is a
// fixed sleep in the drain before each forward (the network control).
// stall is an optional control goroutine (CPU/GC) run during the
// harness; it receives a stop channel and closes done on exit.
func runStallHarness(t *testing.T, viewers, samples int, drainDelay time.Duration, stall func(stop <-chan struct{}, done chan<- struct{})) []time.Duration {
	t.Helper()
	router := &sfuRouter{}
	codec := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}
	subs := make([]*sfuSubscription, viewers)
	drains := make([]*stallDrain, viewers)
	var drainWG sync.WaitGroup
	for i := range subs {
		downTrack, err := webrtc.NewTrackLocalStaticRTP(codec, "video", "stream")
		if err != nil {
			t.Fatalf("NewTrackLocalStaticRTP: %v", err)
		}
		subs[i] = &sfuSubscription{
			viewerID:  fmt.Sprintf("hami-sfu-viewer-%d", i),
			active:    true,
			h264Video: true,
			downTrack: downTrack,
			rtpQueue:  make(chan queuedRTP, sfuRTPQueueSize),
			nackQueue: make(chan queuedNACK, nackQueueSize),
		}
		drains[i] = &stallDrain{subscription: subs[i], done: make(chan struct{})}
		drainWG.Add(1)
		go func(d *stallDrain) {
			defer drainWG.Done()
			for entry := range d.subscription.rtpQueue {
				latency := time.Since(entry.enqueuedAt)
				if drainDelay > 0 {
					time.Sleep(drainDelay)
				}
				router.forwardQueuedRTP(d.subscription, entry)
				d.mu.Lock()
				d.latencies = append(d.latencies, latency)
				n := len(d.latencies)
				d.mu.Unlock()
				if n == samples {
					close(d.done)
				}
			}
		}(drains[i])
	}
	stopStall := make(chan struct{})
	stallDone := make(chan struct{})
	if stall != nil {
		go stall(stopStall, stallDone)
	} else {
		close(stallDone)
	}
	deadline := time.Now().Add(30 * time.Second)
	packet := &rtp.Packet{Payload: []byte{0x41, 0x9a}}
	var sequence uint16
	for time.Now().Before(deadline) {
		allDone := true
		for _, d := range drains {
			select {
			case <-d.done:
			default:
				allDone = false
			}
		}
		if allDone {
			break
		}
		packet.Header.SequenceNumber = sequence
		sequence++
		now := time.Now()
		for _, sub := range subs {
			_, _ = sub.enqueueRTP(packet, layerDelta, now)
		}
		time.Sleep(100 * time.Microsecond)
	}
	close(stopStall)
	select {
	case <-stallDone:
	case <-time.After(10 * time.Second):
		t.Fatal("stall control did not stop within 10s")
	}
	for _, sub := range subs {
		sub.stop()
	}
	drainWG.Wait()
	p99s := make([]time.Duration, 0, viewers)
	for _, d := range drains {
		d.mu.Lock()
		latencies := append([]time.Duration(nil), d.latencies...)
		d.mu.Unlock()
		if len(latencies) < samples {
			t.Fatalf("viewer collected %d/%d latencies within 30s", len(latencies), samples)
		}
		p99s = append(p99s, Percentile(latencies, 99))
	}
	return p99s
}

// cpuStallControl (5.6) is the in-process CPU-starvation control, the
// analog of Mac control A: GOMAXPROCS=1 plus 8 spinning goroutines,
// so the forward (drain) goroutines are starved of CPU. Restores
// GOMAXPROCS on exit.
func cpuStallControl(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	var counter atomic.Uint64
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				counter.Add(1)
			}
		}()
	}
	<-stop
	wg.Wait()
}

// gcStallControl (5.6) is the in-process GC-pressure control: a low
// GOMEMLIMIT (64 MiB) over a retained 40 MiB live set, GOGC=1, plus
// two goroutines allocating 1 MiB of garbage per iteration, so the
// collector runs almost continuously. Restores GOGC and the memory
// limit on exit.
//
// MEASURED LIMIT (25 Nov 2026, three drafts): this control does NOT
// push the forward p99 median over the 50 ms gate in the VM —
// GOGC=1 + churn alone measured 8.01 ms, a 256 MiB retained flat-byte
// heap measured 0.40 ms, and this GOMEMLIMIT draft measured 0.38 ms.
// Go's concurrent collector bounds stop-the-world pauses to low-ms
// even under extreme churn, and the drain (microseconds per packet)
// recovers between pauses, so the queue never builds a 50 ms backlog.
// A >50 ms GC pause needs a multi-GB pointer-heavy heap — the scale
// runs' actual condition, not reproducible in-process here. The GC
// leg of the gate is therefore a Mac-run item (see the 5.6 note); the
// test below proves the pressure is real (NumGC rises) and records
// the VM ceiling.
func gcStallControl(stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	previousGC := debug.SetGCPercent(1)
	defer debug.SetGCPercent(previousGC)
	previousLimit := debug.SetMemoryLimit(64 << 20)
	defer debug.SetMemoryLimit(previousLimit)
	retained := make([][]byte, 0, 40)
	for i := 0; i < 40; i++ {
		retained = append(retained, make([]byte, 1<<20))
	}
	defer func() { _ = retained }()
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = make([]byte, 1<<20)
			}
		}()
	}
	<-stop
	wg.Wait()
}

// TestStallGateFailsCPUStall (5.6): the CPU-starvation control must
// produce a run the gate FAILS.
func TestStallGateFailsCPUStall(t *testing.T) {
	p99s := runStallHarness(t, 3, 100, 0, cpuStallControl)
	median := stallGateP99Median(p99s)
	t.Logf("CPU stall: per-viewer p99s=%v median=%v", p99s, median)
	if evaluateStallGate(median) {
		t.Fatalf("CPU stall must FAIL the gate, median=%v", median)
	}
}

// TestStallGateGCStallMeasured (5.6): the GC-pressure control's
// honest in-process result. The control IS applying real pressure
// (NumGC rises sharply during the run), but Go's concurrent collector
// bounds its pauses to low-ms and the microsecond-fast drain recovers
// between them, so the p99 median stays below the 50 ms gate in the
// VM — medians across three control drafts: 8.01 ms, 0.40 ms,
// 0.38 ms. This test is the trip wire: if a future Go version, heap
// shape, or drain change ever lets GC pressure push the median over
// 50 ms in the VM, it fails and the GC leg is reproduced in-process.
// The GC leg that FAILS the gate is a Mac-run item (multi-GB heap);
// see the 5.6 note.
func TestStallGateGCStallMeasured(t *testing.T) {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	p99s := runStallHarness(t, 3, 100, 0, gcStallControl)
	runtime.ReadMemStats(&after)
	median := stallGateP99Median(p99s)
	t.Logf("GC stall: per-viewer p99s=%v median=%v NumGC during run=%d",
		p99s, median, after.NumGC-before.NumGC)
	if after.NumGC-before.NumGC == 0 {
		t.Fatal("GC control applied no pressure: NumGC did not rise during the run")
	}
	if !evaluateStallGate(median) {
		t.Fatalf("GC stall unexpectedly FAILS the gate in the VM, median=%v (see note)", median)
	}
}

// TestStallGateFailsNetworkStall (5.6): the network control — a fixed
// 200 ms delay in the drain, modelling a stalled network that backs
// the queue up — must produce a run the gate FAILS.
func TestStallGateFailsNetworkStall(t *testing.T) {
	p99s := runStallHarness(t, 3, 25, 200*time.Millisecond, nil)
	median := stallGateP99Median(p99s)
	t.Logf("network stall: per-viewer p99s=%v median=%v", p99s, median)
	if evaluateStallGate(median) {
		t.Fatalf("network stall must FAIL the gate, median=%v", median)
	}
}

// TestStallGatePassesCleanRuns (5.6): three clean runs (no stall,
// no drain delay) must all PASS the gate.
func TestStallGatePassesCleanRuns(t *testing.T) {
	for run := 1; run <= 3; run++ {
		p99s := runStallHarness(t, 3, 100, 0, nil)
		median := stallGateP99Median(p99s)
		t.Logf("clean run %d: per-viewer p99s=%v median=%v", run, p99s, median)
		if !evaluateStallGate(median) {
			t.Fatalf("clean run %d must PASS the gate, median=%v", run, median)
		}
	}
}
