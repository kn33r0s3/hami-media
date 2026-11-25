# Historical log evidence availability

This register records references in historical documentation whose raw log
files were not found in the repository, `/tmp`, or the saved `hami-results`
evidence directories during the J7 audit. The names are preserved so the
historical report remains traceable; they are not claims that the files are
available or that their contents have been re-verified. No hash or line
count is assigned where the raw file was unavailable.

Recovered logs are kept outside the repository and recorded in the external
J7 manifest with verified SHA-256 hashes and line counts.

## Unavailable exact filenames

- `logs/stage3-failbefore-rerun-003.log`
- `logs/stage3-failbefore-rerun-003-restore.log`
- `stage3-failbefore-rerun-003.log`
- `stage3-failbefore-rerun-003-restore.log`
- `stage3-final-fixed.log`
- `stage3-subhop-001-20261124-213001.log`
- `stage3-subhop-002-20261124-213706.log`
- `stage4-reconnect-001.log`
- `stage5-turn-gate-002.log`
- `stage5-turn-sandbox-002.log`
- `stage5-turn-failbefore-001.log`
- `stage5-sandbox-probe-004.log`
- `stage6-webpage-001.log`
- `stage6-webpage-002.log`
- `capacity-detector-verify-004-20261124-231517.log`
- `cap10x6000-launch-001.log`
- `cap10x6000-run-001.log`
- `cap10x6000-probe-002.log`
- `cap10x6000-kill-001.log`
- `build-tags-mac-001-20261124-214015.log`
- `build-tags-linux-test-003-20261124-155047.log`
- `standalone-verify-001-20261124-225857.log`
- `standalone-resync-vm-test.log`
- `mediamtx-phase1-001.log`
- `mediamtx-phase3-diag-001.log`
- `mediamtx-phase3-load-001.log`
- `mediamtx-phase3b-load-002/003.log`
- `heat-invest-probe-001-20261124-234125.log`
- `peakmem-recheck2-20x7500-swap.log`
- `swap-redo-crosscheck-001.log`
- `resource-stall-20261125-205315.log`
- `stall-evidence-001.log`
- `/tmp/clusterA-baseline-verbose.log`
- `resource-pprof-midmedia-002.log`
- `logs/part3-smoke-006.log`
- `part3-smoke-001.log`
- `ladder-2x50.stderr.log`

The historical report also names many JSON result files, source files, and
some logs for which partial hashes or sizes are recorded in the text. Those
partial identifiers do not substitute for the raw files and were not
reconstructed.

## Templates, not evidence citations

The following are naming examples or globs, not individual missing runs:

- `*-run-001.log`
- `logs/mediamtx-phase0-00{1..7}.log`
- `mediamtx-phase2-00{1..4}.log`
- `mediamtx-phase3-setup-00{1..3}.log`
- `bench-20261126-020000-<sha>.log`
- `bench-20261126-020000-<sha>.preflight.log`

The historical report is retained as a historical record. Do not use these
unavailable names as evidence for a new release decision or benchmark
verdict unless the raw files are recovered and verified.
