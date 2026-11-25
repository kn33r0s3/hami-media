# Job 2.8 decision — Linux swap check (PARTIAL)

## The Done line as written

"Either the audio test skips the swap check on Linux (test it) or
record why the Linux /proc/meminfo reader satisfies the intent —
implement the skip if that's what the Done line says, with a test."

## Decision

**Do NOT implement the skip.** The Linux `/proc/meminfo` reader
(`readAudioSwapMiB` in `platform_linux.go`) already satisfies the
intent: it performs a real swap check on Linux instead of the
macOS-only sysctl path. Skipping the check on Linux would remove
a working safety guard for no benefit — the guard is what refused
the 10×6,000 run when free swap was below the floor, and the
used-swap rule (job 18.2) is built on the same reader.

## What was done instead

- Test `TestAudioSwapCheckRunsOnLinux` (in
  `cluster_a_section2_test.go`) pins the reader against
  `/proc/meminfo` on Linux and skips on non-Linux. It passed in
  the pass-after run (`log (sha256 f4f87c68b338, 52 lines; kept outside repo)`).
- This decision document, so the Done line's alternative branch is
  on record.

## What is still owed (why this job is PARTIAL, not DONE)

The Done line needs rewording by the parent/owner: as written it
says "implement the skip if that's what the Done line says", which
reads as mandating the skip. The recommended rewording: "the audio
test keeps its swap check on Linux via the /proc/meminfo reader;
record why that reader satisfies the intent."
