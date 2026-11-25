package outputs

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// syntheticSPS is a minimal 640x360-ish SPS (profile 100, level 3.1).
// It only needs to parse as SPS/PPS bytes for the avcC box; the
// segment bytes are verified structurally and by ffprobe in the
// evidence run, not by decoding here.
var syntheticSPS = []byte{0x64, 0x00, 0x1f, 0xac, 0xd9, 0x40, 0x78, 0x02, 0x27, 0xe5, 0x84}
var syntheticPPS = []byte{0x68, 0xee, 0x3c, 0x80}

func testFrames(n int) []H264Frame {
	frames := make([]H264Frame, n)
	for i := range frames {
		nal := []byte{0x41, 0x9a, 0x00, 0x01} // non-IDR slice, type 1
		key := false
		if i%10 == 0 {
			nal = []byte{0x65, 0x88, 0x84, 0x00} // IDR slice, type 5
			key = true
		}
		frames[i] = H264Frame{NALs: [][]byte{syntheticSPS, syntheticPPS, nal}, KeyFrame: key, Timestamp: uint32(i * 3000), Duration: 3000}
	}
	return frames
}

func TestInitSegmentHasFTYPAndMOOV(t *testing.T) {
	init, err := InitSegment(syntheticSPS, syntheticPPS, 640, 360, 90000)
	if err != nil {
		t.Fatal(err)
	}
	if len(init) < 8 || string(init[4:8]) != "ftyp" {
		t.Fatalf("init segment does not start with ftyp: %q", init[:8])
	}
	if !bytes.Contains(init, []byte("moov")) || !bytes.Contains(init, []byte("avcC")) {
		t.Fatalf("init segment missing moov/avcC")
	}
	if !bytes.Contains(init, []byte("avc1")) {
		t.Fatalf("init segment missing avc1 sample entry")
	}
}

func TestInitSegmentRequiresSPSPPS(t *testing.T) {
	if _, err := InitSegment(nil, syntheticPPS, 640, 360, 90000); err == nil {
		t.Fatal("init segment without SPS must fail")
	}
	if _, err := InitSegment(syntheticSPS, nil, 640, 360, 90000); err == nil {
		t.Fatal("init segment without PPS must fail")
	}
}

func TestMediaSegmentStartsOnKeyframe(t *testing.T) {
	frames := testFrames(10)
	seg, err := MediaSegment(frames, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(seg[4:8]) != "moof" {
		t.Fatalf("segment does not start with moof: %q", seg[:8])
	}
	// Non-keyframe start must be refused (13.2).
	if _, err := MediaSegment(frames[1:], 2, 3000); err == nil {
		t.Fatal("segment starting on a non-keyframe must be refused")
	}
	// Segment must contain an mdat after the moof.
	if !bytes.Contains(seg, []byte("mdat")) {
		t.Fatal("segment missing mdat")
	}
}

func TestPackagerSegmentsAreKeyframeAligned(t *testing.T) {
	p, err := NewPackager(10)
	if err != nil {
		t.Fatal(err)
	}
	frames := testFrames(30)
	// Prepend 3 non-keyframes: they must be dropped, never start a segment.
	lead := []H264Frame{
		{NALs: [][]byte{{0x41, 0x01}}, KeyFrame: false, Duration: 3000},
		{NALs: [][]byte{{0x41, 0x02}}, KeyFrame: false, Duration: 3000},
		{NALs: [][]byte{{0x41, 0x03}}, KeyFrame: false, Duration: 3000},
	}
	for _, fr := range append(lead, frames...) {
		p.Add(fr)
	}
	segments, dropped := p.Close()
	if dropped != 3 {
		t.Fatalf("expected 3 leading non-keyframes dropped, got %d", dropped)
	}
	if len(segments) == 0 {
		t.Fatal("no segments produced")
	}
	for i, seg := range segments {
		if len(seg) == 0 || !seg[0].KeyFrame {
			t.Fatalf("segment %d does not start on a keyframe", i)
		}
		if _, err := MediaSegment(seg, uint32(i+1), uint64(i*30000)); err != nil {
			t.Fatalf("segment %d failed to package: %v", i, err)
		}
	}
}

func TestMediaPlaylistReferencesInitSegment(t *testing.T) {
	segs := []SegmentInfo{
		{Filename: "seg-0.m4s", DurationSecs: 2.0, KeyFrame: true, Frames: 10},
		{Filename: "seg-1.m4s", DurationSecs: 2.0, KeyFrame: true, Frames: 10},
	}
	pl := MediaPlaylist(segs, "init.mp4", 2.0)
	if !strings.Contains(pl, "#EXT-X-MAP:URI=\"init.mp4\"") {
		t.Fatalf("playlist missing EXT-X-MAP: %s", pl)
	}
	if !strings.Contains(pl, "seg-0.m4s") || !strings.Contains(pl, "seg-1.m4s") {
		t.Fatalf("playlist missing segments: %s", pl)
	}
	if !strings.Contains(pl, "#EXT-X-ENDLIST") {
		t.Fatalf("playlist missing ENDLIST: %s", pl)
	}
	master := MasterPlaylist("media.m3u8", 1200000, 640, 360)
	if !strings.Contains(master, "BANDWIDTH=1200000") || !strings.Contains(master, "media.m3u8") {
		t.Fatalf("master playlist malformed: %s", master)
	}
}

func TestTrunDataOffsetPointsAtMdat(t *testing.T) {
	frames := testFrames(5)
	seg, err := MediaSegment(frames, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Find mdat box start.
	idx := bytes.Index(seg, []byte("mdat"))
	if idx < 0 {
		t.Fatal("no mdat")
	}
	mdatStart := idx - 4 // box size field precedes type
	got := binary.BigEndian.Uint32(seg[84:88])
	// data-offset is relative to the start of the moof box and points
	// at the first sample byte: moof size + 8 (mdat header).
	moofSize := binary.BigEndian.Uint32(seg[0:4])
	if got != moofSize+8 {
		t.Fatalf("trun data-offset %d, want moof size %d + 8", got, moofSize)
	}
	if int(got) != mdatStart+8 {
		t.Fatalf("trun data-offset %d does not point at first sample byte %d", got, mdatStart+8)
	}
}

func TestInitSegmentRejectsShortSPSWithoutPanic(t *testing.T) {
	for _, sps := range [][]byte{nil, {}, {0x67}, {0x67, 0x42, 0x00}} {
		if _, err := InitSegment(sps, []byte{0x68, 0xce, 0x3c, 0x80}, 640, 360, 90000); err == nil {
			t.Fatalf("expected error for short SPS of len %d", len(sps))
		}
	}
}
