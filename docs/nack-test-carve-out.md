# NACK loss test carve-out (Option A)

`TestLocalSFURoutesAuthorizedStreamsAndRTCP` is the end-to-end proof
that a viewer receives a NACK retransmission. It is **skipped in the
default suite** and run separately with the build tag
`hami_disable_srtp_replay`. This note explains why, states the
invariants, and records what the default-build failure means (it is
expected, not a regression).

## Why the carve-out exists

The test proves retransmission by counting **duplicate-sequence**
packets arriving at the viewer. Its viewers NACK sequences they
already received (`sendViewerFeedback`, `main.go:15991-16020`); the
router re-sends those sequences from its cache, and the gate
(`main.go:14621-14622`) requires at least one duplicate to arrive
at the viewer.

Job 2.1 restored SRTP replay protection on viewer clients
(`main.go:12754-12762`, and the same gate at the publisher and
router call sites). With protection on, the viewer's SRTP layer
drops the re-sent copy as a replay before it reaches the track
reader, so the duplicate count stays 0 and the gate fails. The
router sends the retransmission correctly — the router-side gate
(`main.go:14614-14619`, NACK received / retransmitted / forwarded)
passes. The NACK path is healthy; the test method is incompatible
with replay protection. The protection is correct, the test is
stale.

Fail-before is on record: on the reference machine the test failed
deterministically, 5/5 runs at ~11.29 s, at the NACK gate
(`main.go:14622: viewer hami-sfu-viewer-1 did not receive a NACK
retransmission`), while the connections themselves established
(TURN relay pairs succeeded). This is not machine-specific: it
fails on any machine where UDP works. In the VM sandbox it fails
earlier, at ICE negotiation, because the sandbox blocks
unconnected UDP — the tag changes nothing there.

## What the carve-out changes

- `scripts/test-all.sh`: the default unit and race runs pass
  `-skip 'TestLocalSFURoutesAuthorizedStreamsAndRTCP'`; a separate
  step runs only this test with
  `-tags hami_disable_srtp_replay`. The step's failures feed the
  same known-failures summary as the other steps.
- `scripts/release-check.sh`: the default `go test -json` run
  skips this test; a second invocation runs only this test with
  the tag and appends its results to the same JSON the test gate
  parses. The known-failures set is unchanged.
- `.github/workflows/ci.yml`: both jobs skip this test in their
  default test steps and add a tagged step running only this test.

## Invariants — never break these

1. **The tag invocation must `-run` ONLY
   `TestLocalSFURoutesAuthorizedStreamsAndRTCP`.** Never run the
   whole suite under the tag: `TestSRTPReplayProtectionDropsDuplicate`
   fails under the tag **by design** (`srtp_replay.go`), because
   the tag exists precisely to turn replay protection off.
2. **The tag must never be used for the shipped/production
   binary.** It exists only so this one test can observe
   retransmitted duplicates. Production builds always run with
   replay protection on (the default).
3. **In the VM sandbox the tag changes nothing for this test.**
   It still fails at ICE there, so the test stays in the VM
   known-failures set in `scripts/test-all.sh` and
   `scripts/release-check.sh`.
4. **A default-build failure of this test is expected.** If it
   fails on a default build on a machine with working UDP, that
   documents the carve-out — it is not a regression, and it must
   not be "fixed" by weakening the gate or by re-disabling replay
   protection on viewer clients (that would reopen the hole job
   2.1 closed).

## The real fix (Option B, later and separate)

The proper fix makes the proof count a **genuine** retransmission
instead of an artificial duplicate: use the existing fault injector
(`faultinject.go`, job 4.6) to drop one sequence once for one
viewer, let the viewer NACK a sequence it genuinely missed, and
gate on that sequence arriving after the NACK. A first-time arrival
passes SRTP replay protection legitimately, so the test can run
in the default suite with no tag anywhere. When Option B lands,
this carve-out — the skip, the tagged steps, and this note — is
removed.
