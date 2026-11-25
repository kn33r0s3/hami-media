// Cluster D job 13.4: glass-to-glass and start-time measurement
// for WebRTC and HLS, with the method stated.
//
// Method (see docs/cluster-d/13-latency-method.md):
//   - WebRTC glass-to-glass: a synthetic frame is timestamped at
//     capture (t0, monotonic clock on the publisher side) and again
//     at render (t1, same clock in the in-process test, so no clock
//     sync error). Glass-to-glass = t1 - t0. In a real two-machine
//     run the clocks must be synchronised (NTP/PTP) and the offset
//     recorded; the VM run is single-clock and labelled so.
//   - WebRTC start time: join request -> first rendered frame.
//   - HLS start time: playlist request -> first segment's first
//     frame available to the player. HLS glass-to-glass adds the
//     segment duration + playlist polling interval (inherent to HLS:
//     a segment must be complete before it can play).
//   - HLS segment latency: time from a frame entering the packager
//     to that frame's segment being listed in the playlist.

package main

import (
	"errors"
	"fmt"
	"time"
)

// clusterDLatencySample is one measured latency sample.
type clusterDLatencySample struct {
	Kind      string // "webrtc-glass-to-glass", "webrtc-start", "hls-start", "hls-segment"
	Duration  time.Duration
	At        time.Time
	ClockNote string // "single-clock" (VM) or "ntp-synced" (two-machine)
}

// clusterDLatencyReport aggregates samples for one kind.
type clusterDLatencyReport struct {
	Kind    string
	Samples []time.Duration
	Min     time.Duration
	P50     time.Duration
	P99     time.Duration
	Max     time.Duration
}

func summarizeLatency(kind string, samples []time.Duration, clockNote string) (clusterDLatencyReport, error) {
	if len(samples) == 0 {
		return clusterDLatencyReport{}, errors.New("latency: no samples")
	}
	sorted := append([]time.Duration(nil), samples...)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	pct := func(p float64) time.Duration {
		idx := int(p * float64(len(sorted)-1))
		return sorted[idx]
	}
	_ = clockNote
	return clusterDLatencyReport{
		Kind: kind, Samples: samples,
		Min: sorted[0], P50: pct(0.50), P99: pct(0.99), Max: sorted[len(sorted)-1],
	}, nil
}

// clusterDGlassToGlass measures a WebRTC-style path in-process:
// capture -> forward -> render, all on one monotonic clock.
type clusterDGlassToGlass struct {
	muSamples []clusterDLatencySample
}

// measureFrame records one frame's capture-to-render latency.
func (g *clusterDGlassToGlass) measureFrame(capture, render time.Time) clusterDLatencySample {
	s := clusterDLatencySample{Kind: "webrtc-glass-to-glass", Duration: render.Sub(capture), At: render, ClockNote: "single-clock"}
	g.muSamples = append(g.muSamples, s)
	return s
}

// hlsStartTime computes HLS start time from its parts: playlist
// fetch + first segment download + segment duration already buffered
// (a player needs one full segment before first frame).
func hlsStartTime(playlistFetch, segmentDownload, segmentDuration time.Duration) time.Duration {
	return playlistFetch + segmentDownload + segmentDuration
}

// hlsSegmentLatency is packager entry -> segment listed in playlist.
func hlsSegmentLatency(packagerEntry, playlistListed time.Time) time.Duration {
	return playlistListed.Sub(packagerEntry)
}

func formatLatencyReport(r clusterDLatencyReport) string {
	return fmt.Sprintf("%s: n=%d min=%s p50=%s p99=%s max=%s",
		r.Kind, len(r.Samples), r.Min, r.P50, r.P99, r.Max)
}
