package main

import (
	"sort"
	"time"
)

// stallGateP99MedianMax is the stall gate (Job 5.6, parent-set 25 Nov
// 2026): FAIL the run if the median over viewers of each viewer's p99
// latency exceeds 50 ms. The p50 gate is dead — control A left the
// p50 median at 4.76 ms (a PASS) while the p99 median blew up to
// 104.91 ms, so the p50 median is insensitive to this failure mode.
// The strict gate (p99 <= 150 ms per viewer, zero lost frames) still
// applies alongside it. 50 ms sits in the clean gap: 1.64x above the
// highest passing p99 median on record (30.45 ms, T3) and 2.1x below
// the lowest failing one (104.91 ms, control A).
const stallGateP99MedianMax = 50 * time.Millisecond

// evaluateStallGate reports whether a run passes the stall gate for
// the given viewer-side p99 median.
func evaluateStallGate(p99Median time.Duration) bool {
	return p99Median <= stallGateP99MedianMax
}

// stallGateP99Median is the median of per-viewer p99 latencies.
// Empty input returns 0. An even count averages the two middle
// values (the same median the saved Mac JSONs were recomputed with).
func stallGateP99Median(p99s []time.Duration) time.Duration {
	if len(p99s) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(p99s))
	copy(sorted, p99s)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}

// sfuProofP99Median extracts the per-viewer p99s from a proof result
// and returns their median. Viewers without latency are skipped (the
// per-viewer checks in the verdict functions already fail the run
// for a missing latency, so the gate cannot mask that).
func sfuProofP99Median(result sfuProofResult) time.Duration {
	p99s := make([]time.Duration, 0, len(result.Viewers))
	for _, viewer := range result.Viewers {
		if viewer.Latency != nil {
			p99s = append(p99s, viewer.Latency.P99)
		}
	}
	return stallGateP99Median(p99s)
}
