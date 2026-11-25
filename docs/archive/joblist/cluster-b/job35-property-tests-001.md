# Job 3.5 — Randomized reorder/duplicate property tests for the frame assembler — DONE

Date: 25 Nov 2026. Tree: `joblist-tree-b/`
(main.go sha after the fix: see diffstat below; pristine main.go sha
`9b253fafc2a73a251101fd4435c98ec2c6e17b0f07d94594b6d918b2294fd19a`).

## Verdict: DONE

Three real assembler bugs found by the property tests and fixed; all 25
fixed seeds pass, plus the duplicates-only and full-shuffle property
tests, plus every pre-existing FIX 31 assembler test. Full suite after
the fix: exit 1 with exactly the known six VM UDP-block failures
(`log (sha256 01891145e76d, 71 lines; kept outside repo)`, 124.616 s) — no regressions.

## Tests (new file `assembler_test.go`)

- `TestVideoFrameAssemblerPropertyReorderAndDuplicate`: 25 fixed seeds
  (deterministic). Each seed builds 8–20 frames of 1–4 packets from a
  stream starting at a random sequence (even seeds start at 65530−rand(10)
  to exercise the 16-bit wrap), shuffles within sliding 8-packet windows,
  duplicates ~20% of packets at random positions. Assertion: every frame
  completes exactly once (set equality — completion ORDER is not asserted:
  a later frame may complete before an earlier one whose packets are
  still in flight; downstream matching is by RTP timestamp, not arrival
  order).
- `TestVideoFrameAssemblerPropertyDuplicatesOnly`: duplicate packets
  only, no reorder — every frame exactly once.
- `TestVideoFrameAssemblerPropertyFullShuffleCompletesEveryFrame`: full
  shuffle, 10 frames — every frame exactly once.
- Existing 3.4 wrap tests (`TestVideoFrameAssemblerSequenceWraparound`,
  `TestVideoFrameAssemblerSequenceWraparoundReordered`) still pass.

Fail-before (before the fixes, same tests): seed 1 completed 11–13/18
frames, full-shuffle order test failed 6/10, seed 12 lost 2 frames,
seed 14 lost the last frame, seed 17 lost 1 frame, seed 19 (after the
first two fixes) double-reported one frame.

## Bug 1 — full-run completion abandoned skipped frames

`lateCompleteOne`'s full-run branch set `lateFirstSequence = marker+1`
unconditionally. A frame skipped by the scan (its run incomplete, rescue
failed) was abandoned even though its packets were merely reordered:
its late packets then failed span membership and were dropped as stale.
Single-packet frames were lost first (no rescue path).

Fix: clamp the advance to the first sequence in
`[lateFirstSequence, marker]` that has not arrived yet
(`main.go`, `lateCompleteOne` full-run branch); delete the completed
marker from `lateMarkers` (it previously stayed, accumulating stale
markers). Genuinely lost frames are still abandoned by the
`lateSpanMaxSequences` (1024) span-length bound in `mergeLateSpan`.

## Bug 2 — single-packet frames could never rescue

`lateMarkerFrameStart` refused `frameStart == marker`. A single-packet
frame whose run was broken by an earlier sequence that completed through
the ACTIVE path (and so never entered the span, e.g. seed 12 seq 9,
seed 17 seq 17984) could never complete.

Fix: a lone marker rescues when its frame is proven single-packet, by
either of two sound proofs (both rest on frames being sequential and
disjoint, and all packets of one frame sharing one RTP timestamp):
(a) the preceding sequence is present in the span with a different
timestamp — it belongs to an earlier frame, so this frame starts at the
marker; (b) the preceding sequence is the marker of a frame that
already completed — recorded in the new `completedMarkers` map
(`videoFrameAssembler.completedMarkers`, populated by
`recordCompletedMarker` on every completion while a span exists,
cleared with the span in `clearLateSpan`). If the preceding sequence
never arrived, or shares the timestamp, the rescue still refuses.

## Bug 3 — boundary-unknown frame dropped when its span was consumed

A frame finalised before its marker arrived (`frameBoundaryKnown=false`)
merges into the span on its marker — but the span may have been consumed
between its first packet and its marker (the previous frame completed
late, clearing the span). The frame was then finalised, never completed,
never merged (seed 14: last frame 1306000 lost).

Fix: when the span is fully consumed, the fix-up in `lateCompleteOne`
that advances `nextFrameFirstSequence` to the first unconsumed sequence
also sets `frameBoundaryKnown = true` — the consumed span proves the
active frame's start boundary — so the frame completes through the
normal path. (An earlier attempt — merging unconditionally when no span
exists — was REVERTED: it wrongly completed the first frame, whose start
is genuinely unknown; `TestVideoFrameAssemblerRequiresCompleteContiguousFrame`
caught it.)

## Bug 4 (introduced by Bug 2's fix, caught by seed 19) — double report

`completedMarkers` made the rescue sound, but a duplicate marker packet
for a frame that already completed via the ACTIVE path matched the span
(the span still covers its sequences) and re-added its marker, reporting
the frame a second time (seed 19: 1126000 twice).

Fix: in `processPacket`'s span branch, a marker whose sequence is in
`completedMarkers` is not re-added to `lateMarkers` (the sequence itself
is still recorded). The pre-existing duplicate guard only covered
markers already in the span; this covers markers that completed through
the active path.

## Files changed

- `main.go`: `lateCompleteOne` (clamp + marker delete + fix-up flag),
  `lateMarkerFrameStart` (proven-single-packet rescue),
  `processPacket` span branch (completed-marker guard),
  `completeActiveFrameIfReady` + `lateCompleteOne` (record markers),
  new `videoFrameAssembler.completedMarkers` field +
  `recordCompletedMarker`, `clearLateSpan` clears it,
  `mergeLateSpan` initialises it.
- `assembler_test.go` (new): 3.4 wrap tests + 3.5 property tests.
- `zz_debug_test.go` (temporary trace harness): deleted after use.

## Evidence

- Full suite log: `log (sha256 01891145e76d, 71 lines; kept outside repo)`
  (exit 1, exactly the 6 known UDP failures).
- `go vet`: clean. `gofmt -l`: clean.
