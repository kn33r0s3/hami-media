package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// burnCPUForProfile spins until stop closes. The CPU-profile test
// runs it during the profile window so the profile holds samples
// from a named function of the test binary itself.
func burnCPUForProfile(stop <-chan struct{}, counter *atomic.Uint64) {
	for {
		select {
		case <-stop:
			return
		default:
		}
		counter.Add(1)
	}
}

// TestWriteM7HCPUProfileSameBinarySymbols (5.8): a 100 ms CPU
// profile of the test binary resolves symbols from the same binary
// (the burn function appears in `go tool pprof -top`), and the
// companion JSON's binary sha equals the test binary's own sha.
func TestWriteM7HCPUProfileSameBinarySymbols(t *testing.T) {
	stop := make(chan struct{})
	var counter atomic.Uint64
	go burnCPUForProfile(stop, &counter)
	path := filepath.Join(t.TempDir(), "cpu.pprof")
	err := writeM7HCPUProfile(context.Background(), path, 100*time.Millisecond)
	close(stop)
	if err != nil {
		t.Fatalf("writeM7HCPUProfile: %v", err)
	}
	if got := counter.Load(); got == 0 {
		t.Fatal("burn goroutine did no work during the profile window")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat profile: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("CPU profile is empty")
	}
	encoded, err := os.ReadFile(path + ".json")
	if err != nil {
		t.Fatalf("read companion JSON: %v", err)
	}
	var metadata cpuProfileMetadata
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		t.Fatalf("parse companion JSON: %v", err)
	}
	if metadata.Window != 100*time.Millisecond {
		t.Fatalf("metadata window = %v, want 100ms", metadata.Window)
	}
	if metadata.PublisherStart != nil {
		t.Fatal("metadata publisher start must be nil when no publisher sent")
	}
	if metadata.InWindow {
		t.Fatal("metadata in-window must be false when the publisher start is unknown")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatalf("read test binary: %v", err)
	}
	sum := sha256.Sum256(data)
	wantSHA := hex.EncodeToString(sum[:])
	if metadata.BinarySHA != wantSHA {
		t.Fatalf("metadata binary sha = %q, want the test binary's sha %q", metadata.BinarySHA, wantSHA)
	}
	goBinary, err := exec.LookPath("go")
	if err != nil {
		t.Logf("go tool not on PATH; skipping pprof -top check (profile %s, sha %s)", path, wantSHA)
		return
	}
	top, err := exec.Command(goBinary, "tool", "pprof", "-top", os.Args[0], path).CombinedOutput()
	if err != nil {
		t.Fatalf("go tool pprof -top: %v\n%s", err, top)
	}
	t.Logf("CPU profile top:\n%s", top)
	if !strings.Contains(string(top), "burnCPUForProfile") {
		t.Fatalf("pprof -top does not show the test binary's burnCPUForProfile:\n%s", top)
	}
}

// TestCPUProfileMetadataInWindow (5.8): the in-window bool is the
// overlap of the profile window with the send window.
func TestCPUProfileMetadataInWindow(t *testing.T) {
	origStart := cpuProfilePublisherStartNanos.Load()
	origDuration := cpuProfileSendDurationNanos.Load()
	defer func() {
		cpuProfilePublisherStartNanos.Store(origStart)
		cpuProfileSendDurationNanos.Store(origDuration)
	}()
	now := time.Now()
	publisherStart := now.Add(-10 * time.Second)
	cpuProfilePublisherStartNanos.Store(publisherStart.UnixNano())
	cpuProfileSendDurationNanos.Store((60 * time.Second).Nanoseconds())
	dir := t.TempDir()
	// Profile starts inside the send window (send ends at +50 s).
	metadata, err := writeCPUProfileMetadata(filepath.Join(dir, "in.pprof"), now, 10*time.Second)
	if err != nil {
		t.Fatalf("writeCPUProfileMetadata: %v", err)
	}
	if !metadata.InWindow {
		t.Fatal("profile starting inside the send window must be in-window")
	}
	if metadata.PublisherStart == nil || !metadata.PublisherStart.Equal(publisherStart.UTC()) {
		t.Fatalf("metadata publisher start = %v, want %v", metadata.PublisherStart, publisherStart.UTC())
	}
	// Profile starts after the send window ends (+50 s).
	metadata, err = writeCPUProfileMetadata(filepath.Join(dir, "out.pprof"), now.Add(61*time.Second), 10*time.Second)
	if err != nil {
		t.Fatalf("writeCPUProfileMetadata: %v", err)
	}
	if metadata.InWindow {
		t.Fatal("profile starting after the send window must not be in-window")
	}
	// Unknown publisher start: never in-window.
	cpuProfilePublisherStartNanos.Store(0)
	metadata, err = writeCPUProfileMetadata(filepath.Join(dir, "unknown.pprof"), now, 10*time.Second)
	if err != nil {
		t.Fatalf("writeCPUProfileMetadata: %v", err)
	}
	if metadata.InWindow || metadata.PublisherStart != nil {
		t.Fatalf("unknown publisher start must give in-window=false, got %+v", metadata)
	}
}
