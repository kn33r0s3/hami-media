package main

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/rtp"
)

// TestEnqueueRTPConcurrentStopDoesNotPanic (5.2): the one per-packet
// lock in the forward path is enqueueRTP's s.mu.RLock. It exists so a
// send on rtpQueue cannot race stop() closing that queue (a send on a
// closed channel panics). Concurrent enqueues racing a stop must
// finish without panic and without a race.
func TestEnqueueRTPConcurrentStopDoesNotPanic(t *testing.T) {
	subscription := &sfuSubscription{
		active:    true,
		h264Video: true,
		rtpQueue:  make(chan queuedRTP, 4),
		nackQueue: make(chan queuedNACK, nackQueueSize),
	}
	packet := &rtp.Packet{
		Header:  rtp.Header{SequenceNumber: 1},
		Payload: []byte{0x41, 0x9a},
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, err := subscription.enqueueRTP(packet, layerDelta, time.Now())
				if err != nil {
					// "queue remained full" is a legitimate
					// contention outcome, not a failure.
					continue
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(time.Millisecond)
		subscription.stop()
	}()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent enqueue/stop did not finish within 10s")
	}
	if subscription.isActive() {
		t.Fatal("subscription is still active after stop()")
	}
}
