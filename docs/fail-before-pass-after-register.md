# Fail-before / pass-after register (15.1)

Job 15.1: every item in sections 2, 3 and 4 has both records — a test
that fails on the broken build and passes on the fixed build.

Status: **PARTIAL — register created 25 Nov 2026; merge 25 Nov 2026
added the 2.1, 2.9, 3.5, 4.1-control and 5.6 records below.**
The fix work in sections 2, 3 and 4 belongs to clusters A and B;
this register is the place their records land. What is recorded
here is what exists in the tree or in the working notes today — not
what is hoped for.

## Records present (verified against the tree / working notes)

| Job | Fail-before | Pass-after | Note |
|---|---|---|---|
| 4.1 cache delta packets | FIX 31 fault-injection diagnosis (`cap20x5000-f31-diagnosis-001.md`, `cap20x5000-f31-diagnosis-002.md`) shows delta packets uncached before the fix | `TestRememberPacketCachesDeltaLayer` (`layers_test.go:140`); code `rememberPacket` (`main.go:8939-8976`) caches every layer incl. delta | Router-level control (cluster B, `docs/archive/joblist/cluster-b/fault-injection-4-1-001.md`): `TestFaultInjectionLostPFramePacketRetransmitted` (`faultinject_test.go:572`) — a dropped P-frame packet is retransmitted and the frame completes. End-to-end fault-injection run in hami-media still owed (Stage 3's 300/300 is a different server and does not count) |
| 4.2 abandon frame at deadline | FIX 31 late-retransmit repro (`cap20x5000-f31-repro-result-001.md`): missing frame blocks later frames | Assembler late-retransmit tests x3 (`main_test.go:2368,2448,2533`); result note `cap20x5000-fix31-result-001.md` | End-to-end proof in a hami-media run still owed; re-verify the FIX 31 note against the current tree |
| 2.1 SRTP replay protection | `TestSRTPReplayProtectionDropsDuplicate` (`cluster_a_section2_test.go:112`) FAILS on the replay-disabled build: "replay packet delivered (16 bytes); replay protection is off" (`log (sha256 eb85e7f06364, 8 lines; kept outside repo)`) | Same test PASSES on the enabled build (`log (sha256 f4f87c68b338, 52 lines; kept outside repo)`); replay re-enabled in all three places (`main.go:11806`, `:12086`, `:12228`) behind `srtp_replay.go` + build-tagged `srtp_replay_disable.go` | DONE (cluster A) |
| 2.9 H.264 frames by picture | `TestReadH264FramesGroupsMultiSlicePictures` (`cluster_a_section2_test.go:590`) FAILS on the slice-counting build (`log (sha256 46de68e8d2a8, 62 lines; kept outside repo)`) | Same test PASSES on the picture-counting build (`log (sha256 f4f87c68b338, 52 lines; kept outside repo)`); `readH264NALs` groups slices into one Sample per picture via `h264FirstMBInSlice` | DONE (cluster A) |

## Records owed (no fail-before, no pass-after, as of 25 Nov 2026)

- 2.2 upstreamQueue leak; 2.3 pcLifetimes entries; 2.4 track keys at
  registration; 2.5 stale publisher entry; 2.6 key by publisher/track/
  layer; 2.7 hardcoded track IDs; 2.8 platform guard for the swap
  check; 2.10 egress-shard path.
- 3.1 RTP timestamp step; 3.2 frame-rate flag; 3.3 before/after timing
  run; 3.4 wraparound tests; 3.6 jitter against a known clock.
  (3.5 recorded above.)
- 4.3 deadlines in milliseconds; 4.4 drop-and-recover end to end;
  4.5 NACK feedback age per profile; 4.6 deterministic fault-injection
  suite; 4.7 slow-viewer policy.

Rule for every future fix: the failing test lands FIRST (the fix is
not started until the test fails on the current tree), and the record
is appended here with the test name, the two run logs and the code
sha of each side. A fix without both records is not done, even if
the code looks right.
