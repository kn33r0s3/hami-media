// Hami media timing helpers (jobs 3.1, 3.2, 3.6).
//
// The RTP timestamp step must follow the SEND interval, not the
// sample's stored Duration: the scale harness sends video every
// 200 ms (5 fps) while readH264Frames stamped every sample with
// videoFrameDuration (60 ms, 5,400 ticks at 90 kHz), so a receiver
// trusting the timestamps saw 140 ms of false jitter per frame.
// videoTimestampStep derives the step from the send interval; the
// frame-rate flag (3.2) makes the stored Duration agree with it.
package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
)

// videoTimestampStep returns the RTP timestamp advance for one video
// frame sent on a ticker of videoInterval at clockRate. The step comes
// from the send interval: that is the spacing a receiver observes.
// A non-positive interval falls back to the sample's Duration (the
// pre-fix behaviour) so callers that do not pass an interval keep
// working; new callers must pass the real send interval.
func videoTimestampStep(videoInterval time.Duration, sample media.Sample, clockRate uint32) uint32 {
	if videoInterval > 0 {
		return rtpTicksForDuration(videoInterval, clockRate)
	}
	return sampleRTPTicks(sample, clockRate)
}

// rtpTicksForDuration converts a wall-clock duration to RTP timestamp
// ticks at clockRate, truncating like sampleRTPTicks does. Keeping the
// one conversion in one place is what keeps the publisher's own clock
// (FrameClock, stamped with these ticks) in agreement with Pion's
// packetizer, which advances by the same integer truncation of
// sample.Duration.
func rtpTicksForDuration(d time.Duration, clockRate uint32) uint32 {
	if d <= 0 || clockRate == 0 {
		return 0
	}
	return uint32(int64(d) * int64(clockRate) / int64(time.Second))
}

// videoIntervalForFrameRate returns the send interval for a frame rate
// in frames per second. Valid rates are 1..120 fps; anything else is
// a configuration error, not a silent default.
func videoIntervalForFrameRate(fps float64) (time.Duration, error) {
	if math.IsNaN(fps) || math.IsInf(fps, 0) || fps < 1 || fps > 120 {
		return 0, fmt.Errorf("frame rate %v fps is outside 1..120 fps", fps)
	}
	interval := time.Duration(math.Round(float64(time.Second) / fps))
	if interval <= 0 {
		return 0, fmt.Errorf("frame rate %v fps gives a non-positive send interval", fps)
	}
	return interval, nil
}

// readH264FramesWithFrameRate parses a raw Annex-B H.264 stream and
// stamps every frame with the send interval for fps (job 3.2). The
// frame Duration is what Pion's packetizer turns into the RTP
// timestamp step, so it must equal the rate the frames are actually
// sent at; a file read without a frame rate cannot know that, which
// is why the harness refuses to run without -frame-rate.
func readH264FramesWithFrameRate(source io.Reader, fps float64) ([]media.Sample, error) {
	interval, err := videoIntervalForFrameRate(fps)
	if err != nil {
		return nil, err
	}
	return readH264FramesWithDuration(source, interval)
}

// readH264FramesWithDuration is readH264Frames with the frame Duration
// supplied by the caller. readH264Frames keeps the historical 60 ms
// stamp (videoFrameDuration) for existing in-process callers.
func readH264FramesWithDuration(source io.Reader, frameDuration time.Duration) ([]media.Sample, error) {
	if frameDuration <= 0 {
		return nil, errors.New("H.264 frame duration must be positive")
	}
	return readH264NALs(source, frameDuration)
}

// proofVideoInterval returns the send interval for scale-proof
// configs: the -frame-rate interval when main() set one, else the
// historical 200 ms (5 fps) the ladder was measured at.
func proofVideoInterval() time.Duration {
	if configuredVideoInterval > 0 {
		return configuredVideoInterval
	}
	return 200 * time.Millisecond
}

// resolveVideoFrameRate validates the -frame-rate flag for a run that
// reads a raw H.264 file and returns the send interval. It refuses
// (error) when no frame rate was given: running without one would
// silently stamp the wrong timestamp step (job 3.1).
func resolveVideoFrameRate(videoPath string, fps float64) (time.Duration, error) {
	if videoPath == "" {
		return 0, nil
	}
	if fps <= 0 {
		return 0, errors.New("--frame-rate is required with --video-file: a raw H.264 stream carries no frame rate (1-120 fps)")
	}
	return videoIntervalForFrameRate(fps)
}

// for consecutive packets, transit = arrival - RTP timestamp (both in
// timestamp units); d = |transit - previous transit|; the estimate
// moves 1/16th of the way toward d each packet. It is clocked by the
// caller's arrival times, so a synthetic source with a known clock
// (job 3.6) gives a known expected value.
type rtpJitterEstimator struct {
	clockRate   uint32
	prevTransit int64
	havePrev    bool
	jitter      float64 // in RTP timestamp ticks
	packets     uint64
}

// newRTPJitterEstimator returns an estimator for clockRate (e.g. 90000
// for video). A zero clock rate is rejected: jitter in ticks is
// meaningless without it.
func newRTPJitterEstimator(clockRate uint32) (*rtpJitterEstimator, error) {
	if clockRate == 0 {
		return nil, errors.New("RTP jitter estimator needs a non-zero clock rate")
	}
	return &rtpJitterEstimator{clockRate: clockRate}, nil
}

// observe feeds one packet: arrival is the wall-clock receive instant
// and rtpTimestamp is the packet's RTP timestamp. The first packet
// only establishes the transit baseline.
func (e *rtpJitterEstimator) observe(arrival time.Time, rtpTimestamp uint32) {
	// Transit in timestamp ticks: arrival expressed in the RTP clock.
	arrivalTicks := arrival.UnixNano() * int64(e.clockRate) / int64(time.Second)
	transit := arrivalTicks - int64(rtpTimestamp)
	if !e.havePrev {
		e.prevTransit = transit
		e.havePrev = true
		e.packets++
		return
	}
	d := transit - e.prevTransit
	if d < 0 {
		d = -d
	}
	e.jitter += (float64(d) - e.jitter) / 16
	e.prevTransit = transit
	e.packets++
}

// jitterTicks returns the current estimate in RTP timestamp ticks.
func (e *rtpJitterEstimator) jitterTicks() float64 {
	return e.jitter
}

// jitterDuration returns the current estimate as a wall-clock duration.
func (e *rtpJitterEstimator) jitterDuration() time.Duration {
	if e.clockRate == 0 {
		return 0
	}
	return time.Duration(e.jitter * float64(time.Second) / float64(e.clockRate))
}

// packetsSeen returns how many packets fed the estimator.
func (e *rtpJitterEstimator) packetsSeen() uint64 {
	return e.packets
}
