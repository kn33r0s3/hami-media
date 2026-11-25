package main

import (
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

// TestParseNACKProfile (4.5): the HAMI_SFU_NACK_PROFILE env value
// parses to a profile; empty means the broadcast default; anything
// else is an error.
func TestParseNACKProfile(t *testing.T) {
	for _, value := range []string{"interactive", "broadcast", "recording"} {
		profile, err := parseNACKProfile(value)
		if err != nil {
			t.Fatalf("parseNACKProfile(%q): %v", value, err)
		}
		if string(profile) != value {
			t.Fatalf("parseNACKProfile(%q) = %q", value, profile)
		}
	}
	profile, err := parseNACKProfile("")
	if err != nil {
		t.Fatalf("parseNACKProfile(\"\"): %v", err)
	}
	if profile != nackProfileBroadcast {
		t.Fatalf("parseNACKProfile(\"\") = %q, want broadcast", profile)
	}
	if _, err := parseNACKProfile("bogus"); err == nil {
		t.Fatal("parseNACKProfile(\"bogus\") must fail")
	}
}

// TestNACKProfileFeedbackMaxAge (4.5): each profile sets its NACK
// feedback age — interactive 100 ms, broadcast 500 ms, recording
// 2000 ms — and the zero value is broadcast.
func TestNACKProfileFeedbackMaxAge(t *testing.T) {
	cases := []struct {
		profile sfuNACKProfile
		want    time.Duration
	}{
		{nackProfileInteractive, 100 * time.Millisecond},
		{nackProfileBroadcast, 500 * time.Millisecond},
		{nackProfileRecording, 2000 * time.Millisecond},
		{"", 500 * time.Millisecond},
	}
	for _, tc := range cases {
		if got := tc.profile.feedbackMaxAge(); got != tc.want {
			t.Fatalf("profile %q feedbackMaxAge = %v, want %v", tc.profile, got, tc.want)
		}
	}
}

// TestNACKProfileEnvWiring (4.5): newSFURouter reads
// HAMI_SFU_NACK_PROFILE into the router default and rejects an
// unknown profile.
func TestNACKProfileEnvWiring(t *testing.T) {
	api := webrtc.NewAPI()
	router, err := newSFURouter(api)
	if err != nil {
		t.Fatalf("newSFURouter without env: %v", err)
	}
	if router.nackProfile != nackProfileBroadcast {
		t.Fatalf("router default nackProfile = %q, want broadcast", router.nackProfile)
	}
	t.Setenv("HAMI_SFU_NACK_PROFILE", "interactive")
	router, err = newSFURouter(api)
	if err != nil {
		t.Fatalf("newSFURouter with interactive: %v", err)
	}
	if router.nackProfile != nackProfileInteractive {
		t.Fatalf("router nackProfile = %q, want interactive", router.nackProfile)
	}
	t.Setenv("HAMI_SFU_NACK_PROFILE", "bogus")
	if _, err := newSFURouter(api); err == nil {
		t.Fatal("newSFURouter with bogus profile must fail")
	}
}

// waitNACKDroppedStale polls the subscription's stale counter until it
// reaches want (or the test times out).
func waitNACKDroppedStale(t *testing.T, subscription *sfuSubscription, want uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := subscription.feedbackSnapshot().NACKDroppedStale; got >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("NACKDroppedStale = %d, want %d within 5s",
				subscription.feedbackSnapshot().NACKDroppedStale, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestNACKProfileStaleBoundary (4.5): the nackWorker stale check is
// the subscription's profile age — for EACH profile, a NACK aged
// just inside the window is retransmitted and one aged just past it
// is dropped as stale. Router-level: the NACK goes through the real
// nackWorker and handleNACK, no UDP.
func TestNACKProfileStaleBoundary(t *testing.T) {
	profiles := []struct {
		profile sfuNACKProfile
		age     time.Duration
	}{
		{nackProfileInteractive, 100 * time.Millisecond},
		{nackProfileBroadcast, 500 * time.Millisecond},
		{nackProfileRecording, 2000 * time.Millisecond},
	}
	for _, tc := range profiles {
		t.Run(string(tc.profile), func(t *testing.T) {
			// Fresh: aged age-1ms, must retransmit, not drop stale.
			_, subscription, viewerSSRC, cleanup := newFaultInjectionRouter(t, nil)
			defer cleanup()
			subscription.nackProfile = tc.profile
			faultInjectionStream(subscription.source)
			now := time.Now()
			subscription.nackQueue <- queuedNACK{
				feedback: &rtcp.TransportLayerNack{
					MediaSSRC: viewerSSRC,
					Nacks:     rtcp.NackPairsFromSequenceNumbers([]uint16{1007}),
				},
				enqueuedAt: now.Add(-tc.age + time.Millisecond),
			}
			waitFaultInjectionWrites(t, subscription, 1)
			if got := subscription.feedbackSnapshot().NACKDroppedStale; got != 0 {
				t.Fatalf("%s: fresh NACK dropped stale = %d, want 0", tc.profile, got)
			}

			// Stale: aged age+1ms, must drop stale, never retransmit.
			_, staleSub, staleSSRC, staleCleanup := newFaultInjectionRouter(t, nil)
			defer staleCleanup()
			staleSub.nackProfile = tc.profile
			faultInjectionStream(staleSub.source)
			staleSub.nackQueue <- queuedNACK{
				feedback: &rtcp.TransportLayerNack{
					MediaSSRC: staleSSRC,
					Nacks:     rtcp.NackPairsFromSequenceNumbers([]uint16{1007}),
				},
				enqueuedAt: time.Now().Add(-tc.age - time.Millisecond),
			}
			waitNACKDroppedStale(t, staleSub, 1)
			if got := staleSub.rtpWriteSucceeded.Load(); got != 0 {
				t.Fatalf("%s: stale NACK retransmitted %d writes, want 0", tc.profile, got)
			}
		})
	}
}
