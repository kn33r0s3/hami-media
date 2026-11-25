package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// blockOnMutex contends on mu until stop closes, counting each
// acquisition. Every acquisition after blocking is one mutex-profile
// sample, so this is the contention source for the 5.1 tests.
func blockOnMutex(mu *sync.Mutex, stop <-chan struct{}, acquired *atomic.Uint64, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-stop:
			return
		default:
		}
		mu.Lock()
		acquired.Add(1)
		mu.Unlock()
		runtime.Gosched()
	}
}

// holdMutexContended is the lock holder for the 5.1 tests: it keeps
// the lock most of the time (10 ms holds) and releases briefly so the
// blockers acquire (a sample is recorded on acquisition after
// blocking) and block again. The mutex profile attributes contention
// to the HOLDER's stack — the code holding the lock too long — so
// this function's name is what pprof -top must show.
func holdMutexContended(mu *sync.Mutex, stop <-chan struct{}, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-stop:
			return
		default:
		}
		mu.Lock()
		time.Sleep(10 * time.Millisecond)
		mu.Unlock()
		time.Sleep(time.Millisecond)
	}
}

// TestWriteMutexProfileCapturesContention (5.1): a mutex profile
// taken while goroutines contend on a lock holds samples, writes a
// companion JSON with the window and sample count, and `go tool
// pprof -top` on the test binary attributes the contention to the
// lock HOLDER (the code holding the lock too long — the mechanism 5.1
// relies on to find a contended router lock).
func TestWriteMutexProfileCapturesContention(t *testing.T) {
	previous := runtime.SetMutexProfileFraction(1)
	defer runtime.SetMutexProfileFraction(previous)
	var mu sync.Mutex
	stop := make(chan struct{})
	var acquired atomic.Uint64
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go blockOnMutex(&mu, stop, &acquired, &wg)
	}
	wg.Add(1)
	go holdMutexContended(&mu, stop, &wg)
	path := filepath.Join(t.TempDir(), "mutex.pprof")
	metadata, err := writeMutexProfile(context.Background(), path, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("writeMutexProfile: %v", err)
	}
	close(stop)
	waitDone := make(chan struct{})
	go func() { wg.Wait(); close(waitDone) }()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		t.Fatal("contention goroutines did not stop")
	}
	if got := acquired.Load(); got == 0 {
		t.Fatal("no blocker acquired the contended mutex during the window")
	}
	if metadata.SampleCount == 0 {
		t.Fatalf("mutex profile holds 0 samples despite %d acquisitions", acquired.Load())
	}
	if metadata.Window != 500*time.Millisecond {
		t.Fatalf("metadata window = %v, want 500ms", metadata.Window)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat profile: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("mutex profile is empty")
	}
	encoded, err := os.ReadFile(path + ".json")
	if err != nil {
		t.Fatalf("read companion JSON: %v", err)
	}
	var companion mutexProfileMetadata
	if err := json.Unmarshal(encoded, &companion); err != nil {
		t.Fatalf("parse companion JSON: %v", err)
	}
	if companion.SampleCount != metadata.SampleCount || companion.Window != metadata.Window {
		t.Fatalf("companion JSON = %+v, want sample_count=%d window=%v", companion, metadata.SampleCount, metadata.Window)
	}
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Logf("go tool not on PATH; skipping pprof -top check (profile %s, %d samples)", path, metadata.SampleCount)
		return
	}
	top, err := exec.Command(goBinary, "tool", "pprof", "-top", os.Args[0], path).CombinedOutput()
	if err != nil {
		t.Fatalf("go tool pprof -top: %v\n%s", err, top)
	}
	t.Logf("mutex profile top:\n%s", top)
	if !strings.Contains(string(top), "holdMutexContended") {
		t.Fatalf("pprof -top does not attribute contention to the lock holder holdMutexContended:\n%s", top)
	}
	// The router lock is not contended in this synthetic test — the
	// verdict on it needs a Mac media-window run (see the 5.1 note).
	if strings.Contains(string(top), "sfuRouter") || strings.Contains(string(top), "sfuSubscription") {
		t.Logf("note: router lock appears in the synthetic profile top (unexpected, see note)")
	}
}

// TestMutexProfileEnvWiring (5.1): startHeapBudgetMonitor reads
// HAMI_MUTEX_PROFILE_PATH / DELAY / WINDOW and writes a per-pid
// profile plus its companion JSON.
func TestMutexProfileEnvWiring(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mutex.pprof")
	t.Setenv("HAMI_MUTEX_PROFILE_PATH", path)
	t.Setenv("HAMI_MUTEX_PROFILE_DELAY", "10ms")
	t.Setenv("HAMI_MUTEX_PROFILE_WINDOW", "50ms")
	// Contention during the window, so the profile is non-empty.
	var mu sync.Mutex
	stop := make(chan struct{})
	defer close(stop)
	var acquired atomic.Uint64
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go blockOnMutex(&mu, stop, &acquired, &wg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := startHeapBudgetMonitor(ctx, 1<<30, cancel)
	want := profilePathForPID(path)
	deadline := time.Now().Add(5 * time.Second)
	for {
		// The JSON is written last, after the profile itself, so
		// waiting for it implies the profile exists too.
		if _, err := os.Stat(want + ".json"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("mutex profile %s (+ companion JSON) did not appear within 5s", want)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("mutex profile %s did not appear: %v", want, err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("heap budget monitor did not stop after cancel")
	}
}
