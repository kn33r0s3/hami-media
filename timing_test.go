package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// Job 3.1: the timestamp step follows the SEND interval, not the
// sample's stored Duration. At 5 fps the step is 18,000 ticks
// (200 ms at 90 kHz); at 30 fps it is 3,000 ticks (33.33 ms). The
// pre-fix code stamped 5,400 ticks (60 ms) at both rates.
func TestVideoTimestampStepAt30And5FPS(t *testing.T) {
	sample := media.Sample{Duration: videoFrameDuration} // 60 ms, the old stamp
	step5 := videoTimestampStep(200*time.Millisecond, sample, 90000)
	if step5 != 18000 {
		t.Fatalf("5 fps step = %d ticks, want 18000 (200 ms at 90 kHz)", step5)
	}
	step30 := videoTimestampStep(33333333*time.Nanosecond, sample, 90000)
	// 33,333,333 ns truncates to 2,999 ticks; one tick off the ideal
	// 3,000 is the integer-truncation cost, shared with Pion's
	// packetizer (same formula), so publisher and receiver agree.
	if step30 < 2999 || step30 > 3000 {
		t.Fatalf("30 fps step = %d ticks, want 2999..3000", step30)
	}
	// The step must NOT follow the sample's 60 ms Duration when the
	// send interval is 200 ms: that was the bug.
	if step5 == sampleRTPTicks(sample, 90000) {
		t.Fatalf("5 fps step still follows the 60 ms sample Duration (%d ticks)", step5)
	}
}

// Job 3.2: the frame-rate flag's interval conversion.
func TestVideoIntervalForFrameRate(t *testing.T) {
	if got, err := videoIntervalForFrameRate(5); err != nil || got != 200*time.Millisecond {
		t.Fatalf("5 fps: interval=%v err=%v, want 200ms", got, err)
	}
	if got, err := videoIntervalForFrameRate(30); err != nil || got != 33333333*time.Nanosecond {
		t.Fatalf("30 fps: interval=%v err=%v, want 33333333ns", got, err)
	}
	for _, bad := range []float64{0, -5, 0.5, 121, math.NaN(), math.Inf(1)} {
		if _, err := videoIntervalForFrameRate(bad); err == nil {
			t.Fatalf("frame rate %v was accepted, want rejection", bad)
		}
	}
}

// Job 3.2: raw H.264 frames carry the declared frame rate's interval,
// and the historical 60 ms stamp is unchanged for the old reader.
func TestReadH264FramesWithFrameRate(t *testing.T) {
	fixture, err := base64.StdEncoding.DecodeString(testH264AccessUnit)
	if err != nil {
		t.Fatal(err)
	}
	frames5, err := readH264FramesWithFrameRate(bytes.NewReader(fixture), 5)
	if err != nil {
		t.Fatalf("read at 5 fps: %v", err)
	}
	for i, f := range frames5 {
		if f.Duration != 200*time.Millisecond {
			t.Fatalf("frame %d at 5 fps: Duration=%v, want 200ms", i, f.Duration)
		}
	}
	frames30, err := readH264FramesWithFrameRate(bytes.NewReader(fixture), 30)
	if err != nil {
		t.Fatalf("read at 30 fps: %v", err)
	}
	for i, f := range frames30 {
		if f.Duration != 33333333*time.Nanosecond {
			t.Fatalf("frame %d at 30 fps: Duration=%v, want 33333333ns", i, f.Duration)
		}
	}
	if _, err := readH264FramesWithFrameRate(bytes.NewReader(fixture), 0); err == nil {
		t.Fatal("frame rate 0 was accepted, want rejection")
	}
	framesOld, err := readH264Frames(bytes.NewReader(fixture))
	if err != nil {
		t.Fatalf("legacy read: %v", err)
	}
	for i, f := range framesOld {
		if f.Duration != videoFrameDuration {
			t.Fatalf("legacy frame %d: Duration=%v, want %v", i, f.Duration, videoFrameDuration)
		}
	}
}

// Job 3.2: the harness refuses a raw H.264 file without a frame
// rate, accepts a valid one, and the proof builders follow it.
func TestResolveVideoFrameRateRefusesWithoutFlag(t *testing.T) {
	if _, err := resolveVideoFrameRate("clip.h264", 0); err == nil {
		t.Fatal("no frame rate was accepted, want refusal")
	}
	if _, err := resolveVideoFrameRate("clip.h264", -3); err == nil {
		t.Fatal("negative frame rate was accepted, want refusal")
	}
	interval, err := resolveVideoFrameRate("clip.h264", 5)
	if err != nil || interval != 200*time.Millisecond {
		t.Fatalf("5 fps: interval=%v err=%v, want 200ms", interval, err)
	}
	if interval, err := resolveVideoFrameRate("", 0); err != nil || interval != 0 {
		t.Fatalf("no video file: interval=%v err=%v, want 0/nil", interval, err)
	}
}

func TestProofVideoIntervalFollowsFrameRate(t *testing.T) {
	old := configuredVideoInterval
	defer func() { configuredVideoInterval = old }()
	configuredVideoInterval = 0
	config, err := buildSFUScaleProofConfig(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if config.videoInterval != 200*time.Millisecond {
		t.Fatalf("unset flag: videoInterval=%v, want 200ms", config.videoInterval)
	}
	configuredVideoInterval = 33333333 * time.Nanosecond
	config, err = buildSFUScaleProofConfig(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if config.videoInterval != 33333333*time.Nanosecond {
		t.Fatalf("30 fps flag: videoInterval=%v, want 33333333ns", config.videoInterval)
	}
}

// Job 3.4: the 32-bit RTP timestamp wraps. Stepping 18,000 ticks from
// 0xFFFFF000 must wrap through zero with the serial difference staying
// +18,000 at every step (int32 serial arithmetic, RFC 3550).
func TestRTPTimestampWraparound(t *testing.T) {
	step := uint32(18000) // variable, not const: the wrap arithmetic must not be constant-folded
	ts := uint32(0xFFFFF000)
	seen := []uint32{ts}
	for i := 0; i < 5; i++ {
		ts += step // uint32 wrap is defined behaviour
		seen = append(seen, ts)
	}
	if seen[1] != uint32(0xFFFFF000)+uint32(step) {
		t.Fatalf("first step: got %d want %d", seen[1], uint32(0xFFFFF000)+uint32(step))
	}
	// 0xFFFFF000 + 2*18000 wraps: 0xFFFFF000 is 2^32-4096, so
	// +36000 lands at 36000-4096 = 31904 = 0x7CA0.
	if seen[2] != 0x7CA0 {
		t.Fatalf("wrap step: got %#x want 0x7ca0", seen[2])
	}
	for i := 1; i < len(seen); i++ {
		if diff := int32(seen[i] - seen[i-1]); diff != int32(step) {
			t.Fatalf("step %d: serial difference %d, want %d", i, diff, step)
		}
	}
	// The step helper agrees with the wrap: a 200 ms interval at
	// 90 kHz is 18,000 ticks even at the wrap boundary.
	if got := rtpTicksForDuration(200*time.Millisecond, 90000); got != step {
		t.Fatalf("interval ticks at wrap = %d, want %d", got, step)
	}
}

// Job 3.6: a synthetic source with KNOWN jitter. Arrivals are late
// by 20 ms in pairs (frames i%4 in {1,2} arrive 20 ms late, the rest
// on time) against a 5 fps timestamp clock, so the transit is
// 0,1800,1800,0,0,1800,1800,0,... ticks and |d| alternates
// 1800,0,1800,0,... The RFC 3550 estimator converges to the analytic
// fixed point of that alternation: J = 870.97 ticks (solve
// J = 15(15J+1800)/256). After 200 packets the measured value must be
// within 10 ticks of it; a perfectly on-time control stream must stay
// near zero.
func TestRTPJitterAgainstKnownClock(t *testing.T) {
	est, err := newRTPJitterEstimator(90000)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Unix(1_000_000, 0)
	const frameTicks = 18000 // 5 fps at 90 kHz
	// 201 packets = 200 updates = 100 complete (1800, 0) pairs, so
	// the estimate lands on the pair fixed point; with 200 packets
	// the last update would be a lone d=1800 and read ~929 instead.
	for i := 0; i < 201; i++ {
		arrival := base.Add(time.Duration(i) * 200 * time.Millisecond)
		if i%4 == 1 || i%4 == 2 {
			arrival = arrival.Add(20 * time.Millisecond)
		}
		est.observe(arrival, uint32(i*frameTicks))
	}
	// Analytic fixed point: 27000/31 = 870.97 ticks.
	const want = 27000.0 / 31.0
	if got := est.jitterTicks(); math.Abs(got-want) > 10 {
		t.Fatalf("alternating-late jitter = %.1f ticks, want %.1f +/- 10", got, want)
	}
	if got := est.jitterDuration(); got < 9*time.Millisecond || got > 11*time.Millisecond {
		t.Fatalf("alternating-late jitter = %v, want ~9.7ms", got)
	}
	if est.packetsSeen() != 201 {
		t.Fatalf("packets seen = %d, want 201", est.packetsSeen())
	}

	control, err := newRTPJitterEstimator(90000)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		control.observe(base.Add(time.Duration(i)*200*time.Millisecond), uint32(i*frameTicks))
	}
	if got := control.jitterTicks(); got != 0 {
		t.Fatalf("on-time control jitter = %.1f ticks, want 0", got)
	}
	if _, err := newRTPJitterEstimator(0); err == nil {
		t.Fatal("zero clock rate was accepted, want rejection")
	}
}

// Job 3.3: one fixed 5 fps run, before and after job 3.1. The run is
// in-process (no UDP: the publisher track is unbound, so WriteSample
// is a no-op and the FrameClock records exactly what the publisher
// stamped). It reports the timestamp step per frame, the send-time
// jitter, and the jitter a receiver would see trusting the RTP
// timestamps. Before the fix the step is 5,400 ticks and the apparent
// receiver jitter is ~140 ms; after, the step is 18,000 and the
// apparent jitter is scheduling noise only.
//
// Raw output: JSON at $HAMI_TIMING_RUN_OUTPUT when set (label from
// $HAMI_TIMING_RUN_LABEL), plus t.Log lines. The same test runs in
// the pristine tree (label "before") and the fixed tree ("after").
type timingRunRecord struct {
	Label             string  `json:"label"`
	FramesPerSecond   float64 `json:"frames_per_second"`
	SendIntervalMs    float64 `json:"send_interval_ms"`
	FramesSent        int     `json:"frames_sent"`
	TimestampStepMean float64 `json:"timestamp_step_mean_ticks"`
	TimestampStepMin  int32   `json:"timestamp_step_min_ticks"`
	TimestampStepMax  int32   `json:"timestamp_step_max_ticks"`
	SendGapMeanMs     float64 `json:"send_gap_mean_ms"`
	SendGapStddevMs   float64 `json:"send_gap_stddev_ms"`
	ApparentJitterMs  float64 `json:"apparent_receiver_jitter_ms"`
}

func TestTimingRun5FPS(t *testing.T) {
	fixture, err := base64.StdEncoding.DecodeString(testH264AccessUnit)
	if err != nil {
		t.Fatal(err)
	}
	frames, err := readH264Frames(bytes.NewReader(fixture))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000},
		"timing-track", "timing-stream",
	)
	if err != nil {
		t.Fatalf("new track: %v", err)
	}
	clock := NewFrameClock()
	sendStats := &m7hPublisherSendStats{publisherID: "timing-publisher"}
	sources := []sfuMediaSource{{
		key:   sfuTrackKey{PublisherID: "timing-publisher", TrackID: "timing-track"},
		track: track,
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 2200*time.Millisecond)
	defer cancel()
	sent, err := sendPublisherVideo(ctx, sources, frames, clock, 1_000_000, 0, 200*time.Millisecond, sendStats, 0)
	if err != nil {
		t.Fatalf("sendPublisherVideo: %v", err)
	}
	if sent < 10 {
		t.Fatalf("frames sent = %d, want >= 10 in 2.2 s at 5 fps", sent)
	}

	// FrameClock captures are keyed by the stamped RTP timestamp.
	clock.mu.Lock()
	stamped := make([]struct {
		ts uint32
		at time.Time
	}, 0, len(clock.captures))
	for id, at := range clock.captures {
		stamped = append(stamped, struct {
			ts uint32
			at time.Time
		}{uint32(id), at})
	}
	clock.mu.Unlock()
	sort.Slice(stamped, func(i, j int) bool { return stamped[i].at.Before(stamped[j].at) })
	if len(stamped) != int(sent) {
		t.Fatalf("clock captures = %d, want %d (one per sent frame)", len(stamped), sent)
	}
	steps := make([]int32, 0, len(stamped)-1)
	gaps := make([]float64, 0, len(stamped)-1)
	for i := 1; i < len(stamped); i++ {
		steps = append(steps, int32(stamped[i].ts-stamped[i-1].ts))
		gaps = append(gaps, float64(stamped[i].at.Sub(stamped[i-1].at))/float64(time.Millisecond))
	}
	var stepSum int64
	stepMin, stepMax := steps[0], steps[0]
	for _, s := range steps {
		stepSum += int64(s)
		if s < stepMin {
			stepMin = s
		}
		if s > stepMax {
			stepMax = s
		}
	}
	var gapSum float64
	for _, g := range gaps {
		gapSum += g
	}
	gapMean := gapSum / float64(len(gaps))
	var gapVar float64
	for _, g := range gaps {
		gapVar += (g - gapMean) * (g - gapMean)
	}
	gapStddev := math.Sqrt(gapVar / float64(len(gaps)))

	est, err := newRTPJitterEstimator(90000)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range stamped {
		est.observe(s.at, s.ts)
	}

	record := timingRunRecord{
		Label:             os.Getenv("HAMI_TIMING_RUN_LABEL"),
		FramesPerSecond:   5,
		SendIntervalMs:    200,
		FramesSent:        int(sent),
		TimestampStepMean: float64(stepSum) / float64(len(steps)),
		TimestampStepMin:  stepMin,
		TimestampStepMax:  stepMax,
		SendGapMeanMs:     gapMean,
		SendGapStddevMs:   gapStddev,
		ApparentJitterMs:  float64(est.jitterDuration()) / float64(time.Millisecond),
	}
	if out := os.Getenv("HAMI_TIMING_RUN_OUTPUT"); out != "" {
		raw, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			t.Fatalf("encode timing record: %v", err)
		}
		if err := os.WriteFile(out, append(raw, '\n'), 0o600); err != nil {
			t.Fatalf("write timing record: %v", err)
		}
	}
	t.Logf("timing run 5fps label=%q frames=%d step mean=%.1f min=%d max=%d gap mean=%.2fms stddev=%.2fms apparent jitter=%.2fms",
		record.Label, record.FramesSent, record.TimestampStepMean, record.TimestampStepMin,
		record.TimestampStepMax, record.SendGapMeanMs, record.SendGapStddevMs, record.ApparentJitterMs)

	// After the fix every step is exactly 18,000 ticks and the
	// apparent receiver jitter is scheduling noise (< 10 ms).
	for i, s := range steps {
		if s != 18000 {
			t.Fatalf("timestamp step %d = %d ticks, want 18000 (200 ms at 90 kHz)", i, s)
		}
	}
	if record.ApparentJitterMs >= 10 {
		t.Fatalf("apparent receiver jitter = %.2f ms, want < 10 ms", record.ApparentJitterMs)
	}
}
