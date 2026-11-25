# VM baseline — full test suite, 2026-11-25 (job 1.5)

Historical evidence note: raw log availability is tracked in
[`../../log-evidence-status.md`](../../log-evidence-status.md).

- Tree: `joblist-tree-a`, `main.go` sha256
  `9b253fafc2a73a251101fd4435c98ec2c6e17b0f07d94594b6d918b2294fd19a`
  (pre-change, before the Section-2 fixes).
- Command: `go test -v -count=1 ./...`, Go 1.27.4 (linux/amd64),
  module Go 1.24, Pion v4.2.26. Package time 122.269 s.
- Raw log: `/tmp/clusterA-baseline-verbose.log` (kept outside the
  repo; the per-test table below is derived from it).
- Totals: 156 test entries (top-level + subtests): 150 PASS, 6 FAIL,
  0 SKIP.

## The 6 failures — all the known sandbox UDP blocks, not code

The VM sandbox blocks unconnected UDP sends (`sendto: operation not
permitted`) and ICE pairs never connect. All six fail at the UDP
send / ICE-connect step with no application error:

| Test | Duration | Failure signature |
|---|---|---|
| TestLocalPairReceivesAudioAndVideoRTP | 12.00 s | ICE pair failed |
| TestCountingUDPMuxCountsPeerWritesWithoutClosingSharedMux | 0.00 s | sendto: operation not permitted |
| TestPacketReadCountingConnCountsDatagrams | 0.01 s | sendto: operation not permitted |
| TestLocalSFURoutesAuthorizedStreamsAndRTCP | 20.00 s | ICE pair failed |
| TestLocalSFUCascadeRoutesRTCPAcrossWorkers | 30.00 s | ICE pair failed |
| TestSFUTURNRelayCarriesMedia | 60.00 s | ICE/TURN pair failed (context deadline) |

## Post-fix re-run (same day, after the Section-2 fixes)

`go test -count=1 ./...`, package time 123.111 s, exit 1 — the
same six failures and no new ones. Raw log:
`log (sha256 7b19b53445e9, 82 lines; kept outside repo)`. A second
post-fix re-run (`log (sha256 ae6328a5f260, 83 lines; kept outside repo)`, 122.995 s)
failed the known 6 plus the 2 profile flakes documented under
the race section below.

## Race run (job 1.6)

`go test -race -count=1 ./...`, package time 123.329 s, exit 1.
Raw log: `log (sha256 04292a7ccc3b, 85 lines; kept outside repo)`.
**Zero `DATA RACE` lines — no data races detected.** 9 failures:
the known 6 UDP blocks plus 3 timing-sensitive flakes that pass
in isolation and pass on re-run (5/5 in a 5× repeat of the same
3-test selection):

- `TestHeapBudgetMonitorCancelsProofWhenLimitIsReached` —
  "proof context error = <nil>, want context cancellation".
  Mechanism (pre-existing, main.go ~1493): the monitor goroutine
  sends on `done` BEFORE calling `cancel()`, so the test can
  check `ctx.Err()` before the cancel lands. Proposed fix (not
  applied here — outside sections 0–2): call `cancel()` before
  `done <- err`.
- `TestM7HRemoteSFUGoroutineProfileWritesLocalFile` and
  `TestM7HRemoteSFUCPUProfileWritesPrivateLocalFile` — 400
  "profile path must be under a temporary directory" in the
  full-suite run only; the same path shape passes
  `isM7HTemporaryProfilePath` in a direct check and both tests
  pass alone, in pairs, and in a 5× repeat of the 3-test
  selection. A second full-suite re-run reproduced both failures
  again (only the known 6 + these 2 failed; heap and egress
  tests passed). No `TMPDIR`/`Setenv`/`Chdir` manipulation exists
  in any test file, and the `RemoveAll` calls in main.go touch
  renderer recording directories, not other tests' temp dirs.
  Correlated observation, NOT a proven cause: /tmp (512 MiB
  tmpfs) was 96–99% full during the failing runs. Mechanism
  unproven — recorded as flaky-under-load, not diagnosed, and
  not fixed by guessing.

The first race attempt failed to BUILD only because the 512 MiB
/tmp tmpfs ran out of space; the re-run used
`GOTMPDIR=$HOME/.cache/go-tmp`.

## Per-test table

| Test | Result | Duration (s) |
|---|---|---|
| TestSFUTokenMintVerifyRoundTrip | PASS | 0.00 |
| TestSFUTokenExpiredRejected | PASS | 0.00 |
| TestSFUTokenWrongKeyRejected | PASS | 0.00 |
| TestSFUTokenMissingRejected | PASS | 0.00 |
| TestSFUTokenWrongRoomRejected | PASS | 0.00 |
| TestSFUTokenWrongRoleRejected | PASS | 0.00 |
| TestSFUTokenTamperedPayloadRejected | PASS | 0.00 |
| TestLoadSFUAuthKeyFromFileAndEnv | PASS | 0.00 |
| TestSFUHTTPAuthPublisher | PASS | 0.01 |
| TestSFUHTTPAuthViewer | PASS | 0.00 |
| TestDeriveCapacityLimitsReferenceDevice | PASS | 0.00 |
| TestDeriveCapacityLimitsScalesDownWithCoresAndRAM | PASS | 0.00 |
| TestDeriveCapacityLimitsUsesTheWeakerAxis | PASS | 0.00 |
| TestDeriveCapacityLimitsNeverScalesUp | PASS | 0.00 |
| TestDeriveCapacityLimitsRejectsEmptyProfile | PASS | 0.00 |
| TestCapacityAnchorsPassingMaximaExcludePendingAndFailing | PASS | 0.00 |
| TestResolveCapacityLimitsExplicitOverrideIsNeverLowered | PASS | 0.00 |
| TestResolveCapacityLimitsAutoOffUsesAnchorMaxima | PASS | 0.00 |
| TestResolveCapacityLimitsRejectsNegativeExplicitValues | PASS | 0.00 |
| TestResolveCapacityLimitsExplicitZeroMeansUnlimited | PASS | 0.00 |
| TestWriteCapacityReportReferenceDevice | PASS | 0.00 |
| TestWriteCapacityReportWeakDeviceShowsScaleDown | PASS | 0.00 |
| TestDetectDeviceProfileReturnsPlausibleValues | PASS | 0.00 |
| TestRunCapacityCommandPrintsReport | PASS | 0.00 |
| TestCapacityServerLimitReached | PASS | 0.00 |
| TestCapacityPublisherLimitEnforcedOverHTTP | PASS | 0.01 |
| TestPercentile | PASS | 0.00 |
| TestFrameClock | PASS | 0.00 |
| TestFrameClockSharedSubscribers | PASS | 0.00 |
| TestAudioFrameClockSharedSubscribers | PASS | 0.00 |
| TestRecorderModeFallback | PASS | 0.00 |
| TestRecorderMatchedAndUnmatched | PASS | 0.00 |
| TestSimulatedPass | PASS | 0.00 |
| TestSimulatedFailHighLatency | PASS | 0.00 |
| TestSimulatedFailTooFewFrames | PASS | 0.00 |
| TestSimulatedFailTooShort | PASS | 0.00 |
| TestSamplesJSONL | PASS | 0.00 |
| TestRecorderConcurrent | PASS | 0.00 |
| TestClassifyPacketLayerMedia | PASS | 0.00 |
| TestClassifyPacketLayerMedia/opus_audio | PASS | 0.00 |
| TestClassifyPacketLayerMedia/PCMU_audio | PASS | 0.00 |
| TestClassifyPacketLayerMedia/H264_SPS | PASS | 0.00 |
| TestClassifyPacketLayerMedia/H264_PPS | PASS | 0.00 |
| TestClassifyPacketLayerMedia/H264_STAP-A_SPS+PPS | PASS | 0.00 |
| TestClassifyPacketLayerMedia/H264_IDR_single_NAL | PASS | 0.00 |
| TestClassifyPacketLayerMedia/H264_IDR_STAP-A | PASS | 0.00 |
| TestClassifyPacketLayerMedia/H264_IDR_FU-A_start | PASS | 0.00 |
| TestClassifyPacketLayerMedia/H264_non-IDR_single_NAL | PASS | 0.00 |
| TestClassifyPacketLayerMedia/H264_FU-A_continuation | PASS | 0.00 |
| TestClassifyPacketLayerMedia/H264_FU-A_start_non-IDR | PASS | 0.00 |
| TestClassifyPacketLayerMedia/H264_empty_payload | PASS | 0.00 |
| TestClassifyPacketLayerMedia/VP8_keyframe | PASS | 0.00 |
| TestClassifyPacketLayerMedia/VP8_interframe | PASS | 0.00 |
| TestClassifyPacketLayerMedia/VP9_unknown_video | PASS | 0.00 |
| TestClassifyPacketLayerMedia/unknown_codec_safe_default | PASS | 0.00 |
| TestClassifyPacketLayerMedia/empty_codec_safe_default | PASS | 0.00 |
| TestClassifyPacketLayerMedia/nil_packet | PASS | 0.00 |
| TestPacketLayerClassifierKeepsFragmentedFramesTogether | PASS | 0.00 |
| TestPacketLayerClassifierKeepsFragmentedFramesTogether/H264_IDR_FU-A | PASS | 0.00 |
| TestPacketLayerClassifierKeepsFragmentedFramesTogether/VP8_key_and_inter_frames | PASS | 0.00 |
| TestRememberPacketCachesEveryH264IDRFragment | PASS | 0.00 |
| TestRememberPacketCachesDeltaLayer | PASS | 0.00 |
| TestEnqueueRTPDropsDeltaFirst | PASS | 0.00 |
| TestEnqueueRTPFallsBackToOldestWhenNoDelta | PASS | 0.00 |
| TestShardEnqueueDropsDeltaFirst | PASS | 0.00 |
| TestLayeredStreamStaysInOrder | PASS | 0.00 |
| TestLocalPairReceivesAudioAndVideoRTP | FAIL | 12.00 |
| TestReadH264FramesPreservesNonIDRSlices | PASS | 0.00 |
| TestCountingUDPMuxCountsPeerWritesWithoutClosingSharedMux | FAIL | 0.00 |
| TestPacketReadCountingConnCountsDatagrams | FAIL | 0.00 |
| TestAggregateSFUSocketCountsDeduplicatesSharedRouterSocket | PASS | 0.00 |
| TestLocalSFURoutesAuthorizedStreamsAndRTCP | FAIL | 20.00 |
| TestLocalSFUCascadeRoutesRTCPAcrossWorkers | FAIL | 30.00 |
| TestProcessNACKBudgetIsBoundedAndReleases | PASS | 0.00 |
| TestEgressShardDistributionIsStableAndBounded | PASS | 0.01 |
| TestVideoFrameAssemblerRequiresCompleteContiguousFrame | PASS | 0.00 |
| TestVideoFrameAssemblerLateCompletionRecoversMarkerLoss | PASS | 0.00 |
| TestVideoFrameAssemblerLateCompletionRecoversMissedFrames | PASS | 0.00 |
| TestNACKGapTrackerSingleAttemptNoRetry | PASS | 0.00 |
| TestParseSFUFrameDiagnosticIDsDeduplicatesAndSorts | PASS | 0.00 |
| TestSFUFrameDiagnosticRecorderFiltersAndBoundsJSONL | PASS | 0.00 |
| TestNACKQueueDropsOldestAtCapacity | PASS | 0.00 |
| TestRTPQueueDropsOldestPacketWithoutBlocking | PASS | 0.00 |
| TestRTPQueueSharesPayloadAndCopiesMutableHeader | PASS | 0.00 |
| TestH264IDRDetectionSupportsSingleNALAggregationAndFUStart | PASS | 0.00 |
| TestH264IDRDetectionSupportsSingleNALAggregationAndFUStart/IDR_single_NAL | PASS | 0.00 |
| TestH264IDRDetectionSupportsSingleNALAggregationAndFUStart/non-IDR_single_NAL | PASS | 0.00 |
| TestH264IDRDetectionSupportsSingleNALAggregationAndFUStart/IDR_STAP-A | PASS | 0.00 |
| TestH264IDRDetectionSupportsSingleNALAggregationAndFUStart/IDR_FU-A_start | PASS | 0.00 |
| TestH264IDRDetectionSupportsSingleNALAggregationAndFUStart/IDR_FU-A_continuation | PASS | 0.00 |
| TestKeyframeRecoverySkipsInterFramesButAllowsParameterSetsAndIDR | PASS | 0.00 |
| TestKeyframeRecoverySkipsInterFramesButAllowsParameterSetsAndIDR/non-IDR_slice | PASS | 0.00 |
| TestKeyframeRecoverySkipsInterFramesButAllowsParameterSetsAndIDR/SPS | PASS | 0.00 |
| TestKeyframeRecoverySkipsInterFramesButAllowsParameterSetsAndIDR/PPS | PASS | 0.00 |
| TestKeyframeRecoverySkipsInterFramesButAllowsParameterSetsAndIDR/SPS_FU-A_continuation | PASS | 0.00 |
| TestKeyframeRecoverySkipsInterFramesButAllowsParameterSetsAndIDR/IDR_single_NAL | PASS | 0.00 |
| TestKeyframeRecoveryForFragmentedIDRCompletesAtMarker | PASS | 0.00 |
| TestKeyframeRecoveryForFragmentedIDRCompletesAtMarker/orphan_continuation | PASS | 0.00 |
| TestKeyframeRecoveryForFragmentedIDRCompletesAtMarker/IDR_start | PASS | 0.00 |
| TestKeyframeRecoveryForFragmentedIDRCompletesAtMarker/IDR_continuation | PASS | 0.00 |
| TestKeyframeRecoveryForFragmentedIDRCompletesAtMarker/IDR_end | PASS | 0.00 |
| TestStaleRTPIsIntentionallyShedAndMarksKeyframeRecovery | PASS | 0.00 |
| TestKeyframeRecoveryDurationIsRecordedAfterRecovery | PASS | 0.00 |
| TestRecoveryKeyframeRequestsAreLimitedPerPublisher | PASS | 0.00 |
| TestMeasurementPassAllowsIntentionalRTPSheddingButRejectsWriteErrors | PASS | 0.00 |
| TestSFUDissolveScoringUsesSyntheticDeliveredLatencyAndConfiguredLimits | PASS | 0.00 |
| TestSFUDissolveMetricsCountQueueDropsOnce | PASS | 0.00 |
| TestSFUHostPreflightParsesCountersAndEnforcesStops | PASS | 0.00 |
| TestReadSwapMiBCheckedZeroTotalFailsClosed | PASS | 0.00 |
| TestParseVMStatFreeMiBUsesHeaderPageSize | PASS | 0.00 |
| TestDefaultSFUHeapBudgetUsesViewerEstimateWithSmallRunFloor | PASS | 0.00 |
| TestCachedPacketsSharePayloadAndCopyMutableHeader | PASS | 0.00 |
| TestBuildSFUScaleProofConfigSupportsTwentyByFiveHundred | PASS | 0.00 |
| TestBuildSFUScaleProofConfigEnforcesTotalViewerLimit | PASS | 0.00 |
| TestBuildM9HProofConfigModelsSelectiveNineteenThousandParticipantEvent | PASS | 0.00 |
| TestProofFailureIsWrittenAsJSON | PASS | 0.00 |
| TestHeapBudgetMonitorCancelsProofWhenLimitIsReached | PASS | 0.10 |
| TestHeapBudgetCanBeDisabled | PASS | 0.00 |
| TestM7HHeapProfileOffsetsFitRunDuration | PASS | 0.00 |
| TestM7HHeapProfileOffsetsFitRunDuration/short_run_omits_scheduled_captures | PASS | 0.00 |
| TestM7HHeapProfileOffsetsFitRunDuration/one_capture_with_time_to_complete | PASS | 0.00 |
| TestM7HHeapProfileOffsetsFitRunDuration/all_captures_with_time_to_complete | PASS | 0.00 |
| TestWaitForSFUFeedbackReturnsPartialCountsOnContextTimeout | PASS | 0.00 |
| TestFeedbackPacingErrorDistinguishesDeadlineFromCancellation | PASS | 0.00 |
| TestFeedbackPacingErrorDistinguishesDeadlineFromCancellation/deadline | PASS | 0.00 |
| TestFeedbackPacingErrorDistinguishesDeadlineFromCancellation/parent_cancellation | PASS | 0.00 |
| TestIncompleteM7HViewerIDsSelectsOneUnmetTrack | PASS | 0.00 |
| TestM7HStatsHandlerStopsAfterRequestCancellation | PASS | 0.00 |
| TestNACKWorkerStopsAfterSubscriptionCloses | PASS | 0.00 |
| TestNACKSelectionDeduplicatesCapsAndPrioritizesRecentSequence | PASS | 0.00 |
| TestFilterUpstreamForwardableDedupsWithinWindow | PASS | 0.00 |
| TestIDRPhaseStaggerSpreadsFirstIDR | PASS | 0.00 |
| TestResetFeedbackReceivedClearsPLIFIRDedup | PASS | 0.00 |
| TestNACKExpectedSequenceFilterPreservesOnlyProofRequests | PASS | 0.00 |
| TestNACKRTCPPacketsSplitAtReportLimitWithoutLosingSequences | PASS | 0.00 |
| TestPrometheusMetricsExposeForwardingAndResourceSignals | PASS | 0.00 |
| TestPublishedTrackPacketCacheAccountingHandlesReplacementAndEviction | PASS | 0.00 |
| TestMetricsEndpointIsLoopbackOnlyAndServesPrometheus | PASS | 0.00 |
| TestM7HSFUTokenPipeRoundTrip | PASS | 0.00 |
| TestReadM7HSFUTokenFile | PASS | 0.00 |
| TestM7HRemoteSFUCommandHasNoTokenInArgs | PASS | 0.00 |
| TestPairNegotiationTimeoutNamesPairAndStates | PASS | 0.00 |
| TestM7HSFUReceiveStallDetectorRequiresArmedConsecutiveSamples | PASS | 0.00 |
| TestM7HSFUReceiveStallIgnoresExpiredSendContext | PASS | 0.00 |
| TestM7HPublisherSendStallRequiresActiveSendContext | PASS | 0.00 |
| TestM7HRemoteSFUGoroutineProfileWritesLocalFile | PASS | 0.01 |
| TestM7HRemoteSFUGoroutineProfileRejectsNonTemporaryPath | PASS | 0.00 |
| TestM7HRemoteSFUCPUProfileWritesPrivateLocalFile | PASS | 0.10 |
| TestM7HRemoteSFUCPUProfileRejectsDurationOverLimit | PASS | 0.00 |
| TestVideoFrameAssemblerLateRetransmitAfterNewerFramesStarted | PASS | 0.00 |
| TestVideoFrameAssemblerLateRetransmitBeforeNextFrameMerges | PASS | 0.00 |
| TestVideoFrameAssemblerLateRetransmitCompletesOnRetransmitArrival | PASS | 0.00 |
| TestSFUTURNConfigFromEnv | PASS | 0.00 |
| TestLoadSFUTURNPasswordRejectsGroupReadableFile | PASS | 0.00 |
| TestSFUTURNWrongCredentialsRejected | PASS | 0.00 |
| TestSFUTURNRelayCarriesMedia | FAIL | 60.00 |
