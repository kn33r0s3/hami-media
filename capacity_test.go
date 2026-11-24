package main

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
	"time"
)

// referenceDeviceProfile is the device the anchors were measured on:
// Apple M5, 10 cores, 24 GiB RAM.
func referenceDeviceProfile() deviceProfile {
	return deviceProfile{
		OS:                 "darwin",
		Kernel:             "24.6.0",
		CPUModel:           "Apple M5",
		CPUCores:           10,
		RAMTotalMiB:        24 * 1024,
		SwapUsedMiB:        0,
		SwapFreeMiB:        4096,
		SwapTotalMiB:       4096,
		FDSoftLimit:        256,
		FDHardLimit:        10240,
		FDMaxFiles:         524288,
		UDPRecvBufferBytes: 262144,
		NICs: []deviceNIC{
			{Name: "lo0", MTU: 16384, Up: true, Loopback: true, Addresses: []string{"127.0.0.1"}},
			{Name: "en0", MTU: 1500, Up: true, Addresses: []string{"192.168.1.2"}},
		},
	}
}

func TestDeriveCapacityLimitsReferenceDevice(t *testing.T) {
	limits, err := deriveCapacityLimits(referenceDeviceProfile())
	if err != nil {
		t.Fatalf("deriveCapacityLimits(reference): %v", err)
	}
	if limits.MaxPublishers != 20 || limits.MaxSubscribers != 7500 {
		t.Fatalf("reference limits = %d publishers / %d subscribers, want 20 / 7500",
			limits.MaxPublishers, limits.MaxSubscribers)
	}
	if limits.ScaleFactor != 1 {
		t.Fatalf("reference scale factor = %g, want 1", limits.ScaleFactor)
	}
	if !limits.AutoDetected {
		t.Fatal("reference limits AutoDetected = false, want true")
	}
}

func TestDeriveCapacityLimitsScalesDownWithCoresAndRAM(t *testing.T) {
	// Half the reference device on both axes: factor 0.5.
	profile := referenceDeviceProfile()
	profile.CPUCores = 5
	profile.RAMTotalMiB = 12 * 1024
	limits, err := deriveCapacityLimits(profile)
	if err != nil {
		t.Fatalf("deriveCapacityLimits(half): %v", err)
	}
	if limits.MaxPublishers != 10 || limits.MaxSubscribers != 3750 {
		t.Fatalf("half-device limits = %d publishers / %d subscribers, want 10 / 3750",
			limits.MaxPublishers, limits.MaxSubscribers)
	}
	if limits.ScaleFactor != 0.5 {
		t.Fatalf("half-device scale factor = %g, want 0.5", limits.ScaleFactor)
	}
}

func TestDeriveCapacityLimitsUsesTheWeakerAxis(t *testing.T) {
	// Plenty of cores, little RAM: RAM is the binding constraint.
	// factor = min(10/10, 4/24) = 1/6: 20/6 = 3 publishers,
	// 7500/6 = 1250 viewers.
	profile := referenceDeviceProfile()
	profile.CPUCores = 10
	profile.RAMTotalMiB = 4 * 1024
	limits, err := deriveCapacityLimits(profile)
	if err != nil {
		t.Fatalf("deriveCapacityLimits(RAM-bound): %v", err)
	}
	if limits.MaxPublishers != 3 || limits.MaxSubscribers != 1250 {
		t.Fatalf("RAM-bound limits = %d publishers / %d subscribers, want 3 / 1250",
			limits.MaxPublishers, limits.MaxSubscribers)
	}
}

func TestDeriveCapacityLimitsNeverScalesUp(t *testing.T) {
	// A device far stronger than the reference Mac still gets the
	// anchor maxima — never more than a measurement.
	profile := referenceDeviceProfile()
	profile.CPUModel = "hypothetical 32-core server"
	profile.CPUCores = 32
	profile.RAMTotalMiB = 128 * 1024
	limits, err := deriveCapacityLimits(profile)
	if err != nil {
		t.Fatalf("deriveCapacityLimits(stronger): %v", err)
	}
	if limits.MaxPublishers != 20 || limits.MaxSubscribers != 7500 {
		t.Fatalf("stronger-device limits = %d publishers / %d subscribers, want 20 / 7500 (never above the anchors)",
			limits.MaxPublishers, limits.MaxSubscribers)
	}
	if limits.ScaleFactor != 1 {
		t.Fatalf("stronger-device scale factor = %g, want 1 (clamped)", limits.ScaleFactor)
	}
}

func TestDeriveCapacityLimitsRejectsEmptyProfile(t *testing.T) {
	profile := referenceDeviceProfile()
	profile.CPUCores = 0
	if _, err := deriveCapacityLimits(profile); err == nil {
		t.Fatal("deriveCapacityLimits(0 cores): got no error, want one")
	}
	profile = referenceDeviceProfile()
	profile.RAMTotalMiB = 0
	if _, err := deriveCapacityLimits(profile); err == nil {
		t.Fatal("deriveCapacityLimits(0 RAM): got no error, want one")
	}
}

func TestCapacityAnchorsPassingMaximaExcludePendingAndFailing(t *testing.T) {
	// The 20 x 10,000 FAIL and the 10 x 6,000 PENDING rows must not
	// move the maxima: the derivation caps at 20 publishers and
	// 7,500 viewers.
	if got := capacityAnchorMaxPublishers(); got != 20 {
		t.Fatalf("capacityAnchorMaxPublishers() = %d, want 20", got)
	}
	if got := capacityAnchorMaxViewers(); got != 7500 {
		t.Fatalf("capacityAnchorMaxViewers() = %d, want 7500", got)
	}
	pending := 0
	for _, anchor := range capacityAnchors {
		if anchor.Result == capacityAnchorPending {
			pending++
			if anchor.Publishers != 10 || anchor.Viewers != 6000 {
				t.Fatalf("pending anchor = %d x %d, want the reserved 10 x 6000 slot",
					anchor.Publishers, anchor.Viewers)
			}
		}
	}
	if pending != 1 {
		t.Fatalf("pending anchors = %d, want exactly 1 (the 10 x 6000 slot)", pending)
	}
}

func TestResolveCapacityLimitsExplicitOverrideIsNeverLowered(t *testing.T) {
	profile := referenceDeviceProfile()
	// Explicit values above the anchors are honoured as-is, with a
	// warning — never clamped down to the auto values.
	limits, notes, err := resolveCapacityLimits(true, true, 50, true, 9000, profile)
	if err != nil {
		t.Fatalf("resolveCapacityLimits(explicit above anchors): %v", err)
	}
	if limits.MaxPublishers != 50 || limits.MaxSubscribers != 9000 {
		t.Fatalf("explicit limits = %d / %d, want 50 / 9000 (never silently lowered)",
			limits.MaxPublishers, limits.MaxSubscribers)
	}
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "exceeds the highest measured anchor") {
		t.Fatalf("explicit-above-anchor notes = %q, want a warning that it exceeds the measured anchors", joined)
	}
	// Explicit values below the auto values are honoured too.
	limits, _, err = resolveCapacityLimits(true, true, 5, true, 100, profile)
	if err != nil {
		t.Fatalf("resolveCapacityLimits(explicit below auto): %v", err)
	}
	if limits.MaxPublishers != 5 || limits.MaxSubscribers != 100 {
		t.Fatalf("explicit limits = %d / %d, want 5 / 100", limits.MaxPublishers, limits.MaxSubscribers)
	}
}

func TestResolveCapacityLimitsAutoOffUsesAnchorMaxima(t *testing.T) {
	profile := referenceDeviceProfile()
	profile.CPUCores = 2
	profile.RAMTotalMiB = 4 * 1024
	limits, notes, err := resolveCapacityLimits(false, false, 0, false, 0, profile)
	if err != nil {
		t.Fatalf("resolveCapacityLimits(auto off): %v", err)
	}
	if limits.MaxPublishers != 20 || limits.MaxSubscribers != 7500 {
		t.Fatalf("auto-off limits = %d / %d, want the anchor maxima 20 / 7500",
			limits.MaxPublishers, limits.MaxSubscribers)
	}
	if limits.AutoDetected {
		t.Fatal("auto-off limits AutoDetected = true, want false")
	}
	if !strings.Contains(strings.Join(notes, "\n"), "auto-capacity is off") {
		t.Fatalf("auto-off notes = %q, want a note that auto-capacity is off", strings.Join(notes, "\n"))
	}
}

func TestResolveCapacityLimitsRejectsNegativeExplicitValues(t *testing.T) {
	profile := referenceDeviceProfile()
	if _, _, err := resolveCapacityLimits(true, true, -1, false, 0, profile); err == nil {
		t.Fatal("resolveCapacityLimits(-max-publishers=-1): got no error, want one")
	}
	if _, _, err := resolveCapacityLimits(true, false, 0, true, -5, profile); err == nil {
		t.Fatal("resolveCapacityLimits(-max-subscribers=-5): got no error, want one")
	}
}

func TestResolveCapacityLimitsExplicitZeroMeansUnlimited(t *testing.T) {
	profile := referenceDeviceProfile()
	limits, _, err := resolveCapacityLimits(true, true, 0, true, 0, profile)
	if err != nil {
		t.Fatalf("resolveCapacityLimits(explicit 0): %v", err)
	}
	if limits.MaxPublishers != 0 || limits.MaxSubscribers != 0 {
		t.Fatalf("explicit-0 limits = %d / %d, want 0 / 0 (unlimited)",
			limits.MaxPublishers, limits.MaxSubscribers)
	}
	if capacityLimitString(0) != "unlimited" {
		t.Fatalf("capacityLimitString(0) = %q, want \"unlimited\"", capacityLimitString(0))
	}
}

func TestWriteCapacityReportReferenceDevice(t *testing.T) {
	profile := referenceDeviceProfile()
	limits, err := deriveCapacityLimits(profile)
	if err != nil {
		t.Fatalf("deriveCapacityLimits(reference): %v", err)
	}
	var buf bytes.Buffer
	writeCapacityReport(&buf, profile, limits, deriveCapacityNotes(profile, limits))
	report := buf.String()
	for _, want := range []string{
		"Apple M5",
		"10 cores",
		"24.0 GiB RAM",
		"recommended max publishers: 20",
		"recommended max subscribers (viewers): 7500",
		"scale factor: 1.00",
		"20 publishers x 5000 viewers — pass",
		"20 publishers x 7500 viewers — pass",
		"20 publishers x 10000 viewers — fail",
		"10 publishers x 6000 viewers — pending",
		"NOT a code limit",
		"NOT measured yet",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("capacity report is missing %q:\n%s", want, report)
		}
	}
	// The pending 10 x 6,000 row must never appear as a recommendation.
	if strings.Contains(report, "recommended max subscribers (viewers): 6000") {
		t.Fatalf("capacity report presents the pending 10 x 6000 anchor as a recommendation:\n%s", report)
	}
}

func TestWriteCapacityReportWeakDeviceShowsScaleDown(t *testing.T) {
	profile := referenceDeviceProfile()
	profile.CPUCores = 5
	profile.RAMTotalMiB = 12 * 1024
	limits, err := deriveCapacityLimits(profile)
	if err != nil {
		t.Fatalf("deriveCapacityLimits(half): %v", err)
	}
	var buf bytes.Buffer
	writeCapacityReport(&buf, profile, limits, deriveCapacityNotes(profile, limits))
	report := buf.String()
	if !strings.Contains(report, "recommended max publishers: 10") ||
		!strings.Contains(report, "recommended max subscribers (viewers): 3750") {
		t.Fatalf("half-device report has the wrong limits:\n%s", report)
	}
	if !strings.Contains(report, "scaled down by 0.50") {
		t.Fatalf("half-device report does not say it was scaled down:\n%s", report)
	}
}

func TestDetectDeviceProfileReturnsPlausibleValues(t *testing.T) {
	if runtime.GOOS == "windows" {
		if _, err := detectDeviceProfile(); err == nil {
			t.Fatal("detectDeviceProfile(windows): got no error, want the not-supported error")
		}
		return
	}
	profile, err := detectDeviceProfile()
	if err != nil {
		t.Fatalf("detectDeviceProfile(): %v", err)
	}
	if profile.CPUCores <= 0 {
		t.Fatalf("detected CPU cores = %d, want at least 1", profile.CPUCores)
	}
	if profile.RAMTotalMiB == 0 {
		t.Fatal("detected total RAM = 0 MiB, want a real reading")
	}
	if profile.CPUModel == "" {
		t.Fatal("detected CPU model is empty, want a real reading")
	}
	// syscall.Sysctl returns string sysctls NUL-terminated; the
	// reader must have trimmed that (a NUL in the model would
	// corrupt the capacity report).
	if strings.Contains(profile.CPUModel, "\x00") || strings.Contains(profile.Kernel, "\x00") {
		t.Fatalf("detected CPU model %q / kernel %q contain a NUL byte, want trimmed text", profile.CPUModel, profile.Kernel)
	}
	if len(profile.NICs) == 0 {
		t.Fatal("detected no network interfaces, want at least one")
	}
}

func TestRunCapacityCommandPrintsReport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("capacity detection is not supported on windows")
	}
	var buf bytes.Buffer
	if err := runCapacityCommand(&buf); err != nil {
		t.Fatalf("runCapacityCommand(): %v", err)
	}
	if !strings.Contains(buf.String(), "recommended max publishers:") {
		t.Fatalf("capacity subcommand output has no recommended publishers line:\n%s", buf.String())
	}
}

func TestCapacityServerLimitReached(t *testing.T) {
	// The limit predicates both handlers call, at the boundary.
	server := &m7hRemoteSFUServer{
		maxPublishers:  1,
		maxSubscribers: 2,
		publishers:     make(map[string]m7hRemotePublisherState),
		viewers:        make(map[string]viewerEndpoint),
	}
	if server.publisherLimitReached() {
		t.Fatal("publisherLimitReached() with 0/1 publishers = true, want false")
	}
	if server.subscriberLimitReached() {
		t.Fatal("subscriberLimitReached() with 0/2 viewers = true, want false")
	}
	server.publishers["pub-1"] = m7hRemotePublisherState{}
	if !server.publisherLimitReached() {
		t.Fatal("publisherLimitReached() with 1/1 publishers = false, want true")
	}
	server.viewers["viewer-1"] = viewerEndpoint{}
	if server.subscriberLimitReached() {
		t.Fatal("subscriberLimitReached() with 1/2 viewers = true, want false")
	}
	server.viewers["viewer-2"] = viewerEndpoint{}
	if !server.subscriberLimitReached() {
		t.Fatal("subscriberLimitReached() with 2/2 viewers = false, want true")
	}
	// 0 = unlimited on both sides, however full the maps are.
	server.maxPublishers = 0
	server.maxSubscribers = 0
	if server.publisherLimitReached() || server.subscriberLimitReached() {
		t.Fatal("limit of 0 must mean unlimited, but a limit reported reached")
	}
}

func TestCapacityPublisherLimitEnforcedOverHTTP(t *testing.T) {
	// One publisher allowed; the second, fully authorized, gets 503
	// with the limit in the body. This drives handlePublisher
	// end-to-end through the same handlers runM7HSFUServer
	// registers.
	httpServer, verifier := testSFUHTTPServerWithLimits(t, capacityLimits{MaxPublishers: 1})
	offer := testSFUClientOffer(t)
	publish := func(publisherID string) (int, string) {
		token, err := verifier.mintSFUToken("default", publisherID, []string{sfuTokenRolePublish}, time.Hour)
		if err != nil {
			t.Fatalf("mint publish token for %s: %v", publisherID, err)
		}
		return testSFUPostJSON(t, httpServer.URL+"/publisher", "test-bearer", m7hRemoteSDPRequest{
			PublisherID: publisherID,
			Offer:       offer,
			Token:       token,
		})
	}
	status, _ := publish("pub-1")
	if status != 200 {
		t.Fatalf("first publisher: status = %d, want 200", status)
	}
	status, body := publish("pub-2")
	if status != 503 {
		t.Fatalf("second publisher past the limit: status = %d, body = %s, want 503", status, body)
	}
	if !strings.Contains(body, "publisher limit reached") {
		t.Fatalf("second publisher body = %q, want it to name the publisher limit", body)
	}
}
