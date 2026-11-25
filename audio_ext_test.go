package main

import (
	"bytes"
	"testing"
	"time"
)

// 12.1: one speaker -> 100 listeners, Opus payloads unchanged.
func TestClusterDOpusOneSpeakerHundredListeners(t *testing.T) {
	fwd := newClusterDOpusForwarder()
	var listeners []*clusterDOpusListener
	for i := 0; i < 100; i++ {
		listeners = append(listeners, fwd.addListener("listener-"+itoa(i)))
	}
	// 50 Opus frames (1 s at 20 ms), distinct payloads.
	var source [][]byte
	for i := 0; i < 50; i++ {
		payload := []byte{0x78, 0x9a, byte(i), 0x01, 0x02, 0x03, 0x04, 0x05}
		source = append(source, payload)
		fwd.forward(payload)
	}
	if fwd.forwarded != 50 {
		t.Fatalf("forwarded %d frames, want 50", fwd.forwarded)
	}
	for _, l := range listeners {
		if got := l.count(); got != 50 {
			t.Fatalf("listener %s got %d frames, want 50", l.id, got)
		}
		if !l.payloadsEqual(source) {
			t.Fatalf("listener %s payload bytes differ from source (transcoding or corruption)", l.id)
		}
	}
}

// 12.2: speaker change fires within the stated 500 ms.
func TestClusterDActiveSpeakerFiresWithinStatedTime(t *testing.T) {
	d := newClusterDActiveSpeaker()
	base := time.Now()
	// Alice speaks loudly; Bob is quiet.
	d.reportLevel("alice", 90, base)
	d.reportLevel("bob", 10, base)
	if got := d.currentSpeaker(); got != "" {
		t.Fatalf("no speaker should be confirmed yet (hold window), got %q", got)
	}
	// Alice stays loudest past the hold window -> event fires.
	ev := d.reportLevel("alice", 95, base.Add(150*time.Millisecond))
	if ev == nil {
		t.Fatal("active-speaker event did not fire after hold window")
	}
	if ev.Speaker != "alice" {
		t.Fatalf("event speaker %q, want alice", ev.Speaker)
	}
	// Bob takes over: event must fire within 500 ms of Bob becoming loudest.
	d.reportLevel("bob", 100, base.Add(200*time.Millisecond))
	ev2 := d.reportLevel("bob", 100, base.Add(350*time.Millisecond))
	if ev2 == nil {
		t.Fatal("speaker-change event did not fire")
	}
	if ev2.Speaker != "bob" {
		t.Fatalf("event speaker %q, want bob", ev2.Speaker)
	}
	latency := ev2.At.Sub(base.Add(200 * time.Millisecond))
	if latency > 500*time.Millisecond {
		t.Fatalf("speaker change took %s, over the stated 500 ms", latency)
	}
	// Silence: nobody loud enough -> no new event.
	if ev3 := d.reportLevel("alice", 5, base.Add(400*time.Millisecond)); ev3 != nil {
		t.Fatalf("silent level must not fire an event, got %+v", ev3)
	}
}

// 12.3: silent periods send only DTX keepalives.
func TestClusterDDTXSilentPeriodSendsOnlyKeepalives(t *testing.T) {
	g := newClusterDDTXGate()
	speech := []byte{0x78, 0x9a, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	dtx := []byte{0xf8} // 1-byte DTX comfort-noise frame
	// 10 speech frames go out.
	for i := 0; i < 10; i++ {
		if !g.send(speech, 80) {
			t.Fatal("speech frame dropped while speaking")
		}
	}
	// 10 silent rounds (speech + DTX each): the first 2 speech frames
	// still pass (silence not yet declared after 5 silent frames),
	// then only DTX keepalives pass; speech frames are dropped.
	sentSpeech, sentDTX := 0, 0
	for i := 0; i < 10; i++ {
		if g.send(speech, 0) {
			sentSpeech++
		}
		if g.send(dtx, 0) {
			sentDTX++
		}
	}
	if sentSpeech != 2 {
		t.Fatalf("silent period sent %d speech frames, want 2 (grace frames before silence declared)", sentSpeech)
	}
	if sentDTX != 10 {
		t.Fatalf("silent period sent %d DTX keepalives, want 10", sentDTX)
	}
	if g.dropped == 0 {
		t.Fatal("gate dropped nothing in silence")
	}
	if !isDTXFrame(dtx) || isDTXFrame(speech) {
		t.Fatal("DTX classification wrong")
	}
}

// 12.6: lip-sync offset within the stated 80 ms tolerance, per layer.
func TestClusterDAudioVideoSyncPerLayer(t *testing.T) {
	// Anchor: audio TS 48000 (1 s in) and video TS 90000 (1 s in).
	// Layer 720p: audio 48000+480 (10 ms later), video 90000+900.
	if err := checkAVSync(48000, 48480, 90000, 90900); err != nil {
		t.Fatalf("in-sync layers failed: %v", err)
	}
	// 50 ms offset: within tolerance.
	if err := checkAVSync(48000, 50400, 90000, 90000); err != nil {
		t.Fatalf("50 ms offset should pass: %v", err)
	}
	// 120 ms offset: outside tolerance.
	if err := checkAVSync(48000, 53760, 90000, 90000); err == nil {
		t.Fatal("120 ms offset must fail the 80 ms tolerance")
	}
	// Offset is symmetric: video ahead by 120 ms also fails.
	if err := checkAVSync(48000, 48000, 90000, 100800); err == nil {
		t.Fatal("video-ahead 120 ms offset must fail")
	}
	_ = bytes.Equal
}
