package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 13.5: recording round trip plays back, deletion works.
func TestClusterDRecordingRoundTripAndDeletion(t *testing.T) {
	base := t.TempDir()
	consents := newClusterDConsentStore()
	// 13.6: no consent yet -> recording must refuse to start.
	rec := newClusterDRecorder(base, consents)
	if _, err := rec.startRecording("rec-1", "room-1", time.Hour); err == nil {
		t.Fatal("recording started without consent")
	}
	consents.record("room-1", "alice", "audio+video")
	r, err := rec.startRecording("rec-1", "room-1", time.Hour)
	if err != nil {
		t.Fatalf("recording with consent refused: %v", err)
	}
	// Write two real segments with the packager's output shape:
	// use ffmpeg to generate a tiny H.264 MP4, then split it into
	// segment files the recorder can remux. Simpler and honest:
	// generate two short MP4s and concat-remux them.
	seg1 := filepath.Join(r.Dir, "seg-0.mp4")
	seg2 := filepath.Join(r.Dir, "seg-1.mp4")
	for _, seg := range []string{seg1, seg2} {
		if err := runFFmpegTestClip(seg); err != nil {
			t.Skipf("ffmpeg clip generation unavailable: %v", err)
		}
	}
	if err := rec.addSegment("rec-1", seg1); err != nil {
		t.Fatal(err)
	}
	if err := rec.addSegment("rec-1", seg2); err != nil {
		t.Fatal(err)
	}
	mp4, err := rec.finishMP4("rec-1")
	if err != nil {
		t.Fatalf("finish MP4: %v", err)
	}
	info, err := os.Stat(mp4)
	if err != nil || info.Size() == 0 {
		t.Fatalf("MP4 missing or empty: %v size %d", err, info.Size())
	}
	// Round trip plays back: ffprobe must read streams and duration.
	if err := probeMP4Plays(mp4); err != nil {
		t.Fatalf("recorded MP4 does not play back: %v", err)
	}
	// Retention: not expired yet; after the retention time it is.
	if got := rec.expired(time.Now()); len(got) != 0 {
		t.Fatalf("recording expired early: %v", got)
	}
	if got := rec.expired(time.Now().Add(2 * time.Hour)); len(got) != 1 || got[0] != "rec-1" {
		t.Fatalf("expired list %v, want [rec-1]", got)
	}
	// Deletion works: files and directory are gone.
	removed, err := rec.deleteRecording("rec-1")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(removed) == 0 {
		t.Fatal("deletion reported no files removed")
	}
	if _, err := os.Stat(r.Dir); !os.IsNotExist(err) {
		t.Fatalf("recording dir still exists after deletion: %v", err)
	}
	if _, err := os.Stat(mp4); !os.IsNotExist(err) {
		t.Fatal("MP4 still exists after deletion")
	}
}

// 13.6: consent is recorded before recording starts, and is visible.
func TestClusterDRecordingConsentFlow(t *testing.T) {
	consents := newClusterDConsentStore()
	if consents.has("room-9") {
		t.Fatal("consent present before recording it")
	}
	c := consents.record("room-9", "bob", "video")
	if c.RoomID != "room-9" || c.Participant != "bob" || c.ConsentedAt.IsZero() {
		t.Fatalf("consent record incomplete: %+v", c)
	}
	if !consents.has("room-9") {
		t.Fatal("consent not visible after recording it")
	}
}

func TestClusterDRecordingRejectsTraversalIDs(t *testing.T) {
	consents := newClusterDConsentStore()
	consents.record("room-1", "alice", "audio+video")
	rec := newClusterDRecorder(t.TempDir(), consents)
	for _, badID := range []string{"", ".", "..", "../escape", "a/b", `a\b`} {
		if _, err := rec.startRecording(badID, "room-1", time.Hour); err == nil {
			t.Fatalf("expected error for invalid recording id %q", badID)
		}
	}
}
