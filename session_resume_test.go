package main

import (
	"errors"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

// 14.9: after a process restart, clients reconnect and rejoin.
func TestClusterDCrashRestartRejoinsRoom(t *testing.T) {
	calls := 0
	join := func(roomID string) error {
		calls++
		if calls <= 2 {
			// Server is down for the first two attempts, then back.
			return errors.New("connection refused: server down")
		}
		if roomID != "room-restart" {
			return errors.New("wrong room")
		}
		return nil
	}
	client := &clusterDReconnectClient{
		RoomID: "room-restart", MaxAttempts: 5, Backoff: time.Millisecond,
		join: join, sleep: func(time.Duration) {},
	}
	if err := client.reconnect(); err != nil {
		t.Fatalf("client did not rejoin after restart: %v", err)
	}
	if !client.Rejoined {
		t.Fatal("client did not rejoin")
	}
	if client.Attempts != 3 {
		t.Fatalf("rejoin took %d attempts, want 3 (2 down + 1 up)", client.Attempts)
	}
	// A server that never comes back is reported, not retried forever.
	never := &clusterDReconnectClient{
		RoomID: "room-gone", MaxAttempts: 3, Backoff: time.Millisecond,
		join:  func(string) error { return errors.New("connection refused") },
		sleep: func(time.Duration) {},
	}
	if err := never.reconnect(); err == nil {
		t.Fatal("reconnect to a dead server must fail after the budget")
	}
	if never.Attempts != 3 {
		t.Fatalf("attempts %d, want exactly 3", never.Attempts)
	}
}

// 13.7 (encoder side): ffmpeg's WHIP muxer posts an SDP offer to
// the endpoint. The media path needs ICE over UDP (blocked in this
// VM), but the WHIP handshake (POST offer -> 201 answer) must
// succeed: that is what this test asserts.
func TestClusterDWHIPFFmpegEncoderAttempt(t *testing.T) {
	api := webrtc.NewAPI()
	h := newWHIPIngestHandler(api)
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctxTimeout := 15 * time.Second
	cmd := exec.Command("ffmpeg", "-v", "info", "-y",
		"-f", "lavfi", "-i", "testsrc=duration=2:size=320x180:rate=15",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-g", "15", "-bf", "0",
		"-f", "whip", srv.URL+"/whip/room-ffmpeg")
	// Run ffmpeg in the background; it will stall on ICE (VM UDP
	// block) after the HTTP handshake. Give it 12 s, then kill it.
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		t.Logf("ffmpeg whip exited early: %v (offers=%d)", err, h.offerCount())
	case <-time.After(12 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		_ = ctxTimeout
	}
	if got := h.offerCount(); got < 1 {
		t.Fatalf("ffmpeg WHIP encoder never completed an offer/answer handshake (offers=%d)", got)
	}
}
