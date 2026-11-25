package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"
)

// runtimeResourcesSample is one per-process resource sample (Job 5.7):
// goroutine count, Go stack memory in use, heap, and RSS. The heap
// budget monitor logs one per second during a run, per process — so
// each cell produces its own samples and the per-cell goroutine /
// stack figures can be reconciled against the ladder evidence
// (75,184 goroutines, 32 MB stacks) instead of guessed.
type runtimeResourcesSample struct {
	ObservedAt time.Time `json:"observed_at_utc"`
	Goroutines int       `json:"goroutines"`
	StackInuse uint64    `json:"stack_inuse_bytes"`
	StackSys   uint64    `json:"stack_sys_bytes"`
	HeapAlloc  uint64    `json:"heap_alloc_bytes"`
	RSSBytes   uint64    `json:"rss_bytes"`
	RSSError   string    `json:"rss_error,omitempty"`
}

// sampleRuntimeResources reads the current process's goroutine count,
// Go stack + heap memory, and RSS in one place. RSS comes from
// readProcessRSSBytes (platform-specific: /proc/self/statm on Linux,
// ps on Darwin); a failure there is recorded in RSSError and leaves
// RSSBytes at 0 rather than failing the sample.
func sampleRuntimeResources() runtimeResourcesSample {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	sample := runtimeResourcesSample{
		ObservedAt: time.Now().UTC(),
		Goroutines: runtime.NumGoroutine(),
		StackInuse: memory.StackInuse,
		StackSys:   memory.StackSys,
		HeapAlloc:  memory.HeapAlloc,
	}
	rss, err := readProcessRSSBytes()
	if err != nil {
		sample.RSSError = err.Error()
	} else {
		sample.RSSBytes = rss
	}
	return sample
}

// logRuntimeResourcesSample writes one sample as a JSON log line.
func logRuntimeResourcesSample(sample runtimeResourcesSample) {
	encoded, err := json.Marshal(sample)
	if err != nil {
		fmt.Fprintf(os.Stderr, "runtime resources sample: encode: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "runtime_resources_sample=%s\n", encoded)
}
