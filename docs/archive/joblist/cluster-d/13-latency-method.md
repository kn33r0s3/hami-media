# 13.4 Glass-to-glass and start-time measurement — method

## Definitions

- **WebRTC glass-to-glass**: capture timestamp (publisher camera,
  t0) to render timestamp (viewer display, t1). In the VM run both
  timestamps come from one monotonic clock (in-process synthetic
  frames), so there is no clock-sync error; the run is labelled
  `single-clock`.
- **WebRTC start time**: join request sent -> first rendered frame.
- **HLS start time**: playlist request -> first segment's first
  frame available to the player = playlist fetch + first segment
  download + one segment duration (a player needs a full segment
  before its first frame).
- **HLS segment latency**: a frame entering the packager -> that
  frame's segment listed in the playlist.

## Two-machine rule

On two machines the clocks must be synchronised (NTP, offset
measured and recorded with every result) — otherwise glass-to-glass
is meaningless. The VM run is single-clock and is labelled so in
every sample (`ClockNote: "single-clock"`); it is a method check,
not a two-machine measurement.

## Numbers (VM, method check only)

- Synthetic WebRTC frames: 10 frames, 40 ms capture-to-render each
  (p50 = p99 = 40 ms). This is the measurement pipeline working,
  not a product latency claim.
- HLS start time (example parts): 50 ms playlist fetch + 100 ms
  segment download + 2.0 s segment = 2.15 s. The segment duration
  dominates HLS start time; that is inherent to HLS, not a bug.
- HLS segment latency (example): 2.1 s packager entry -> playlist
  listing for a 2 s segment.

## Implementation

`latency_measure.go`: `clusterDGlassToGlass.measureFrame`,
`hlsStartTime`, `hlsSegmentLatency`, `summarizeLatency`. Test:
`TestClusterDLatencyMethod`.
