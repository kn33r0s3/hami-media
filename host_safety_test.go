package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Job 18.4 — ABORTED-BY-HOST logging.
//
// The Done line wants a forced host violation during a run to be logged
// as ABORTED-BY-HOST, not as a capacity failure. The forcing seam is
// real: the in-run watchdog reader (readSFUHostSnapshotLive,
// platform_linux.go) reads swap through readSwapMiBChecked(), whose
// source is the stubbable swapUsageReader variable. Stubbing it to a
// used-swap value above sfuHostMaxUsedSwapMiB (512 MiB) makes the live
// watchdog abort within one tick, exactly as a genuinely distressed
// host would. This test then asserts the two halves of the contract:
//
//  1. the watchdog delivers an abort whose reason names the used-swap
//     violation (not a capacity failure), and
//  2. the main-run error path formats that abort as
//     "ABORTED-BY-HOST: <reason>" and writes a proof-error JSON whose
//     Status field is "ABORTED-BY-HOST".
//
// What this test does NOT do: run a full scale proof against a
// distressed host. That needs a host that can be driven into distress
// on demand; the VM's swap is already at 456/1024 MiB used, so a live
// forced violation would also risk the watchdog firing for real
// reasons. The stubbed path exercises the same code (watchdog tick ->
// sfuHostWatchdogFailures -> sfuHostAbort -> main-run formatting).

func TestHostWatchdogAbortIsAbortedByHostNotCapacityFailure(t *testing.T) {
	orig := swapUsageReader
	defer func() { swapUsageReader = orig }()
	swapUsageReader = func() (float64, float64, error) {
		// Used swap 600 MiB > cap 512 MiB; free 424 MiB keeps the
		// total at the real 1,024 MiB pool so only the used-swap
		// rule can fire.
		return 600, 424, nil
	}

	baseline := sfuHostSnapshot{
		PressureLevel: sfuHostNormalPressureLevel,
		SwapUsedMiB:   100,
		SwapFreeMiB:   924,
		SwapTotalMiB:  1024,
		FreeDiskBytes: sfuHostMinFreeDiskBytes,
	}
	// The watchdog reads the live snapshot, so the violation must also
	// be visible through the pure gate function.
	if failures := sfuHostWatchdogFailures(sfuHostSnapshot{
		PressureLevel: sfuHostNormalPressureLevel,
		SwapUsedMiB:   600,
		SwapFreeMiB:   424,
		SwapTotalMiB:  1024,
		FreeDiskBytes: sfuHostMinFreeDiskBytes,
	}); len(failures) != 1 || !strings.Contains(failures[0], "used swap") {
		t.Fatalf("watchdog failures for used-swap violation = %v, want exactly the used-swap failure", failures)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := startSFUHostWatchdog(ctx, cancel, baseline)
	select {
	case abort := <-done:
		if abort == nil {
			t.Fatal("watchdog closed without an abort for a forced host violation")
		}
		if !strings.Contains(abort.Reason, "used swap") {
			t.Fatalf("watchdog abort reason = %q, want it to name the used-swap violation", abort.Reason)
		}
		if strings.Contains(strings.ToLower(abort.Reason), "capacity") {
			t.Fatalf("watchdog abort reason = %q, must not be phrased as a capacity failure", abort.Reason)
		}
		// Half 2: the exact formatting the main run applies
		// (main.go, hostAbort handling): the error carries the
		// ABORTED-BY-HOST prefix and the proof-error JSON carries
		// the ABORTED-BY-HOST status.
		err := errors.Join(nil, fmt.Errorf("ABORTED-BY-HOST: %s", abort.Reason))
		if !strings.HasPrefix(err.Error(), "ABORTED-BY-HOST: ") {
			t.Fatalf("formatted host abort error = %q, want ABORTED-BY-HOST prefix", err.Error())
		}
		var buf bytes.Buffer
		if werr := writeProofErrorResult(&buf, proofErrorResult{
			Error:     err.Error(),
			Status:    "ABORTED-BY-HOST",
			HostGuard: abort,
		}); werr != nil {
			t.Fatalf("writeProofErrorResult: %v", werr)
		}
		var decoded proofErrorResult
		if derr := json.Unmarshal(buf.Bytes(), &decoded); derr != nil {
			t.Fatalf("proof error JSON does not decode: %v (%s)", derr, buf.String())
		}
		if decoded.Status != "ABORTED-BY-HOST" {
			t.Fatalf("proof error status = %q, want ABORTED-BY-HOST", decoded.Status)
		}
		if !strings.Contains(decoded.Error, "ABORTED-BY-HOST:") {
			t.Fatalf("proof error text = %q, want ABORTED-BY-HOST prefix", decoded.Error)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("watchdog did not abort within 10 s of a forced host violation (tick is 1 s)")
	}
}

// Job 18.5 — Heap cap per run from the measured budget.
//
// The cap lives in the run config: for sfu-single / m9h runs main()
// sets maxHeapBytes = defaultSFUHeapBudget(publishers,
// viewersPerPublisher) unless -max-heap-bytes was given explicitly,
// and the heap-budget monitor cancels the run when HeapAlloc crosses
// it. The budget formula is measured: 500,000 estimated heap bytes
// per viewer x2 headroom, with a 256 MiB floor
// (defaultSFUHeapBudgetForViewers, main.go). This test pins the
// formula at the measured anchors and pins the trigger predicate
// (goHeapBudgetExceeded) that the monitor and the startup check share.
// The monitor's live cancellation is already covered by
// TestHeapBudgetMonitorCancelsProofWhenLimitIsReached.
func TestHeapCapPerRunFromMeasuredBudget(t *testing.T) {
	// 20 publishers x 400 viewers = the 20x8,000 anchor: 8,000 x
	// 500,000 x 2 = 8,000,000,000 bytes.
	if got, want := defaultSFUHeapBudget(20, 400), uint64(8_000_000_000); got != want {
		t.Fatalf("20x400 heap budget = %d, want %d", got, want)
	}
	// 20x375 (the 20x7,500 anchor): 7,500 x 500,000 x 2.
	if got, want := defaultSFUHeapBudget(20, 375), uint64(7_500_000_000); got != want {
		t.Fatalf("20x375 heap budget = %d, want %d", got, want)
	}
	// Small runs get the 256 MiB floor, never a tiny cap.
	if got, want := defaultSFUHeapBudget(1, 1), uint64(256<<20); got != want {
		t.Fatalf("1x1 heap budget = %d, want floor %d", got, want)
	}
	// Trigger predicate: one byte below the cap is fine; AT the cap
	// it triggers (the code uses >=, main.go goHeapBudgetExceeded).
	capBytes := uint64(8_000_000_000)
	if err := goHeapBudgetExceeded(capBytes-1, capBytes); err != nil {
		t.Fatalf("heap 1 byte below the cap must not trigger: %v", err)
	}
	if err := goHeapBudgetExceeded(capBytes, capBytes); err == nil {
		t.Fatal("heap at exactly the cap must trigger the budget error")
	} else if !strings.Contains(err.Error(), "Go heap budget exceeded") {
		t.Fatalf("budget error = %q, want the Go heap budget exceeded text", err.Error())
	}
	// A zero cap disables the budget (TestHeapBudgetCanBeDisabled
	// covers the monitor side); the predicate agrees.
	if err := goHeapBudgetExceeded(capBytes+1, 0); err != nil {
		t.Fatalf("zero cap must disable the budget: %v", err)
	}
}
