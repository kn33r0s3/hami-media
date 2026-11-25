# Job 5.8 — CPU profiles inside the media window — DONE

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdict: DONE (routine + same-binary proof; Mac in-window run OPEN)

## The recording routine (`writeCPUProfileMetadata`)

Every `writeM7HCPUProfile` call now writes a companion JSON at
`<path>.json` beside the profile:

- `profile_path` — the profile file.
- `binary_sha256` — sha256 of the running binary (`os.Executable`),
  computed once (`sync.OnceValues`). A profile is provably from the
  same binary as the run when this sha matches the binary's sha —
  not merely its path. A read failure is recorded in
  `binary_sha_error`, not hidden.
- `publisher_start_utc` — the first publisher frame's send time,
  set once by `m7hPublisherSendStats.record` (first call wins via
  `markCPUProfilePublisherStart`, CAS). This is the same "first send
  sample" the stall evidence defines as publisher start. Nil when
  no publisher has sent.
- `profile_start_utc` — when `pprof.StartCPUProfile` began.
- `window_ns` — the profile duration.
- `send_duration_ns` — the run's media duration, set by `main()`
  from `-duration` (the send window is
  [first send, first send + duration]).
- `in_window` — true iff the profile window
  [profileStart, profileStart+window] overlaps the send window.
  False when either endpoint is unknown — recorded, not guessed.

A metadata-write failure is logged to stderr and does not fail the
profile write (the profile already landed).

## Same-binary proof (`cpuprofile_test.go`)

`TestWriteM7HCPUProfileSameBinarySymbols`: a 100 ms profile of the
test binary while `burnCPUForProfile` spins. `go tool pprof -top
<test binary> <profile>` shows `burnCPUForProfile` at 87.50%
(70 ms of 80 ms) — symbols resolve from the same binary — and the
companion JSON's `binary_sha256` equals the test binary's own sha.
`TestCPUProfileMetadataInWindow`: profile inside the send window
→ in-window; profile after it ends → not; unknown publisher start
→ in-window false with a nil start.

## Delay recommendation (for 60 s runs)

The `HAMI_CPU_PROFILE_DELAY` default (90 s) is mid-media for the
long runs it was designed for (setup ~40 s, media ~40–230 s) and is
unchanged. For a 60 s run, use `HAMI_CPU_PROFILE_DELAY=40s..50s`
so the 10 s window sits inside the media window — the stall
evidence shows a 60 s delay lands on the send window's edge. This
is now recorded in the env block's comment.

## OPEN — Mac in-window run

Run a scale proof on the Mac with e.g.
`HAMI_CPU_PROFILE_PATH=/tmp/<run>.cpu.pprof HAMI_CPU_PROFILE_DELAY=45s HAMI_CPU_PROFILE_WINDOW=10s`,
then read `/tmp/<run>.cpu.<pid>.pprof.json`: `in_window` must be
true, `binary_sha256` must match the binary that ran, and
`go tool pprof -top ./hami-media /tmp/<run>.cpu.<pid>.pprof`
must show the run's own symbols (sendPublisherVideo,
forwardTrack, nackWorker) in the top lines.
