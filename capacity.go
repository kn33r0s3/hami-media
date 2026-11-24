// capacity.go — automatic device-capability detector for hami-media.
//
// The detector reads what the machine actually is (CPU, RAM, swap, file
// descriptor limits, UDP buffers, network interfaces) — reusing the
// platform readers in platform_darwin.go / platform_linux.go /
// platform_windows.go where they already exist — and turns it into the
// recommended max publishers and max subscribers (viewers) for the
// m7h-sfu serve path.
//
// The recommendation comes ONLY from the measured anchor table below
// (capacityAnchors). The rules are:
//
//   - NEVER extrapolate beyond measured anchors. A device stronger than
//     the reference Mac gets the anchor maxima, never more.
//   - Scale DOWN for weaker devices relative to the reference Mac, never
//     up. The scale factor is min(cores/10, RAM/24 GiB), clamped to 1.
//   - The scale-down is a conservative heuristic, not a measurement: the
//     anchors were measured on one device only (the reference Mac), so a
//     weaker device gets proportionally less and the result is labelled
//     as derived, not measured.
//   - A pending anchor is never used by the derivation. The
//     10 publishers x 6,000 viewers slot is reserved for a measurement
//     that is still running; until it lands as PASS it must not be
//     claimed as measured and must not raise any limit.
//
// Reference device: Apple M5, 10 cores, 24 GiB RAM (Niroj's Mac).
// Measured anchors (strict gate: p99 <= 150 ms, zero lost frames):
//
//   - 20 publishers x 5,000 viewers — PASS strict.
//   - 20 publishers x 7,500 viewers — PASS strict, three times.
//   - 20 publishers x 10,000 viewers — FAIL, systematically, at the
//     macOS kernel loopback limit (~98% ingress wait). That is NOT a
//     code limit, and 10,000 viewers must never be presented as
//     achievable on this Mac.
//
// An operator's explicit -max-publishers / -max-subscribers value is
// never silently lowered: it overrides the auto-detected value as-is
// (see resolveCapacityLimits). If it exceeds the highest measured
// anchor, startup logs a warning saying so — it does not clamp it.

package main

import (
	"fmt"
	"io"
	"net"
	"runtime"
	"strings"
)

// capacityAnchorResult is the outcome of one measured anchor run.
type capacityAnchorResult string

const (
	// capacityAnchorPass is a strict-gate pass (p99 <= 150 ms, zero
	// lost frames). Only passing anchors feed the derivation.
	capacityAnchorPass capacityAnchorResult = "pass"
	// capacityAnchorFail is a systematic fail. Failing anchors cap
	// the derivation: nothing above the highest passing anchor is
	// ever recommended.
	capacityAnchorFail capacityAnchorResult = "fail"
	// capacityAnchorPending is a reserved slot for a measurement
	// that has not landed yet. Pending anchors are listed in the
	// capacity report but never used by the derivation.
	capacityAnchorPending capacityAnchorResult = "pending"
)

// capacityAnchor is one measured (or reserved) publishers x viewers
// point. Viewers is the TOTAL viewer count across all publishers.
type capacityAnchor struct {
	Publishers int
	Viewers    int
	Result     capacityAnchorResult
	Runs       int
	Note       string
}

// capacityAnchors is the single source of truth for the derivation.
// Do not add a row without a measurement behind it, and do not change
// a Result without the run that justifies it.
//
// SLOT: the 10 x 6,000 row is PENDING — a separate measurement agent
// is verifying that bar right now. Do NOT flip it to pass, and do NOT
// claim 10 x 6,000 as measured, until that measurement lands.
var capacityAnchors = []capacityAnchor{
	{
		Publishers: 20, Viewers: 5000, Result: capacityAnchorPass, Runs: 1,
		Note: "PASS strict (p99 <= 150 ms, zero lost frames)",
	},
	{
		Publishers: 20, Viewers: 7500, Result: capacityAnchorPass, Runs: 3,
		Note: "PASS strict (p99 <= 150 ms, zero lost frames), three times",
	},
	{
		Publishers: 20, Viewers: 10000, Result: capacityAnchorFail, Runs: 0,
		Note: "FAIL systematically at the macOS kernel loopback limit (~98% ingress wait); NOT a code limit — 10,000 viewers is not achievable on this Mac",
	},
	{
		Publishers: 10, Viewers: 6000, Result: capacityAnchorPending, Runs: 0,
		Note: "PENDING — measurement in progress; NOT measured yet, not used by the derivation",
	},
}

// capacityReferenceCores and capacityReferenceRAMGiB describe the
// device the anchors were measured on: Apple M5, 10 cores, 24 GiB RAM.
const (
	capacityReferenceCores  = 10
	capacityReferenceRAMGiB = 24.0
)

// capacityAnchorMaxPublishers is the highest publisher count among
// PASSING anchors. It is computed from the table, not hard-coded, so
// a future passing anchor updates the derivation — but a pending or
// failing anchor never does.
func capacityAnchorMaxPublishers() int {
	max := 0
	for _, anchor := range capacityAnchors {
		if anchor.Result == capacityAnchorPass && anchor.Publishers > max {
			max = anchor.Publishers
		}
	}
	return max
}

// capacityAnchorMaxViewers is the highest total viewer count among
// PASSING anchors, computed from the table like
// capacityAnchorMaxPublishers.
func capacityAnchorMaxViewers() int {
	max := 0
	for _, anchor := range capacityAnchors {
		if anchor.Result == capacityAnchorPass && anchor.Viewers > max {
			max = anchor.Viewers
		}
	}
	return max
}

// deviceNIC is one network interface as seen by the detector.
type deviceNIC struct {
	Name      string
	MTU       int
	Up        bool
	Loopback  bool
	Addresses []string
}

// deviceProfile is everything the detector reads about the machine.
// Fields a platform cannot read stay zero and are reported as
// unknown, never invented.
type deviceProfile struct {
	OS       string
	Kernel   string
	CPUModel string
	CPUCores int
	// RAMTotalMiB is total physical memory.
	RAMTotalMiB uint64
	// Swap figures reuse readAudioSwapMiB (platform readers).
	SwapUsedMiB  float64
	SwapFreeMiB  float64
	SwapTotalMiB float64
	// FDSoftLimit / FDHardLimit are the RLIMIT_NOFILE soft/hard
	// limits for this process (0 = unknown).
	FDSoftLimit uint64
	FDHardLimit uint64
	// FDMaxFiles is the system-wide file cap (kern.maxfiles on
	// darwin, fs.file-max on Linux; 0 = unknown).
	FDMaxFiles uint64
	// UDPRecvBufferBytes / UDPSendBufferBytes are the system UDP
	// buffer ceilings (net.inet.udp.recvspace on darwin,
	// net.core.rmem_max / wmem_max on Linux; 0 = unknown).
	UDPRecvBufferBytes int
	UDPSendBufferBytes int
	NICs               []deviceNIC
}

// capacityLimits is the detector's recommendation. A zero
// MaxPublishers or MaxSubscribers means UNLIMITED — that value is
// only ever produced by an explicit operator override (see
// resolveCapacityLimits), never by the auto derivation.
type capacityLimits struct {
	MaxPublishers  int
	MaxSubscribers int
	// ScaleFactor is the down-scale applied to the anchor maxima
	// (1.0 on the reference device; never above 1.0).
	ScaleFactor float64
	// AutoDetected is true when the values came from the detector
	// and false when they are the anchor maxima used with
	// -auto-capacity=false.
	AutoDetected bool
}

// detectDeviceProfile reads the live device profile. It reuses the
// platform readers (readAudioSwapMiB) and the per-platform
// readDeviceProfilePlatform hook; NIC enumeration is shared because
// net.Interfaces works on every platform.
func detectDeviceProfile() (deviceProfile, error) {
	profile := deviceProfile{
		OS:       runtime.GOOS,
		CPUCores: runtime.NumCPU(),
	}
	if err := readDeviceProfilePlatform(&profile); err != nil {
		return profile, err
	}
	swapUsed, swapFree, err := readAudioSwapMiB()
	if err != nil {
		return profile, fmt.Errorf("read device swap usage: %w", err)
	}
	profile.SwapUsedMiB = swapUsed
	profile.SwapFreeMiB = swapFree
	profile.SwapTotalMiB = swapUsed + swapFree
	interfaces, err := net.Interfaces()
	if err != nil {
		return profile, fmt.Errorf("list device network interfaces: %w", err)
	}
	for _, iface := range interfaces {
		nic := deviceNIC{
			Name:     iface.Name,
			MTU:      iface.MTU,
			Up:       iface.Flags&net.FlagUp != 0,
			Loopback: iface.Flags&net.FlagLoopback != 0,
		}
		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				nic.Addresses = append(nic.Addresses, addr.String())
			}
		}
		profile.NICs = append(profile.NICs, nic)
	}
	return profile, nil
}

// deriveCapacityLimits turns a device profile into recommended
// limits, using only the passing anchors:
//
//	factor = min(cores / 10, RAM GiB / 24), clamped to [0, 1]
//	publishers = floor(anchorMaxPublishers * factor), at least 1
//	viewers    = floor(anchorMaxViewers * factor), at least 1
//
// Worked for the reference Mac (10 cores, 24 GiB RAM):
// factor = min(10/10, 24/24) = 1.0, so 20 publishers and 7,500
// viewers — exactly the highest passing anchors, nothing more.
// A 5-core / 12 GiB device gets factor 0.5: 10 publishers, 3,750
// viewers. A stronger device still gets 20 / 7,500: the factor is
// clamped at 1, so the detector never recommends past a measurement.
func deriveCapacityLimits(profile deviceProfile) (capacityLimits, error) {
	if profile.CPUCores <= 0 {
		return capacityLimits{}, fmt.Errorf("derive capacity limits: CPU core count is %d, want at least 1", profile.CPUCores)
	}
	if profile.RAMTotalMiB == 0 {
		return capacityLimits{}, fmt.Errorf("derive capacity limits: total RAM is unknown (0 MiB)")
	}
	coreRatio := float64(profile.CPUCores) / float64(capacityReferenceCores)
	ramRatio := float64(profile.RAMTotalMiB) / (capacityReferenceRAMGiB * 1024)
	factor := min(coreRatio, ramRatio)
	if factor > 1 {
		factor = 1
	}
	if factor <= 0 {
		return capacityLimits{}, fmt.Errorf("derive capacity limits: scale factor is %g (cores=%d, RAM=%d MiB)", factor, profile.CPUCores, profile.RAMTotalMiB)
	}
	publishers := int(float64(capacityAnchorMaxPublishers()) * factor)
	if publishers < 1 {
		publishers = 1
	}
	viewers := int(float64(capacityAnchorMaxViewers()) * factor)
	if viewers < 1 {
		viewers = 1
	}
	return capacityLimits{
		MaxPublishers:  publishers,
		MaxSubscribers: viewers,
		ScaleFactor:    factor,
		AutoDetected:   true,
	}, nil
}

// deriveCapacityNotes returns the human-readable notes for a derived
// recommendation: how it was scaled, and any preflight bar the
// device is already below. These are warnings, not silent
// adjustments — the numbers are never changed by a note.
func deriveCapacityNotes(profile deviceProfile, limits capacityLimits) []string {
	var notes []string
	if limits.ScaleFactor < 1 {
		notes = append(notes, fmt.Sprintf(
			"scaled down by %.2f from the reference Mac (cores %d/10, RAM %.1f/24 GiB); this is a conservative heuristic, not a measurement",
			limits.ScaleFactor, profile.CPUCores, float64(profile.RAMTotalMiB)/1024))
	} else {
		notes = append(notes, "reference device (or stronger): recommended limits are exactly the highest passing anchors, never above them")
	}
	if profile.SwapTotalMiB > 0 && profile.SwapFreeMiB < float64(sfuHostMinFreeSwapMiB) {
		notes = append(notes, fmt.Sprintf(
			"WARNING: free swap is %.0f MiB, below the %d MiB scale-proof preflight bar (sfuHostMinFreeSwapMiB); a scale run would refuse to start",
			profile.SwapFreeMiB, sfuHostMinFreeSwapMiB))
	}
	for _, anchor := range capacityAnchors {
		if anchor.Result == capacityAnchorPending {
			notes = append(notes, fmt.Sprintf(
				"anchor %d publishers x %d viewers is PENDING — measurement in progress, not used and not claimed as measured",
				anchor.Publishers, anchor.Viewers))
		}
	}
	return notes
}

// resolveCapacityLimits combines the auto detection with the
// operator's explicit flags, for the m7h-sfu serve path.
//
//   - auto=true: start from deriveCapacityLimits(profile).
//   - auto=false: start from the anchor maxima (20 publishers,
//     7,500 viewers) and say so in the notes.
//   - An explicit -max-publishers / -max-subscribers value replaces
//     the corresponding auto value as-is — it is NEVER lowered, even
//     when it exceeds the highest measured anchor. That case is
//     reported in the notes as a warning, not clamped.
//   - An explicit 0 means unlimited for that limit.
//   - A negative explicit value is an error.
func resolveCapacityLimits(auto bool, publishersExplicit bool, publishers int, subscribersExplicit bool, subscribers int, profile deviceProfile) (capacityLimits, []string, error) {
	if publishersExplicit && publishers < 0 {
		return capacityLimits{}, nil, fmt.Errorf("resolve capacity limits: -max-publishers must not be negative, got %d", publishers)
	}
	if subscribersExplicit && subscribers < 0 {
		return capacityLimits{}, nil, fmt.Errorf("resolve capacity limits: -max-subscribers must not be negative, got %d", subscribers)
	}
	var limits capacityLimits
	var notes []string
	if auto {
		derived, err := deriveCapacityLimits(profile)
		if err != nil {
			return capacityLimits{}, nil, err
		}
		limits = derived
		notes = append(notes, deriveCapacityNotes(profile, derived)...)
	} else {
		limits = capacityLimits{
			MaxPublishers:  capacityAnchorMaxPublishers(),
			MaxSubscribers: capacityAnchorMaxViewers(),
			ScaleFactor:    1,
			AutoDetected:   false,
		}
		notes = append(notes, "auto-capacity is off: using the highest measured anchors (never above a measurement)")
	}
	if publishersExplicit {
		if publishers == 0 {
			notes = append(notes, "explicit -max-publishers=0: publisher limit disabled (unlimited)")
		} else {
			notes = append(notes, fmt.Sprintf("explicit -max-publishers=%d overrides the auto-detected %d (never silently lowered)", publishers, limits.MaxPublishers))
			if publishers > capacityAnchorMaxPublishers() {
				notes = append(notes, fmt.Sprintf("WARNING: explicit -max-publishers=%d exceeds the highest measured anchor (%d publishers) — unmeasured, use at your own risk", publishers, capacityAnchorMaxPublishers()))
			}
		}
		limits.MaxPublishers = publishers
	}
	if subscribersExplicit {
		if subscribers == 0 {
			notes = append(notes, "explicit -max-subscribers=0: subscriber limit disabled (unlimited)")
		} else {
			notes = append(notes, fmt.Sprintf("explicit -max-subscribers=%d overrides the auto-detected %d (never silently lowered)", subscribers, limits.MaxSubscribers))
			if subscribers > capacityAnchorMaxViewers() {
				notes = append(notes, fmt.Sprintf("WARNING: explicit -max-subscribers=%d exceeds the highest measured anchor (%d viewers) — unmeasured, use at your own risk", subscribers, capacityAnchorMaxViewers()))
			}
		}
		limits.MaxSubscribers = subscribers
	}
	return limits, notes, nil
}

// capacityLimitString renders one limit for the startup log and the
// capacity report: a number, or "unlimited" for 0.
func capacityLimitString(limit int) string {
	if limit == 0 {
		return "unlimited"
	}
	return fmt.Sprintf("%d", limit)
}

// writeCapacityReport prints the detected device, the recommended
// limits, and the anchors they came from, to w. It is the output of
// the `hami-media capacity` subcommand.
func writeCapacityReport(w io.Writer, profile deviceProfile, limits capacityLimits, notes []string) {
	fmt.Fprintf(w, "hami-media capacity\n")
	fmt.Fprintf(w, "device: %s / %s %s / %d cores / %.1f GiB RAM\n",
		profile.CPUModel, profile.OS, profile.Kernel, profile.CPUCores, float64(profile.RAMTotalMiB)/1024)
	fmt.Fprintf(w, "swap: used %.0f MiB, free %.0f MiB, total %.0f MiB\n",
		profile.SwapUsedMiB, profile.SwapFreeMiB, profile.SwapTotalMiB)
	if profile.FDSoftLimit > 0 || profile.FDMaxFiles > 0 {
		fmt.Fprintf(w, "file descriptors: soft limit %d, hard limit %d, system max %d\n",
			profile.FDSoftLimit, profile.FDHardLimit, profile.FDMaxFiles)
	} else {
		fmt.Fprintf(w, "file descriptors: unknown on this platform\n")
	}
	if profile.UDPRecvBufferBytes > 0 || profile.UDPSendBufferBytes > 0 {
		fmt.Fprintf(w, "UDP buffers: receive %d bytes, send %d bytes\n",
			profile.UDPRecvBufferBytes, profile.UDPSendBufferBytes)
	} else {
		fmt.Fprintf(w, "UDP buffers: unknown on this platform\n")
	}
	for _, nic := range profile.NICs {
		kind := "up"
		if !nic.Up {
			kind = "down"
		}
		if nic.Loopback {
			kind += ", loopback"
		}
		fmt.Fprintf(w, "nic: %s (mtu %d, %s) %s\n",
			nic.Name, nic.MTU, kind, strings.Join(nic.Addresses, " "))
	}
	fmt.Fprintf(w, "recommended max publishers: %s\n", capacityLimitString(limits.MaxPublishers))
	fmt.Fprintf(w, "recommended max subscribers (viewers): %s\n", capacityLimitString(limits.MaxSubscribers))
	fmt.Fprintf(w, "scale factor: %.2f (reference device: Apple M5, 10 cores, 24 GiB RAM)\n", limits.ScaleFactor)
	fmt.Fprintf(w, "anchors used:\n")
	for _, anchor := range capacityAnchors {
		runs := fmt.Sprintf("%d run(s)", anchor.Runs)
		if anchor.Result == capacityAnchorPending {
			runs = "0 runs yet"
		}
		fmt.Fprintf(w, "  %d publishers x %d viewers — %s (%s): %s\n",
			anchor.Publishers, anchor.Viewers, anchor.Result, runs, anchor.Note)
	}
	if len(notes) > 0 {
		fmt.Fprintf(w, "notes:\n")
		for _, note := range notes {
			fmt.Fprintf(w, "  - %s\n", note)
		}
	}
}

// runCapacityCommand implements the `hami-media capacity`
// subcommand: detect the device, derive the limits, print the
// report. A detection failure is an error here — printing limits
// for a device that could not be read would be invented numbers.
func runCapacityCommand(w io.Writer) error {
	profile, err := detectDeviceProfile()
	if err != nil {
		return fmt.Errorf("detect device capability: %w", err)
	}
	limits, err := deriveCapacityLimits(profile)
	if err != nil {
		return err
	}
	writeCapacityReport(w, profile, limits, deriveCapacityNotes(profile, limits))
	return nil
}
