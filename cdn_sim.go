// Cluster D job 13.3: edge-simulator CDN measurement.
//
// A real CDN costs money and needs owner approval ([!] 13.3): the
// real-CDN part is NEEDS-APPROVAL and is NOT started. What is
// measured here is an edge simulator: a local cache in front of the
// packager's segment origin. It reports cache hit ratio and start
// time (time from playlist request to first segment byte served from
// the edge). The same measurement method applies to a real CDN
// later; the numbers here are simulator numbers and are labelled so.

package main

import (
	"fmt"
	"sync"
	"time"
)

// clusterDEdgeSimulator is a single-edge cache in front of an origin.
type clusterDEdgeSimulator struct {
	mu     sync.Mutex
	origin map[string][]byte // path -> bytes
	cache  map[string][]byte
	hits   int64
	misses int64
}

func newClusterDEdgeSimulator() *clusterDEdgeSimulator {
	return &clusterDEdgeSimulator{origin: make(map[string][]byte), cache: make(map[string][]byte)}
}

func (e *clusterDEdgeSimulator) putOrigin(path string, data []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.origin[path] = data
}

// fetch serves path from the edge cache, filling from origin on a miss.
func (e *clusterDEdgeSimulator) fetch(path string) ([]byte, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if data, ok := e.cache[path]; ok {
		e.hits++
		return data, true, nil
	}
	data, ok := e.origin[path]
	if !ok {
		return nil, false, fmt.Errorf("edge simulator: origin has no %s", path)
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	e.cache[path] = cp
	e.misses++
	return cp, false, nil
}

// hitRatio is hits / (hits + misses).
func (e *clusterDEdgeSimulator) hitRatio() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	total := e.hits + e.misses
	if total == 0 {
		return 0
	}
	return float64(e.hits) / float64(total)
}

// clusterDCDNMeasurement is one edge-simulator measurement run.
type clusterDCDNMeasurement struct {
	Requests   int
	Hits       int64
	Misses     int64
	HitRatio   float64
	StartTime  time.Duration // playlist request -> first segment byte
	PlaylistMs int64
}

// measureCDN runs a simulated viewer session: one playlist fetch
// followed by segment fetches, twice (second pass should hit cache).
// startTime is measured from the playlist request to the first
// segment byte on the cold pass.
func (e *clusterDEdgeSimulator) measureCDN(playlistPath string, segmentPaths []string) (clusterDCDNMeasurement, error) {
	start := time.Now()
	if _, _, err := e.fetch(playlistPath); err != nil {
		return clusterDCDNMeasurement{}, err
	}
	playlistMs := time.Since(start).Milliseconds()
	firstSegAt := time.Now()
	for _, seg := range segmentPaths {
		if _, _, err := e.fetch(seg); err != nil {
			return clusterDCDNMeasurement{}, err
		}
	}
	firstSeg := time.Since(firstSegAt)
	// Second pass: everything should be a cache hit.
	for _, seg := range append([]string{playlistPath}, segmentPaths...) {
		if _, _, err := e.fetch(seg); err != nil {
			return clusterDCDNMeasurement{}, err
		}
	}
	e.mu.Lock()
	hits, misses := e.hits, e.misses
	e.mu.Unlock()
	total := hits + misses
	ratio := 0.0
	if total > 0 {
		ratio = float64(hits) / float64(total)
	}
	return clusterDCDNMeasurement{
		Requests:   int(total),
		Hits:       hits,
		Misses:     misses,
		HitRatio:   ratio,
		StartTime:  firstSeg,
		PlaylistMs: playlistMs,
	}, nil
}
