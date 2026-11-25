# Job 5.1 — Mutex profile during the media window — DONE (mechanism; router-lock verdict needs a Mac run)

Date: 25 Nov 2026. Tree: `joblist-tree-b/`.

## Verdict: DONE (mechanism proven in the VM; the router-lock
## verdict itself is OPEN for a Mac media-window run)

## What was built

- `writeMutexProfile(ctx, path, window)` (main.go): enables mutex
  profiling for the window only (previous fraction restored after),
  writes the `mutex` profile to `path` (0600) plus a companion JSON
  at `path + ".json"` (`profile_path`, `window_ns`, `started_at_utc`,
  `sample_count`). Window must be in (0, 10 s]; a cancelled context
  ends it early with an error.
- Env trigger beside the CPU-profile block in
  `startHeapBudgetMonitor`: `HAMI_MUTEX_PROFILE_PATH` /
  `HAMI_MUTEX_PROFILE_DELAY` (default 90 s, mid-media) /
  `HAMI_MUTEX_PROFILE_WINDOW` (default 10 s, max 10 s). The path goes
  through `profilePathForPID` (Job 5.7) so cells do not truncate each
  other's file.
- `profilePathForPID` landed (Job 5.7, from the verified per-pid
  patch sha `e0e9bda9…`): the pid goes before the `.pprof`
  extension. Applied to the heap and CPU profile paths too (they
  previously shared one path across parent + cells and produced
  interleaved, unparseable gzip in 4/5 T0–T4 trials).

## Tests (`mutexprofile_test.go`, passing, `-race` clean)

- `TestWriteMutexProfileCapturesContention`: 3 blockers + 1 holder
  on a test mutex, 500 ms window. Profile holds samples
  (1.33 s total in the run logged), companion JSON matches
  (window + sample count), and `go tool pprof -top` on the test
  binary attributes the contention to the lock HOLDER
  (`holdMutexContended`) — the mutex profile attributes contention
  to the code holding the lock too long, which is exactly the
  mechanism 5.1 needs to find a contended router lock. (First draft
  of this test asserted the WAITER's name and failed; the profile
  shows the holder — corrected, not hidden.)
- `TestMutexProfileEnvWiring`: `startHeapBudgetMonitor` with the env
  set writes a per-pid profile + companion JSON.

## OPEN — the router-lock verdict (Mac)

No router lock was contended in the VM test (the contention was
synthetic, on a test mutex), so this job does NOT confirm or refute
router-lock contention. The verdict needs a Mac media-window run:

```
HAMI_MUTEX_PROFILE_PATH=/tmp/<run>.mutex.pprof HAMI_MUTEX_PROFILE_DELAY=50s HAMI_MUTEX_PROFILE_WINDOW=10s \
  ./hami-media -mode=sfu-single -sfu-publishers=20 -sfu-viewers-per-publisher=250 -frame-rate=5 \
  -video-file=<clip> -duration=60s
go tool pprof -top ./hami-media /tmp/<run>.mutex.<pid>.pprof
```

Look for `sfuRouter` / `sfuSubscription` mutex functions in the top
lines. (Delay 50 s lands mid-media for a 60 s run; the existing CPU
guidance says 40–50 s for 60 s runs — see 5.8.) If the router lock
appears in top contention, 5.2 reopens; if it does not, 5.2 stands
closed as no-removal (see the 5.2 note).
