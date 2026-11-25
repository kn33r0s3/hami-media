// Cluster C §8 — input limits (8.4) and rate limits (8.5).
//
// 8.4 limits: SDP size, candidate count, tracks per participant,
// rooms per node. Each has an exceed-the-limit test in
// cluster_c_section8_test.go. The 4 MB body cap already in
// decodeM7HJSON (main.go:7755) is the transport cap; these are the
// per-field caps on top of it.
//
// 8.5 rate limits: token-bucket per IP and per account for joins and
// signaling. Limits are stated here so a test can exceed them.
package main

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// --- 8.4 input limits ---------------------------------------------------------

const (
	maxSDPBytes             = 64 << 10 // 64 KiB SDP — an SDP is text; larger is abuse
	maxICECandidates        = 64       // per participant
	maxTracksPerParticipant = 8
	maxRoomsPerNode         = 1024
	maxSignalingJSONBytes   = 64 << 10
)

// validateSDPSize rejects an SDP over the limit.
func validateSDPSize(sdp string) error {
	if len(sdp) > maxSDPBytes {
		return fmt.Errorf("SDP size %d bytes exceeds limit %d", len(sdp), maxSDPBytes)
	}
	if len(sdp) == 0 {
		return errors.New("SDP is empty")
	}
	return nil
}

// validateCandidateCount rejects more candidates than the limit.
func validateCandidateCount(n int) error {
	if n < 0 {
		return errors.New("candidate count is negative")
	}
	if n > maxICECandidates {
		return fmt.Errorf("candidate count %d exceeds limit %d", n, maxICECandidates)
	}
	return nil
}

// validateTracksPerParticipant rejects too many tracks for one participant.
func validateTracksPerParticipant(n int) error {
	if n < 0 {
		return errors.New("track count is negative")
	}
	if n > maxTracksPerParticipant {
		return fmt.Errorf("tracks per participant %d exceeds limit %d", n, maxTracksPerParticipant)
	}
	return nil
}

// validateRoomsPerNode rejects creating a room past the node limit.
func validateRoomsPerNode(current int) error {
	if current < 0 {
		return errors.New("room count is negative")
	}
	if current >= maxRoomsPerNode {
		return fmt.Errorf("rooms per node %d exceeds limit %d", current, maxRoomsPerNode)
	}
	return nil
}

// --- 8.5 rate limits ----------------------------------------------------------

// Rate limits (stated, tested): joins 10/min per IP and 5/min per
// account; signaling messages 60/min per IP and 30/min per account.
// A burst of 5 joins is allowed; the 6th within the minute is not.
const (
	joinLimitPerIPPerMin        = 10
	joinLimitPerAccountPerMin   = 5
	signalLimitPerIPPerMin      = 60
	signalLimitPerAccountPerMin = 30
)

// rateLimiter is a fixed-window counter per key. It is deliberately
// simple (no leaky bucket): the window and counts are inspectable in
// tests, and the behaviour at the boundary is exact.
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	counts  map[string]int
	started map[string]time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, counts: map[string]int{}, started: map[string]time.Time{}}
}

// allow reports whether one more event for key is within the limit.
func (l *rateLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	start, ok := l.started[key]
	if !ok || now.Sub(start) >= l.window {
		l.started[key] = now
		l.counts[key] = 1
		return true
	}
	if l.counts[key] >= l.limit {
		return false
	}
	l.counts[key]++
	return true
}

// joinRateLimits bundles the four §8.5 limiters.
type joinRateLimits struct {
	joinPerIP        *rateLimiter
	joinPerAccount   *rateLimiter
	signalPerIP      *rateLimiter
	signalPerAccount *rateLimiter
}

func newJoinRateLimits() *joinRateLimits {
	return &joinRateLimits{
		joinPerIP:        newRateLimiter(joinLimitPerIPPerMin, time.Minute),
		joinPerAccount:   newRateLimiter(joinLimitPerAccountPerMin, time.Minute),
		signalPerIP:      newRateLimiter(signalLimitPerIPPerMin, time.Minute),
		signalPerAccount: newRateLimiter(signalLimitPerAccountPerMin, time.Minute),
	}
}

// allowJoin checks both the per-IP and per-account join limits.
func (l *joinRateLimits) allowJoin(ip, account string, now time.Time) error {
	if !l.joinPerIP.allow("ip:"+ip, now) {
		return fmt.Errorf("join rate limit exceeded for IP (limit %d/min)", joinLimitPerIPPerMin)
	}
	if !l.joinPerAccount.allow("account:"+account, now) {
		return fmt.Errorf("join rate limit exceeded for account (limit %d/min)", joinLimitPerAccountPerMin)
	}
	return nil
}

// allowSignal checks both the per-IP and per-account signaling limits.
func (l *joinRateLimits) allowSignal(ip, account string, now time.Time) error {
	if !l.signalPerIP.allow("ip:"+ip, now) {
		return fmt.Errorf("signaling rate limit exceeded for IP (limit %d/min)", signalLimitPerIPPerMin)
	}
	if !l.signalPerAccount.allow("account:"+account, now) {
		return fmt.Errorf("signaling rate limit exceeded for account (limit %d/min)", signalLimitPerAccountPerMin)
	}
	return nil
}
