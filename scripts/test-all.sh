#!/bin/sh
# One test command (job 1.8): unit + race + integration + fuzz smoke,
# then one summary. Run from the repo root:
#   sh scripts/test-all.sh
# The repo's local-pair / SFU / TURN tests are part of the Go test
# suite, not a separate package. Fuzz targets are run one at a time
# from the main package.
#
# Known VM limit: this sandbox blocks unconnected UDP, so exactly
# six tests fail here for that reason, not code. The summary lists
# them separately. On a machine where UDP works they must pass;
# the script exits non-zero whenever any test fails.
#
# NACK carve-out (Option A, docs/nack-test-carve-out.md):
# TestLocalSFURoutesAuthorizedStreamsAndRTCP is skipped in the
# default suite and run separately below with
# -tags hami_disable_srtp_replay. The test counts duplicate-sequence
# packets at the viewer, but job 2.1 restored SRTP replay protection,
# so the viewer's SRTP layer drops the re-sent copy as a replay and
# the gate can never pass on a default build. The tag invocation must
# -run ONLY that test (never the whole suite: the replay test fails
# under the tag by design) and the tag must never be used for a
# shipped/production binary. In this VM the tag changes nothing for
# this test (it still fails at ICE), so it stays in KNOWN below.
set -u
cd "$(dirname "$0")/.."

# The race build writes large work files; /tmp here is a 512 MiB
# tmpfs and runs out of space. Use the home volume unless the
# caller already set GOTMPDIR.
GOTMPDIR="${GOTMPDIR:-$HOME/.cache/go-tmp}"
mkdir -p "$GOTMPDIR"
export GOTMPDIR
export GOFLAGS="${GOFLAGS:--mod=mod}"
export PATH="$HOME/go/bin:$PATH"

KNOWN="TestLocalPairReceivesAudioAndVideoRTP TestCountingUDPMuxCountsPeerWritesWithoutClosingSharedMux TestPacketReadCountingConnCountsDatagrams TestLocalSFURoutesAuthorizedStreamsAndRTCP TestLocalSFUCascadeRoutesRTCPAcrossWorkers TestSFUTURNRelayCarriesMedia"

RESULTS_ROOT="${TEST_ALL_RESULTS_DIR:-results/test-all}"
mkdir -p "$RESULTS_ROOT"
TMPDIR_WORK="$(mktemp -d "$RESULTS_ROOT/$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX")"
printf 'trial_id\t%s\n' "$(basename "$TMPDIR_WORK")" > "$TMPDIR_WORK/steps.tsv"

record_step() {
  printf '%s\t%s\n' "$1" "$2" >> "$TMPDIR_WORK/steps.tsv"
}

overall=0

echo "== leak scan =="
if python3 scripts/leak-scan.py . > "$TMPDIR_WORK/leak.txt" 2>&1; then
  echo "leak scan: PASS"
  record_step leak_scan PASS
else
  echo "leak scan: FAIL (see $TMPDIR_WORK/leak.txt)"
  cat "$TMPDIR_WORK/leak.txt"
  record_step leak_scan FAIL
  overall=1
fi

echo "== gofmt =="
if [ -z "$(gofmt -l .)" ]; then
  echo "gofmt: PASS"
  record_step gofmt PASS
else
  echo "gofmt: FAIL (unformatted: $(gofmt -l . | tr '\n' ' '))"
  record_step gofmt FAIL
  overall=1
fi

echo "== go vet =="
if go vet ./... > "$TMPDIR_WORK/vet.txt" 2>&1; then
  echo "go vet: PASS"
  record_step go_vet PASS
else
  echo "go vet: FAIL (see $TMPDIR_WORK/vet.txt)"
  cat "$TMPDIR_WORK/vet.txt"
  record_step go_vet FAIL
  overall=1
fi

echo "== unit + integration (go test) =="
# The NACK loss test is skipped here; it runs separately below with
# the carve-out tag (docs/nack-test-carve-out.md).
if go test -skip 'TestLocalSFURoutesAuthorizedStreamsAndRTCP' -count=1 ./... > "$TMPDIR_WORK/unit.txt" 2>&1; then
  echo "unit: PASS"
  record_step unit_and_integration PASS
else
  echo "unit: FAIL"
  grep "^--- FAIL" "$TMPDIR_WORK/unit.txt" || true
  record_step unit_and_integration FAIL
  overall=1
fi

echo "== NACK loss test (tagged carve-out, see docs/nack-test-carve-out.md) =="
# -run ONLY this test, never the whole suite: the replay test fails
# under this tag by design. The tag is test-only, never for a
# shipped/production binary.
if go test -tags hami_disable_srtp_replay -run 'TestLocalSFURoutesAuthorizedStreamsAndRTCP' -count=1 ./... > "$TMPDIR_WORK/nack.txt" 2>&1; then
  echo "nack loss test: PASS"
  record_step nack_carve_out PASS
else
  echo "nack loss test: FAIL"
  grep "^--- FAIL" "$TMPDIR_WORK/nack.txt" || true
  record_step nack_carve_out FAIL
  overall=1
fi

echo "== race (go test -race) =="
if go test -race -skip 'TestLocalSFURoutesAuthorizedStreamsAndRTCP' -count=1 ./... > "$TMPDIR_WORK/race.txt" 2>&1; then
  echo "race: PASS (no data races)"
  record_step race PASS
else
  echo "race: FAIL"
  grep -E "DATA RACE|^--- FAIL" "$TMPDIR_WORK/race.txt" | head -n 20 || true
  record_step race FAIL
  overall=1
fi

echo "== fuzz smoke =="
if grep -rq "^func Fuzz" --include="*_test.go" .; then
  # -fuzz accepts exactly one matching target and requires one package.
  for target in $(grep -rh "^func Fuzz" --include="*_test.go" . | sed -E 's/^func ([^(]+).*/\1/'); do
    echo "--- $target ---"
    if go test -run='^$' -fuzz="^${target}$" -fuzztime=10s . > "$TMPDIR_WORK/fuzz-$target.txt" 2>&1; then
      echo "fuzz $target: PASS"
      record_step "fuzz:$target" PASS
    else
      echo "fuzz $target: FAIL (see $TMPDIR_WORK/fuzz-$target.txt)"
      record_step "fuzz:$target" FAIL
      overall=1
    fi
  done
else
  echo "fuzz smoke: SKIPPED — no func Fuzz targets in the tree"
  record_step fuzz_smoke SKIPPED
fi

echo "== summary =="
if [ "$overall" -eq 0 ]; then
  echo "test-all: ALL PASS (leak, gofmt, vet, unit, nack, race, fuzz)"
  record_step overall PASS
else
  fails="$(grep "^--- FAIL" "$TMPDIR_WORK/unit.txt" "$TMPDIR_WORK/race.txt" "$TMPDIR_WORK/nack.txt" 2>/dev/null | sed 's/.*FAIL: \([A-Za-z0-9]*\).*/\1/' | sort -u | tr '\n' ' ')"
  known_only=1
  for f in $fails; do
    case " $KNOWN " in *" $f "*) ;; *) known_only=0 ;; esac
  done
  if [ -n "$fails" ] && [ "$known_only" -eq 1 ]; then
    echo "test-all: FAIL — only the known sandbox UDP failures: $fails"
    echo "  (VM sandbox blocks unconnected UDP; these must pass on a machine where UDP works)"
  elif [ -n "$fails" ]; then
    echo "test-all: FAIL — failing tests: $fails"
  else
    echo "test-all: FAIL — see the sections above"
  fi
  record_step overall FAIL
fi
if ! python3 scripts/write-test-summary.py "$TMPDIR_WORK"; then
  overall=1
fi
echo "test-all evidence: $TMPDIR_WORK"
exit "$overall"
