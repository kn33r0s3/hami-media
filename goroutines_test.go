package main

import (
	"runtime"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

// TestGoroutinesPerPeerConnectionLowerBound (5.5): a small-scale VM
// measurement of goroutines per PeerConnection — PCs created with
// AddTrack but never negotiated, so no ICE/DTLS/SRTP transport runs.
// The per-PC figure is therefore a LOWER BOUND on the real per-PC
// cost (the ladder inventory measures ~16/PC with a live connection).
// The test also asserts no goroutine leak: closing all PCs returns
// the count to baseline.
func TestGoroutinesPerPeerConnectionLowerBound(t *testing.T) {
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	baseline := runtime.NumGoroutine()
	const pcs = 20
	api := webrtc.NewAPI()
	codec := webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}
	peers := make([]*webrtc.PeerConnection, 0, pcs)
	for i := 0; i < pcs; i++ {
		peer, err := api.NewPeerConnection(webrtc.Configuration{})
		if err != nil {
			t.Fatalf("NewPeerConnection: %v", err)
		}
		track, err := webrtc.NewTrackLocalStaticRTP(codec, "video", "stream")
		if err != nil {
			t.Fatalf("NewTrackLocalStaticRTP: %v", err)
		}
		if _, err := peer.AddTrack(track); err != nil {
			t.Fatalf("AddTrack: %v", err)
		}
		peers = append(peers, peer)
	}
	time.Sleep(200 * time.Millisecond)
	during := runtime.NumGoroutine()
	delta := during - baseline
	perPC := float64(delta) / pcs
	t.Logf("goroutines: baseline=%d with %d PCs=%d delta=%d per-PC=%.2f (LOWER BOUND: no ICE/DTLS/SRTP, no media flowing)",
		baseline, pcs, during, delta, perPC)
	if delta <= 0 {
		t.Fatalf("creating %d PCs added %d goroutines, want > 0", pcs, delta)
	}
	for _, peer := range peers {
		_ = peer.Close()
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		time.Sleep(100 * time.Millisecond)
		if got := runtime.NumGoroutine(); got <= baseline+2 {
			t.Logf("goroutines after close: %d (baseline %d)", got, baseline)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutines after close = %d, want <= %d within 5s (leak?)",
				runtime.NumGoroutine(), baseline+2)
		}
	}
}
