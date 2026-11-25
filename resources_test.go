package main

import (
	"runtime"
	"testing"
	"time"
)

// TestSampleRuntimeResourcesParkedGoroutines (5.7): the sampler's
// empirical per-goroutine stack — 1000 parked goroutines, StackInuse
// delta divided by the goroutine delta. Go stacks start at 2 KiB, so
// the per-goroutine figure must be in the low-KiB range; the exact
// number is what reconciles the ladder figures (75,184 goroutines,
// 32 MB stacks). Also asserts RSS reads (Linux /proc/self/statm) and
// no goroutine leak after unparking.
func TestSampleRuntimeResourcesParkedGoroutines(t *testing.T) {
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	before := sampleRuntimeResources()
	if before.RSSError != "" {
		t.Fatalf("baseline sample RSS error: %s", before.RSSError)
	}
	if before.RSSBytes == 0 {
		t.Fatal("baseline sample RSS is 0")
	}
	const parked = 1000
	release := make(chan struct{})
	for i := 0; i < parked; i++ {
		go func() {
			<-release
		}()
	}
	// Let the goroutines start and park; a parked goroutine's stack
	// is at its minimum (shallow frames), so this measures the
	// per-goroutine stack floor.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got := runtime.NumGoroutine(); got >= before.Goroutines+parked {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutines = %d, want >= %d within 5s", runtime.NumGoroutine(), before.Goroutines+parked)
		}
		time.Sleep(10 * time.Millisecond)
	}
	during := sampleRuntimeResources()
	goroutineDelta := during.Goroutines - before.Goroutines
	stackDelta := int64(during.StackInuse) - int64(before.StackInuse)
	rssDelta := int64(during.RSSBytes) - int64(before.RSSBytes)
	perGoroutine := float64(stackDelta) / float64(goroutineDelta)
	t.Logf("parked goroutines: count delta=%d StackInuse delta=%d bytes (%.0f B/goroutine) RSS delta=%d bytes StackSys=%d",
		goroutineDelta, stackDelta, perGoroutine, rssDelta, during.StackSys)
	if goroutineDelta < parked {
		t.Fatalf("goroutine delta = %d, want >= %d", goroutineDelta, parked)
	}
	if perGoroutine < 1024 || perGoroutine > 8192 {
		t.Fatalf("per-goroutine StackInuse = %.0f B, want in [1024, 8192] (Go stacks start at 2 KiB)", perGoroutine)
	}
	if during.StackSys < during.StackInuse {
		t.Fatalf("StackSys=%d < StackInuse=%d", during.StackSys, during.StackInuse)
	}
	close(release)
	deadline = time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		time.Sleep(100 * time.Millisecond)
		if got := runtime.NumGoroutine(); got <= before.Goroutines+2 {
			t.Logf("goroutines after release: %d (baseline %d)", got, before.Goroutines)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutines after release = %d, want <= %d within 5s (leak?)",
				runtime.NumGoroutine(), before.Goroutines+2)
		}
	}
}
